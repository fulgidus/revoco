// Package processors provides data processing for Google Tasks Takeout.
package processors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/tasks/metadata"
)

// Processor handles the Tasks processing pipeline.
type Processor struct{}

// NewTasksProcessor creates a new Tasks processor.
func NewTasksProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) ID() string   { return "tasks-processor" }
func (p *Processor) Name() string { return "Tasks Processor" }
func (p *Processor) Description() string {
	return "Process Google Tasks Takeout JSON - task lists to JSON/Markdown/CSV"
}

// ConfigSchema returns the configuration options for this processor.
func (p *Processor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{}
}

// Process runs the Tasks processing pipeline.
// Phases: 1) Scan JSON files, 2) Parse task lists, 3) Build hierarchy, 4) Generate summary
func (p *Processor) Process(ctx context.Context, cfg core.ProcessorConfig, events chan<- core.ProgressEvent) (*core.ProcessResult, error) {
	defer close(events)

	emit := func(phase int, label string, done, total int, msg string) {
		select {
		case events <- core.ProgressEvent{
			Phase:   phase,
			Label:   label,
			Done:    done,
			Total:   total,
			Message: msg,
		}:
		case <-ctx.Done():
		}
	}

	// Setup logging
	logDir := cfg.SessionDir
	if logDir == "" {
		logDir = cfg.WorkDir
	}
	os.MkdirAll(logDir, 0o755)

	logPath := filepath.Join(logDir, "process.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)
	logger.Printf("=== Tasks processing started (source=%s) ===", cfg.SourceDir)

	// Find Tasks directory
	tasksPath, err := detectTasksDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(tasksPath)))
	logger.Printf("[Setup] Tasks directory: %s", tasksPath)

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	// ── Phase 1: Scan for JSON files ────────────────────────────────────────
	emit(1, "Scanning task files", 0, 0, "")
	jsonFiles, err := p.scanJSONFiles(ctx, tasksPath, logger)
	if err != nil {
		logger.Printf("[Phase 1] Error scanning JSON files: %v", err)
		return nil, err
	}
	result.Stats["json_files"] = len(jsonFiles)
	emit(1, "Scan complete", len(jsonFiles), len(jsonFiles),
		fmt.Sprintf("%d JSON files found", len(jsonFiles)))
	logger.Printf("[Phase 1] json_files=%d", len(jsonFiles))

	// ── Phase 2: Parse task lists ───────────────────────────────────────────
	emit(2, "Parsing task lists", 0, len(jsonFiles), "")
	taskLists, parseErrors := p.parseTaskLists(ctx, jsonFiles, emit, logger)
	result.Stats["lists_parsed"] = len(taskLists)
	result.Stats["parse_errors"] = parseErrors
	emit(2, "Parse complete", len(taskLists), len(jsonFiles),
		fmt.Sprintf("%d lists parsed (%d errors)", len(taskLists), parseErrors))
	logger.Printf("[Phase 2] lists_parsed=%d, errors=%d", len(taskLists), parseErrors)

	// ── Phase 3: Build task hierarchies ─────────────────────────────────────
	emit(3, "Building task hierarchies", 0, len(taskLists), "")
	totalTasks := 0
	completedTasks := 0
	pendingTasks := 0
	deletedTasks := 0
	tasksWithDueDates := 0
	tasksWithNotes := 0
	tasksWithLinks := 0
	subtasks := 0

	for i, list := range taskLists {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		totalTasks += len(list.Tasks)
		completedTasks += list.CountCompleted()
		pendingTasks += list.CountPending()

		for _, task := range list.Tasks {
			if task.IsDeleted {
				deletedTasks++
			}
			if task.HasDueDate() {
				tasksWithDueDates++
			}
			if task.HasNotes() {
				tasksWithNotes++
			}
			if task.HasLinks() {
				tasksWithLinks++
			}
			if task.HasParent() {
				subtasks++
			}
		}

		if i%5 == 0 {
			emit(3, "Building hierarchies", i, len(taskLists), "")
		}
	}

	result.Stats["total_tasks"] = totalTasks
	result.Stats["completed_tasks"] = completedTasks
	result.Stats["pending_tasks"] = pendingTasks
	result.Stats["deleted_tasks"] = deletedTasks
	result.Stats["tasks_with_due_dates"] = tasksWithDueDates
	result.Stats["tasks_with_notes"] = tasksWithNotes
	result.Stats["tasks_with_links"] = tasksWithLinks
	result.Stats["subtasks"] = subtasks

	emit(3, "Hierarchy complete", len(taskLists), len(taskLists),
		fmt.Sprintf("%d tasks across %d lists", totalTasks, len(taskLists)))
	logger.Printf("[Phase 3] total_tasks=%d, completed=%d, pending=%d, subtasks=%d",
		totalTasks, completedTasks, pendingTasks, subtasks)

	// ── Phase 4: Write summary ──────────────────────────────────────────────
	emit(4, "Writing metadata", 0, 1, "")

	// Store task lists in metadata for outputs
	result.Metadata["task_lists"] = taskLists

	// Write summary as JSON
	summaryPath := filepath.Join(cfg.WorkDir, "tasks_summary.json")
	summaryData := map[string]any{
		"stats":      result.Stats,
		"task_lists": taskLists,
	}
	summaryJSON, _ := json.MarshalIndent(summaryData, "", "  ")
	os.WriteFile(summaryPath, summaryJSON, 0o644)

	result.Metadata["summary_path"] = summaryPath

	emit(4, "Output complete", 1, 1, fmt.Sprintf("Wrote %s", filepath.Base(summaryPath)))
	logger.Printf("[Phase 4] Wrote tasks_summary.json")
	logger.Printf("=== Tasks processing complete ===")

	// Build ProcessedItems
	result.Items = p.buildProcessedItems(taskLists, cfg.WorkDir)

	return result, nil
}

