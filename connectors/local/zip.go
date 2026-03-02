package local

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	core "github.com/fulgidus/revoco/connectors"
)

// ══════════════════════════════════════════════════════════════════════════════
// ZIP Connector
// ══════════════════════════════════════════════════════════════════════════════

// ZipConnector reads data from ZIP archives.
type ZipConnector struct {
	cfg     core.ConnectorConfig
	zipPath string
	reader  *zip.ReadCloser
	files   map[string]*zip.File // index by relative path
}

// NewZipConnector creates a new ZIP connector.
func NewZipConnector() *ZipConnector {
	return &ZipConnector{
		files: make(map[string]*zip.File),
	}
}

// ── Identity ──────────────────────────────────────────────────────────────────

func (c *ZipConnector) ID() string          { return "local-zip" }
func (c *ZipConnector) Name() string        { return "ZIP Archive" }
func (c *ZipConnector) Description() string { return "Read data from ZIP archive files" }

// ── Capabilities ──────────────────────────────────────────────────────────────

func (c *ZipConnector) Capabilities() []core.ConnectorCapability {
	return []core.ConnectorCapability{
		core.CapabilityRead,
		core.CapabilityList,
	}
}

func (c *ZipConnector) SupportedDataTypes() []core.DataType {
	return []core.DataType{
		core.DataTypePhoto,
		core.DataTypeVideo,
		core.DataTypeAudio,
		core.DataTypeDocument,
		core.DataTypeUnknown,
	}
}

func (c *ZipConnector) RequiresAuth() bool { return false }
func (c *ZipConnector) AuthType() string   { return "none" }

// ── Configuration ─────────────────────────────────────────────────────────────

func (c *ZipConnector) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "path",
			Name:        "ZIP File Path",
			Description: "Path to the ZIP archive",
			Type:        "path",
			Required:    true,
		},
		{
			ID:          "extensions",
			Name:        "File Extensions",
			Description: "Comma-separated list of extensions to include (empty = all)",
			Type:        "string",
			Default:     "",
		},
		{
			ID:          "strip_prefix",
			Name:        "Strip Prefix",
			Description: "Prefix to strip from paths inside the archive",
			Type:        "string",
			Default:     "",
		},
	}
}

func (c *ZipConnector) ValidateConfig(cfg core.ConnectorConfig) error {
	path, ok := cfg.Settings["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("zip connector: path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("zip connector: path not accessible: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("zip connector: %s is a directory, not a ZIP file", path)
	}

	// Try to open the ZIP to validate format
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("zip connector: invalid ZIP file: %w", err)
	}
	r.Close()

	return nil
}

func (c *ZipConnector) FallbackOptions() []core.FallbackOption {
	return nil
}

// ── Reader Implementation ─────────────────────────────────────────────────────

func (c *ZipConnector) Initialize(ctx context.Context, cfg core.ConnectorConfig) error {
	if err := c.ValidateConfig(cfg); err != nil {
		return err
	}

	c.cfg = cfg
	c.zipPath = cfg.Settings["path"].(string)

	// Open the ZIP file
	r, err := zip.OpenReader(c.zipPath)
	if err != nil {
		return fmt.Errorf("zip connector: open %s: %w", c.zipPath, err)
	}
	c.reader = r

	// Build file index
	c.files = make(map[string]*zip.File)
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			c.files[f.Name] = f
		}
	}

	return nil
}

func (c *ZipConnector) List(ctx context.Context, progress core.ProgressFunc) ([]core.DataItem, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("zip connector: not initialized")
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

	stripPrefix := ""
	if sp, ok := c.cfg.Settings["strip_prefix"].(string); ok {
		stripPrefix = sp
	}

	var items []core.DataItem
	total := len(c.files)
	done := 0

	for name, f := range c.files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Skip directories (shouldn't be in index but double-check)
		if f.FileInfo().IsDir() {
			continue
		}

		// Filter by extension
		if !matchesExtension(name, extensions) {
			done++
			continue
		}

		// Calculate relative path
		relPath := name
		if stripPrefix != "" {
			relPath = strings.TrimPrefix(name, stripPrefix)
			relPath = strings.TrimPrefix(relPath, "/")
		}

		item := core.DataItem{
			ID:           relPath,
			Type:         detectDataType(name),
			Path:         "",   // No local path until extracted
			RemoteID:     name, // Store original path in ZIP
			SourceConnID: c.cfg.InstanceID,
			Size:         int64(f.UncompressedSize64),
			Metadata: map[string]any{
				"zip_path":        name,
				"compressed_size": f.CompressedSize64,
				"mod_time":        f.Modified.Unix(),
				"file_name":       filepath.Base(name),
			},
		}

		items = append(items, item)
		done++
		if progress != nil {
			progress(done, total)
		}
	}

	return items, nil
}

