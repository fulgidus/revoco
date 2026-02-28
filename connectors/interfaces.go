// Package core defines the interfaces for revoco's modular connector architecture.
//
// The architecture consists of:
//   - Connectors: Unified I/O abstractions that can pull data, push data, or both
//   - DataTypes: Categories of data (photos, videos, notes, music, etc.)
//   - Processors: Transform data, can be type-specific or source-specific
//   - Credentials: Authentication storage (global or per-session)
//
// Flow:
//  1. Create empty session
//  2. Add connectors (configure role: input/output/fallback, auth, import mode)
//  3. Retrieve data from input connectors (parallel by default)
//  4. Detect data types, configure processors
//  5. Process data (manual or auto)
//  6. Analyze statistics
//  7. Repair using fallback connectors
//  8. Push to output connectors
package core

import (
	"context"
	"io"
)

// ══════════════════════════════════════════════════════════════════════════════
// Data Types
// ══════════════════════════════════════════════════════════════════════════════

// DataType represents a category of data that can be processed.
type DataType string

const (
	DataTypePhoto    DataType = "photo"
	DataTypeVideo    DataType = "video"
	DataTypeAudio    DataType = "audio"
	DataTypeNote     DataType = "note"
	DataTypePlaylist DataType = "playlist"
	DataTypeAlbum    DataType = "album"
	DataTypeContact  DataType = "contact"
	DataTypeDocument DataType = "document"
	DataTypeUnknown  DataType = "unknown"
)

