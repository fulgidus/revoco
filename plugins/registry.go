package plugins

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages all loaded plugins.
type Registry struct {
	mu sync.RWMutex

	// Plugins by ID
	connectors map[string]ConnectorPlugin
	processors map[string]ProcessorPlugin
	outputs    map[string]OutputPlugin

	// All plugins (for iteration)
	all map[string]Plugin

	// Plugin load order (for deterministic iteration)
	order []string

	// Event callbacks
	onLoad   []func(Plugin)
	onUnload []func(Plugin)
	onReload []func(Plugin)
}

// NewRegistry creates a new plugin registry.
func NewRegistry() *Registry {
	return &Registry{
		connectors: make(map[string]ConnectorPlugin),
		processors: make(map[string]ProcessorPlugin),
		outputs:    make(map[string]OutputPlugin),
		all:        make(map[string]Plugin),
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Registration
// ══════════════════════════════════════════════════════════════════════════════

// Register adds a plugin to the registry.
func (r *Registry) Register(p Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info := p.Info()
	id := info.ID

	if _, exists := r.all[id]; exists {
		return fmt.Errorf("%w: %s", ErrPluginAlreadyLoaded, id)
	}

	// Register by type
	switch info.Type {
	case PluginTypeConnector:
		cp, ok := p.(ConnectorPlugin)
		if !ok {
			return fmt.Errorf("%w: expected ConnectorPlugin for %s", ErrPluginTypeMismatch, id)
		}
		r.connectors[id] = cp

	case PluginTypeProcessor:
		pp, ok := p.(ProcessorPlugin)
		if !ok {
			return fmt.Errorf("%w: expected ProcessorPlugin for %s", ErrPluginTypeMismatch, id)
		}
		r.processors[id] = pp

	case PluginTypeOutput:
		op, ok := p.(OutputPlugin)
		if !ok {
			return fmt.Errorf("%w: expected OutputPlugin for %s", ErrPluginTypeMismatch, id)
		}
		r.outputs[id] = op

	default:
		return fmt.Errorf("%w: unknown type %s for %s", ErrPluginTypeMismatch, info.Type, id)
	}

	r.all[id] = p
	r.order = append(r.order, id)

	// Fire callbacks
	for _, cb := range r.onLoad {
		cb(p)
	}

	return nil
}

// Unregister removes a plugin from the registry.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.all[id]
	if !exists {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}

	info := p.Info()

	// Remove from type-specific maps
	switch info.Type {
	case PluginTypeConnector:
		delete(r.connectors, id)
	case PluginTypeProcessor:
		delete(r.processors, id)
	case PluginTypeOutput:
		delete(r.outputs, id)
	}

	delete(r.all, id)

	// Remove from order
	for i, oid := range r.order {
		if oid == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}

	// Fire callbacks
	for _, cb := range r.onUnload {
		cb(p)
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Lookup
// ══════════════════════════════════════════════════════════════════════════════

// Get returns a plugin by ID.
func (r *Registry) Get(id string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.all[id]
	return p, ok
}

// GetConnector returns a connector plugin by ID.
func (r *Registry) GetConnector(id string) (ConnectorPlugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.connectors[id]
	return p, ok
}

// GetProcessor returns a processor plugin by ID.
func (r *Registry) GetProcessor(id string) (ProcessorPlugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.processors[id]
	return p, ok
}

// GetOutput returns an output plugin by ID.
func (r *Registry) GetOutput(id string) (OutputPlugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.outputs[id]
	return p, ok
}

// ══════════════════════════════════════════════════════════════════════════════
// Listing
// ══════════════════════════════════════════════════════════════════════════════

// All returns all plugins in registration order.
func (r *Registry) All() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Plugin, 0, len(r.order))
	for _, id := range r.order {
		if p, ok := r.all[id]; ok {
			result = append(result, p)
		}
	}
	return result
}

// Connectors returns all connector plugins.
func (r *Registry) Connectors() []ConnectorPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ConnectorPlugin, 0, len(r.connectors))
	for _, id := range r.order {
		if p, ok := r.connectors[id]; ok {
			result = append(result, p)
		}
	}
	return result
}

// Processors returns all processor plugins.
func (r *Registry) Processors() []ProcessorPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ProcessorPlugin, 0, len(r.processors))
	for _, id := range r.order {
		if p, ok := r.processors[id]; ok {
			result = append(result, p)
		}
	}
	return result
}

// Outputs returns all output plugins.
func (r *Registry) Outputs() []OutputPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]OutputPlugin, 0, len(r.outputs))
	for _, id := range r.order {
		if p, ok := r.outputs[id]; ok {
			result = append(result, p)
		}
	}
	return result
}

// ══════════════════════════════════════════════════════════════════════════════
// Filtering
// ══════════════════════════════════════════════════════════════════════════════

// ConnectorsByCapability returns connectors with the given capability.
func (r *Registry) ConnectorsByCapability(cap string) []ConnectorPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ConnectorPlugin
	for _, id := range r.order {
		p, ok := r.connectors[id]
		if !ok {
			continue
		}
		for _, c := range p.Info().Capabilities {
			if string(c) == cap {
				result = append(result, p)
				break
			}
		}
	}
	return result
}

