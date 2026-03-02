// Package services provides comprehensive QA tests for all Google Takeout service parsers.
// This file tests: gmail, contacts, calendar, keep, tasks, maps, chrome, fit, passwords.
package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	calendarMeta "github.com/fulgidus/revoco/services/calendar/metadata"
	chromeMeta "github.com/fulgidus/revoco/services/chrome/metadata"
	contactsMeta "github.com/fulgidus/revoco/services/contacts/metadata"
	fitMeta "github.com/fulgidus/revoco/services/fit/metadata"
	gmailMeta "github.com/fulgidus/revoco/services/gmail/metadata"
	keepMeta "github.com/fulgidus/revoco/services/keep/metadata"
	mapsMeta "github.com/fulgidus/revoco/services/maps/metadata"
	passwordsMeta "github.com/fulgidus/revoco/services/passwords/metadata"
	tasksMeta "github.com/fulgidus/revoco/services/tasks/metadata"
)

// ============================================================================
// GMAIL TESTS
// ============================================================================

func TestGmail_ParseMboxHeader_ValidEmail(t *testing.T) {
	header := "From: alice@example.com\r\n" +
		"To: bob@example.com, charlie@example.com\r\n" +
		"Cc: dave@example.com\r\n" +
		"Bcc: secret@example.com\r\n" +
		"Subject: Quarterly Report\r\n" +
		"Date: Wed, 15 Jan 2025 10:30:00 -0500\r\n" +
		"Message-ID: <abc123@mail.example.com>\r\n" +
		"In-Reply-To: <parent456@mail.example.com>\r\n" +
		"Content-Type: multipart/mixed; boundary=----boundary\r\n" +
		"X-Gmail-Labels: Inbox,Important,Work\r\n" +
		"\r\n" +
		"Body text here.\r\n"

	msg, err := gmailMeta.ParseMboxHeader(header)
	if err != nil {
		t.Fatalf("ParseMboxHeader failed: %v", err)
	}

	// Validate all fields
	if msg.From != "alice@example.com" {
		t.Errorf("From = %q, want %q", msg.From, "alice@example.com")
	}
	if len(msg.To) != 2 {
		t.Errorf("To count = %d, want 2", len(msg.To))
	}
	if len(msg.CC) != 1 {
		t.Errorf("CC count = %d, want 1", len(msg.CC))
	}
	if len(msg.BCC) != 1 {
		t.Errorf("BCC count = %d, want 1", len(msg.BCC))
	}
	if msg.Subject != "Quarterly Report" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "Quarterly Report")
	}
	if msg.MessageID != "<abc123@mail.example.com>" {
		t.Errorf("MessageID = %q", msg.MessageID)
	}
	if msg.InReplyTo != "<parent456@mail.example.com>" {
		t.Errorf("InReplyTo = %q", msg.InReplyTo)
	}
	if !msg.HasAttachments {
		t.Error("HasAttachments should be true for multipart/mixed")
	}
	if len(msg.Labels) != 3 {
		t.Errorf("Labels count = %d, want 3", len(msg.Labels))
	}
	if msg.Date.IsZero() {
		t.Error("Date should not be zero")
	}
}

func TestGmail_ParseMboxHeader_MinimalEmail(t *testing.T) {
	header := "From: test@example.com\r\n\r\n"
	msg, err := gmailMeta.ParseMboxHeader(header)
	if err != nil {
		t.Fatalf("ParseMboxHeader failed on minimal email: %v", err)
	}
	if msg.From != "test@example.com" {
		t.Errorf("From = %q", msg.From)
	}
	if msg.HasAttachments {
		t.Error("HasAttachments should be false for minimal email")
	}
}

func TestGmail_ParseMboxHeader_InvalidInput(t *testing.T) {
	_, err := gmailMeta.ParseMboxHeader("")
	if err == nil {
		t.Error("Expected error on empty input")
	}
}

func TestGmail_CSVOutput(t *testing.T) {
	headers := gmailMeta.CSVHeaders()
	if len(headers) != 7 {
		t.Errorf("CSVHeaders count = %d, want 7", len(headers))
	}

	header := "From: test@example.com\r\n" +
		"Subject: CSV Test\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 -0700\r\n" +
		"Message-ID: <csv-test@example.com>\r\n" +
		"X-Gmail-Labels: Sent\r\n" +
		"\r\nBody\r\n"

	msg, err := gmailMeta.ParseMboxHeader(header)
	if err != nil {
		t.Fatalf("ParseMboxHeader failed: %v", err)
	}
	row := msg.ToCSVRow()
	if len(row) != len(headers) {
		t.Errorf("CSV row length %d != headers length %d", len(row), len(headers))
	}
	if row[0] != "<csv-test@example.com>" {
		t.Errorf("CSV row[0] (MessageID) = %q", row[0])
	}
}

// ============================================================================
// CONTACTS TESTS
// ============================================================================

