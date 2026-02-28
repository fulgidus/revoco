// Package processors provides data processing modules for Google Photos.
package processors

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/fulgidus/revoco/engine"
	"github.com/fulgidus/revoco/services/core"
)

// PhotosProcessor handles the full Google Photos processing pipeline.
type PhotosProcessor struct{}

// NewPhotosProcessor creates a new photos processor.
func NewPhotosProcessor() *PhotosProcessor {
	return &PhotosProcessor{}
}

func (p *PhotosProcessor) ID() string   { return "google-photos-processor" }
func (p *PhotosProcessor) Name() string { return "Photos Processor" }
func (p *PhotosProcessor) Description() string {
	return "Process Google Photos: organize, deduplicate, apply metadata, convert motion photos"
}

// ConfigSchema returns the configuration options for this processor.
func (p *PhotosProcessor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "exif_embedding",
			Name:        "EXIF Embedding",
			Description: "Write metadata (dates, GPS, tags) into image/video files",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "album_organization",
			Name:        "Album Organization",
			Description: "Organize files into album-based folder structure",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "deduplication",
			Name:        "Deduplication",
			Description: "Remove duplicate files based on content hash",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "motion_photo_conversion",
			Name:        "Motion Photo Conversion",
			Description: "Convert Google motion photos (.MP) to standard MP4",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "use_move",
			Name:        "Move Files",
			Description: "Move files instead of copying (destructive)",
			Type:        "bool",
			Default:     false,
		},
	}
}

