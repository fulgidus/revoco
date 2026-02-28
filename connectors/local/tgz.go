package local

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	core "github.com/fulgidus/revoco/connectors"
)

// ══════════════════════════════════════════════════════════════════════════════
// TGZ Connector
// ══════════════════════════════════════════════════════════════════════════════

// TgzConnector reads data from .tar.gz/.tgz archives.
// Unlike ZIP, tar.gz archives must be read sequentially, so we extract
// to a temp directory on initialization for random access.
type TgzConnector struct {
	cfg       core.ConnectorConfig
	tgzPath   string
	extractTo string              // Temp directory for extraction
	files     map[string]fileInfo // index by relative path
	extracted bool
}

type fileInfo struct {
	path    string // Full path in extracted location
	size    int64
	modTime int64
}

// NewTgzConnector creates a new TGZ connector.
func NewTgzConnector() *TgzConnector {
	return &TgzConnector{
		files: make(map[string]fileInfo),
	}
}

// ── Identity ──────────────────────────────────────────────────────────────────

func (c *TgzConnector) ID() string          { return "local-tgz" }
func (c *TgzConnector) Name() string        { return "TGZ Archive" }
func (c *TgzConnector) Description() string { return "Read data from .tar.gz/.tgz archive files" }

// ── Capabilities ──────────────────────────────────────────────────────────────

func (c *TgzConnector) Capabilities() []core.ConnectorCapability {
	return []core.ConnectorCapability{
		core.CapabilityRead,
		core.CapabilityList,
	}
}

func (c *TgzConnector) SupportedDataTypes() []core.DataType {
	return []core.DataType{
		core.DataTypePhoto,
		core.DataTypeVideo,
		core.DataTypeAudio,
		core.DataTypeDocument,
		core.DataTypeUnknown,
	}
}

func (c *TgzConnector) RequiresAuth() bool { return false }
func (c *TgzConnector) AuthType() string   { return "none" }

// ── Configuration ─────────────────────────────────────────────────────────────

func (c *TgzConnector) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "path",
			Name:        "TGZ File Path",
			Description: "Path to the .tar.gz or .tgz archive",
			Type:        "path",
			Required:    true,
		},
		{
			ID:          "extract_dir",
			Name:        "Extract Directory",
			Description: "Directory to extract files (temp dir if empty)",
			Type:        "path",
			Default:     "",
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
		{
			ID:          "lazy_extract",
			Name:        "Lazy Extract",
			Description: "Only index files, extract on demand (slower reads but saves disk)",
			Type:        "bool",
			Default:     false,
		},
	}
}

