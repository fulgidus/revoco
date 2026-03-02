// Package ingesters provides Calendar Takeout ingestion modules.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/core/ingesters"
)

// NewCalendarIngesters returns the standard set of ingesters for Calendar Takeout.
// Detects "Calendar" or "Calendario" folders in Takeout archives.
func NewCalendarIngesters() []core.Ingester {
	return ingesters.NewServiceIngesters(
		"calendar",
		ingesters.NewServiceFolderDetector([]string{"Calendar", "Calendario"}),
	)
}
