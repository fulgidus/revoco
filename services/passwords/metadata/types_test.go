package metadata

import (
	"strings"
	"testing"

	conncore "github.com/fulgidus/revoco/connectors"
)

func TestParsePasswordsCSV_Valid(t *testing.T) {
	csv := `name,url,username,password,note
Google,https://google.com,user@example.com,testpass123,My Google account
Facebook,https://facebook.com,user@example.com,testpass456,
GitHub,https://github.com,gituser,testpass789,Work account`

	entries, err := ParsePasswordsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParsePasswordsCSV failed: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	// Check first entry
	if entries[0].Name != "Google" {
		t.Errorf("Expected name 'Google', got '%s'", entries[0].Name)
	}
	if entries[0].URL != "https://google.com" {
		t.Errorf("Expected URL 'https://google.com', got '%s'", entries[0].URL)
	}
	if entries[0].Username != "user@example.com" {
		t.Errorf("Expected username 'user@example.com', got '%s'", entries[0].Username)
	}
	if entries[0].Password != "testpass123" {
		t.Errorf("Expected password 'testpass123', got '%s'", entries[0].Password)
	}
	if entries[0].Note != "My Google account" {
		t.Errorf("Expected note 'My Google account', got '%s'", entries[0].Note)
	}

	// Check second entry (no note)
	if entries[1].Note != "" {
		t.Errorf("Expected empty note, got '%s'", entries[1].Note)
	}
}

func TestParsePasswordsCSV_NoNameColumn(t *testing.T) {
	csv := `url,username,password,note
https://google.com,user@example.com,testpass123,My account
https://facebook.com,user@example.com,testpass456,`

	entries, err := ParsePasswordsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParsePasswordsCSV failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	// Name should be extracted from URL
	if entries[0].Name != "google.com" {
		t.Errorf("Expected name 'google.com' (extracted from URL), got '%s'", entries[0].Name)
	}
}

func TestParsePasswordsCSV_Empty(t *testing.T) {
	csv := ``

	entries, err := ParsePasswordsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("Expected no error for empty CSV, got: %v", err)
	}

	if entries != nil {
		t.Errorf("Expected nil entries for empty CSV, got %d entries", len(entries))
	}
}

func TestParsePasswordsCSV_MissingRequiredColumn(t *testing.T) {
	// Missing "password" column
	csv := `name,url,username,note
Google,https://google.com,user@example.com,My account`

	_, err := ParsePasswordsCSV(strings.NewReader(csv))
	if err == nil {
		t.Fatal("Expected error for missing required column, got nil")
	}

	if !strings.Contains(err.Error(), "missing required column") {
		t.Errorf("Expected 'missing required column' error, got: %v", err)
	}
}

func TestParsePasswordsCSV_EmptyFields(t *testing.T) {
	csv := `name,url,username,password,note
,,testuser,testpass123,
GitHub,https://github.com,,testpass456,`

	entries, err := ParsePasswordsCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ParsePasswordsCSV failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	// First entry has no name and no URL
	if entries[0].Name != "" {
		t.Errorf("Expected empty name, got '%s'", entries[0].Name)
	}
	if entries[0].URL != "" {
		t.Errorf("Expected empty URL, got '%s'", entries[0].URL)
	}

	// Second entry has no username
	if entries[1].Username != "" {
		t.Errorf("Expected empty username, got '%s'", entries[1].Username)
	}
}

func TestExtractDomainFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://www.google.com/path", "google.com"},
		{"https://github.com", "github.com"},
		{"http://example.org/page", "example.org"},
		{"invalid-url", "invalid-url"},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractDomainFromURL(tt.url)
		if result != tt.expected {
			t.Errorf("extractDomainFromURL(%q) = %q, expected %q", tt.url, result, tt.expected)
		}
	}
}

