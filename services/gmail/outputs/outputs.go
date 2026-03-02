// Package outputs provides output modules for Gmail data.
package outputs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/gmail/metadata"
)

// ── JSON Output ──────────────────────────────────────────────────────────────

// JSONOutput exports Gmail messages to JSON files.
type JSONOutput struct {
	destDir string
	pretty  bool
}

// NewJSON creates a new JSON output.
func NewJSON() *JSONOutput {
	return &JSONOutput{pretty: true}
}

func (o *JSONOutput) ID() string          { return "gmail-json" }
func (o *JSONOutput) Name() string        { return "Gmail JSON Export" }
func (o *JSONOutput) Description() string { return "Export email messages to JSON format" }

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"email"}
}

func (o *JSONOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for JSON files",
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
	if item.Type != "email" {
		return nil
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)
	os.MkdirAll(filepath.Dir(destPath), 0o755)

	// Ensure .json extension
	if !strings.HasSuffix(destPath, ".json") {
		destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".json"
	}

	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(item.Metadata, "", "  ")
	} else {
		data, err = json.Marshal(item.Metadata)
	}
	if err != nil {
		return err
	}

	return os.WriteFile(destPath, data, 0o644)
}

func (o *JSONOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *JSONOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── EML Output ───────────────────────────────────────────────────────────────

// EMLOutput exports Gmail messages as individual .eml files.
type EMLOutput struct {
	destDir         string
	organizeByLabel bool
}

// NewEML creates a new EML output.
func NewEML() *EMLOutput {
	return &EMLOutput{organizeByLabel: true}
}

func (o *EMLOutput) ID() string          { return "gmail-eml" }
func (o *EMLOutput) Name() string        { return "Gmail EML Export" }
func (o *EMLOutput) Description() string { return "Export messages as .eml files" }

func (o *EMLOutput) SupportedItemTypes() []string {
	return []string{"email"}
}

func (o *EMLOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for .eml files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "organize_by_label",
			Name:        "Organize by Label",
			Description: "Create subdirectories for each Gmail label",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (o *EMLOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["organize_by_label"].(bool); ok {
		o.organizeByLabel = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *EMLOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "email" {
		return nil
	}

	// Extract message from metadata
	msgData, ok := item.Metadata["message"]
	if !ok {
		return nil
	}

	msg, ok := msgData.(metadata.EmailMessage)
	if !ok {
		return nil
	}

	// Determine output directory
	destDir := o.destDir
	if o.organizeByLabel && len(msg.Labels) > 0 {
		destDir = filepath.Join(o.destDir, sanitizeFilename(msg.Labels[0]))
		os.MkdirAll(destDir, 0o755)
	}

	// Create .eml filename
	filename := sanitizeFilename(msg.MessageID)
	if filename == "" {
		filename = "message"
	}
	emlPath := filepath.Join(destDir, filename+".eml")

	// For now, write metadata as JSON (full .eml reconstruction would require
	// original message body from MBOX parsing phase)
	// In production, this would write the raw RFC 822 message
	data, _ := json.MarshalIndent(msg, "", "  ")
	return os.WriteFile(emlPath, data, 0o644)
}

func (o *EMLOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *EMLOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── CSV Output ───────────────────────────────────────────────────────────────

// CSVOutput exports Gmail messages to CSV format.
type CSVOutput struct {
	destDir  string
	filename string
}

// NewCSV creates a new CSV output.
func NewCSV() *CSVOutput {
	return &CSVOutput{filename: "messages.csv"}
}

func (o *CSVOutput) ID() string          { return "gmail-csv" }
func (o *CSVOutput) Name() string        { return "Gmail CSV Export" }
func (o *CSVOutput) Description() string { return "Export message metadata to CSV" }

func (o *CSVOutput) SupportedItemTypes() []string {
	return []string{"email"}
}

func (o *CSVOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for CSV file",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "filename",
			Name:        "Filename",
			Description: "Name of the CSV file",
			Type:        "string",
			Default:     "messages.csv",
		},
	}
}

func (o *CSVOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["filename"].(string); ok && v != "" {
		o.filename = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *CSVOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// CSV output works in batch mode only
	return nil
}

func (o *CSVOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	csvPath := filepath.Join(o.destDir, o.filename)
	f, err := os.Create(csvPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Write header
	w.Write(metadata.CSVHeaders())

	// Write rows
	for i, item := range items {
		if item.Type != "email" {
			continue
		}

		msgData, ok := item.Metadata["message"]
		if !ok {
			continue
		}

		msg, ok := msgData.(metadata.EmailMessage)
		if !ok {
			continue
		}

		w.Write(msg.ToCSVRow())

		if progress != nil {
			progress(i+1, len(items))
		}
	}

	return nil
}

func (o *CSVOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	clean := replacer.Replace(name)
	// Remove angle brackets from Message-IDs
	clean = strings.Trim(clean, "<>")
	return clean
}

// Register all outputs
func init() {
	_ = core.RegisterOutput(NewJSON())
	_ = core.RegisterOutput(NewEML())
	_ = core.RegisterOutput(NewCSV())
}
