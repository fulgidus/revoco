//go:build linux

package cookies

import (
	"os"
	"path/filepath"
)

// ChromeDefaultDBPath returns the default Chrome cookies SQLite path on Linux.
func ChromeDefaultDBPath() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".config", "google-chrome", "Default", "Cookies"),
		filepath.Join(home, ".config", "chromium", "Default", "Cookies"),
		filepath.Join(home, ".config", "google-chrome-beta", "Default", "Cookies"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return candidates[0]
}
