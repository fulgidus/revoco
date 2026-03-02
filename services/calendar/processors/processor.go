// Package processors provides data processing for Calendar Takeout.
package processors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	conncore "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/services/calendar/metadata"
	"github.com/fulgidus/revoco/services/core"
)

// Processor handles the Calendar ICS processing pipeline.
type Processor struct{}

// NewCalendarProcessor creates a new Calendar processor.
func NewCalendarProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) ID() string   { return "calendar-processor" }
func (p *Processor) Name() string { return "Calendar Processor" }
func (p *Processor) Description() string {
	return "Process Google Calendar Takeout ICS files - parse events, extract metadata"
}

// ConfigSchema returns the configuration options for this processor.
func (p *Processor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "timezone_convert",
			Name:        "Convert to Local Timezone",
			Description: "Convert all event times to local timezone",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "exclude_cancelled",
			Name:        "Exclude Cancelled Events",
			Description: "Filter out events with STATUS:CANCELLED",
			Type:        "bool",
			Default:     false,
		},
	}
}

// Process runs the Calendar ICS processing pipeline.
// Phases: 1) Scan ICS files, 2) Parse ICS, 3) Extract events,
// 4) Normalize/filter, 5) Generate summary
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

	excludeCancelled := getBool(settings, "exclude_cancelled", false)

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
	logger.Printf("=== Calendar ICS processing started (source=%s) ===", cfg.SourceDir)

	// Find Calendar directory
	calendarPath, err := detectCalendarDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(calendarPath)))
	logger.Printf("[Setup] Calendar directory: %s", calendarPath)

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	library := &metadata.CalendarLibrary{}

	// ── Phase 1: Scan for ICS files ────────────────────────────────────────
	emit(1, "Scanning ICS files", 0, 0, "")
	icsFiles, err := p.scanICSFiles(ctx, calendarPath, logger)
	if err != nil {
		logger.Printf("[Phase 1] Error scanning ICS files: %v", err)
		return nil, err
	}
	result.Stats["ics_files"] = len(icsFiles)
	emit(1, "Scan complete", len(icsFiles), len(icsFiles),
		fmt.Sprintf("%d ICS files found", len(icsFiles)))
	logger.Printf("[Phase 1] ics_files=%d", len(icsFiles))

	// ── Phase 2: Parse ICS files ────────────────────────────────────────────
	emit(2, "Parsing ICS files", 0, len(icsFiles), "")
	allCalendars, err := p.parseICSFiles(ctx, icsFiles, emit, logger)
	if err != nil {
		logger.Printf("[Phase 2] Error parsing ICS: %v", err)
		return nil, err
	}
	library.Calendars = allCalendars
	result.Stats["calendars"] = len(allCalendars)
	emit(2, "Parse complete", len(allCalendars), len(icsFiles),
		fmt.Sprintf("%d calendars parsed", len(allCalendars)))
	logger.Printf("[Phase 2] calendars=%d", len(allCalendars))

	// ── Phase 3: Extract events ──────────────────────────────────────────────
	emit(3, "Extracting events", 0, 0, "")
	totalEvents := 0
	for _, cal := range allCalendars {
		totalEvents += len(cal.Events)
	}
	result.Stats["events"] = totalEvents
	emit(3, "Extraction complete", totalEvents, totalEvents,
		fmt.Sprintf("%d events extracted", totalEvents))
	logger.Printf("[Phase 3] events=%d", totalEvents)

	// ── Phase 4: Normalize and filter ───────────────────────────────────────
	emit(4, "Normalizing events", 0, totalEvents, "")
	filteredCount := 0
	if excludeCancelled {
		for i := range library.Calendars {
			filtered := []metadata.CalendarEvent{}
			for _, event := range library.Calendars[i].Events {
				if strings.ToUpper(event.Status) != "CANCELLED" {
					filtered = append(filtered, event)
				} else {
					filteredCount++
				}
			}
			library.Calendars[i].Events = filtered
		}
		logger.Printf("[Phase 4] filtered out %d cancelled events", filteredCount)
	}
	result.Stats["filtered_events"] = filteredCount
	emit(4, "Normalization complete", totalEvents-filteredCount, totalEvents,
		fmt.Sprintf("%d events after filtering", totalEvents-filteredCount))
	logger.Printf("[Phase 4] normalized")

	// ── Phase 5: Generate summary ────────────────────────────────────────────
	emit(5, "Generating summary", 0, 1, "")

	// Write library as JSON
	libraryPath := filepath.Join(cfg.WorkDir, "library.json")
	libraryData, _ := json.MarshalIndent(library, "", "  ")
	os.WriteFile(libraryPath, libraryData, 0o644)

	result.Metadata["library"] = library
	result.Metadata["library_path"] = libraryPath

	emit(5, "Summary complete", 1, 1, fmt.Sprintf("Wrote %s", filepath.Base(libraryPath)))
	logger.Printf("[Phase 5] Wrote library.json")
	logger.Printf("=== Calendar ICS processing complete ===")

	// Build ProcessedItems
	result.Items = p.buildProcessedItems(library, cfg.WorkDir)

	return result, nil
}

