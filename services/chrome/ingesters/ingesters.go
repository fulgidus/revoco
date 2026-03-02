// Package ingesters provides data ingestion modules for Chrome Takeout.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	ingesters "github.com/fulgidus/revoco/services/core/ingesters"
)

// NewChromeIngesters returns the standard ingesters for Chrome Takeout data.
// Detects "Chrome" directory and .json/.html file extensions.
func NewChromeIngesters() []core.Ingester {
	// Create folder detector for Chrome Takeout directories
	detector := ingesters.NewServiceFolderDetector([]string{
		"Chrome",
	})

	// Return standard ingester set (folder, zip, tgz)
	return ingesters.NewServiceIngesters("chrome", detector)
}
