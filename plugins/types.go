// Package plugins provides a dynamic plugin system for revoco.
//
// The plugin system supports two tiers:
//   - Lua plugins: Sandboxed, single-file plugins for most use cases
//   - External plugins: Any language via JSON-RPC for complex integrations
//
// Plugin types: Connector, Processor, Output
package plugins

import (
	"context"
	"io"

	core "github.com/fulgidus/revoco/connectors"
	svccore "github.com/fulgidus/revoco/services/core"
)

// PluginType identifies the kind of plugin.
type PluginType string

const (
	PluginTypeConnector PluginType = "connector"
	PluginTypeProcessor PluginType = "processor"
	PluginTypeOutput    PluginType = "output"
)

// PluginTier identifies how the plugin is executed.
type PluginTier string

const (
	PluginTierLua      PluginTier = "lua"      // Embedded Lua interpreter
	PluginTierExternal PluginTier = "external" // External process via JSON-RPC
)

// PluginSource identifies where the plugin came from.
type PluginSource string

const (
	PluginSourceBuiltIn   PluginSource = "builtin"   // Shipped with revoco
	PluginSourceUser      PluginSource = "user"      // User's plugin directory
	PluginSourceCommunity PluginSource = "community" // Future: community repository
)

// PluginState represents the current state of a plugin.
type PluginState string

const (
	PluginStateUnloaded    PluginState = "unloaded"     // Not yet loaded
	PluginStateLoading     PluginState = "loading"      // Currently loading
	PluginStateReady       PluginState = "ready"        // Loaded and ready
	PluginStateMissingDeps PluginState = "missing_deps" // Missing binary dependencies
	PluginStateError       PluginState = "error"        // Failed to load
	PluginStateDisabled    PluginState = "disabled"     // Disabled by user
)

// PluginInfo contains metadata about a plugin.
type PluginInfo struct {
	// Identity
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Author      string `json:"author,omitempty"`

	// Classification
	Type   PluginType   `json:"type"`
	Tier   PluginTier   `json:"tier"`
	Source PluginSource `json:"source"`

	// Location
	Path string `json:"path"` // File path or directory

	// State
	State      PluginState `json:"state"`
	StateError string      `json:"state_error,omitempty"`

	// Capabilities (for connectors)
	Capabilities []core.ConnectorCapability `json:"capabilities,omitempty"`
	DataTypes    []core.DataType            `json:"data_types,omitempty"`
	RequiresAuth bool                       `json:"requires_auth,omitempty"`
	AuthType     string                     `json:"auth_type,omitempty"`

	// Configuration
	ConfigSchema []ConfigOption `json:"config_schema,omitempty"`

	// Dependencies
	Dependencies []BinaryDependency `json:"dependencies,omitempty"`

	// Default selector (for processors and outputs)
	DefaultSelector *Selector `json:"default_selector,omitempty"`
}

// ConfigOption describes a configurable setting for a plugin.
type ConfigOption struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"` // bool, string, int, float, select, path, password
	Default     any      `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"` // For select type
	Required    bool     `json:"required,omitempty"`
	Sensitive   bool     `json:"sensitive,omitempty"` // If true, mask in UI
}

// BinaryDependency describes an external binary required by a plugin.
type BinaryDependency struct {
	Binary       string            `json:"binary"`        // Binary name (e.g., "ffmpeg")
	Check        string            `json:"check"`         // Command to verify (e.g., "ffmpeg -version")
	VersionRegex string            `json:"version_regex"` // Regex to extract version
	MinVersion   string            `json:"min_version"`   // Minimum required version
	Install      map[string]string `json:"install"`       // Package manager -> install command

	// Runtime state (not serialized)
	Found   bool   `json:"-"`
	Version string `json:"-"`
}

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Interfaces
// ══════════════════════════════════════════════════════════════════════════════

// Plugin is the base interface for all plugins.
type Plugin interface {
	// Info returns the plugin metadata.
	Info() PluginInfo

	// Load initializes the plugin. Called once when discovered.
	Load(ctx context.Context) error

	// Unload releases any resources held by the plugin.
	Unload() error

	// Reload reloads the plugin (for hot-reload support).
	Reload(ctx context.Context) error
}

// ConnectorPlugin wraps a dynamically-loaded connector.
type ConnectorPlugin interface {
	Plugin

	// AsConnector returns the base connector interface.
	AsConnector() core.Connector

	// AsReader returns the reader interface if supported.
	AsReader() (core.ConnectorReader, bool)

	// AsWriter returns the writer interface if supported.
	AsWriter() (core.ConnectorWriter, bool)

	// AsTester returns the tester interface if supported.
	AsTester() (core.ConnectorTester, bool)
}

