package plugins

import (
	"errors"
	"fmt"
)

// Common plugin errors.
var (
	ErrPluginNotFound      = errors.New("plugin not found")
	ErrPluginAlreadyLoaded = errors.New("plugin already loaded")
	ErrPluginLoadFailed    = errors.New("plugin load failed")
	ErrPluginDisabled      = errors.New("plugin is disabled")
	ErrPluginTypeMismatch  = errors.New("plugin type mismatch")
	ErrInvalidManifest     = errors.New("invalid plugin manifest")
	ErrMissingDependency   = errors.New("missing binary dependency")
	ErrRuntimeNotFound     = errors.New("runtime not found")
	ErrSelectorInvalid     = errors.New("invalid selector")
	ErrProcessingFailed    = errors.New("processing failed")
	ErrConnectionFailed    = errors.New("connection failed")
	ErrConfigInvalid       = errors.New("invalid configuration")
)

// PluginError wraps an error with plugin context.
type PluginError struct {
	PluginID string
	Op       string // Operation that failed
	Err      error
}

func (e *PluginError) Error() string {
	if e.PluginID != "" {
		return fmt.Sprintf("plugin %s: %s: %v", e.PluginID, e.Op, e.Err)
	}
	return fmt.Sprintf("plugin: %s: %v", e.Op, e.Err)
}

func (e *PluginError) Unwrap() error {
	return e.Err
}

// NewPluginError creates a new PluginError.
func NewPluginError(pluginID, op string, err error) *PluginError {
	return &PluginError{
		PluginID: pluginID,
		Op:       op,
		Err:      err,
	}
}

// SelectorError wraps an error with selector field context.
type SelectorError struct {
	Field string
	Err   error
}

func (e *SelectorError) Error() string {
	return fmt.Sprintf("selector %s: %v", e.Field, e.Err)
}

func (e *SelectorError) Unwrap() error {
	return e.Err
}

// DependencyError wraps an error with dependency context.
type DependencyError struct {
	Binary  string
	Message string
	Install map[string]string
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("dependency %s: %s", e.Binary, e.Message)
}

// RPCCallError wraps an error from an external plugin RPC call.
type RPCCallError struct {
	PluginID string
	Method   string
	Code     int
	Message  string
}

func (e *RPCCallError) Error() string {
	return fmt.Sprintf("plugin %s: RPC %s failed (%d): %s", e.PluginID, e.Method, e.Code, e.Message)
}

// LuaError wraps an error from Lua execution.
type LuaError struct {
	PluginID string
	Function string
	Message  string
}

func (e *LuaError) Error() string {
	return fmt.Sprintf("plugin %s: Lua %s: %s", e.PluginID, e.Function, e.Message)
}

// ManifestError wraps an error in manifest parsing/validation.
type ManifestError struct {
	Path   string
	Field  string
	Reason string
}

func (e *ManifestError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("manifest %s: field %s: %s", e.Path, e.Field, e.Reason)
	}
	return fmt.Sprintf("manifest %s: %s", e.Path, e.Reason)
}

// IsPluginError checks if err is a PluginError for the given plugin.
func IsPluginError(err error, pluginID string) bool {
	var pe *PluginError
	if errors.As(err, &pe) {
		return pe.PluginID == pluginID || pluginID == ""
	}
	return false
}

// IsMissingDependency checks if err is a missing dependency error.
func IsMissingDependency(err error) bool {
	return errors.Is(err, ErrMissingDependency) || errors.As(err, new(*DependencyError))
}

// IsRPCError checks if err is an RPC call error.
func IsRPCError(err error) bool {
	var rpcErr *RPCCallError
	return errors.As(err, &rpcErr)
}

// IsLuaError checks if err is a Lua execution error.
func IsLuaError(err error) bool {
	var luaErr *LuaError
	return errors.As(err, &luaErr)
}
