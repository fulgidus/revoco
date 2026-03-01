// Package cmd implements the revoco CLI using cobra.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/fulgidus/revoco/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage revoco configuration",
	Long: `View and modify revoco configuration settings.

Configuration is stored in ~/.config/revoco/config.json
`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()

		fmt.Println("Update Settings:")
		fmt.Printf("  Auto-check:          %v\n", cfg.Updates.AutoCheck)
		fmt.Printf("  Check interval:      %s\n", cfg.Updates.AutoCheckInterval)
		fmt.Printf("  Auto-install:        %v\n", cfg.Updates.AutoInstall)
		fmt.Printf("  Include prerelease:  %v\n", cfg.Updates.IncludePrerelease)
		fmt.Printf("  Update channel:      %s\n", cfg.Updates.Channel)
		if !cfg.Updates.LastCheck.IsZero() {
			fmt.Printf("  Last check:          %s\n", cfg.Updates.LastCheck.Format("2006-01-02 15:04:05"))
		}
		if cfg.Updates.SkippedVersion != "" {
			fmt.Printf("  Skipped version:     %s\n", cfg.Updates.SkippedVersion)
		}

		fmt.Println("\nPlugin Settings:")
		fmt.Printf("  Auto-update:         %v\n", cfg.Plugins.AutoUpdate)
		fmt.Printf("  Update interval:     %s\n", cfg.Plugins.AutoUpdateInterval)
		if cfg.Plugins.RegistryURL != "" {
			fmt.Printf("  Custom registry:     %s\n", cfg.Plugins.RegistryURL)
		}
		if !cfg.Plugins.LastPluginCheck.IsZero() {
			fmt.Printf("  Last check:          %s\n", cfg.Plugins.LastPluginCheck.Format("2006-01-02 15:04:05"))
		}

		fmt.Println("\nTelemetry:")
		fmt.Printf("  Enabled:             %v\n", cfg.Telemetry.Enabled)

		path, _ := config.ConfigPath()
		fmt.Printf("\nConfig file: %s\n", path)

		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

Available keys:
  updates.auto-check        Enable/disable automatic update checks (true/false)
  updates.auto-install      Enable/disable automatic update installation (true/false)
  updates.check-interval    How often to check for updates (e.g., "24h", "1h")
  updates.include-prerelease Include pre-release versions (true/false)
  updates.channel           Update channel (stable/beta/nightly)
  plugins.auto-update       Enable/disable automatic plugin updates (true/false)
  plugins.update-interval   How often to check for plugin updates
  plugins.registry-url      Custom plugin registry URL
  telemetry.enabled         Enable/disable anonymous telemetry (true/false)

Examples:
  revoco config set updates.auto-check true
  revoco config set updates.check-interval 12h
  revoco config set plugins.auto-update true
`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		cfg := config.Get()

		switch key {
		case "updates.auto-check":
			cfg.Updates.AutoCheck = parseBool(value)
		case "updates.auto-install":
			cfg.Updates.AutoInstall = parseBool(value)
		case "updates.check-interval":
			cfg.Updates.AutoCheckInterval = value
		case "updates.include-prerelease":
			cfg.Updates.IncludePrerelease = parseBool(value)
		case "updates.channel":
			if value != "stable" && value != "beta" && value != "nightly" {
				return fmt.Errorf("invalid channel: %s (must be stable, beta, or nightly)", value)
			}
			cfg.Updates.Channel = value
		case "plugins.auto-update":
			cfg.Plugins.AutoUpdate = parseBool(value)
		case "plugins.update-interval":
			cfg.Plugins.AutoUpdateInterval = value
		case "plugins.registry-url":
			cfg.Plugins.RegistryURL = value
		case "telemetry.enabled":
			cfg.Telemetry.Enabled = parseBool(value)
		default:
			return fmt.Errorf("unknown configuration key: %s", key)
		}

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration to defaults",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.DefaultConfig()
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Println("Configuration reset to defaults.")
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	Run: func(cmd *cobra.Command, args []string) {
		path, err := config.ConfigPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Println(path)
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configPathCmd)

	rootCmd.AddCommand(configCmd)
}

func parseBool(s string) bool {
	return s == "true" || s == "1" || s == "yes" || s == "on"
}
