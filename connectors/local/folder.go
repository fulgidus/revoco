// Package local provides connectors for local filesystem sources.
package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	core "github.com/fulgidus/revoco/connectors"
)

// ══════════════════════════════════════════════════════════════════════════════
// Folder Connector
// ══════════════════════════════════════════════════════════════════════════════

// FolderConnector reads and writes data from/to local folders.
type FolderConnector struct {
	cfg      core.ConnectorConfig
	rootPath string
}

// NewFolderConnector creates a new folder connector.
func NewFolderConnector() *FolderConnector {
	return &FolderConnector{}
}

// ── Identity ──────────────────────────────────────────────────────────────────

func (c *FolderConnector) ID() string   { return "local-folder" }
func (c *FolderConnector) Name() string { return "Local Folder" }
func (c *FolderConnector) Description() string {
	return "Read/write data from local filesystem directories"
}

// ── Capabilities ──────────────────────────────────────────────────────────────

func (c *FolderConnector) Capabilities() []core.ConnectorCapability {
	return []core.ConnectorCapability{
		core.CapabilityRead,
		core.CapabilityWrite,
		core.CapabilityList,
		core.CapabilityDelete,
	}
}

func (c *FolderConnector) SupportedDataTypes() []core.DataType {
	return []core.DataType{
		core.DataTypePhoto,
		core.DataTypeVideo,
		core.DataTypeAudio,
		core.DataTypeDocument,
		core.DataTypeUnknown,
	}
}

func (c *FolderConnector) RequiresAuth() bool { return false }
func (c *FolderConnector) AuthType() string   { return "none" }

// ── Configuration ─────────────────────────────────────────────────────────────

func (c *FolderConnector) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "path",
			Name:        "Folder Path",
			Description: "Path to the local folder",
			Type:        "path",
			Required:    true,
		},
		{
			ID:          "recursive",
			Name:        "Recursive",
			Description: "Include files in subdirectories",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "extensions",
			Name:        "File Extensions",
			Description: "Comma-separated list of extensions to include (empty = all)",
			Type:        "string",
			Default:     "",
		},
	}
}

func (c *FolderConnector) ValidateConfig(cfg core.ConnectorConfig) error {
	path, ok := cfg.Settings["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("folder connector: path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("folder connector: path not accessible: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("folder connector: %s is not a directory", path)
	}

	return nil
}

func (c *FolderConnector) FallbackOptions() []core.FallbackOption {
	return nil // Local folders typically don't have fallbacks
}

// ── Reader Implementation ─────────────────────────────────────────────────────

func (c *FolderConnector) Initialize(ctx context.Context, cfg core.ConnectorConfig) error {
	if err := c.ValidateConfig(cfg); err != nil {
		return err
	}

	c.cfg = cfg
	c.rootPath = cfg.Settings["path"].(string)

	return nil
}

func (c *FolderConnector) List(ctx context.Context, progress core.ProgressFunc) ([]core.DataItem, error) {
	recursive := true
	if v, ok := c.cfg.Settings["recursive"].(bool); ok {
		recursive = v
	}

	var extensions []string
	if extStr, ok := c.cfg.Settings["extensions"].(string); ok && extStr != "" {
		for _, ext := range strings.Split(extStr, ",") {
			ext = strings.TrimSpace(ext)
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			extensions = append(extensions, strings.ToLower(ext))
		}
	}

	var items []core.DataItem
	var total int

	// First pass: count files for progress
	if progress != nil {
		_ = filepath.Walk(c.rootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !recursive && filepath.Dir(path) != c.rootPath {
				return filepath.SkipDir
			}
			if matchesExtension(path, extensions) {
				total++
			}
			return nil
		})
	}

	// Second pass: collect items
	done := 0
	err := filepath.Walk(c.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories
		if info.IsDir() {
			if !recursive && path != c.rootPath {
				return filepath.SkipDir
			}
			return nil
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Filter by extension
		if !matchesExtension(path, extensions) {
			return nil
		}

		relPath, _ := filepath.Rel(c.rootPath, path)
		item := core.DataItem{
			ID:           relPath,
			Type:         detectDataType(path),
			Path:         path,
			SourceConnID: c.cfg.InstanceID,
			Size:         info.Size(),
			Metadata: map[string]any{
				"mod_time":  info.ModTime().Unix(),
				"mode":      info.Mode().String(),
				"rel_path":  relPath,
				"file_name": info.Name(),
			},
		}

		items = append(items, item)
		done++
		if progress != nil {
			progress(done, total)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("folder connector: walk failed: %w", err)
	}

	return items, nil
}

func (c *FolderConnector) Read(ctx context.Context, item core.DataItem) (io.ReadCloser, error) {
	path := item.Path
	if path == "" {
		path = filepath.Join(c.rootPath, item.ID)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("folder connector: open %s: %w", path, err)
	}
	return f, nil
}

func (c *FolderConnector) ReadTo(ctx context.Context, item core.DataItem, destPath string, mode core.ImportMode) error {
	srcPath := item.Path
	if srcPath == "" {
		srcPath = filepath.Join(c.rootPath, item.ID)
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("folder connector: create dest dir: %w", err)
	}

	switch mode {
	case core.ImportModeMove:
		// Try rename first (fast if same filesystem)
		if err := os.Rename(srcPath, destPath); err == nil {
			return nil
		}
		// Fall back to copy + delete
		if err := copyFile(srcPath, destPath); err != nil {
			return err
		}
		return os.Remove(srcPath)

	case core.ImportModeCopy:
		return copyFile(srcPath, destPath)

	case core.ImportModeReference:
		// Just create a symlink
		return os.Symlink(srcPath, destPath)

	default:
		return copyFile(srcPath, destPath)
	}
}

func (c *FolderConnector) Close() error {
	return nil
}

// ── Writer Implementation ─────────────────────────────────────────────────────

func (c *FolderConnector) Write(ctx context.Context, item core.DataItem, reader io.Reader) error {
	destPath := filepath.Join(c.rootPath, item.ID)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("folder connector: create dir: %w", err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("folder connector: create %s: %w", destPath, err)
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	if err != nil {
		return fmt.Errorf("folder connector: write %s: %w", destPath, err)
	}

	return nil
}

func (c *FolderConnector) WriteFrom(ctx context.Context, item core.DataItem, sourcePath string) error {
	destPath := filepath.Join(c.rootPath, item.ID)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("folder connector: create dir: %w", err)
	}

	return copyFile(sourcePath, destPath)
}

func (c *FolderConnector) WriteBatch(ctx context.Context, items []core.DataItem, getReader func(core.DataItem) (io.Reader, error), progress core.ProgressFunc) error {
	total := len(items)
	for i, item := range items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		reader, err := getReader(item)
		if err != nil {
			return fmt.Errorf("folder connector: get reader for %s: %w", item.ID, err)
		}

		if err := c.Write(ctx, item, reader); err != nil {
			return err
		}

		if progress != nil {
			progress(i+1, total)
		}
	}
	return nil
}

func (c *FolderConnector) Delete(ctx context.Context, item core.DataItem) error {
	path := item.Path
	if path == "" {
		path = filepath.Join(c.rootPath, item.ID)
	}
	return os.Remove(path)
}

// ══════════════════════════════════════════════════════════════════════════════
// Helpers
// ══════════════════════════════════════════════════════════════════════════════

func matchesExtension(path string, extensions []string) bool {
	if len(extensions) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range extensions {
		if ext == e {
			return true
		}
	}
	return false
}

func detectDataType(path string) core.DataType {
	ext := strings.ToLower(filepath.Ext(path))

	// Photo extensions
	photoExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".webp": true, ".heic": true, ".heif": true, ".raw": true,
		".cr2": true, ".nef": true, ".arw": true, ".dng": true,
		".tiff": true, ".tif": true, ".bmp": true,
	}
	if photoExts[ext] {
		return core.DataTypePhoto
	}

	// Video extensions
	videoExts := map[string]bool{
		".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
		".webm": true, ".m4v": true, ".wmv": true, ".flv": true,
		".3gp": true, ".mts": true, ".m2ts": true,
	}
	if videoExts[ext] {
		return core.DataTypeVideo
	}

	// Audio extensions
	audioExts := map[string]bool{
		".mp3": true, ".m4a": true, ".wav": true, ".flac": true,
		".aac": true, ".ogg": true, ".wma": true, ".opus": true,
	}
	if audioExts[ext] {
		return core.DataTypeAudio
	}

	// Document extensions
	docExts := map[string]bool{
		".pdf": true, ".doc": true, ".docx": true, ".txt": true,
		".md": true, ".json": true, ".xml": true, ".html": true,
		".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
	}
	if docExts[ext] {
		return core.DataTypeDocument
	}

	return core.DataTypeUnknown
}

