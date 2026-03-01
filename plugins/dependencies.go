package plugins

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// DependencyChecker verifies binary dependencies for plugins.
type DependencyChecker struct {
	// Cache of checked binaries
	cache map[string]*DependencyStatus
}

// DependencyStatus holds the status of a binary dependency.
type DependencyStatus struct {
	Binary    string
	Found     bool
	Path      string
	Version   string
	MeetsMin  bool
	MinNeeded string
	Error     string
}

// NewDependencyChecker creates a new dependency checker.
func NewDependencyChecker() *DependencyChecker {
	return &DependencyChecker{
		cache: make(map[string]*DependencyStatus),
	}
}

// Check verifies a single dependency.
func (c *DependencyChecker) Check(dep BinaryDependency) *DependencyStatus {
	// Check cache
	if status, ok := c.cache[dep.Binary]; ok {
		// Update min version check if different
		if dep.MinVersion != "" && status.Version != "" {
			status.MinNeeded = dep.MinVersion
			status.MeetsMin = c.versionMeetsMin(status.Version, dep.MinVersion)
		}
		return status
	}

	status := &DependencyStatus{
		Binary:    dep.Binary,
		MinNeeded: dep.MinVersion,
	}

	// Find binary in PATH
	path, err := exec.LookPath(dep.Binary)
	if err != nil {
		status.Found = false
		status.Error = fmt.Sprintf("binary not found in PATH: %s", dep.Binary)
		c.cache[dep.Binary] = status
		return status
	}

	status.Found = true
	status.Path = path

	// Get version if check command provided
	if dep.Check != "" {
		version, err := c.getVersion(dep.Check, dep.VersionRegex)
		if err != nil {
			status.Error = fmt.Sprintf("failed to get version: %v", err)
		} else {
			status.Version = version
			if dep.MinVersion != "" {
				status.MeetsMin = c.versionMeetsMin(version, dep.MinVersion)
			} else {
				status.MeetsMin = true
			}
		}
	} else {
		status.MeetsMin = true
	}

	c.cache[dep.Binary] = status
	return status
}

// CheckAll verifies all dependencies for a plugin.
func (c *DependencyChecker) CheckAll(deps []BinaryDependency) []*DependencyStatus {
	results := make([]*DependencyStatus, len(deps))
	for i, dep := range deps {
		results[i] = c.Check(dep)
	}
	return results
}

// AllSatisfied returns true if all dependencies are satisfied.
func (c *DependencyChecker) AllSatisfied(deps []BinaryDependency) bool {
	for _, dep := range deps {
		status := c.Check(dep)
		if !status.Found || !status.MeetsMin {
			return false
		}
	}
	return true
}

// MissingDependencies returns all unsatisfied dependencies.
func (c *DependencyChecker) MissingDependencies(deps []BinaryDependency) []BinaryDependency {
	var missing []BinaryDependency
	for _, dep := range deps {
		status := c.Check(dep)
		if !status.Found || !status.MeetsMin {
			missing = append(missing, dep)
		}
	}
	return missing
}

