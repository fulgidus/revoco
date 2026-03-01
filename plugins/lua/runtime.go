// Package lua provides the Lua runtime for revoco plugins.
//
// This package embeds a sandboxed Lua interpreter using gopher-lua,
// providing a safe environment for user-created plugins.
package lua

import (
	"context"
	"fmt"
	"os"
	"sync"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/plugins"
	lua "github.com/yuin/gopher-lua"
)

// Runtime manages Lua plugin execution.
type Runtime struct {
	mu sync.Mutex

	// Pool of Lua states for concurrent execution
	pool    *StatePool
	maxPool int

	// Registered built-in modules
	modules map[string]lua.LGFunction

	// Dependency checker
	checker *plugins.DependencyChecker
}

// NewRuntime creates a new Lua runtime.
func NewRuntime() *Runtime {
	r := &Runtime{
		maxPool: 4,
		modules: make(map[string]lua.LGFunction),
		checker: plugins.NewDependencyChecker(),
	}

	// Register default modules
	r.registerDefaultModules()

	return r
}

// registerDefaultModules registers the built-in revoco module.
func (r *Runtime) registerDefaultModules() {
	r.modules["revoco"] = r.loaderRevoco
}

// StatePool manages a pool of Lua states.
type StatePool struct {
	mu     sync.Mutex
	states []*lua.LState
	create func() *lua.LState
	max    int
}

// NewStatePool creates a new state pool.
func NewStatePool(max int, create func() *lua.LState) *StatePool {
	return &StatePool{
		states: make([]*lua.LState, 0, max),
		create: create,
		max:    max,
	}
}

// Get retrieves a state from the pool or creates a new one.
func (p *StatePool) Get() *lua.LState {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.states) > 0 {
		L := p.states[len(p.states)-1]
		p.states = p.states[:len(p.states)-1]
		return L
	}

	return p.create()
}

// Put returns a state to the pool.
func (p *StatePool) Put(L *lua.LState) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.states) < p.max {
		p.states = append(p.states, L)
	} else {
		L.Close()
	}
}

// Shutdown closes all states in the pool.
func (p *StatePool) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, L := range p.states {
		L.Close()
	}
	p.states = nil
}

// ══════════════════════════════════════════════════════════════════════════════
// State Creation
// ══════════════════════════════════════════════════════════════════════════════

// CreateState creates a new sandboxed Lua state.
func (r *Runtime) CreateState() *lua.LState {
	// Create state with limited libraries
	L := lua.NewState(lua.Options{
		SkipOpenLibs: true, // Don't open any standard libs
	})

	// Open only safe libraries
	openSafeLibs(L)

	// Pre-load our modules
	for name, loader := range r.modules {
		L.PreloadModule(name, loader)
	}

	return L
}

// openSafeLibs opens only the safe standard libraries.
func openSafeLibs(L *lua.LState) {
	// Base library (print, type, pairs, ipairs, etc.)
	// We'll provide a custom version without dangerous functions
	lua.OpenBase(L)

	// Remove dangerous base functions
	L.SetGlobal("dofile", lua.LNil)
	L.SetGlobal("loadfile", lua.LNil)
	L.SetGlobal("load", lua.LNil)       // Can execute arbitrary code
	L.SetGlobal("loadstring", lua.LNil) // Can execute arbitrary code
	L.SetGlobal("rawget", lua.LNil)     // Can bypass metatables
	L.SetGlobal("rawset", lua.LNil)     // Can bypass metatables
	L.SetGlobal("rawequal", lua.LNil)
	L.SetGlobal("collectgarbage", lua.LNil) // Can affect GC

	// Table library - safe
	lua.OpenTable(L)

	// String library - safe
	lua.OpenString(L)

	// Math library - safe
	lua.OpenMath(L)

	// Package library - needed for require() and PreloadModule
	// We open it but then restrict it
	lua.OpenPackage(L)

	// Restrict package library - remove dangerous functions
	pkg := L.GetGlobal("package")
	if pkgTbl, ok := pkg.(*lua.LTable); ok {
		// Remove searchpath and loadlib
		pkgTbl.RawSetString("searchpath", lua.LNil)
		pkgTbl.RawSetString("loadlib", lua.LNil)
		// Clear path and cpath to prevent loading from filesystem
		pkgTbl.RawSetString("path", lua.LString(""))
		pkgTbl.RawSetString("cpath", lua.LString(""))
	}

	// We explicitly DO NOT open:
	// - os: file system and process access
	// - io: file I/O
	// - debug: can break sandbox
	// - channel: goroutine communication
	// - coroutine: can be abused
}

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Loading
// ══════════════════════════════════════════════════════════════════════════════

