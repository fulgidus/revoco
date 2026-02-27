//go:build windows

package cookies

import "path/filepath"

// defaultChromeCandidates returns Chrome/Chromium cookie DB paths for Windows.
func defaultChromeCandidates(home string) []string {
	localAppData := filepath.Join(home, "AppData", "Local")
	return []string{
		filepath.Join(localAppData, "Google", "Chrome", "User Data", "Default", "Network", "Cookies"),
		filepath.Join(localAppData, "Google", "Chrome", "User Data", "Default", "Cookies"),
		filepath.Join(localAppData, "Chromium", "User Data", "Default", "Network", "Cookies"),
		filepath.Join(localAppData, "Chromium", "User Data", "Default", "Cookies"),
	}
}
