package metadata_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fulgidus/revoco/services/keep/metadata"
)

func TestParseKeepNote_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	noteJSON := `{
		"title": "Test Note",
		"textContent": "This is a test note with some content.",
		"color": "BLUE",
		"isPinned": true,
		"isArchived": false,
		"isTrashed": false,
		"createdTimestampUsec": "1640000000000000",
		"userEditedTimestampUsec": "1640001000000000"
	}`

	notePath := filepath.Join(tmpDir, "test_note.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	note, err := metadata.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed: %v", err)
	}

	if note.Title != "Test Note" {
		t.Errorf("Expected title 'Test Note', got %q", note.Title)
	}

	if note.TextContent != "This is a test note with some content." {
		t.Errorf("Unexpected text content: %q", note.TextContent)
	}

	if note.Color != "BLUE" {
		t.Errorf("Expected color 'BLUE', got %q", note.Color)
	}

	if !note.IsPinned {
		t.Error("Expected note to be pinned")
	}

	if note.IsArchived {
		t.Error("Expected note to not be archived")
	}

	if note.Created.IsZero() {
		t.Error("Expected non-zero created timestamp")
	}

	if note.Modified.IsZero() {
		t.Error("Expected non-zero modified timestamp")
	}
}

func TestParseKeepNote_WithCheckboxes(t *testing.T) {
	tmpDir := t.TempDir()

	noteJSON := `{
		"title": "Shopping List",
		"listContent": [
			{"text": "Milk", "isChecked": false},
			{"text": "Eggs", "isChecked": true},
			{"text": "Bread", "isChecked": false}
		],
		"color": "DEFAULT",
		"isPinned": false,
		"isArchived": false,
		"isTrashed": false,
		"createdTimestampUsec": "1640000000000000",
		"userEditedTimestampUsec": "1640000000000000"
	}`

	notePath := filepath.Join(tmpDir, "checklist.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	note, err := metadata.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed: %v", err)
	}

	if note.Title != "Shopping List" {
		t.Errorf("Expected title 'Shopping List', got %q", note.Title)
	}

	if len(note.Checkboxes) != 3 {
		t.Fatalf("Expected 3 checkboxes, got %d", len(note.Checkboxes))
	}

	if note.Checkboxes[0].Text != "Milk" || note.Checkboxes[0].Checked {
		t.Errorf("Checkbox 0: expected unchecked 'Milk', got %v", note.Checkboxes[0])
	}

	if note.Checkboxes[1].Text != "Eggs" || !note.Checkboxes[1].Checked {
		t.Errorf("Checkbox 1: expected checked 'Eggs', got %v", note.Checkboxes[1])
	}

	if !note.HasCheckboxes() {
		t.Error("HasCheckboxes() should return true")
	}
}

func TestParseKeepNote_WithLabels(t *testing.T) {
	tmpDir := t.TempDir()

	noteJSON := `{
		"title": "Project Ideas",
		"textContent": "Some brilliant ideas here.",
		"labels": [
			{"name": "Work"},
			{"name": "Important"},
			{"name": "Follow-up"}
		],
		"color": "GREEN",
		"isPinned": false,
		"isArchived": false,
		"isTrashed": false,
		"createdTimestampUsec": "1640000000000000",
		"userEditedTimestampUsec": "1640000000000000"
	}`

	notePath := filepath.Join(tmpDir, "labeled.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	note, err := metadata.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed: %v", err)
	}

	if len(note.Labels) != 3 {
		t.Fatalf("Expected 3 labels, got %d", len(note.Labels))
	}

	expectedLabels := []string{"Work", "Important", "Follow-up"}
	actualLabels := note.GetLabelNames()

	for i, expected := range expectedLabels {
		if actualLabels[i] != expected {
			t.Errorf("Label %d: expected %q, got %q", i, expected, actualLabels[i])
		}
	}
}

func TestParseKeepNote_WithAttachments(t *testing.T) {
	tmpDir := t.TempDir()

	noteJSON := `{
		"title": "Trip Photos",
		"textContent": "Vacation memories",
		"attachments": [
			{"filePath": "image1.jpg", "mimetype": "image/jpeg"},
			{"filePath": "image2.png", "mimetype": "image/png"}
		],
		"color": "DEFAULT",
		"isPinned": false,
		"isArchived": false,
		"isTrashed": false,
		"createdTimestampUsec": "1640000000000000",
		"userEditedTimestampUsec": "1640000000000000"
	}`

	notePath := filepath.Join(tmpDir, "with_attachments.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	note, err := metadata.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed: %v", err)
	}

	if len(note.Attachments) != 2 {
		t.Fatalf("Expected 2 attachments, got %d", len(note.Attachments))
	}

	if note.Attachments[0].FilePath != "image1.jpg" {
		t.Errorf("Attachment 0: expected 'image1.jpg', got %q", note.Attachments[0].FilePath)
	}

	if note.Attachments[0].MimeType != "image/jpeg" {
		t.Errorf("Attachment 0: expected 'image/jpeg', got %q", note.Attachments[0].MimeType)
	}

	if !note.HasAttachments() {
		t.Error("HasAttachments() should return true")
	}
}

func TestParseKeepNote_ArchivedAndTrashed(t *testing.T) {
	tmpDir := t.TempDir()

	noteJSON := `{
		"title": "Archived Note",
		"textContent": "This note is archived.",
		"color": "DEFAULT",
		"isPinned": false,
		"isArchived": true,
		"isTrashed": false,
		"createdTimestampUsec": "1640000000000000",
		"userEditedTimestampUsec": "1640000000000000"
	}`

	notePath := filepath.Join(tmpDir, "archived.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	note, err := metadata.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed: %v", err)
	}

	if !note.IsArchived {
		t.Error("Expected note to be archived")
	}

	if note.IsTrashed {
		t.Error("Expected note to not be trashed")
	}
}

