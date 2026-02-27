package engine

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
)

// ProgressEvent is emitted by the pipeline to report progress to the TUI/CLI.
type ProgressEvent struct {
	Phase   int
	Label   string
	Done    int
	Total   int
	Message string // optional log message
}

// PipelineConfig holds all options for a full processing run.
type PipelineConfig struct {
	SourceDir  string
	DestDir    string
	SessionDir string // if set, logs go here instead of DestDir
	UseMove    bool
	DryRun     bool
}

// PipelineResult is the final output of a complete run.
type PipelineResult struct {
	Stats   Stats
	Report  *MissingReport
	LogPath string
}

// Run executes the full 8-phase pipeline, emitting progress events on the provided channel.
// The channel is closed when the run completes.
func Run(cfg PipelineConfig, events chan<- ProgressEvent) (*PipelineResult, error) {
	defer close(events)

	emit := func(phase int, label string, done, total int, msg string) {
		events <- ProgressEvent{
			Phase:   phase,
			Label:   label,
			Done:    done,
			Total:   total,
			Message: msg,
		}
	}

	// Setup logging
	if err := os.MkdirAll(cfg.DestDir, 0o755); err != nil {
		return nil, fmt.Errorf("create dest dir: %w", err)
	}
	logDir := cfg.DestDir
	if cfg.SessionDir != "" {
		logDir = cfg.SessionDir
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return nil, fmt.Errorf("create session dir: %w", err)
		}
	}
	logPath := filepath.Join(logDir, "process.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)
	logger.Printf("=== revoco run started (source=%s dest=%s dry=%v move=%v) ===",
		cfg.SourceDir, cfg.DestDir, cfg.DryRun, cfg.UseMove)

	// Detect Google Photos subfolder
	gfotoPath, err := detectGooglePhotosDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(gfotoPath)))

	stats := Stats{}

	// ── Phase 1: Index ──────────────────────────────────────────────────────
	emit(1, "Indexing files", 0, 0, "Scanning...")
	idx, err := IndexFiles(gfotoPath, func(done, total int) {
		emit(1, "Matching metadata", done, total, "")
	})
	if err != nil {
		return nil, fmt.Errorf("phase 1: %w", err)
	}
	stats.MediaFound = idx.TotalMedia
	stats.JSONMatched = idx.TotalMatched
	stats.JSONOrphans = idx.TotalOrphans
	emit(1, "Indexing done", idx.TotalJSON, idx.TotalJSON,
		fmt.Sprintf("%d media, %d matched, %d orphan JSONs", idx.TotalMedia, idx.TotalMatched, idx.TotalOrphans))
	logger.Printf("[Phase 1] media=%d matched=%d orphans=%d", idx.TotalMedia, idx.TotalMatched, idx.TotalOrphans)

	// ── Phase 2: Albums ─────────────────────────────────────────────────────
	emit(2, "Resolving albums", 0, 1, "")
	albums, err := AssignAlbums(gfotoPath, idx.MediaFiles)
	if err != nil {
		return nil, fmt.Errorf("phase 2: %w", err)
	}
	stats.Albums = len(albums.NamedAlbums)
	emit(2, "Albums resolved", 1, 1,
		fmt.Sprintf("%d named albums", stats.Albums))
	logger.Printf("[Phase 2] albums=%d", stats.Albums)

	// ── Phase 3: Dedup ──────────────────────────────────────────────────────
	dedup, err := DeduplicateFiles(idx.MediaFiles, albums.MediaAlbum, func(done, total int) {
		emit(3, "Deduplicating", done, total, "")
	})
	if err != nil {
		return nil, fmt.Errorf("phase 3: %w", err)
	}
	stats.DuplicatesRemoved = dedup.Duplicates
	emit(3, "Dedup done", len(dedup.Unique), len(dedup.Unique),
		fmt.Sprintf("%d unique, %d duplicates removed", len(dedup.Unique), dedup.Duplicates))
	logger.Printf("[Phase 3] unique=%d duplicates=%d", len(dedup.Unique), dedup.Duplicates)

	// ── Phase 4: Transfer ───────────────────────────────────────────────────
	xfer, err := TransferFiles(
		dedup.Unique,
		albums.MediaAlbum,
		idx.MediaFiles,
		dedup.MediaHash,
		TransferConfig{DestDir: cfg.DestDir, UseMove: cfg.UseMove, DryRun: cfg.DryRun},
		func(done, total int) {
			emit(4, "Transferring files", done, total, "")
		},
	)
	if err != nil {
		return nil, fmt.Errorf("phase 4: %w", err)
	}
	stats.FilesTransferred = xfer.FilesTransferred
	stats.ConflictsResolved = xfer.ConflictsResolved
	stats.Errors += xfer.Errors
	emit(4, "Transfer done", xfer.FilesTransferred, xfer.FilesTransferred,
		fmt.Sprintf("%d transferred, %d conflicts, %d errors", xfer.FilesTransferred, xfer.ConflictsResolved, xfer.Errors))
	logger.Printf("[Phase 4] transferred=%d conflicts=%d errors=%d", xfer.FilesTransferred, xfer.ConflictsResolved, xfer.Errors)

	// ── Phase 5: Motion Photos ──────────────────────────────────────────────
	converted, mpErrs := ConvertMotionPhotos(xfer.DestMap, cfg.DryRun, func(done, total int) {
		emit(5, "Converting motion photos", done, total, "")
	})
	stats.MPConverted = converted
	stats.Errors += len(mpErrs)
	emit(5, "Motion photos done", converted, converted,
		fmt.Sprintf("%d converted", converted))
	logger.Printf("[Phase 5] converted=%d errors=%d", converted, len(mpErrs))

	// ── Phase 6+7: EXIF ─────────────────────────────────────────────────────
	applied, dateFallback, exifErrCount, _ := ApplyEXIFBatch(
		xfer.DestMap,
		idx.MediaFiles,
		cfg.DryRun,
		func(done, total int) {
			emit(6, "Applying metadata", done, total, "")
		},
	)
	stats.EXIFApplied = applied
	stats.DateFromFilename = dateFallback
	stats.Errors += exifErrCount
	emit(6, "Metadata done", applied+dateFallback, len(xfer.DestMap),
		fmt.Sprintf("%d from JSON, %d from filename, %d errors", applied, dateFallback, exifErrCount))
	logger.Printf("[Phase 6+7] json=%d filename=%d errors=%d", applied, dateFallback, exifErrCount)

	// ── Phase 8: Report ─────────────────────────────────────────────────────
	emit(7, "Generating report", 0, 1, "")
	reportDir := cfg.DestDir
	if cfg.SessionDir != "" {
		reportDir = cfg.SessionDir
	}
	report, err := GenerateReport(idx.OrphanJSONs, reportDir)
	if err != nil {
		logger.Printf("[Phase 8] report error: %v", err)
	}
	emit(7, "Report done", 1, 1,
		fmt.Sprintf("%d missing entries", len(report.Entries)))
	logger.Printf("[Phase 8] missing=%d", len(report.Entries))
	logger.Printf("=== revoco run complete ===")

	return &PipelineResult{
		Stats:   stats,
		Report:  report,
		LogPath: logPath,
	}, nil
}

// DetectGooglePhotosDir finds the Google Photos locale subfolder inside sourceDir.
var googlePhotosVariants = []string{
	"Google Foto",   // Italian
	"Google Photos", // English
	"Google Fotos",  // Spanish/Portuguese
}

// googlePhotosLocalePattern matches "Google Foto" / "Google Photos" / "Google Fotos"
var googlePhotosLocaleRe = regexp.MustCompile(`(?i)^Google Fo(to|tos|tos|to)s?$`)

func detectGooglePhotosDir(sourceDir string) (string, error) {
	for _, variant := range googlePhotosVariants {
		p := filepath.Join(sourceDir, variant)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p, nil
		}
	}
	// Also accept being pointed directly at the Google Photos folder
	for _, variant := range googlePhotosVariants {
		if filepath.Base(sourceDir) == variant {
			return sourceDir, nil
		}
	}
	_ = googlePhotosLocaleRe
	return "", fmt.Errorf(
		"cannot find Google Photos folder in %q — expected one of: %v",
		sourceDir, googlePhotosVariants,
	)
}
