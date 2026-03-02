// Package cmd implements the revoco CLI using cobra.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/session"

	// Import Google Drive connector to trigger registration (local connectors are now Lua plugins)
	_ "github.com/fulgidus/revoco/connectors/googledrive"
)

var (
	flagConnectorSession  string
	flagConnectorType     string
	flagConnectorName     string
	flagConnectorRole     string
	flagConnectorSettings string
	flagConnectorID       string
	flagImportDest        string
	flagImportMode        string
	flagConnectorTimeout  int // timeout in seconds for test/auth commands
)

// connectorCmd is the parent command for connector operations.
var connectorCmd = &cobra.Command{
	Use:   "connector",
	Short: "Manage session connectors",
	Long: `Manage connectors for data import and export within sessions.

Connectors are data sources (inputs) and destinations (outputs) that can be
configured per session. Available connector types include:
  - local-folder: Read/write files from local directories
  - local-zip: Read files from ZIP archives
  - local-multi-zip: Read files from multiple ZIP archives
  - local-tgz: Read files from .tar.gz archives
  - google-drive: Import from Google Drive (requires OAuth)

Examples:
  # List available connector types
  revoco connector types

  # Add a ZIP connector to a session
  revoco connector add --session mydata --type local-multi-zip \
    --name "Takeout Archives" --role input \
    --settings '{"paths": ["/path/to/file1.zip", "/path/to/file2.zip"]}'

  # List connectors in a session
  revoco connector list --session mydata

  # Import data from configured input connectors
  revoco connector import --session mydata --dest ./data
`,
}

// connectorTypesCmd lists available connector types.
var connectorTypesCmd = &cobra.Command{
	Use:   "types",
	Short: "List available connector types",
	RunE: func(cmd *cobra.Command, args []string) error {
		connectors := core.ListConnectors()
		if len(connectors) == 0 {
			fmt.Println("No connector types registered.")
			return nil
		}

		fmt.Println("Available connector types:")
		fmt.Println()
		for _, info := range connectors {
			fmt.Printf("  %-20s %s\n", info.ID, info.Name)
			fmt.Printf("  %-20s %s\n", "", info.Description)

			// Show capabilities
			var caps []string
			for _, c := range info.Capabilities {
				caps = append(caps, string(c))
			}
			fmt.Printf("  %-20s Capabilities: %s\n", "", strings.Join(caps, ", "))

			// Show data types
			var types []string
			for _, t := range info.DataTypes {
				types = append(types, string(t))
			}
			fmt.Printf("  %-20s Data types: %s\n", "", strings.Join(types, ", "))

			// Show auth requirement
			if info.RequiresAuth {
				fmt.Printf("  %-20s Auth: %s\n", "", info.AuthType)
			}
			fmt.Println()
		}
		return nil
	},
}

// connectorSchemaCmd shows the config schema for a connector type.
var connectorSchemaCmd = &cobra.Command{
	Use:   "schema <type>",
	Short: "Show configuration schema for a connector type",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		connectorID := args[0]

		info, ok := core.GetConnectorInfo(connectorID)
		if !ok {
			return fmt.Errorf("unknown connector type: %s", connectorID)
		}

		// Create an instance to get the config schema
		connector, err := core.CreateConnector(connectorID)
		if err != nil {
			return fmt.Errorf("create connector: %w", err)
		}

		fmt.Printf("Configuration schema for %s (%s):\n\n", info.Name, info.ID)

		schema := connector.ConfigSchema()
		if len(schema) == 0 {
			fmt.Println("  No configuration options.")
			return nil
		}

		for _, opt := range schema {
			reqStr := ""
			if opt.Required {
				reqStr = " (required)"
			}
			fmt.Printf("  %s%s\n", opt.ID, reqStr)
			fmt.Printf("    Name: %s\n", opt.Name)
			fmt.Printf("    Type: %s\n", opt.Type)
			if opt.Description != "" {
				fmt.Printf("    Description: %s\n", opt.Description)
			}
			if opt.Default != nil {
				fmt.Printf("    Default: %v\n", opt.Default)
			}
			if len(opt.Options) > 0 {
				fmt.Printf("    Options: %s\n", strings.Join(opt.Options, ", "))
			}
			fmt.Println()
		}
		return nil
	},
}

