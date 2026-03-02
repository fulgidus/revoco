// Package ingesters provides data ingestion modules for Google Maps Takeout.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	ingesters "github.com/fulgidus/revoco/services/core/ingesters"
)

// NewMapsIngesters returns the standard ingesters for Maps Takeout data.
// Detects "Maps", "Maps (I tuoi luoghi)", "Location History", or "Cronologia delle posizioni" directories.
func NewMapsIngesters() []core.Ingester {
	// Create folder detector for Maps Takeout directories
	detector := ingesters.NewServiceFolderDetector([]string{
		"Maps",                                 // English (saved places)
		"Maps (I tuoi luoghi)",                 // Italian (saved places)
		"Location History",                     // English (location history)
		"Cronologia delle posizioni",           // Italian (location history)
		"Semantic Location History",            // English (timeline data)
		"Cronologia delle posizioni semantica", // Italian (timeline data)
	})

	// Return standard ingester set (folder, zip, tgz)
	return ingesters.NewServiceIngesters("maps", detector)
}
