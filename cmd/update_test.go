package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fulgidus/revoco/config"
	"github.com/fulgidus/revoco/internal/update"
	"github.com/fulgidus/revoco/internal/version"
)

// TestUpdateCommand_StableChannel verifies that update command uses stable release fetcher
// when config.Updates.Channel is "stable".
func TestUpdateCommand_StableChannel(t *testing.T) {
	// Mock GitHub API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/fulgidus/revoco/releases/latest" {
			release := update.Release{
				TagName:     "v1.0.0",
				Name:        "Release 1.0.0",
				Draft:       false,
				Prerelease:  false,
				Body:        "Stable release notes",
				PublishedAt: time.Now(),
				Assets:      []update.Asset{},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(release)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create test config with stable channel
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := config.DefaultConfig()
	cfg.Updates.Channel = "stable"
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Set config path env var
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Fetch release (integration point)
	ctx := context.Background()
	release, err := fetchLatestReleaseWithChannel(ctx, server.URL, cfg.Updates.Channel)
	if err != nil {
		t.Fatalf("Expected stable release fetch to succeed, got error: %v", err)
	}

	if release.TagName != "v1.0.0" {
		t.Errorf("Expected stable release v1.0.0, got %s", release.TagName)
	}
}

// TestUpdateCommand_DevChannel verifies that update command uses dev release fetcher
// when config.Updates.Channel is "dev".
func TestUpdateCommand_DevChannel(t *testing.T) {
	// Mock GitHub API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/fulgidus/revoco/releases" {
			releases := []update.Release{
				{
					TagName:     "v1.0.0",
					Name:        "Release 1.0.0",
					Prerelease:  false,
					PublishedAt: time.Now().Add(-48 * time.Hour),
				},
				{
					TagName:     "v1.0.1-dev-2026-03-05T10-00-00-abc123",
					Name:        "Dev Release",
					Prerelease:  true,
					PublishedAt: time.Now().Add(-24 * time.Hour),
				},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(releases)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create test config with dev channel
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := config.DefaultConfig()
	cfg.Updates.Channel = "dev"
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Fetch release (integration point)
	ctx := context.Background()
	release, err := fetchLatestReleaseWithChannel(ctx, server.URL, cfg.Updates.Channel)
	if err != nil {
		t.Fatalf("Expected dev release fetch to succeed, got error: %v", err)
	}

	if release.TagName != "v1.0.1-dev-2026-03-05T10-00-00-abc123" {
		t.Errorf("Expected dev release, got %s", release.TagName)
	}
}

// TestUpdateCommand_VersionComparison verifies that version comparison uses IsNewer
// to determine if an update is available.
func TestUpdateCommand_VersionComparison(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		latest      string
		wantNewer   bool
		description string
	}{
		{
			name:        "newer stable available",
			current:     "v1.0.0",
			latest:      "v1.1.0",
			wantNewer:   true,
			description: "Newer stable version should trigger update",
		},
		{
			name:        "same version",
			current:     "v1.0.0",
			latest:      "v1.0.0",
			wantNewer:   false,
			description: "Same version should not trigger update",
		},
		{
			name:        "current newer than latest",
			current:     "v1.1.0",
			latest:      "v1.0.0",
			wantNewer:   false,
			description: "Older latest version should not trigger update",
		},
		{
			name:        "dev version newer",
			current:     "v1.0.0",
			latest:      "v1.1.0-dev-2026-03-05T10-00-00-abc",
			wantNewer:   true,
			description: "Newer dev version should trigger update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isNewer, err := compareVersions(tt.latest, tt.current)
			if err != nil {
				t.Fatalf("Version comparison failed: %v", err)
			}

			if isNewer != tt.wantNewer {
				t.Errorf("%s: IsNewer(%s, %s) = %v, want %v",
					tt.description, tt.latest, tt.current, isNewer, tt.wantNewer)
			}
		})
	}
}

