// Package outputs provides output modules for Keep data.
package outputs

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/keep/metadata"
)

// ── Markdown Output ──────────────────────────────────────────────────────────

// MarkdownOutput exports Keep notes to Markdown files.
type MarkdownOutput struct {
	destDir         string
	includeMetadata bool
	organizeByLabel bool
}

// NewMarkdown creates a new Markdown output.
func NewMarkdown() *MarkdownOutput {
	return &MarkdownOutput{
		includeMetadata: true,
		organizeByLabel: false,
	}
}

func (o *MarkdownOutput) ID() string          { return "keep-md" }
func (o *MarkdownOutput) Name() string        { return "Keep Markdown Export" }
func (o *MarkdownOutput) Description() string { return "Export notes to Markdown format" }

func (o *MarkdownOutput) SupportedItemTypes() []string {
	return []string{"note"}
}

func (o *MarkdownOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for Markdown files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "include_metadata",
			Name:        "Include Metadata",
			Description: "Include timestamps and labels in Markdown",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "organize_by_label",
			Name:        "Organize by Label",
			Description: "Create subdirectories for each label",
			Type:        "bool",
			Default:     false,
		},
	}
}

func (o *MarkdownOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["include_metadata"].(bool); ok {
		o.includeMetadata = v
	}

	if v, ok := cfg.Settings["organize_by_label"].(bool); ok {
		o.organizeByLabel = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *MarkdownOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "note" {
		return nil
	}

	// Extract note from metadata
	noteData, ok := item.Metadata["note"]
	if !ok {
		return fmt.Errorf("missing note data in metadata")
	}

	note, ok := noteData.(metadata.Note)
	if !ok {
		return fmt.Errorf("invalid note type in metadata")
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)

	// Organize by label if requested
	if o.organizeByLabel && len(note.Labels) > 0 {
		labelDir := filepath.Join(o.destDir, sanitizeFilename(note.Labels[0].Name))
		destPath = filepath.Join(labelDir, filepath.Base(item.DestRelPath))
	}

	// Ensure .md extension
	if !strings.HasSuffix(destPath, ".md") {
		destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".md"
	}

	os.MkdirAll(filepath.Dir(destPath), 0o755)

	// Convert to Markdown
	markdown := note.ToMarkdown(o.includeMetadata)

	return os.WriteFile(destPath, []byte(markdown), 0o644)
}

func (o *MarkdownOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
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

func (o *MarkdownOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── JSON Output ──────────────────────────────────────────────────────────────

// JSONOutput exports Keep notes to a single JSON file.
type JSONOutput struct {
	destPath string
	pretty   bool
}

// NewJSON creates a new JSON output.
func NewJSON() *JSONOutput {
	return &JSONOutput{pretty: true}
}

func (o *JSONOutput) ID() string          { return "keep-json" }
func (o *JSONOutput) Name() string        { return "Keep JSON Export" }
func (o *JSONOutput) Description() string { return "Export notes to JSON format" }

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"note"}
}

func (o *JSONOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_path",
			Name:        "Destination File",
			Description: "Output JSON file path (e.g., notes.json)",
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
	o.destPath = cfg.DestDir
	if o.destPath == "" {
		if d, ok := cfg.Settings["dest_path"].(string); ok {
			o.destPath = d
		}
	}
	if o.destPath == "" {
		o.destPath = filepath.Join(cfg.DestDir, "notes.json")
	}

	if v, ok := cfg.Settings["pretty"].(bool); ok {
		o.pretty = v
	}

	return os.MkdirAll(filepath.Dir(o.destPath), 0o755)
}

func (o *JSONOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// Individual export not supported for batch JSON output
	return nil
}

func (o *JSONOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	var notes []metadata.Note

	for _, item := range items {
		if item.Type != "note" {
			continue
		}

		noteData, ok := item.Metadata["note"]
		if !ok {
			continue
		}

		note, ok := noteData.(metadata.Note)
		if !ok {
			continue
		}

		notes = append(notes, note)
	}

	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(notes, "", "  ")
	} else {
		data, err = json.Marshal(notes)
	}
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	if err := os.WriteFile(o.destPath, data, 0o644); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}

	if progress != nil {
		progress(len(notes), len(notes))
	}

	return nil
}

