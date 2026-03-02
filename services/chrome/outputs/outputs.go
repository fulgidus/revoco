// Package outputs provides output modules for Chrome Takeout data.
package outputs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/chrome/metadata"
	"github.com/fulgidus/revoco/services/core"
)

func init() {
	// Register all outputs
	core.RegisterOutput(NewJSONOutput())
	core.RegisterOutput(NewHTMLOutput())
	core.RegisterOutput(NewCSVOutput())
}

// ── JSON Output ───────────────────────────────────────────────────────────────

// JSONOutput exports Chrome data as structured JSON.
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

func (o *JSONOutput) ID() string          { return "chrome-json" }
func (o *JSONOutput) Name() string        { return "Chrome JSON Export" }
func (o *JSONOutput) Description() string { return "Export Chrome data to structured JSON" }

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"bookmark", "browser_history"}
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
	// Group items by type
	bookmarks := []map[string]any{}
	history := []map[string]any{}

	for _, item := range items {
		switch item.Type {
		case "bookmark":
			bookmarks = append(bookmarks, item.Metadata)
		case "browser_history":
			history = append(history, item.Metadata)
		}
	}

	// Create output structure
	output := map[string]any{
		"bookmarks": bookmarks,
		"history":   history,
	}

	// Write JSON file
	outputPath := filepath.Join(o.destDir, "chrome-data.json")
	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(output, "", "  ")
	} else {
		data, err = json.Marshal(output)
	}
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}

	if progress != nil {
		progress(len(items), len(items))
	}

	return nil
}

func (o *JSONOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── HTML Output ───────────────────────────────────────────────────────────────

// HTMLOutput exports bookmarks as Netscape Bookmark HTML (importable format).
type HTMLOutput struct {
	destDir string
}

// NewHTMLOutput creates a new HTML output.
func NewHTMLOutput() *HTMLOutput {
	return &HTMLOutput{}
}

func (o *HTMLOutput) ID() string   { return "chrome-html" }
func (o *HTMLOutput) Name() string { return "Chrome HTML Bookmarks Export" }
func (o *HTMLOutput) Description() string {
	return "Export bookmarks as Netscape HTML format (importable)"
}

func (o *HTMLOutput) SupportedItemTypes() []string {
	return []string{"bookmark"}
}

func (o *HTMLOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for HTML file",
			Type:        "string",
			Required:    true,
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

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *HTMLOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// HTML output is batch-only
	return nil
}

func (o *HTMLOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	// Extract bookmarks from items
	var bookmarks []metadata.Bookmark
	for _, item := range items {
		if item.Type == "bookmark" {
			// Reconstruct bookmark from metadata
			bm := metadata.Bookmark{
				Name:          getString(item.Metadata, "name"),
				URL:           getString(item.Metadata, "url"),
				Folder:        getString(item.Metadata, "folder"),
				ParentFolders: getStringSlice(item.Metadata, "parent_folders"),
			}
			if dateAdded, ok := item.Metadata["date_added"]; ok {
				// Date is already a time.Time
				if t, ok := dateAdded.(string); ok {
					// If it's a string, skip parsing for now
					_ = t
				}
			}
			bookmarks = append(bookmarks, bm)
		}
	}

	// Build folder hierarchy
	folderTree := buildFolderTree(bookmarks)

	// Generate HTML
	outputPath := filepath.Join(o.destDir, "bookmarks.html")
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create HTML file: %w", err)
	}
	defer file.Close()

	// Write HTML header
	file.WriteString("<!DOCTYPE NETSCAPE-Bookmark-file-1>\n")
	file.WriteString("<META HTTP-EQUIV=\"Content-Type\" CONTENT=\"text/html; charset=UTF-8\">\n")
	file.WriteString("<TITLE>Bookmarks</TITLE>\n")
	file.WriteString("<H1>Bookmarks</H1>\n")
	file.WriteString("<DL><p>\n")

	// Write folder tree
	writeFolderTree(file, folderTree, 1)

	file.WriteString("</DL><p>\n")

	if progress != nil {
		progress(len(items), len(items))
	}

	return nil
}

func (o *HTMLOutput) Finalize(ctx context.Context) error {
	return nil
}

// folderNode represents a node in the bookmark folder tree.
type folderNode struct {
	Name      string
	Bookmarks []metadata.Bookmark
	Children  map[string]*folderNode
}

