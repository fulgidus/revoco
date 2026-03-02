// Package maps implements the Google Maps service for revoco.
package maps

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/maps/ingesters"
	"github.com/fulgidus/revoco/services/maps/processors"

	_ "github.com/fulgidus/revoco/services/maps/outputs"
)

// ServiceID is the unique identifier for the Google Maps service.
const ServiceID = "maps"

// Service handles Google Maps Takeout data.
type Service struct{}

// New creates a new Google Maps service instance.
func New() *Service {
	return &Service{}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Google Maps"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Google Maps Takeout processor - location history and saved places"
}

// Ingesters returns all available ingestion modules for this service.
func (s *Service) Ingesters() []core.Ingester {
	return ingesters.NewMapsIngesters()
}

// Processors returns all available processors for this service.
func (s *Service) Processors() []core.Processor {
	return []core.Processor{processors.NewMapsProcessor()}
}

// SupportedOutputs returns the IDs of compatible output modules.
func (s *Service) SupportedOutputs() []string {
	return []string{"local-folder"}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"include_location_history": true,
			"include_saved_places":     true,
			"include_timeline":         true,
		},
	}
}

// Register registers the Google Maps service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
