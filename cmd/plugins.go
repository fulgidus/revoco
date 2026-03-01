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
	// Import lua package to register the Lua plugin factory
	_ "github.com/fulgidus/revoco/plugins/lua"
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Initialize plugins
		if err := plugins.InitializePlugins(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: some plugins failed to load: %v\n", err)
		}
		defer plugins.ShutdownPlugins()

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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := plugins.InitializePlugins(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: some plugins failed to load: %v\n", err)
		}
		defer plugins.ShutdownPlugins()

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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := plugins.InitializePlugins(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: some plugins failed to load: %v\n", err)
		}
		defer plugins.ShutdownPlugins()

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

		if err := plugins.InitializePlugins(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: some plugins failed to load: %v\n", err)
		}
		defer plugins.ShutdownPlugins()

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

func init() {
	// Add plugins subcommands
	pluginsCmd.AddCommand(pluginsListCmd)
	pluginsCmd.AddCommand(pluginsInfoCmd)
	pluginsCmd.AddCommand(pluginsCheckCmd)
	pluginsCmd.AddCommand(pluginsReloadCmd)
	pluginsCmd.AddCommand(pluginsDirsCmd)

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