// buildFolderTree constructs a hierarchical folder structure from flat bookmark list.
func buildFolderTree(bookmarks []metadata.Bookmark) *folderNode {
	root := &folderNode{
		Name:     "root",
		Children: make(map[string]*folderNode),
	}

	for _, bm := range bookmarks {
		if len(bm.ParentFolders) == 0 {
			// Root-level bookmark
			root.Bookmarks = append(root.Bookmarks, bm)
			continue
		}

		// Navigate/create folder path
		current := root
		for _, folderName := range bm.ParentFolders {
			if current.Children[folderName] == nil {
				current.Children[folderName] = &folderNode{
					Name:     folderName,
					Children: make(map[string]*folderNode),
				}
			}
			current = current.Children[folderName]
		}
		current.Bookmarks = append(current.Bookmarks, bm)
	}

	return root
}

// writeFolderTree recursively writes the folder tree to HTML.
func writeFolderTree(file *os.File, node *folderNode, depth int) {
	indent := strings.Repeat("    ", depth)

	// Write bookmarks at this level
	for _, bm := range node.Bookmarks {
		addDate := ""
		if !bm.DateAdded.IsZero() {
			addDate = fmt.Sprintf(" ADD_DATE=\"%d\"", bm.DateAdded.Unix())
		}
		file.WriteString(fmt.Sprintf("%s<DT><A HREF=\"%s\"%s>%s</A>\n",
			indent, escapeHTML(bm.URL), addDate, escapeHTML(bm.Name)))
	}

	// Write child folders
	for _, child := range node.Children {
		file.WriteString(fmt.Sprintf("%s<DT><H3>%s</H3>\n", indent, escapeHTML(child.Name)))
		file.WriteString(fmt.Sprintf("%s<DL><p>\n", indent))
		writeFolderTree(file, child, depth+1)
		file.WriteString(fmt.Sprintf("%s</DL><p>\n", indent))
	}
}

// ── CSV Output ────────────────────────────────────────────────────────────────

// CSVOutput exports Chrome data to CSV files (separate files for bookmarks and history).
type CSVOutput struct {
	destDir string
}

// NewCSVOutput creates a new CSV output.
func NewCSVOutput() *CSVOutput {
	return &CSVOutput{}
}

func (o *CSVOutput) ID() string          { return "chrome-csv" }
func (o *CSVOutput) Name() string        { return "Chrome CSV Export" }
func (o *CSVOutput) Description() string { return "Export Chrome data to CSV files" }

func (o *CSVOutput) SupportedItemTypes() []string {
	return []string{"bookmark", "browser_history"}
}

func (o *CSVOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for CSV files",
			Type:        "string",
			Required:    true,
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

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *CSVOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// CSV output is batch-only
	return nil
}

func (o *CSVOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	// Separate bookmarks and history
	bookmarks := []map[string]any{}
	history := []map[string]any{}

	for _, item := range items {
		switch item.Type {
		case "bookmark":
			bookmarks = append(bookmarks, item.Metadata)
		case "browser_history":
			history = append(history, item.Metadata)
		}
	}

	// Write bookmarks CSV
	if len(bookmarks) > 0 {
		bookmarksPath := filepath.Join(o.destDir, "bookmarks.csv")
		if err := o.writeBookmarksCSV(bookmarksPath, bookmarks); err != nil {
			return fmt.Errorf("write bookmarks CSV: %w", err)
		}
	}

	// Write history CSV
	if len(history) > 0 {
		historyPath := filepath.Join(o.destDir, "history.csv")
		if err := o.writeHistoryCSV(historyPath, history); err != nil {
			return fmt.Errorf("write history CSV: %w", err)
		}
	}

	if progress != nil {
		progress(len(items), len(items))
	}

	return nil
}

func (o *CSVOutput) writeBookmarksCSV(path string, bookmarks []map[string]any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	writer.Write([]string{"Name", "URL", "Folder", "Date Added"})

	// Write rows
	for _, bm := range bookmarks {
		name := getString(bm, "name")
		url := getString(bm, "url")
		folder := getString(bm, "folder")
		dateAdded := fmt.Sprintf("%v", bm["date_added"])

		writer.Write([]string{name, url, folder, dateAdded})
	}

	return nil
}

func (o *CSVOutput) writeHistoryCSV(path string, history []map[string]any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	writer.Write([]string{"Title", "URL", "Last Visited", "Visit Count", "Page Transition"})

	// Write rows
	for _, entry := range history {
		title := getString(entry, "title")
		url := getString(entry, "url")
		lastVisited := fmt.Sprintf("%v", entry["last_visited"])
		visitCount := fmt.Sprintf("%v", entry["visit_count"])
		pageTransition := getString(entry, "page_transition")

		writer.Write([]string{title, url, lastVisited, visitCount, pageTransition})
	}

	return nil
}

func (o *CSVOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── Helper functions ──────────────────────────────────────────────────────────

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getStringSlice(m map[string]any, key string) []string {
	if v, ok := m[key].([]string); ok {
		return v
	}
	// Try interface slice
	if v, ok := m[key].([]any); ok {
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