func TestPasswordLibrary_CalculateStats(t *testing.T) {
	lib := &PasswordLibrary{
		Entries: []PasswordEntry{
			{Name: "Google", URL: "https://google.com", Username: "user1", Password: "pass1", Note: "Note 1"},
			{Name: "Gmail", URL: "https://mail.google.com", Username: "user2", Password: "pass2", Note: ""},
			{Name: "Facebook", URL: "https://facebook.com", Username: "user3", Password: "pass3", Note: "Note 2"},
			{Name: "GitHub", URL: "https://github.com", Username: "", Password: "pass4", Note: ""},
			{Name: "Unknown", URL: "", Username: "user5", Password: "pass5", Note: ""},
		},
	}

	lib.CalculateStats()

	if lib.Stats.TotalEntries != 5 {
		t.Errorf("Expected 5 total entries, got %d", lib.Stats.TotalEntries)
	}

	if lib.Stats.UniqueDomains != 4 {
		t.Errorf("Expected 4 unique domains, got %d", lib.Stats.UniqueDomains)
	}

	if lib.Stats.EntriesWithNotes != 2 {
		t.Errorf("Expected 2 entries with notes, got %d", lib.Stats.EntriesWithNotes)
	}

	if lib.Stats.EntriesNoURL != 1 {
		t.Errorf("Expected 1 entry without URL, got %d", lib.Stats.EntriesNoURL)
	}

	if lib.Stats.EntriesNoUsername != 1 {
		t.Errorf("Expected 1 entry without username, got %d", lib.Stats.EntriesNoUsername)
	}

	// Check domain breakdown — subdomains are tracked separately
	if count, ok := lib.Stats.DomainBreakdown["google.com"]; !ok || count != 1 {
		t.Errorf("Expected 1 entry for google.com, got %d", count)
	}
	if count, ok := lib.Stats.DomainBreakdown["mail.google.com"]; !ok || count != 1 {
		t.Errorf("Expected 1 entry for mail.google.com, got %d", count)
	}
}

func TestPasswordLibrary_GetTitle(t *testing.T) {
	lib := &PasswordLibrary{
		Entries: []PasswordEntry{{}, {}, {}},
	}
	lib.CalculateStats()

	title := lib.GetTitle()
	expected := "Google Passwords (3 entries)"
	if title != expected {
		t.Errorf("Expected title '%s', got '%s'", expected, title)
	}
}

func TestPasswordLibrary_GetDataType(t *testing.T) {
	lib := &PasswordLibrary{}
	dataType := lib.GetDataType()

	if dataType != conncore.DataTypePassword {
		t.Errorf("Expected DataTypePassword, got %v", dataType)
	}
}

func TestPasswordLibrary_GetSize(t *testing.T) {
	lib := &PasswordLibrary{
		Entries: []PasswordEntry{{}, {}, {}, {}},
	}

	size := lib.GetSize()
	if size != 4 {
		t.Errorf("Expected size 4, got %d", size)
	}
}

func TestPasswordLibrary_GetMimeType(t *testing.T) {
	lib := &PasswordLibrary{}
	mimeType := lib.GetMimeType()

	expected := "text/csv"
	if mimeType != expected {
		t.Errorf("Expected MIME type '%s', got '%s'", expected, mimeType)
	}
}

func TestPasswordLibrary_GetTags(t *testing.T) {
	lib := &PasswordLibrary{}
	tags := lib.GetTags()

	expectedTags := []string{"passwords", "credentials", "security"}
	if len(tags) != len(expectedTags) {
		t.Fatalf("Expected %d tags, got %d", len(expectedTags), len(tags))
	}

	for i, tag := range tags {
		if tag != expectedTags[i] {
			t.Errorf("Expected tag '%s' at index %d, got '%s'", expectedTags[i], i, tag)
		}
	}
}

func TestPasswordLibrary_GetDescription(t *testing.T) {
	lib := &PasswordLibrary{
		Entries: []PasswordEntry{
			{URL: "https://google.com"},
			{URL: "https://facebook.com"},
			{URL: "https://google.com"},
		},
	}
	lib.CalculateStats()

	desc := lib.GetDescription()
	if !strings.Contains(desc, "3 total entries") {
		t.Errorf("Description should contain '3 total entries', got: %s", desc)
	}
	if !strings.Contains(desc, "2 unique domains") {
		t.Errorf("Description should contain '2 unique domains', got: %s", desc)
	}
}

func TestPasswordEntry_GetTitle(t *testing.T) {
	entry := PasswordEntry{
		Name: "Google Account",
		URL:  "https://google.com",
	}

	title := entry.GetTitle()
	if title != "Google Account" {
		t.Errorf("Expected title 'Google Account', got '%s'", title)
	}

	// Test with empty name
	entry2 := PasswordEntry{
		Name: "",
		URL:  "https://github.com",
	}

	title2 := entry2.GetTitle()
	if title2 != "github.com" {
		t.Errorf("Expected title 'github.com' (from URL), got '%s'", title2)
	}
}

func TestPasswordEntry_GetTags(t *testing.T) {
	entry := PasswordEntry{
		URL: "https://github.com",
	}

	tags := entry.GetTags()
	if len(tags) < 2 {
		t.Fatalf("Expected at least 2 tags, got %d", len(tags))
	}

	// Should contain "password", "credential", and "github.com"
	hasPassword := false
	hasGitHub := false
	for _, tag := range tags {
		if tag == "password" {
			hasPassword = true
		}
		if tag == "github.com" {
			hasGitHub = true
		}
	}

	if !hasPassword {
		t.Error("Tags should contain 'password'")
	}
	if !hasGitHub {
		t.Error("Tags should contain 'github.com'")
	}
}