// LoadPlugin loads a Lua plugin from a file.
func (r *Runtime) LoadPlugin(ctx context.Context, path string) (*LuaPlugin, error) {
	// Read the file
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin: %w", err)
	}

	// Create a state for parsing
	L := r.CreateState()
	defer L.Close()

	// Execute the file to get the plugin definition
	if err := L.DoString(string(content)); err != nil {
		return nil, fmt.Errorf("failed to parse plugin: %w", err)
	}

	// Extract plugin metadata
	info, err := r.extractPluginInfo(L, path)
	if err != nil {
		return nil, err
	}

	// Extract dependencies
	deps, err := r.extractDependencies(L)
	if err != nil {
		return nil, err
	}
	info.Dependencies = deps

	// Extract default selector
	selector, err := r.extractSelector(L, "DefaultSelector")
	if err != nil {
		return nil, err
	}
	info.DefaultSelector = selector

	// Extract config schema
	schema, err := r.extractConfigSchema(L)
	if err != nil {
		return nil, err
	}
	info.ConfigSchema = schema

	// Create the plugin
	plugin := &LuaPlugin{
		runtime: r,
		info:    info,
		path:    path,
		source:  string(content),
	}

	return plugin, nil
}

// extractPluginInfo extracts the Plugin table from Lua state.
func (r *Runtime) extractPluginInfo(L *lua.LState, path string) (*plugins.PluginInfo, error) {
	pluginTable := L.GetGlobal("Plugin")
	if pluginTable == lua.LNil {
		return nil, fmt.Errorf("plugin missing required 'Plugin' table")
	}

	tbl, ok := pluginTable.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("'Plugin' must be a table")
	}

	info := &plugins.PluginInfo{
		Tier:   plugins.PluginTierLua,
		Source: plugins.PluginSourceUser,
		Path:   path,
		State:  plugins.PluginStateUnloaded,
	}

	// Extract required fields
	if id := getStringField(tbl, "id"); id != "" {
		info.ID = id
	} else {
		return nil, fmt.Errorf("plugin missing required 'id' field")
	}

	// Extract optional fields
	info.Name = getStringField(tbl, "name")
	if info.Name == "" {
		info.Name = info.ID
	}
	info.Description = getStringField(tbl, "description")
	info.Version = getStringField(tbl, "version")

	// Extract type
	typeStr := getStringField(tbl, "type")
	switch typeStr {
	case "connector":
		info.Type = plugins.PluginTypeConnector
	case "processor":
		info.Type = plugins.PluginTypeProcessor
	case "output":
		info.Type = plugins.PluginTypeOutput
	default:
		return nil, fmt.Errorf("invalid plugin type: %s", typeStr)
	}

	// Extract capabilities (for connectors)
	if caps := getStringArrayField(tbl, "capabilities"); len(caps) > 0 {
		for _, cap := range caps {
			info.Capabilities = append(info.Capabilities, core.ConnectorCapability(cap))
		}
	}

	// Extract data types
	if dts := getStringArrayField(tbl, "data_types"); len(dts) > 0 {
		for _, dt := range dts {
			info.DataTypes = append(info.DataTypes, core.DataType(dt))
		}
	}

	// Extract auth info
	info.RequiresAuth = getBoolField(tbl, "requires_auth")
	info.AuthType = getStringField(tbl, "auth_type")

	return info, nil
}

// extractDependencies extracts the Dependencies table from Lua state.
func (r *Runtime) extractDependencies(L *lua.LState) ([]plugins.BinaryDependency, error) {
	depsTable := L.GetGlobal("Dependencies")
	if depsTable == lua.LNil {
		return nil, nil
	}

	tbl, ok := depsTable.(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("'Dependencies' must be a table")
	}

	var deps []plugins.BinaryDependency

	tbl.ForEach(func(_, value lua.LValue) {
		depTbl, ok := value.(*lua.LTable)
		if !ok {
			return
		}

		dep := plugins.BinaryDependency{
			Binary:       getStringField(depTbl, "binary"),
			Check:        getStringField(depTbl, "check"),
			VersionRegex: getStringField(depTbl, "version_regex"),
			MinVersion:   getStringField(depTbl, "min_version"),
		}

		// Extract install commands
		installTbl := depTbl.RawGetString("install")
		if installTbl != lua.LNil {
			if it, ok := installTbl.(*lua.LTable); ok {
				dep.Install = make(map[string]string)
				it.ForEach(func(key, value lua.LValue) {
					if k, ok := key.(lua.LString); ok {
						if v, ok := value.(lua.LString); ok {
							dep.Install[string(k)] = string(v)
						}
					}
				})
			}
		}

		if dep.Binary != "" {
			deps = append(deps, dep)
		}
	})

	return deps, nil
}

