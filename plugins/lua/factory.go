package lua

import (
	"context"
	"os"

	"github.com/fulgidus/revoco/plugins"
)

// Global runtime for the factory
var globalRuntime *Runtime

// init registers the Lua plugin factory with the plugin loader.
func init() {
	globalRuntime = NewRuntime()
	plugins.RegisterLuaPluginFactory(CreateLuaPlugin)
}

// CreateLuaPlugin creates a Lua plugin from discovered metadata.
// This is the factory function registered with the plugin loader.
func CreateLuaPlugin(dp *plugins.DiscoveredPlugin) (plugins.Plugin, error) {
	// Read the plugin source
	content, err := os.ReadFile(dp.Path)
	if err != nil {
		return nil, err
	}

	// Create a temporary state to parse and extract metadata
	L := globalRuntime.CreateState()
	defer L.Close()

	// Execute the file to parse it
	if err := L.DoString(string(content)); err != nil {
		return nil, &plugins.LuaError{
			PluginID: dp.Info.ID,
			Function: "parse",
			Message:  err.Error(),
		}
	}

	// Use the pre-extracted info from discovery
	info := dp.Info

	// Extract additional info from Lua state
	deps, err := globalRuntime.extractDependencies(L)
	if err == nil && len(deps) > 0 {
		info.Dependencies = deps
	}

	selector, err := globalRuntime.extractSelector(L, "DefaultSelector")
	if err == nil && selector != nil {
		info.DefaultSelector = selector
	}

	schema, err := globalRuntime.extractConfigSchema(L)
	if err == nil && len(schema) > 0 {
		info.ConfigSchema = schema
	}

	// Create the base Lua plugin
	luaPlugin := &LuaPlugin{
		runtime: globalRuntime,
		info:    info,
		path:    dp.Path,
		source:  string(content),
	}

	// Wrap in appropriate type-specific plugin
	switch info.Type {
	case plugins.PluginTypeConnector:
		return NewLuaConnectorPlugin(luaPlugin), nil
	case plugins.PluginTypeProcessor:
		return NewLuaProcessorPlugin(luaPlugin), nil
	case plugins.PluginTypeOutput:
		return NewLuaOutputPlugin(luaPlugin), nil
	default:
		return nil, &plugins.PluginError{
			PluginID: info.ID,
			Op:       "create",
			Err:      plugins.ErrPluginTypeMismatch,
		}
	}
}

// Runtime returns the global Lua runtime.
func GetRuntime() *Runtime {
	return globalRuntime
}

// LoadPluginFromFile loads a Lua plugin directly from a file path.
// This is a convenience function for testing and direct loading.
func LoadPluginFromFile(ctx context.Context, path string) (plugins.Plugin, error) {
	// Create a discovered plugin placeholder
	dp := &plugins.DiscoveredPlugin{
		Path: path,
		Tier: plugins.PluginTierLua,
		Info: &plugins.PluginInfo{
			Path: path,
		},
	}

	// Extract basic info from file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse to extract Plugin table
	L := globalRuntime.CreateState()
	defer L.Close()

	if err := L.DoString(string(content)); err != nil {
		return nil, err
	}

	// Extract info
	info, err := globalRuntime.extractPluginInfo(L, path)
	if err != nil {
		return nil, err
	}

	dp.Info = info
	dp.Type = info.Type

	return CreateLuaPlugin(dp)
}