// connectorAddCmd adds a connector to a session.
var connectorAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a connector to a session",
	Long: `Add a connector to a session for data import or export.

The connector configuration is specified via --settings as a JSON object.
Use 'revoco connector schema <type>' to see available options.

Roles:
  - input: Primary data source (for importing data)
  - output: Primary data destination (for exporting data)
  - fallback: Secondary source for repairing missing data
  - input+output: Both input and output

Examples:
  # Add a multi-ZIP connector for Google Takeout
  revoco connector add --session takeout --type local-multi-zip \
    --name "Google Photos" --role input \
    --settings '{"paths": ["/home/user/Downloads/takeout-001.zip", "/home/user/Downloads/takeout-002.zip"]}'

  # Add an output folder
  revoco connector add --session takeout --type local-folder \
    --name "Processed Output" --role output \
    --settings '{"path": "/home/user/Photos/Organized"}'
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConnectorSession == "" {
			return fmt.Errorf("--session is required")
		}
		if flagConnectorType == "" {
			return fmt.Errorf("--type is required")
		}

		// Load or create session
		s, err := session.Load(flagConnectorSession)
		if err != nil {
			// Try to create V2 session if it doesn't exist
			s, err = session.CreateV2(flagConnectorSession)
			if err != nil {
				return fmt.Errorf("load/create session: %w", err)
			}
			fmt.Printf("Created new session: %s\n", flagConnectorSession)
		}

		// Verify connector type exists
		info, ok := core.GetConnectorInfo(flagConnectorType)
		if !ok {
			return fmt.Errorf("unknown connector type: %s (use 'revoco connector types' to list)", flagConnectorType)
		}

		// Parse settings
		var settings map[string]any
		if flagConnectorSettings != "" {
			if err := json.Unmarshal([]byte(flagConnectorSettings), &settings); err != nil {
				return fmt.Errorf("invalid --settings JSON: %w", err)
			}
		} else {
			settings = make(map[string]any)
		}

		// Parse roles
		roles := core.ConnectorRoles{}
		for _, role := range strings.Split(flagConnectorRole, "+") {
			switch strings.TrimSpace(strings.ToLower(role)) {
			case "input":
				roles.IsInput = true
			case "output":
				roles.IsOutput = true
			case "fallback":
				roles.IsFallback = true
			case "":
				// ignore empty
			default:
				return fmt.Errorf("invalid role: %s (use input, output, fallback, or combinations like input+output)", role)
			}
		}

		// Default to input if no role specified
		if !roles.HasAnyRole() {
			roles.IsInput = true
		}

		// Generate instance ID
		instanceID := uuid.New().String()[:8]

		// Determine name
		name := flagConnectorName
		if name == "" {
			name = info.Name
		}

		// Create connector config
		cfg := core.ConnectorConfig{
			ConnectorID: flagConnectorType,
			InstanceID:  instanceID,
			Name:        name,
			Roles:       roles,
			Settings:    settings,
			Enabled:     true,
		}

		// Validate the configuration
		connector, err := core.CreateConnector(flagConnectorType)
		if err != nil {
			return fmt.Errorf("create connector: %w", err)
		}
		if err := connector.ValidateConfig(cfg); err != nil {
			return fmt.Errorf("invalid configuration: %w", err)
		}

		// Add to session
		s.AddConnector(cfg)
		if err := s.Save(); err != nil {
			return fmt.Errorf("save session: %w", err)
		}

		fmt.Printf("Added connector %q (id: %s, type: %s, role: %s)\n",
			name, instanceID, flagConnectorType, roles.String())
		return nil
	},
}

// connectorListCmd lists connectors in a session.
var connectorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List connectors in a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConnectorSession == "" {
			return fmt.Errorf("--session is required")
		}

		s, err := session.Load(flagConnectorSession)
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		connectors := s.ListConnectors()
		if len(connectors) == 0 {
			fmt.Printf("No connectors in session %q.\n", flagConnectorSession)
			fmt.Println("Use 'revoco connector add --session <name> ...' to add one.")
			return nil
		}

		fmt.Printf("Connectors in session %q:\n\n", flagConnectorSession)
		for _, c := range connectors {
			status := "enabled"
			if !c.Enabled {
				status = "disabled"
			}
			fmt.Printf("  ID: %s\n", c.InstanceID)
			fmt.Printf("  Name: %s\n", c.Name)
			fmt.Printf("  Type: %s\n", c.ConnectorID)
			fmt.Printf("  Role: %s\n", c.Roles.String())
			fmt.Printf("  Status: %s\n", status)

			// Show key settings
			if len(c.Settings) > 0 {
				fmt.Println("  Settings:")
				for k, v := range c.Settings {
					fmt.Printf("    %s: %v\n", k, v)
				}
			}
			fmt.Println()
		}
		return nil
	},
}

// connectorRemoveCmd removes a connector from a session.
var connectorRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a connector from a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConnectorSession == "" {
			return fmt.Errorf("--session is required")
		}
		if flagConnectorID == "" {
			return fmt.Errorf("--id is required")
		}

		s, err := session.Load(flagConnectorSession)
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		if !s.RemoveConnector(flagConnectorID) {
			return fmt.Errorf("connector %q not found in session", flagConnectorID)
		}

		if err := s.Save(); err != nil {
			return fmt.Errorf("save session: %w", err)
		}

		fmt.Printf("Removed connector %q from session %q\n", flagConnectorID, flagConnectorSession)
		return nil
	},
}

// connectorEnableCmd enables a connector in a session.
var connectorEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable a connector in a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConnectorSession == "" {
			return fmt.Errorf("--session is required")
		}
		if flagConnectorID == "" {
			return fmt.Errorf("--id is required")
		}

		s, err := session.Load(flagConnectorSession)
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		if !s.Config.Connectors.EnableConnector(flagConnectorID) {
			return fmt.Errorf("connector %q not found in session", flagConnectorID)
		}

		if err := s.Save(); err != nil {
			return fmt.Errorf("save session: %w", err)
		}

		fmt.Printf("Enabled connector %q in session %q\n", flagConnectorID, flagConnectorSession)
		return nil
	},
}

// connectorDisableCmd disables a connector in a session.
var connectorDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable a connector in a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConnectorSession == "" {
			return fmt.Errorf("--session is required")
		}
		if flagConnectorID == "" {
			return fmt.Errorf("--id is required")
		}

		s, err := session.Load(flagConnectorSession)
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		if !s.Config.Connectors.DisableConnector(flagConnectorID) {
			return fmt.Errorf("connector %q not found in session", flagConnectorID)
		}

		if err := s.Save(); err != nil {
			return fmt.Errorf("save session: %w", err)
		}

		fmt.Printf("Disabled connector %q in session %q\n", flagConnectorID, flagConnectorSession)
		return nil
	},
}

// connectorTestCmd tests a connector configuration.
var connectorTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Test a connector's connection",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConnectorSession == "" {
			return fmt.Errorf("--session is required")
		}
		if flagConnectorID == "" {
			return fmt.Errorf("--id is required")
		}

		s, err := session.Load(flagConnectorSession)
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		cfg, ok := s.GetConnector(flagConnectorID)
		if !ok {
			return fmt.Errorf("connector %q not found in session", flagConnectorID)
		}

		// Create connector instance
		connector, err := core.CreateConnector(cfg.ConnectorID)
		if err != nil {
			return fmt.Errorf("create connector: %w", err)
		}

		// Test if it supports testing
		tester, ok := connector.(core.ConnectorTester)
		if !ok {
			return fmt.Errorf("connector %q does not support connection testing", cfg.ConnectorID)
		}

		// Print OAuth-aware messaging
		info, _ := core.GetConnectorInfo(cfg.ConnectorID)
		if info != nil && info.RequiresAuth && info.AuthType == "oauth2" {
			fmt.Printf("Testing connector %q (%s)...\n", cfg.Name, cfg.ConnectorID)
			fmt.Println("This connector requires OAuth authentication.")
			fmt.Println("Check your browser for the authentication prompt...")
			fmt.Printf("(Timeout: %ds — use --timeout to change)\n", flagConnectorTimeout)
		} else {
			fmt.Printf("Testing connector %q (%s)...\n", cfg.Name, cfg.ConnectorID)
		}

		timeout := time.Duration(flagConnectorTimeout) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := tester.TestConnection(ctx, cfg); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			return err
		}

		fmt.Println("SUCCESS: Connection test passed")
		return nil
	},
}

// connectorAuthCmd authenticates with an OAuth-based connector.
var connectorAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with an OAuth-based connector",
	Long: `Trigger the OAuth authentication flow for a connector that requires it.

