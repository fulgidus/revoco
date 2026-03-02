// Package processors provides data processing for Google Maps Takeout.
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
	"github.com/fulgidus/revoco/services/maps/metadata"
)

// Processor handles the Maps location history processing pipeline.
type Processor struct{}

// NewMapsProcessor creates a new Maps processor.
func NewMapsProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) ID() string   { return "maps-processor" }
func (p *Processor) Name() string { return "Maps Processor" }
func (p *Processor) Description() string {
	return "Process Google Maps Takeout - location history, saved places, and timeline data"
}

// ConfigSchema returns the configuration options for this processor.
func (p *Processor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "coordinate_precision",
			Name:        "Coordinate Precision",
			Description: "Number of decimal places for coordinates (1-10)",
			Type:        "int",
			Default:     6,
		},
		{
			ID:          "include_timeline",
			Name:        "Include Timeline Data",
			Description: "Process semantic location history timeline",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "min_accuracy",
			Name:        "Minimum Accuracy",
			Description: "Filter out location records with accuracy > N meters (0 = no filter)",
			Type:        "int",
			Default:     0,
		},
	}
}

// Process runs the Maps processing pipeline.
// Phases: 1) Scan files, 2) Parse location history, 3) Parse saved places,
// 4) Parse timeline, 5) Generate summary
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

	coordinatePrecision := getInt(settings, "coordinate_precision", 6)
	includeTimeline := getBool(settings, "include_timeline", true)
	minAccuracy := getInt(settings, "min_accuracy", 0)

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
	logger.Printf("=== Maps processing started (source=%s) ===", cfg.SourceDir)

	// Find Maps directory
	mapsPath, err := detectMapsDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(mapsPath)))
	logger.Printf("[Setup] Maps directory: %s", mapsPath)

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	library := &metadata.MapsLibrary{
		Stats: make(map[string]int),
	}

	// ── Phase 1: Scan for data files ───────────────────────────────────────
	emit(1, "Scanning Maps files", 0, 0, "")
	dataFiles, err := p.scanMapsFiles(ctx, mapsPath, logger)
	if err != nil {
		logger.Printf("[Phase 1] Error scanning files: %v", err)
		return nil, err
	}
	emit(1, "Scan complete", 1, 1,
		fmt.Sprintf("Found %d data files", dataFiles.TotalFiles()))
	logger.Printf("[Phase 1] records=%s, saved_places=%d, timeline=%d",
		dataFiles.RecordsJSON, len(dataFiles.SavedPlacesFiles), len(dataFiles.TimelineFiles))

	// ── Phase 2: Parse location history ────────────────────────────────────
	if dataFiles.RecordsJSON != "" {
		emit(2, "Parsing location history", 0, 0, "Streaming Records.json...")
		records, err := p.parseLocationHistory(ctx, dataFiles.RecordsJSON, minAccuracy, logger)
		if err != nil {
			logger.Printf("[Phase 2] Error parsing location history: %v", err)
			return nil, err
		}
		library.LocationHistory = records
		result.Stats["location_records"] = len(records)
		emit(2, "Location history parsed", len(records), len(records),
			fmt.Sprintf("%d location records", len(records)))
		logger.Printf("[Phase 2] location_records=%d", len(records))
	} else {
		emit(2, "Location history not found", 0, 0, "Skipping")
		logger.Printf("[Phase 2] No Records.json found")
	}

	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// ── Phase 3: Parse saved places ────────────────────────────────────────
	emit(3, "Parsing saved places", 0, len(dataFiles.SavedPlacesFiles), "")
	places, err := p.parseSavedPlaces(ctx, dataFiles.SavedPlacesFiles, emit, logger)
	if err != nil {
		logger.Printf("[Phase 3] Error parsing saved places: %v", err)
		return nil, err
	}
	library.SavedPlaces = places
	result.Stats["saved_places"] = len(places)
	emit(3, "Saved places parsed", len(places), len(dataFiles.SavedPlacesFiles),
		fmt.Sprintf("%d saved places", len(places)))
	logger.Printf("[Phase 3] saved_places=%d", len(places))

	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// ── Phase 4: Parse timeline data ───────────────────────────────────────
	if includeTimeline && len(dataFiles.TimelineFiles) > 0 {
		emit(4, "Parsing timeline", 0, len(dataFiles.TimelineFiles), "")
		visits, err := p.parseTimeline(ctx, dataFiles.TimelineFiles, emit, logger)
		if err != nil {
			logger.Printf("[Phase 4] Error parsing timeline: %v", err)
			return nil, err
		}
		library.Timeline = visits
		result.Stats["timeline_visits"] = len(visits)
		emit(4, "Timeline parsed", len(visits), len(dataFiles.TimelineFiles),
			fmt.Sprintf("%d place visits", len(visits)))
		logger.Printf("[Phase 4] timeline_visits=%d", len(visits))
	} else {
		emit(4, "Timeline skipped", 0, 0, "")
		logger.Printf("[Phase 4] Timeline processing skipped (enabled=%v, files=%d)",
			includeTimeline, len(dataFiles.TimelineFiles))
	}

	// Check for cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// ── Phase 5: Write summary and metadata ────────────────────────────────
	emit(5, "Writing metadata", 0, 1, "")

	// Store metadata
	library.Stats = result.Stats
	result.Metadata["library"] = library
	result.Metadata["coordinate_precision"] = coordinatePrecision
	result.Metadata["min_accuracy_filter"] = minAccuracy

	// Write library as JSON
	libraryPath := filepath.Join(cfg.WorkDir, "library.json")
	libraryData, _ := json.MarshalIndent(library, "", "  ")
	os.WriteFile(libraryPath, libraryData, 0o644)

	result.Metadata["library_path"] = libraryPath

	emit(5, "Output complete", 1, 1, fmt.Sprintf("Wrote %s", filepath.Base(libraryPath)))
	logger.Printf("[Phase 5] Wrote library.json")
	logger.Printf("=== Maps processing complete ===")

	// Build ProcessedItems
	result.Items = p.buildProcessedItems(library, cfg.WorkDir)

	return result, nil
}

