// Package ingesters provides reusable ingesters for the Contacts service.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/core/ingesters"
)

// NewContactsIngesters returns the standard set of ingesters for Google Contacts Takeout.
func NewContactsIngesters() []core.Ingester {
	// Detect "Contacts" or "Contatti" folders in Takeout
	detector := ingesters.NewServiceFolderDetector([]string{
		"Contacts",
		"Contatti",  // Italian
		"Contactos", // Spanish
		"Kontakte",  // German
	})

	return ingesters.NewServiceIngesters("contacts", detector)
}
