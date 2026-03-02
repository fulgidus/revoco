// Package gmail implements the Gmail Takeout service for revoco.
package gmail

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/gmail/ingesters"
	"github.com/fulgidus/revoco/services/gmail/processors"

	// Trigger output registration
	_ "github.com/fulgidus/revoco/services/gmail/outputs"
)

// ServiceID is the unique identifier for the Gmail service.
const ServiceID = "gmail"

// Service handles Gmail Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Gmail service instance.
func New() *Service {
	return &Service{
		ingesters: ingesters.NewGmailIngesters(),
		processors: []core.Processor{
			processors.NewGmailProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Gmail"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process Gmail Takeout exports - MBOX to .eml extraction with metadata"
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
		"gmail-json",
		"gmail-eml",
		"gmail-csv",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"extract_attachments": false,
			"body_preview_length": 200,
		},
	}
}

// Register registers the Gmail service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
