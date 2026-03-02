// Package processors provides data processing for Chrome Takeout.
package processors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	conncore "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/services/chrome/metadata"
	"github.com/fulgidus/revoco/services/core"
)

// Processor handles the Chrome Takeout processing pipeline.
type Processor struct{}

// NewChromeProcessor creates a new Chrome processor.
func NewChromeProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) ID() string   { return "chrome-processor" }
func (p *Processor) Name() string { return "Chrome Processor" }
func (p *Processor) Description() string {
	return "Process Chrome Takeout data - bookmarks, history, search engines, and autofill"
}

// ConfigSchema returns the configuration options for this processor.
func (p *Processor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "include_search_engines",
			Name:        "Include Search Engines",
			Description: "Parse and include search engine data",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "include_autofill",
			Name:        "Include Autofill",
			Description: "Parse and include autofill data",
			Type:        "bool",
			Default:     false,
		},
	}
}

// Process runs the Chrome Takeout processing pipeline.
// Phases: 1) Scan files, 2) Parse bookmarks, 3) Parse history,
// 4) Parse extras (search engines, autofill), 5) Generate summary
func (p *Processor) Process(ctx context.Context, cfg core.ProcessorConfig, events chan<- core.ProgressEvent) (*core.ProcessResult, error) {
	defer close(events)

	emit := func(phase int, label string, done, total int, msg string) {
		select {
		case events <- core.ProgressEvent{
			Phase:   phase,
			Label:   label,
			Done:    done,
			Total:   total,
			Message: msg,
		}:
		case <-ctx.Done():
		}
	}

	settings := cfg.Settings
	if settings == nil {
		settings = make(map[string]any)
	}

	includeSearchEngines := getBool(settings, "include_search_engines", false)
	includeAutofill := getBool(settings, "include_autofill", false)

	// Setup logging
	logDir := cfg.SessionDir
	if logDir == "" {
		logDir = cfg.WorkDir
	}
	os.MkdirAll(logDir, 0o755)

	logPath := filepath.Join(logDir, "process.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)
	logger.Printf("=== Chrome Takeout processing started (source=%s) ===", cfg.SourceDir)

	// Find Chrome directory
	chromePath, err := detectChromeDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(chromePath)))
	logger.Printf("[Setup] Chrome directory: %s", chromePath)

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	library := &metadata.ChromeLibrary{
		Stats:        make(map[string]int),
		FolderCounts: make(map[string]int),
	}

	// ── Phase 1: Scan for Chrome files ──────────────────────────────────────
	emit(1, "Scanning Chrome files", 0, 0, "")

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	bookmarksPath := filepath.Join(chromePath, "Bookmarks.html")
	historyPath := filepath.Join(chromePath, "BrowserHistory.json")
	searchEnginesPath := filepath.Join(chromePath, "SearchEngines.json")
	autofillPath := filepath.Join(chromePath, "Autofill.json")

	filesFound := 0
	if _, err := os.Stat(bookmarksPath); err == nil {
		library.BookmarksPath = bookmarksPath
		filesFound++
		logger.Printf("[Phase 1] Found Bookmarks.html")
	}
	if _, err := os.Stat(historyPath); err == nil {
		library.HistoryPath = historyPath
		filesFound++
		logger.Printf("[Phase 1] Found BrowserHistory.json")
	}
	if _, err := os.Stat(searchEnginesPath); err == nil {
		filesFound++
		logger.Printf("[Phase 1] Found SearchEngines.json")
	}
	if _, err := os.Stat(autofillPath); err == nil {
		filesFound++
		logger.Printf("[Phase 1] Found Autofill.json")
	}

	if filesFound == 0 {
		return nil, fmt.Errorf("no Chrome data files found in %s", chromePath)
	}

	result.Stats["files_found"] = filesFound
	emit(1, "Scan complete", filesFound, filesFound, fmt.Sprintf("%d files found", filesFound))
	logger.Printf("[Phase 1] files_found=%d", filesFound)

	// ── Phase 2: Parse Bookmarks ────────────────────────────────────────────
	emit(2, "Parsing bookmarks", 0, 1, "")

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if library.BookmarksPath != "" {
		file, err := os.Open(library.BookmarksPath)
		if err != nil {
			logger.Printf("[Phase 2] Error opening bookmarks: %v", err)
		} else {
			bookmarks, err := metadata.ParseBookmarksHTML(file)
			file.Close()
			if err != nil {
				logger.Printf("[Phase 2] Error parsing bookmarks: %v", err)
			} else {
				library.Bookmarks = bookmarks
				result.Stats["bookmarks"] = len(bookmarks)
				logger.Printf("[Phase 2] Parsed %d bookmarks", len(bookmarks))

				// Count bookmarks per folder
				for _, bm := range bookmarks {
					if bm.Folder != "" {
						library.FolderCounts[bm.Folder]++
					}
				}
			}
		}
	}

	emit(2, "Bookmarks parsed", 1, 1, fmt.Sprintf("%d bookmarks", result.Stats["bookmarks"]))
	library.Stats["bookmarks"] = result.Stats["bookmarks"]
	library.Stats["folders"] = len(library.FolderCounts)

	// ── Phase 3: Parse History ──────────────────────────────────────────────
	emit(3, "Parsing browser history", 0, 1, "")

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if library.HistoryPath != "" {
		file, err := os.Open(library.HistoryPath)
		if err != nil {
			logger.Printf("[Phase 3] Error opening history: %v", err)
		} else {
			history, err := metadata.ParseBrowserHistoryJSON(file)
			file.Close()
			if err != nil {
				logger.Printf("[Phase 3] Error parsing history: %v", err)
			} else {
				library.History = history
				result.Stats["history_entries"] = len(history)
				logger.Printf("[Phase 3] Parsed %d history entries", len(history))
			}
		}
	}

	emit(3, "History parsed", 1, 1, fmt.Sprintf("%d entries", result.Stats["history_entries"]))
	library.Stats["history_entries"] = result.Stats["history_entries"]

	// ── Phase 4: Parse Extras ───────────────────────────────────────────────
	emit(4, "Parsing extras", 0, 2, "")

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	parsed := 0

	// Search Engines
	if includeSearchEngines {
		if _, err := os.Stat(searchEnginesPath); err == nil {
			file, err := os.Open(searchEnginesPath)
			if err != nil {
				logger.Printf("[Phase 4] Error opening search engines: %v", err)
			} else {
				engines, err := metadata.ParseSearchEnginesJSON(file)
				file.Close()
				if err != nil {
					logger.Printf("[Phase 4] Error parsing search engines: %v", err)
				} else {
					library.SearchEngines = engines
					result.Stats["search_engines"] = len(engines)
					logger.Printf("[Phase 4] Parsed %d search engines", len(engines))
				}
			}
			parsed++
			emit(4, "Parsing search engines", parsed, 2, "")
		}
	}

	// Autofill
	if includeAutofill {
		if _, err := os.Stat(autofillPath); err == nil {
			file, err := os.Open(autofillPath)
			if err != nil {
				logger.Printf("[Phase 4] Error opening autofill: %v", err)
			} else {
				autofills, err := metadata.ParseAutofillJSON(file)
				file.Close()
				if err != nil {
					logger.Printf("[Phase 4] Error parsing autofill: %v", err)
				} else {
					library.Autofills = autofills
					result.Stats["autofill_entries"] = len(autofills)
					logger.Printf("[Phase 4] Parsed %d autofill entries", len(autofills))
				}
			}
			parsed++
			emit(4, "Parsing autofill", parsed, 2, "")
		}
	}

	if !includeSearchEngines && !includeAutofill {
		emit(4, "Extras skipped", 2, 2, "Disabled in config")
		logger.Printf("[Phase 4] Skipped")
	} else {
		emit(4, "Extras complete", 2, 2, fmt.Sprintf("Parsed %d extras", parsed))
	}

	library.Stats["search_engines"] = result.Stats["search_engines"]
	library.Stats["autofill_entries"] = result.Stats["autofill_entries"]

	// ── Phase 5: Generate Summary and Write Output ─────────────────────────
	emit(5, "Generating summary", 0, 1, "")

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Build processed items for each bookmark and history entry
	for _, bm := range library.Bookmarks {
		folderPath := strings.Join(bm.ParentFolders, "/")
		if folderPath == "" {
			folderPath = "root"
		}
		destRelPath := filepath.Join("bookmarks", folderPath, sanitizeFilename(bm.Name)+".json")

		result.Items = append(result.Items, core.ProcessedItem{
			SourcePath:    library.BookmarksPath,
			ProcessedPath: library.BookmarksPath,
			DestRelPath:   destRelPath,
			Type:          string(conncore.DataTypeBookmark),
			Metadata: map[string]any{
				"name":           bm.Name,
				"url":            bm.URL,
				"folder":         bm.Folder,
				"date_added":     bm.DateAdded,
				"parent_folders": bm.ParentFolders,
			},
		})
	}

	for i, entry := range library.History {
		destRelPath := filepath.Join("history", fmt.Sprintf("entry-%05d.json", i))

		result.Items = append(result.Items, core.ProcessedItem{
			SourcePath:    library.HistoryPath,
			ProcessedPath: library.HistoryPath,
			DestRelPath:   destRelPath,
			Type:          string(conncore.DataTypeBrowserHistory),
			Metadata: map[string]any{
				"title":           entry.Title,
				"url":             entry.URL,
				"visit_count":     entry.VisitCount,
				"last_visited":    entry.LastVisited,
				"typed_count":     entry.TypedCount,
				"page_transition": entry.PageTransition,
			},
		})
	}

	// Write library as JSON
	libraryPath := filepath.Join(cfg.WorkDir, "library.json")
	libraryData, _ := json.MarshalIndent(library, "", "  ")
	os.WriteFile(libraryPath, libraryData, 0o644)

	result.Metadata["library"] = library
	result.Metadata["library_path"] = libraryPath

	// Summary stats
	summary := generateSummary(library)
	result.Metadata["summary"] = summary
	logger.Printf("[Phase 5] Summary: %v", summary)

	emit(5, "Summary complete", 1, 1, fmt.Sprintf("%d total items", len(result.Items)))
	logger.Printf("=== Chrome Takeout processing complete ===")
	logger.Printf("[Summary] bookmarks=%d, history=%d, items=%d",
		len(library.Bookmarks), len(library.History), len(result.Items))

	return result, nil
}