func TestContacts_ParseVCard_SingleContact(t *testing.T) {
	vcard := `BEGIN:VCARD
VERSION:3.0
UID:contact-1234
FN:John Doe
N:Doe;John;Michael;Mr.;III
NICKNAME:Johnny
EMAIL;TYPE=HOME:john@example.com
EMAIL;TYPE=WORK;PREF:john.doe@company.com
TEL;TYPE=CELL:+1-555-0100
TEL;TYPE=HOME:+1-555-0101
ADR;TYPE=HOME:;;123 Main St;Springfield;IL;62701;USA
ORG:Acme Corp
TITLE:Software Engineer
BDAY:1990-05-15
NOTE:This is a test contact
CATEGORIES:Friends,Work
URL:https://johndoe.example.com
END:VCARD`

	contacts, err := contactsMeta.ParseVCard(strings.NewReader(vcard))
	if err != nil {
		t.Fatalf("ParseVCard failed: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("Expected 1 contact, got %d", len(contacts))
	}

	c := contacts[0]
	if c.UID != "contact-1234" {
		t.Errorf("UID = %q", c.UID)
	}
	if c.FullName != "John Doe" {
		t.Errorf("FullName = %q", c.FullName)
	}
	if c.GivenName != "John" {
		t.Errorf("GivenName = %q", c.GivenName)
	}
	if c.FamilyName != "Doe" {
		t.Errorf("FamilyName = %q", c.FamilyName)
	}
	if c.MiddleName != "Michael" {
		t.Errorf("MiddleName = %q", c.MiddleName)
	}
	if c.Prefix != "Mr." {
		t.Errorf("Prefix = %q", c.Prefix)
	}
	if c.Suffix != "III" {
		t.Errorf("Suffix = %q", c.Suffix)
	}
	if c.Nickname != "Johnny" {
		t.Errorf("Nickname = %q", c.Nickname)
	}
	if len(c.Emails) != 2 {
		t.Errorf("Emails count = %d, want 2", len(c.Emails))
	}
	if len(c.Phones) != 2 {
		t.Errorf("Phones count = %d, want 2", len(c.Phones))
	}
	if len(c.Addresses) != 1 {
		t.Errorf("Addresses count = %d, want 1", len(c.Addresses))
	} else {
		if c.Addresses[0].Street != "123 Main St" {
			t.Errorf("Address Street = %q", c.Addresses[0].Street)
		}
		if c.Addresses[0].City != "Springfield" {
			t.Errorf("Address City = %q", c.Addresses[0].City)
		}
		if c.Addresses[0].Country != "USA" {
			t.Errorf("Address Country = %q", c.Addresses[0].Country)
		}
	}
	if c.Organization != "Acme Corp" {
		t.Errorf("Organization = %q", c.Organization)
	}
	if c.Title != "Software Engineer" {
		t.Errorf("Title = %q", c.Title)
	}
	if c.Birthday == nil {
		t.Error("Birthday is nil, expected 1990-05-15")
	} else {
		if c.Birthday.Format("2006-01-02") != "1990-05-15" {
			t.Errorf("Birthday = %v", c.Birthday)
		}
	}
	if c.Notes != "This is a test contact" {
		t.Errorf("Notes = %q", c.Notes)
	}
	if c.URL != "https://johndoe.example.com" {
		t.Errorf("URL = %q", c.URL)
	}
	if c.Version != "3.0" {
		t.Errorf("Version = %q", c.Version)
	}
}

func TestContacts_ParseVCard_MultipleContacts(t *testing.T) {
	vcard := `BEGIN:VCARD
VERSION:3.0
FN:Alice Smith
EMAIL:alice@example.com
END:VCARD
BEGIN:VCARD
VERSION:3.0
FN:Bob Jones
EMAIL:bob@example.com
END:VCARD
BEGIN:VCARD
VERSION:3.0
FN:Charlie Brown
END:VCARD`

	contacts, err := contactsMeta.ParseVCard(strings.NewReader(vcard))
	if err != nil {
		t.Fatalf("ParseVCard failed: %v", err)
	}
	if len(contacts) != 3 {
		t.Errorf("Expected 3 contacts, got %d", len(contacts))
	}
}

func TestContacts_ParseVCard_EmptyInput(t *testing.T) {
	contacts, err := contactsMeta.ParseVCard(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseVCard error on empty: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("Expected 0 contacts on empty input, got %d", len(contacts))
	}
}

func TestContacts_CSVOutput(t *testing.T) {
	headers := contactsMeta.CSVHeaders()
	if len(headers) != 10 {
		t.Errorf("CSVHeaders count = %d, want 10", len(headers))
	}

	vcard := `BEGIN:VCARD
VERSION:3.0
FN:CSV Test
EMAIL:csvtest@example.com
TEL:+1-555-0199
END:VCARD`

	contacts, err := contactsMeta.ParseVCard(strings.NewReader(vcard))
	if err != nil {
		t.Fatalf("ParseVCard failed: %v", err)
	}
	if len(contacts) < 1 {
		t.Fatal("No contacts parsed")
	}
	row := contacts[0].ToCSVRow()
	if len(row) != len(headers) {
		t.Errorf("CSV row length %d != headers length %d", len(row), len(headers))
	}
}

// ============================================================================
// CALENDAR TESTS
// ============================================================================

func TestCalendar_ParseICS_ValidCalendar(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Google Inc//Google Calendar 70.9054//EN
X-WR-CALNAME:My Calendar
X-WR-CALDESC:Personal events
X-WR-TIMEZONE:America/New_York
BEGIN:VEVENT
UID:event-abc123@google.com
DTSTART:20250115T100000Z
DTEND:20250115T110000Z
SUMMARY:Team Meeting
DESCRIPTION:Discuss Q1 goals
LOCATION:Conference Room A
STATUS:CONFIRMED
ORGANIZER:mailto:boss@example.com
ATTENDEE:mailto:alice@example.com
ATTENDEE:mailto:bob@example.com
RRULE:FREQ=WEEKLY;BYDAY=WE
CREATED:20250101T000000Z
LAST-MODIFIED:20250114T120000Z
END:VEVENT
BEGIN:VEVENT
UID:event-def456@google.com
DTSTART:20250120
DTEND:20250121
SUMMARY:Holiday
STATUS:CONFIRMED
END:VEVENT
END:VCALENDAR`

	calendars, err := calendarMeta.ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}
	if len(calendars) != 1 {
		t.Fatalf("Expected 1 calendar, got %d", len(calendars))
	}

	cal := calendars[0]
	if cal.Name != "My Calendar" {
		t.Errorf("Calendar Name = %q", cal.Name)
	}
	if cal.Description != "Personal events" {
		t.Errorf("Calendar Description = %q", cal.Description)
	}
	if cal.Timezone != "America/New_York" {
		t.Errorf("Calendar Timezone = %q", cal.Timezone)
	}
	if len(cal.Events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(cal.Events))
	}

	// Verify first event
	event := cal.Events[0]
	if event.UID != "event-abc123@google.com" {
		t.Errorf("Event UID = %q", event.UID)
	}
	if event.Summary != "Team Meeting" {
		t.Errorf("Event Summary = %q", event.Summary)
	}
	if event.Description != "Discuss Q1 goals" {
		t.Errorf("Event Description = %q", event.Description)
	}
	if event.Location != "Conference Room A" {
		t.Errorf("Event Location = %q", event.Location)
	}
	if event.Status != "CONFIRMED" {
		t.Errorf("Event Status = %q", event.Status)
	}
	if event.Organizer != "boss@example.com" {
		t.Errorf("Event Organizer = %q", event.Organizer)
	}
	if len(event.Attendees) != 2 {
		t.Errorf("Event Attendees count = %d, want 2", len(event.Attendees))
	}
	if event.Recurrence != "FREQ=WEEKLY;BYDAY=WE" {
		t.Errorf("Event Recurrence = %q", event.Recurrence)
	}
	if event.StartTime.IsZero() {
		t.Error("Event StartTime is zero")
	}
	if event.EndTime.IsZero() {
		t.Error("Event EndTime is zero")
	}
	if event.Created.IsZero() {
		t.Error("Event Created is zero")
	}
	if event.Modified.IsZero() {
		t.Error("Event Modified is zero")
	}

	// Verify date-only event
	dateEvent := cal.Events[1]
	if dateEvent.Summary != "Holiday" {
		t.Errorf("Date event Summary = %q", dateEvent.Summary)
	}
	if dateEvent.StartTime.IsZero() {
		t.Error("Date event StartTime is zero")
	}
}

func TestCalendar_ParseICS_EmptyInput(t *testing.T) {
	calendars, err := calendarMeta.ParseICS(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseICS error on empty: %v", err)
	}
	if len(calendars) != 0 {
		t.Errorf("Expected 0 calendars on empty input, got %d", len(calendars))
	}
}

func TestCalendar_BuildICS_RoundTrip(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
X-WR-CALNAME:Roundtrip Test
BEGIN:VEVENT
UID:rt-001@test
DTSTART:20250301T090000Z
DTEND:20250301T100000Z
SUMMARY:Roundtrip Event
DESCRIPTION:Testing ICS roundtrip with special chars: comma\, semicolon\;
STATUS:CONFIRMED
END:VEVENT
END:VCALENDAR`

	calendars, err := calendarMeta.ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}
	if len(calendars) != 1 || len(calendars[0].Events) != 1 {
		t.Fatal("Expected 1 calendar with 1 event")
	}

	// Build ICS from parsed data
	output := calendarMeta.BuildICS(&calendars[0])
	if !strings.Contains(output, "BEGIN:VCALENDAR") {
		t.Error("BuildICS missing BEGIN:VCALENDAR")
	}
	if !strings.Contains(output, "END:VCALENDAR") {
		t.Error("BuildICS missing END:VCALENDAR")
	}
	if !strings.Contains(output, "BEGIN:VEVENT") {
		t.Error("BuildICS missing BEGIN:VEVENT")
	}
	if !strings.Contains(output, "END:VEVENT") {
		t.Error("BuildICS missing END:VEVENT")
	}
	if !strings.Contains(output, "VERSION:2.0") {
		t.Error("BuildICS missing VERSION:2.0")
	}
	if !strings.Contains(output, "Roundtrip Test") {
		t.Error("BuildICS missing calendar name")
	}
	if !strings.Contains(output, "Roundtrip Event") {
		t.Error("BuildICS missing event summary")
	}
}