This opens your browser for authentication and waits for the callback.
Use --timeout to allow more time for completing the browser flow (default: 300s).

Examples:
  # Authenticate a Google Drive connector
  revoco connector auth --session mydata --id abc123

  # With a longer timeout
  revoco connector auth --session mydata --id abc123 --timeout 600
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConnectorSession == "" {
			return fmt.Errorf("--session is required")
		}
		if flagConnectorID == "" {
			return fmt.Errorf("--id is required")
		}

		s, err := session.Load(flagConnectorSession)
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		cfg, ok := s.GetConnector(flagConnectorID)
		if !ok {
			return fmt.Errorf("connector %q not found in session", flagConnectorID)
		}

		// Verify this connector requires OAuth
		info, _ := core.GetConnectorInfo(cfg.ConnectorID)
		if info == nil {
			return fmt.Errorf("unknown connector type: %s", cfg.ConnectorID)
		}
		if !info.RequiresAuth || info.AuthType != "oauth2" {
			return fmt.Errorf("connector %q (%s) does not use OAuth authentication", cfg.Name, cfg.ConnectorID)
		}

		// Create connector instance
		connector, err := core.CreateConnector(cfg.ConnectorID)
		if err != nil {
			return fmt.Errorf("create connector: %w", err)
		}

		// Must support testing (TestConnection triggers the OAuth flow)
		tester, ok := connector.(core.ConnectorTester)
		if !ok {
			return fmt.Errorf("connector %q does not support authentication testing", cfg.ConnectorID)
		}

		// Use auth-specific timeout (default 300s, much longer than test)
		timeout := flagConnectorTimeout
		if !cmd.Flags().Changed("timeout") {
			timeout = 300 // default for auth is 5 minutes
		}

		fmt.Printf("Authenticating connector %q (%s)...\n", cfg.Name, cfg.ConnectorID)
		fmt.Println("Opening your browser for OAuth authentication...")
		fmt.Printf("You have %ds to complete the flow. Use --timeout to adjust.\n", timeout)

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		if err := tester.TestConnection(ctx, cfg); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			return err
		}

		fmt.Println("SUCCESS: Authentication completed")
		return nil
	},
}

