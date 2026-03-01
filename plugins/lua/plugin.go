package lua

import (
	"context"
	"fmt"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/plugins"
	lua "github.com/yuin/gopher-lua"
)

// LuaPlugin represents a loaded Lua plugin.
type LuaPlugin struct {
	runtime *Runtime
	info    *plugins.PluginInfo
	path    string
	source  string

	// Current Lua state (nil when not loaded)
	state *lua.LState
}

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Interface Implementation
// ══════════════════════════════════════════════════════════════════════════════

// Info returns the plugin metadata.
func (p *LuaPlugin) Info() plugins.PluginInfo {
	if p.info == nil {
		return plugins.PluginInfo{}
	}
	return *p.info
}

// Load initializes the plugin.
func (p *LuaPlugin) Load(ctx context.Context) error {
	// Check dependencies
	if len(p.info.Dependencies) > 0 {
		missing := p.runtime.checker.MissingDependencies(p.info.Dependencies)
		if len(missing) > 0 {
			p.info.State = plugins.PluginStateMissingDeps
			p.info.StateError = fmt.Sprintf("missing: %s", missing[0].Binary)
			return &plugins.DependencyError{
				Binary:  missing[0].Binary,
				Message: "binary not found",
				Install: missing[0].Install,
			}
		}
	}

	// Create Lua state
	p.state = p.runtime.CreateState()

	// Execute the plugin source
	if err := p.state.DoString(p.source); err != nil {
		p.state.Close()
		p.state = nil
		p.info.State = plugins.PluginStateError
		p.info.StateError = err.Error()
		return &plugins.LuaError{
			PluginID: p.info.ID,
			Function: "load",
			Message:  err.Error(),
		}
	}

	p.info.State = plugins.PluginStateReady
	return nil
}

// Unload releases resources held by the plugin.
func (p *LuaPlugin) Unload() error {
	if p.state != nil {
		p.state.Close()
		p.state = nil
	}
	p.info.State = plugins.PluginStateUnloaded
	return nil
}

// Reload reloads the plugin.
func (p *LuaPlugin) Reload(ctx context.Context) error {
	if err := p.Unload(); err != nil {
		return err
	}
	return p.Load(ctx)
}

// ══════════════════════════════════════════════════════════════════════════════
// Lua Function Invocation
// ══════════════════════════════════════════════════════════════════════════════

// CallFunction calls a Lua function by name.
func (p *LuaPlugin) CallFunction(name string, args ...lua.LValue) ([]lua.LValue, error) {
	if p.state == nil {
		return nil, fmt.Errorf("plugin not loaded")
	}

	fn := p.state.GetGlobal(name)
	if fn == lua.LNil {
		return nil, fmt.Errorf("function %s not found", name)
	}

	if _, ok := fn.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("%s is not a function", name)
	}

	// Push arguments
	if err := p.state.CallByParam(lua.P{
		Fn:      fn,
		NRet:    lua.MultRet,
		Protect: true,
	}, args...); err != nil {
		return nil, &plugins.LuaError{
			PluginID: p.info.ID,
			Function: name,
			Message:  err.Error(),
		}
	}

	// Collect return values
	var results []lua.LValue
	for i := 1; i <= p.state.GetTop(); i++ {
		results = append(results, p.state.Get(i))
	}
	p.state.Pop(p.state.GetTop())

	return results, nil
}

// HasFunction checks if a function exists in the plugin.
func (p *LuaPlugin) HasFunction(name string) bool {
	if p.state == nil {
		return false
	}
	fn := p.state.GetGlobal(name)
	_, ok := fn.(*lua.LFunction)
	return ok
}

// ══════════════════════════════════════════════════════════════════════════════
// Data Conversion
// ══════════════════════════════════════════════════════════════════════════════

// DataItemToLua converts a DataItem to a Lua table.
func (p *LuaPlugin) DataItemToLua(item *core.DataItem) *lua.LTable {
	if p.state == nil || item == nil {
		return nil
	}

	tbl := p.state.NewTable()
	tbl.RawSetString("id", lua.LString(item.ID))
	tbl.RawSetString("type", lua.LString(string(item.Type)))
	tbl.RawSetString("path", lua.LString(item.Path))
	tbl.RawSetString("remote_id", lua.LString(item.RemoteID))
	tbl.RawSetString("source_conn_id", lua.LString(item.SourceConnID))
	tbl.RawSetString("size", lua.LNumber(item.Size))
	tbl.RawSetString("checksum", lua.LString(item.Checksum))

	// Convert metadata
	if item.Metadata != nil {
		metaTbl := p.state.NewTable()
		for k, v := range item.Metadata {
			metaTbl.RawSetString(k, goValueToLua(p.state, v))
		}
		tbl.RawSetString("metadata", metaTbl)
	}

	return tbl
}

// LuaToDataItem converts a Lua table to a DataItem.
func (p *LuaPlugin) LuaToDataItem(tbl *lua.LTable) *core.DataItem {
	if tbl == nil {
		return nil
	}

	item := &core.DataItem{
		ID:           getStringField(tbl, "id"),
		Type:         core.DataType(getStringField(tbl, "type")),
		Path:         getStringField(tbl, "path"),
		RemoteID:     getStringField(tbl, "remote_id"),
		SourceConnID: getStringField(tbl, "source_conn_id"),
		Checksum:     getStringField(tbl, "checksum"),
	}

	// Get size
	if size := tbl.RawGetString("size"); size != lua.LNil {
		if n, ok := size.(lua.LNumber); ok {
			item.Size = int64(n)
		}
	}

	// Get metadata
	if meta := tbl.RawGetString("metadata"); meta != lua.LNil {
		if metaTbl, ok := meta.(*lua.LTable); ok {
			item.Metadata = luaTableToMap(metaTbl)
		}
	}

	return item
}

// ConfigToLua converts a config map to a Lua table.
func (p *LuaPlugin) ConfigToLua(config map[string]any) *lua.LTable {
	if p.state == nil {
		return nil
	}

	tbl := p.state.NewTable()
	for k, v := range config {
		tbl.RawSetString(k, goValueToLua(p.state, v))
	}
	return tbl
}

// ══════════════════════════════════════════════════════════════════════════════
// Selector Implementation
// ══════════════════════════════════════════════════════════════════════════════

// Selector returns the default selector for this plugin.
func (p *LuaPlugin) Selector() *plugins.Selector {
	return p.info.DefaultSelector
}

// EvaluateCondition evaluates a Lua condition expression.
func (p *LuaPlugin) EvaluateCondition(item *core.DataItem, condition string) (bool, error) {
	if p.state == nil {
		return false, fmt.Errorf("plugin not loaded")
	}

	// Create a wrapper function that returns the condition result
	wrapper := fmt.Sprintf("return function(item) return %s end", condition)

	if err := p.state.DoString(wrapper); err != nil {
		return false, &plugins.LuaError{
			PluginID: p.info.ID,
			Function: "condition",
			Message:  err.Error(),
		}
	}

	// Get the returned function
	fn := p.state.Get(-1)
	p.state.Pop(1)

	if _, ok := fn.(*lua.LFunction); !ok {
		return false, fmt.Errorf("condition did not return a function")
	}

	// Call with item
	itemTbl := p.DataItemToLua(item)
	if err := p.state.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, itemTbl); err != nil {
		return false, err
	}

	result := p.state.Get(-1)
	p.state.Pop(1)

	// Convert to bool
	switch v := result.(type) {
	case lua.LBool:
		return bool(v), nil
	case *lua.LNilType:
		return false, nil
	default:
		// Truthy check
		return result != lua.LNil && result != lua.LFalse, nil
	}
}
