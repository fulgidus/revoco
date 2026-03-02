// Package chrome implements the Chrome Takeout service for revoco.
package chrome

import (
	"github.com/fulgidus/revoco/services/chrome/ingesters"
	"github.com/fulgidus/revoco/services/chrome/processors"
	"github.com/fulgidus/revoco/services/core"

	// Trigger output registration
	_ "github.com/fulgidus/revoco/services/chrome/outputs"
)

// ServiceID is the unique identifier for the Chrome service.
const ServiceID = "chrome"

// Service handles Chrome Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Chrome service instance.
func New() *Service {
	return &Service{
		ingesters: ingesters.NewChromeIngesters(),
		processors: []core.Processor{
			processors.NewChromeProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Chrome"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process Chrome Takeout exports - bookmarks, browser history, search engines, and autofill"
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
		"chrome-json",
		"chrome-html",
		"chrome-csv",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"include_search_engines": false,
			"include_autofill":       false,
		},
	}
}

// Register registers the Chrome service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
