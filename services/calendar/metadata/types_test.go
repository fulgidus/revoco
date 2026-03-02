package metadata

import (
	"strings"
	"testing"
	"time"
)

func TestParseICS_SimpleEvent(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Google Inc//Google Calendar 70.9054//EN
X-WR-CALNAME:Work Calendar
X-WR-TIMEZONE:America/New_York
BEGIN:VEVENT
UID:event123@google.com
DTSTART:20240315T140000Z
DTEND:20240315T150000Z
SUMMARY:Team Meeting
DESCRIPTION:Weekly team sync
LOCATION:Conference Room A
STATUS:CONFIRMED
ORGANIZER:mailto:boss@example.com
ATTENDEE:mailto:alice@example.com
ATTENDEE:mailto:bob@example.com
CREATED:20240301T120000Z
LAST-MODIFIED:20240301T130000Z
END:VEVENT
END:VCALENDAR`

	calendars, err := ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}

	if len(calendars) != 1 {
		t.Fatalf("expected 1 calendar, got %d", len(calendars))
	}

	cal := calendars[0]
	if cal.Name != "Work Calendar" {
		t.Errorf("expected calendar name 'Work Calendar', got '%s'", cal.Name)
	}
	if cal.Timezone != "America/New_York" {
		t.Errorf("expected timezone 'America/New_York', got '%s'", cal.Timezone)
	}

	if len(cal.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(cal.Events))
	}

	event := cal.Events[0]
	if event.UID != "event123@google.com" {
		t.Errorf("expected UID 'event123@google.com', got '%s'", event.UID)
	}
	if event.Summary != "Team Meeting" {
		t.Errorf("expected summary 'Team Meeting', got '%s'", event.Summary)
	}
	if event.Description != "Weekly team sync" {
		t.Errorf("expected description 'Weekly team sync', got '%s'", event.Description)
	}
	if event.Location != "Conference Room A" {
		t.Errorf("expected location 'Conference Room A', got '%s'", event.Location)
	}
	if event.Status != "CONFIRMED" {
		t.Errorf("expected status 'CONFIRMED', got '%s'", event.Status)
	}
	if event.Organizer != "boss@example.com" {
		t.Errorf("expected organizer 'boss@example.com', got '%s'", event.Organizer)
	}
	if len(event.Attendees) != 2 {
		t.Errorf("expected 2 attendees, got %d", len(event.Attendees))
	}

	expectedStart := time.Date(2024, 3, 15, 14, 0, 0, 0, time.UTC)
	if !event.StartTime.Equal(expectedStart) {
		t.Errorf("expected start time %v, got %v", expectedStart, event.StartTime)
	}

	expectedEnd := time.Date(2024, 3, 15, 15, 0, 0, 0, time.UTC)
	if !event.EndTime.Equal(expectedEnd) {
		t.Errorf("expected end time %v, got %v", expectedEnd, event.EndTime)
	}
}

func TestParseICS_RecurringEvent(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:recurring123@google.com
DTSTART:20240301T090000Z
DTEND:20240301T100000Z
SUMMARY:Daily Standup
RRULE:FREQ=DAILY;COUNT=30
END:VEVENT
END:VCALENDAR`

	calendars, err := ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}

	if len(calendars) != 1 || len(calendars[0].Events) != 1 {
		t.Fatalf("expected 1 calendar with 1 event")
	}

	event := calendars[0].Events[0]
	if event.Recurrence != "FREQ=DAILY;COUNT=30" {
		t.Errorf("expected recurrence 'FREQ=DAILY;COUNT=30', got '%s'", event.Recurrence)
	}
	if event.Summary != "Daily Standup" {
		t.Errorf("expected summary 'Daily Standup', got '%s'", event.Summary)
	}
}

