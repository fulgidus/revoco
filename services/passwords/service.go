// Package passwords implements the Google Passwords service for revoco.
package passwords

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/passwords/ingesters"
	"github.com/fulgidus/revoco/services/passwords/processors"

	_ "github.com/fulgidus/revoco/services/passwords/outputs"
)

// ServiceID is the unique identifier for the Google Passwords service.
const ServiceID = "passwords"

// Service handles Google Passwords Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Google Passwords service instance.
func New() *Service {
	return &Service{
		ingesters: ingesters.NewPasswordsIngesters(),
		processors: []core.Processor{
			processors.NewPasswordsProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Google Passwords"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process Google Passwords Takeout - CSV password exports"
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
		"passwords-keepass-csv",
		"passwords-json",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{},
	}
}

// Register registers the Google Passwords service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
