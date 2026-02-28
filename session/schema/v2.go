package schema

import (
	"time"
)

// ConfigV2 is the connector-based session configuration.
// This is the current/target schema version.
type ConfigV2 struct {
	Name               string           `json:"name"`
	Created            time.Time        `json:"created"`
	Updated            time.Time        `json:"updated"`
	Source             SourceV2         `json:"source"`
	OutputDir          string           `json:"output_dir"`
	UseMove            bool             `json:"use_move"`
	DryRun             bool             `json:"dry_run"`
	Recover            RecoverSettings  `json:"recover"`
	Status             string           `json:"status"`
	LastPhaseCompleted int              `json:"last_phase_completed"`
	LastError          string           `json:"last_error,omitempty"`
	Version            int              `json:"version"` // Always 2 for this schema
	Connectors         ConnectorsConfig `json:"connectors"`
	Pipeline           *PipelineConfig  `json:"pipeline,omitempty"` // Kept for reference during migration
}

// SourceV2 describes the input data for a v2 session.
// In v2, source is mostly managed by connectors, but we keep this for backwards compat.
type SourceV2 struct {
	Type         string `json:"type,omitempty"`
	OriginalPath string `json:"original_path,omitempty"`
	ImportedPath string `json:"imported_path,omitempty"`
}

// ConnectorsConfig holds the connector-based configuration.
type ConnectorsConfig struct {
	Connectors        []ConnectorConfigV2 `json:"connectors"`
	ProcessorConfigs  []ProcessorConfigV2 `json:"processor_configs,omitempty"`
	AutoProcess       bool                `json:"auto_process"`
	ParallelRetrieval bool                `json:"parallel_retrieval"`
	DetectedDataTypes []string            `json:"detected_data_types,omitempty"`
	Stats             *DataStats          `json:"stats,omitempty"`
}

// ConnectorRolesV2 describes how a connector instance is used in a session.
// A connector can have any combination of roles.
type ConnectorRolesV2 struct {
	IsInput    bool `json:"is_input"`    // Primary data source
	IsOutput   bool `json:"is_output"`   // Primary data destination
	IsFallback bool `json:"is_fallback"` // Used for repair/missing data
}

// ConnectorConfigV2 holds the configuration for a connector instance.
type ConnectorConfigV2 struct {
	ConnectorID  string           `json:"connector_id"`
	InstanceID   string           `json:"instance_id"`
	Name         string           `json:"name"`
	Roles        ConnectorRolesV2 `json:"roles"`
	ImportMode   string           `json:"import_mode,omitempty"`
	Settings     map[string]any   `json:"settings"`
	CredentialID string           `json:"credential_id,omitempty"`
	FallbackFor  []string         `json:"fallback_for,omitempty"`
	Enabled      bool             `json:"enabled"`
}

// ProcessorConfigV2 holds configuration for a processor.
type ProcessorConfigV2 struct {
	ProcessorID string         `json:"processor_id"`
	WorkDir     string         `json:"work_dir"`
	SessionDir  string         `json:"session_dir"`
	DryRun      bool           `json:"dry_run"`
	Settings    map[string]any `json:"settings"`
}

// DataStats holds statistics about imported/processed data.
type DataStats struct {
	TotalItems    int            `json:"total_items"`
	ByType        map[string]int `json:"by_type"`
	ByConnector   map[string]int `json:"by_connector"`
	TotalSize     int64          `json:"total_size"`
	Duplicates    int            `json:"duplicates"`
	Missing       int            `json:"missing"`
	Repairable    int            `json:"repairable"`
	ProcessedOK   int            `json:"processed_ok"`
	ProcessedFail int            `json:"processed_fail"`
	Errors        []string       `json:"errors,omitempty"`
}