func TestParseKeepNote_EmptyNote(t *testing.T) {
	tmpDir := t.TempDir()

	noteJSON := `{
		"title": "",
		"textContent": "",
		"color": "DEFAULT",
		"isPinned": false,
		"isArchived": false,
		"isTrashed": false,
		"createdTimestampUsec": "1640000000000000",
		"userEditedTimestampUsec": "1640000000000000"
	}`

	notePath := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	note, err := metadata.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed: %v", err)
	}

	if !note.IsEmpty() {
		t.Error("IsEmpty() should return true for empty note")
	}
}

func TestParseKeepNote_WithAnnotations(t *testing.T) {
	tmpDir := t.TempDir()

	noteJSON := `{
		"title": "Research Links",
		"textContent": "Useful resources",
		"annotations": [
			{
				"description": "Official docs",
				"source": "WEBLINK",
				"title": "Go Documentation",
				"url": "https://go.dev/doc/"
			}
		],
		"color": "DEFAULT",
		"isPinned": false,
		"isArchived": false,
		"isTrashed": false,
		"createdTimestampUsec": "1640000000000000",
		"userEditedTimestampUsec": "1640000000000000"
	}`

	notePath := filepath.Join(tmpDir, "with_links.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	note, err := metadata.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed: %v", err)
	}

	if len(note.Annotations) != 1 {
		t.Fatalf("Expected 1 annotation, got %d", len(note.Annotations))
	}

	ann := note.Annotations[0]
	if ann.URL != "https://go.dev/doc/" {
		t.Errorf("Expected URL 'https://go.dev/doc/', got %q", ann.URL)
	}

	if !note.HasAnnotations() {
		t.Error("HasAnnotations() should return true")
	}
}

func TestNote_ContentType(t *testing.T) {
	tests := []struct {
		name     string
		note     metadata.Note
		expected string
	}{
		{
			name: "checklist",
			note: metadata.Note{
				Checkboxes: []metadata.Checkbox{{Text: "Item", Checked: false}},
			},
			expected: "checklist",
		},
		{
			name: "note with attachments",
			note: metadata.Note{
				TextContent: "Text",
				Attachments: []metadata.Attachment{{FilePath: "file.jpg"}},
			},
			expected: "note_with_attachments",
		},
		{
			name: "text note",
			note: metadata.Note{
				TextContent: "Some text",
			},
			expected: "text_note",
		},
		{
			name:     "empty",
			note:     metadata.Note{},
			expected: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.note.ContentType(); got != tt.expected {
				t.Errorf("ContentType() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestNote_CheckboxesMarkdown(t *testing.T) {
	note := metadata.Note{
		Checkboxes: []metadata.Checkbox{
			{Text: "Task 1", Checked: false},
			{Text: "Task 2", Checked: true},
			{Text: "Task 3", Checked: false},
		},
	}

	md := note.CheckboxesMarkdown()
	expected := "- [ ] Task 1\n- [x] Task 2\n- [ ] Task 3\n"

	if md != expected {
		t.Errorf("CheckboxesMarkdown() = %q, expected %q", md, expected)
	}
}

func TestNote_LabelsMarkdown(t *testing.T) {
	note := metadata.Note{
		Labels: []metadata.Label{
			{Name: "Work"},
			{Name: "Important"},
		},
	}

	md := note.LabelsMarkdown()
	expected := "#Work #Important "

	if md != expected {
		t.Errorf("LabelsMarkdown() = %q, expected %q", md, expected)
	}
}

func TestNote_ToMarkdown(t *testing.T) {
	note := metadata.Note{
		Title:       "Test Note",
		TextContent: "Some content here",
		Labels:      []metadata.Label{{Name: "Test"}},
		Color:       "BLUE",
		Created:     time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Modified:    time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
		Checkboxes: []metadata.Checkbox{
			{Text: "Item 1", Checked: true},
		},
	}

	md := note.ToMarkdown(true)

	// Check for key components
	if !contains(md, "# Test Note") {
		t.Error("Markdown should contain title")
	}

	if !contains(md, "Some content here") {
		t.Error("Markdown should contain text content")
	}

	if !contains(md, "#Test") {
		t.Error("Markdown should contain labels")
	}

	if !contains(md, "- [x] Item 1") {
		t.Error("Markdown should contain checkboxes")
	}

	if !contains(md, "Created:") {
		t.Error("Markdown should contain created timestamp")
	}
}

func TestNote_ToMarkdown_WithoutMetadata(t *testing.T) {
	note := metadata.Note{
		Title:       "Simple Note",
		TextContent: "Just content",
	}

	md := note.ToMarkdown(false)

	if contains(md, "Created:") {
		t.Error("Markdown should not contain metadata when includeMetadata=false")
	}

	if !contains(md, "# Simple Note") {
		t.Error("Markdown should contain title")
	}
}

func TestKeepLibrary_JSONRoundTrip(t *testing.T) {
	library := metadata.KeepLibrary{
		Notes: []metadata.Note{
			{
				Title:       "Note 1",
				TextContent: "Content 1",
				Color:       "RED",
			},
			{
				Title:       "Note 2",
				TextContent: "Content 2",
				Color:       "BLUE",
			},
		},
		NotesPath: "/path/to/notes",
		Stats: map[string]int{
			"total_notes": 2,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(library)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal back
	var decoded metadata.KeepLibrary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Notes) != 2 {
		t.Errorf("Expected 2 notes after round-trip, got %d", len(decoded.Notes))
	}

	if decoded.Notes[0].Title != "Note 1" {
		t.Errorf("Expected first note title 'Note 1', got %q", decoded.Notes[0].Title)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
