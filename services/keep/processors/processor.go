// Package processors provides data processing for Google Keep Takeout.
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
	"github.com/fulgidus/revoco/services/keep/metadata"
)

// Processor handles the Keep note processing pipeline.
type Processor struct{}

// NewKeepProcessor creates a new Keep processor.
func NewKeepProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) ID() string   { return "keep-processor" }
func (p *Processor) Name() string { return "Keep Processor" }
func (p *Processor) Description() string {
	return "Process Google Keep Takeout JSON notes - extract and convert to Markdown/JSON/HTML"
}

// ConfigSchema returns the configuration options for this processor.
func (p *Processor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "preserve_colors",
			Name:        "Preserve Note Colors",
			Description: "Include color information in exported notes",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "export_archived",
			Name:        "Export Archived Notes",
			Description: "Include archived notes in export",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "export_trashed",
			Name:        "Export Trashed Notes",
			Description: "Include trashed notes in export",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "include_labels",
			Name:        "Include Labels",
			Description: "Include note labels/tags in output",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "timestamp_format",
			Name:        "Timestamp Format",
			Description: "Date format for timestamps (Go time layout)",
			Type:        "string",
			Default:     "2006-01-02 15:04:05",
		},
	}
}

// Process runs the Keep note processing pipeline.
// Phases: 1) Scan JSON files, 2) Parse notes, 3) Filter by status,
// 4) Convert to formats, 5) Generate summary
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

	preserveColors := getBool(settings, "preserve_colors", true)
	exportArchived := getBool(settings, "export_archived", true)
	exportTrashed := getBool(settings, "export_trashed", false)
	includeLabels := getBool(settings, "include_labels", true)
	timestampFormat := getString(settings, "timestamp_format", "2006-01-02 15:04:05")

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
	logger.Printf("=== Keep note processing started (source=%s) ===", cfg.SourceDir)

	// Find Keep directory
	keepPath, err := detectKeepDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(keepPath)))
	logger.Printf("[Setup] Keep directory: %s", keepPath)

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	library := &metadata.KeepLibrary{
		NotesPath: keepPath,
		Stats:     make(map[string]int),
	}

	// ── Phase 1: Scan for JSON files ────────────────────────────────────────
	emit(1, "Scanning note files", 0, 0, "")
	jsonFiles, err := p.scanJSONFiles(ctx, keepPath, logger)
	if err != nil {
		logger.Printf("[Phase 1] Error scanning JSON files: %v", err)
		return nil, err
	}
	result.Stats["json_files"] = len(jsonFiles)
	emit(1, "Scan complete", len(jsonFiles), len(jsonFiles),
		fmt.Sprintf("%d JSON files found", len(jsonFiles)))
	logger.Printf("[Phase 1] json_files=%d", len(jsonFiles))

	// ── Phase 2: Parse notes ────────────────────────────────────────────────
	emit(2, "Parsing notes", 0, len(jsonFiles), "")
	notes, parseErrors := p.parseNotes(ctx, jsonFiles, emit, logger)
	result.Stats["notes_parsed"] = len(notes)
	result.Stats["parse_errors"] = parseErrors
	emit(2, "Parse complete", len(notes), len(jsonFiles),
		fmt.Sprintf("%d notes parsed (%d errors)", len(notes), parseErrors))
	logger.Printf("[Phase 2] notes_parsed=%d, errors=%d", len(notes), parseErrors)

	// ── Phase 3: Filter notes ───────────────────────────────────────────────
	emit(3, "Filtering notes", 0, len(notes), "")
	filteredNotes := p.filterNotes(notes, exportArchived, exportTrashed, logger)
	library.Notes = filteredNotes
	result.Stats["notes_filtered"] = len(filteredNotes)
	result.Stats["notes_archived"] = countArchived(notes)
	result.Stats["notes_trashed"] = countTrashed(notes)
	result.Stats["notes_pinned"] = countPinned(notes)
	result.Stats["notes_with_checkboxes"] = countCheckboxes(notes)
	result.Stats["notes_with_labels"] = countLabels(notes)
	result.Stats["notes_with_attachments"] = countAttachments(notes)
	emit(3, "Filter complete", len(filteredNotes), len(notes),
		fmt.Sprintf("%d notes after filtering", len(filteredNotes)))
	logger.Printf("[Phase 3] notes_filtered=%d (archived=%d, trashed=%d)",
		len(filteredNotes), result.Stats["notes_archived"], result.Stats["notes_trashed"])

	// ── Phase 4: Convert notes ──────────────────────────────────────────────
	emit(4, "Converting notes", 0, len(filteredNotes), "")
	library.Stats = result.Stats
	result.Metadata["library"] = library
	result.Metadata["preserve_colors"] = preserveColors
	result.Metadata["include_labels"] = includeLabels
	result.Metadata["timestamp_format"] = timestampFormat
	emit(4, "Conversion complete", len(filteredNotes), len(filteredNotes), "")
	logger.Printf("[Phase 4] Notes ready for output")

	// ── Phase 5: Write summary ──────────────────────────────────────────────
	emit(5, "Writing metadata", 0, 1, "")

	// Write library as JSON
	libraryPath := filepath.Join(cfg.WorkDir, "library.json")
	libraryData, _ := json.MarshalIndent(library, "", "  ")
	os.WriteFile(libraryPath, libraryData, 0o644)

	result.Metadata["library_path"] = libraryPath

	emit(5, "Output complete", 1, 1, fmt.Sprintf("Wrote %s", filepath.Base(libraryPath)))
	logger.Printf("[Phase 5] Wrote library.json")
	logger.Printf("=== Keep note processing complete ===")

	// Build ProcessedItems
	result.Items = p.buildProcessedItems(filteredNotes, cfg.WorkDir)

	return result, nil
}