func TestCalendar_EscapeUnescapeText(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello\\nWorld", "Hello\nWorld"},
		{"A\\,B\\;C", "A,B;C"},
		{"Back\\\\slash", "Back\\slash"},
		{"No escapes", "No escapes"},
	}

	for _, tt := range tests {
		got := calendarMeta.EscapeText(tt.want)
		// Escape then check it differs from raw
		if !strings.Contains(got, "\\") && strings.ContainsAny(tt.want, ",;\n\\") {
			t.Errorf("EscapeText(%q) should contain backslash escapes", tt.want)
		}
	}
}

func TestCalendar_FoldLine(t *testing.T) {
	short := "SHORT LINE"
	if calendarMeta.FoldLine(short) != short {
		t.Errorf("FoldLine should not modify lines <= 75 chars")
	}

	long := strings.Repeat("A", 200)
	folded := calendarMeta.FoldLine(long)
	if !strings.Contains(folded, "\r\n ") {
		t.Error("FoldLine should fold long lines with CRLF+space")
	}
}

// ============================================================================
// KEEP TESTS
// ============================================================================

func TestKeep_ParseKeepNote_FullNote(t *testing.T) {
	noteJSON := `{
		"title": "Shopping List",
		"textContent": "Buy groceries",
		"color": "YELLOW",
		"isPinned": true,
		"isArchived": false,
		"isTrashed": false,
		"createdTimestampUsec": "1704067200000000",
		"userEditedTimestampUsec": "1704153600000000",
		"labels": [
			{"name": "Shopping"},
			{"name": "Personal"}
		],
		"listContent": [
			{"text": "Milk", "isChecked": true},
			{"text": "Eggs", "isChecked": false},
			{"text": "Bread", "isChecked": false}
		],
		"attachments": [
			{"filePath": "image.jpg", "mimetype": "image/jpeg"}
		],
		"annotations": [
			{
				"description": "Recipe link",
				"source": "web",
				"title": "Best Bread Recipe",
				"url": "https://example.com/bread"
			}
		]
	}`

	tmpDir := t.TempDir()
	notePath := filepath.Join(tmpDir, "note.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0644); err != nil {
		t.Fatalf("Failed to write temp note: %v", err)
	}

	note, err := keepMeta.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed: %v", err)
	}

	if note.Title != "Shopping List" {
		t.Errorf("Title = %q", note.Title)
	}
	if note.TextContent != "Buy groceries" {
		t.Errorf("TextContent = %q", note.TextContent)
	}
	if note.Color != "YELLOW" {
		t.Errorf("Color = %q", note.Color)
	}
	if !note.IsPinned {
		t.Error("IsPinned should be true")
	}
	if note.IsArchived {
		t.Error("IsArchived should be false")
	}
	if note.Created.IsZero() {
		t.Error("Created should not be zero")
	}
	if note.Modified.IsZero() {
		t.Error("Modified should not be zero")
	}
	if len(note.Labels) != 2 {
		t.Errorf("Labels count = %d, want 2", len(note.Labels))
	}
	if len(note.Checkboxes) != 3 {
		t.Errorf("Checkboxes count = %d, want 3", len(note.Checkboxes))
	}
	if !note.Checkboxes[0].Checked {
		t.Error("First checkbox should be checked")
	}
	if note.Checkboxes[1].Checked {
		t.Error("Second checkbox should not be checked")
	}
	if len(note.Attachments) != 1 {
		t.Errorf("Attachments count = %d, want 1", len(note.Attachments))
	}
	if len(note.Annotations) != 1 {
		t.Errorf("Annotations count = %d, want 1", len(note.Annotations))
	}
	if note.Annotations[0].URL != "https://example.com/bread" {
		t.Errorf("Annotation URL = %q", note.Annotations[0].URL)
	}
}

func TestKeep_ParseKeepNote_EmptyNote(t *testing.T) {
	noteJSON := `{"title": "", "textContent": ""}`
	tmpDir := t.TempDir()
	notePath := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0644); err != nil {
		t.Fatalf("Failed to write temp note: %v", err)
	}

	note, err := keepMeta.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed on empty: %v", err)
	}
	if !note.IsEmpty() {
		t.Error("Empty note should report IsEmpty() = true")
	}
	if note.ContentType() != "empty" {
		t.Errorf("ContentType = %q, want empty", note.ContentType())
	}
}

func TestKeep_ParseKeepNote_InvalidPath(t *testing.T) {
	_, err := keepMeta.ParseKeepNote("/nonexistent/path/note.json")
	if err == nil {
		t.Error("Expected error on nonexistent file")
	}
}

