// Package googlephotos implements the Google Photos Takeout service for revoco.
package googlephotos

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/googlephotos/ingesters"
	"github.com/fulgidus/revoco/services/googlephotos/processors"
)

// ServiceID is the unique identifier for the Google Photos service.
const ServiceID = "google-photos"

// Service handles Google Photos Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new Google Photos service instance.
func New() *Service {
	return &Service{
		ingesters: []core.Ingester{
			ingesters.NewFolder(),
			ingesters.NewZip(),
			ingesters.NewTGZ(),
		},
		processors: []core.Processor{
			processors.NewPhotosProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "Google Photos"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process Google Photos Takeout exports - organize photos, apply metadata, convert motion photos, and export to various destinations"
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
		"immich",
		"photoprism",
		"s3",
		"google-photos-api",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"exif_embedding":          true,
			"album_organization":      true,
			"deduplication":           true,
			"motion_photo_conversion": true,
			"use_move":                false,
		},
	}
}

// Register registers the Google Photos service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
