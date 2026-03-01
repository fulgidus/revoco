package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	core "github.com/fulgidus/revoco/connectors"
)

// PluginFactory creates a Plugin from discovered metadata.
type PluginFactory func(dp *DiscoveredPlugin) (Plugin, error)

// Global factories for different plugin tiers.
var (
	luaPluginFactory      PluginFactory
	externalPluginFactory PluginFactory
)

// RegisterLuaPluginFactory registers the Lua plugin factory.
// This is called by the lua package during initialization.
func RegisterLuaPluginFactory(factory PluginFactory) {
	luaPluginFactory = factory
}

// RegisterExternalPluginFactory registers the external plugin factory.
// This is called by the external package during initialization.
func RegisterExternalPluginFactory(factory PluginFactory) {
	externalPluginFactory = factory
}

// Loader discovers and loads plugins from directories.
type Loader struct {
	registry *Registry
	checker  *DependencyChecker
	dirs     []string
}

// NewLoader creates a new plugin loader.
func NewLoader(registry *Registry, dirs ...string) *Loader {
	return &Loader{
		registry: registry,
		checker:  NewDependencyChecker(),
		dirs:     dirs,
	}
}

// DefaultPluginDirs returns the default plugin directories.
func DefaultPluginDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	return []string{
		filepath.Join(home, ".config", "revoco", "plugins"),
	}
}

// DiscoverAll scans all plugin directories and returns discovered plugins.
func (l *Loader) DiscoverAll() ([]*DiscoveredPlugin, error) {
	var all []*DiscoveredPlugin

	for _, dir := range l.dirs {
		plugins, err := l.Discover(dir)
		if err != nil {
			// Log warning but continue
			continue
		}
		all = append(all, plugins...)
	}

	return all, nil
}

// Discover scans a directory for plugins.
func (l *Loader) Discover(dir string) ([]*DiscoveredPlugin, error) {
	var plugins []*DiscoveredPlugin

	// Scan subdirectories for each type
	for _, subdir := range []string{"connectors", "processors", "outputs"} {
		typePath := filepath.Join(dir, subdir)
		typePlugins, err := l.discoverInDir(typePath, pluginTypeFromDir(subdir))
		if err != nil {
			continue // Directory may not exist
		}
		plugins = append(plugins, typePlugins...)
	}

	return plugins, nil
}

// discoverInDir scans a specific type directory for plugins.
func (l *Loader) discoverInDir(dir string, ptype PluginType) ([]*DiscoveredPlugin, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var plugins []*DiscoveredPlugin

	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)

		var dp *DiscoveredPlugin

		if entry.IsDir() {
			// Check for external plugin (manifest.json)
			dp, err = l.discoverExternal(path, ptype)
		} else if strings.HasSuffix(name, ".lua") {
			// Lua plugin
			dp, err = l.discoverLua(path, ptype)
		}

		if err != nil {
			// Record error but continue
			dp = &DiscoveredPlugin{
				Path:  path,
				Type:  ptype,
				Error: err.Error(),
			}
		}

		if dp != nil {
			plugins = append(plugins, dp)
		}
	}

	return plugins, nil
}

// discoverLua parses a Lua plugin file for metadata.
func (l *Loader) discoverLua(path string, ptype PluginType) (*DiscoveredPlugin, error) {
	// Read the file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Extract Plugin table using simple parsing
	// Full Lua parsing will be done in the Lua runtime
	info, err := extractLuaPluginInfo(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse plugin info: %w", err)
	}

	info.Type = ptype
	info.Tier = PluginTierLua
	info.Source = PluginSourceUser
	info.Path = path
	info.State = PluginStateUnloaded

	return &DiscoveredPlugin{
		Path:   path,
		Type:   ptype,
		Tier:   PluginTierLua,
		Info:   info,
		Source: PluginSourceUser,
	}, nil
}

// discoverExternal parses an external plugin directory.
func (l *Loader) discoverExternal(dir string, ptype PluginType) (*DiscoveredPlugin, error) {
	manifestPath := filepath.Join(dir, "manifest.json")

	// Check if manifest exists
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return nil, nil // Not a plugin directory
	}

	// Read manifest
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest ExternalManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate manifest
	if err := validateManifest(&manifest, dir); err != nil {
		return nil, err
	}

	// Convert to PluginInfo
	info := manifestToPluginInfo(&manifest, dir)
	info.State = PluginStateUnloaded

	// Check runtime availability
	rtStatus := CheckRuntime(manifest.Spec.Runtime)
	if !rtStatus.Found {
		info.State = PluginStateMissingDeps
		info.StateError = fmt.Sprintf("runtime %s not found", manifest.Spec.Runtime.Command)
	}

	return &DiscoveredPlugin{
		Path:     dir,
		Type:     ptype,
		Tier:     PluginTierExternal,
		Info:     info,
		Manifest: &manifest,
		Source:   PluginSourceUser,
	}, nil
}

