// Package core defines the interfaces for revoco's modular service architecture.
//
// The architecture consists of:
//   - Services: Top-level handlers for specific data sources (Google Photos, YouTube Music, etc.)
//   - Ingesters: Import data from various formats (folder, zip, tgz, API, etc.)
//   - Processors: Transform and process data with configurable behaviors
//   - Outputs: Export processed data to various destinations (local, cloud, API, etc.)
//
// Each service implements its own pipeline with service-specific stages.
package core

import (
	"context"
	"io"
)

// ProgressFunc is called to report progress during long operations.
type ProgressFunc func(done, total int)

// ProgressEvent is emitted during pipeline execution.
type ProgressEvent struct {
	Phase   int
	Label   string
	Done    int
	Total   int
	Message string
}

// Service represents a data source handler (e.g., Google Photos, YouTube Music).
type Service interface {
	// ID returns the unique identifier for this service.
	ID() string
	// Name returns the human-readable name for display.
	Name() string
	// Description returns a brief description of what this service handles.
	Description() string
	// Ingesters returns all available ingestion modules for this service.
	Ingesters() []Ingester
	// Processors returns all available processors for this service.
	Processors() []Processor
	// SupportedOutputs returns the IDs of compatible output modules.
	SupportedOutputs() []string
	// DefaultConfig returns the default configuration for this service.
	DefaultConfig() ServiceConfig
}

// ServiceConfig holds configurable options for a service.
type ServiceConfig struct {
	// Service-specific settings stored as a map for flexibility
	Settings map[string]any `json:"settings"`
}

// Ingester imports data from a specific format or source.
type Ingester interface {
	// ID returns the unique identifier for this ingester.
	ID() string
	// Name returns the human-readable name for display.
	Name() string
	// Description returns a brief description of this ingester.
	Description() string
	// SupportedExtensions returns file extensions this ingester handles (e.g., ".zip", ".tgz").
	// For folder ingesters, this returns an empty slice.
	SupportedExtensions() []string
	// CanIngest checks if the given path can be ingested by this module.
	CanIngest(path string) bool
	// Ingest imports data from the source path to the destination directory.
	// Returns the path where data was extracted/copied to.
	Ingest(ctx context.Context, sourcePath, destDir string, progress ProgressFunc) (string, error)
}

// Processor transforms and processes ingested data.
type Processor interface {
	// ID returns the unique identifier for this processor.
	ID() string
	// Name returns the human-readable name for display.
	Name() string
	// Description returns a brief description of this processor.
	Description() string
	// ConfigSchema returns the configuration schema for this processor.
	ConfigSchema() []ConfigOption
	// Process runs the processing pipeline on the data.
	Process(ctx context.Context, cfg ProcessorConfig, events chan<- ProgressEvent) (*ProcessResult, error)
}

// ProcessorConfig holds configuration for a processing run.
type ProcessorConfig struct {
	SourceDir  string         `json:"source_dir"`
	WorkDir    string         `json:"work_dir"`    // Working directory for intermediate files
	SessionDir string         `json:"session_dir"` // Session directory for logs
	DryRun     bool           `json:"dry_run"`
	Settings   map[string]any `json:"settings"` // Processor-specific settings
}

// ProcessResult holds the output of a processing run.
type ProcessResult struct {
	// ProcessedItems contains items ready for output
	Items []ProcessedItem
	// Stats contains processing statistics
	Stats map[string]int
	// Metadata contains service-specific metadata
	Metadata map[string]any
	// LogPath is the path to the processing log
	LogPath string
}

// ProcessedItem represents a single processed item ready for output.
type ProcessedItem struct {
	// SourcePath is the original source file path
	SourcePath string
	// ProcessedPath is the path after processing (may be same as source)
	ProcessedPath string
	// DestRelPath is the suggested relative path for output
	DestRelPath string
	// Metadata contains item-specific metadata
	Metadata map[string]any
	// Type indicates the item type (e.g., "photo", "video", "playlist", "track")
	Type string
}

// ConfigOption describes a configurable option for a processor or output.
type ConfigOption struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"` // "bool", "string", "int", "select"
	Default     any      `json:"default"`
	Options     []string `json:"options,omitempty"` // For "select" type
	Required    bool     `json:"required"`
}

// Output exports processed data to a destination.
type Output interface {
	// ID returns the unique identifier for this output.
	ID() string
	// Name returns the human-readable name for display.
	Name() string
	// Description returns a brief description of this output.
	Description() string
	// ConfigSchema returns the configuration schema for this output.
	ConfigSchema() []ConfigOption
	// SupportedItemTypes returns the item types this output can handle.
	SupportedItemTypes() []string
	// Initialize prepares the output for use (e.g., authentication, connection).
	Initialize(ctx context.Context, cfg OutputConfig) error
	// Export sends a single item to the destination.
	Export(ctx context.Context, item ProcessedItem) error
	// ExportBatch sends multiple items to the destination.
	ExportBatch(ctx context.Context, items []ProcessedItem, progress ProgressFunc) error
	// Finalize completes the export process (e.g., cleanup, finalization).
	Finalize(ctx context.Context) error
}

// OutputConfig holds configuration for an output module.
type OutputConfig struct {
	DestDir  string         `json:"dest_dir"`
	Settings map[string]any `json:"settings"`
}

// StreamingOutput extends Output with streaming capabilities for cross-provider transfers.
type StreamingOutput interface {
	Output
	// SupportsStreaming returns true if this output can receive streamed data.
	SupportsStreaming() bool
	// ExportStream writes streamed data directly to the destination.
	ExportStream(ctx context.Context, item ProcessedItem, reader io.Reader, size int64) error
}

// AuthProvider handles authentication for outputs that require it.
type AuthProvider interface {
	// ID returns the unique identifier for this auth provider.
	ID() string
	// Name returns the human-readable name.
	Name() string
	// AuthType returns the authentication type (e.g., "oauth", "apikey", "cookie").
	AuthType() string
	// Authenticate performs the authentication flow.
	Authenticate(ctx context.Context) error
	// IsAuthenticated checks if valid credentials exist.
	IsAuthenticated() bool
	// GetCredentials returns the current credentials.
	GetCredentials() (any, error)
	// ClearCredentials removes stored credentials.
	ClearCredentials() error
}