func TestParseICS_MultipleConcatenatedEvents(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:event1@google.com
DTSTART:20240315T140000Z
SUMMARY:Event One
END:VEVENT
BEGIN:VEVENT
UID:event2@google.com
DTSTART:20240316T140000Z
SUMMARY:Event Two
END:VEVENT
BEGIN:VEVENT
UID:event3@google.com
DTSTART:20240317T140000Z
SUMMARY:Event Three
END:VEVENT
END:VCALENDAR`

	calendars, err := ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}

	if len(calendars) != 1 {
		t.Fatalf("expected 1 calendar, got %d", len(calendars))
	}

	if len(calendars[0].Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(calendars[0].Events))
	}

	expectedSummaries := []string{"Event One", "Event Two", "Event Three"}
	for i, event := range calendars[0].Events {
		if event.Summary != expectedSummaries[i] {
			t.Errorf("event %d: expected summary '%s', got '%s'", i, expectedSummaries[i], event.Summary)
		}
	}
}

func TestParseICS_EscapedText(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:escaped@google.com
DTSTART:20240315T140000Z
SUMMARY:Meeting with Alice\, Bob\; and Carol
DESCRIPTION:Line one\nLine two\nLine three
LOCATION:Building A\, Floor 2\; Room 301
END:VEVENT
END:VCALENDAR`

	calendars, err := ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}

	event := calendars[0].Events[0]
	if event.Summary != "Meeting with Alice, Bob; and Carol" {
		t.Errorf("escaped summary failed: got '%s'", event.Summary)
	}
	if !strings.Contains(event.Description, "Line one\nLine two") {
		t.Errorf("escaped description failed: got '%s'", event.Description)
	}
	if event.Location != "Building A, Floor 2; Room 301" {
		t.Errorf("escaped location failed: got '%s'", event.Location)
	}
}

func TestParseICS_UnfoldedLines(t *testing.T) {
	// RFC 5545: Long lines are folded with CRLF + space/tab
	ics := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:unfold@google.com\r\n" +
		"DTSTART:20240315T140000Z\r\n" +
		"SUMMARY:This is a very long summary that has been folded across multipl\r\n" +
		" e lines according to RFC 5545 rules\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	calendars, err := ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}

	event := calendars[0].Events[0]
	expectedSummary := "This is a very long summary that has been folded across multiple lines according to RFC 5545 rules"
	if event.Summary != expectedSummary {
		t.Errorf("unfolded summary failed:\nexpected: %s\ngot: %s", expectedSummary, event.Summary)
	}
}

func TestParseICS_EmptyCalendar(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
END:VCALENDAR`

	calendars, err := ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}

	if len(calendars) != 1 {
		t.Fatalf("expected 1 calendar, got %d", len(calendars))
	}

	if len(calendars[0].Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(calendars[0].Events))
	}
}

func TestParseICS_MalformedInput(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "empty input",
			input: "",
		},
		{
			name:  "only whitespace",
			input: "   \n\n  \n",
		},
		{
			name:  "incomplete event",
			input: "BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:test\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			calendars, err := ParseICS(strings.NewReader(tc.input))
			// Should not crash - may return empty or partial result
			if err != nil {
				t.Logf("ParseICS returned error (acceptable): %v", err)
			}
			t.Logf("ParseICS returned %d calendars", len(calendars))
		})
	}
}

func TestParseICS_DateOnlyFormat(t *testing.T) {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:allday@google.com
DTSTART:20240315
SUMMARY:All Day Event
END:VEVENT
END:VCALENDAR`

	calendars, err := ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("ParseICS failed: %v", err)
	}

	event := calendars[0].Events[0]
	expectedDate := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !event.StartTime.Equal(expectedDate) {
		t.Errorf("expected date-only start %v, got %v", expectedDate, event.StartTime)
	}
}

