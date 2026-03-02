// Package ingesters provides data ingestion modules for Gmail Takeout.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	ingesters "github.com/fulgidus/revoco/services/core/ingesters"
)

// NewGmailIngesters returns the standard ingesters for Gmail Takeout data.
// Detects "Mail" or "Posta" directories and .mbox file extensions.
func NewGmailIngesters() []core.Ingester {
	// Create folder detector for Gmail Takeout directories
	detector := ingesters.NewServiceFolderDetector([]string{
		"Mail",  // English
		"Posta", // Italian
	})

	// Return standard ingester set (folder, zip, tgz)
	return ingesters.NewServiceIngesters("gmail", detector)
}