// ProcessorPlugin wraps a dynamically-loaded processor.
type ProcessorPlugin interface {
	Plugin

	// AsProcessor returns the processor interface.
	AsProcessor() Processor

	// Selector returns the default selector for this processor.
	Selector() *Selector
}

// OutputPlugin wraps a dynamically-loaded output.
type OutputPlugin interface {
	Plugin

	// AsOutput returns the output interface.
	AsOutput() svccore.Output

	// Selector returns the default selector for this output.
	Selector() *Selector
}

// ══════════════════════════════════════════════════════════════════════════════
// Processor Interface (Plugin-aware)
// ══════════════════════════════════════════════════════════════════════════════

// Processor is the interface for data processors.
// This extends the service-level processor with plugin-specific features.
type Processor interface {
	// Identity
	ID() string
	Name() string
	Description() string

	// Configuration
	ConfigSchema() []ConfigOption

	// Selector
	DefaultSelector() *Selector

	// Processing
	CanProcess(item *core.DataItem) bool
	Process(ctx context.Context, item *core.DataItem, config map[string]any) (*core.DataItem, error)
	ProcessBatch(ctx context.Context, items []*core.DataItem, config map[string]any, progress ProgressFunc) ([]*core.DataItem, error)
}

// ProgressFunc reports progress during processing.
type ProgressFunc func(done, total int, message string)

// ══════════════════════════════════════════════════════════════════════════════
// External Plugin Protocol
// ══════════════════════════════════════════════════════════════════════════════

// ExternalManifest is the manifest.json structure for external plugins.
type ExternalManifest struct {
	APIVersion string               `json:"apiVersion"` // e.g., "revoco/v1"
	Kind       string               `json:"kind"`       // "ExternalPlugin"
	Metadata   ExternalManifestMeta `json:"metadata"`
	Spec       ExternalManifestSpec `json:"spec"`
}

// ExternalManifestMeta contains plugin identity information.
type ExternalManifestMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
	Author      string `json:"author,omitempty"`
}

// ExternalManifestSpec contains plugin specification.
type ExternalManifestSpec struct {
	Type            PluginType         `json:"type"` // connector, processor, output
	Runtime         ExternalRuntime    `json:"runtime"`
	Entrypoint      string             `json:"entrypoint"`
	Setup           *ExternalSetup     `json:"setup,omitempty"`
	Capabilities    []string           `json:"capabilities,omitempty"`
	DataTypes       []string           `json:"dataTypes,omitempty"`
	RequiresAuth    bool               `json:"requiresAuth,omitempty"`
	AuthType        string             `json:"authType,omitempty"`
	ConfigSchema    []ConfigOption     `json:"configSchema,omitempty"`
	DefaultSelector *Selector          `json:"defaultSelector,omitempty"`
	Dependencies    []BinaryDependency `json:"dependencies,omitempty"`
}

// ExternalRuntime describes the runtime requirements.
type ExternalRuntime struct {
	Command      string `json:"command"`      // e.g., "python3"
	MinVersion   string `json:"minVersion"`   // e.g., "3.9"
	VersionCheck string `json:"versionCheck"` // e.g., "python3 --version"
}

// ExternalSetup describes setup requirements.
type ExternalSetup struct {
	Command string `json:"command"` // e.g., "pip install -r requirements.txt"
	RunWhen string `json:"runWhen"` // File to check for changes (e.g., "requirements.txt")
}

// ══════════════════════════════════════════════════════════════════════════════
// JSON-RPC Protocol Types
// ══════════════════════════════════════════════════════════════════════════════

// RPCRequest is a JSON-RPC 2.0 request.
type RPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

// RPCResponse is a JSON-RPC 2.0 response.
type RPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      int       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// Lua Plugin Types
// ══════════════════════════════════════════════════════════════════════════════

// LuaPluginMeta represents the Plugin table in a Lua plugin.
type LuaPluginMeta struct {
	ID           string   `lua:"id"`
	Name         string   `lua:"name"`
	Description  string   `lua:"description"`
	Version      string   `lua:"version"`
	Type         string   `lua:"type"` // connector, processor, output
	Capabilities []string `lua:"capabilities,omitempty"`
	DataTypes    []string `lua:"data_types,omitempty"`
	RequiresAuth bool     `lua:"requires_auth,omitempty"`
	AuthType     string   `lua:"auth_type,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Types
// ══════════════════════════════════════════════════════════════════════════════

// PluginReader wraps io.ReadCloser with plugin context.
type PluginReader struct {
	io.ReadCloser
	Plugin Plugin
	Item   *core.DataItem
}

// PluginConfig holds configuration for a plugin instance.
type PluginConfig struct {
	PluginID string         `json:"plugin_id"`
	Enabled  bool           `json:"enabled"`
	Settings map[string]any `json:"settings"`
	Selector *Selector      `json:"selector,omitempty"`
}
