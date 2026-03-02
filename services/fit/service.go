// Package fit implements the Google Fit service for revoco.
package fit

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/fit/ingesters"
	"github.com/fulgidus/revoco/services/fit/processors"

	_ "github.com/fulgidus/revoco/services/fit/outputs"
)

// ServiceID is the unique identifier for the Google Fit service.
const ServiceID = "fit"

// Service handles Google Fit Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Google Fit service instance.
func New() *Service {
	return &Service{
		ingesters: ingesters.NewFitIngesters(),
		processors: []core.Processor{
			processors.NewFitProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Google Fit"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process Google Fit Takeout - daily aggregations and activity sessions"
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
		"fit-json",
		"fit-csv",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"include_activities": true,
		},
	}
}

// Register registers the Google Fit service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
