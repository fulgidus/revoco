//go:build darwin

package cookies

import (
	"os"
	"path/filepath"
)

// ChromeDefaultDBPath returns the default Chrome cookies SQLite path on macOS.
func ChromeDefaultDBPath() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Cookies"),
		filepath.Join(home, "Library", "Application Support", "Chromium", "Default", "Cookies"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return candidates[0]
}
