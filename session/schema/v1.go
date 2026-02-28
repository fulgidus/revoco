package schema

import (
	"time"
)

// ConfigV1 is the legacy pipeline-based session configuration.
// This is used for parsing and migration purposes only.
type ConfigV1 struct {
	Name               string          `json:"name"`
	Created            time.Time       `json:"created"`
	Updated            time.Time       `json:"updated"`
	Source             SourceV1        `json:"source"`
	OutputDir          string          `json:"output_dir"`
	UseMove            bool            `json:"use_move"`
	DryRun             bool            `json:"dry_run"`
	Recover            RecoverSettings `json:"recover"`
	Status             string          `json:"status"`
	LastPhaseCompleted int             `json:"last_phase_completed"`
	LastError          string          `json:"last_error,omitempty"`
	Pipeline           PipelineConfig  `json:"pipeline,omitempty"`
}

// SourceV1 describes the input data for a v1 session.
type SourceV1 struct {
	Type         string `json:"type"`          // "folder", "zip", "tgz"
	OriginalPath string `json:"original_path"` // path the user provided
	ImportedPath string `json:"imported_path"` // path inside session (if imported)
}

// RecoverSettings holds recovery-specific configuration (same for both versions).
type RecoverSettings struct {
	InputJSON   string  `json:"input_json"`
	OutputDir   string  `json:"output_dir"`
	Concurrency int     `json:"concurrency"`
	Delay       float64 `json:"delay"`
	MaxRetry    int     `json:"max_retry"`
	StartFrom   int     `json:"start_from"`
}

// PipelineConfig is the v1 service-based pipeline configuration.
type PipelineConfig struct {
	ServiceID         string            `json:"service_id"`
	IngesterID        string            `json:"ingester_id"`
	ProcessorSettings map[string]any    `json:"processor_settings"`
	OutputSettings    []OutputSettingV1 `json:"output_settings"`
}

// OutputSettingV1 is a v1 output configuration.
type OutputSettingV1 struct {
	OutputID string         `json:"output_id"`
	Config   map[string]any `json:"config"`
}
