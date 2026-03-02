// Package ingesters provides data import modules for Google Photos Takeout.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	ingesters "github.com/fulgidus/revoco/services/core/ingesters"
)

// NewGooglePhotosIngesters returns the standard set of ingesters for Google Photos.
//
// Returns three ingesters with IDs:
//   - google-photos-folder
//   - google-photos-zip
//   - google-photos-tgz
func NewGooglePhotosIngesters() []core.Ingester {
	detector := ingesters.NewServiceFolderDetector([]string{
		"Google Photos",
		"Google Foto",   // Italian
		"Google Fotos",  // Spanish/Portuguese
		"Googleフォト",    // Japanese
	})
	return ingesters.NewServiceIngesters("google-photos", detector)
}

// Legacy constructors for backwards compatibility

// NewFolder creates a new folder ingester.
func NewFolder() core.Ingester {
	return NewGooglePhotosIngesters()[0]
}

// NewZip creates a new ZIP ingester.
func NewZip() core.Ingester {
	return NewGooglePhotosIngesters()[1]
}

// NewTGZ creates a new TGZ ingester.
func NewTGZ() core.Ingester {
	return NewGooglePhotosIngesters()[2]
}