func (c *ZipConnector) Read(ctx context.Context, item core.DataItem) (io.ReadCloser, error) {
	if c.reader == nil {
		return nil, fmt.Errorf("zip connector: not initialized")
	}

	// Find the file in the ZIP
	zipPath := item.RemoteID
	if zipPath == "" {
		if mp, ok := item.Metadata["zip_path"].(string); ok {
			zipPath = mp
		} else {
			zipPath = item.ID
		}
	}

	f, ok := c.files[zipPath]
	if !ok {
		return nil, fmt.Errorf("zip connector: file %s not found in archive", zipPath)
	}

	return f.Open()
}

func (c *ZipConnector) ReadTo(ctx context.Context, item core.DataItem, destPath string, mode core.ImportMode) error {
	reader, err := c.Read(ctx, item)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("zip connector: create dest dir: %w", err)
	}

	// Create destination file
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("zip connector: create %s: %w", destPath, err)
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	if err != nil {
		return fmt.Errorf("zip connector: extract to %s: %w", destPath, err)
	}

	// Note: ImportMode is largely ignored for ZIP (can't move from archive)
	// Reference mode doesn't make sense either, so we always copy
	return nil
}

func (c *ZipConnector) Close() error {
	if c.reader != nil {
		err := c.reader.Close()
		c.reader = nil
		c.files = nil
		return err
	}
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Multi-ZIP Support
// ══════════════════════════════════════════════════════════════════════════════

// MultiZipConnector reads data from multiple ZIP archives.
type MultiZipConnector struct {
	cfg      core.ConnectorConfig
	zipPaths []string
	readers  []*zip.ReadCloser
	files    map[string]*zipFileRef // index by relative path
}

type zipFileRef struct {
	zipIndex int
	file     *zip.File
}

// NewMultiZipConnector creates a connector for multiple ZIP files.
func NewMultiZipConnector() *MultiZipConnector {
	return &MultiZipConnector{
		files: make(map[string]*zipFileRef),
	}
}

func (c *MultiZipConnector) ID() string          { return "local-multi-zip" }
func (c *MultiZipConnector) Name() string        { return "Multiple ZIP Archives" }
func (c *MultiZipConnector) Description() string { return "Read data from multiple ZIP archive files" }

func (c *MultiZipConnector) Capabilities() []core.ConnectorCapability {
	return []core.ConnectorCapability{
		core.CapabilityRead,
		core.CapabilityList,
	}
}

func (c *MultiZipConnector) SupportedDataTypes() []core.DataType {
	return []core.DataType{
		core.DataTypePhoto,
		core.DataTypeVideo,
		core.DataTypeAudio,
		core.DataTypeDocument,
		core.DataTypeUnknown,
	}
}

func (c *MultiZipConnector) RequiresAuth() bool { return false }
func (c *MultiZipConnector) AuthType() string   { return "none" }

func (c *MultiZipConnector) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "paths",
			Name:        "ZIP File Paths",
			Description: "Comma-separated paths to ZIP archives",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "extensions",
			Name:        "File Extensions",
			Description: "Comma-separated list of extensions to include (empty = all)",
			Type:        "string",
			Default:     "",
		},
		{
			ID:          "strip_prefix",
			Name:        "Strip Prefix",
			Description: "Prefix to strip from paths inside archives",
			Type:        "string",
			Default:     "",
		},
	}
}

func (c *MultiZipConnector) ValidateConfig(cfg core.ConnectorConfig) error {
	paths := extractPaths(cfg.Settings["paths"])
	if len(paths) == 0 {
		return fmt.Errorf("multi-zip connector: paths is required")
	}

	for _, p := range paths {
		if p == "" {
			continue
		}

		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("multi-zip connector: %s not accessible: %w", p, err)
		}
		if info.IsDir() {
			return fmt.Errorf("multi-zip connector: %s is a directory", p)
		}

		r, err := zip.OpenReader(p)
		if err != nil {
			return fmt.Errorf("multi-zip connector: %s is not a valid ZIP: %w", p, err)
		}
		r.Close()
	}

	return nil
}

func (c *MultiZipConnector) FallbackOptions() []core.FallbackOption {
	return nil
}

func (c *MultiZipConnector) Initialize(ctx context.Context, cfg core.ConnectorConfig) error {
	if err := c.ValidateConfig(cfg); err != nil {
		return err
	}

	c.cfg = cfg

	// Parse paths (handles both array and comma-separated string)
	c.zipPaths = extractPaths(cfg.Settings["paths"])

	// Open all ZIP files
	c.readers = make([]*zip.ReadCloser, len(c.zipPaths))
	c.files = make(map[string]*zipFileRef)

	for i, path := range c.zipPaths {
		r, err := zip.OpenReader(path)
		if err != nil {
			c.Close() // Clean up already opened files
			return fmt.Errorf("multi-zip connector: open %s: %w", path, err)
		}
		c.readers[i] = r

		// Index files
		for _, f := range r.File {
			if !f.FileInfo().IsDir() {
				// Later files override earlier ones (allows for incremental archives)
				c.files[f.Name] = &zipFileRef{zipIndex: i, file: f}
			}
		}
	}

	return nil
}

