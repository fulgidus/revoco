// Package cmd implements the revoco CLI using cobra.
package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/fulgidus/revoco/plugins"
)

var (
	flagPluginDir string
)

// pluginsCmd manages plugins from the CLI.
var pluginsCmd = &cobra.Command{
	Use:   "plugins",
	Short: "Manage revoco plugins",
	Long: `Plugin management for revoco.

Plugins extend revoco with custom connectors, processors, and outputs.
They can be written in Lua (simple) or any language via JSON-RPC (complex).

Plugin locations:
  ~/.config/revoco/plugins/connectors/
  ~/.config/revoco/plugins/processors/
  ~/.config/revoco/plugins/outputs/
`,
}

var pluginsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installed plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Plugin system is already initialized by PersistentPreRunE
		manager := plugins.PluginManager()
		if manager == nil {
			return fmt.Errorf("plugin system not initialized")
		}

		infos := manager.ListPlugins()
		if len(infos) == 0 {
			fmt.Println("No plugins installed.")
			fmt.Println("\nTo install plugins, place them in:")
			for _, dir := range plugins.DefaultPluginDirs() {
				fmt.Printf("  %s\n", dir)
			}
			return nil
		}

		// Group by type
		connectors := filterByType(infos, plugins.PluginTypeConnector)
		processors := filterByType(infos, plugins.PluginTypeProcessor)
		outputs := filterByType(infos, plugins.PluginTypeOutput)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

		if len(connectors) > 0 {
			fmt.Fprintln(w, "\nCONNECTORS")
			fmt.Fprintln(w, "ID\tNAME\tTIER\tSTATE\tVERSION")
			for _, p := range connectors {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					p.ID, p.Name, p.Tier, stateEmoji(p.State), p.Version)
			}
		}

		if len(processors) > 0 {
			fmt.Fprintln(w, "\nPROCESSORS")
			fmt.Fprintln(w, "ID\tNAME\tTIER\tSTATE\tVERSION")
			for _, p := range processors {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					p.ID, p.Name, p.Tier, stateEmoji(p.State), p.Version)
			}
		}

		if len(outputs) > 0 {
			fmt.Fprintln(w, "\nOUTPUTS")
			fmt.Fprintln(w, "ID\tNAME\tTIER\tSTATE\tVERSION")
			for _, p := range outputs {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					p.ID, p.Name, p.Tier, stateEmoji(p.State), p.Version)
			}
		}

		w.Flush()

		fmt.Printf("\nTotal: %d plugins (%d connectors, %d processors, %d outputs)\n",
			len(infos), len(connectors), len(processors), len(outputs))

		return nil
	},
}

var pluginsInfoCmd = &cobra.Command{
	Use:   "info <plugin-id>",
	Short: "Show detailed information about a plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Plugin system is already initialized by PersistentPreRunE
		manager := plugins.PluginManager()
		if manager == nil {
			return fmt.Errorf("plugin system not initialized")
		}

		pluginID := args[0]

		// Find the plugin
		var found *plugins.PluginInfo
		for _, p := range manager.ListPlugins() {
			if p.ID == pluginID {
				info := p
				found = &info
				break
			}
		}

		if found == nil {
			return fmt.Errorf("plugin not found: %s", pluginID)
		}

		p := found
		fmt.Printf("Plugin: %s\n", p.Name)
		fmt.Printf("  ID:          %s\n", p.ID)
		fmt.Printf("  Version:     %s\n", p.Version)
		fmt.Printf("  Type:        %s\n", p.Type)
		fmt.Printf("  Tier:        %s\n", p.Tier)
		fmt.Printf("  State:       %s\n", stateDescription(p.State))
		fmt.Printf("  Path:        %s\n", p.Path)

		if p.Description != "" {
			fmt.Printf("  Description: %s\n", p.Description)
		}

		if p.Author != "" {
			fmt.Printf("  Author:      %s\n", p.Author)
		}

		if len(p.Capabilities) > 0 {
			caps := make([]string, len(p.Capabilities))
			for i, c := range p.Capabilities {
				caps[i] = string(c)
			}
			fmt.Printf("  Capabilities: %s\n", strings.Join(caps, ", "))
		}

		if len(p.DataTypes) > 0 {
			types := make([]string, len(p.DataTypes))
			for i, t := range p.DataTypes {
				types[i] = string(t)
			}
			fmt.Printf("  Data Types: %s\n", strings.Join(types, ", "))
		}

		if len(p.ConfigSchema) > 0 {
			fmt.Println("  Config Options:")
			for _, opt := range p.ConfigSchema {
				req := ""
				if opt.Required {
					req = " (required)"
				}
				fmt.Printf("    - %s (%s)%s: %s\n", opt.ID, opt.Type, req, opt.Description)
			}
		}

		if len(p.Dependencies) > 0 {
			fmt.Println("  Dependencies:")
			for _, dep := range p.Dependencies {
				fmt.Printf("    - %s (min: %s)\n", dep.Binary, dep.MinVersion)
			}
		}

		if p.StateError != "" {
			fmt.Printf("\n  Error: %s\n", p.StateError)
		}

		return nil
	},
}

var pluginsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check plugin dependencies",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Plugin system is already initialized by PersistentPreRunE
		manager := plugins.PluginManager()
		if manager == nil {
			return fmt.Errorf("plugin system not initialized")
		}

		statuses := manager.CheckDependencies()
		if len(statuses) == 0 {
			fmt.Println("No plugins have binary dependencies.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "BINARY\tFOUND\tVERSION\tMEETS MIN\tERROR")

		allOK := true
		for _, s := range statuses {
			found := "No"
			if s.Found {
				found = "Yes"
			}
			meetsMin := "-"
			if s.Found {
				if s.MeetsMin {
					meetsMin = "Yes"
				} else {
					meetsMin = fmt.Sprintf("No (need %s)", s.MinNeeded)
					allOK = false
				}
			} else {
				allOK = false
			}
			errStr := s.Error
			if errStr == "" {
				errStr = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				s.Binary, found, s.Version, meetsMin, errStr)
		}
		w.Flush()

		if allOK {
			fmt.Println("\nAll dependencies satisfied.")
		} else {
			fmt.Println("\nSome dependencies are missing. Install them to use all plugins.")
		}

		return nil
	},
}

var pluginsReloadCmd = &cobra.Command{
	Use:   "reload [plugin-id]",
	Short: "Reload a plugin (or all plugins)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Plugin system is already initialized by PersistentPreRunE
		manager := plugins.PluginManager()
		if manager == nil {
			return fmt.Errorf("plugin system not initialized")
		}

		if len(args) == 0 {
			// Reload all
			fmt.Println("Reloading all plugins...")
			registry := manager.Registry()
			for _, p := range registry.All() {
				if err := registry.Reload(ctx, p.Info().ID); err != nil {
					fmt.Fprintf(os.Stderr, "  Failed to reload %s: %v\n", p.Info().ID, err)
				} else {
					fmt.Printf("  Reloaded %s\n", p.Info().ID)
				}
			}
		} else {
			// Reload specific plugin
			pluginID := args[0]
			if err := manager.ReloadPlugin(ctx, pluginID); err != nil {
				return fmt.Errorf("failed to reload %s: %w", pluginID, err)
			}
			fmt.Printf("Reloaded %s\n", pluginID)
		}

		return nil
	},
}

var pluginsDirsCmd = &cobra.Command{
	Use:   "dirs",
	Short: "Show plugin directory locations",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Plugin directories:")
		for _, dir := range plugins.DefaultPluginDirs() {
			exists := "missing"
			if _, err := os.Stat(dir); err == nil {
				exists = "exists"
			}
			fmt.Printf("  %s (%s)\n", dir, exists)
		}

		fmt.Println("\nPlugin structure:")
		fmt.Println("  connectors/   - Data source plugins (local files, cloud, etc.)")
		fmt.Println("  processors/   - Data processing plugins (EXIF, conversion, etc.)")
		fmt.Println("  outputs/      - Export destination plugins (local, cloud, etc.)")
	},
}

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Installation Commands
// ══════════════════════════════════════════════════════════════════════════════

var pluginsSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for plugins in the registry",
	Long: `Search for plugins in the revoco plugin registry.

Without a query, lists all available plugins.
With a query, filters by ID, name, description, and tags.

Examples:
  revoco plugins search
  revoco plugins search csv
  revoco plugins search "google photos"
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		query := ""
		if len(args) > 0 {
			query = args[0]
		}

		installer := plugins.NewInstaller()
		results, err := installer.Search(ctx, query)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if len(results) == 0 {
			if query != "" {
				fmt.Printf("No plugins found matching '%s'\n", query)
			} else {
				fmt.Println("No plugins available in registry.")
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tVERSION\tTYPE\tDESCRIPTION")

		for _, entry := range results {
			desc := entry.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				entry.ID, entry.Name, entry.Version, entry.Type, desc)
		}
		w.Flush()

		fmt.Printf("\nFound %d plugin(s).\n", len(results))
		fmt.Println("Use 'revoco plugins install <id>' to install a plugin.")

		return nil
	},
}

var pluginsInstallCmd = &cobra.Command{
	Use:   "install <plugin-id|url|path>",
	Short: "Install a plugin",
	Long: `Install a plugin from the registry, URL, or local path.

Examples:
  revoco plugins install google-photos-connector
  revoco plugins install https://example.com/plugin.lua
  revoco plugins install ./my-plugin.lua
  revoco plugins install ./my-plugin-dir/
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		source := args[0]
		installer := plugins.NewInstaller()

		// Determine source type
		if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
			// URL
			fmt.Printf("Installing from URL: %s\n", source)
			if err := installer.InstallFromURL(ctx, source); err != nil {
				return fmt.Errorf("installation failed: %w", err)
			}
		} else if fileExists(source) {
			// Local path
			fmt.Printf("Installing from path: %s\n", source)
			if err := installer.InstallFromPath(ctx, source); err != nil {
				return fmt.Errorf("installation failed: %w", err)
			}
		} else {
			// Registry ID
			fmt.Printf("Installing from registry: %s\n", source)
			if err := installer.Install(ctx, source); err != nil {
				return fmt.Errorf("installation failed: %w", err)
			}
		}

		fmt.Println("Plugin installed successfully.")
		fmt.Println("Run 'revoco plugins list' to see installed plugins.")

		return nil
	},
}

var pluginsRemoveCmd = &cobra.Command{
	Use:     "remove <plugin-id>",
	Aliases: []string{"uninstall", "rm"},
	Short:   "Remove an installed plugin",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pluginID := args[0]
		installer := plugins.NewInstaller()

		fmt.Printf("Removing plugin: %s\n", pluginID)
		if err := installer.Remove(ctx, pluginID); err != nil {
			return fmt.Errorf("removal failed: %w", err)
		}

		fmt.Println("Plugin removed successfully.")

		return nil
	},
}

