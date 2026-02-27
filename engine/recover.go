package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fulgidus/revoco/cookies"
	"github.com/fulgidus/revoco/metadata"
)

// RecoverConfig holds all options for a recovery run.
type RecoverConfig struct {
	// InputJSON is the path to missing-files.json produced by Phase 8.
	InputJSON string
	// OutputDir is where recovered files are placed.
	OutputDir string
	// CookieJar is the path to the Netscape cookie jar file.
	CookieJar string
	// Concurrency is the number of parallel downloads.
	Concurrency int
	// Delay is the minimum pause between requests (per worker) in seconds.
	Delay float64
	// MaxRetry is the maximum number of attempts per file.
	MaxRetry int
	// StartFrom is the 1-indexed entry to resume from.
	StartFrom int
	// DryRun reports what would be downloaded without doing it.
	DryRun bool
}

// RecoverResult is the final summary of a recovery run.
type RecoverResult struct {
	Downloaded int
	Skipped    int
	Failed     int
	FailedPath string
}

// RecoverEvent is emitted during a recovery run.
type RecoverEvent struct {
	Done    int
	Total   int
	Message string
	IsError bool
}

var videoExtRe = regexp.MustCompile(`(?i)\.(mp4|mov|avi|3gp|mkv|wmv|flv|webm|m4v|mpg|mpeg)$`)

// fifeBaseRe matches a bare fife base URL.
var fifeBaseRe = regexp.MustCompile(`https://photos\.fife\.usercontent\.google\.com/pw/[A-Za-z0-9_\-]+`)

// RunRecover executes the recovery pipeline.
// The events channel is closed when the run completes.
func RunRecover(cfg RecoverConfig, events chan<- RecoverEvent) (*RecoverResult, error) {
	defer close(events)

	emit := func(done, total int, msg string, isErr bool) {
		events <- RecoverEvent{Done: done, Total: total, Message: msg, IsError: isErr}
	}

	// Validate inputs
	if _, err := os.Stat(cfg.InputJSON); err != nil {
		return nil, fmt.Errorf("input JSON not found: %s", cfg.InputJSON)
	}
	if _, err := os.Stat(cfg.CookieJar); err != nil {
		return nil, fmt.Errorf("cookie jar not found: %s", cfg.CookieJar)
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	// Logging
	logPath := filepath.Join(cfg.OutputDir, "recovery.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)
	logger.Printf("=== recovery started ===")

	// Load missing entries
	data, err := os.ReadFile(cfg.InputJSON)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	var entries []metadata.MissingEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}

	total := len(entries)
	if total == 0 {
		emit(0, 0, "No entries to recover", false)
		return &RecoverResult{}, nil
	}

	// Apply start-from offset
	startIdx := cfg.StartFrom - 1
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= total {
		return nil, fmt.Errorf("--start-from %d exceeds total entries (%d)", cfg.StartFrom, total)
	}
	work := entries[startIdx:]
	effectiveTotal := len(work)

	// Build HTTP client with the cookie jar
	cookieJarPath := cfg.CookieJar
	jar, err := cookies.LoadNetscapeCookieJar(cookieJarPath)
	if err != nil {
		return nil, fmt.Errorf("load cookie jar: %w", err)
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return fmt.Errorf("too many redirects")
			}
			// Propagate cookies on redirects
			return nil
		},
	}

	// Concurrency
	concurrency := cfg.Concurrency
	if concurrency < 1 {
		concurrency = 3
	}
	maxRetry := cfg.MaxRetry
	if maxRetry < 1 {
		maxRetry = 3
	}
	delay := cfg.Delay
	if delay <= 0 {
		delay = 1.0
	}

	// Progress counters (protected by mu)
	var mu sync.Mutex
	var countOK, countSkip, countFail int
	var failedEntries []metadata.FailedEntry

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	doneCh := make(chan struct{})
	var processedCount int

	// Progress reporter goroutine
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-doneCh:
				return
			case <-ticker.C:
				mu.Lock()
				done := processedCount
				ok := countOK
				sk := countSkip
				fa := countFail
				mu.Unlock()
				emit(done, effectiveTotal,
					fmt.Sprintf("ok=%d skip=%d fail=%d", ok, sk, fa), false)
			}
		}
	}()

	ctx := context.Background()

	for i, entry := range work {
		entry := entry
		idx := startIdx + i + 1

		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()

			if cfg.DryRun {
				dest := filepath.Join(cfg.OutputDir, entry.Title)
				if fileExists(dest) {
					logger.Printf("[%d] DRY SKIP: %s", idx, entry.Title)
					mu.Lock()
					countSkip++
					processedCount++
					mu.Unlock()
				} else {
					logger.Printf("[%d] DRY WOULD: %s", idx, entry.Title)
					mu.Lock()
					countOK++
					processedCount++
					mu.Unlock()
				}
				return
			}

			var result string
			for attempt := 1; attempt <= maxRetry; attempt++ {
				result = downloadOne(ctx, client, entry, cfg.OutputDir, idx, total, logger)
				if result == "OK" || result == "SKIP" {
					break
				}
				if attempt < maxRetry {
					time.Sleep(time.Duration(attempt*2) * time.Second)
				}
			}

			mu.Lock()
			processedCount++
			switch result {
			case "OK":
				countOK++
			case "SKIP":
				countSkip++
			default:
				countFail++
				failedEntries = append(failedEntries, metadata.FailedEntry{
					MissingEntry: entry,
					Error:        result,
				})
				logger.Printf("[%d] FAILED: %s reason=%s", idx, entry.Title, result)
			}
			mu.Unlock()

			// Rate limiting
			time.Sleep(time.Duration(float64(time.Second) * delay))
		}()
	}

	wg.Wait()
	close(doneCh)

	// Write failed.json
	result := &RecoverResult{
		Downloaded: countOK,
		Skipped:    countSkip,
		Failed:     countFail,
	}
	if len(failedEntries) > 0 {
		failPath := filepath.Join(cfg.OutputDir, "failed.json")
		if fdata, ferr := json.MarshalIndent(failedEntries, "", "  "); ferr == nil {
			_ = os.WriteFile(failPath, fdata, 0o644)
			result.FailedPath = failPath
		}
	}

	logger.Printf("=== recovery done: ok=%d skip=%d fail=%d ===", countOK, countSkip, countFail)
	emit(effectiveTotal, effectiveTotal,
		fmt.Sprintf("Done: %d downloaded, %d skipped, %d failed", countOK, countSkip, countFail), false)
	return result, nil
}

