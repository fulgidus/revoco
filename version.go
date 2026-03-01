package main

// Version information set by ldflags during build.
// Example: go build -ldflags "-X main.Version=1.0.0 -X main.Commit=abc123"
var (
	// Version is the semantic version (e.g., "0.1.0")
	Version = "dev"

	// Commit is the git commit hash
	Commit = "none"

	// BuildDate is the build timestamp
	BuildDate = "unknown"
)

// VersionInfo returns formatted version information.
func VersionInfo() string {
	return Version
}

// FullVersionInfo returns detailed version information.
func FullVersionInfo() string {
	info := "revoco " + Version
	if Commit != "none" && Commit != "" {
		info += " (" + Commit + ")"
	}
	if BuildDate != "unknown" && BuildDate != "" {
		info += " built " + BuildDate
	}
	return info
}
