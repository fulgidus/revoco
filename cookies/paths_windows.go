//go:build windows

package cookies

import (
	"os"
	"path/filepath"
)

// ChromeDefaultDBPath returns the default Chrome cookies SQLite path on Windows.
func ChromeDefaultDBPath() string {
	local := os.Getenv("LOCALAPPDATA")
	candidates := []string{
		filepath.Join(local, "Google", "Chrome", "User Data", "Default", "Cookies"),
		filepath.Join(local, "Google", "Chrome", "User Data", "Default", "Network", "Cookies"),
		filepath.Join(local, "Chromium", "User Data", "Default", "Cookies"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return candidates[0]
}

// ChromeLocalStatePath returns the path to Chrome's Local State file (needed for v20 key).
func ChromeLocalStatePath() string {
	local := os.Getenv("LOCALAPPDATA")
	return filepath.Join(local, "Google", "Chrome", "User Data", "Local State")
}