// TestUpdateCommand_ChannelFlagOverride verifies that --channel flag overrides
// the config file channel setting.
func TestUpdateCommand_ChannelFlagOverride(t *testing.T) {
	// Mock GitHub API server that tracks which endpoint is called
	var calledEndpoint string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledEndpoint = r.URL.Path

		if r.URL.Path == "/repos/fulgidus/revoco/releases/latest" {
			release := update.Release{
				TagName:     "v1.0.0",
				Name:        "Stable Release",
				Prerelease:  false,
				PublishedAt: time.Now(),
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(release)
			return
		}

		if r.URL.Path == "/repos/fulgidus/revoco/releases" {
			releases := []update.Release{
				{
					TagName:     "v1.0.1-dev-2026-03-05T10-00-00-abc",
					Name:        "Dev Release",
					Prerelease:  true,
					PublishedAt: time.Now(),
				},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(releases)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create test config with stable channel
	tmpDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Updates.Channel = "stable"
	if err := cfg.SaveTo(filepath.Join(tmpDir, "config.json")); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	tests := []struct {
		name             string
		flagChannel      string
		expectedEndpoint string
		expectedTag      string
	}{
		{
			name:             "flag overrides to dev",
			flagChannel:      "dev",
			expectedEndpoint: "/repos/fulgidus/revoco/releases",
			expectedTag:      "v1.0.1-dev-2026-03-05T10-00-00-abc",
		},
		{
			name:             "no flag uses config stable",
			flagChannel:      "",
			expectedEndpoint: "/repos/fulgidus/revoco/releases/latest",
			expectedTag:      "v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calledEndpoint = ""

			// Determine channel (simulate flag override logic)
			channel := tt.flagChannel
			if channel == "" {
				channel = cfg.Updates.Channel
			}

			ctx := context.Background()
			release, err := fetchLatestReleaseWithChannel(ctx, server.URL, channel)
			if err != nil {
				t.Fatalf("Failed to fetch release: %v", err)
			}

			if calledEndpoint != tt.expectedEndpoint {
				t.Errorf("Expected endpoint %s, got %s", tt.expectedEndpoint, calledEndpoint)
			}

			if release.TagName != tt.expectedTag {
				t.Errorf("Expected tag %s, got %s", tt.expectedTag, release.TagName)
			}
		})
	}
}

// TestUpdateCommand_DevToStableDowngradeWarning verifies that when switching
// from dev to stable, if the stable version is older, a warning is shown.
func TestUpdateCommand_DevToStableDowngradeWarning(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		latestVersion  string
		latestChannel  string
		wantWarning    bool
		description    string
	}{
		{
			name:           "dev to older stable shows warning",
			currentVersion: "v1.0.1-dev-2026-03-05T10-00-00-abc",
			latestVersion:  "v1.0.0",
			latestChannel:  "stable",
			wantWarning:    true,
			description:    "Dev version newer than stable should warn",
		},
		{
			name:           "dev to newer stable no warning",
			currentVersion: "v1.0.0-dev-2026-03-05T10-00-00-abc",
			latestVersion:  "v1.1.0",
			latestChannel:  "stable",
			wantWarning:    false,
			description:    "Newer stable version should not warn",
		},
		{
			name:           "stable to stable no warning",
			currentVersion: "v1.0.0",
			latestVersion:  "v1.1.0",
			latestChannel:  "stable",
			wantWarning:    false,
			description:    "Stable to stable upgrade should not warn",
		},
		{
			name:           "dev to dev no warning",
			currentVersion: "v1.0.0-dev-2026-03-05T09-00-00-xyz",
			latestVersion:  "v1.0.0-dev-2026-03-05T10-00-00-abc",
			latestChannel:  "dev",
			wantWarning:    false,
			description:    "Dev to dev upgrade should not warn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldWarn := shouldShowDowngradeWarning(
				tt.currentVersion,
				tt.latestVersion,
				tt.latestChannel,
			)

			if shouldWarn != tt.wantWarning {
				t.Errorf("%s: shouldShowDowngradeWarning(%s, %s, %s) = %v, want %v",
					tt.description,
					tt.currentVersion,
					tt.latestVersion,
					tt.latestChannel,
					shouldWarn,
					tt.wantWarning)
			}
		})
	}
}

// Helper functions that will be implemented in cmd/update.go

// fetchLatestReleaseWithChannel is a wrapper that uses internal/update dispatcher
func fetchLatestReleaseWithChannel(ctx context.Context, apiBase, channel string) (*update.Release, error) {
	return update.FetchLatestRelease(ctx, apiBase, githubOwner, githubRepo, channel)
}

// compareVersions wraps version.IsNewer for testing
func compareVersions(candidate, current string) (bool, error) {
	// Import from internal/version package
	return version.IsNewer(candidate, current)
}

// shouldShowDowngradeWarning determines if a downgrade warning should be shown
func shouldShowDowngradeWarning(currentVersion, latestVersion, latestChannel string) bool {
	// Dev to stable with older stable version
	if !version.IsDevVersion(currentVersion) {
		return false
	}
	if version.IsDevVersion(latestVersion) {
		return false
	}
	if latestChannel != "stable" {
		return false
	}

	// Check if latest is older than current
	isNewer, err := version.IsNewer(latestVersion, currentVersion)
	if err != nil {
		return false
	}

	// If latest is NOT newer, it's a downgrade
	return !isNewer
}
