// Package config provides global configuration for revoco.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config holds the global revoco configuration.
type Config struct {
	// Update settings
	Updates UpdateConfig `json:"updates"`

	// Plugin settings
	Plugins PluginConfig `json:"plugins"`

	// Telemetry settings (for future use)
	Telemetry TelemetryConfig `json:"telemetry,omitempty"`
}

// UpdateConfig holds update-related settings.
type UpdateConfig struct {
	// AutoCheck enables automatic update checking on startup
	AutoCheck bool `json:"auto_check"`

	// AutoCheckInterval is how often to check for updates (e.g., "24h")
	AutoCheckInterval string `json:"auto_check_interval"`

	// AutoInstall enables automatic installation of updates
	AutoInstall bool `json:"auto_install"`

	// IncludePrerelease includes pre-release versions in update checks
	IncludePrerelease bool `json:"include_prerelease"`

	// LastCheck is the timestamp of the last update check
	LastCheck time.Time `json:"last_check,omitempty"`

	// SkippedVersion is a version the user chose to skip
	SkippedVersion string `json:"skipped_version,omitempty"`

	// Channel is the update channel (stable, dev)
	Channel string `json:"channel"`
}

// PluginConfig holds plugin-related global settings.
type PluginConfig struct {
	// AutoUpdate enables automatic plugin updates
	AutoUpdate bool `json:"auto_update"`

	// AutoUpdateInterval is how often to check for plugin updates
	AutoUpdateInterval string `json:"auto_update_interval"`

	// LastPluginCheck is the timestamp of the last plugin update check
	LastPluginCheck time.Time `json:"last_plugin_check,omitempty"`

	// RegistryURL is a custom plugin registry URL
	RegistryURL string `json:"registry_url,omitempty"`
}

// TelemetryConfig holds telemetry settings.
type TelemetryConfig struct {
	// Enabled controls whether anonymous usage data is collected
	Enabled bool `json:"enabled"`

	// MachineID is an anonymous identifier
	MachineID string `json:"machine_id,omitempty"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Updates: UpdateConfig{
			AutoCheck:         true,
			AutoCheckInterval: "24h",
			AutoInstall:       false,
			IncludePrerelease: false,
			Channel:           "stable",
		},
		Plugins: PluginConfig{
			AutoUpdate:         false,
			AutoUpdateInterval: "24h",
		},
		Telemetry: TelemetryConfig{
			Enabled: false,
		},
	}
}

// ValidateChannel validates that the channel is either "stable" or "dev".
func ValidateChannel(channel string) error {
	if channel == "stable" || channel == "dev" {
		return nil
	}
	return fmt.Errorf("invalid channel: %q, must be 'stable' or 'dev'", channel)
}

// NormalizeChannel ensures the channel is valid, defaulting to "stable" for unrecognized values.
func NormalizeChannel(channel string) string {
	if channel == "stable" || channel == "dev" {
		return channel
	}
	return "stable"
}

// ConfigDir returns the configuration directory path.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "revoco"), nil
}

// ConfigPath returns the global config file path.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load loads the configuration from the default path.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return DefaultConfig(), nil
	}
	return LoadFrom(path)
}

// LoadFrom loads the configuration from a specific path.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Normalize channel (migrate old beta/nightly to stable)
	cfg.Updates.Channel = NormalizeChannel(cfg.Updates.Channel)

	return cfg, nil
}

// Save saves the configuration to the default path.
func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return c.SaveTo(path)
}

// SaveTo saves the configuration to a specific path.
func (c *Config) SaveTo(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// ShouldCheckForUpdates returns true if an update check should be performed.
func (c *Config) ShouldCheckForUpdates() bool {
	if !c.Updates.AutoCheck {
		return false
	}

	interval, err := time.ParseDuration(c.Updates.AutoCheckInterval)
	if err != nil {
		interval = 24 * time.Hour
	}

	return time.Since(c.Updates.LastCheck) > interval
}

// ShouldCheckForPluginUpdates returns true if a plugin update check should be performed.
func (c *Config) ShouldCheckForPluginUpdates() bool {
	if !c.Plugins.AutoUpdate {
		return false
	}

	interval, err := time.ParseDuration(c.Plugins.AutoUpdateInterval)
	if err != nil {
		interval = 24 * time.Hour
	}

	return time.Since(c.Plugins.LastPluginCheck) > interval
}

// RecordUpdateCheck updates the last update check timestamp.
func (c *Config) RecordUpdateCheck() error {
	c.Updates.LastCheck = time.Now()
	return c.Save()
}

// RecordPluginCheck updates the last plugin check timestamp.
func (c *Config) RecordPluginCheck() error {
	c.Plugins.LastPluginCheck = time.Now()
	return c.Save()
}

// SkipVersion marks a version as skipped.
func (c *Config) SkipVersion(version string) error {
	c.Updates.SkippedVersion = version
	return c.Save()
}

// IsVersionSkipped checks if a version has been skipped.
func (c *Config) IsVersionSkipped(version string) bool {
	return c.Updates.SkippedVersion == version
}

// Global config instance
var globalConfig *Config

// Get returns the global configuration, loading it if necessary.
func Get() *Config {
	if globalConfig == nil {
		var err error
		globalConfig, err = Load()
		if err != nil {
			globalConfig = DefaultConfig()
		}
	}
	return globalConfig
}

// Reload reloads the global configuration from disk.
func Reload() error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	globalConfig = cfg
	return nil
}