func TestKeep_ParseKeepNote_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	notePath := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(notePath, []byte("not json"), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	_, err := keepMeta.ParseKeepNote(notePath)
	if err == nil {
		t.Error("Expected error on invalid JSON")
	}
}

func TestKeep_ToMarkdown(t *testing.T) {
	noteJSON := `{
		"title": "Markdown Test",
		"textContent": "Hello world",
		"createdTimestampUsec": "1704067200000000",
		"labels": [{"name": "TestLabel"}],
		"listContent": [
			{"text": "Done item", "isChecked": true},
			{"text": "Todo item", "isChecked": false}
		]
	}`
	tmpDir := t.TempDir()
	notePath := filepath.Join(tmpDir, "md.json")
	if err := os.WriteFile(notePath, []byte(noteJSON), 0644); err != nil {
		t.Fatalf("Failed to write temp note: %v", err)
	}

	note, err := keepMeta.ParseKeepNote(notePath)
	if err != nil {
		t.Fatalf("ParseKeepNote failed: %v", err)
	}

	md := note.ToMarkdown(true)
	if !strings.Contains(md, "# Markdown Test") {
		t.Error("Markdown missing title")
	}
	if !strings.Contains(md, "Hello world") {
		t.Error("Markdown missing text content")
	}
	if !strings.Contains(md, "- [x] Done item") {
		t.Error("Markdown missing checked checkbox")
	}
	if !strings.Contains(md, "- [ ] Todo item") {
		t.Error("Markdown missing unchecked checkbox")
	}
	if !strings.Contains(md, "#TestLabel") {
		t.Error("Markdown missing label tag")
	}
}

// ============================================================================
// TASKS TESTS
// ============================================================================

func TestTasks_ParseTasksJSON_ValidList(t *testing.T) {
	tasksJSON := `{
		"kind": "tasks#taskLists",
		"items": [
			{
				"kind": "tasks#taskList",
				"id": "list-001",
				"title": "My Tasks",
				"updated": "2025-01-15T10:00:00Z",
				"tasks": [
					{
						"id": "task-001",
						"title": "Buy groceries",
						"notes": "From the organic store",
						"status": "needsAction",
						"due": "2025-01-20T00:00:00Z",
						"parent": "",
						"position": "00000001",
						"links": [
							{
								"description": "Store website",
								"link": "https://store.example.com",
								"type": "related"
							}
						],
						"deleted": false,
						"updated": "2025-01-15T10:00:00Z"
					},
					{
						"id": "task-002",
						"title": "Complete report",
						"status": "completed",
						"completed": "2025-01-14T16:00:00Z",
						"position": "00000002",
						"deleted": false,
						"updated": "2025-01-14T16:00:00Z"
					},
					{
						"id": "task-003",
						"title": "Deleted task",
						"status": "needsAction",
						"position": "00000003",
						"deleted": true,
						"updated": "2025-01-10T00:00:00Z"
					}
				]
			}
		]
	}`

	list, err := tasksMeta.ParseTasksJSON(strings.NewReader(tasksJSON))
	if err != nil {
		t.Fatalf("ParseTasksJSON failed: %v", err)
	}

	if list.Title != "My Tasks" {
		t.Errorf("Title = %q", list.Title)
	}
	if list.LastModified.IsZero() {
		t.Error("LastModified is zero")
	}
	if len(list.Tasks) != 3 {
		t.Fatalf("Tasks count = %d, want 3", len(list.Tasks))
	}

	// Task 1 - pending with due date and links
	task1 := list.Tasks[0]
	if task1.Title != "Buy groceries" {
		t.Errorf("Task1 Title = %q", task1.Title)
	}
	if task1.Notes != "From the organic store" {
		t.Errorf("Task1 Notes = %q", task1.Notes)
	}
	if task1.IsCompleted() {
		t.Error("Task1 should not be completed")
	}
	if !task1.HasDueDate() {
		t.Error("Task1 should have due date")
	}
	if !task1.HasNotes() {
		t.Error("Task1 should have notes")
	}
	if !task1.HasLinks() {
		t.Error("Task1 should have links")
	}
	if len(task1.Links) != 1 {
		t.Errorf("Task1 Links count = %d", len(task1.Links))
	}

	// Task 2 - completed
	task2 := list.Tasks[1]
	if !task2.IsCompleted() {
		t.Error("Task2 should be completed")
	}
	if task2.Completed.IsZero() {
		t.Error("Task2 Completed timestamp should not be zero")
	}

	// Task 3 - deleted
	task3 := list.Tasks[2]
	if !task3.IsDeleted {
		t.Error("Task3 should be deleted")
	}

	// Verify counts
	if list.CountCompleted() != 1 {
		t.Errorf("CountCompleted = %d, want 1", list.CountCompleted())
	}
	if list.CountPending() != 1 {
		t.Errorf("CountPending = %d, want 1 (excludes deleted)", list.CountPending())
	}
}

func TestTasks_ParseTasksJSON_EmptyList(t *testing.T) {
	tasksJSON := `{"kind": "tasks#taskLists", "items": []}`
	_, err := tasksMeta.ParseTasksJSON(strings.NewReader(tasksJSON))
	if err == nil {
		t.Error("Expected error on empty items array")
	}
}

func TestTasks_ParseTasksJSON_InvalidJSON(t *testing.T) {
	_, err := tasksMeta.ParseTasksJSON(strings.NewReader("not json"))
	if err == nil {
		t.Error("Expected error on invalid JSON")
	}
}

func TestTasks_ToMarkdown(t *testing.T) {
	tasksJSON := `{
		"kind": "tasks#taskLists",
		"items": [{
			"kind": "tasks#taskList",
			"id": "list-md",
			"title": "Markdown List",
			"updated": "2025-01-15T10:00:00Z",
			"tasks": [
				{"id": "t1", "title": "Done", "status": "completed", "position": "1", "updated": "2025-01-15T10:00:00Z"},
				{"id": "t2", "title": "Pending", "status": "needsAction", "due": "2025-02-01T00:00:00Z", "position": "2", "updated": "2025-01-15T10:00:00Z"}
			]
		}]
	}`

	list, err := tasksMeta.ParseTasksJSON(strings.NewReader(tasksJSON))
	if err != nil {
		t.Fatalf("ParseTasksJSON failed: %v", err)
	}

	md := list.Tasks[0].ToMarkdown(0)
	if !strings.Contains(md, "[x]") {
		t.Error("Completed task markdown should contain [x]")
	}

	md2 := list.Tasks[1].ToMarkdown(0)
	if !strings.Contains(md2, "[ ]") {
		t.Error("Pending task markdown should contain [ ]")
	}
	if !strings.Contains(md2, "due:") {
		t.Error("Task with due date markdown should contain due date")
	}
}