// scanICSFiles finds all .ics files in the Calendar directory.
func (p *Processor) scanICSFiles(ctx context.Context, calendarPath string, logger *log.Logger) ([]string, error) {
	var icsFiles []string

	err := filepath.WalkDir(calendarPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable paths
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(strings.ToLower(d.Name()), ".ics") {
			icsFiles = append(icsFiles, path)
		}

		return nil
	})

	return icsFiles, err
}

// parseICSFiles parses all ICS files and returns calendars.
func (p *Processor) parseICSFiles(ctx context.Context, icsFiles []string, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.Calendar, error) {
	var allCalendars []metadata.Calendar

	for fileIdx, icsPath := range icsFiles {
		select {
		case <-ctx.Done():
			return allCalendars, ctx.Err()
		default:
		}

		calendars, err := p.parseSingleICS(ctx, icsPath, logger)
		if err != nil {
			logger.Printf("Error parsing %s: %v", icsPath, err)
			continue
		}

		allCalendars = append(allCalendars, calendars...)

		emit(2, "Parsing ICS files", fileIdx+1, len(icsFiles),
			fmt.Sprintf("%s: %d calendars", filepath.Base(icsPath), len(calendars)))
	}

	return allCalendars, nil
}

// parseSingleICS parses a single ICS file.
func (p *Processor) parseSingleICS(ctx context.Context, path string, logger *log.Logger) ([]metadata.Calendar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	calendars, err := metadata.ParseICS(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}

	// Set calendar name from filename if not set
	baseName := strings.TrimSuffix(filepath.Base(path), ".ics")
	for i := range calendars {
		if calendars[i].Name == "" {
			calendars[i].Name = baseName
		}
	}

	return calendars, nil
}

// buildProcessedItems creates ProcessedItem entries for outputs.
func (p *Processor) buildProcessedItems(library *metadata.CalendarLibrary, workDir string) []core.ProcessedItem {
	var items []core.ProcessedItem

	// Add each calendar as an item
	for _, cal := range library.Calendars {
		// Calendar-level item
		items = append(items, core.ProcessedItem{
			SourcePath:    "",
			ProcessedPath: "",
			DestRelPath:   fmt.Sprintf("calendars/%s.json", sanitizeFilename(cal.Name)),
			Type:          string(conncore.DataTypeCalendarEvent),
			Metadata: map[string]any{
				"calendar_name": cal.Name,
				"description":   cal.Description,
				"timezone":      cal.Timezone,
				"event_count":   len(cal.Events),
				"calendar":      cal,
			},
		})

		// Individual event items
		for evtIdx, event := range cal.Events {
			items = append(items, core.ProcessedItem{
				SourcePath:    "",
				ProcessedPath: "",
				DestRelPath:   fmt.Sprintf("events/%s/%04d_%s.json", sanitizeFilename(cal.Name), evtIdx, sanitizeFilename(event.Summary)),
				Type:          string(conncore.DataTypeCalendarEvent),
				Metadata: map[string]any{
					"uid":        event.UID,
					"summary":    event.Summary,
					"start_time": event.StartTime,
					"end_time":   event.EndTime,
					"location":   event.Location,
					"status":     event.Status,
					"recurrence": event.Recurrence,
					"calendar":   cal.Name,
					"event":      event,
				},
			})
		}
	}

	return items
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func getBool(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

var calendarDirVariants = []string{
	"Calendar",
	"Calendario", // Italian/Spanish
}

func detectCalendarDir(sourceDir string) (string, error) {
	baseName := filepath.Base(sourceDir)
	for _, variant := range calendarDirVariants {
		if strings.EqualFold(baseName, variant) {
			return sourceDir, nil
		}
	}

	var found string
	filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(sourceDir, path)
		depth := len(strings.Split(rel, string(os.PathSeparator)))
		if depth > 3 {
			return filepath.SkipDir
		}
		for _, variant := range calendarDirVariants {
			if strings.EqualFold(d.Name(), variant) {
				found = path
				return filepath.SkipAll
			}
		}
		return nil
	})

	if found != "" {
		return found, nil
	}

	return sourceDir, nil
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	name = replacer.Replace(name)
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}