func (c *TgzConnector) ValidateConfig(cfg core.ConnectorConfig) error {
	path, ok := cfg.Settings["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("tgz connector: path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("tgz connector: path not accessible: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("tgz connector: %s is a directory, not a TGZ file", path)
	}

	// Try to open and verify it's a valid gzip/tar
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("tgz connector: open %s: %w", path, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("tgz connector: invalid gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	_, err = tr.Next()
	if err != nil && err != io.EOF {
		return fmt.Errorf("tgz connector: invalid tar: %w", err)
	}

	return nil
}

func (c *TgzConnector) FallbackOptions() []core.FallbackOption {
	return nil
}

// ── Reader Implementation ─────────────────────────────────────────────────────

func (c *TgzConnector) Initialize(ctx context.Context, cfg core.ConnectorConfig) error {
	if err := c.ValidateConfig(cfg); err != nil {
		return err
	}

	c.cfg = cfg
	c.tgzPath = cfg.Settings["path"].(string)

	// Determine extraction directory
	if extractDir, ok := cfg.Settings["extract_dir"].(string); ok && extractDir != "" {
		c.extractTo = extractDir
	} else {
		// Create temp directory
		tmpDir, err := os.MkdirTemp("", "revoco-tgz-*")
		if err != nil {
			return fmt.Errorf("tgz connector: create temp dir: %w", err)
		}
		c.extractTo = tmpDir
	}

	// Check if lazy extraction is enabled
	lazyExtract := false
	if v, ok := cfg.Settings["lazy_extract"].(bool); ok {
		lazyExtract = v
	}

	if lazyExtract {
		// Just index the files without extracting
		return c.indexFiles(ctx)
	}

	// Extract all files
	return c.extractAll(ctx)
}

func (c *TgzConnector) indexFiles(ctx context.Context) error {
	f, err := os.Open(c.tgzPath)
	if err != nil {
		return fmt.Errorf("tgz connector: open %s: %w", c.tgzPath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("tgz connector: gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	c.files = make(map[string]fileInfo)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tgz connector: read tar: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		c.files[hdr.Name] = fileInfo{
			path:    "", // Not extracted yet
			size:    hdr.Size,
			modTime: hdr.ModTime.Unix(),
		}
	}

	return nil
}

func (c *TgzConnector) extractAll(ctx context.Context) error {
	f, err := os.Open(c.tgzPath)
	if err != nil {
		return fmt.Errorf("tgz connector: open %s: %w", c.tgzPath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("tgz connector: gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	c.files = make(map[string]fileInfo)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tgz connector: read tar: %w", err)
		}

		target := filepath.Join(c.extractTo, hdr.Name)

		// Guard against tar-slip
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(c.extractTo)+string(os.PathSeparator)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("tgz connector: mkdir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("tgz connector: mkdir for %s: %w", target, err)
			}

			wf, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("tgz connector: create %s: %w", target, err)
			}

			if _, err := io.Copy(wf, tr); err != nil {
				wf.Close()
				return fmt.Errorf("tgz connector: extract %s: %w", target, err)
			}
			wf.Close()

			c.files[hdr.Name] = fileInfo{
				path:    target,
				size:    hdr.Size,
				modTime: hdr.ModTime.Unix(),
			}
		}
	}

	c.extracted = true
	return nil
}

func (c *TgzConnector) List(ctx context.Context, progress core.ProgressFunc) ([]core.DataItem, error) {
	if c.files == nil {
		return nil, fmt.Errorf("tgz connector: not initialized")
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

	for name, info := range c.files {
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
			Path:         info.path, // Empty if not extracted
			RemoteID:     name,
			SourceConnID: c.cfg.InstanceID,
			Size:         info.size,
			Metadata: map[string]any{
				"tgz_path":  name,
				"mod_time":  info.modTime,
				"file_name": filepath.Base(name),
				"extracted": info.path != "",
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

func (c *TgzConnector) Read(ctx context.Context, item core.DataItem) (io.ReadCloser, error) {
	// If file was extracted, read from disk
	if item.Path != "" {
		return os.Open(item.Path)
	}

	// Otherwise, need to extract on-the-fly from the archive
	tgzPath := item.RemoteID
	if tgzPath == "" {
		if tp, ok := item.Metadata["tgz_path"].(string); ok {
			tgzPath = tp
		} else {
			tgzPath = item.ID
		}
	}

	return c.extractSingle(tgzPath)
}

func (c *TgzConnector) extractSingle(targetPath string) (io.ReadCloser, error) {
	f, err := os.Open(c.tgzPath)
	if err != nil {
		return nil, fmt.Errorf("tgz connector: open %s: %w", c.tgzPath, err)
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("tgz connector: gzip reader: %w", err)
	}

	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			gz.Close()
			f.Close()
			return nil, fmt.Errorf("tgz connector: file %s not found in archive", targetPath)
		}
		if err != nil {
			gz.Close()
			f.Close()
			return nil, fmt.Errorf("tgz connector: read tar: %w", err)
		}

		if hdr.Name == targetPath {
			// Return a reader that closes the underlying file when done
			return &tgzReader{
				Reader: tr,
				closer: func() error {
					gz.Close()
					return f.Close()
				},
			}, nil
		}
	}
}

// tgzReader wraps tar reader to properly close underlying file
type tgzReader struct {
	io.Reader
	closer func() error
}

func (r *tgzReader) Close() error {
	if r.closer != nil {
		return r.closer()
	}
	return nil
}

func (c *TgzConnector) ReadTo(ctx context.Context, item core.DataItem, destPath string, mode core.ImportMode) error {
	// If already extracted, copy from extracted location
	if item.Path != "" && c.extracted {
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("tgz connector: create dest dir: %w", err)
		}
		return copyFile(item.Path, destPath)
	}

	// Extract on-the-fly
	reader, err := c.Read(ctx, item)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("tgz connector: create dest dir: %w", err)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("tgz connector: create %s: %w", destPath, err)
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	return err
}

func (c *TgzConnector) Close() error {
	// Clean up temp directory if we created one
	if c.extractTo != "" {
		// Only remove if it was a temp dir we created
		if strings.Contains(c.extractTo, "revoco-tgz-") {
			os.RemoveAll(c.extractTo)
		}
	}
	c.files = nil
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Multi-TGZ Support
// ══════════════════════════════════════════════════════════════════════════════

// MultiTgzConnector handles multiple TGZ archives.
type MultiTgzConnector struct {
	cfg       core.ConnectorConfig
	tgzPaths  []string
	extractTo string
	files     map[string]multiFileInfo
	extracted bool
}

type multiFileInfo struct {
	tgzIndex int
	path     string // Full path in extracted location
	size     int64
	modTime  int64
}

// NewMultiTgzConnector creates a connector for multiple TGZ files.
func NewMultiTgzConnector() *MultiTgzConnector {
	return &MultiTgzConnector{
		files: make(map[string]multiFileInfo),
	}
}

func (c *MultiTgzConnector) ID() string   { return "local-multi-tgz" }
func (c *MultiTgzConnector) Name() string { return "Multiple TGZ Archives" }
func (c *MultiTgzConnector) Description() string {
	return "Read data from multiple .tar.gz/.tgz archives"
}

func (c *MultiTgzConnector) Capabilities() []core.ConnectorCapability {
	return []core.ConnectorCapability{
		core.CapabilityRead,
		core.CapabilityList,
	}
}

func (c *MultiTgzConnector) SupportedDataTypes() []core.DataType {
	return []core.DataType{
		core.DataTypePhoto,
		core.DataTypeVideo,
		core.DataTypeAudio,
		core.DataTypeDocument,
		core.DataTypeUnknown,
	}
}

func (c *MultiTgzConnector) RequiresAuth() bool { return false }
func (c *MultiTgzConnector) AuthType() string   { return "none" }

func (c *MultiTgzConnector) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "paths",
			Name:        "TGZ File Paths",
			Description: "Comma-separated paths to .tar.gz/.tgz archives",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "extract_dir",
			Name:        "Extract Directory",
			Description: "Directory to extract files (temp dir if empty)",
			Type:        "path",
			Default:     "",
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

func (c *MultiTgzConnector) ValidateConfig(cfg core.ConnectorConfig) error {
	paths := extractPaths(cfg.Settings["paths"])
	if len(paths) == 0 {
		return fmt.Errorf("multi-tgz connector: paths is required")
	}

	for _, p := range paths {
		if p == "" {
			continue
		}

		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("multi-tgz connector: %s not accessible: %w", p, err)
		}
		if info.IsDir() {
			return fmt.Errorf("multi-tgz connector: %s is a directory", p)
		}

		// Validate gzip/tar format
		f, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("multi-tgz connector: open %s: %w", p, err)
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return fmt.Errorf("multi-tgz connector: %s is not a valid gzip: %w", p, err)
		}
		gz.Close()
		f.Close()
	}

	return nil
}

func (c *MultiTgzConnector) FallbackOptions() []core.FallbackOption {
	return nil
}

func (c *MultiTgzConnector) Initialize(ctx context.Context, cfg core.ConnectorConfig) error {
	if err := c.ValidateConfig(cfg); err != nil {
		return err
	}

	c.cfg = cfg

	// Parse paths (handles both array and comma-separated string)
	c.tgzPaths = extractPaths(cfg.Settings["paths"])

	// Determine extraction directory
	if extractDir, ok := cfg.Settings["extract_dir"].(string); ok && extractDir != "" {
		c.extractTo = extractDir
	} else {
		tmpDir, err := os.MkdirTemp("", "revoco-multi-tgz-*")
		if err != nil {
			return fmt.Errorf("multi-tgz connector: create temp dir: %w", err)
		}
		c.extractTo = tmpDir
	}

	// Extract all archives
	c.files = make(map[string]multiFileInfo)

	for tgzIdx, tgzPath := range c.tgzPaths {
		if err := c.extractArchive(ctx, tgzIdx, tgzPath); err != nil {
			return err
		}
	}

	c.extracted = true
	return nil
}

func (c *MultiTgzConnector) extractArchive(ctx context.Context, tgzIdx int, tgzPath string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return fmt.Errorf("multi-tgz connector: open %s: %w", tgzPath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("multi-tgz connector: gzip reader for %s: %w", tgzPath, err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("multi-tgz connector: read tar from %s: %w", tgzPath, err)
		}

		target := filepath.Join(c.extractTo, hdr.Name)

		// Guard against tar-slip
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(c.extractTo)+string(os.PathSeparator)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}

			wf, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}

			if _, err := io.Copy(wf, tr); err != nil {
				wf.Close()
				return err
			}
			wf.Close()

			// Later archives override earlier ones
			c.files[hdr.Name] = multiFileInfo{
				tgzIndex: tgzIdx,
				path:     target,
				size:     hdr.Size,
				modTime:  hdr.ModTime.Unix(),
			}
		}
	}

	return nil
}

