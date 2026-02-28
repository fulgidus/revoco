package engine

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// TransferConfig holds options for Phase 4.
type TransferConfig struct {
	DestDir string
	UseMove bool
	DryRun  bool
}

// TransferResult holds Phase 4 output.
type TransferResult struct {
	// DestMap maps source media path -> destination path
	DestMap           map[string]string
	FilesTransferred  int
	ConflictsResolved int
	Errors            int
	ErrorList         []error
}

// TransferFiles copies (or moves) deduplicated media files to the destination,
// preserving album subdirectory structure and resolving name conflicts.
// The context allows cancellation during the transfer loop.
func TransferFiles(
	ctx context.Context,
	unique []string,
	mediaAlbum map[string]string,
	mediaJSON map[string]string,
	mediaHash map[string]string,
	cfg TransferConfig,
	logger *log.Logger,
	progress func(done, total int),
) (*TransferResult, error) {
	if !cfg.DryRun {
		if err := os.MkdirAll(cfg.DestDir, 0o755); err != nil {
			return nil, err
		}
	}

	opType := "COPY"
	if cfg.UseMove {
		opType = "MOVE"
	}
	if cfg.DryRun {
		logger.Printf("[Transfer] DRY-RUN mode - simulating %s operations", opType)
	} else {
		logger.Printf("[Transfer] Starting %s to %s", opType, cfg.DestDir)
	}

	result := &TransferResult{
		DestMap: make(map[string]string, len(unique)),
	}
	total := len(unique)

	// Track which destination basenames are already in use to detect conflicts
	usedDest := make(map[string]bool, total)

	for i, src := range unique {
		// Check for cancellation
		select {
		case <-ctx.Done():
			logger.Printf("[Transfer] Cancelled after %d/%d files", i, total)
			return result, ctx.Err()
		default:
		}

		if progress != nil {
			progress(i, total)
		}

		album := mediaAlbum[src]
		base := filepath.Base(src)

		// Build destination path
		var destDir string
		if album != "" {
			destDir = filepath.Join(cfg.DestDir, album)
		} else {
			destDir = cfg.DestDir
		}

		dest := filepath.Join(destDir, base)
		wasConflict := false

		// Conflict resolution: append 6-char hash prefix to filename
		if usedDest[dest] {
			hash := mediaHash[src]
			if len(hash) >= 6 {
				hash = hash[:6]
			}
			ext := filepath.Ext(base)
			stem := strings.TrimSuffix(base, ext)
			newName := fmt.Sprintf("%s_%s%s", stem, hash, ext)
			dest = filepath.Join(destDir, newName)
			result.ConflictsResolved++
			wasConflict = true
			logger.Printf("[Transfer] CONFLICT resolved: %s -> %s (added hash suffix)", base, newName)
		}
		usedDest[dest] = true
		result.DestMap[src] = dest

		if cfg.DryRun {
			if wasConflict {
				// Already logged above
			} else if album != "" {
				logger.Printf("[Transfer] [DRY-RUN] Would %s: %s -> %s/%s", opType, base, album, base)
			} else {
				logger.Printf("[Transfer] [DRY-RUN] Would %s: %s", opType, base)
			}
			result.FilesTransferred++
			continue
		}

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			result.Errors++
			result.ErrorList = append(result.ErrorList, fmt.Errorf("mkdir %s: %w", destDir, err))
			logger.Printf("[Transfer] ERROR creating dir %s: %v", destDir, err)
			continue
		}

		var err error
		if cfg.UseMove {
			err = os.Rename(src, dest)
			if err != nil {
				// Cross-device move: copy then delete
				err = copyFile(src, dest)
				if err == nil {
					os.Remove(src)
					logger.Printf("[Transfer] MOVED (cross-device): %s -> %s", base, dest)
				}
			} else {
				logger.Printf("[Transfer] MOVED: %s -> %s", base, dest)
			}
		} else {
			err = copyFile(src, dest)
			if err == nil {
				logger.Printf("[Transfer] COPIED: %s -> %s", base, dest)
			}
		}

		if err != nil {
			result.Errors++
			result.ErrorList = append(result.ErrorList, fmt.Errorf("%s %s: %w", opType, base, err))
			logger.Printf("[Transfer] ERROR %s %s: %v", opType, base, err)
			continue
		}
		result.FilesTransferred++

		// Copy matched JSON to .metadata/ tree
		if jsonPath := mediaJSON[src]; jsonPath != "" {
			metaBase := filepath.Base(jsonPath)
			var metaDir string
			if album != "" {
				metaDir = filepath.Join(cfg.DestDir, ".metadata", album)
			} else {
				metaDir = filepath.Join(cfg.DestDir, ".metadata")
			}
			if err := os.MkdirAll(metaDir, 0o755); err == nil {
				if err := copyFile(jsonPath, filepath.Join(metaDir, metaBase)); err != nil {
					logger.Printf("[Transfer] WARNING: failed to copy JSON metadata %s: %v", metaBase, err)
				}
			}
		}
	}

	if progress != nil {
		progress(total, total)
	}

	logger.Printf("[Transfer] Complete: transferred=%d, conflicts=%d, errors=%d", result.FilesTransferred, result.ConflictsResolved, result.Errors)
	return result, nil
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