// ============================================================================
// MAPS TESTS
// ============================================================================

func TestMaps_ParseRecordsJSON_Valid(t *testing.T) {
	recordsJSON := `{
		"locations": [
			{
				"latitudeE7": 408900000,
				"longitudeE7": -740600000,
				"accuracy": 20,
				"timestamp": "2025-01-15T10:30:00Z",
				"source": "WIFI",
				"altitude": 50,
				"velocity": 5,
				"heading": 90
			},
			{
				"latitudeE7": 518000000,
				"longitudeE7": -1200000,
				"accuracy": 15,
				"timestamp": "2025-01-15T11:00:00Z",
				"source": "GPS"
			}
		]
	}`

	records, err := mapsMeta.ParseRecordsJSON(strings.NewReader(recordsJSON))
	if err != nil {
		t.Fatalf("ParseRecordsJSON failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}

	r1 := records[0]
	// E7 conversion: 408900000 / 1e7 = 40.89
	if r1.Latitude < 40.88 || r1.Latitude > 40.90 {
		t.Errorf("Latitude = %f, expected ~40.89", r1.Latitude)
	}
	if r1.Longitude > -74.05 || r1.Longitude < -74.07 {
		t.Errorf("Longitude = %f, expected ~-74.06", r1.Longitude)
	}
	if r1.Accuracy != 20 {
		t.Errorf("Accuracy = %d, want 20", r1.Accuracy)
	}
	if r1.Source != "WIFI" {
		t.Errorf("Source = %q", r1.Source)
	}
	if r1.Timestamp.IsZero() {
		t.Error("Timestamp is zero")
	}
	if r1.Altitude != 50.0 {
		t.Errorf("Altitude = %f, want 50", r1.Altitude)
	}
}

func TestMaps_ParseRecordsJSON_EmptyLocations(t *testing.T) {
	recordsJSON := `{"locations": []}`
	records, err := mapsMeta.ParseRecordsJSON(strings.NewReader(recordsJSON))
	if err != nil {
		t.Fatalf("ParseRecordsJSON error on empty: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("Expected 0 records, got %d", len(records))
	}
}

func TestMaps_ParseRecordsJSON_InvalidTimestamp(t *testing.T) {
	recordsJSON := `{
		"locations": [
			{
				"latitudeE7": 408900000,
				"longitudeE7": -740600000,
				"accuracy": 20,
				"timestamp": "not-a-timestamp"
			}
		]
	}`
	records, err := mapsMeta.ParseRecordsJSON(strings.NewReader(recordsJSON))
	if err != nil {
		t.Fatalf("ParseRecordsJSON failed: %v", err)
	}
	// Invalid timestamps are skipped
	if len(records) != 0 {
		t.Errorf("Expected 0 records (invalid timestamp skipped), got %d", len(records))
	}
}

func TestMaps_ParseSavedPlacesKML_Valid(t *testing.T) {
	kml := `<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2">
<Document>
	<Placemark>
		<name>Home</name>
		<description>123 Main St, Springfield, IL</description>
		<Point>
			<coordinates>-89.6501,39.7817,0</coordinates>
		</Point>
	</Placemark>
	<Placemark>
		<name>Office</name>
		<description>456 Business Ave</description>
		<Point>
			<coordinates>-87.6298,41.8781,0</coordinates>
		</Point>
		<ExtendedData>
			<Data name="gx_media_links">
				<value>https://maps.google.com/place123</value>
			</Data>
		</ExtendedData>
	</Placemark>
</Document>
</kml>`

	places, err := mapsMeta.ParseSavedPlacesKML(strings.NewReader(kml))
	if err != nil {
		t.Fatalf("ParseSavedPlacesKML failed: %v", err)
	}
	if len(places) != 2 {
		t.Fatalf("Expected 2 places, got %d", len(places))
	}

	if places[0].Name != "Home" {
		t.Errorf("Place[0] Name = %q", places[0].Name)
	}
	// KML coordinates are lon,lat
	if places[0].Latitude < 39.7 || places[0].Latitude > 39.8 {
		t.Errorf("Place[0] Latitude = %f", places[0].Latitude)
	}
	if places[1].GoogleMapsURL != "https://maps.google.com/place123" {
		t.Errorf("Place[1] GoogleMapsURL = %q", places[1].GoogleMapsURL)
	}
}

func TestMaps_ParseTimelineJSON_Valid(t *testing.T) {
	timelineJSON := `{
		"timelineObjects": [
			{
				"placeVisit": {
					"location": {
						"latitudeE7": 408900000,
						"longitudeE7": -740600000,
						"name": "Coffee Shop",
						"address": "789 Brew Lane"
					},
					"duration": {
						"startTimestamp": "2025-01-15T08:00:00Z",
						"endTimestamp": "2025-01-15T09:00:00Z"
					}
				}
			},
			{
				"activitySegment": {
					"startLocation": {"latitudeE7": 408900000, "longitudeE7": -740600000},
					"endLocation": {"latitudeE7": 409000000, "longitudeE7": -740700000},
					"duration": {
						"startTimestamp": "2025-01-15T09:00:00Z",
						"endTimestamp": "2025-01-15T09:30:00Z"
					},
					"activityType": "WALKING"
				}
			}
		]
	}`

	visits, err := mapsMeta.ParseTimelineJSON(strings.NewReader(timelineJSON))
	if err != nil {
		t.Fatalf("ParseTimelineJSON failed: %v", err)
	}
	if len(visits) != 1 {
		t.Fatalf("Expected 1 visit (activity segments not returned), got %d", len(visits))
	}

	v := visits[0]
	if v.Location.Name != "Coffee Shop" {
		t.Errorf("Visit Location Name = %q", v.Location.Name)
	}
	if v.StartTime.IsZero() {
		t.Error("Visit StartTime is zero")
	}
	if v.Duration <= 0 {
		t.Errorf("Visit Duration = %v, want > 0", v.Duration)
	}
}

// ============================================================================
// CHROME TESTS
// ============================================================================

func TestChrome_ParseBookmarksHTML_Valid(t *testing.T) {
	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><H3 ADD_DATE="1700000000" LAST_MODIFIED="1700100000">Bookmarks Bar</H3>
    <DL><p>
        <DT><A HREF="https://golang.org" ADD_DATE="1700000001">Go Programming</A>
        <DT><H3>Tech</H3>
        <DL><p>
            <DT><A HREF="https://github.com" ADD_DATE="1700000002">GitHub</A>
            <DT><A HREF="https://news.ycombinator.com" ADD_DATE="1700000003">Hacker News</A>
        </DL><p>
    </DL><p>
    <DT><H3>Other</H3>
    <DL><p>
        <DT><A HREF="https://example.com">Example Site</A>
    </DL><p>
</DL><p>`

	bookmarks, err := chromeMeta.ParseBookmarksHTML(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseBookmarksHTML failed: %v", err)
	}
	if len(bookmarks) != 4 {
		t.Fatalf("Expected 4 bookmarks, got %d", len(bookmarks))
	}

	// Verify hierarchy
	bm0 := bookmarks[0] // Go Programming in Bookmarks Bar
	if bm0.Name != "Go Programming" {
		t.Errorf("Bookmark[0] Name = %q", bm0.Name)
	}
	if bm0.URL != "https://golang.org" {
		t.Errorf("Bookmark[0] URL = %q", bm0.URL)
	}
	if bm0.Folder != "Bookmarks Bar" {
		t.Errorf("Bookmark[0] Folder = %q, want 'Bookmarks Bar'", bm0.Folder)
	}
	if bm0.DateAdded.IsZero() {
		t.Error("Bookmark[0] DateAdded is zero")
	}

	// GitHub should be in Tech subfolder
	bm1 := bookmarks[1]
	if bm1.Name != "GitHub" {
		t.Errorf("Bookmark[1] Name = %q", bm1.Name)
	}
	if bm1.Folder != "Tech" {
		t.Errorf("Bookmark[1] Folder = %q, want 'Tech'", bm1.Folder)
	}
	if len(bm1.ParentFolders) != 2 {
		t.Errorf("Bookmark[1] ParentFolders len = %d, want 2", len(bm1.ParentFolders))
	}
}

func TestChrome_ParseBookmarksHTML_Empty(t *testing.T) {
	bookmarks, err := chromeMeta.ParseBookmarksHTML(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseBookmarksHTML error on empty: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Errorf("Expected 0 bookmarks, got %d", len(bookmarks))
	}
}

func TestChrome_ParseBrowserHistoryJSON_Valid(t *testing.T) {
	historyJSON := `[
		{
			"favicon_url": "https://example.com/favicon.ico",
			"page_transition": "LINK",
			"title": "Example Page",
			"url": "https://example.com/page1",
			"client_id": "client1",
			"time_usec": 1705312200000000
		},
		{
			"favicon_url": "",
			"page_transition": "TYPED",
			"title": "Another Page",
			"url": "https://another.com",
			"client_id": "client1",
			"time_usec": 1705312300000000
		}
	]`

	entries, err := chromeMeta.ParseBrowserHistoryJSON(strings.NewReader(historyJSON))
	if err != nil {
		t.Fatalf("ParseBrowserHistoryJSON failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].Title != "Example Page" {
		t.Errorf("Entry[0] Title = %q", entries[0].Title)
	}
	if entries[0].URL != "https://example.com/page1" {
		t.Errorf("Entry[0] URL = %q", entries[0].URL)
	}
	if entries[0].PageTransition != "LINK" {
		t.Errorf("Entry[0] PageTransition = %q", entries[0].PageTransition)
	}
	if entries[0].LastVisited.IsZero() {
		t.Error("Entry[0] LastVisited is zero")
	}
}

func TestChrome_ParseSearchEnginesJSON_Valid(t *testing.T) {
	enginesJSON := `[
		{
			"name": "Google",
			"keyword": "google.com",
			"url": "https://www.google.com/search?q={searchTerms}",
			"is_active": true,
			"prepopulate_id": 1
		},
		{
			"name": "DuckDuckGo",
			"keyword": "ddg",
			"url": "https://duckduckgo.com/?q={searchTerms}",
			"is_active": false
		}
	]`

	engines, err := chromeMeta.ParseSearchEnginesJSON(strings.NewReader(enginesJSON))
	if err != nil {
		t.Fatalf("ParseSearchEnginesJSON failed: %v", err)
	}
	if len(engines) != 2 {
		t.Fatalf("Expected 2 engines, got %d", len(engines))
	}
	if engines[0].Name != "Google" {
		t.Errorf("Engine[0] Name = %q", engines[0].Name)
	}
	if !engines[0].IsDefault {
		t.Error("Google should be default (is_active=true)")
	}
	if engines[1].IsDefault {
		t.Error("DuckDuckGo should not be default")
	}
}

func TestChrome_ParseAutofillJSON_Valid(t *testing.T) {
	autofillJSON := `[
		{
			"name": "email",
			"value": "user@example.com",
			"date_created": "2024-01-01",
			"count": 5
		},
		{
			"name": "phone",
			"value": "+1-555-0100",
			"date_created": "2024-06-15"
		}
	]`

	autofills, err := chromeMeta.ParseAutofillJSON(strings.NewReader(autofillJSON))
	if err != nil {
		t.Fatalf("ParseAutofillJSON failed: %v", err)
	}
	if len(autofills) != 2 {
		t.Fatalf("Expected 2 autofills, got %d", len(autofills))
	}
	if autofills[0].Name != "email" {
		t.Errorf("Autofill[0] Name = %q", autofills[0].Name)
	}
	if autofills[0].Value != "user@example.com" {
		t.Errorf("Autofill[0] Value = %q", autofills[0].Value)
	}
}

// ============================================================================
// FIT TESTS
// ============================================================================

func TestFit_ParseDailyAggregationCSV_Valid(t *testing.T) {
	csvData := "Date,Move Minutes count,Calories (kcal),Distance (m),Step count,Heart Points\n" +
		"2025-01-01,45,350.5,5000,8000,25\n" +
		"2025-01-02,30,250.0,3000,5500,15\n" +
		"2025-01-03,60,500.0,8000,12000,40\n"

	aggs, err := fitMeta.ParseDailyAggregationCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("ParseDailyAggregationCSV failed: %v", err)
	}
	if len(aggs) != 3 {
		t.Fatalf("Expected 3 aggregations, got %d", len(aggs))
	}

	a1 := aggs[0]
	if a1.Date.Format("2006-01-02") != "2025-01-01" {
		t.Errorf("Date = %v", a1.Date)
	}
	if a1.MoveMinutes != 45 {
		t.Errorf("MoveMinutes = %d, want 45", a1.MoveMinutes)
	}
	if a1.CaloriesExpended < 350.4 || a1.CaloriesExpended > 350.6 {
		t.Errorf("CaloriesExpended = %f, want ~350.5", a1.CaloriesExpended)
	}
	if a1.Steps != 8000 {
		t.Errorf("Steps = %d, want 8000", a1.Steps)
	}
	if a1.HeartPoints != 25 {
		t.Errorf("HeartPoints = %d, want 25", a1.HeartPoints)
	}
}

func TestFit_ParseDailyAggregationCSV_MissingColumns(t *testing.T) {
	// CSV with only Date and Steps columns
	csvData := "Date,Step count\n" +
		"2025-01-01,5000\n"

	aggs, err := fitMeta.ParseDailyAggregationCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("ParseDailyAggregationCSV failed with partial columns: %v", err)
	}
	if len(aggs) != 1 {
		t.Fatalf("Expected 1 aggregation, got %d", len(aggs))
	}
	if aggs[0].Steps != 5000 {
		t.Errorf("Steps = %d, want 5000", aggs[0].Steps)
	}
	// Missing columns should default to 0
	if aggs[0].CaloriesExpended != 0 {
		t.Errorf("CaloriesExpended should be 0 when column missing, got %f", aggs[0].CaloriesExpended)
	}
}

func TestFit_ParseDailyAggregationCSV_NoDateColumn(t *testing.T) {
	csvData := "Step count,Calories (kcal)\n5000,350\n"
	_, err := fitMeta.ParseDailyAggregationCSV(strings.NewReader(csvData))
	if err == nil {
		t.Error("Expected error when Date column is missing")
	}
}

func TestFit_ParseDailyAggregationCSV_EmptyCSV(t *testing.T) {
	_, err := fitMeta.ParseDailyAggregationCSV(strings.NewReader(""))
	if err == nil {
		t.Error("Expected error on empty CSV")
	}
}

func TestFit_ParseActivityJSON_Valid(t *testing.T) {
	activityJSON := `{
		"startTime": "2025-01-15T06:00:00Z",
		"endTime": "2025-01-15T07:30:00Z",
		"activity": "Running",
		"fitnessActivity": "running"
	}`

	session, err := fitMeta.ParseActivityJSON(strings.NewReader(activityJSON))
	if err != nil {
		t.Fatalf("ParseActivityJSON failed: %v", err)
	}
	if session.Activity != "Running" {
		t.Errorf("Activity = %q", session.Activity)
	}
	if session.FitnessActivity != "running" {
		t.Errorf("FitnessActivity = %q", session.FitnessActivity)
	}
	if session.StartTime.IsZero() {
		t.Error("StartTime is zero")
	}
	if session.EndTime.IsZero() {
		t.Error("EndTime is zero")
	}
	duration := session.EndTime.Sub(session.StartTime)
	if duration.Minutes() < 89 || duration.Minutes() > 91 {
		t.Errorf("Duration = %v, expected ~90 minutes", duration)
	}
}

func TestFit_ParseActivityJSON_InvalidJSON(t *testing.T) {
	_, err := fitMeta.ParseActivityJSON(strings.NewReader("not json"))
	if err == nil {
		t.Error("Expected error on invalid JSON")
	}
}

func TestFit_ParseActivityJSON_InvalidTimestamp(t *testing.T) {
	actJSON := `{"startTime": "not-a-time", "endTime": "2025-01-15T07:30:00Z"}`
	_, err := fitMeta.ParseActivityJSON(strings.NewReader(actJSON))
	if err == nil {
		t.Error("Expected error on invalid startTime")
	}
}

// ============================================================================
// PASSWORDS TESTS
// ============================================================================

func TestPasswords_ParsePasswordsCSV_Valid(t *testing.T) {
	csvData := "name,url,username,password,note\n" +
		"Example,https://example.com,user1,pass123,My login\n" +
		"GitHub,https://github.com,devuser,ghtoken,\n" +
		"NoNote,https://nonote.com,admin,admin123,\n"

	entries, err := passwordsMeta.ParsePasswordsCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("ParsePasswordsCSV failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	e1 := entries[0]
	if e1.Name != "Example" {
		t.Errorf("Name = %q, want 'Example'", e1.Name)
	}
	if e1.URL != "https://example.com" {
		t.Errorf("URL = %q", e1.URL)
	}
	if e1.Username != "user1" {
		t.Errorf("Username = %q", e1.Username)
	}
	if e1.Password != "pass123" {
		t.Errorf("Password = %q", e1.Password)
	}
	if e1.Note != "My login" {
		t.Errorf("Note = %q", e1.Note)
	}
}

func TestPasswords_ParsePasswordsCSV_WithoutNameColumn(t *testing.T) {
	csvData := "url,username,password,note\n" +
		"https://example.com,user1,pass123,note1\n"

	entries, err := passwordsMeta.ParsePasswordsCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("ParsePasswordsCSV failed without name column: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}
	// Name should be auto-extracted from URL
	if entries[0].Name != "example.com" {
		t.Errorf("Auto-extracted Name = %q, want 'example.com'", entries[0].Name)
	}
}

func TestPasswords_ParsePasswordsCSV_MissingRequiredColumn(t *testing.T) {
	csvData := "name,url,note\nTest,https://test.com,note\n"
	_, err := passwordsMeta.ParsePasswordsCSV(strings.NewReader(csvData))
	if err == nil {
		t.Error("Expected error when required columns (username, password) are missing")
	}
}

func TestPasswords_ParsePasswordsCSV_EmptyCSV(t *testing.T) {
	entries, err := passwordsMeta.ParsePasswordsCSV(strings.NewReader(""))
	if err != nil {
		// Empty CSV returns EOF on first read, which should return nil, nil
		t.Logf("Empty CSV returned error: %v (acceptable)", err)
	}
	if entries != nil && len(entries) > 0 {
		t.Errorf("Expected nil or empty entries on empty CSV, got %d", len(entries))
	}
}

func TestPasswords_CalculateStats(t *testing.T) {
	lib := &passwordsMeta.PasswordLibrary{
		Entries: []passwordsMeta.PasswordEntry{
			{Name: "Site1", URL: "https://example.com", Username: "user1", Password: "pass1", Note: "note1"},
			{Name: "Site2", URL: "https://example.com", Username: "user2", Password: "pass2"},
			{Name: "Site3", URL: "https://other.com", Username: "user3", Password: "pass3"},
			{Name: "NoURL", URL: "", Username: "", Password: "pass4"},
		},
	}
	lib.CalculateStats()

	if lib.Stats.TotalEntries != 4 {
		t.Errorf("TotalEntries = %d, want 4", lib.Stats.TotalEntries)
	}
	if lib.Stats.EntriesWithNotes != 1 {
		t.Errorf("EntriesWithNotes = %d, want 1", lib.Stats.EntriesWithNotes)
	}
	if lib.Stats.EntriesNoURL != 1 {
		t.Errorf("EntriesNoURL = %d, want 1", lib.Stats.EntriesNoURL)
	}
	if lib.Stats.EntriesNoUsername != 1 {
		t.Errorf("EntriesNoUsername = %d, want 1", lib.Stats.EntriesNoUsername)
	}
	if lib.Stats.UniqueDomains != 2 {
		t.Errorf("UniqueDomains = %d, want 2", lib.Stats.UniqueDomains)
	}
}

func TestPasswords_JSONSerialization(t *testing.T) {
	entry := passwordsMeta.PasswordEntry{
		Name:     "TestSite",
		URL:      "https://test.com",
		Username: "testuser",
		Password: "testpass",
		Note:     "test note",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded passwordsMeta.PasswordEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Name != entry.Name {
		t.Errorf("Roundtrip Name = %q, want %q", decoded.Name, entry.Name)
	}
	if decoded.URL != entry.URL {
		t.Errorf("Roundtrip URL = %q", decoded.URL)
	}
	if decoded.Username != entry.Username {
		t.Errorf("Roundtrip Username = %q", decoded.Username)
	}
}

// ============================================================================
// CROSS-SERVICE EDGE CASE TESTS
// ============================================================================

func TestEdge_AllParsers_NoNilPanic(t *testing.T) {
	// Ensure no parser panics on various bad inputs
	t.Run("Gmail-MalformedHeader", func(t *testing.T) {
		_, _ = gmailMeta.ParseMboxHeader("garbage with no headers")
	})

	t.Run("Contacts-MalformedVCard", func(t *testing.T) {
		_, _ = contactsMeta.ParseVCard(strings.NewReader("BEGIN:VCARD\nINVALID\nEND:VCARD"))
	})

	t.Run("Calendar-MalformedICS", func(t *testing.T) {
		_, _ = calendarMeta.ParseICS(strings.NewReader("BEGIN:VCALENDAR\nGARBAGE\nEND:VCALENDAR"))
	})

	t.Run("Tasks-MalformedJSON", func(t *testing.T) {
		_, _ = tasksMeta.ParseTasksJSON(strings.NewReader(`{"items": "not array"}`))
	})

	t.Run("Maps-MalformedJSON", func(t *testing.T) {
		_, _ = mapsMeta.ParseRecordsJSON(strings.NewReader(`{"not": "locations"}`))
	})

	t.Run("Maps-MalformedKML", func(t *testing.T) {
		_, _ = mapsMeta.ParseSavedPlacesKML(strings.NewReader("<not-kml/>"))
	})

	t.Run("Chrome-MalformedHTML", func(t *testing.T) {
		_, _ = chromeMeta.ParseBookmarksHTML(strings.NewReader("<html><body>no bookmarks</body></html>"))
	})

	t.Run("Chrome-MalformedHistoryJSON", func(t *testing.T) {
		_, _ = chromeMeta.ParseBrowserHistoryJSON(strings.NewReader("not json"))
	})

	t.Run("Fit-MalformedCSV", func(t *testing.T) {
		_, _ = fitMeta.ParseDailyAggregationCSV(strings.NewReader("Date\n\"bad,row"))
	})

	t.Run("Passwords-MalformedCSV", func(t *testing.T) {
		_, _ = passwordsMeta.ParsePasswordsCSV(strings.NewReader("url,username,password\n\"bad,row"))
	})
}

func TestEdge_UnicodeContent(t *testing.T) {
	t.Run("Gmail-UnicodeSubject", func(t *testing.T) {
		header := "From: test@example.com\r\nSubject: 日本語テスト 🎉\r\nDate: Mon, 02 Jan 2006 15:04:05 -0700\r\n\r\nBody\r\n"
		msg, err := gmailMeta.ParseMboxHeader(header)
		if err != nil {
			t.Fatalf("Unicode subject failed: %v", err)
		}
		if msg.Subject != "日本語テスト 🎉" {
			t.Errorf("Subject = %q", msg.Subject)
		}
	})

	t.Run("Contacts-UnicodeName", func(t *testing.T) {
		vcard := "BEGIN:VCARD\nVERSION:3.0\nFN:田中太郎\nEND:VCARD"
		contacts, err := contactsMeta.ParseVCard(strings.NewReader(vcard))
		if err != nil {
			t.Fatalf("Unicode name failed: %v", err)
		}
		if len(contacts) != 1 || contacts[0].FullName != "田中太郎" {
			t.Errorf("FullName = %q", contacts[0].FullName)
		}
	})

	t.Run("Calendar-UnicodeSummary", func(t *testing.T) {
		ics := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:unicode@test\nSUMMARY:Réunion d'équipe\nDTSTART:20250115T100000Z\nEND:VEVENT\nEND:VCALENDAR"
		calendars, err := calendarMeta.ParseICS(strings.NewReader(ics))
		if err != nil {
			t.Fatalf("Unicode summary failed: %v", err)
		}
		if len(calendars) != 1 || len(calendars[0].Events) != 1 {
			t.Fatal("Expected 1 calendar with 1 event")
		}
		if calendars[0].Events[0].Summary != "Réunion d'équipe" {
			t.Errorf("Summary = %q", calendars[0].Events[0].Summary)
		}
	})
}

func TestEdge_LargeInputs(t *testing.T) {
	t.Run("Contacts-ManyContacts", func(t *testing.T) {
		var sb strings.Builder
		for i := 0; i < 100; i++ {
			sb.WriteString("BEGIN:VCARD\n")
			sb.WriteString("VERSION:3.0\n")
			sb.WriteString("FN:Contact " + strings.Repeat("X", 10) + "\n")
			sb.WriteString("EMAIL:contact" + strings.Repeat("x", 5) + "@example.com\n")
			sb.WriteString("END:VCARD\n")
		}
		contacts, err := contactsMeta.ParseVCard(strings.NewReader(sb.String()))
		if err != nil {
			t.Fatalf("100 contacts failed: %v", err)
		}
		if len(contacts) != 100 {
			t.Errorf("Expected 100 contacts, got %d", len(contacts))
		}
	})

	t.Run("Maps-ManyLocations", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString(`{"locations": [`)
		for i := 0; i < 200; i++ {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(`{"latitudeE7": 408900000, "longitudeE7": -740600000, "accuracy": 20, "timestamp": "2025-01-15T10:30:00Z"}`)
		}
		sb.WriteString(`]}`)
		records, err := mapsMeta.ParseRecordsJSON(strings.NewReader(sb.String()))
		if err != nil {
			t.Fatalf("200 locations failed: %v", err)
		}
		if len(records) != 200 {
			t.Errorf("Expected 200 records, got %d", len(records))
		}
	})
}