func (c *MultiTgzConnector) List(ctx context.Context, progress core.ProgressFunc) ([]core.DataItem, error) {
	if c.files == nil {
		return nil, fmt.Errorf("multi-tgz connector: not initialized")
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

	for name, info := range c.files {
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
			Path:         info.path,
			RemoteID:     name,
			SourceConnID: c.cfg.InstanceID,
			Size:         info.size,
			Metadata: map[string]any{
				"tgz_path":  name,
				"tgz_index": info.tgzIndex,
				"tgz_file":  c.tgzPaths[info.tgzIndex],
				"mod_time":  info.modTime,
				"file_name": filepath.Base(name),
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

func (c *MultiTgzConnector) Read(ctx context.Context, item core.DataItem) (io.ReadCloser, error) {
	if item.Path != "" {
		return os.Open(item.Path)
	}
	return nil, fmt.Errorf("multi-tgz connector: file not extracted")
}

func (c *MultiTgzConnector) ReadTo(ctx context.Context, item core.DataItem, destPath string, mode core.ImportMode) error {
	if item.Path == "" {
		return fmt.Errorf("multi-tgz connector: file not extracted")
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("multi-tgz connector: create dest dir: %w", err)
	}

	return copyFile(item.Path, destPath)
}

func (c *MultiTgzConnector) Close() error {
	if c.extractTo != "" && strings.Contains(c.extractTo, "revoco-multi-tgz-") {
		os.RemoveAll(c.extractTo)
	}
	c.files = nil
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Registration
// ══════════════════════════════════════════════════════════════════════════════

// TestConnection verifies the TGZ file is accessible and valid.
func (c *TgzConnector) TestConnection(ctx context.Context, cfg core.ConnectorConfig) error {
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
		return fmt.Errorf("path is a directory, expected TGZ file: %s", path)
	}

	// Try to open and verify it's valid gzip/tar
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("invalid gzip format: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	_, err = tr.Next()
	if err != nil && err != io.EOF {
		return fmt.Errorf("invalid tar format: %w", err)
	}

	return nil
}

// TestConnection verifies all TGZ files are accessible and valid.
func (c *MultiTgzConnector) TestConnection(ctx context.Context, cfg core.ConnectorConfig) error {
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
			return fmt.Errorf("path is a directory, expected TGZ file: %s", path)
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("cannot open %s: %w", path, err)
		}

		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return fmt.Errorf("invalid gzip format in %s: %w", path, err)
		}
		gz.Close()
		f.Close()
	}

	return nil
}

func init() {
	core.RegisterConnector(func() core.Connector {
		return NewTgzConnector()
	})
	core.RegisterConnector(func() core.Connector {
		return NewMultiTgzConnector()
	})
}

// Ensure interfaces are satisfied
var (
	_ core.ConnectorReader = (*TgzConnector)(nil)
	_ core.ConnectorReader = (*MultiTgzConnector)(nil)
	_ core.ConnectorTester = (*TgzConnector)(nil)
	_ core.ConnectorTester = (*MultiTgzConnector)(nil)
)
