// Package plugins provides a dynamic plugin system for revoco.
//
// This file contains the integration layer that bridges the plugin system
// with revoco's existing connector, processor, and output registries.
package plugins

import (
	"context"
	"fmt"
	"log"

	core "github.com/fulgidus/revoco/connectors"
	svccore "github.com/fulgidus/revoco/services/core"
)

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Manager
// ══════════════════════════════════════════════════════════════════════════════

// Manager handles plugin discovery, loading, and integration with revoco.
type Manager struct {
	registry    *Registry
	loader      *Loader
	initialized bool

	// Options
	hotReload bool
	logLevel  string
}

// ManagerOption configures the plugin manager.
type ManagerOption func(*Manager)

// WithHotReload enables or disables hot-reload for Lua plugins.
func WithHotReload(enabled bool) ManagerOption {
	return func(m *Manager) {
		m.hotReload = enabled
	}
}

// WithLogLevel sets the log level for plugin operations.
func WithLogLevel(level string) ManagerOption {
	return func(m *Manager) {
		m.logLevel = level
	}
}

// WithPluginDirs sets the directories to search for plugins.
func WithPluginDirs(dirs ...string) ManagerOption {
	return func(m *Manager) {
		if m.loader != nil {
			m.loader.dirs = dirs
		}
	}
}

// NewManager creates a new plugin manager.
func NewManager(opts ...ManagerOption) *Manager {
	registry := NewRegistry()

	m := &Manager{
		registry:  registry,
		loader:    NewLoader(registry, DefaultPluginDirs()...),
		hotReload: true,
		logLevel:  "info",
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// ══════════════════════════════════════════════════════════════════════════════
// Initialization
// ══════════════════════════════════════════════════════════════════════════════

// Initialize discovers and loads all plugins.
// This method is idempotent - calling it multiple times is safe.
func (m *Manager) Initialize(ctx context.Context) error {
	if m.initialized {
		return nil // Already initialized, no-op
	}

	// Ensure plugin directories exist
	if err := EnsurePluginDirs(); err != nil {
		log.Printf("[plugins] Warning: could not create plugin directories: %v", err)
	}

	// Extract default plugins on first run (or when version changes)
	if err := ExtractDefaultPlugins(""); err != nil {
		log.Printf("[plugins] Warning: could not extract default plugins: %v", err)
		// Continue - this is not fatal
	}

	// Load all discovered plugins
	if err := m.loader.LoadAll(ctx); err != nil {
		log.Printf("[plugins] Warning: some plugins failed to load: %v", err)
		// Continue - partial load is acceptable
	}

	// Log stats
	stats := m.registry.Stats()
	log.Printf("[plugins] Loaded %d plugins (%d connectors, %d processors, %d outputs)",
		stats.Total, stats.Connectors, stats.Processors, stats.Outputs)

	// Integrate with existing registries
	if err := m.integrateWithRegistries(); err != nil {
		return fmt.Errorf("failed to integrate with registries: %w", err)
	}

	m.initialized = true
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Registry Integration
// ══════════════════════════════════════════════════════════════════════════════

// integrateWithRegistries registers plugins with revoco's built-in registries.
func (m *Manager) integrateWithRegistries() error {
	// Register connectors
	for _, cp := range m.registry.Connectors() {
		if err := m.registerConnectorWithCore(cp); err != nil {
			log.Printf("[plugins] Warning: failed to register connector %s: %v", cp.Info().ID, err)
		}
	}

	// Register outputs
	for _, op := range m.registry.Outputs() {
		if err := m.registerOutputWithCore(op); err != nil {
			log.Printf("[plugins] Warning: failed to register output %s: %v", op.Info().ID, err)
		}
	}

	// Note: Processors are handled differently - they're selected at runtime
	// based on selectors, not pre-registered.

	return nil
}

// registerConnectorWithCore registers a plugin connector with the core registry.
func (m *Manager) registerConnectorWithCore(cp ConnectorPlugin) error {
	// Create a factory that returns the plugin's connector
	factory := func() core.Connector {
		return cp.AsConnector()
	}

	// Register with the core registry
	return core.RegisterConnector(factory)
}

// registerOutputWithCore registers a plugin output with the core registry.
func (m *Manager) registerOutputWithCore(op OutputPlugin) error {
	// Register with the service core registry
	return svccore.RegisterOutput(op.AsOutput())
}

// ══════════════════════════════════════════════════════════════════════════════
// Public API
// ══════════════════════════════════════════════════════════════════════════════

// Registry returns the plugin registry.
func (m *Manager) Registry() *Registry {
	return m.registry
}

// Shutdown stops all plugins and releases resources.
func (m *Manager) Shutdown() error {
	if err := m.registry.UnloadAll(); err != nil {
		return err
	}

	m.initialized = false
	return nil
}

// GetConnector returns a connector by ID (checks both plugins and built-in).
func (m *Manager) GetConnector(id string) (core.Connector, bool) {
	// First check plugin registry
	if cp, ok := m.registry.GetConnector(id); ok {
		return cp.AsConnector(), true
	}

	// Fall back to core registry
	conn, err := core.CreateConnector(id)
	if err != nil {
		return nil, false
	}
	return conn, true
}

// GetProcessor returns a processor plugin by ID.
func (m *Manager) GetProcessor(id string) (Processor, bool) {
	if pp, ok := m.registry.GetProcessor(id); ok {
		return pp.AsProcessor(), true
	}
	return nil, false
}

// GetOutput returns an output by ID (checks both plugins and built-in).
func (m *Manager) GetOutput(id string) (svccore.Output, bool) {
	// First check plugin registry
	if op, ok := m.registry.GetOutput(id); ok {
		return op.AsOutput(), true
	}

	// Fall back to core registry
	return svccore.GetOutput(id)
}

// GetApplicableProcessors returns all processors that can handle the given item.
func (m *Manager) GetApplicableProcessors(item *core.DataItem) []Processor {
	var processors []Processor

	for _, pp := range m.registry.Processors() {
		processor := pp.AsProcessor()
		if processor.CanProcess(item) {
			processors = append(processors, processor)
		}
	}

	return processors
}

// ListPlugins returns information about all loaded plugins.
func (m *Manager) ListPlugins() []PluginInfo {
	var infos []PluginInfo
	for _, p := range m.registry.All() {
		infos = append(infos, p.Info())
	}
	return infos
}

// ReloadPlugin reloads a specific plugin by ID.
func (m *Manager) ReloadPlugin(ctx context.Context, id string) error {
	return m.registry.Reload(ctx, id)
}

// CheckDependencies checks binary dependencies for all plugins.
func (m *Manager) CheckDependencies() []*DependencyStatus {
	var statuses []*DependencyStatus
	checker := NewDependencyChecker()

	for _, p := range m.registry.All() {
		info := p.Info()
		for _, dep := range info.Dependencies {
			status := checker.Check(dep)
			statuses = append(statuses, status)
		}
	}

	return statuses
}

// ══════════════════════════════════════════════════════════════════════════════
// Global Manager
// ══════════════════════════════════════════════════════════════════════════════

var globalManager *Manager

// InitializePlugins initializes the global plugin manager.
func InitializePlugins(ctx context.Context, opts ...ManagerOption) error {
	globalManager = NewManager(opts...)
	return globalManager.Initialize(ctx)
}

// ShutdownPlugins shuts down the global plugin manager.
func ShutdownPlugins() error {
	if globalManager != nil {
		return globalManager.Shutdown()
	}
	return nil
}

// PluginManager returns the global plugin manager.
func PluginManager() *Manager {
	return globalManager
}
