// Package tasks implements the Google Tasks Takeout service for revoco.
package tasks

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/tasks/ingesters"
	"github.com/fulgidus/revoco/services/tasks/processors"

	// Trigger output registration
	_ "github.com/fulgidus/revoco/services/tasks/outputs"
)

// ServiceID is the unique identifier for the Tasks service.
const ServiceID = "tasks"

// Service handles Google Tasks Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Tasks service instance.
func New() *Service {
	return &Service{
		ingesters: ingesters.NewTasksIngesters(),
		processors: []core.Processor{
			processors.NewTasksProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Google Tasks"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process Google Tasks Takeout exports - Task lists to JSON/Markdown/CSV"
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
		"tasks-json",
		"tasks-markdown",
		"tasks-csv",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{},
	}
}

// Register registers the Tasks service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