func (o *JSONOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── HTML Output ──────────────────────────────────────────────────────────────

// HTMLOutput exports Keep notes to clean HTML files.
type HTMLOutput struct {
	destDir         string
	organizeByLabel bool
	includeStyle    bool
}

// NewHTML creates a new HTML output.
func NewHTML() *HTMLOutput {
	return &HTMLOutput{
		organizeByLabel: false,
		includeStyle:    true,
	}
}

func (o *HTMLOutput) ID() string          { return "keep-html" }
func (o *HTMLOutput) Name() string        { return "Keep HTML Export" }
func (o *HTMLOutput) Description() string { return "Export notes to HTML format" }

func (o *HTMLOutput) SupportedItemTypes() []string {
	return []string{"note"}
}

func (o *HTMLOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for HTML files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "organize_by_label",
			Name:        "Organize by Label",
			Description: "Create subdirectories for each label",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "include_style",
			Name:        "Include Styles",
			Description: "Include inline CSS styling",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (o *HTMLOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
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

	if v, ok := cfg.Settings["include_style"].(bool); ok {
		o.includeStyle = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *HTMLOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "note" {
		return nil
	}

	noteData, ok := item.Metadata["note"]
	if !ok {
		return fmt.Errorf("missing note data in metadata")
	}

	note, ok := noteData.(metadata.Note)
	if !ok {
		return fmt.Errorf("invalid note type in metadata")
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)

	// Organize by label if requested
	if o.organizeByLabel && len(note.Labels) > 0 {
		labelDir := filepath.Join(o.destDir, sanitizeFilename(note.Labels[0].Name))
		destPath = filepath.Join(labelDir, filepath.Base(item.DestRelPath))
	}

	// Ensure .html extension
	if !strings.HasSuffix(destPath, ".html") {
		destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".html"
	}

	os.MkdirAll(filepath.Dir(destPath), 0o755)

	// Convert to HTML
	htmlContent := o.noteToHTML(note)

	return os.WriteFile(destPath, []byte(htmlContent), 0o644)
}

func (o *HTMLOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
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

func (o *HTMLOutput) Finalize(ctx context.Context) error {
	return nil
}

func (o *HTMLOutput) noteToHTML(note metadata.Note) string {
	var sb strings.Builder

	sb.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	sb.WriteString("<meta charset=\"UTF-8\">\n")
	sb.WriteString("<title>")
	sb.WriteString(html.EscapeString(note.Title))
	sb.WriteString("</title>\n")

	if o.includeStyle {
		sb.WriteString(`<style>
body { font-family: Arial, sans-serif; max-width: 800px; margin: 40px auto; padding: 20px; }
h1 { color: #333; }
.metadata { color: #666; font-size: 0.9em; margin-bottom: 20px; }
.labels { margin: 10px 0; }
.label { background: #e0e0e0; padding: 3px 8px; border-radius: 3px; margin-right: 5px; }
.content { line-height: 1.6; }
.checkboxes { list-style: none; padding-left: 0; }
.checkboxes li { margin: 5px 0; }
.attachments { margin-top: 20px; }
.annotations { margin-top: 20px; }
</style>
`)
	}

	sb.WriteString("</head>\n<body>\n")

	// Title
	if note.Title != "" {
		sb.WriteString("<h1>")
		sb.WriteString(html.EscapeString(note.Title))
		sb.WriteString("</h1>\n")
	}

	// Metadata
	sb.WriteString("<div class=\"metadata\">\n")
	if !note.Created.IsZero() {
		sb.WriteString("<p>Created: ")
		sb.WriteString(html.EscapeString(note.Created.Format("2006-01-02 15:04:05")))
		sb.WriteString("</p>\n")
	}
	if !note.Modified.IsZero() {
		sb.WriteString("<p>Modified: ")
		sb.WriteString(html.EscapeString(note.Modified.Format("2006-01-02 15:04:05")))
		sb.WriteString("</p>\n")
	}
	if len(note.Labels) > 0 {
		sb.WriteString("<div class=\"labels\">Labels: ")
		for _, label := range note.Labels {
			sb.WriteString("<span class=\"label\">")
			sb.WriteString(html.EscapeString(label.Name))
			sb.WriteString("</span>")
		}
		sb.WriteString("</div>\n")
	}
	sb.WriteString("</div>\n")

	// Content
	sb.WriteString("<div class=\"content\">\n")
	if note.TextContent != "" {
		sb.WriteString("<p>")
		sb.WriteString(html.EscapeString(note.TextContent))
		sb.WriteString("</p>\n")
	}

	// Checkboxes
	if len(note.Checkboxes) > 0 {
		sb.WriteString("<ul class=\"checkboxes\">\n")
		for _, item := range note.Checkboxes {
			sb.WriteString("<li>")
			if item.Checked {
				sb.WriteString("☑ ")
			} else {
				sb.WriteString("☐ ")
			}
			sb.WriteString(html.EscapeString(item.Text))
			sb.WriteString("</li>\n")
		}
		sb.WriteString("</ul>\n")
	}
	sb.WriteString("</div>\n")

	// Annotations
	if len(note.Annotations) > 0 {
		sb.WriteString("<div class=\"annotations\">\n<h2>Links</h2>\n<ul>\n")
		for _, ann := range note.Annotations {
			sb.WriteString("<li><a href=\"")
			sb.WriteString(html.EscapeString(ann.URL))
			sb.WriteString("\">")
			if ann.Title != "" {
				sb.WriteString(html.EscapeString(ann.Title))
			} else {
				sb.WriteString(html.EscapeString(ann.URL))
			}
			sb.WriteString("</a>")
			if ann.Description != "" {
				sb.WriteString(" - ")
				sb.WriteString(html.EscapeString(ann.Description))
			}
			sb.WriteString("</li>\n")
		}
		sb.WriteString("</ul>\n</div>\n")
	}

	// Attachments
	if len(note.Attachments) > 0 {
		sb.WriteString("<div class=\"attachments\">\n<h2>Attachments</h2>\n<ul>\n")
		for _, att := range note.Attachments {
			sb.WriteString("<li>")
			sb.WriteString(html.EscapeString(att.FilePath))
			sb.WriteString(" (")
			sb.WriteString(html.EscapeString(att.MimeType))
			sb.WriteString(")</li>\n")
		}
		sb.WriteString("</ul>\n</div>\n")
	}

	sb.WriteString("</body>\n</html>\n")

	return sb.String()
}

// ── Helper functions ─────────────────────────────────────────────────────────

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
		"\n", "_",
		"\r", "_",
		"\t", "_",
	)
	return replacer.Replace(name)
}

// ── Registration ─────────────────────────────────────────────────────────────

func init() {
	// Register outputs on import
	core.RegisterOutput(NewMarkdown())
	core.RegisterOutput(NewJSON())
	core.RegisterOutput(NewHTML())
}
