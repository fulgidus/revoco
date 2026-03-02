// Package ingesters provides data import modules for YouTube Music Takeout.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	ingesters "github.com/fulgidus/revoco/services/core/ingesters"
)

// NewYouTubeMusicIngesters returns the standard set of ingesters for YouTube Music.
//
// Returns three ingesters with IDs:
//   - youtube-music-folder
//   - youtube-music-zip
//   - youtube-music-tgz
func NewYouTubeMusicIngesters() []core.Ingester {
	detector := ingesters.NewServiceFolderDetector([]string{
		"YouTube Music",
		"YouTube e YouTube Music",
		"YouTube and YouTube Music",
	})
	return ingesters.NewServiceIngesters("youtube-music", detector)
}

// Legacy constructors for backwards compatibility

// NewFolder creates a new folder ingester.
func NewFolder() core.Ingester {
	return NewYouTubeMusicIngesters()[0]
}

// NewZip creates a new ZIP ingester.
func NewZip() core.Ingester {
	return NewYouTubeMusicIngesters()[1]
}

// NewTGZ creates a new TGZ ingester.
func NewTGZ() core.Ingester {
	return NewYouTubeMusicIngesters()[2]
}