// Process runs the Google Photos processing pipeline.
func (p *PhotosProcessor) Process(ctx context.Context, cfg core.ProcessorConfig, events chan<- core.ProgressEvent) (*core.ProcessResult, error) {
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

	// Read settings with defaults
	settings := cfg.Settings
	if settings == nil {
		settings = make(map[string]any)
	}

	exifEnabled := getBool(settings, "exif_embedding", true)
	albumEnabled := getBool(settings, "album_organization", true)
	dedupEnabled := getBool(settings, "deduplication", true)
	motionEnabled := getBool(settings, "motion_photo_conversion", true)
	useMove := getBool(settings, "use_move", false)
	dryRun := cfg.DryRun

	// Setup logging
	logDir := cfg.SessionDir
	if logDir == "" {
		logDir = cfg.WorkDir
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	logPath := filepath.Join(logDir, "process.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)
	logger.Printf("=== Google Photos processing started (source=%s dry=%v move=%v) ===",
		cfg.SourceDir, dryRun, useMove)

	// Detect Google Photos subfolder
	gfotoPath, err := detectGooglePhotosDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(gfotoPath)))

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	// ── Phase 1: Index ──────────────────────────────────────────────────────
	emit(1, "Indexing files", 0, 0, "Scanning...")
	idx, err := engine.IndexFiles(ctx, gfotoPath, func(done, total int) {
		emit(1, "Matching metadata", done, total, "")
	})
	if err != nil {
		return nil, fmt.Errorf("phase 1 index: %w", err)
	}
	result.Stats["media_found"] = idx.TotalMedia
	result.Stats["json_matched"] = idx.TotalMatched
	result.Stats["json_orphans"] = idx.TotalOrphans
	emit(1, "Indexing done", idx.TotalJSON, idx.TotalJSON,
		fmt.Sprintf("%d media, %d matched, %d orphan JSONs", idx.TotalMedia, idx.TotalMatched, idx.TotalOrphans))
	logger.Printf("[Phase 1] media=%d matched=%d orphans=%d", idx.TotalMedia, idx.TotalMatched, idx.TotalOrphans)

	// ── Phase 2: Albums ─────────────────────────────────────────────────────
	var albums *engine.AlbumsResult
	if albumEnabled {
		emit(2, "Resolving albums", 0, 1, "")
		albums, err = engine.AssignAlbums(gfotoPath, idx.MediaFiles)
		if err != nil {
			return nil, fmt.Errorf("phase 2 albums: %w", err)
		}
		result.Stats["albums"] = len(albums.NamedAlbums)
		emit(2, "Albums resolved", 1, 1, fmt.Sprintf("%d named albums", len(albums.NamedAlbums)))
		logger.Printf("[Phase 2] albums=%d", len(albums.NamedAlbums))
	} else {
		// No album organization - all files go to root
		albums = &engine.AlbumsResult{
			MediaAlbum: make(map[string]string, len(idx.MediaFiles)),
		}
		for path := range idx.MediaFiles {
			albums.MediaAlbum[path] = ""
		}
		emit(2, "Albums skipped", 1, 1, "Album organization disabled")
		logger.Printf("[Phase 2] Skipped - album organization disabled")
	}

	// ── Phase 3: Dedup ──────────────────────────────────────────────────────
	var dedup *engine.HashResult
	if dedupEnabled {
		dedup, err = engine.DeduplicateFiles(idx.MediaFiles, albums.MediaAlbum, func(done, total int) {
			emit(3, "Deduplicating", done, total, "")
		})
		if err != nil {
			return nil, fmt.Errorf("phase 3 dedup: %w", err)
		}
		result.Stats["duplicates_removed"] = dedup.Duplicates
		emit(3, "Dedup done", len(dedup.Unique), len(dedup.Unique),
			fmt.Sprintf("%d unique, %d duplicates removed", len(dedup.Unique), dedup.Duplicates))
		logger.Printf("[Phase 3] unique=%d duplicates=%d", len(dedup.Unique), dedup.Duplicates)
	} else {
		// No deduplication - keep all files
		unique := make([]string, 0, len(idx.MediaFiles))
		hashes := make(map[string]string, len(idx.MediaFiles))
		for path := range idx.MediaFiles {
			unique = append(unique, path)
			hashes[path] = "" // Empty hash, won't affect conflict resolution
		}
		dedup = &engine.HashResult{
			Unique:    unique,
			MediaHash: hashes,
		}
		emit(3, "Dedup skipped", len(unique), len(unique), "Deduplication disabled")
		logger.Printf("[Phase 3] Skipped - deduplication disabled")
	}

	// ── Phase 4: Transfer ───────────────────────────────────────────────────
	xfer, err := engine.TransferFiles(
		ctx,
		dedup.Unique,
		albums.MediaAlbum,
		idx.MediaFiles,
		dedup.MediaHash,
		engine.TransferConfig{DestDir: cfg.WorkDir, UseMove: useMove, DryRun: dryRun},
		logger,
		func(done, total int) {
			emit(4, "Transferring files", done, total, "")
		},
	)
	if err != nil {
		return nil, fmt.Errorf("phase 4 transfer: %w", err)
	}
	result.Stats["files_transferred"] = xfer.FilesTransferred
	result.Stats["conflicts_resolved"] = xfer.ConflictsResolved
	result.Stats["errors"] = xfer.Errors
	emit(4, "Transfer done", xfer.FilesTransferred, xfer.FilesTransferred,
		fmt.Sprintf("%d transferred, %d conflicts, %d errors", xfer.FilesTransferred, xfer.ConflictsResolved, xfer.Errors))
	logger.Printf("[Phase 4] transferred=%d conflicts=%d errors=%d", xfer.FilesTransferred, xfer.ConflictsResolved, xfer.Errors)

	// ── Phase 5: Motion Photos ──────────────────────────────────────────────
	if motionEnabled {
		converted, mpErrs := engine.ConvertMotionPhotos(xfer.DestMap, dryRun, logger, func(done, total int) {
			emit(5, "Converting motion photos", done, total, "")
		})
		result.Stats["motion_photos_converted"] = converted
		result.Stats["errors"] = result.Stats["errors"] + len(mpErrs)
		emit(5, "Motion photos done", converted, converted, fmt.Sprintf("%d converted", converted))
		logger.Printf("[Phase 5] converted=%d errors=%d", converted, len(mpErrs))
	} else {
		emit(5, "Motion photos skipped", 1, 1, "Motion photo conversion disabled")
		logger.Printf("[Phase 5] Skipped - motion photo conversion disabled")
	}

	// ── Phase 6+7: EXIF ─────────────────────────────────────────────────────
	if exifEnabled {
		applied, dateFallback, exifSkipped, exifErrCount, _, exifFatalErr := engine.ApplyEXIFBatch(
			ctx,
			xfer.DestMap,
			idx.MediaFiles,
			dryRun,
			logger,
			func(done, total int) {
				emit(6, "Applying metadata", done, total, "")
			},
		)
		if exifFatalErr != nil {
			return nil, fmt.Errorf("phase 6+7 exif: %w", exifFatalErr)
		}
		result.Stats["exif_applied"] = applied
		result.Stats["date_from_filename"] = dateFallback
		result.Stats["errors"] = result.Stats["errors"] + exifErrCount
		emit(6, "Metadata done", applied+dateFallback, len(xfer.DestMap),
			fmt.Sprintf("%d from JSON, %d from filename, %d skipped, %d errors", applied, dateFallback, exifSkipped, exifErrCount))
		logger.Printf("[Phase 6+7] json=%d filename=%d skipped=%d errors=%d", applied, dateFallback, exifSkipped, exifErrCount)
	} else {
		emit(6, "Metadata skipped", 1, 1, "EXIF embedding disabled")
		logger.Printf("[Phase 6+7] Skipped - EXIF embedding disabled")
	}

	// ── Phase 8: Generate Report ────────────────────────────────────────────
	emit(7, "Generating report", 0, 1, "")
	reportDir := cfg.SessionDir
	if reportDir == "" {
		reportDir = cfg.WorkDir
	}
	report, err := engine.GenerateReport(idx.OrphanJSONs, reportDir)
	if err != nil {
		logger.Printf("[Phase 8] report error: %v", err)
	}
	result.Stats["missing_entries"] = len(report.Entries)
	result.Metadata["missing_report_path"] = report.Path
	result.Metadata["orphan_jsons"] = idx.OrphanJSONs
	emit(7, "Report done", 1, 1, fmt.Sprintf("%d missing entries", len(report.Entries)))
	logger.Printf("[Phase 8] missing=%d", len(report.Entries))
	logger.Printf("=== Google Photos processing complete ===")

	// Build ProcessedItems for output modules
	result.Items = make([]core.ProcessedItem, 0, len(xfer.DestMap))
	for src, dest := range xfer.DestMap {
		relPath, _ := filepath.Rel(cfg.WorkDir, dest)
		item := core.ProcessedItem{
			SourcePath:    src,
			ProcessedPath: dest,
			DestRelPath:   relPath,
			Type:          "photo", // Could be "video" based on extension
			Metadata: map[string]any{
				"album":     albums.MediaAlbum[src],
				"json_path": idx.MediaFiles[src],
			},
		}
		result.Items = append(result.Items, item)
	}

	return result, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func getBool(m map[string]any, key string, def bool) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

var googlePhotosVariants = []string{
	"Google Foto",   // Italian
	"Google Photos", // English
	"Google Fotos",  // Spanish/Portuguese
}

func detectGooglePhotosDir(sourceDir string) (string, error) {
	baseName := filepath.Base(sourceDir)
	for _, variant := range googlePhotosVariants {
		if equalFold(baseName, variant) {
			return sourceDir, nil
		}
	}

	// Recursively search (up to 3 levels)
	var found string
	filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(sourceDir, path)
		depth := len(filepath.SplitList(rel))
		if depth > 3 {
			return filepath.SkipDir
		}
		for _, variant := range googlePhotosVariants {
			if equalFold(d.Name(), variant) {
				found = path
				return filepath.SkipAll
			}
		}
		return nil
	})

	if found != "" {
		return found, nil
	}

	return "", fmt.Errorf("cannot find Google Photos folder in %q — searched for: %v", sourceDir, googlePhotosVariants)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
