package engine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fulgidus/revoco/filedate"
	"github.com/fulgidus/revoco/internal/tools"
	"github.com/fulgidus/revoco/metadata"
)

// ExifTool manages a persistent exiftool -stay_open process to avoid
// per-file process spawn overhead.
type ExifTool struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	logger *log.Logger
}

// NewExifTool starts a persistent exiftool process.
// The logger is used to capture stderr output from exiftool.
func NewExifTool(logger *log.Logger) (*ExifTool, error) {
	exiftoolBin, err := tools.FindTool("exiftool")
	if err != nil {
		return nil, fmt.Errorf("exiftool not available: %w", err)
	}
	cmd := exec.Command(exiftoolBin, "-stay_open", "true", "-@", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("exiftool stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("exiftool stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("exiftool stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start exiftool: %w", err)
	}

	// Goroutine to capture stderr and route to logger
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if logger != nil {
				logger.Printf("[EXIF] exiftool stderr: %s", scanner.Text())
			}
		}
	}()

	return &ExifTool{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewScanner(stdout),
		logger: logger,
	}, nil
}

// Close shuts down the persistent exiftool process.
func (e *ExifTool) Close() error {
	fmt.Fprintln(e.stdin, "-stay_open")
	fmt.Fprintln(e.stdin, "false")
	fmt.Fprintln(e.stdin, "-execute")
	e.stdin.Close()
	return e.cmd.Wait()
}

// Execute sends a batch of arguments to exiftool and waits for completion.
// Arguments are one per line; exiftool reads until it sees "-execute".
func (e *ExifTool) Execute(args []string) error {
	for _, arg := range args {
		fmt.Fprintln(e.stdin, arg)
	}
	fmt.Fprintln(e.stdin, "-execute")

	// Read output until "{ready}" sentinel
	for e.stdout.Scan() {
		line := e.stdout.Text()
		if strings.HasPrefix(line, "{ready}") {
			return nil
		}
	}
	if err := e.stdout.Err(); err != nil {
		return fmt.Errorf("exiftool output: %w", err)
	}
	return nil
}

// ApplyEXIFFromJSON applies metadata from a Takeout JSON file to its media file.
func (e *ExifTool) ApplyEXIFFromJSON(mediaPath, jsonPath string) error {
	photo, err := metadata.ParseFile(jsonPath)
	if err != nil {
		return fmt.Errorf("parse json %s: %w", jsonPath, err)
	}

	ts := photo.PhotoTakenTime.Unix()
	if ts == 0 {
		ts = photo.CreationTime.Unix()
	}
	if ts == 0 {
		return nil // no timestamp at all — skip
	}

	exifDate := time.Unix(ts, 0).Local().Format("2006:01:02 15:04:05")
	isVideo := isVideoExt(filepath.Ext(mediaPath))

	args := []string{
		"-overwrite_original",
		"-DateTimeOriginal=" + exifDate,
		"-CreateDate=" + exifDate,
		"-ModifyDate=" + exifDate,
	}

	if isVideo {
		args = append(args,
			"-MediaCreateDate="+exifDate,
			"-MediaModifyDate="+exifDate,
			"-TrackCreateDate="+exifDate,
			"-TrackModifyDate="+exifDate,
		)
	}

	// GPS
	geo := photo.BestGeo()
	if !geo.IsZero() {
		latRef := "N"
		lonRef := "E"
		lat := geo.Latitude
		lon := geo.Longitude
		if lat < 0 {
			latRef = "S"
			lat = math.Abs(lat)
		}
		if lon < 0 {
			lonRef = "W"
			lon = math.Abs(lon)
		}
		args = append(args,
			fmt.Sprintf("-GPSLatitude=%.8f", lat),
			"-GPSLatitudeRef="+latRef,
			fmt.Sprintf("-GPSLongitude=%.8f", lon),
			"-GPSLongitudeRef="+lonRef,
			fmt.Sprintf("-GPSAltitude=%.2f", geo.Altitude),
			"-GPSAltitudeRef=0",
		)
	}

	// Description
	if photo.Description != "" {
		args = append(args, "-ImageDescription="+photo.Description)
	}

	// People as keywords
	for _, kw := range photo.Keywords() {
		args = append(args, "-Keywords+="+kw, "-Subject+="+kw)
	}

	args = append(args, mediaPath)

	if err := e.Execute(args); err != nil {
		return fmt.Errorf("exiftool apply %s: %w", mediaPath, err)
	}

	// Set filesystem mtime
	os.Chtimes(mediaPath, time.Unix(ts, 0), time.Unix(ts, 0)) //nolint:errcheck
	return nil
}

