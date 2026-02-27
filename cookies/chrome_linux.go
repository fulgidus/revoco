//go:build linux

package cookies

import "path/filepath"

// defaultChromeCandidates returns Chrome/Chromium cookie DB paths for Linux.
func defaultChromeCandidates(home string) []string {
	return []string{
		filepath.Join(home, ".config", "google-chrome", "Default", "Cookies"),
		filepath.Join(home, ".config", "chromium", "Default", "Cookies"),
		filepath.Join(home, ".config", "google-chrome-stable", "Default", "Cookies"),
		filepath.Join(home, "snap", "chromium", "current", ".config", "chromium", "Default", "Cookies"),
		// Network subfolder (Chrome ≥ 96)
		filepath.Join(home, ".config", "google-chrome", "Default", "Network", "Cookies"),
		filepath.Join(home, ".config", "chromium", "Default", "Network", "Cookies"),
	}
}
