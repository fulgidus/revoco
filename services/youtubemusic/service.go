// Package youtubemusic implements the YouTube Music Takeout service for revoco.
package youtubemusic

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/youtubemusic/ingesters"
	"github.com/fulgidus/revoco/services/youtubemusic/processors"
)

// ServiceID is the unique identifier for the YouTube Music service.
const ServiceID = "youtube-music"

// Service handles YouTube Music Takeout data.
type Service struct {
	ingesters  []core.Ingester
	processors []core.Processor
}

// New creates a new YouTube Music service instance.
func New() *Service {
	return &Service{
		ingesters: []core.Ingester{
			ingesters.NewFolder(),
			ingesters.NewZip(),
			ingesters.NewTGZ(),
		},
		processors: []core.Processor{
			processors.NewMusicProcessor(),
		},
	}
}

// ID returns the unique identifier for this service.
func (s *Service) ID() string {
	return ServiceID
}

// Name returns the human-readable name for display.
func (s *Service) Name() string {
	return "YouTube Music"
}

// Description returns a brief description of what this service handles.
func (s *Service) Description() string {
	return "Process YouTube Music Takeout exports - playlists, liked songs, uploads, subscriptions"
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
		"ytmusic-json",
		"ytmusic-csv",
		"ytmusic-m3u",
		"ytmusic-spotify",
	}
}

// DefaultConfig returns the default configuration for this service.
func (s *Service) DefaultConfig() core.ServiceConfig {
	return core.ServiceConfig{
		Settings: map[string]any{
			"include_playlists":     true,
			"include_liked_songs":   true,
			"include_uploads":       true,
			"include_subscriptions": true,
			"local_library_path":    "",
		},
	}
}

// Register registers the YouTube Music service and its components.
func Register() error {
	return core.RegisterService(New())
}

func init() {
	// Auto-register on import
	_ = Register()
}
