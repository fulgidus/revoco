package engine

import (
	"fmt"
	"io"
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
}

// TransferFiles copies (or moves) deduplicated media files to the destination,
// preserving album subdirectory structure and resolving name conflicts.
func TransferFiles(
	unique []string,
	mediaAlbum map[string]string,
	mediaJSON map[string]string,
	mediaHash map[string]string,
	cfg TransferConfig,
	progress func(done, total int),
) (*TransferResult, error) {
	if err := os.MkdirAll(cfg.DestDir, 0o755); err != nil {
		return nil, err
	}

	result := &TransferResult{
		DestMap: make(map[string]string, len(unique)),
	}
	total := len(unique)

	// Track which destination basenames are already in use to detect conflicts
	usedDest := make(map[string]bool, total)

	for i, src := range unique {
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

		// Conflict resolution: append 6-char hash prefix to filename
		if usedDest[dest] {
			hash := mediaHash[src]
			if len(hash) >= 6 {
				hash = hash[:6]
			}
			ext := filepath.Ext(base)
			stem := strings.TrimSuffix(base, ext)
			dest = filepath.Join(destDir, fmt.Sprintf("%s_%s%s", stem, hash, ext))
			result.ConflictsResolved++
		}
		usedDest[dest] = true
		result.DestMap[src] = dest

		if cfg.DryRun {
			result.FilesTransferred++
			continue
		}

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			result.Errors++
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
				}
			}
		} else {
			err = copyFile(src, dest)
		}

		if err != nil {
			result.Errors++
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
				copyFile(jsonPath, filepath.Join(metaDir, metaBase)) //nolint:errcheck
			}
		}
	}

	if progress != nil {
		progress(total, total)
	}

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
