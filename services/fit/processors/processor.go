// Package processors provides data processing for Google Fit Takeout.
package processors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/fit/metadata"
)

// Processor handles the Fit activity and fitness data processing pipeline.
type Processor struct{}

// NewFitProcessor creates a new Fit processor.
func NewFitProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) ID() string   { return "fit-processor" }
func (p *Processor) Name() string { return "Fit Processor" }
func (p *Processor) Description() string {
	return "Process Google Fit Takeout - daily aggregations and activity sessions"
}

// ConfigSchema returns the configuration options for this processor.
func (p *Processor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "include_activities",
			Name:        "Include Activities",
			Description: "Process activity session data",
			Type:        "bool",
			Default:     true,
		},
	}
}

// Process runs the Fit processing pipeline.
// Phases: 1) Scan files, 2) Parse daily aggregations, 3) Parse activities, 4) Generate summary
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

	includeActivities := getBool(settings, "include_activities", true)

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
	logger.Printf("=== Fit processing started (source=%s) ===", cfg.SourceDir)

	// Find Fit directory
	fitPath, err := detectFitDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(fitPath)))
	logger.Printf("[Setup] Fit directory: %s", fitPath)

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	library := &metadata.FitLibrary{
		Stats: make(map[string]int),
	}

	// ── Phase 1: Scan for data files ───────────────────────────────────────
	emit(1, "Scanning Fit files", 0, 0, "")
	dataFiles, err := p.scanFitFiles(ctx, fitPath, logger)
	if err != nil {
		logger.Printf("[Phase 1] Error scanning files: %v", err)
		return nil, err
	}
	emit(1, "Scan complete", 1, 1,
		fmt.Sprintf("Found %d CSV files, %d activity JSONs", len(dataFiles.DailyCSVFiles), len(dataFiles.ActivityFiles)))
	logger.Printf("[Phase 1] csv_files=%d, activity_files=%d",
		len(dataFiles.DailyCSVFiles), len(dataFiles.ActivityFiles))

	// ── Phase 2: Parse daily aggregations ──────────────────────────────────
	emit(2, "Parsing daily aggregations", 0, len(dataFiles.DailyCSVFiles), "")
	aggregations, err := p.parseDailyAggregations(ctx, dataFiles.DailyCSVFiles, emit, logger)
	if err != nil {
		logger.Printf("[Phase 2] Error parsing daily aggregations: %v", err)
		return nil, err
	}
	library.DailyAggregations = aggregations
	result.Stats["daily_aggregations"] = len(aggregations)
	emit(2, "Daily aggregations parsed", len(aggregations), len(dataFiles.DailyCSVFiles),
		fmt.Sprintf("%d daily records", len(aggregations)))
	logger.Printf("[Phase 2] daily_aggregations=%d", len(aggregations))

	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// ── Phase 3: Parse activity sessions ───────────────────────────────────
	if includeActivities && len(dataFiles.ActivityFiles) > 0 {
		emit(3, "Parsing activity sessions", 0, len(dataFiles.ActivityFiles), "")
		activities, err := p.parseActivities(ctx, dataFiles.ActivityFiles, emit, logger)
		if err != nil {
			logger.Printf("[Phase 3] Error parsing activities: %v", err)
			return nil, err
		}
		library.Activities = activities
		result.Stats["activity_sessions"] = len(activities)
		emit(3, "Activities parsed", len(activities), len(dataFiles.ActivityFiles),
			fmt.Sprintf("%d activity sessions", len(activities)))
		logger.Printf("[Phase 3] activity_sessions=%d", len(activities))
	} else {
		emit(3, "Activities skipped", 0, 0, "")
		logger.Printf("[Phase 3] Activity processing skipped (enabled=%v, files=%d)",
			includeActivities, len(dataFiles.ActivityFiles))
	}

	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// ── Phase 4: Write summary and metadata ────────────────────────────────
	emit(4, "Writing metadata", 0, 1, "")

	// Store metadata
	library.Stats = result.Stats
	result.Metadata["library"] = library

	// Write library as JSON
	libraryPath := filepath.Join(cfg.WorkDir, "library.json")
	libraryData, _ := json.MarshalIndent(library, "", "  ")
	os.WriteFile(libraryPath, libraryData, 0o644)

	result.Metadata["library_path"] = libraryPath

	emit(4, "Output complete", 1, 1, fmt.Sprintf("Wrote %s", filepath.Base(libraryPath)))
	logger.Printf("[Phase 4] Wrote library.json")
	logger.Printf("=== Fit processing complete ===")

	// Build ProcessedItems
	result.Items = p.buildProcessedItems(library, cfg.WorkDir)

	return result, nil
}

// fitDataFiles holds references to all found data files.
type fitDataFiles struct {
	DailyCSVFiles []string
	ActivityFiles []string
}