// getVersion runs the check command and extracts the version.
func (c *DependencyChecker) getVersion(checkCmd, versionRegex string) (string, error) {
	parts := strings.Fields(checkCmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty check command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Ignore exit code - some tools output version to stderr
	_ = cmd.Run()

	output := stdout.String() + stderr.String()

	if versionRegex != "" {
		re, err := regexp.Compile(versionRegex)
		if err != nil {
			return "", fmt.Errorf("invalid version regex: %w", err)
		}

		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			return matches[1], nil
		} else if len(matches) == 1 {
			return matches[0], nil
		}
		return "", fmt.Errorf("version regex did not match output")
	}

	// Try common patterns
	patterns := []string{
		`(\d+\.\d+\.\d+)`,           // 1.2.3
		`(\d+\.\d+)`,                // 1.2
		`version\s+(\d+\.\d+\.\d+)`, // version 1.2.3
		`v(\d+\.\d+\.\d+)`,          // v1.2.3
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("could not extract version from output")
}

// versionMeetsMin checks if version >= minVersion.
func (c *DependencyChecker) versionMeetsMin(version, minVersion string) bool {
	// Try semver comparison
	v, err := semver.NewVersion(version)
	if err != nil {
		// Fallback to string comparison
		return version >= minVersion
	}

	min, err := semver.NewVersion(minVersion)
	if err != nil {
		return version >= minVersion
	}

	return v.Compare(min) >= 0
}

// ClearCache clears the dependency cache.
func (c *DependencyChecker) ClearCache() {
	c.cache = make(map[string]*DependencyStatus)
}

// ══════════════════════════════════════════════════════════════════════════════
// Installation Helpers
// ══════════════════════════════════════════════════════════════════════════════

// GetInstallCommand returns the install command for the current OS/package manager.
func GetInstallCommand(dep BinaryDependency) (string, string) {
	// Detect package manager
	pm := detectPackageManager()

	if cmd, ok := dep.Install[pm]; ok {
		return pm, cmd
	}

	// Fallback to manual
	if manual, ok := dep.Install["manual"]; ok {
		return "manual", manual
	}

	return "", ""
}

// detectPackageManager returns the likely package manager for this system.
func detectPackageManager() string {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("brew"); err == nil {
			return "brew"
		}
		return "manual"

	case "linux":
		// Check common package managers
		managers := []string{"apt", "dnf", "yum", "pacman", "zypper", "apk"}
		for _, pm := range managers {
			if _, err := exec.LookPath(pm); err == nil {
				return pm
			}
		}
		return "manual"

	case "windows":
		if _, err := exec.LookPath("choco"); err == nil {
			return "choco"
		}
		if _, err := exec.LookPath("scoop"); err == nil {
			return "scoop"
		}
		if _, err := exec.LookPath("winget"); err == nil {
			return "winget"
		}
		return "manual"

	default:
		return "manual"
	}
}

// InstallDependency attempts to install a dependency.
func InstallDependency(dep BinaryDependency) error {
	pm, cmd := GetInstallCommand(dep)
	if pm == "" || pm == "manual" {
		return fmt.Errorf("no automatic installation available for %s", dep.Binary)
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return fmt.Errorf("empty install command for %s", dep.Binary)
	}

	execCmd := exec.Command(parts[0], parts[1:]...)
	execCmd.Stdout = nil // Let output go to terminal
	execCmd.Stderr = nil

	return execCmd.Run()
}

// FormatInstallInstructions returns human-readable installation instructions.
func FormatInstallInstructions(dep BinaryDependency) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Install %s:\n", dep.Binary))

	// Current OS first
	pm := detectPackageManager()
	if cmd, ok := dep.Install[pm]; ok && pm != "manual" {
		sb.WriteString(fmt.Sprintf("  %s: %s\n", pm, cmd))
	}

	// Other options
	for name, cmd := range dep.Install {
		if name == pm || name == "manual" {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %s: %s\n", name, cmd))
	}

	// Manual last
	if manual, ok := dep.Install["manual"]; ok {
		sb.WriteString(fmt.Sprintf("  Manual: %s\n", manual))
	}

	return sb.String()
}

// ══════════════════════════════════════════════════════════════════════════════
// Runtime Checker (for External Plugins)
// ══════════════════════════════════════════════════════════════════════════════

// CheckRuntime verifies that a runtime is available.
func CheckRuntime(rt ExternalRuntime) *DependencyStatus {
	checker := NewDependencyChecker()

	dep := BinaryDependency{
		Binary:       rt.Command,
		Check:        rt.VersionCheck,
		VersionRegex: `(\d+\.\d+(?:\.\d+)?)`,
		MinVersion:   rt.MinVersion,
	}

	return checker.Check(dep)
}

// CommonRuntimes contains common runtime configurations.
var CommonRuntimes = map[string]ExternalRuntime{
	"python3": {
		Command:      "python3",
		MinVersion:   "3.8",
		VersionCheck: "python3 --version",
	},
	"python": {
		Command:      "python",
		MinVersion:   "3.8",
		VersionCheck: "python --version",
	},
	"node": {
		Command:      "node",
		MinVersion:   "16.0",
		VersionCheck: "node --version",
	},
	"ruby": {
		Command:      "ruby",
		MinVersion:   "2.7",
		VersionCheck: "ruby --version",
	},
	"go": {
		Command:      "go",
		MinVersion:   "1.20",
		VersionCheck: "go version",
	},
}