// DataItem represents a single piece of data with its type and metadata.
type DataItem struct {
	ID           string         `json:"id"`
	Type         DataType       `json:"type"`
	Path         string         `json:"path"`           // Local path (if available)
	RemoteID     string         `json:"remote_id"`      // ID in remote system (if applicable)
	SourceConnID string         `json:"source_conn_id"` // Which connector provided this
	Metadata     map[string]any `json:"metadata"`
	Size         int64          `json:"size"`
	Checksum     string         `json:"checksum,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════════
// Connector Capabilities & Roles
// ══════════════════════════════════════════════════════════════════════════════

// ConnectorCapability describes what a connector can do.
type ConnectorCapability string

const (
	CapabilityRead   ConnectorCapability = "read"   // Can pull/retrieve data
	CapabilityWrite  ConnectorCapability = "write"  // Can push/upload data
	CapabilityDelete ConnectorCapability = "delete" // Can delete data
	CapabilityList   ConnectorCapability = "list"   // Can list available data
	CapabilitySearch ConnectorCapability = "search" // Can search for specific items
	CapabilityRepair ConnectorCapability = "repair" // Can fetch missing/corrupted items
)

// ConnectorRoles describes how a connector instance is used in a session.
// A connector can have any combination of roles.
type ConnectorRoles struct {
	IsInput    bool `json:"is_input"`    // Primary data source
	IsOutput   bool `json:"is_output"`   // Primary data destination
	IsFallback bool `json:"is_fallback"` // Used for repair/missing data
}

// String returns a human-readable representation of the roles.
func (r ConnectorRoles) String() string {
	var roles []string
	if r.IsInput {
		roles = append(roles, "input")
	}
	if r.IsOutput {
		roles = append(roles, "output")
	}
	if r.IsFallback {
		roles = append(roles, "fallback")
	}
	if len(roles) == 0 {
		return "none"
	}
	result := roles[0]
	for i := 1; i < len(roles); i++ {
		result += "+" + roles[i]
	}
	return result
}

// HasAnyRole returns true if at least one role is set.
func (r ConnectorRoles) HasAnyRole() bool {
	return r.IsInput || r.IsOutput || r.IsFallback
}

// CanRead returns true if this connector should be used for reading data.
func (r ConnectorRoles) CanRead() bool {
	return r.IsInput || r.IsFallback
}

// CanWrite returns true if this connector should be used for writing data.
func (r ConnectorRoles) CanWrite() bool {
	return r.IsOutput
}

// Legacy role constants for migration compatibility
type ConnectorRole string

const (
	RoleInput    ConnectorRole = "input"
	RoleOutput   ConnectorRole = "output"
	RoleFallback ConnectorRole = "fallback"
	RoleBoth     ConnectorRole = "both"
)

// RolesToLegacy converts ConnectorRoles to the legacy single-value format.
func RolesToLegacy(r ConnectorRoles) ConnectorRole {
	if r.IsInput && r.IsOutput {
		return RoleBoth
	}
	if r.IsInput {
		return RoleInput
	}
	if r.IsOutput {
		return RoleOutput
	}
	if r.IsFallback {
		return RoleFallback
	}
	return ""
}

// RolesFromLegacy converts a legacy role string to ConnectorRoles.
func RolesFromLegacy(role ConnectorRole) ConnectorRoles {
	switch role {
	case RoleInput:
		return ConnectorRoles{IsInput: true}
	case RoleOutput:
		return ConnectorRoles{IsOutput: true}
	case RoleFallback:
		return ConnectorRoles{IsFallback: true}
	case RoleBoth:
		return ConnectorRoles{IsInput: true, IsOutput: true}
	default:
		return ConnectorRoles{}
	}
}

// ImportMode describes how data should be imported from a connector.
type ImportMode string

const (
	ImportModeCopy      ImportMode = "copy"      // Copy data to session folder
	ImportModeMove      ImportMode = "move"      // Move data to session folder
	ImportModeReference ImportMode = "reference" // Just reference, don't copy
)

// ══════════════════════════════════════════════════════════════════════════════
// Connector Interface
// ══════════════════════════════════════════════════════════════════════════════

// Connector is the unified interface for all data sources and destinations.
// A connector can act as input, output, or both depending on its capabilities
// and how it's configured in a session.
type Connector interface {
	// ── Identity ──────────────────────────────────────────────────────────────

	// ID returns the unique identifier for this connector type.
	ID() string
	// Name returns the human-readable name for display.
	Name() string
	// Description returns a brief description of this connector.
	Description() string

	// ── Capabilities ──────────────────────────────────────────────────────────

	// Capabilities returns what this connector can do.
	Capabilities() []ConnectorCapability
	// SupportedDataTypes returns what types of data this connector handles.
	SupportedDataTypes() []DataType
	// RequiresAuth returns true if this connector needs authentication.
	RequiresAuth() bool
	// AuthType returns the authentication type (e.g., "oauth", "apikey", "cookie", "none").
	AuthType() string

	// ── Configuration ─────────────────────────────────────────────────────────

	// ConfigSchema returns the configuration options for this connector.
	ConfigSchema() []ConfigOption
	// ValidateConfig checks if the provided configuration is valid.
	ValidateConfig(cfg ConnectorConfig) error

	// ── Fallback Support ──────────────────────────────────────────────────────

	// FallbackOptions returns connectors that can be used as fallbacks for this one.
	// Each option includes setup instructions for the user.
	FallbackOptions() []FallbackOption
}

// ConnectorWithSetup is an optional interface for connectors that need setup instructions.
// This is useful for OAuth-based connectors or those requiring external API keys.
type ConnectorWithSetup interface {
	Connector
	// SetupInstructions returns markdown-formatted setup instructions.
	// These are displayed to the user before configuration.
	SetupInstructions() string
}

// ConnectorTester is an optional interface for connectors that can test their connection.
// This is used to verify configuration before use and in the dashboard context menu.
type ConnectorTester interface {
	Connector
	// TestConnection verifies the connector configuration is valid and can connect.
	// Returns nil if successful, or an error describing what failed.
	// For OAuth connectors, this may trigger the authorization flow.
	TestConnection(ctx context.Context, cfg ConnectorConfig) error
}

// TestResult contains the result of a connection test.
type TestResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// ConnectorConfig holds the configuration for a connector instance in a session.
type ConnectorConfig struct {
	// ConnectorID is the type of connector (e.g., "google-photos-api", "local-folder")
	ConnectorID string `json:"connector_id"`
	// InstanceID is a unique ID for this instance within the session
	InstanceID string `json:"instance_id"`
	// Name is a user-friendly label (e.g., "My Takeout ZIP", "iCloud Backup")
	Name string `json:"name"`
	// Roles defines how this connector is used (input/output/fallback in any combination)
	Roles ConnectorRoles `json:"roles"`
	// ImportMode defines how data is imported (for input connectors)
	ImportMode ImportMode `json:"import_mode,omitempty"`
	// Settings holds connector-specific configuration
	Settings map[string]any `json:"settings"`
	// CredentialID references stored credentials (if RequiresAuth)
	CredentialID string `json:"credential_id,omitempty"`
	// FallbackFor lists instance IDs this connector serves as fallback for
	FallbackFor []string `json:"fallback_for,omitempty"`
	// Enabled indicates if this connector is active
	Enabled bool `json:"enabled"`
}

// FallbackOption describes a possible fallback connector with setup instructions.
type FallbackOption struct {
	// ConnectorID is the connector type that can serve as fallback
	ConnectorID string `json:"connector_id"`
	// Name is a human-readable name
	Name string `json:"name"`
	// Description explains what this fallback provides
	Description string `json:"description"`
	// SetupInstructions tells the user how to configure this fallback
	SetupInstructions string `json:"setup_instructions"`
	// RequiredCapabilities lists what the fallback needs to provide
	RequiredCapabilities []ConnectorCapability `json:"required_capabilities"`
}

// ══════════════════════════════════════════════════════════════════════════════
// Connector Operations
// ══════════════════════════════════════════════════════════════════════════════

// ConnectorReader is implemented by connectors that can read/retrieve data.
type ConnectorReader interface {
	Connector
	// Initialize prepares the connector for reading (auth, connection, etc.)
	Initialize(ctx context.Context, cfg ConnectorConfig) error
	// List returns all available items from this connector.
	List(ctx context.Context, progress ProgressFunc) ([]DataItem, error)
	// Read retrieves a single item's content.
	Read(ctx context.Context, item DataItem) (io.ReadCloser, error)
	// ReadTo retrieves an item and writes it to the specified path.
	ReadTo(ctx context.Context, item DataItem, destPath string, mode ImportMode) error
	// Close releases any resources held by the connector.
	Close() error
}

// ConnectorWriter is implemented by connectors that can write/push data.
type ConnectorWriter interface {
	Connector
	// Initialize prepares the connector for writing.
	Initialize(ctx context.Context, cfg ConnectorConfig) error
	// Write sends a single item to the destination.
	Write(ctx context.Context, item DataItem, reader io.Reader) error
	// WriteFrom sends an item from a local path to the destination.
	WriteFrom(ctx context.Context, item DataItem, sourcePath string) error
	// WriteBatch sends multiple items (for connectors that benefit from batching).
	WriteBatch(ctx context.Context, items []DataItem, getReader func(DataItem) (io.Reader, error), progress ProgressFunc) error
	// Delete removes an item from the destination (if supported).
	Delete(ctx context.Context, item DataItem) error
	// Close releases any resources.
	Close() error
}

// ConnectorRepairer is implemented by connectors that can fetch missing/corrupted items.
type ConnectorRepairer interface {
	ConnectorReader
	// CanRepair checks if this connector can provide the given item.
	CanRepair(ctx context.Context, item DataItem) (bool, error)
	// Repair fetches a missing or corrupted item.
	Repair(ctx context.Context, item DataItem, destPath string) error
}

// ══════════════════════════════════════════════════════════════════════════════
// Processor Interface
// ══════════════════════════════════════════════════════════════════════════════

// ProcessorScope defines what a processor operates on.
type ProcessorScope string

const (
	// ProcessorScopeDataType - processor works on any data of specific types
	ProcessorScopeDataType ProcessorScope = "data_type"
	// ProcessorScopeConnector - processor is specific to data from certain connectors
	ProcessorScopeConnector ProcessorScope = "connector"
)

// Processor transforms and processes data items.
type Processor interface {
	// ID returns the unique identifier for this processor.
	ID() string
	// Name returns the human-readable name.
	Name() string
	// Description describes what this processor does.
	Description() string

	// Scope returns whether this processor is data-type or connector specific.
	Scope() ProcessorScope
	// SupportedDataTypes returns data types this processor handles (if scope is data_type).
	SupportedDataTypes() []DataType
	// SupportedConnectors returns connector IDs this processor handles (if scope is connector).
	SupportedConnectors() []string

	// ConfigSchema returns configuration options.
	ConfigSchema() []ConfigOption
	// CanProcess checks if this processor can handle the given item.
	CanProcess(item DataItem) bool
	// Process transforms a single item. Returns the processed item (may be same or new).
	Process(ctx context.Context, item DataItem, cfg ProcessorConfig) (*DataItem, error)
	// ProcessBatch transforms multiple items.
	ProcessBatch(ctx context.Context, items []DataItem, cfg ProcessorConfig, progress ProgressFunc) ([]DataItem, error)
}

// ProcessorConfig holds configuration for a processor.
type ProcessorConfig struct {
	ProcessorID string         `json:"processor_id"`
	WorkDir     string         `json:"work_dir"`
	SessionDir  string         `json:"session_dir"`
	DryRun      bool           `json:"dry_run"`
	Settings    map[string]any `json:"settings"`
}

// ══════════════════════════════════════════════════════════════════════════════
// Credentials
// ══════════════════════════════════════════════════════════════════════════════

// CredentialScope defines where credentials are stored.
type CredentialScope string

const (
	CredentialScopeGlobal  CredentialScope = "global"  // Shared across sessions
	CredentialScopeSession CredentialScope = "session" // Specific to one session
)

// Credential represents stored authentication data.
type Credential struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"` // User-friendly label
	ConnectorID string          `json:"connector_id"`
	Scope       CredentialScope `json:"scope"`
	AuthType    string          `json:"auth_type"` // "oauth", "apikey", "cookie", etc.
	// Data is encrypted at rest - contents depend on AuthType
	Data      map[string]any `json:"data"`
	CreatedAt int64          `json:"created_at"`
	UpdatedAt int64          `json:"updated_at"`
	ExpiresAt int64          `json:"expires_at,omitempty"` // 0 = no expiry
}