// scanJSONFiles finds all .json files in the Keep directory.
func (p *Processor) scanJSONFiles(ctx context.Context, keepPath string, logger *log.Logger) ([]string, error) {
	var jsonFiles []string

	err := filepath.WalkDir(keepPath, func(path string, d os.DirEntry, err error) error {
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

		// Keep notes are .json files
		if strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			jsonFiles = append(jsonFiles, path)
		}

		return nil
	})

	return jsonFiles, err
}

// parseNotes parses all JSON files into Note structs.
func (p *Processor) parseNotes(ctx context.Context, jsonFiles []string, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.Note, int) {
	var notes []metadata.Note
	parseErrors := 0

	for i, jsonPath := range jsonFiles {
		select {
		case <-ctx.Done():
			return notes, parseErrors
		default:
		}

		note, err := metadata.ParseKeepNote(jsonPath)
		if err != nil {
			logger.Printf("Error parsing %s: %v", jsonPath, err)
			parseErrors++
			continue
		}

		notes = append(notes, note)

		if i%10 == 0 {
			emit(2, "Parsing notes", i, len(jsonFiles), "")
		}
	}

	return notes, parseErrors
}

// filterNotes filters notes based on archived/trashed status.
func (p *Processor) filterNotes(notes []metadata.Note, exportArchived, exportTrashed bool, logger *log.Logger) []metadata.Note {
	var filtered []metadata.Note

	for _, note := range notes {
		// Skip trashed notes if not exporting them
		if note.IsTrashed && !exportTrashed {
			logger.Printf("Skipping trashed note: %s", note.Title)
			continue
		}

		// Skip archived notes if not exporting them
		if note.IsArchived && !exportArchived {
			logger.Printf("Skipping archived note: %s", note.Title)
			continue
		}

		filtered = append(filtered, note)
	}

	return filtered
}

// buildProcessedItems creates ProcessedItem entries for outputs.
func (p *Processor) buildProcessedItems(notes []metadata.Note, workDir string) []core.ProcessedItem {
	var items []core.ProcessedItem

	for _, note := range notes {
		// Generate a safe filename from title
		filename := sanitizeFilename(note.Title)
		if filename == "" {
			filename = "untitled"
		}

		// Use created timestamp for uniqueness
		timestamp := note.Created.Format("20060102_150405")
		if note.Created.IsZero() {
			timestamp = "unknown"
		}

		relPath := fmt.Sprintf("notes/%s_%s.md", timestamp, filename)

		items = append(items, core.ProcessedItem{
			SourcePath:    "",
			ProcessedPath: "",
			DestRelPath:   relPath,
			Type:          "note",
			Metadata: map[string]any{
				"title":           note.Title,
				"content":         note.TextContent,
				"color":           note.Color,
				"is_pinned":       note.IsPinned,
				"is_archived":     note.IsArchived,
				"is_trashed":      note.IsTrashed,
				"created":         note.Created,
				"modified":        note.Modified,
				"labels":          note.GetLabelNames(),
				"has_checkboxes":  note.HasCheckboxes(),
				"has_attachments": note.HasAttachments(),
				"has_annotations": note.HasAnnotations(),
				"content_type":    note.ContentType(),
				"note":            note,
			},
		})
	}

	return items
}

// ── Helper functions ─────────────────────────────────────────────────────────

func getBool(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func getString(m map[string]any, key string, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

var keepDirVariants = []string{
	"Keep",
	"Notes",
	"Google Keep",
}

func detectKeepDir(sourceDir string) (string, error) {
	baseName := filepath.Base(sourceDir)
	for _, variant := range keepDirVariants {
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
		for _, variant := range keepDirVariants {
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
		"\n", "_",
		"\r", "_",
		"\t", "_",
	)
	sanitized := replacer.Replace(name)
	// Limit length
	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}
	return sanitized
}

// Statistics helpers

func countArchived(notes []metadata.Note) int {
	count := 0
	for _, note := range notes {
		if note.IsArchived {
			count++
		}
	}
	return count
}

func countTrashed(notes []metadata.Note) int {
	count := 0
	for _, note := range notes {
		if note.IsTrashed {
			count++
		}
	}
	return count
}

func countPinned(notes []metadata.Note) int {
	count := 0
	for _, note := range notes {
		if note.IsPinned {
			count++
		}
	}
	return count
}

func countCheckboxes(notes []metadata.Note) int {
	count := 0
	for _, note := range notes {
		if note.HasCheckboxes() {
			count++
		}
	}
	return count
}

func countLabels(notes []metadata.Note) int {
	count := 0
	for _, note := range notes {
		if len(note.Labels) > 0 {
			count++
		}
	}
	return count
}

func countAttachments(notes []metadata.Note) int {
	count := 0
	for _, note := range notes {
		if note.HasAttachments() {
			count++
		}
	}
	return count
}
