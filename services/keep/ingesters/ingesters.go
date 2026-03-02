// Package ingesters provides Keep-specific data import modules.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/core/ingesters"
)

// NewKeepIngesters returns the standard set of ingesters for Google Keep Takeout.
// Detects "Keep" or "Notes" folders in Takeout exports.
func NewKeepIngesters() []core.Ingester {
	return ingesters.NewServiceIngesters(
		"keep",
		ingesters.NewServiceFolderDetector([]string{
			"Keep",
			"Notes",
			"Google Keep",
		}),
	)
}
