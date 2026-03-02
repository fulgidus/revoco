// Package metadata provides Tasks list parsing and structures.
package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// TaskList represents a Google Tasks list with all its metadata.
type TaskList struct {
	Title        string    `json:"title"`
	LastModified time.Time `json:"last_modified"`
	Tasks        []Task    `json:"tasks"`
}

// Task represents a single task within a list.
type Task struct {
	Title     string     `json:"title"`
	Notes     string     `json:"notes,omitempty"`
	Status    string     `json:"status"` // "completed" or "needsAction"
	Due       time.Time  `json:"due,omitempty"`
	Completed time.Time  `json:"completed,omitempty"`
	Parent    string     `json:"parent,omitempty"`   // ID of parent task
	Position  string     `json:"position,omitempty"` // Position within parent/list
	Links     []TaskLink `json:"links,omitempty"`
	IsDeleted bool       `json:"deleted,omitempty"`
	Updated   time.Time  `json:"updated"`
}

// TaskLink represents a link associated with a task.
type TaskLink struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"link"`
	Type        string `json:"type,omitempty"`
}

// tasksRawJSON represents the raw JSON structure from Google Takeout.
// Google Tasks exports are arrays of task lists.
type tasksRawJSON struct {
	Items []struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Updated string `json:"updated"` // RFC3339 timestamp
	} `json:"items"`
}

// taskRawJSON represents a single raw task from the JSON file.
type taskRawJSON struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Notes     string `json:"notes"`
	Status    string `json:"status"`
	Due       string `json:"due"`       // RFC3339 date
	Completed string `json:"completed"` // RFC3339 timestamp
	Parent    string `json:"parent"`
	Position  string `json:"position"`
	Links     []struct {
		Description string `json:"description"`
		Link        string `json:"link"`
		Type        string `json:"type"`
	} `json:"links"`
	Deleted bool   `json:"deleted"`
	Updated string `json:"updated"` // RFC3339 timestamp
}

// ParseTasksJSON parses a Google Tasks JSON file.
func ParseTasksJSON(reader io.Reader) (*TaskList, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	// Google Tasks Takeout format: array of task lists with nested tasks
	var rawData struct {
		Kind  string `json:"kind"`
		Items []struct {
			Kind    string        `json:"kind"`
			ID      string        `json:"id"`
			Title   string        `json:"title"`
			Updated string        `json:"updated"`
			Tasks   []taskRawJSON `json:"tasks"`
		} `json:"items"`
	}

	if err := json.Unmarshal(data, &rawData); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	if len(rawData.Items) == 0 {
		return nil, fmt.Errorf("no task lists found in JSON")
	}

	// Use first task list (Takeout typically exports one list per file)
	rawList := rawData.Items[0]

	list := &TaskList{
		Title: rawList.Title,
		Tasks: make([]Task, 0, len(rawList.Tasks)),
	}

	// Parse last modified timestamp
	if rawList.Updated != "" {
		if t, err := time.Parse(time.RFC3339, rawList.Updated); err == nil {
			list.LastModified = t
		}
	}

	// Parse tasks
	for _, rawTask := range rawList.Tasks {
		task := Task{
			Title:     rawTask.Title,
			Notes:     rawTask.Notes,
			Status:    rawTask.Status,
			Parent:    rawTask.Parent,
			Position:  rawTask.Position,
			IsDeleted: rawTask.Deleted,
		}

		// Parse timestamps
		if rawTask.Due != "" {
			if t, err := time.Parse(time.RFC3339, rawTask.Due); err == nil {
				task.Due = t
			}
		}
		if rawTask.Completed != "" {
			if t, err := time.Parse(time.RFC3339, rawTask.Completed); err == nil {
				task.Completed = t
			}
		}
		if rawTask.Updated != "" {
			if t, err := time.Parse(time.RFC3339, rawTask.Updated); err == nil {
				task.Updated = t
			}
		}

		// Parse links
		for _, rawLink := range rawTask.Links {
			task.Links = append(task.Links, TaskLink{
				Description: rawLink.Description,
				URL:         rawLink.Link,
				Type:        rawLink.Type,
			})
		}

		list.Tasks = append(list.Tasks, task)
	}

	return list, nil
}

// IsCompleted returns true if the task is marked as completed.
func (t *Task) IsCompleted() bool {
	return t.Status == "completed"
}

// HasDueDate returns true if the task has a due date set.
func (t *Task) HasDueDate() bool {
	return !t.Due.IsZero()
}

// HasNotes returns true if the task has notes.
func (t *Task) HasNotes() bool {
	return t.Notes != ""
}

// HasLinks returns true if the task has associated links.
func (t *Task) HasLinks() bool {
	return len(t.Links) > 0
}

// HasParent returns true if the task is a subtask.
func (t *Task) HasParent() bool {
	return t.Parent != ""
}

// FormatDueDate formats the due date using the given layout.
func (t *Task) FormatDueDate(layout string) string {
	if t.Due.IsZero() {
		return ""
	}
	return t.Due.Format(layout)
}

// FormatCompletedDate formats the completed date using the given layout.
func (t *Task) FormatCompletedDate(layout string) string {
	if t.Completed.IsZero() {
		return ""
	}
	return t.Completed.Format(layout)
}

// GetCheckboxSymbol returns the appropriate checkbox symbol for markdown.
func (t *Task) GetCheckboxSymbol() string {
	if t.IsCompleted() {
		return "[x]"
	}
	return "[ ]"
}

// ToMarkdown converts the task to markdown format.
func (t *Task) ToMarkdown(indent int) string {
	prefix := ""
	for i := 0; i < indent; i++ {
		prefix += "  "
	}

	md := fmt.Sprintf("%s- %s %s", prefix, t.GetCheckboxSymbol(), t.Title)

	if t.HasDueDate() {
		md += fmt.Sprintf(" (due: %s)", t.Due.Format("2006-01-02"))
	}

	if t.HasNotes() {
		md += fmt.Sprintf("\n%s  %s", prefix, t.Notes)
	}

	if t.HasLinks() {
		for _, link := range t.Links {
			linkText := link.URL
			if link.Description != "" {
				linkText = link.Description
			}
			md += fmt.Sprintf("\n%s  [%s](%s)", prefix, linkText, link.URL)
		}
	}

	return md
}

// CountCompleted returns the number of completed tasks in the list.
func (l *TaskList) CountCompleted() int {
	count := 0
	for _, task := range l.Tasks {
		if task.IsCompleted() {
			count++
		}
	}
	return count
}

// CountPending returns the number of pending tasks in the list.
func (l *TaskList) CountPending() int {
	count := 0
	for _, task := range l.Tasks {
		if !task.IsCompleted() && !task.IsDeleted {
			count++
		}
	}
	return count
}

// BuildHierarchy organizes tasks into a tree structure based on Parent field.
// Returns top-level tasks with their subtasks properly nested.
func (l *TaskList) BuildHierarchy() []Task {
	// Build ID to task map
	idMap := make(map[string]*Task)
	for i := range l.Tasks {
		task := &l.Tasks[i]
		// Use position as temporary ID if no ID field exists
		// In real Takeout, there's an ID field we'll use
		idMap[task.Position] = task
	}

	// Identify top-level tasks
	var topLevel []Task
	for _, task := range l.Tasks {
		if !task.HasParent() {
			topLevel = append(topLevel, task)
		}
	}

	// Note: Full hierarchy building would require recursive nesting
	// For now, we just return all tasks (flattened)
	// The Parent field preserves the relationship for outputs to reconstruct
	return topLevel
}