var pluginsUpdateCmd = &cobra.Command{
	Use:   "update [plugin-id]",
	Short: "Update plugins to latest version",
	Long: `Update plugins to their latest versions from the registry.

Without arguments, checks for and lists available updates.
With a plugin ID, updates that specific plugin.
With --all flag, updates all plugins with available updates.

Examples:
  revoco plugins update           # Check for updates
  revoco plugins update csv-conn  # Update specific plugin
  revoco plugins update --all     # Update all plugins
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		installer := plugins.NewInstaller()
		updateAll, _ := cmd.Flags().GetBool("all")

		if len(args) > 0 {
			// Update specific plugin
			pluginID := args[0]
			fmt.Printf("Updating plugin: %s\n", pluginID)
			if err := installer.Update(ctx, pluginID); err != nil {
				return fmt.Errorf("update failed: %w", err)
			}
			fmt.Println("Plugin updated successfully.")
			return nil
		}

		// Check for updates
		updates, err := installer.CheckUpdates(ctx)
		if err != nil {
			return fmt.Errorf("failed to check updates: %w", err)
		}

		if len(updates) == 0 {
			fmt.Println("All plugins are up to date.")
			return nil
		}

		if !updateAll {
			// Just list available updates
			fmt.Println("Available updates:")
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tCURRENT\tLATEST")
			for _, u := range updates {
				fmt.Fprintf(w, "%s\t%s\t%s\n", u.ID, u.CurrentVersion, u.LatestVersion)
			}
			w.Flush()
			fmt.Println("\nRun 'revoco plugins update --all' to update all plugins.")
			return nil
		}

		// Update all
		fmt.Println("Updating all plugins...")
		updated, err := installer.UpdateAll(ctx)
		if err != nil {
			return fmt.Errorf("update failed: %w", err)
		}

		if len(updated) > 0 {
			fmt.Printf("Updated %d plugin(s): %s\n", len(updated), strings.Join(updated, ", "))
		} else {
			fmt.Println("No plugins were updated.")
		}

		return nil
	},
}

var pluginsResetDefaultsCmd = &cobra.Command{
	Use:   "reset-defaults",
	Short: "Re-extract default plugins (overwrites existing)",
	Long: `Re-extracts the bundled default plugins to your plugin directory.

Warning: This will overwrite any modifications you made to the default plugins.
Custom plugins that you added will not be affected.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		if !force {
			fmt.Println("This will overwrite any modifications to default plugins.")
			fmt.Println("Use --force to confirm.")
			return nil
		}

		if err := plugins.ForceExtractDefaultPlugins(""); err != nil {
			return fmt.Errorf("failed to extract defaults: %w", err)
		}

		fmt.Println("Default plugins have been re-extracted.")

		return nil
	},
}

func init() {
	// Add plugins subcommands
	pluginsCmd.AddCommand(pluginsListCmd)
	pluginsCmd.AddCommand(pluginsInfoCmd)
	pluginsCmd.AddCommand(pluginsCheckCmd)
	pluginsCmd.AddCommand(pluginsReloadCmd)
	pluginsCmd.AddCommand(pluginsDirsCmd)

	// Plugin installation commands
	pluginsCmd.AddCommand(pluginsSearchCmd)
	pluginsCmd.AddCommand(pluginsInstallCmd)
	pluginsCmd.AddCommand(pluginsRemoveCmd)
	pluginsCmd.AddCommand(pluginsUpdateCmd)
	pluginsCmd.AddCommand(pluginsResetDefaultsCmd)

	// Command flags
	pluginsUpdateCmd.Flags().Bool("all", false, "Update all plugins")
	pluginsResetDefaultsCmd.Flags().Bool("force", false, "Force re-extraction, overwriting existing files")

	// Add to root
	rootCmd.AddCommand(pluginsCmd)
}

// Helper functions

func filterByType(infos []plugins.PluginInfo, ptype plugins.PluginType) []plugins.PluginInfo {
	var result []plugins.PluginInfo
	for _, p := range infos {
		if p.Type == ptype {
			result = append(result, p)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func stateEmoji(state plugins.PluginState) string {
	switch state {
	case plugins.PluginStateReady:
		return "ready"
	case plugins.PluginStateLoading:
		return "loading"
	case plugins.PluginStateMissingDeps:
		return "missing-deps"
	case plugins.PluginStateError:
		return "error"
	case plugins.PluginStateDisabled:
		return "disabled"
	default:
		return "unloaded"
	}
}

func stateDescription(state plugins.PluginState) string {
	switch state {
	case plugins.PluginStateReady:
		return "Ready (loaded successfully)"
	case plugins.PluginStateLoading:
		return "Loading..."
	case plugins.PluginStateMissingDeps:
		return "Missing Dependencies"
	case plugins.PluginStateError:
		return "Error"
	case plugins.PluginStateDisabled:
		return "Disabled by user"
	default:
		return "Not loaded"
	}
}

// fileExists checks if a path exists on the filesystem.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
