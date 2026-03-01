// Package plugins provides a dynamic plugin system for revoco.
//
// This file handles plugin installation, removal, and updates from the
// plugin registry hosted on the GitHub plugins branch.
package plugins

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// Registry Types
// ══════════════════════════════════════════════════════════════════════════════

// RegistryIndex represents the plugin registry index.json file.
type RegistryIndex struct {
	Version     string                   `json:"version"`
	LastUpdated time.Time                `json:"last_updated"`
	Plugins     map[string]RegistryEntry `json:"plugins"`
}

// RegistryEntry represents a plugin entry in the registry.
type RegistryEntry struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	Version     string            `json:"version"`
	Type        PluginType        `json:"type"`
	Tier        PluginTier        `json:"tier"`
	Tags        []string          `json:"tags,omitempty"`
	Homepage    string            `json:"homepage,omitempty"`
	Repository  string            `json:"repository,omitempty"`
	License     string            `json:"license,omitempty"`
	Downloads   int               `json:"downloads,omitempty"`
	Path        string            `json:"path"` // Path within the plugins branch
	Checksum    string            `json:"checksum,omitempty"`
	Platforms   []string          `json:"platforms,omitempty"`   // e.g., ["linux", "darwin", "windows"]
	MinVersion  string            `json:"min_version,omitempty"` // Minimum revoco version
	Deprecated  bool              `json:"deprecated,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// InstalledPlugin represents a locally installed plugin.
type InstalledPlugin struct {
	Entry       RegistryEntry `json:"entry"`
	InstalledAt time.Time     `json:"installed_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Source      string        `json:"source"` // "registry", "local", "url"
	SourcePath  string        `json:"source_path"`
	LocalPath   string        `json:"local_path"`
}

