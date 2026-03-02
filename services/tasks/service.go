// Package tasks implements the Google Tasks service for revoco.
package tasks

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/tasks/ingesters"
	"github.com/fulgidus/revoco/services/tasks/processors"

	_ "github.com/fulgidus/revoco/services/tasks/outputs"
)

// ServiceID is the unique identifier for the Google Tasks service.
const ServiceID = "tasks"

// Service handles Google Tasks Takeout data.
type Service struct{}

// New creates a new Google Tasks service instance.
func New() *Service {
	return &Service{}
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
	return "Google Tasks Takeout processor"
}

// Ingesters returns all available ingestion modules for this service.
func (s *Service) Ingesters() []core.Ingester {
	return ingesters.NewTasksIngesters()
}

// Processors returns all available processors for this service.
func (s *Service) Processors() []core.Processor {
	return []core.Processor{processors.NewTasksProcessor()}
}

// SupportedOutputs returns the IDs of compatible output modules.
func (s *Service) SupportedOutputs() []string {
	return []string{"local-folder"}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"include_tasks": true,
			"include_lists": true,
		},
	}
}

// Register registers the Google Tasks service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