// ══════════════════════════════════════════════════════════════════════════════
// Progress & Events
// ══════════════════════════════════════════════════════════════════════════════

// ProgressFunc reports progress during operations.
type ProgressFunc func(done, total int)

// EventType categorizes events during operations.
type EventType string

const (
	EventTypeInfo     EventType = "info"
	EventTypeProgress EventType = "progress"
	EventTypeWarning  EventType = "warning"
	EventTypeError    EventType = "error"
	EventTypeComplete EventType = "complete"
)

// Event represents something that happened during an operation.
type Event struct {
	Type        EventType `json:"type"`
	ConnectorID string    `json:"connector_id,omitempty"`
	ProcessorID string    `json:"processor_id,omitempty"`
	Phase       string    `json:"phase"`
	Done        int       `json:"done"`
	Total       int       `json:"total"`
	Message     string    `json:"message"`
	ItemID      string    `json:"item_id,omitempty"`
	Error       error     `json:"-"`
}

// ══════════════════════════════════════════════════════════════════════════════
// Configuration Option
// ══════════════════════════════════════════════════════════════════════════════

// ConfigOption describes a configurable setting.
type ConfigOption struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"` // "bool", "string", "int", "float", "select", "path", "password"
	Default     any      `json:"default"`
	Options     []string `json:"options,omitempty"` // For "select" type
	Required    bool     `json:"required"`
	Sensitive   bool     `json:"sensitive"` // If true, value should be masked in UI
}

// ══════════════════════════════════════════════════════════════════════════════
// Statistics & Analysis
// ══════════════════════════════════════════════════════════════════════════════

// DataStats holds statistics about imported/processed data.
type DataStats struct {
	TotalItems    int              `json:"total_items"`
	ByType        map[DataType]int `json:"by_type"`
	ByConnector   map[string]int   `json:"by_connector"`
	TotalSize     int64            `json:"total_size"`
	Duplicates    int              `json:"duplicates"`
	Missing       int              `json:"missing"`        // Items that failed to import
	Repairable    int              `json:"repairable"`     // Missing items that can be repaired
	ProcessedOK   int              `json:"processed_ok"`   // Successfully processed
	ProcessedFail int              `json:"processed_fail"` // Failed processing
	Errors        []string         `json:"errors,omitempty"`
}