func (c *MultiZipConnector) List(ctx context.Context, progress core.ProgressFunc) ([]core.DataItem, error) {
	if c.readers == nil {
		return nil, fmt.Errorf("multi-zip connector: not initialized")
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

	stripPrefix := ""
	if sp, ok := c.cfg.Settings["strip_prefix"].(string); ok {
		stripPrefix = sp
	}

	var items []core.DataItem
	total := len(c.files)
	done := 0

	for name, ref := range c.files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if !matchesExtension(name, extensions) {
			done++
			continue
		}

		relPath := name
		if stripPrefix != "" {
			relPath = strings.TrimPrefix(name, stripPrefix)
			relPath = strings.TrimPrefix(relPath, "/")
		}

		item := core.DataItem{
			ID:           relPath,
			Type:         detectDataType(name),
			RemoteID:     name,
			SourceConnID: c.cfg.InstanceID,
			Size:         int64(ref.file.UncompressedSize64),
			Metadata: map[string]any{
				"zip_path":        name,
				"zip_index":       ref.zipIndex,
				"zip_file":        c.zipPaths[ref.zipIndex],
				"compressed_size": ref.file.CompressedSize64,
				"mod_time":        ref.file.Modified.Unix(),
				"file_name":       filepath.Base(name),
			},
		}

		items = append(items, item)
		done++
		if progress != nil {
			progress(done, total)
		}
	}

	return items, nil
}

func (c *MultiZipConnector) Read(ctx context.Context, item core.DataItem) (io.ReadCloser, error) {
	if c.readers == nil {
		return nil, fmt.Errorf("multi-zip connector: not initialized")
	}

	zipPath := item.RemoteID
	if zipPath == "" {
		if mp, ok := item.Metadata["zip_path"].(string); ok {
			zipPath = mp
		} else {
			zipPath = item.ID
		}
	}

	ref, ok := c.files[zipPath]
	if !ok {
		return nil, fmt.Errorf("multi-zip connector: file %s not found", zipPath)
	}

	return ref.file.Open()
}

func (c *MultiZipConnector) ReadTo(ctx context.Context, item core.DataItem, destPath string, mode core.ImportMode) error {
	reader, err := c.Read(ctx, item)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("multi-zip connector: create dest dir: %w", err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("multi-zip connector: create %s: %w", destPath, err)
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	return err
}

func (c *MultiZipConnector) Close() error {
	var lastErr error
	for _, r := range c.readers {
		if r != nil {
			if err := r.Close(); err != nil {
				lastErr = err
			}
		}
	}
	c.readers = nil
	c.files = nil
	return lastErr
}

// ══════════════════════════════════════════════════════════════════════════════
// Registration
// ══════════════════════════════════════════════════════════════════════════════

// TestConnection verifies the ZIP file is accessible and valid.
func (c *ZipConnector) TestConnection(ctx context.Context, cfg core.ConnectorConfig) error {
	path, ok := cfg.Settings["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", path)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied: %s", path)
		}
		return fmt.Errorf("cannot access file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("path is a directory, expected ZIP file: %s", path)
	}

	// Try to open the ZIP to verify it's valid
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("invalid ZIP file: %w", err)
	}
	defer r.Close()

	// Count entries
	fileCount := 0
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			fileCount++
		}
	}

	return nil
}

// TestConnection verifies all ZIP files are accessible and valid.
func (c *MultiZipConnector) TestConnection(ctx context.Context, cfg core.ConnectorConfig) error {
	paths := extractPaths(cfg.Settings["paths"])
	if len(paths) == 0 {
		return fmt.Errorf("paths is required")
	}

	for _, path := range paths {
		if path == "" {
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file does not exist: %s", path)
			}
			if os.IsPermission(err) {
				return fmt.Errorf("permission denied: %s", path)
			}
			return fmt.Errorf("cannot access file %s: %w", path, err)
		}

		if info.IsDir() {
			return fmt.Errorf("path is a directory, expected ZIP file: %s", path)
		}

		r, err := zip.OpenReader(path)
		if err != nil {
			return fmt.Errorf("invalid ZIP file %s: %w", path, err)
		}
		r.Close()
	}

	return nil
}

// init() registration removed — local-zip and local-multi-zip are now provided by Lua plugins
// plugins/defaults/connectors/local-zip.lua
// plugins/defaults/connectors/local-multi-zip.lua

// Ensure interfaces are satisfied
var (
	_ core.ConnectorReader = (*ZipConnector)(nil)
	_ core.ConnectorReader = (*MultiZipConnector)(nil)
	_ core.ConnectorTester = (*ZipConnector)(nil)
	_ core.ConnectorTester = (*MultiZipConnector)(nil)
)