// downloadOne fetches a single Google Photos entry.
// Returns "OK", "SKIP", or an error string.
func downloadOne(ctx context.Context, client *http.Client, entry metadata.MissingEntry,
	outputDir string, idx, total int, logger *log.Logger) string {

	dest := filepath.Join(outputDir, entry.Title)

	// Already downloaded?
	if fileExists(dest) {
		if info, _ := os.Stat(dest); info != nil && info.Size() > 0 {
			return "SKIP"
		}
	}

	// Conflict: file exists but is empty — use photo_id suffix
	if fileExists(dest) {
		photoID := extractPhotoID(entry.URL)
		base := strings.TrimSuffix(entry.Title, filepath.Ext(entry.Title))
		ext := filepath.Ext(entry.Title)
		if len(photoID) > 6 {
			dest = filepath.Join(outputDir, base+"_"+photoID[:6]+ext)
		}
	}

	// Step 1: fetch the photo page
	pageHTML, err := fetchText(ctx, client, entry.URL)
	if err != nil {
		return "FAIL_PAGE:" + err.Error()
	}

	// Step 2: extract fife URL
	photoID := extractPhotoID(entry.URL)
	fifeURL := extractFifeURL(photoID, pageHTML)
	if fifeURL == "" {
		return "FAIL_EXTRACT"
	}

	// Step 3: download with =d or =dv suffix
	dlSuffix := "=d"
	if videoExtRe.MatchString(entry.Title) {
		dlSuffix = "=dv"
	}

	tmpPath := dest + ".tmp"
	if err := downloadToFile(ctx, client, fifeURL+dlSuffix, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "FAIL_DL:" + err.Error()
	}

	// Step 4: validate size
	info, _ := os.Stat(tmpPath)
	if info == nil || info.Size() < 100 {
		_ = os.Remove(tmpPath)
		return fmt.Sprintf("FAIL_SIZE:%d", func() int64 {
			if info != nil {
				return info.Size()
			}
			return 0
		}())
	}

	// Step 5: make sure it's not HTML (auth failure response)
	if isHTMLFile(tmpPath) {
		_ = os.Remove(tmpPath)
		return "FAIL_HTML"
	}

	if err := os.Rename(tmpPath, dest); err != nil {
		_ = os.Remove(tmpPath)
		return "FAIL_RENAME:" + err.Error()
	}

	logger.Printf("[%d/%d] OK: %s (%d bytes)", idx, total, entry.Title, info.Size())
	return "OK"
}

// fetchText does a GET and returns the response body as a string.
func fetchText(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024)) // 4 MB limit for pages
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// downloadToFile streams a URL response to a local file.
func downloadToFile(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

// extractPhotoID extracts the last path segment from a Google Photos URL.
func extractPhotoID(url string) string {
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// extractFifeURL tries three strategies to find the fife CDN URL in a page's HTML.
func extractFifeURL(photoID, html string) string {
	if photoID != "" {
		// Method 1: data-media-key="<id>" ... data-url="<fife>"
		re1 := regexp.MustCompile(`data-media-key="` + regexp.QuoteMeta(photoID) + `"[^>]*data-url="([^"]+)"`)
		if m := re1.FindStringSubmatch(html); len(m) > 1 {
			return m[1]
		}
		// Method 2: reversed attribute order
		re2 := regexp.MustCompile(`data-url="([^"]+)"[^>]*data-media-key="` + regexp.QuoteMeta(photoID) + `"`)
		if m := re2.FindStringSubmatch(html); len(m) > 1 {
			return m[1]
		}
	}
	// Method 3: first unique fife URL in the page
	matches := fifeBaseRe.FindAllString(html, -1)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// isHTMLFile checks if a file starts with HTML markers.
func isHTMLFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n == 0 {
		return false
	}
	snippet := strings.ToLower(strings.TrimSpace(string(buf[:n])))
	return strings.HasPrefix(snippet, "<!doctype html") ||
		strings.HasPrefix(snippet, "<html")
}

// fileExists reports whether a path exists (file or dir).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
