// Package calendar implements the Google Calendar Takeout service for revoco.
package calendar

import (
	"github.com/fulgidus/revoco/services/calendar/ingesters"
	"github.com/fulgidus/revoco/services/calendar/processors"
	"github.com/fulgidus/revoco/services/core"

	// Trigger output registration
	_ "github.com/fulgidus/revoco/services/calendar/outputs"
)

// ServiceID is the unique identifier for the Calendar service.
const ServiceID = "calendar"

// Service handles Google Calendar Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Calendar service instance.
func New() *Service {
	return &Service{
		ingesters: ingesters.NewCalendarIngesters(),
		processors: []core.Processor{
			processors.NewCalendarProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Google Calendar"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process Google Calendar Takeout exports - ICS parsing, event extraction, metadata"
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
		"calendar-ics",
		"calendar-json",
		"calendar-csv",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"timezone_convert":  false,
			"exclude_cancelled": false,
		},
	}
}

// Register registers the Calendar service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
