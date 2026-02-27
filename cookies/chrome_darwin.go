//go:build darwin

package cookies

import "path/filepath"

// defaultChromeCandidates returns Chrome/Chromium cookie DB paths for macOS.
func defaultChromeCandidates(home string) []string {
	return []string{
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Cookies"),
		filepath.Join(home, "Library", "Application Support", "Chromium", "Default", "Cookies"),
		// Network subfolder (Chrome ≥ 96)
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Network", "Cookies"),
		filepath.Join(home, "Library", "Application Support", "Chromium", "Default", "Network", "Cookies"),
	}
}