// ApplyDateFromFilename extracts a date from the filename and applies it via exiftool.
func (e *ExifTool) ApplyDateFromFilename(mediaPath string) (bool, error) {
	base := filepath.Base(mediaPath)
	t, _ := filedate.Extract(base)
	if t.IsZero() {
		return false, nil
	}

	exifDate := t.Format("2006:01:02 15:04:05")
	isVideo := isVideoExt(filepath.Ext(mediaPath))

	args := []string{
		"-overwrite_original",
		"-DateTimeOriginal=" + exifDate,
		"-CreateDate=" + exifDate,
		"-ModifyDate=" + exifDate,
	}
	if isVideo {
		args = append(args,
			"-MediaCreateDate="+exifDate,
			"-MediaModifyDate="+exifDate,
			"-TrackCreateDate="+exifDate,
			"-TrackModifyDate="+exifDate,
		)
	}
	args = append(args, mediaPath)

	if err := e.Execute(args); err != nil {
		return false, fmt.Errorf("exiftool filename-date %s: %w", mediaPath, err)
	}

	os.Chtimes(mediaPath, t, t) //nolint:errcheck
	return true, nil
}

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".avi": true, ".3gp": true,
	".mkv": true, ".wmv": true, ".flv": true, ".webm": true,
	".m4v": true, ".mpg": true, ".mpeg": true,
}

func isVideoExt(ext string) bool {
	return videoExts[strings.ToLower(ext)]
}

// ApplyEXIFBatch applies metadata to a batch of files using the persistent exiftool process.
// For files with JSON: uses JSON metadata. For files without JSON: falls back to filename date.
// The context allows cancellation during the batch processing loop.
// Returns an error if exiftool is not available (fatal error).
func ApplyEXIFBatch(
	ctx context.Context,
	destMap map[string]string, // src -> dest
	mediaJSON map[string]string, // src -> json path
	dryRun bool,
	logger *log.Logger,
	progress func(done, total int),
) (applied, dateFallback, skipped, errCount int, errs []error, fatalErr error) {
	total := len(destMap)

	// Build src list for deterministic ordering
	srcs := make([]string, 0, total)
	for src := range destMap {
		srcs = append(srcs, src)
	}

	if dryRun {
		logger.Printf("[EXIF] DRY-RUN mode - simulating metadata application")
		for i, src := range srcs {
			// Check for cancellation
			select {
			case <-ctx.Done():
				logger.Printf("[EXIF] Cancelled during dry-run after %d/%d files", i, total)
				fatalErr = ctx.Err()
				return
			default:
			}

			dest := destMap[src]
			jsonPath := mediaJSON[src]
			if jsonPath != "" {
				logger.Printf("[EXIF] [DRY-RUN] Would apply JSON metadata: %s -> %s", filepath.Base(jsonPath), filepath.Base(dest))
				applied++
			} else {
				base := filepath.Base(dest)
				t, _ := filedate.Extract(base)
				if !t.IsZero() {
					logger.Printf("[EXIF] [DRY-RUN] Would apply filename date (%s): %s", t.Format("2006-01-02"), base)
					dateFallback++
				} else {
					logger.Printf("[EXIF] [DRY-RUN] No metadata source for: %s", base)
					skipped++
				}
			}
			if progress != nil {
				progress(i+1, total)
			}
		}
		return
	}

	// Initialize exiftool - this is now a fatal error
	et, err := NewExifTool(logger)
	if err != nil {
		fatalErr = fmt.Errorf("exiftool not available: %w", err)
		logger.Printf("[EXIF] FATAL: %v", fatalErr)
		return
	}
	defer et.Close()
	logger.Printf("[EXIF] Started exiftool process")

	done := 0
	for _, src := range srcs {
		// Check for cancellation
		select {
		case <-ctx.Done():
			logger.Printf("[EXIF] Cancelled after %d/%d files", done, total)
			fatalErr = ctx.Err()
			return
		default:
		}

		dest := destMap[src]
		jsonPath := mediaJSON[src]
		baseName := filepath.Base(dest)

		var applyErr error
		if jsonPath != "" {
			applyErr = et.ApplyEXIFFromJSON(dest, jsonPath)
			if applyErr == nil {
				applied++
				logger.Printf("[EXIF] Applied JSON metadata: %s <- %s", baseName, filepath.Base(jsonPath))
			} else {
				errCount++
				errs = append(errs, applyErr)
				logger.Printf("[EXIF] ERROR applying JSON to %s: %v", baseName, applyErr)
			}
		} else {
			ok, applyErr2 := et.ApplyDateFromFilename(dest)
			if applyErr2 == nil {
				if ok {
					dateFallback++
					t, _ := filedate.Extract(baseName)
					logger.Printf("[EXIF] Applied filename date (%s): %s", t.Format("2006-01-02"), baseName)
				} else {
					skipped++
					logger.Printf("[EXIF] No metadata source (no JSON, no date in filename): %s", baseName)
				}
			} else {
				errCount++
				errs = append(errs, applyErr2)
				logger.Printf("[EXIF] ERROR applying filename date to %s: %v", baseName, applyErr2)
			}
		}

		done++
		if progress != nil {
			progress(done, total)
		}
	}

	logger.Printf("[EXIF] Complete: applied=%d, filename-fallback=%d, skipped=%d, errors=%d", applied, dateFallback, skipped, errCount)
	return
}
