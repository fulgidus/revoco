// Package outputs provides output modules for Tasks data.
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
	"github.com/fulgidus/revoco/services/tasks/metadata"
)

// ── JSON Output ──────────────────────────────────────────────────────────────

// JSONOutput exports task lists to hierarchical JSON files.
type JSONOutput struct {
	destDir string
	pretty  bool
}

// NewJSON creates a new JSON output.
func NewJSON() *JSONOutput {
	return &JSONOutput{pretty: true}
}

func (o *JSONOutput) ID() string          { return "tasks-json" }
func (o *JSONOutput) Name() string        { return "Tasks JSON Export" }
func (o *JSONOutput) Description() string { return "Export task lists to hierarchical JSON" }

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"task_list"}
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
	if item.Type != "task_list" {
		return nil
	}

	taskListData, ok := item.Metadata["task_list"]
	if !ok {
		return fmt.Errorf("missing task_list in metadata")
	}

	taskList, ok := taskListData.(metadata.TaskList)
	if !ok {
		return fmt.Errorf("invalid task_list type in metadata")
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)
	os.MkdirAll(filepath.Dir(destPath), 0o755)

	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(taskList, "", "  ")
	} else {
		data, err = json.Marshal(taskList)
	}
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
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

// ── Markdown Output ──────────────────────────────────────────────────────────

// MarkdownOutput exports task lists to Markdown files with checkbox syntax.
type MarkdownOutput struct {
	destDir          string
	includeCompleted bool
	includeDeleted   bool
}

// NewMarkdown creates a new Markdown output.
func NewMarkdown() *MarkdownOutput {
	return &MarkdownOutput{
		includeCompleted: true,
		includeDeleted:   false,
	}
}

func (o *MarkdownOutput) ID() string          { return "tasks-markdown" }
func (o *MarkdownOutput) Name() string        { return "Tasks Markdown Export" }
func (o *MarkdownOutput) Description() string { return "Export task lists to Markdown with checkboxes" }

func (o *MarkdownOutput) SupportedItemTypes() []string {
	return []string{"task_list"}
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
			ID:          "include_completed",
			Name:        "Include Completed",
			Description: "Include completed tasks in output",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "include_deleted",
			Name:        "Include Deleted",
			Description: "Include deleted tasks in output",
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

	if v, ok := cfg.Settings["include_completed"].(bool); ok {
		o.includeCompleted = v
	}
	if v, ok := cfg.Settings["include_deleted"].(bool); ok {
		o.includeDeleted = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *MarkdownOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "task_list" {
		return nil
	}

	taskListData, ok := item.Metadata["task_list"]
	if !ok {
		return fmt.Errorf("missing task_list in metadata")
	}

	taskList, ok := taskListData.(metadata.TaskList)
	if !ok {
		return fmt.Errorf("invalid task_list type in metadata")
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)
	// Change extension to .md
	destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".md"

	os.MkdirAll(filepath.Dir(destPath), 0o755)

	// Build markdown content
	var md strings.Builder
	md.WriteString("# ")
	md.WriteString(taskList.Title)
	md.WriteString("\n\n")

	if !taskList.LastModified.IsZero() {
		md.WriteString("*Last modified: ")
		md.WriteString(taskList.LastModified.Format("2006-01-02 15:04:05"))
		md.WriteString("*\n\n")
	}

	// Build parent-child map for hierarchy
	childMap := make(map[string][]metadata.Task)
	var topLevel []metadata.Task

	for _, task := range taskList.Tasks {
		// Skip based on filters
		if !o.includeCompleted && task.IsCompleted() {
			continue
		}
		if !o.includeDeleted && task.IsDeleted {
			continue
		}

		if task.HasParent() {
			childMap[task.Parent] = append(childMap[task.Parent], task)
		} else {
			topLevel = append(topLevel, task)
		}
	}

	// Write tasks recursively
	for _, task := range topLevel {
		o.writeTaskMarkdown(&md, task, childMap, 0)
	}

	return os.WriteFile(destPath, []byte(md.String()), 0o644)
}

func (o *MarkdownOutput) writeTaskMarkdown(md *strings.Builder, task metadata.Task, childMap map[string][]metadata.Task, indent int) {
	md.WriteString(task.ToMarkdown(indent))
	md.WriteString("\n")

	// Write subtasks
	// Note: We use Position as ID since raw JSON parsing captures it
	if children, ok := childMap[task.Position]; ok {
		for _, child := range children {
			o.writeTaskMarkdown(md, child, childMap, indent+1)
		}
	}
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

// ── CSV Output ───────────────────────────────────────────────────────────────

// CSVOutput exports tasks to a flat CSV file.
type CSVOutput struct {
	destPath string
}

// NewCSV creates a new CSV output.
func NewCSV() *CSVOutput {
	return &CSVOutput{}
}

func (o *CSVOutput) ID() string          { return "tasks-csv" }
func (o *CSVOutput) Name() string        { return "Tasks CSV Export" }
func (o *CSVOutput) Description() string { return "Export tasks to flat CSV format" }

func (o *CSVOutput) SupportedItemTypes() []string {
	return []string{"task_list"}
}

func (o *CSVOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_path",
			Name:        "Destination File",
			Description: "Output CSV file path (e.g., tasks.csv)",
			Type:        "string",
			Required:    true,
		},
	}
}

func (o *CSVOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destPath = cfg.DestDir
	if o.destPath == "" {
		if d, ok := cfg.Settings["dest_path"].(string); ok {
			o.destPath = d
		}
	}
	if o.destPath == "" {
		o.destPath = filepath.Join(cfg.DestDir, "tasks.csv")
	}

	return os.MkdirAll(filepath.Dir(o.destPath), 0o755)
}

func (o *CSVOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// CSV export is batch-only
	return nil
}

func (o *CSVOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	file, err := os.Create(o.destPath)
	if err != nil {
		return fmt.Errorf("create CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"list_name",
		"task_title",
		"status",
		"due_date",
		"completed_date",
		"notes",
		"parent_task",
		"has_links",
		"is_deleted",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	// Write tasks
	totalWritten := 0
	for _, item := range items {
		if item.Type != "task_list" {
			continue
		}

		taskListData, ok := item.Metadata["task_list"]
		if !ok {
			continue
		}

		taskList, ok := taskListData.(metadata.TaskList)
		if !ok {
			continue
		}

		for _, task := range taskList.Tasks {
			row := []string{
				taskList.Title,
				task.Title,
				task.Status,
				task.FormatDueDate("2006-01-02"),
				task.FormatCompletedDate("2006-01-02 15:04:05"),
				task.Notes,
				task.Parent,
				fmt.Sprintf("%t", task.HasLinks()),
				fmt.Sprintf("%t", task.IsDeleted),
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
			totalWritten++
		}
	}

	if progress != nil {
		progress(totalWritten, totalWritten)
	}

	return nil
}

func (o *CSVOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── Registration ─────────────────────────────────────────────────────────────

func init() {
	// Register outputs on import
	core.RegisterOutput(NewJSON())
	core.RegisterOutput(NewMarkdown())
	core.RegisterOutput(NewCSV())
}