// LoadAll loads all discovered plugins into the registry.
func (l *Loader) LoadAll(ctx context.Context) error {
	discovered, err := l.DiscoverAll()
	if err != nil {
		return err
	}

	var loadErrors []error

	for _, dp := range discovered {
		if dp.Error != "" {
			loadErrors = append(loadErrors, fmt.Errorf("%s: %s", dp.Path, dp.Error))
			continue
		}

		if dp.Info.State == PluginStateMissingDeps {
			// Skip but record
			continue
		}

		plugin, err := l.createPlugin(dp)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("%s: %w", dp.Path, err))
			continue
		}

		if err := l.registry.Register(plugin); err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("%s: %w", dp.Path, err))
			continue
		}

		// Load the plugin
		if err := plugin.Load(ctx); err != nil {
			// Update state but keep registered
			loadErrors = append(loadErrors, fmt.Errorf("%s: %w", dp.Path, err))
		}
	}

	if len(loadErrors) > 0 {
		// Return first error, log others
		return loadErrors[0]
	}

	return nil
}

// createPlugin creates a Plugin instance from discovered metadata.
func (l *Loader) createPlugin(dp *DiscoveredPlugin) (Plugin, error) {
	switch dp.Tier {
	case PluginTierLua:
		return l.createLuaPlugin(dp)
	case PluginTierExternal:
		return l.createExternalPlugin(dp)
	default:
		return nil, fmt.Errorf("unknown plugin tier: %s", dp.Tier)
	}
}

// createLuaPlugin creates a Lua plugin instance.
func (l *Loader) createLuaPlugin(dp *DiscoveredPlugin) (Plugin, error) {
	if luaPluginFactory != nil {
		return luaPluginFactory(dp)
	}
	// Fall back to stub if no factory registered
	return &luaPluginStub{
		info:    dp.Info,
		path:    dp.Path,
		checker: l.checker,
	}, nil
}

// createExternalPlugin creates an external plugin instance.
func (l *Loader) createExternalPlugin(dp *DiscoveredPlugin) (Plugin, error) {
	if externalPluginFactory != nil {
		return externalPluginFactory(dp)
	}
	// Fall back to stub if no factory registered
	return &externalPluginStub{
		info:     dp.Info,
		path:     dp.Path,
		manifest: dp.Manifest,
		checker:  l.checker,
	}, nil
}

// Watch starts watching plugin directories for changes (hot-reload).
func (l *Loader) Watch(ctx context.Context, onChange func(path string, op WatchOp)) error {
	// TODO: Implement file watching using fsnotify
	// For now, return nil (no-op)
	return nil
}

// WatchOp represents a file system change operation.
type WatchOp int

const (
	WatchOpCreate WatchOp = iota
	WatchOpModify
	WatchOpDelete
)

// ══════════════════════════════════════════════════════════════════════════════
// Helper Types
// ══════════════════════════════════════════════════════════════════════════════

// DiscoveredPlugin represents a plugin found during discovery.
type DiscoveredPlugin struct {
	Path     string
	Type     PluginType
	Tier     PluginTier
	Info     *PluginInfo
	Manifest *ExternalManifest
	Source   PluginSource
	Error    string
}

// ══════════════════════════════════════════════════════════════════════════════
// Stub Implementations (to be replaced with real implementations)
// ══════════════════════════════════════════════════════════════════════════════

// luaPluginStub is a placeholder for Lua plugin implementation.
type luaPluginStub struct {
	info    *PluginInfo
	path    string
	checker *DependencyChecker
	loaded  bool
}

func (p *luaPluginStub) Info() PluginInfo {
	if p.info == nil {
		return PluginInfo{}
	}
	return *p.info
}

func (p *luaPluginStub) Load(ctx context.Context) error {
	// Check dependencies
	if len(p.info.Dependencies) > 0 {
		missing := p.checker.MissingDependencies(p.info.Dependencies)
		if len(missing) > 0 {
			p.info.State = PluginStateMissingDeps
			p.info.StateError = fmt.Sprintf("missing: %s", missing[0].Binary)
			return &DependencyError{
				Binary:  missing[0].Binary,
				Message: "binary not found",
				Install: missing[0].Install,
			}
		}
	}

	p.loaded = true
	p.info.State = PluginStateReady
	return nil
}

func (p *luaPluginStub) Unload() error {
	p.loaded = false
	p.info.State = PluginStateUnloaded
	return nil
}

func (p *luaPluginStub) Reload(ctx context.Context) error {
	if err := p.Unload(); err != nil {
		return err
	}
	return p.Load(ctx)
}

// externalPluginStub is a placeholder for external plugin implementation.
type externalPluginStub struct {
	info     *PluginInfo
	path     string
	manifest *ExternalManifest
	checker  *DependencyChecker
	loaded   bool
}

func (p *externalPluginStub) Info() PluginInfo {
	if p.info == nil {
		return PluginInfo{}
	}
	return *p.info
}

func (p *externalPluginStub) Load(ctx context.Context) error {
	// Check runtime
	if p.manifest != nil {
		rtStatus := CheckRuntime(p.manifest.Spec.Runtime)
		if !rtStatus.Found || !rtStatus.MeetsMin {
			p.info.State = PluginStateMissingDeps
			p.info.StateError = fmt.Sprintf("runtime %s not available", p.manifest.Spec.Runtime.Command)
			return ErrRuntimeNotFound
		}
	}

	p.loaded = true
	p.info.State = PluginStateReady
	return nil
}