// ProcessorsByDataType returns processors that handle the given data type.
func (r *Registry) ProcessorsByDataType(dt string) []ProcessorPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ProcessorPlugin
	for _, id := range r.order {
		p, ok := r.processors[id]
		if !ok {
			continue
		}
		selector := p.Selector()
		if selector == nil || selector.All {
			result = append(result, p)
			continue
		}
		for _, t := range selector.DataTypes {
			if string(t) == dt {
				result = append(result, p)
				break
			}
		}
	}
	return result
}

// PluginsByState returns plugins in the given state.
func (r *Registry) PluginsByState(state PluginState) []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Plugin
	for _, id := range r.order {
		if p, ok := r.all[id]; ok && p.Info().State == state {
			result = append(result, p)
		}
	}
	return result
}

// PluginsByTier returns plugins of the given tier.
func (r *Registry) PluginsByTier(tier PluginTier) []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Plugin
	for _, id := range r.order {
		if p, ok := r.all[id]; ok && p.Info().Tier == tier {
			result = append(result, p)
		}
	}
	return result
}

// ══════════════════════════════════════════════════════════════════════════════
// Lifecycle
// ══════════════════════════════════════════════════════════════════════════════

// LoadAll loads all registered plugins.
func (r *Registry) LoadAll(ctx context.Context) error {
	r.mu.RLock()
	plugins := make([]Plugin, 0, len(r.all))
	for _, id := range r.order {
		if p, ok := r.all[id]; ok {
			plugins = append(plugins, p)
		}
	}
	r.mu.RUnlock()

	var errs []error
	for _, p := range plugins {
		if err := p.Load(ctx); err != nil {
			errs = append(errs, NewPluginError(p.Info().ID, "load", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to load %d plugins: %v", len(errs), errs[0])
	}
	return nil
}

// UnloadAll unloads all registered plugins.
func (r *Registry) UnloadAll() error {
	r.mu.RLock()
	plugins := make([]Plugin, 0, len(r.all))
	for _, id := range r.order {
		if p, ok := r.all[id]; ok {
			plugins = append(plugins, p)
		}
	}
	r.mu.RUnlock()

	// Unload in reverse order
	var errs []error
	for i := len(plugins) - 1; i >= 0; i-- {
		p := plugins[i]
		if err := p.Unload(); err != nil {
			errs = append(errs, NewPluginError(p.Info().ID, "unload", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to unload %d plugins: %v", len(errs), errs[0])
	}
	return nil
}

// Reload reloads a specific plugin.
func (r *Registry) Reload(ctx context.Context, id string) error {
	r.mu.RLock()
	p, ok := r.all[id]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}

	if err := p.Reload(ctx); err != nil {
		return NewPluginError(id, "reload", err)
	}

	// Fire callbacks
	r.mu.RLock()
	callbacks := r.onReload
	r.mu.RUnlock()

	for _, cb := range callbacks {
		cb(p)
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Events
// ══════════════════════════════════════════════════════════════════════════════

// OnLoad registers a callback for when a plugin is loaded.
func (r *Registry) OnLoad(cb func(Plugin)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onLoad = append(r.onLoad, cb)
}

// OnUnload registers a callback for when a plugin is unloaded.
func (r *Registry) OnUnload(cb func(Plugin)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onUnload = append(r.onUnload, cb)
}

// OnReload registers a callback for when a plugin is reloaded.
func (r *Registry) OnReload(cb func(Plugin)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onReload = append(r.onReload, cb)
}

// ══════════════════════════════════════════════════════════════════════════════
// Stats
// ══════════════════════════════════════════════════════════════════════════════

// Stats returns registry statistics.
func (r *Registry) Stats() RegistryStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := RegistryStats{
		Total:      len(r.all),
		Connectors: len(r.connectors),
		Processors: len(r.processors),
		Outputs:    len(r.outputs),
		ByTier:     make(map[PluginTier]int),
		ByState:    make(map[PluginState]int),
	}

	for _, p := range r.all {
		info := p.Info()
		stats.ByTier[info.Tier]++
		stats.ByState[info.State]++
	}

	return stats
}

// RegistryStats contains registry statistics.
type RegistryStats struct {
	Total      int
	Connectors int
	Processors int
	Outputs    int
	ByTier     map[PluginTier]int
	ByState    map[PluginState]int
}

// ══════════════════════════════════════════════════════════════════════════════
// Global Registry
// ══════════════════════════════════════════════════════════════════════════════

var globalRegistry = NewRegistry()

// Global returns the global plugin registry.
func Global() *Registry {
	return globalRegistry
}

// Register registers a plugin with the global registry.
func RegisterPlugin(p Plugin) error {
	return globalRegistry.Register(p)
}

// GetPlugin returns a plugin from the global registry.
func GetPlugin(id string) (Plugin, bool) {
	return globalRegistry.Get(id)
}

// GetConnectorPlugin returns a connector plugin from the global registry.
func GetConnectorPlugin(id string) (ConnectorPlugin, bool) {
	return globalRegistry.GetConnector(id)
}

// GetProcessorPlugin returns a processor plugin from the global registry.
func GetProcessorPlugin(id string) (ProcessorPlugin, bool) {
	return globalRegistry.GetProcessor(id)
}

// GetOutputPlugin returns an output plugin from the global registry.
func GetOutputPlugin(id string) (OutputPlugin, bool) {
	return globalRegistry.GetOutput(id)
}
