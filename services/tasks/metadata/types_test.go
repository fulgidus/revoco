package metadata

import (
	"strings"
	"testing"
	"time"
)

func TestParseTasksJSON(t *testing.T) {
	jsonData := `{
		"kind": "tasks#taskLists",
		"items": [{
			"kind": "tasks#taskList",
			"id": "MTIzNDU2Nzg5MDEyMzQ1Njc4OTA6MDow",
			"title": "My Tasks",
			"updated": "2024-01-15T10:30:00.000Z",
			"tasks": [
				{
					"id": "task1",
					"title": "Buy groceries",
					"notes": "Milk, eggs, bread",
					"status": "needsAction",
					"due": "2024-01-20T00:00:00.000Z",
					"position": "00000000000000000001",
					"updated": "2024-01-15T10:30:00.000Z",
					"links": [
						{
							"description": "Shopping list",
							"link": "https://example.com/list",
							"type": "email"
						}
					]
				},
				{
					"id": "task2",
					"title": "Finish report",
					"status": "completed",
					"completed": "2024-01-14T15:45:00.000Z",
					"position": "00000000000000000002",
					"updated": "2024-01-14T15:45:00.000Z"
				},
				{
					"id": "task3",
					"title": "Buy milk",
					"status": "needsAction",
					"parent": "task1",
					"position": "00000000000000000003",
					"updated": "2024-01-15T10:32:00.000Z"
				}
			]
		}]
	}`

	reader := strings.NewReader(jsonData)
	list, err := ParseTasksJSON(reader)
	if err != nil {
		t.Fatalf("ParseTasksJSON failed: %v", err)
	}

	if list.Title != "My Tasks" {
		t.Errorf("Expected title 'My Tasks', got '%s'", list.Title)
	}

	if len(list.Tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(list.Tasks))
	}

	// Check first task
	task1 := list.Tasks[0]
	if task1.Title != "Buy groceries" {
		t.Errorf("Expected task title 'Buy groceries', got '%s'", task1.Title)
	}
	if task1.Notes != "Milk, eggs, bread" {
		t.Errorf("Expected notes, got '%s'", task1.Notes)
	}
	if task1.Status != "needsAction" {
		t.Errorf("Expected status 'needsAction', got '%s'", task1.Status)
	}
	if task1.Due.IsZero() {
		t.Error("Expected due date to be set")
	}
	if len(task1.Links) != 1 {
		t.Errorf("Expected 1 link, got %d", len(task1.Links))
	}

	// Check completed task
	task2 := list.Tasks[1]
	if task2.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", task2.Status)
	}
	if task2.Completed.IsZero() {
		t.Error("Expected completed date to be set")
	}

	// Check subtask
	task3 := list.Tasks[2]
	if task3.Parent != "task1" {
		t.Errorf("Expected parent 'task1', got '%s'", task3.Parent)
	}
}

func TestParseTasksJSON_EmptyList(t *testing.T) {
	jsonData := `{
		"kind": "tasks#taskLists",
		"items": [{
			"kind": "tasks#taskList",
			"id": "list1",
			"title": "Empty List",
			"updated": "2024-01-15T10:30:00.000Z",
			"tasks": []
		}]
	}`

	reader := strings.NewReader(jsonData)
	list, err := ParseTasksJSON(reader)
	if err != nil {
		t.Fatalf("ParseTasksJSON failed: %v", err)
	}

	if len(list.Tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(list.Tasks))
	}
}

func TestParseTasksJSON_NoLists(t *testing.T) {
	jsonData := `{
		"kind": "tasks#taskLists",
		"items": []
	}`

	reader := strings.NewReader(jsonData)
	_, err := ParseTasksJSON(reader)
	if err == nil {
		t.Error("Expected error for empty items array")
	}
}

func TestParseTasksJSON_MalformedJSON(t *testing.T) {
	jsonData := `{invalid json}`

	reader := strings.NewReader(jsonData)
	_, err := ParseTasksJSON(reader)
	if err == nil {
		t.Error("Expected error for malformed JSON")
	}
}

