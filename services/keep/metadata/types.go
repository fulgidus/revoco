// Package metadata provides Keep note parsing and structures.
package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Note represents a Google Keep note with all its metadata.
type Note struct {
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	TextContent string         `json:"textContent"`
	Created     time.Time      `json:"created"`
	Modified    time.Time      `json:"modified"`
	Labels      []Label        `json:"labels"`
	Color       string         `json:"color"`
	IsArchived  bool           `json:"isArchived"`
	IsPinned    bool           `json:"isPinned"`
	IsTrashed   bool           `json:"isTrashed"`
	Checkboxes  []Checkbox     `json:"checkboxes"`
	Attachments []Attachment   `json:"attachments"`
	Annotations []Annotation   `json:"annotations"`
	RawJSON     map[string]any `json:"-"`
}

// Label represents a Keep label/tag.
type Label struct {
	Name string `json:"name"`
}

// Checkbox represents a checklist item.
type Checkbox struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
}

// Attachment represents a file attached to a note.
type Attachment struct {
	FilePath string `json:"filePath"`
	MimeType string `json:"mimetype"`
}

// Annotation represents a link annotation in a note.
type Annotation struct {
	Description string `json:"description"`
	Source      string `json:"source"`
	Title       string `json:"title"`
	URL         string `json:"url"`
}

// KeepLibrary represents a collection of parsed Keep notes.
type KeepLibrary struct {
	Notes     []Note         `json:"notes"`
	NotesPath string         `json:"notes_path"`
	Stats     map[string]int `json:"stats"`
}

// keepRawNote represents the raw JSON structure from Google Takeout.
type keepRawNote struct {
	Title                   string                   `json:"title"`
	TextContent             string                   `json:"textContent"`
	ListContent             []map[string]interface{} `json:"listContent"`
	Labels                  []map[string]string      `json:"labels"`
	Color                   string                   `json:"color"`
	IsPinned                bool                     `json:"isPinned"`
	IsArchived              bool                     `json:"isArchived"`
	IsTrashed               bool                     `json:"isTrashed"`
	CreatedTimestampUsec    string                   `json:"createdTimestampUsec"`
	UserEditedTimestampUsec string                   `json:"userEditedTimestampUsec"`
	Attachments             []map[string]string      `json:"attachments"`
	Annotations             []map[string]string      `json:"annotations"`
}

// ParseKeepNote parses a Keep note JSON file.
func ParseKeepNote(jsonPath string) (Note, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return Note{}, fmt.Errorf("read file: %w", err)
	}

	var raw keepRawNote
	if err := json.Unmarshal(data, &raw); err != nil {
		return Note{}, fmt.Errorf("unmarshal JSON: %w", err)
	}

	note := Note{
		Title:       raw.Title,
		TextContent: raw.TextContent,
		Content:     raw.TextContent,
		Color:       raw.Color,
		IsPinned:    raw.IsPinned,
		IsArchived:  raw.IsArchived,
		IsTrashed:   raw.IsTrashed,
	}

	// Parse timestamps
	if raw.CreatedTimestampUsec != "" {
		note.Created, _ = parseKeepTimestamp(raw.CreatedTimestampUsec)
	}
	if raw.UserEditedTimestampUsec != "" {
		note.Modified, _ = parseKeepTimestamp(raw.UserEditedTimestampUsec)
	}

	// Parse labels
	for _, labelData := range raw.Labels {
		if name, ok := labelData["name"]; ok {
			note.Labels = append(note.Labels, Label{Name: name})
		}
	}

	// Parse checkboxes (listContent)
	for _, item := range raw.ListContent {
		text, _ := item["text"].(string)
		checked, _ := item["isChecked"].(bool)
		note.Checkboxes = append(note.Checkboxes, Checkbox{
			Text:    text,
			Checked: checked,
		})
	}

	// Parse attachments
	for _, attachData := range raw.Attachments {
		note.Attachments = append(note.Attachments, Attachment{
			FilePath: attachData["filePath"],
			MimeType: attachData["mimetype"],
		})
	}

	// Parse annotations (links)
	for _, annotData := range raw.Annotations {
		note.Annotations = append(note.Annotations, Annotation{
			Description: annotData["description"],
			Source:      annotData["source"],
			Title:       annotData["title"],
			URL:         annotData["url"],
		})
	}

	// Store raw JSON for custom fields
	var rawMap map[string]any
	json.Unmarshal(data, &rawMap)
	note.RawJSON = rawMap

	return note, nil
}