func (p *externalPluginStub) Unload() error {
	p.loaded = false
	p.info.State = PluginStateUnloaded
	return nil
}

func (p *externalPluginStub) Reload(ctx context.Context) error {
	if err := p.Unload(); err != nil {
		return err
	}
	return p.Load(ctx)
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Functions
// ══════════════════════════════════════════════════════════════════════════════

// pluginTypeFromDir returns the plugin type for a subdirectory name.
func pluginTypeFromDir(dir string) PluginType {
	switch dir {
	case "connectors":
		return PluginTypeConnector
	case "processors":
		return PluginTypeProcessor
	case "outputs":
		return PluginTypeOutput
	default:
		return ""
	}
}

// extractLuaPluginInfo extracts plugin info from Lua source.
// This is a simple parser - full parsing done by Lua runtime.
func extractLuaPluginInfo(source string) (*PluginInfo, error) {
	info := &PluginInfo{}

	// Look for Plugin = { ... } table
	// This is simplified - real parsing will use Lua runtime

	// Extract id
	if id := extractLuaField(source, "id"); id != "" {
		info.ID = id
	}

	// Extract name
	if name := extractLuaField(source, "name"); name != "" {
		info.Name = name
	}

	// Extract description
	if desc := extractLuaField(source, "description"); desc != "" {
		info.Description = desc
	}

	// Extract version
	if version := extractLuaField(source, "version"); version != "" {
		info.Version = version
	}

	if info.ID == "" {
		return nil, fmt.Errorf("plugin missing required 'id' field")
	}

	return info, nil
}

// extractLuaField extracts a string field from Lua source.
// This is a simplified extractor - real parsing done by Lua runtime.
func extractLuaField(source, field string) string {
	// Look for patterns like: id = "value" or id = 'value'
	patterns := []string{
		field + `\s*=\s*"([^"]*)"`,
		field + `\s*=\s*'([^']*)'`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(source)
		if len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}

// validateManifest validates an external plugin manifest.
func validateManifest(m *ExternalManifest, dir string) error {
	if m.APIVersion == "" {
		return &ManifestError{Path: dir, Field: "apiVersion", Reason: "required"}
	}

	if m.Kind != "ExternalPlugin" {
		return &ManifestError{Path: dir, Field: "kind", Reason: "must be 'ExternalPlugin'"}
	}

	if m.Metadata.ID == "" {
		return &ManifestError{Path: dir, Field: "metadata.id", Reason: "required"}
	}

	if m.Spec.Runtime.Command == "" {
		return &ManifestError{Path: dir, Field: "spec.runtime.command", Reason: "required"}
	}

	if m.Spec.Entrypoint == "" {
		return &ManifestError{Path: dir, Field: "spec.entrypoint", Reason: "required"}
	}

	// Check entrypoint exists
	entryPath := filepath.Join(dir, m.Spec.Entrypoint)
	if _, err := os.Stat(entryPath); os.IsNotExist(err) {
		return &ManifestError{Path: dir, Field: "spec.entrypoint", Reason: fmt.Sprintf("file not found: %s", m.Spec.Entrypoint)}
	}

	return nil
}

// manifestToPluginInfo converts an external manifest to PluginInfo.
func manifestToPluginInfo(m *ExternalManifest, dir string) *PluginInfo {
	info := &PluginInfo{
		ID:              m.Metadata.ID,
		Name:            m.Metadata.Name,
		Description:     m.Metadata.Description,
		Version:         m.Metadata.Version,
		Author:          m.Metadata.Author,
		Type:            m.Spec.Type,
		Tier:            PluginTierExternal,
		Source:          PluginSourceUser,
		Path:            dir,
		RequiresAuth:    m.Spec.RequiresAuth,
		AuthType:        m.Spec.AuthType,
		ConfigSchema:    m.Spec.ConfigSchema,
		Dependencies:    m.Spec.Dependencies,
		DefaultSelector: m.Spec.DefaultSelector,
	}

	// Convert capabilities
	for _, cap := range m.Spec.Capabilities {
		info.Capabilities = append(info.Capabilities, core.ConnectorCapability(cap))
	}

	// Convert data types
	for _, dt := range m.Spec.DataTypes {
		info.DataTypes = append(info.DataTypes, core.DataType(dt))
	}

	return info
}

// EnsurePluginDirs creates the plugin directory structure if it doesn't exist.
func EnsurePluginDirs() error {
	dirs := DefaultPluginDirs()
	if len(dirs) == 0 {
		return fmt.Errorf("could not determine plugin directory")
	}

	base := dirs[0]
	subdirs := []string{"connectors", "processors", "outputs"}

	for _, sub := range subdirs {
		path := filepath.Join(base, sub)
		if err := os.MkdirAll(path, fs.ModePerm); err != nil {
			return fmt.Errorf("failed to create %s: %w", path, err)
		}
	}

	return nil
}
