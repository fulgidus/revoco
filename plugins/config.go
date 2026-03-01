package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the plugin system configuration.
type Config struct {
	// Plugin directories to scan
	PluginDirs []string `json:"plugin_dirs,omitempty"`

	// Disabled plugins (by ID)
	Disabled []string `json:"disabled,omitempty"`

	// Plugin-specific settings
	PluginSettings map[string]map[string]any `json:"plugin_settings,omitempty"`

	// Hot-reload enabled
	HotReload bool `json:"hot_reload,omitempty"`
}

// DefaultConfig returns the default plugin configuration.
func DefaultConfig() *Config {
	return &Config{
		PluginDirs:     DefaultPluginDirs(),
		Disabled:       []string{},
		PluginSettings: make(map[string]map[string]any),
		HotReload:      true,
	}
}

// ConfigPath returns the default config file path.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "revoco", "plugins.json")
}

// LoadConfig loads the plugin configuration from the default path.
func LoadConfig() (*Config, error) {
	return LoadConfigFrom(ConfigPath())
}

// LoadConfigFrom loads the plugin configuration from a specific path.
func LoadConfigFrom(path string) (*Config, error) {
	if path == "" {
		return DefaultConfig(), nil
	}

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

	return cfg, nil
}

// SaveConfig saves the plugin configuration to the default path.
func SaveConfig(cfg *Config) error {
	return SaveConfigTo(cfg, ConfigPath())
}

// SaveConfigTo saves the plugin configuration to a specific path.
func SaveConfigTo(cfg *Config, path string) error {
	if path == "" {
		return fmt.Errorf("no config path specified")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// IsDisabled checks if a plugin is disabled.
func (c *Config) IsDisabled(pluginID string) bool {
	for _, id := range c.Disabled {
		if id == pluginID {
			return true
		}
	}
	return false
}

// Disable disables a plugin.
func (c *Config) Disable(pluginID string) {
	if !c.IsDisabled(pluginID) {
		c.Disabled = append(c.Disabled, pluginID)
	}
}

// Enable enables a previously disabled plugin.
func (c *Config) Enable(pluginID string) {
	for i, id := range c.Disabled {
		if id == pluginID {
			c.Disabled = append(c.Disabled[:i], c.Disabled[i+1:]...)
			return
		}
	}
}

// GetPluginSettings returns settings for a specific plugin.
func (c *Config) GetPluginSettings(pluginID string) map[string]any {
	if c.PluginSettings == nil {
		return nil
	}
	return c.PluginSettings[pluginID]
}

// SetPluginSettings sets settings for a specific plugin.
func (c *Config) SetPluginSettings(pluginID string, settings map[string]any) {
	if c.PluginSettings == nil {
		c.PluginSettings = make(map[string]map[string]any)
	}
	c.PluginSettings[pluginID] = settings
}

// GetPluginSetting returns a specific setting for a plugin.
func (c *Config) GetPluginSetting(pluginID, key string) (any, bool) {
	settings := c.GetPluginSettings(pluginID)
	if settings == nil {
		return nil, false
	}
	val, ok := settings[key]
	return val, ok
}

// SetPluginSetting sets a specific setting for a plugin.
func (c *Config) SetPluginSetting(pluginID, key string, value any) {
	if c.PluginSettings == nil {
		c.PluginSettings = make(map[string]map[string]any)
	}
	if c.PluginSettings[pluginID] == nil {
		c.PluginSettings[pluginID] = make(map[string]any)
	}
	c.PluginSettings[pluginID][key] = value
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Check plugin directories exist
	for _, dir := range c.PluginDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			// Not an error - directory will be created
			continue
		}
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Configuration Validation
// ══════════════════════════════════════════════════════════════════════════════

// ValidatePluginConfig validates configuration for a specific plugin.
func ValidatePluginConfig(plugin Plugin, settings map[string]any) error {
	info := plugin.Info()

	for _, opt := range info.ConfigSchema {
		val, exists := settings[opt.ID]

		// Check required
		if opt.Required && !exists {
			return &ConfigError{
				PluginID: info.ID,
				Field:    opt.ID,
				Reason:   "required field missing",
			}
		}

		if !exists {
			continue
		}

		// Type validation
		if err := validateConfigValue(opt, val); err != nil {
			return &ConfigError{
				PluginID: info.ID,
				Field:    opt.ID,
				Reason:   err.Error(),
			}
		}
	}

	return nil
}

// validateConfigValue validates a single config value against its schema.
func validateConfigValue(opt ConfigOption, val any) error {
	switch opt.Type {
	case "bool":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", val)
		}

	case "string", "path", "password":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("expected string, got %T", val)
		}

	case "int":
		switch val.(type) {
		case int, int64, float64:
			// OK - JSON numbers are float64
		default:
			return fmt.Errorf("expected int, got %T", val)
		}

	case "float":
		switch val.(type) {
		case float64, float32, int, int64:
			// OK
		default:
			return fmt.Errorf("expected float, got %T", val)
		}

	case "select":
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", val)
		}

		valid := false
		for _, opt := range opt.Options {
			if s == opt {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid option: %s (valid: %v)", s, opt.Options)
		}
	}

	return nil
}

// ConfigError represents a configuration validation error.
type ConfigError struct {
	PluginID string
	Field    string
	Reason   string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("plugin %s: config %s: %s", e.PluginID, e.Field, e.Reason)
}

// ApplyDefaults applies default values to a settings map.
func ApplyDefaults(schema []ConfigOption, settings map[string]any) map[string]any {
	if settings == nil {
		settings = make(map[string]any)
	}

	for _, opt := range schema {
		if _, exists := settings[opt.ID]; !exists && opt.Default != nil {
			settings[opt.ID] = opt.Default
		}
	}

	return settings
}