func TestTask_IsCompleted(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"completed task", "completed", true},
		{"needs action", "needsAction", false},
		{"empty status", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Status: tt.status}
			if got := task.IsCompleted(); got != tt.expected {
				t.Errorf("IsCompleted() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTask_HasDueDate(t *testing.T) {
	tests := []struct {
		name     string
		due      time.Time
		expected bool
	}{
		{"with due date", time.Now(), true},
		{"no due date", time.Time{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Due: tt.due}
			if got := task.HasDueDate(); got != tt.expected {
				t.Errorf("HasDueDate() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTask_HasNotes(t *testing.T) {
	tests := []struct {
		name     string
		notes    string
		expected bool
	}{
		{"with notes", "Some notes", true},
		{"no notes", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Notes: tt.notes}
			if got := task.HasNotes(); got != tt.expected {
				t.Errorf("HasNotes() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTask_HasLinks(t *testing.T) {
	tests := []struct {
		name     string
		links    []TaskLink
		expected bool
	}{
		{"with links", []TaskLink{{URL: "http://example.com"}}, true},
		{"no links", []TaskLink{}, false},
		{"nil links", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Links: tt.links}
			if got := task.HasLinks(); got != tt.expected {
				t.Errorf("HasLinks() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTask_HasParent(t *testing.T) {
	tests := []struct {
		name     string
		parent   string
		expected bool
	}{
		{"with parent", "parent1", true},
		{"no parent", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Parent: tt.parent}
			if got := task.HasParent(); got != tt.expected {
				t.Errorf("HasParent() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTask_FormatDueDate(t *testing.T) {
	dueDate := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	task := &Task{Due: dueDate}

	formatted := task.FormatDueDate("2006-01-02")
	if formatted != "2024-01-20" {
		t.Errorf("FormatDueDate() = %s, want 2024-01-20", formatted)
	}

	// Test zero date
	emptyTask := &Task{}
	if got := emptyTask.FormatDueDate("2006-01-02"); got != "" {
		t.Errorf("FormatDueDate() on zero date = %s, want empty string", got)
	}
}

func TestTask_FormatCompletedDate(t *testing.T) {
	completedDate := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	task := &Task{Completed: completedDate}

	formatted := task.FormatCompletedDate("2006-01-02 15:04:05")
	expected := "2024-01-15 14:30:00"
	if formatted != expected {
		t.Errorf("FormatCompletedDate() = %s, want %s", formatted, expected)
	}

	// Test zero date
	emptyTask := &Task{}
	if got := emptyTask.FormatCompletedDate("2006-01-02"); got != "" {
		t.Errorf("FormatCompletedDate() on zero date = %s, want empty string", got)
	}
}

func TestTask_GetCheckboxSymbol(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{"completed", "completed", "[x]"},
		{"needs action", "needsAction", "[ ]"},
		{"empty status", "", "[ ]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Status: tt.status}
			if got := task.GetCheckboxSymbol(); got != tt.expected {
				t.Errorf("GetCheckboxSymbol() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestTask_ToMarkdown(t *testing.T) {
	dueDate := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	task := &Task{
		Title:  "Buy groceries",
		Status: "needsAction",
		Due:    dueDate,
		Notes:  "Milk and eggs",
		Links: []TaskLink{
			{Description: "Shopping list", URL: "http://example.com"},
		},
	}

	md := task.ToMarkdown(0)
	if !strings.Contains(md, "- [ ] Buy groceries") {
		t.Error("Markdown should contain checkbox and title")
	}
	if !strings.Contains(md, "(due: 2024-01-20)") {
		t.Error("Markdown should contain due date")
	}
	if !strings.Contains(md, "Milk and eggs") {
		t.Error("Markdown should contain notes")
	}
	if !strings.Contains(md, "[Shopping list](http://example.com)") {
		t.Error("Markdown should contain link")
	}

	// Test indented subtask
	mdIndented := task.ToMarkdown(1)
	if !strings.HasPrefix(mdIndented, "  - [ ]") {
		t.Error("Indented markdown should have 2-space prefix")
	}
}

func TestTaskList_CountCompleted(t *testing.T) {
	list := &TaskList{
		Tasks: []Task{
			{Status: "completed"},
			{Status: "needsAction"},
			{Status: "completed"},
			{Status: "needsAction"},
		},
	}

	if got := list.CountCompleted(); got != 2 {
		t.Errorf("CountCompleted() = %d, want 2", got)
	}
}

func TestTaskList_CountPending(t *testing.T) {
	list := &TaskList{
		Tasks: []Task{
			{Status: "completed"},
			{Status: "needsAction", IsDeleted: false},
			{Status: "needsAction", IsDeleted: false},
			{Status: "needsAction", IsDeleted: true},
		},
	}

	if got := list.CountPending(); got != 2 {
		t.Errorf("CountPending() = %d, want 2", got)
	}
}

func TestTaskList_BuildHierarchy(t *testing.T) {
	list := &TaskList{
		Tasks: []Task{
			{Title: "Task 1", Position: "pos1"},
			{Title: "Task 2", Position: "pos2", Parent: "pos1"},
			{Title: "Task 3", Position: "pos3"},
		},
	}

	topLevel := list.BuildHierarchy()
	if len(topLevel) != 2 {
		t.Errorf("BuildHierarchy() returned %d top-level tasks, want 2", len(topLevel))
	}

	// Check that subtask is not in top-level
	for _, task := range topLevel {
		if task.Title == "Task 2" {
			t.Error("Subtask should not be in top-level tasks")
		}
	}
}
