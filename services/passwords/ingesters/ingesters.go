// Package ingesters provides data ingestion modules for Google Passwords.
package ingesters

import (
	"github.com/fulgidus/revoco/services/core"
	ingesters "github.com/fulgidus/revoco/services/core/ingesters"
)

// NewPasswordsIngesters returns the standard ingesters for Google Passwords CSV data.
// Detects "Chrome" or "Passwords" directory with .csv file extensions.
func NewPasswordsIngesters() []core.Ingester {
	// Create folder detector for Passwords directories
	// Google exports passwords in Chrome folder or sometimes a dedicated Passwords folder
	detector := ingesters.NewServiceFolderDetector([]string{
		"Chrome",
		"Passwords",
		"Password",
	})

	// Return standard ingester set (folder, zip, tgz)
	return ingesters.NewServiceIngesters("passwords", detector)
}
