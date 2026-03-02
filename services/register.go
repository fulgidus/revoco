// Package services provides the service registration entry point.
// Import this package to register all built-in services, outputs, and auth providers.
package services

import (
	// Import services to trigger their init() registration
	_ "github.com/fulgidus/revoco/services/googlephotos"
	_ "github.com/fulgidus/revoco/services/youtubemusic"
	_ "github.com/fulgidus/revoco/services/gmail"
	_ "github.com/fulgidus/revoco/services/keep"
	_ "github.com/fulgidus/revoco/services/contacts"
	_ "github.com/fulgidus/revoco/services/calendar"
	_ "github.com/fulgidus/revoco/services/tasks"

	// Import common outputs to trigger their init() registration
	_ "github.com/fulgidus/revoco/outputs/common"
)

// Init is a no-op function that can be called to ensure this package is imported.
// All registration happens via init() functions in the imported packages.
func Init() {
	// Registration happens automatically via init() functions
}
