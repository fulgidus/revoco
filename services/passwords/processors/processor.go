// Package processors provides data processing for Google Passwords Takeout.
package processors

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/passwords/metadata"
)

// Processor handles the Passwords CSV processing pipeline.
type Processor struct{}

// NewPasswordsProcessor creates a new Passwords processor.
func NewPasswordsProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) ID() string   { return "passwords-processor" }
func (p *Processor) Name() string { return "Passwords Processor" }
func (p *Processor) Description() string {
	return "Process Google Passwords Takeout - CSV password exports"
}

// ConfigSchema returns the configuration options for this processor.
func (p *Processor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{}
}

// Process runs the Passwords processing pipeline.
// Phases: 1) Scan files, 2) Parse CSV, 3) Sanitize, 4) Generate summary
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
	logger.Printf("=== Passwords processing started (source=%s) ===", cfg.SourceDir)

	// Find Passwords/Chrome directory
	passwordsPath, err := detectPasswordsDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(passwordsPath)))
	logger.Printf("[Setup] Passwords directory: %s", passwordsPath)

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	library := &metadata.PasswordLibrary{
		SourcePath: passwordsPath,
	}

	// Phase 1: Scan for CSV files
	csvFiles, err := scanPasswordCSVFiles(passwordsPath)
	if err != nil {
		return nil, fmt.Errorf("scan CSV files: %w", err)
	}
	emit(1, "Scan", len(csvFiles), len(csvFiles), fmt.Sprintf("Found %d password CSV files", len(csvFiles)))
	logger.Printf("[Scan] Found %d CSV files", len(csvFiles))

	if len(csvFiles) == 0 {
		return nil, fmt.Errorf("no password CSV files found in %s", passwordsPath)
	}

	// Phase 2: Parse CSV files
	allEntries := []metadata.PasswordEntry{}
	for i, csvPath := range csvFiles {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		f, err := os.Open(csvPath)
		if err != nil {
			logger.Printf("[Parse] ERROR: Failed to open %s: %v", filepath.Base(csvPath), err)
			continue
		}

		entries, err := metadata.ParsePasswordsCSV(f)
		f.Close()
		if err != nil {
			logger.Printf("[Parse] ERROR: Failed to parse %s: %v", filepath.Base(csvPath), err)
			continue
		}

		allEntries = append(allEntries, entries...)
		logger.Printf("[Parse] %s: %d entries", filepath.Base(csvPath), len(entries))
		emit(2, "Parse", i+1, len(csvFiles), fmt.Sprintf("Parsed %d entries from %s", len(entries), filepath.Base(csvPath)))
	}

	library.Entries = allEntries
	emit(2, "Parse", len(csvFiles), len(csvFiles), fmt.Sprintf("Total: %d password entries", len(allEntries)))

	// Phase 3: Sanitize and log warnings
	sanitizeWarnings := 0
	for i, entry := range library.Entries {
		if entry.URL == "" {
			logger.Printf("[Sanitize] WARNING: Entry #%d has empty URL (username: %s)", i+1, entry.Username)
			sanitizeWarnings++
		}
		if entry.Username == "" {
			logger.Printf("[Sanitize] WARNING: Entry #%d has empty username (URL: %s)", i+1, entry.URL)
			sanitizeWarnings++
		}
		// Never log password values
	}
	emit(3, "Sanitize", 1, 1, fmt.Sprintf("%d warnings found", sanitizeWarnings))
	logger.Printf("[Sanitize] %d warnings logged", sanitizeWarnings)

	// Phase 4: Calculate statistics
	library.CalculateStats()
	emit(4, "Summary", 1, 1, fmt.Sprintf("%d entries, %d unique domains", library.Stats.TotalEntries, library.Stats.UniqueDomains))
	logger.Printf("[Summary] Total entries: %d", library.Stats.TotalEntries)
	logger.Printf("[Summary] Unique domains: %d", library.Stats.UniqueDomains)
	logger.Printf("[Summary] Entries with notes: %d", library.Stats.EntriesWithNotes)
	logger.Printf("[Summary] Entries without URL: %d", library.Stats.EntriesNoURL)
	logger.Printf("[Summary] Entries without username: %d", library.Stats.EntriesNoUsername)

	// Create ProcessedItem
	item := core.ProcessedItem{
		Type:       "passwords_library",
		SourcePath: passwordsPath,
		Metadata: map[string]any{
			"passwords_library": library, // Store for outputs
		},
	}

	result.Items = []core.ProcessedItem{item}
	result.Stats["total_entries"] = library.Stats.TotalEntries
	result.Stats["unique_domains"] = library.Stats.UniqueDomains
	result.Stats["entries_with_notes"] = library.Stats.EntriesWithNotes
	result.Stats["warnings"] = sanitizeWarnings
	result.Metadata["library"] = library

	logger.Printf("=== Passwords processing complete ===")
	return result, nil
}

// detectPasswordsDir finds the Passwords or Chrome directory.
func detectPasswordsDir(sourceDir string) (string, error) {
	candidates := []string{
		filepath.Join(sourceDir, "Passwords"),
		filepath.Join(sourceDir, "Chrome"),
		filepath.Join(sourceDir, "Password"),
	}

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, nil
		}
	}

	// Try case-insensitive search
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return "", fmt.Errorf("read source dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		lower := strings.ToLower(entry.Name())
		if strings.Contains(lower, "password") || strings.Contains(lower, "chrome") {
			return filepath.Join(sourceDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no Passwords/Chrome directory found in %s", sourceDir)
}

// scanPasswordCSVFiles finds all password CSV files.
func scanPasswordCSVFiles(dir string) ([]string, error) {
	var csvFiles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Check for .csv extension
		if !strings.HasSuffix(strings.ToLower(path), ".csv") {
			return nil
		}

		// Check if it's a password CSV by reading first 512 bytes
		f, err := os.Open(path)
		if err != nil {
			return nil // Skip files we can't read
		}
		defer f.Close()

		buf := make([]byte, 512)
		n, _ := f.Read(buf)
		content := strings.ToLower(string(buf[:n]))

		// Must contain "password" and either "username" or "url"
		if strings.Contains(content, "password") &&
			(strings.Contains(content, "username") || strings.Contains(content, "url")) {
			csvFiles = append(csvFiles, path)
		}

		return nil
	})

	return csvFiles, err
}
