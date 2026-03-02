// Package contacts implements the Google Contacts Takeout service for revoco.
package contacts

import (
	"github.com/fulgidus/revoco/services/contacts/ingesters"
	"github.com/fulgidus/revoco/services/contacts/processors"
	"github.com/fulgidus/revoco/services/core"

	// Trigger output registration
	_ "github.com/fulgidus/revoco/services/contacts/outputs"
)

// ServiceID is the unique identifier for the Contacts service.
const ServiceID = "contacts"

// Service handles Google Contacts Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Contacts service instance.
func New() *Service {
	return &Service{
		ingesters: ingesters.NewContactsIngesters(),
		processors: []core.Processor{
			processors.NewContactsProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Google Contacts"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process Google Contacts Takeout exports - vCard (.vcf) parsing with metadata extraction"
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
		"contacts-vcf",
		"contacts-json",
		"contacts-csv",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"merge_duplicates": false,
			"extract_photos":   false,
		},
	}
}

// Register registers the Contacts service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
