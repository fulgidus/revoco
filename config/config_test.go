package config

import (
	"path/filepath"
	"testing"
)

// TestValidateChannel_ValidChannels tests that "stable" and "dev" are accepted
func TestValidateChannel_ValidChannels(t *testing.T) {
	tests := []struct {
		name    string
		channel string
	}{
		{name: "stable channel", channel: "stable"},
		{name: "dev channel", channel: "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateChannel(tt.channel)
			if err != nil {
				t.Errorf("ValidateChannel(%q) returned error: %v, want nil", tt.channel, err)
			}
		})
	}
}

// TestValidateChannel_InvalidChannels tests that invalid channels are rejected
func TestValidateChannel_InvalidChannels(t *testing.T) {
	tests := []struct {
		name    string
		channel string
	}{
		{name: "beta channel", channel: "beta"},
		{name: "nightly channel", channel: "nightly"},
		{name: "empty string", channel: ""},
		{name: "random string", channel: "foobar"},
		{name: "uppercase stable", channel: "STABLE"},
		{name: "mixed case dev", channel: "Dev"},
		{name: "whitespace", channel: "  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateChannel(tt.channel)
			if err == nil {
				t.Errorf("ValidateChannel(%q) returned nil, want error", tt.channel)
			}
		})
	}
}

// TestValidateChannel_DefaultOnInvalid tests that unrecognized channels default to "stable"
func TestValidateChannel_DefaultOnInvalid(t *testing.T) {
	tests := []struct {
		name     string
		channel  string
		expected string
	}{
		{name: "beta defaults to stable", channel: "beta", expected: "stable"},
		{name: "nightly defaults to stable", channel: "nightly", expected: "stable"},
		{name: "empty defaults to stable", channel: "", expected: "stable"},
		{name: "unknown defaults to stable", channel: "unknown", expected: "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeChannel(tt.channel)
			if result != tt.expected {
				t.Errorf("NormalizeChannel(%q) = %q, want %q", tt.channel, result, tt.expected)
			}
		})
	}
}

// TestConfigRoundTrip_ChannelPersistence tests save/load channel persistence
func TestConfigRoundTrip_ChannelPersistence(t *testing.T) {
	tests := []struct {
		name    string
		channel string
	}{
		{name: "stable channel persists", channel: "stable"},
		{name: "dev channel persists", channel: "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp config directory
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")

			// Create config with channel
			cfg := DefaultConfig()
			cfg.Updates.Channel = tt.channel

			// Save
			if err := cfg.SaveTo(configPath); err != nil {
				t.Fatalf("SaveTo() error: %v", err)
			}

			// Load
			loaded, err := LoadFrom(configPath)
			if err != nil {
				t.Fatalf("LoadFrom() error: %v", err)
			}

			// Verify channel persisted
			if loaded.Updates.Channel != tt.channel {
				t.Errorf("Channel not persisted: got %q, want %q", loaded.Updates.Channel, tt.channel)
			}
		})
	}
}

// TestConfigMigration_BetaNightlyToStable tests old configs migrate to "stable"
func TestConfigMigration_BetaNightlyToStable(t *testing.T) {
	tests := []struct {
		name        string
		oldChannel  string
		wantChannel string
	}{
		{name: "beta migrates to stable", oldChannel: "beta", wantChannel: "stable"},
		{name: "nightly migrates to stable", oldChannel: "nightly", wantChannel: "stable"},
		{name: "empty migrates to stable", oldChannel: "", wantChannel: "stable"},
		{name: "unknown migrates to stable", oldChannel: "foobar", wantChannel: "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp config directory
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")

			// Create config with old channel value
			cfg := DefaultConfig()
			cfg.Updates.Channel = tt.oldChannel
			if err := cfg.SaveTo(configPath); err != nil {
				t.Fatalf("SaveTo() error: %v", err)
			}

			// Load with migration (LoadFrom calls NormalizeChannel)
			loaded, err := LoadFrom(configPath)
			if err != nil {
				t.Fatalf("LoadFrom() error: %v", err)
			}

			// Verify migration occurred
			if loaded.Updates.Channel != tt.wantChannel {
				t.Errorf("Channel not migrated: got %q, want %q", loaded.Updates.Channel, tt.wantChannel)
			}
		})
	}
}

// TestConfigDefaultChannel tests that default config has "stable" channel
func TestConfigDefaultChannel(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Updates.Channel != "stable" {
		t.Errorf("DefaultConfig().Updates.Channel = %q, want %q", cfg.Updates.Channel, "stable")
	}
}