// InstalledPluginsDB tracks installed plugins.
type InstalledPluginsDB struct {
	Version   string                     `json:"version"`
	Plugins   map[string]InstalledPlugin `json:"plugins"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

// ══════════════════════════════════════════════════════════════════════════════
// Installer
// ══════════════════════════════════════════════════════════════════════════════

// Installer handles plugin installation and management.
type Installer struct {
	registryURL    string
	cacheDir       string
	pluginDir      string
	httpClient     *http.Client
	cachedIndex    *RegistryIndex
	indexCacheTime time.Time
}

// DefaultRegistryURL is the default URL for the plugin registry.
const DefaultRegistryURL = "https://raw.githubusercontent.com/fulgidus/revoco/plugins"

// IndexCacheDuration is how long to cache the registry index.
const IndexCacheDuration = 15 * time.Minute

// InstallerOption configures the installer.
type InstallerOption func(*Installer)

// WithRegistryURL sets a custom registry URL.
func WithRegistryURL(url string) InstallerOption {
	return func(i *Installer) {
		i.registryURL = url
	}
}

// WithCacheDir sets a custom cache directory.
func WithCacheDir(dir string) InstallerOption {
	return func(i *Installer) {
		i.cacheDir = dir
	}
}

// WithPluginDir sets a custom plugin directory.
func WithPluginDir(dir string) InstallerOption {
	return func(i *Installer) {
		i.pluginDir = dir
	}
}

// NewInstaller creates a new plugin installer.
func NewInstaller(opts ...InstallerOption) *Installer {
	home, _ := os.UserHomeDir()

	i := &Installer{
		registryURL: DefaultRegistryURL,
		cacheDir:    filepath.Join(home, ".cache", "revoco", "plugins"),
		pluginDir:   filepath.Join(home, ".config", "revoco", "plugins"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(i)
	}

	return i
}

// ══════════════════════════════════════════════════════════════════════════════
// Registry Operations
// ══════════════════════════════════════════════════════════════════════════════

// FetchIndex fetches the plugin registry index.
func (i *Installer) FetchIndex(ctx context.Context) (*RegistryIndex, error) {
	// Check cache
	if i.cachedIndex != nil && time.Since(i.indexCacheTime) < IndexCacheDuration {
		return i.cachedIndex, nil
	}

	url := i.registryURL + "/index.json"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var index RegistryIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("failed to decode index: %w", err)
	}

	// Update cache
	i.cachedIndex = &index
	i.indexCacheTime = time.Now()

	return &index, nil
}

// Search searches the plugin registry.
func (i *Installer) Search(ctx context.Context, query string) ([]RegistryEntry, error) {
	index, err := i.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []RegistryEntry

	for _, entry := range index.Plugins {
		// Skip deprecated plugins unless explicitly searched
		if entry.Deprecated {
			continue
		}

		// Check platform compatibility
		if !i.isPlatformCompatible(entry) {
			continue
		}

		// Match against ID, name, description, and tags
		if i.matchesQuery(entry, query) {
			results = append(results, entry)
		}
	}

	return results, nil
}

// matchesQuery checks if an entry matches a search query.
func (i *Installer) matchesQuery(entry RegistryEntry, query string) bool {
	if query == "" {
		return true
	}

	// Match ID
	if strings.Contains(strings.ToLower(entry.ID), query) {
		return true
	}

	// Match name
	if strings.Contains(strings.ToLower(entry.Name), query) {
		return true
	}

	// Match description
	if strings.Contains(strings.ToLower(entry.Description), query) {
		return true
	}

	// Match tags
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}

	// Match type
	if strings.Contains(strings.ToLower(string(entry.Type)), query) {
		return true
	}

	return false
}

// isPlatformCompatible checks if a plugin is compatible with the current platform.
func (i *Installer) isPlatformCompatible(entry RegistryEntry) bool {
	if len(entry.Platforms) == 0 {
		return true // No restrictions
	}

	for _, platform := range entry.Platforms {
		if platform == runtime.GOOS || platform == "all" {
			return true
		}
	}

	return false
}

// GetEntry returns a specific plugin entry from the registry.
func (i *Installer) GetEntry(ctx context.Context, id string) (*RegistryEntry, error) {
	index, err := i.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}

	entry, ok := index.Plugins[id]
	if !ok {
		return nil, fmt.Errorf("plugin not found: %s", id)
	}

	return &entry, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Installation Operations
// ══════════════════════════════════════════════════════════════════════════════

// Install installs a plugin by ID from the registry.
func (i *Installer) Install(ctx context.Context, id string) error {
	entry, err := i.GetEntry(ctx, id)
	if err != nil {
		return err
	}

	return i.installFromRegistry(ctx, *entry)
}

// InstallFromURL installs a plugin from a URL.
func (i *Installer) InstallFromURL(ctx context.Context, url string) error {
	// Download to temp file
	tmpFile, err := i.downloadToTemp(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer os.Remove(tmpFile)

	// Detect type and install
	return i.installFromFile(ctx, tmpFile, url)
}

// InstallFromPath installs a plugin from a local path.
func (i *Installer) InstallFromPath(ctx context.Context, path string) error {
	// Check if it's a file or directory
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	if info.IsDir() {
		return i.installFromDir(ctx, path)
	}

	return i.installFromFile(ctx, path, path)
}

// installFromRegistry installs a plugin from the registry.
func (i *Installer) installFromRegistry(ctx context.Context, entry RegistryEntry) error {
	// Construct download URL
	url := i.registryURL + "/" + entry.Path

	// Download to temp file
	tmpFile, err := i.downloadToTemp(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to download plugin: %w", err)
	}
	defer os.Remove(tmpFile)

	// Determine destination directory based on plugin type
	destDir := filepath.Join(i.pluginDir, string(entry.Type)+"s")

	// Install based on tier
	switch entry.Tier {
	case PluginTierLua:
		// Single Lua file
		destPath := filepath.Join(destDir, entry.ID+".lua")
		if err := copyFile(tmpFile, destPath); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

	case PluginTierExternal:
		// Extract tarball/zip to directory
		destPath := filepath.Join(destDir, entry.ID)
		if err := extractArchive(tmpFile, destPath); err != nil {
			return fmt.Errorf("failed to extract: %w", err)
		}

	default:
		return fmt.Errorf("unknown plugin tier: %s", entry.Tier)
	}

	// Record installation
	if err := i.recordInstallation(entry, "registry", url); err != nil {
		// Non-fatal
		fmt.Printf("Warning: failed to record installation: %v\n", err)
	}

	return nil
}

// installFromFile installs a plugin from a downloaded file.
func (i *Installer) installFromFile(ctx context.Context, path string, source string) error {
	// Detect file type
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".lua":
		// Lua plugin - need to parse to determine type
		info, err := extractLuaPluginInfo(readFileContents(path))
		if err != nil {
			return fmt.Errorf("failed to parse Lua plugin: %w", err)
		}

		destDir := filepath.Join(i.pluginDir, string(info.Type)+"s")
		destPath := filepath.Join(destDir, info.ID+".lua")

		if err := os.MkdirAll(destDir, 0755); err != nil {
			return err
		}

		if err := copyFile(path, destPath); err != nil {
			return fmt.Errorf("failed to install: %w", err)
		}

		// Record installation
		entry := RegistryEntry{
			ID:   info.ID,
			Name: info.Name,
			Type: info.Type,
			Tier: PluginTierLua,
		}
		return i.recordInstallation(entry, "url", source)

	case ".tar", ".gz", ".tgz":
		// Archive - extract and look for manifest
		tmpDir, err := os.MkdirTemp("", "revoco-plugin-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)

		if err := extractArchive(path, tmpDir); err != nil {
			return fmt.Errorf("failed to extract: %w", err)
		}

		return i.installFromDir(ctx, tmpDir)

	default:
		return fmt.Errorf("unsupported file type: %s", ext)
	}
}

// installFromDir installs a plugin from a directory.
func (i *Installer) installFromDir(ctx context.Context, dir string) error {
	// Look for manifest.json (external plugin)
	manifestPath := filepath.Join(dir, "manifest.json")
	if _, err := os.Stat(manifestPath); err == nil {
		content, err := os.ReadFile(manifestPath)
		if err != nil {
			return fmt.Errorf("failed to read manifest: %w", err)
		}

		var manifest ExternalManifest
		if err := json.Unmarshal(content, &manifest); err != nil {
			return fmt.Errorf("failed to parse manifest: %w", err)
		}

		// Install to appropriate directory
		destDir := filepath.Join(i.pluginDir, string(manifest.Spec.Type)+"s", manifest.Metadata.ID)
		if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
			return err
		}

		// Copy directory
		if err := copyDir(dir, destDir); err != nil {
			return fmt.Errorf("failed to copy plugin: %w", err)
		}

		// Record installation
		entry := RegistryEntry{
			ID:   manifest.Metadata.ID,
			Name: manifest.Metadata.Name,
			Type: manifest.Spec.Type,
			Tier: PluginTierExternal,
		}
		return i.recordInstallation(entry, "local", dir)
	}

	// Look for plugin.lua (Lua plugin directory)
	luaPath := filepath.Join(dir, "plugin.lua")
	if _, err := os.Stat(luaPath); err == nil {
		return i.installFromFile(ctx, luaPath, dir)
	}

	return fmt.Errorf("no valid plugin found in directory")
}

// downloadToTemp downloads a URL to a temporary file.
func (i *Installer) downloadToTemp(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "revoco-download-*")
	if err != nil {
		return "", err
	}

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()

	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Removal Operations
// ══════════════════════════════════════════════════════════════════════════════

// Remove removes an installed plugin.
func (i *Installer) Remove(ctx context.Context, id string) error {
	// Find the plugin
	installed, err := i.GetInstalled(id)
	if err != nil {
		return fmt.Errorf("plugin not installed: %s", id)
	}

	// Remove files
	if err := os.RemoveAll(installed.LocalPath); err != nil {
		return fmt.Errorf("failed to remove plugin files: %w", err)
	}

	// Update database
	return i.removeFromDB(id)
}

// ══════════════════════════════════════════════════════════════════════════════
// Update Operations
// ══════════════════════════════════════════════════════════════════════════════

// CheckUpdates checks for available plugin updates.
func (i *Installer) CheckUpdates(ctx context.Context) ([]PluginUpdate, error) {
	db, err := i.loadDB()
	if err != nil {
		return nil, err
	}

	index, err := i.FetchIndex(ctx)
	if err != nil {
		return nil, err
	}

	var updates []PluginUpdate

	for id, installed := range db.Plugins {
		if installed.Source != "registry" {
			continue // Can only update registry plugins
		}

		if entry, ok := index.Plugins[id]; ok {
			if entry.Version != installed.Entry.Version {
				updates = append(updates, PluginUpdate{
					ID:             id,
					CurrentVersion: installed.Entry.Version,
					LatestVersion:  entry.Version,
					Entry:          entry,
				})
			}
		}
	}

	return updates, nil
}

// PluginUpdate represents an available plugin update.
type PluginUpdate struct {
	ID             string
	CurrentVersion string
	LatestVersion  string
	Entry          RegistryEntry
}

// Update updates a specific plugin to the latest version.
func (i *Installer) Update(ctx context.Context, id string) error {
	// Get current installation
	installed, err := i.GetInstalled(id)
	if err != nil {
		return fmt.Errorf("plugin not installed: %s", id)
	}

	if installed.Source != "registry" {
		return fmt.Errorf("can only update plugins installed from registry")
	}

	// Get latest version
	entry, err := i.GetEntry(ctx, id)
	if err != nil {
		return err
	}

	// Remove old version
	if err := os.RemoveAll(installed.LocalPath); err != nil {
		return fmt.Errorf("failed to remove old version: %w", err)
	}

	// Install new version
	return i.installFromRegistry(ctx, *entry)
}

// UpdateAll updates all plugins from the registry.
func (i *Installer) UpdateAll(ctx context.Context) ([]string, error) {
	updates, err := i.CheckUpdates(ctx)
	if err != nil {
		return nil, err
	}

	var updated []string

	for _, update := range updates {
		if err := i.Update(ctx, update.ID); err != nil {
			fmt.Printf("Warning: failed to update %s: %v\n", update.ID, err)
			continue
		}
		updated = append(updated, update.ID)
	}

	return updated, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Database Operations
// ══════════════════════════════════════════════════════════════════════════════

// dbPath returns the path to the installed plugins database.
func (i *Installer) dbPath() string {
	return filepath.Join(i.pluginDir, ".installed.json")
}

// loadDB loads the installed plugins database.
func (i *Installer) loadDB() (*InstalledPluginsDB, error) {
	path := i.dbPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &InstalledPluginsDB{
			Version: "1",
			Plugins: make(map[string]InstalledPlugin),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	var db InstalledPluginsDB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, err
	}

	return &db, nil
}

// saveDB saves the installed plugins database.
func (i *Installer) saveDB(db *InstalledPluginsDB) error {
	db.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(i.dbPath(), data, 0644)
}

// recordInstallation records a plugin installation.
func (i *Installer) recordInstallation(entry RegistryEntry, source, sourcePath string) error {
	db, err := i.loadDB()
	if err != nil {
		return err
	}

	// Determine local path
	var localPath string
	switch entry.Tier {
	case PluginTierLua:
		localPath = filepath.Join(i.pluginDir, string(entry.Type)+"s", entry.ID+".lua")
	case PluginTierExternal:
		localPath = filepath.Join(i.pluginDir, string(entry.Type)+"s", entry.ID)
	}

	db.Plugins[entry.ID] = InstalledPlugin{
		Entry:       entry,
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Source:      source,
		SourcePath:  sourcePath,
		LocalPath:   localPath,
	}

	return i.saveDB(db)
}

// removeFromDB removes a plugin from the database.
func (i *Installer) removeFromDB(id string) error {
	db, err := i.loadDB()
	if err != nil {
		return err
	}

	delete(db.Plugins, id)

	return i.saveDB(db)
}

// GetInstalled returns information about an installed plugin.
func (i *Installer) GetInstalled(id string) (*InstalledPlugin, error) {
	db, err := i.loadDB()
	if err != nil {
		return nil, err
	}

	plugin, ok := db.Plugins[id]
	if !ok {
		return nil, fmt.Errorf("plugin not installed: %s", id)
	}

	return &plugin, nil
}

// ListInstalled returns all installed plugins.
func (i *Installer) ListInstalled() ([]InstalledPlugin, error) {
	db, err := i.loadDB()
	if err != nil {
		return nil, err
	}

	var plugins []InstalledPlugin
	for _, p := range db.Plugins {
		plugins = append(plugins, p)
	}

	return plugins, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Functions
// ══════════════════════════════════════════════════════════════════════════════

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// copyDir copies a directory recursively.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

// extractArchive extracts a tar.gz archive.
func extractArchive(src, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	// Check if gzipped
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		// Not gzipped, reset and try as plain tar
		file.Seek(0, 0)
		return extractTar(file, dst)
	}
	defer gzReader.Close()

	return extractTar(gzReader, dst)
}

// extractTar extracts a tar archive.
func extractTar(r io.Reader, dst string) error {
	tarReader := tar.NewReader(r)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dst, header.Name)

		// Check for path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}

			outFile, err := os.Create(target)
			if err != nil {
				return err
			}

			_, err = io.Copy(outFile, tarReader)
			outFile.Close()

			if err != nil {
				return err
			}

			// Preserve executable bit
			if header.Mode&0111 != 0 {
				os.Chmod(target, 0755)
			}
		}
	}

	return nil
}

// readFileContents reads a file's contents as a string.
func readFileContents(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
