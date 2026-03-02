// Package ingesters provides data import modules for Google Fit Takeout.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	coreingesters "github.com/fulgidus/revoco/services/core/ingesters"
)

// NewFitIngesters returns the standard set of ingesters for Google Fit data.
func NewFitIngesters() []core.Ingester {
	// Create detection function for Fit folder variants
	detector := coreingesters.NewServiceFolderDetector([]string{
		"Fit",
		"Google Fit",
		"Fitness",
	})

	return coreingesters.NewServiceIngesters("fit", detector)
}
