//go:build darwin

package cookies

import (
	"fmt"
	"os/exec"
	"strings"
)

// GetChromeStoragePassword retrieves the Chrome Safe Storage password from
// the macOS Keychain using the `security` CLI.
func GetChromeStoragePassword() (string, error) {
	out, err := exec.Command(
		"security", "find-generic-password",
		"-s", "Chrome Safe Storage",
		"-a", "Chrome",
		"-w",
	).Output()
	if err != nil {
		// Try Chromium variant
		out2, err2 := exec.Command(
			"security", "find-generic-password",
			"-s", "Chromium Safe Storage",
			"-a", "Chromium",
			"-w",
		).Output()
		if err2 != nil {
			return "", fmt.Errorf("keychain lookup failed: %w (chromium: %v)", err, err2)
		}
		out = out2
	}
	return strings.TrimSpace(string(out)), nil
}