func TestBuildICS(t *testing.T) {
	cal := &Calendar{
		Name:        "Test Calendar",
		Description: "A test calendar",
		Timezone:    "America/New_York",
		Events: []CalendarEvent{
			{
				UID:         "test123@revoco",
				Summary:     "Test Event",
				Description: "Test Description",
				Location:    "Test Location",
				StartTime:   time.Date(2024, 3, 15, 14, 0, 0, 0, time.UTC),
				EndTime:     time.Date(2024, 3, 15, 15, 0, 0, 0, time.UTC),
				Status:      "CONFIRMED",
				Organizer:   "organizer@example.com",
				Attendees:   []string{"alice@example.com", "bob@example.com"},
				Created:     time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
				Modified:    time.Date(2024, 3, 1, 13, 0, 0, 0, time.UTC),
			},
		},
	}

	ics := BuildICS(cal)

	// Verify structure
	if !strings.Contains(ics, "BEGIN:VCALENDAR") {
		t.Error("missing BEGIN:VCALENDAR")
	}
	if !strings.Contains(ics, "END:VCALENDAR") {
		t.Error("missing END:VCALENDAR")
	}
	if !strings.Contains(ics, "BEGIN:VEVENT") {
		t.Error("missing BEGIN:VEVENT")
	}
	if !strings.Contains(ics, "END:VEVENT") {
		t.Error("missing END:VEVENT")
	}
	if !strings.Contains(ics, "UID:test123@revoco") {
		t.Error("missing UID")
	}
	if !strings.Contains(ics, "SUMMARY:Test Event") {
		t.Error("missing SUMMARY")
	}
	if !strings.Contains(ics, "X-WR-CALNAME:Test Calendar") {
		t.Error("missing X-WR-CALNAME")
	}

	// Parse it back to verify round-trip
	calendars, err := ParseICS(strings.NewReader(ics))
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}

	if len(calendars) != 1 || len(calendars[0].Events) != 1 {
		t.Fatal("round-trip failed: wrong structure")
	}

	rtEvent := calendars[0].Events[0]
	if rtEvent.UID != "test123@revoco" {
		t.Errorf("round-trip UID mismatch: got '%s'", rtEvent.UID)
	}
	if rtEvent.Summary != "Test Event" {
		t.Errorf("round-trip Summary mismatch: got '%s'", rtEvent.Summary)
	}
}

func TestEscapeText(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"simple text", "simple text"},
		{"text, with, commas", "text\\, with\\, commas"},
		{"text; with; semicolons", "text\\; with\\; semicolons"},
		{"text\\with\\backslashes", "text\\\\with\\\\backslashes"},
		{"line1\nline2\nline3", "line1\\nline2\\nline3"},
		{"mixed, text; with\neverything", "mixed\\, text\\; with\\neverything"},
	}

	for _, tc := range testCases {
		result := EscapeText(tc.input)
		if result != tc.expected {
			t.Errorf("EscapeText(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestUnescapeText(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"simple text", "simple text"},
		{"text\\, with\\, commas", "text, with, commas"},
		{"text\\; with\\; semicolons", "text; with; semicolons"},
		{"text\\\\with\\\\backslashes", "text\\with\\backslashes"},
		{"line1\\nline2\\nline3", "line1\nline2\nline3"},
		{"line1\\Nline2", "line1\nline2"},
	}

	for _, tc := range testCases {
		result := unescapeText(tc.input)
		if result != tc.expected {
			t.Errorf("unescapeText(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestFoldLine(t *testing.T) {
	// Short line should not be folded
	short := "SUMMARY:Short"
	if FoldLine(short) != short {
		t.Errorf("short line was incorrectly folded")
	}

	// Long line should be folded at 75 characters
	long := strings.Repeat("A", 150)
	folded := FoldLine(long)
	lines := strings.Split(folded, "\r\n")
	if len(lines) < 2 {
		t.Errorf("long line was not folded")
	}
	// First line should be 75 chars
	if len(lines[0]) != 75 {
		t.Errorf("first line length = %d, expected 75", len(lines[0]))
	}
	// Continuation lines should start with space
	for i := 1; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], " ") {
			t.Errorf("continuation line %d does not start with space", i)
		}
	}
}
