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
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Google Maps service instance.
func New() *Service {
	return &Service{
		ingesters: ingesters.NewMapsIngesters(),
		processors: []core.Processor{
			processors.NewMapsProcessor(),
		},
	}
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
	return "Process Google Maps Takeout - location history, saved places, and timeline data"
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
		"maps-geojson",
		"maps-kml",
		"maps-json",
		"maps-csv",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"coordinate_precision": 6,
			"include_timeline":     true,
			"min_accuracy":         0,
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