// parseKeepTimestamp converts Google Keep microsecond timestamps to time.Time.
// Keep timestamps are microseconds since Unix epoch.
func parseKeepTimestamp(usec string) (time.Time, error) {
	n, err := strconv.ParseInt(usec, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp: %w", err)
	}
	// Convert microseconds to nanoseconds
	return time.Unix(0, n*1000), nil
}

// FormatTimestamp formats a timestamp using the given format string.
func (n *Note) FormatTimestamp(layout string) string {
	if !n.Modified.IsZero() {
		return n.Modified.Format(layout)
	}
	if !n.Created.IsZero() {
		return n.Created.Format(layout)
	}
	return ""
}

// HasCheckboxes returns true if this note contains checkboxes.
func (n *Note) HasCheckboxes() bool {
	return len(n.Checkboxes) > 0
}

// HasAttachments returns true if this note has attachments.
func (n *Note) HasAttachments() bool {
	return len(n.Attachments) > 0
}

// HasAnnotations returns true if this note has link annotations.
func (n *Note) HasAnnotations() bool {
	return len(n.Annotations) > 0
}

// GetLabelNames returns a slice of label names.
func (n *Note) GetLabelNames() []string {
	names := make([]string, len(n.Labels))
	for i, label := range n.Labels {
		names[i] = label.Name
	}
	return names
}

// IsEmpty returns true if the note has no meaningful content.
func (n *Note) IsEmpty() bool {
	return n.Title == "" && n.TextContent == "" && len(n.Checkboxes) == 0
}

// ContentType returns a string describing the note's content type.
func (n *Note) ContentType() string {
	if len(n.Checkboxes) > 0 {
		return "checklist"
	}
	if len(n.Attachments) > 0 {
		return "note_with_attachments"
	}
	if n.TextContent != "" {
		return "text_note"
	}
	return "empty"
}

// CheckboxesMarkdown returns the checkboxes formatted as Markdown.
func (n *Note) CheckboxesMarkdown() string {
	if len(n.Checkboxes) == 0 {
		return ""
	}

	result := ""
	for _, item := range n.Checkboxes {
		if item.Checked {
			result += "- [x] " + item.Text + "\n"
		} else {
			result += "- [ ] " + item.Text + "\n"
		}
	}
	return result
}

// LabelsMarkdown returns the labels formatted as Markdown tags.
func (n *Note) LabelsMarkdown() string {
	if len(n.Labels) == 0 {
		return ""
	}

	result := ""
	for _, label := range n.Labels {
		result += "#" + label.Name + " "
	}
	return result
}

// AttachmentsMarkdown returns the attachments formatted as Markdown.
func (n *Note) AttachmentsMarkdown() string {
	if len(n.Attachments) == 0 {
		return ""
	}

	result := "\n## Attachments\n\n"
	for _, att := range n.Attachments {
		result += fmt.Sprintf("- [%s](%s) (%s)\n", att.FilePath, att.FilePath, att.MimeType)
	}
	return result
}

// AnnotationsMarkdown returns the annotations formatted as Markdown links.
func (n *Note) AnnotationsMarkdown() string {
	if len(n.Annotations) == 0 {
		return ""
	}

	result := "\n## Links\n\n"
	for _, ann := range n.Annotations {
		if ann.URL != "" {
			title := ann.Title
			if title == "" {
				title = ann.URL
			}
			result += fmt.Sprintf("- [%s](%s)", title, ann.URL)
			if ann.Description != "" {
				result += " - " + ann.Description
			}
			result += "\n"
		}
	}
	return result
}

// ToMarkdown converts the note to Markdown format.
func (n *Note) ToMarkdown(includeMetadata bool) string {
	result := ""

	// Title
	if n.Title != "" {
		result += "# " + n.Title + "\n\n"
	}

	// Metadata
	if includeMetadata {
		if !n.Created.IsZero() {
			result += "*Created: " + n.Created.Format("2006-01-02 15:04:05") + "*\n"
		}
		if !n.Modified.IsZero() {
			result += "*Modified: " + n.Modified.Format("2006-01-02 15:04:05") + "*\n"
		}
		if len(n.Labels) > 0 {
			result += "\n**Tags:** " + n.LabelsMarkdown() + "\n"
		}
		if n.Color != "" && n.Color != "DEFAULT" {
			result += "*Color: " + n.Color + "*\n"
		}
		result += "\n"
	}

	// Text content
	if n.TextContent != "" {
		result += n.TextContent + "\n\n"
	}

	// Checkboxes
	if len(n.Checkboxes) > 0 {
		result += n.CheckboxesMarkdown() + "\n"
	}

	// Annotations
	result += n.AnnotationsMarkdown()

	// Attachments
	result += n.AttachmentsMarkdown()

	return result
}