// extractSelector extracts a selector table from Lua state.
func (r *Runtime) extractSelector(L *lua.LState, name string) (*plugins.Selector, error) {
	selectorTable := L.GetGlobal(name)
	if selectorTable == lua.LNil {
		return nil, nil
	}

	tbl, ok := selectorTable.(*lua.LTable)
	if !ok {
		return nil, nil
	}

	selector := &plugins.Selector{
		Extensions:       getStringArrayField(tbl, "extensions"),
		MimeTypes:        getStringArrayField(tbl, "mime_types"),
		SourceConnectors: getStringArrayField(tbl, "source_connectors"),
		PathMatch:        getStringField(tbl, "path_match"),
		PathExclude:      getStringField(tbl, "path_exclude"),
		Condition:        getStringField(tbl, "condition"),
		All:              getBoolField(tbl, "all"),
	}

	// Extract data types
	if dts := getStringArrayField(tbl, "data_types"); len(dts) > 0 {
		for _, dt := range dts {
			selector.DataTypes = append(selector.DataTypes, core.DataType(dt))
		}
	}

	return selector, nil
}

// extractConfigSchema extracts the ConfigSchema table from Lua state.
func (r *Runtime) extractConfigSchema(L *lua.LState) ([]plugins.ConfigOption, error) {
	schemaTable := L.GetGlobal("ConfigSchema")
	if schemaTable == lua.LNil {
		return nil, nil
	}

	tbl, ok := schemaTable.(*lua.LTable)
	if !ok {
		return nil, nil
	}

	var schema []plugins.ConfigOption

	tbl.ForEach(func(_, value lua.LValue) {
		optTbl, ok := value.(*lua.LTable)
		if !ok {
			return
		}

		opt := plugins.ConfigOption{
			ID:          getStringField(optTbl, "id"),
			Name:        getStringField(optTbl, "name"),
			Description: getStringField(optTbl, "description"),
			Type:        getStringField(optTbl, "type"),
			Required:    getBoolField(optTbl, "required"),
			Sensitive:   getBoolField(optTbl, "sensitive"),
			Options:     getStringArrayField(optTbl, "options"),
		}

		// Extract default value
		defaultVal := optTbl.RawGetString("default")
		if defaultVal != lua.LNil {
			opt.Default = luaValueToGo(defaultVal)
		}

		if opt.ID != "" {
			schema = append(schema, opt)
		}
	})

	return schema, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Functions
// ══════════════════════════════════════════════════════════════════════════════

// getStringField extracts a string field from a Lua table.
func getStringField(tbl *lua.LTable, key string) string {
	val := tbl.RawGetString(key)
	if str, ok := val.(lua.LString); ok {
		return string(str)
	}
	return ""
}

// getBoolField extracts a bool field from a Lua table.
func getBoolField(tbl *lua.LTable, key string) bool {
	val := tbl.RawGetString(key)
	if b, ok := val.(lua.LBool); ok {
		return bool(b)
	}
	return false
}

// getStringArrayField extracts a string array from a Lua table.
func getStringArrayField(tbl *lua.LTable, key string) []string {
	val := tbl.RawGetString(key)
	arrTbl, ok := val.(*lua.LTable)
	if !ok {
		return nil
	}

	var result []string
	arrTbl.ForEach(func(_, value lua.LValue) {
		if str, ok := value.(lua.LString); ok {
			result = append(result, string(str))
		}
	})
	return result
}

// luaValueToGo converts a Lua value to a Go value.
func luaValueToGo(val lua.LValue) any {
	switch v := val.(type) {
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LTable:
		return luaTableToMap(v)
	case *lua.LNilType:
		return nil
	default:
		return nil
	}
}

// luaTableToMap converts a Lua table to a Go map.
func luaTableToMap(tbl *lua.LTable) map[string]any {
	result := make(map[string]any)
	tbl.ForEach(func(key, value lua.LValue) {
		if k, ok := key.(lua.LString); ok {
			result[string(k)] = luaValueToGo(value)
		}
	})
	return result
}

// goValueToLua converts a Go value to a Lua value.
func goValueToLua(L *lua.LState, val any) lua.LValue {
	if val == nil {
		return lua.LNil
	}

	switch v := val.(type) {
	case bool:
		return lua.LBool(v)
	case int:
		return lua.LNumber(v)
	case int64:
		return lua.LNumber(v)
	case float64:
		return lua.LNumber(v)
	case string:
		return lua.LString(v)
	case []string:
		tbl := L.NewTable()
		for i, s := range v {
			tbl.RawSetInt(i+1, lua.LString(s))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for k, val := range v {
			tbl.RawSetString(k, goValueToLua(L, val))
		}
		return tbl
	default:
		return lua.LNil
	}
}
