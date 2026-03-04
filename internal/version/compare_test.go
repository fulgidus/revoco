package version

import (
	"testing"
)

func TestIsDevVersion(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected bool
	}{
		{"stable version", "v1.0.0", false},
		{"dev version with timestamp", "v1.0.0-dev-2026-03-05T10-00-00-abc1234", true},
		{"dev version without v prefix", "1.0.0-dev-2026-03-05T10-00-00-abc1234", true},
		{"stable with v prefix", "v0.1.10", false},
		{"empty string", "", false},
		{"dev version short", "v1.0.0-dev-suffix", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDevVersion(tt.tag)
			if got != tt.expected {
				t.Errorf("IsDevVersion(%q) = %v, want %v", tt.tag, got, tt.expected)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name      string
		tag       string
		wantError bool
	}{
		{"valid stable version with v", "v1.0.0", false},
		{"valid stable version without v", "1.0.0", false},
		{"valid dev version", "v1.0.0-dev-2026-03-05T10-00-00-abc1234", false},
		{"valid dev version without v", "1.0.0-dev-2026-03-05T10-00-00-abc1234", false},
		{"empty string", "", true},
		{"invalid semver", "not-a-version", true},
		{"malformed version", "v1.0.x", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := ParseVersion(tt.tag)
			if tt.wantError {
				if err == nil {
					t.Errorf("ParseVersion(%q) expected error, got nil", tt.tag)
				}
			} else {
				if err != nil {
					t.Errorf("ParseVersion(%q) unexpected error: %v", tt.tag, err)
				}
				if v == nil {
					t.Errorf("ParseVersion(%q) returned nil version", tt.tag)
				}
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		current   string
		expected  bool
		wantError bool
	}{
		{"stable newer than stable", "v1.1.0", "v1.0.0", true, false},
		{"stable older than stable", "v1.0.0", "v1.1.0", false, false},
		{"stable equal to stable", "v1.0.0", "v1.0.0", false, false},
		{"dev newer than stable", "v1.0.1-dev-2026-03-05T10-00-00-abc1234", "v1.0.0", true, false},
		{"dev older than stable (base version)", "v1.0.0-dev-2026-03-05T10-00-00-abc1234", "v1.0.1", false, false},
		{"dev newer than dev (same base, newer timestamp)", "v1.0.0-dev-2026-03-05T11-00-00-xyz9999", "v1.0.0-dev-2026-03-05T10-00-00-abc1234", true, false},
		{"dev older than dev (same base, older timestamp)", "v1.0.0-dev-2026-03-05T09-00-00-abc1234", "v1.0.0-dev-2026-03-05T10-00-00-xyz9999", false, false},
		{"dev equal to dev (same timestamp)", "v1.0.0-dev-2026-03-05T10-00-00-abc1234", "v1.0.0-dev-2026-03-05T10-00-00-xyz9999", false, false},
		{"invalid candidate version", "invalid", "v1.0.0", false, true},
		{"invalid current version", "v1.0.0", "invalid", false, true},
		{"both invalid", "invalid1", "invalid2", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsNewer(tt.candidate, tt.current)
			if tt.wantError {
				if err == nil {
					t.Errorf("IsNewer(%q, %q) expected error, got nil", tt.candidate, tt.current)
				}
			} else {
				if err != nil {
					t.Errorf("IsNewer(%q, %q) unexpected error: %v", tt.candidate, tt.current, err)
				}
				if got != tt.expected {
					t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.candidate, tt.current, got, tt.expected)
				}
			}
		})
	}
}

func TestIsNewer_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		current   string
		expected  bool
		wantError bool
	}{
		{"empty candidate", "", "v1.0.0", false, true},
		{"empty current", "v1.0.0", "", false, true},
		{"both empty", "", "", false, true},
		{"major version upgrade", "v2.0.0", "v1.9.9", true, false},
		{"patch version upgrade", "v1.0.1", "v1.0.0", true, false},
		{"prerelease dev vs stable", "v1.0.0-dev-2026-03-05T10-00-00-abc1234", "v1.0.0", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsNewer(tt.candidate, tt.current)
			if tt.wantError {
				if err == nil {
					t.Errorf("IsNewer(%q, %q) expected error, got nil", tt.candidate, tt.current)
				}
			} else {
				if err != nil {
					t.Errorf("IsNewer(%q, %q) unexpected error: %v", tt.candidate, tt.current, err)
				}
				if got != tt.expected {
					t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.candidate, tt.current, got, tt.expected)
				}
			}
		})
	}
}