// connectorImportCmd imports data from input connectors.
var connectorImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import data from session's input connectors",
	Long: `Import data from all enabled input connectors in a session.

This command retrieves data from configured input connectors and saves
it to the specified destination directory (or session's data directory).

Import modes:
  - copy: Copy files to destination (default, non-destructive)
  - move: Move files to destination (destructive for local sources)
  - reference: Create symlinks to original files (local sources only)

Examples:
  # Import to session's data directory
  revoco connector import --session takeout

  # Import to a specific directory
  revoco connector import --session takeout --dest /mnt/backup/photos

  # Dry run - show what would be imported
  revoco connector import --session takeout --dry-run
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagConnectorSession == "" {
			return fmt.Errorf("--session is required")
		}

		s, err := session.Load(flagConnectorSession)
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		inputs := s.GetInputConnectors()
		if len(inputs) == 0 {
			return fmt.Errorf("no input connectors configured in session %q", flagConnectorSession)
		}

		// Determine destination
		destDir := flagImportDest
		if destDir == "" {
			destDir = s.DataDir()
		}

		// Parse import mode
		importMode := core.ImportModeCopy
		switch strings.ToLower(flagImportMode) {
		case "copy", "":
			importMode = core.ImportModeCopy
		case "move":
			importMode = core.ImportModeMove
		case "reference", "ref", "link":
			importMode = core.ImportModeReference
		default:
			return fmt.Errorf("invalid --mode: %s (use copy, move, or reference)", flagImportMode)
		}

		// Create destination directory
		if !flagDryRun {
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				return fmt.Errorf("create destination: %w", err)
			}
		}

		fmt.Printf("Importing from %d input connector(s) to %s...\n", len(inputs), destDir)
		if flagDryRun {
			fmt.Println("[DRY RUN - no files will be written]")
		}

		ctx := context.Background()
		totalItems := 0
		totalSize := int64(0)

		for _, cfg := range inputs {
			fmt.Printf("\nConnector: %s (%s)\n", cfg.Name, cfg.ConnectorID)

			// Create and initialize connector
			connector, err := core.CreateConnector(cfg.ConnectorID)
			if err != nil {
				fmt.Printf("  ERROR: create connector: %v\n", err)
				continue
			}

			reader, ok := connector.(core.ConnectorReader)
			if !ok {
				fmt.Printf("  ERROR: connector does not support reading\n")
				continue
			}

			// Initialize
			if err := reader.Initialize(ctx, cfg); err != nil {
				fmt.Printf("  ERROR: initialize: %v\n", err)
				continue
			}
			defer reader.Close()

			// List items
			fmt.Printf("  Scanning...\n")
			items, err := reader.List(ctx, func(done, total int) {
				if total > 0 {
					fmt.Printf("\r  Scanning: %d/%d", done, total)
				} else {
					fmt.Printf("\r  Scanning: %d items", done)
				}
			})
			fmt.Println()
			if err != nil {
				fmt.Printf("  ERROR: list items: %v\n", err)
				continue
			}

			fmt.Printf("  Found %d items\n", len(items))

			if flagDryRun {
				// Just count and show summary
				for _, item := range items {
					totalItems++
					totalSize += item.Size
				}
				continue
			}

			// Import each item
			imported := 0
			failed := 0
			for i, item := range items {
				destPath := fmt.Sprintf("%s/%s", destDir, item.ID)

				if err := reader.ReadTo(ctx, item, destPath, importMode); err != nil {
					failed++
					if failed <= 5 {
						fmt.Printf("  ERROR [%s]: %v\n", item.ID, err)
					}
				} else {
					imported++
					totalItems++
					totalSize += item.Size
				}

				// Progress every 100 items or at end
				if (i+1)%100 == 0 || i == len(items)-1 {
					fmt.Printf("\r  Progress: %d/%d (failed: %d)", imported, len(items), failed)
				}
			}
			fmt.Println()

			if failed > 5 {
				fmt.Printf("  ... and %d more errors\n", failed-5)
			}
		}

		fmt.Printf("\nImport complete: %d items, %.2f MB\n", totalItems, float64(totalSize)/(1024*1024))
		return nil
	},
}

func init() {
	// Common flags
	connectorCmd.PersistentFlags().StringVar(&flagConnectorSession, "session", "", "Session name")

	// Add command flags
	connectorAddCmd.Flags().StringVar(&flagConnectorType, "type", "", "Connector type (required)")
	connectorAddCmd.Flags().StringVar(&flagConnectorName, "name", "", "Display name for the connector")
	connectorAddCmd.Flags().StringVar(&flagConnectorRole, "role", "input", "Role: input, output, fallback, or combinations (e.g., input+output)")
	connectorAddCmd.Flags().StringVar(&flagConnectorSettings, "settings", "", "Connector settings as JSON object")

	// Remove/enable/disable/test flags
	connectorRemoveCmd.Flags().StringVar(&flagConnectorID, "id", "", "Connector instance ID (required)")
	connectorEnableCmd.Flags().StringVar(&flagConnectorID, "id", "", "Connector instance ID (required)")
	connectorDisableCmd.Flags().StringVar(&flagConnectorID, "id", "", "Connector instance ID (required)")
	connectorTestCmd.Flags().StringVar(&flagConnectorID, "id", "", "Connector instance ID (required)")
	connectorTestCmd.Flags().IntVar(&flagConnectorTimeout, "timeout", 120, "Timeout in seconds (default: 120)")

	// Auth command flags
	connectorAuthCmd.Flags().StringVar(&flagConnectorID, "id", "", "Connector instance ID (required)")
	connectorAuthCmd.Flags().IntVar(&flagConnectorTimeout, "timeout", 300, "Timeout in seconds (default: 300)")
	connectorAuthCmd.Flags().StringVar(&flagConnectorSession, "session", "", "Session name")

	// Import flags
	connectorImportCmd.Flags().StringVar(&flagImportDest, "dest", "", "Destination directory (default: session data dir)")
	connectorImportCmd.Flags().StringVar(&flagImportMode, "mode", "copy", "Import mode: copy, move, or reference")
	connectorImportCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would be imported without writing files")

	// Build command tree
	connectorCmd.AddCommand(connectorTypesCmd)
	connectorCmd.AddCommand(connectorSchemaCmd)
	connectorCmd.AddCommand(connectorAddCmd)
	connectorCmd.AddCommand(connectorListCmd)
	connectorCmd.AddCommand(connectorRemoveCmd)
	connectorCmd.AddCommand(connectorEnableCmd)
	connectorCmd.AddCommand(connectorDisableCmd)
	connectorCmd.AddCommand(connectorTestCmd)
	connectorCmd.AddCommand(connectorAuthCmd)
	connectorCmd.AddCommand(connectorImportCmd)

	// Add to root
	rootCmd.AddCommand(connectorCmd)
}