// extractPaths parses paths from settings that can be either:
// - a []interface{} (JSON array from config file)
// - a []string (Go string slice)
// - a string (comma-separated paths)
func extractPaths(v any) []string {
	if v == nil {
		return nil
	}

	// Handle []interface{} (from JSON unmarshaling)
	if arr, ok := v.([]interface{}); ok {
		var paths []string
		for _, item := range arr {
			if s, ok := item.(string); ok && s != "" {
				paths = append(paths, strings.TrimSpace(s))
			}
		}
		return paths
	}

	// Handle []string directly
	if arr, ok := v.([]string); ok {
		var paths []string
		for _, s := range arr {
			if s = strings.TrimSpace(s); s != "" {
				paths = append(paths, s)
			}
		}
		return paths
	}

	// Handle comma-separated string
	if s, ok := v.(string); ok && s != "" {
		var paths []string
		for _, p := range strings.Split(s, ",") {
			if p = strings.TrimSpace(p); p != "" {
				paths = append(paths, p)
			}
		}
		return paths
	}

	return nil
}

func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer sf.Close()

	info, err := sf.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer df.Close()

	if _, err := io.Copy(df, sf); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	return nil
}

// ComputeChecksum calculates SHA256 checksum for a file.
func ComputeChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Registration
// ══════════════════════════════════════════════════════════════════════════════

// TestConnection verifies the folder path is accessible.
func (c *FolderConnector) TestConnection(ctx context.Context, cfg core.ConnectorConfig) error {
	path, ok := cfg.Settings["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied: %s", path)
		}
		return fmt.Errorf("cannot access path: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	// Try to list the directory to verify read access
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("cannot read directory: %w", err)
	}

	// Count files for informational purposes
	fileCount := 0
	for _, e := range entries {
		if !e.IsDir() {
			fileCount++
		}
	}

	return nil
}

func init() {
	// Auto-register with global registry
	core.RegisterConnector(func() core.Connector {
		return NewFolderConnector()
	})
}

// Ensure FolderConnector implements both Reader and Writer interfaces
var (
	_ core.ConnectorReader = (*FolderConnector)(nil)
	_ core.ConnectorWriter = (*FolderConnector)(nil)
	_ core.ConnectorTester = (*FolderConnector)(nil)
)