func (f *fitDataFiles) TotalFiles() int {
	return len(f.DailyCSVFiles) + len(f.ActivityFiles)
}

// detectFitDir finds the Fit directory in the source.
func detectFitDir(sourceDir string) (string, error) {
	candidates := []string{
		"Fit",
		"Google Fit",
		"Fitness",
	}

	// First try direct match
	for _, candidate := range candidates {
		path := filepath.Join(sourceDir, candidate)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, nil
		}
	}

	// Try walking one level
	var foundPath string
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return "", fmt.Errorf("read source directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, candidate := range candidates {
			if entry.Name() == candidate {
				foundPath = filepath.Join(sourceDir, entry.Name())
				return foundPath, nil
			}
		}
	}

	return "", fmt.Errorf("Fit directory not found in %s", sourceDir)
}

// scanFitFiles scans for all relevant Fit data files.
func (p *Processor) scanFitFiles(ctx context.Context, fitPath string, logger *log.Logger) (*fitDataFiles, error) {
	files := &fitDataFiles{
		DailyCSVFiles: []string{},
		ActivityFiles: []string{},
	}

	// Walk directory for CSV and JSON files
	err := filepath.WalkDir(fitPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return nil
		}

		name := d.Name()
		nameLower := strings.ToLower(name)

		// Daily aggregation CSV files
		if strings.HasSuffix(nameLower, ".csv") {
			// Look for daily aggregation patterns
			if strings.Contains(nameLower, "daily") ||
				strings.Contains(nameLower, "aggregation") ||
				strings.Contains(nameLower, "activities") {
				files.DailyCSVFiles = append(files.DailyCSVFiles, path)
				return nil
			}
		}

		// Activity JSON files
		if strings.HasSuffix(nameLower, ".json") {
			// Look for activity session patterns
			// Google Fit exports activities as individual JSON files or in Activities/ subfolder
			if strings.Contains(path, "Activities") ||
				strings.Contains(path, "sessions") ||
				strings.Contains(nameLower, "activity") {
				files.ActivityFiles = append(files.ActivityFiles, path)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return files, nil
}

// parseDailyAggregations parses all daily aggregation CSV files.
func (p *Processor) parseDailyAggregations(ctx context.Context, files []string, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.DailyAggregation, error) {
	var allAggregations []metadata.DailyAggregation

	for i, filePath := range files {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emit(2, "Parsing daily aggregations", i, len(files), filepath.Base(filePath))

		file, err := os.Open(filePath)
		if err != nil {
			logger.Printf("Failed to open %s: %v", filePath, err)
			continue
		}

		aggs, err := metadata.ParseDailyAggregationCSV(file)
		file.Close()

		if err != nil {
			logger.Printf("Failed to parse %s: %v", filePath, err)
			continue
		}

		allAggregations = append(allAggregations, aggs...)
		logger.Printf("Parsed %d aggregations from %s", len(aggs), filepath.Base(filePath))
	}

	return allAggregations, nil
}

// parseActivities parses all activity session JSON files.
func (p *Processor) parseActivities(ctx context.Context, files []string, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.ActivitySession, error) {
	var allActivities []metadata.ActivitySession

	for i, filePath := range files {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emit(3, "Parsing activity sessions", i, len(files), filepath.Base(filePath))

		file, err := os.Open(filePath)
		if err != nil {
			logger.Printf("Failed to open %s: %v", filePath, err)
			continue
		}

		activity, err := metadata.ParseActivityJSON(file)
		file.Close()

		if err != nil {
			logger.Printf("Failed to parse %s: %v", filePath, err)
			continue
		}

		allActivities = append(allActivities, *activity)
		logger.Printf("Parsed activity from %s: %s", filepath.Base(filePath), activity.FitnessActivity)
	}

	return allActivities, nil
}

// buildProcessedItems creates ProcessedItem entries for the library.
func (p *Processor) buildProcessedItems(library *metadata.FitLibrary, workDir string) []core.ProcessedItem {
	items := []core.ProcessedItem{}

	// Add daily aggregations as a single item
	if len(library.DailyAggregations) > 0 {
		items = append(items, core.ProcessedItem{
			SourcePath:    workDir,
			ProcessedPath: workDir,
			DestRelPath:   "daily_aggregations",
			Type:          "fit_library",
			Metadata: map[string]any{
				"fit_library":       library,
				"aggregation_count": len(library.DailyAggregations),
				"date_start":        library.GetCreatedDate(),
				"date_end":          library.GetModifiedDate(),
			},
		})
	}

	return items
}

// Helper functions for settings extraction

func getBool(settings map[string]any, key string, defaultVal bool) bool {
	if val, ok := settings[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultVal
}
