// Package outputs provides output modules for Passwords Takeout data.
package outputs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/passwords/metadata"
)

func init() {
	// Register all outputs
	core.RegisterOutput(NewKeePassCSVOutput())
	core.RegisterOutput(NewJSONOutput())
}

// ── KeePass CSV Output ────────────────────────────────────────────────────────

// KeePassCSVOutput exports passwords in KeePass-compatible CSV format.
type KeePassCSVOutput struct {
	destDir string
}

// NewKeePassCSVOutput creates a new KeePass CSV output.
func NewKeePassCSVOutput() *KeePassCSVOutput {
	return &KeePassCSVOutput{}
}

func (o *KeePassCSVOutput) ID() string   { return "passwords-keepass-csv" }
func (o *KeePassCSVOutput) Name() string { return "KeePass CSV Export" }
func (o *KeePassCSVOutput) Description() string {
	return "Export passwords to KeePass-compatible CSV format"
}

func (o *KeePassCSVOutput) SupportedItemTypes() []string {
	return []string{"passwords_library"}
}

func (o *KeePassCSVOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for CSV file",
			Type:        "string",
			Required:    true,
		},
	}
}

func (o *KeePassCSVOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *KeePassCSVOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// CSV output is batch-only
	return nil
}

func (o *KeePassCSVOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	// Extract library from items
	var library *metadata.PasswordLibrary
	for _, item := range items {
		if item.Type == "passwords_library" {
			if lib, ok := item.Metadata["passwords_library"].(*metadata.PasswordLibrary); ok {
				library = lib
				break
			}
		}
	}

	if library == nil {
		return fmt.Errorf("no password library found in items")
	}

	// Write KeePass CSV
	outputPath := filepath.Join(o.destDir, "passwords-keepass.csv")
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create CSV file: %w", err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// KeePass CSV format: Group,Title,Username,Password,URL,Notes
	// Column order is CRITICAL for KeePass import
	if err := writer.Write([]string{"Group", "Title", "Username", "Password", "URL", "Notes"}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, entry := range library.Entries {
		title := entry.Name
		if title == "" {
			title = entry.URL
		}

		record := []string{
			"Google Passwords", // Group - all entries in one group
			title,              // Title
			entry.Username,     // Username
			entry.Password,     // Password (plaintext)
			entry.URL,          // URL
			entry.Note,         // Notes
		}

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write record: %w", err)
		}
	}

	if progress != nil {
		progress(len(items), len(items))
	}

	return nil
}

func (o *KeePassCSVOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── JSON Output ───────────────────────────────────────────────────────────────

// JSONOutput exports passwords as structured JSON with security warning.
type JSONOutput struct {
	destDir string
	pretty  bool
}

// NewJSONOutput creates a new JSON output.
func NewJSONOutput() *JSONOutput {
	return &JSONOutput{
		pretty: true,
	}
}

func (o *JSONOutput) ID() string   { return "passwords-json" }
func (o *JSONOutput) Name() string { return "Passwords JSON Export" }
func (o *JSONOutput) Description() string {
	return "Export passwords to structured JSON (WARNING: plaintext)"
}

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"passwords_library"}
}

func (o *JSONOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for JSON file",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "pretty",
			Name:        "Pretty Print",
			Description: "Format JSON with indentation",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (o *JSONOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["pretty"].(bool); ok {
		o.pretty = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *JSONOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// JSON output is batch-only
	return nil
}

func (o *JSONOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	// Extract library from items
	var library *metadata.PasswordLibrary
	for _, item := range items {
		if item.Type == "passwords_library" {
			if lib, ok := item.Metadata["passwords_library"].(*metadata.PasswordLibrary); ok {
				library = lib
				break
			}
		}
	}

	if library == nil {
		return fmt.Errorf("no password library found in items")
	}

	// Write JSON file
	outputPath := filepath.Join(o.destDir, "passwords.json")
	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(library, "", "  ")
	} else {
		data, err = json.Marshal(library)
	}
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}

	// Write security warning file
	warningPath := filepath.Join(o.destDir, "SECURITY_WARNING.txt")
	warning := `WARNING: PASSWORDS STORED IN PLAINTEXT

This JSON file contains your passwords in plaintext format.

SECURITY RECOMMENDATIONS:
1. Secure this file immediately with appropriate file permissions
2. Encrypt this file if storing long-term
3. Delete this file after importing into a password manager
4. Never commit this file to version control
5. Never share this file over unsecured channels

For better security, use the KeePass CSV export format and import
into an encrypted password manager like KeePass, Bitwarden, or 1Password.
`
	os.WriteFile(warningPath, []byte(warning), 0o644)

	if progress != nil {
		progress(len(items), len(items))
	}

	return nil
}

func (o *JSONOutput) Finalize(ctx context.Context) error {
	return nil
}