// scanJSONFiles finds all .json files in the Tasks directory.
func (p *Processor) scanJSONFiles(ctx context.Context, tasksPath string, logger *log.Logger) ([]string, error) {
	var jsonFiles []string

	err := filepath.WalkDir(tasksPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable paths
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return nil
		}

		// Tasks are .json files
		if strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			jsonFiles = append(jsonFiles, path)
		}

		return nil
	})

	return jsonFiles, err
}

// parseTaskLists parses all JSON files into TaskList structs.
func (p *Processor) parseTaskLists(ctx context.Context, jsonFiles []string, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.TaskList, int) {
	var lists []metadata.TaskList
	parseErrors := 0

	for i, jsonPath := range jsonFiles {
		select {
		case <-ctx.Done():
			return lists, parseErrors
		default:
		}

		file, err := os.Open(jsonPath)
		if err != nil {
			logger.Printf("Error opening %s: %v", jsonPath, err)
			parseErrors++
			continue
		}

		list, err := metadata.ParseTasksJSON(file)
		file.Close()

		if err != nil {
			logger.Printf("Error parsing %s: %v", jsonPath, err)
			parseErrors++
			continue
		}

		lists = append(lists, *list)

		if i%5 == 0 {
			emit(2, "Parsing task lists", i, len(jsonFiles), "")
		}
	}

	return lists, parseErrors
}

// buildProcessedItems creates ProcessedItem entries for outputs.
func (p *Processor) buildProcessedItems(taskLists []metadata.TaskList, workDir string) []core.ProcessedItem {
	var items []core.ProcessedItem

	for listIdx, list := range taskLists {
		// Generate a safe filename from title
		filename := sanitizeFilename(list.Title)
		if filename == "" {
			filename = fmt.Sprintf("tasklist_%d", listIdx)
		}

		relPath := fmt.Sprintf("task_lists/%s.json", filename)

		items = append(items, core.ProcessedItem{
			SourcePath:    "",
			ProcessedPath: "",
			DestRelPath:   relPath,
			Type:          "task_list",
			Metadata: map[string]any{
				"title":         list.Title,
				"last_modified": list.LastModified,
				"total_tasks":   len(list.Tasks),
				"completed":     list.CountCompleted(),
				"pending":       list.CountPending(),
				"task_list":     list,
			},
		})
	}

	return items
}

// ── Helper functions ─────────────────────────────────────────────────────────

var tasksDirVariants = []string{
	"Tasks",
	"Attività", // Italian
}

func detectTasksDir(sourceDir string) (string, error) {
	baseName := filepath.Base(sourceDir)
	for _, variant := range tasksDirVariants {
		if strings.EqualFold(baseName, variant) {
			return sourceDir, nil
		}
	}

	var found string
	filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(sourceDir, path)
		depth := len(strings.Split(rel, string(os.PathSeparator)))
		if depth > 3 {
			return filepath.SkipDir
		}
		for _, variant := range tasksDirVariants {
			if strings.EqualFold(d.Name(), variant) {
				found = path
				return filepath.SkipAll
			}
		}
		return nil
	})

	if found != "" {
		return found, nil
	}

	return sourceDir, nil
}

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
	sanitized := replacer.Replace(name)
	// Limit length
	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}
	return sanitized
}