// mapsDataFiles holds references to all found data files.
type mapsDataFiles struct {
	RecordsJSON      string
	SavedPlacesFiles []string
	TimelineFiles    []string
}

func (m *mapsDataFiles) TotalFiles() int {
	count := 0
	if m.RecordsJSON != "" {
		count++
	}
	return count + len(m.SavedPlacesFiles) + len(m.TimelineFiles)
}

// detectMapsDir finds the Maps directory in the source.
func detectMapsDir(sourceDir string) (string, error) {
	candidates := []string{
		"Maps",
		"Maps (I tuoi luoghi)",
		"Location History",
		"Cronologia delle posizioni",
		"Semantic Location History",
		"Cronologia delle posizioni semantica",
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

	return "", fmt.Errorf("Maps directory not found in %s", sourceDir)
}

// scanMapsFiles scans for all relevant Maps data files.
func (p *Processor) scanMapsFiles(ctx context.Context, mapsPath string, logger *log.Logger) (*mapsDataFiles, error) {
	files := &mapsDataFiles{
		SavedPlacesFiles: []string{},
		TimelineFiles:    []string{},
	}

	// Look for Records.json or Cronologia.json
	recordsCandidates := []string{
		"Records.json",
		"Cronologia.json",
	}
	for _, candidate := range recordsCandidates {
		path := filepath.Join(mapsPath, candidate)
		if _, err := os.Stat(path); err == nil {
			files.RecordsJSON = path
			break
		}
		// Also check Location History subdirectory
		path = filepath.Join(mapsPath, "Location History", candidate)
		if _, err := os.Stat(path); err == nil {
			files.RecordsJSON = path
			break
		}
		path = filepath.Join(mapsPath, "Cronologia delle posizioni", candidate)
		if _, err := os.Stat(path); err == nil {
			files.RecordsJSON = path
			break
		}
	}

	// Walk directory for saved places and timeline files
	err := filepath.WalkDir(mapsPath, func(path string, d os.DirEntry, err error) error {
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

		// Saved places files (KML or JSON)
		if strings.HasSuffix(name, ".kml") ||
			(strings.HasSuffix(name, ".json") && strings.Contains(strings.ToLower(name), "saved")) {
			files.SavedPlacesFiles = append(files.SavedPlacesFiles, path)
			return nil
		}

		// Timeline JSON files (in Semantic Location History/YYYY/)
		if strings.HasSuffix(name, ".json") &&
			(strings.Contains(path, "Semantic Location History") ||
				strings.Contains(path, "Cronologia delle posizioni semantica")) {
			// Skip Records.json
			if name != "Records.json" && name != "Cronologia.json" {
				files.TimelineFiles = append(files.TimelineFiles, path)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return files, nil
}

// parseLocationHistory streams and parses Records.json.
func (p *Processor) parseLocationHistory(ctx context.Context, recordsPath string, minAccuracy int, logger *log.Logger) ([]metadata.LocationRecord, error) {
	file, err := os.Open(recordsPath)
	if err != nil {
		return nil, fmt.Errorf("open Records.json: %w", err)
	}
	defer file.Close()

	logger.Printf("Streaming Records.json from: %s", recordsPath)

	records, err := metadata.ParseRecordsJSON(file)
	if err != nil {
		return nil, fmt.Errorf("parse Records.json: %w", err)
	}

	// Filter by accuracy if specified
	if minAccuracy > 0 {
		filtered := make([]metadata.LocationRecord, 0, len(records))
		for _, rec := range records {
			if rec.Accuracy <= minAccuracy || rec.Accuracy == 0 {
				filtered = append(filtered, rec)
			}
		}
		logger.Printf("Filtered by accuracy: %d -> %d records", len(records), len(filtered))
		records = filtered
	}

	return records, nil
}

// parseSavedPlaces parses all saved places files.
func (p *Processor) parseSavedPlaces(ctx context.Context, files []string, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.SavedPlace, error) {
	var allPlaces []metadata.SavedPlace

	for i, filePath := range files {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emit(3, "Parsing saved places", i, len(files), filepath.Base(filePath))

		file, err := os.Open(filePath)
		if err != nil {
			logger.Printf("Failed to open %s: %v", filePath, err)
			continue
		}

		var places []metadata.SavedPlace

		// Determine format by extension
		if strings.HasSuffix(filePath, ".kml") {
			places, err = metadata.ParseSavedPlacesKML(file)
		} else if strings.HasSuffix(filePath, ".json") {
			// JSON saved places format (if exists)
			// For now, skip JSON saved places (Google typically uses KML)
			logger.Printf("Skipping JSON saved places (not implemented): %s", filePath)
			file.Close()
			continue
		}

		file.Close()

		if err != nil {
			logger.Printf("Failed to parse %s: %v", filePath, err)
			continue
		}

		allPlaces = append(allPlaces, places...)
		logger.Printf("Parsed %d places from %s", len(places), filepath.Base(filePath))
	}

	return allPlaces, nil
}

// parseTimeline parses all timeline JSON files.
func (p *Processor) parseTimeline(ctx context.Context, files []string, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.PlaceVisit, error) {
	var allVisits []metadata.PlaceVisit

	for i, filePath := range files {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emit(4, "Parsing timeline", i, len(files), filepath.Base(filePath))

		file, err := os.Open(filePath)
		if err != nil {
			logger.Printf("Failed to open %s: %v", filePath, err)
			continue
		}

		visits, err := metadata.ParseTimelineJSON(file)
		file.Close()

		if err != nil {
			logger.Printf("Failed to parse %s: %v", filePath, err)
			continue
		}

		allVisits = append(allVisits, visits...)
		logger.Printf("Parsed %d visits from %s", len(visits), filepath.Base(filePath))
	}

	return allVisits, nil
}

// buildProcessedItems creates ProcessedItem entries for the library.
func (p *Processor) buildProcessedItems(library *metadata.MapsLibrary, workDir string) []core.ProcessedItem {
	items := []core.ProcessedItem{}

	// Add location history as a single item
	if len(library.LocationHistory) > 0 {
		items = append(items, core.ProcessedItem{
			SourcePath:    workDir,
			ProcessedPath: workDir,
			DestRelPath:   "location_history",
			Type:          "location",
			Metadata: map[string]any{
				"record_count": len(library.LocationHistory),
				"date_start":   library.GetCreatedDate(),
				"date_end":     library.GetModifiedDate(),
			},
		})
	}

	// Add saved places as a single item
	if len(library.SavedPlaces) > 0 {
		items = append(items, core.ProcessedItem{
			SourcePath:    workDir,
			ProcessedPath: workDir,
			DestRelPath:   "saved_places",
			Type:          "location",
			Metadata: map[string]any{
				"place_count": len(library.SavedPlaces),
			},
		})
	}

	// Add timeline as a single item
	if len(library.Timeline) > 0 {
		items = append(items, core.ProcessedItem{
			SourcePath:    workDir,
			ProcessedPath: workDir,
			DestRelPath:   "timeline",
			Type:          "location",
			Metadata: map[string]any{
				"visit_count": len(library.Timeline),
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

func getInt(settings map[string]any, key string, defaultVal int) int {
	if val, ok := settings[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return defaultVal
}
