// Package keep implements the Google Keep Takeout service for revoco.
package keep

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/keep/ingesters"
	"github.com/fulgidus/revoco/services/keep/processors"

	// Trigger output registration
	_ "github.com/fulgidus/revoco/services/keep/outputs"
)

// ServiceID is the unique identifier for the Keep service.
const ServiceID = "keep"

// Service handles Google Keep Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Keep service instance.
func New() *Service {
	return &Service{
		ingesters: ingesters.NewKeepIngesters(),
		processors: []core.Processor{
			processors.NewKeepProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Google Keep"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process Google Keep Takeout exports - Notes to Markdown/JSON/HTML"
}

// Ingesters returns all available ingestion modules for this service.
func (s *Service) Ingesters() []core.Ingester {
	return s.ingesters
}

// Processors returns all available processors for this service.
func (s *Service) Processors() []core.Processor {
	return s.processors
}

// SupportedOutputs returns the IDs of compatible output modules.
func (s *Service) SupportedOutputs() []string {
	return []string{
		"local-folder",
		"keep-md",
		"keep-json",
		"keep-html",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"preserve_colors":  true,
			"export_archived":  true,
			"markdown_format":  true,
			"include_labels":   true,
			"timestamp_format": "2006-01-02",
		},
	}
}

// Register registers the Keep service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