// detectChromeDir finds the Chrome directory in the source path.
func detectChromeDir(sourcePath string) (string, error) {
	// Try direct path
	chromePath := filepath.Join(sourcePath, "Chrome")
	if info, err := os.Stat(chromePath); err == nil && info.IsDir() {
		return chromePath, nil
	}

	// Try case-insensitive search
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read source dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.EqualFold(entry.Name(), "chrome") {
			return filepath.Join(sourcePath, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("Chrome directory not found in %s", sourcePath)
}

// generateSummary creates a human-readable summary of the Chrome library.
func generateSummary(lib *metadata.ChromeLibrary) map[string]any {
	summary := make(map[string]any)

	summary["bookmarks_count"] = len(lib.Bookmarks)
	summary["history_count"] = len(lib.History)
	summary["folders_count"] = len(lib.FolderCounts)

	if len(lib.SearchEngines) > 0 {
		summary["search_engines_count"] = len(lib.SearchEngines)
	}
	if len(lib.Autofills) > 0 {
		summary["autofill_count"] = len(lib.Autofills)
	}

	// Top folders by bookmark count
	type folderCount struct {
		Name  string
		Count int
	}
	var folders []folderCount
	for name, count := range lib.FolderCounts {
		folders = append(folders, folderCount{Name: name, Count: count})
	}
	sort.Slice(folders, func(i, j int) bool {
		return folders[i].Count > folders[j].Count
	})

	topFolders := make(map[string]int)
	for i := 0; i < len(folders) && i < 10; i++ {
		topFolders[folders[i].Name] = folders[i].Count
	}
	if len(topFolders) > 0 {
		summary["top_folders"] = topFolders
	}

	// History date range
	if len(lib.History) > 0 {
		var earliest, latest = lib.History[0].LastVisited, lib.History[0].LastVisited
		for _, entry := range lib.History {
			if !entry.LastVisited.IsZero() {
				if entry.LastVisited.Before(earliest) {
					earliest = entry.LastVisited
				}
				if entry.LastVisited.After(latest) {
					latest = entry.LastVisited
				}
			}
		}
		if !earliest.IsZero() {
			summary["history_earliest"] = earliest.Format("2006-01-02")
			summary["history_latest"] = latest.Format("2006-01-02")
		}
	}

	return summary
}

// sanitizeFilename removes characters that are invalid in filenames.
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, "*", "-")
	name = strings.ReplaceAll(name, "?", "-")
	name = strings.ReplaceAll(name, "\"", "-")
	name = strings.ReplaceAll(name, "<", "-")
	name = strings.ReplaceAll(name, ">", "-")
	name = strings.ReplaceAll(name, "|", "-")
	return name
}

// getBool extracts a boolean setting with a default value.
func getBool(settings map[string]any, key string, defaultVal bool) bool {
	if v, ok := settings[key].(bool); ok {
		return v
	}
	return defaultVal
}
