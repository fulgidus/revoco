// Package common provides shared output modules that work across services.
package common

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fulgidus/revoco/services/core"
)

// ── Local Folder Output ──────────────────────────────────────────────────────

// LocalFolderOutput copies processed files to a local directory.
type LocalFolderOutput struct {
	destDir string
	useMove bool
}

// NewLocalFolder creates a new local folder output.
func NewLocalFolder() *LocalFolderOutput {
	return &LocalFolderOutput{}
}

func (o *LocalFolderOutput) ID() string          { return "local-folder" }
func (o *LocalFolderOutput) Name() string        { return "Local Folder" }
func (o *LocalFolderOutput) Description() string { return "Copy files to a local directory" }

func (o *LocalFolderOutput) SupportedItemTypes() []string {
	return []string{"photo", "video", "audio", "playlist", "document"}
}

func (o *LocalFolderOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Target directory for output files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "use_move",
			Name:        "Move Files",
			Description: "Move instead of copy (deletes source)",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "preserve_structure",
			Name:        "Preserve Structure",
			Description: "Keep the relative path structure from source",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (o *LocalFolderOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["use_move"].(bool); ok {
		o.useMove = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *LocalFolderOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	if o.useMove {
		if err := os.Rename(item.ProcessedPath, destPath); err != nil {
			// Cross-device, fall back to copy+delete
			if err := copyFile(item.ProcessedPath, destPath); err != nil {
				return err
			}
			os.Remove(item.ProcessedPath)
		}
	} else {
		if err := copyFile(item.ProcessedPath, destPath); err != nil {
			return err
		}
	}

	return nil
}

func (o *LocalFolderOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	total := len(items)
	for i, item := range items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := o.Export(ctx, item); err != nil {
			return fmt.Errorf("export %s: %w", item.DestRelPath, err)
		}

		if progress != nil {
			progress(i+1, total)
		}
	}
	return nil
}

func (o *LocalFolderOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── Streaming support ────────────────────────────────────────────────────────

func (o *LocalFolderOutput) SupportsStreaming() bool {
	return true
}

func (o *LocalFolderOutput) ExportStream(ctx context.Context, item core.ProcessedItem, reader io.Reader, size int64) error {
	destPath := filepath.Join(o.destDir, item.DestRelPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	return err
}

// ── Helper ───────────────────────────────────────────────────────────────────

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

// Register registers the local folder output globally.
func init() {
	_ = core.RegisterOutput(NewLocalFolder())
}
