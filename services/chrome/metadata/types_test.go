package metadata

import (
	"strings"
	"testing"
	"time"
)

// ── Bookmark Parser Tests ────────────────────────────────────────────────────

func TestParseBookmarksHTML_Simple(t *testing.T) {
	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><A HREF="https://example.com" ADD_DATE="1234567890">Example Site</A>
    <DT><A HREF="https://golang.org" ADD_DATE="1609459200">Go Programming</A>
</DL><p>`

	bookmarks, err := ParseBookmarksHTML(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseBookmarksHTML failed: %v", err)
	}

	if len(bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(bookmarks))
	}

	// Check first bookmark
	bm := bookmarks[0]
	if bm.Name != "Example Site" {
		t.Errorf("expected name 'Example Site', got %q", bm.Name)
	}
	if bm.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %q", bm.URL)
	}
	expectedTime := time.Unix(1234567890, 0)
	if !bm.DateAdded.Equal(expectedTime) {
		t.Errorf("expected date %v, got %v", expectedTime, bm.DateAdded)
	}
	if bm.Folder != "" {
		t.Errorf("expected empty folder, got %q", bm.Folder)
	}
}

func TestParseBookmarksHTML_NestedFolders(t *testing.T) {
	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
    <DT><H3 ADD_DATE="1234567890">Bookmarks Bar</H3>
    <DL><p>
        <DT><H3 ADD_DATE="1234567891">Tech</H3>
        <DL><p>
            <DT><H3 ADD_DATE="1234567892">Go</H3>
            <DL><p>
                <DT><A HREF="https://golang.org" ADD_DATE="1609459200">Go Homepage</A>
            </DL><p>
            <DT><A HREF="https://github.com" ADD_DATE="1609459201">GitHub</A>
        </DL><p>
        <DT><A HREF="https://news.ycombinator.com" ADD_DATE="1609459202">Hacker News</A>
    </DL><p>
</DL><p>`

	bookmarks, err := ParseBookmarksHTML(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseBookmarksHTML failed: %v", err)
	}

	if len(bookmarks) != 3 {
		t.Fatalf("expected 3 bookmarks, got %d", len(bookmarks))
	}

	// Check Go Homepage (deepest nesting)
	bm := bookmarks[0]
	if bm.Name != "Go Homepage" {
		t.Errorf("expected name 'Go Homepage', got %q", bm.Name)
	}
	if bm.Folder != "Go" {
		t.Errorf("expected folder 'Go', got %q", bm.Folder)
	}
	expectedFolders := []string{"Bookmarks Bar", "Tech", "Go"}
	if len(bm.ParentFolders) != len(expectedFolders) {
		t.Fatalf("expected %d parent folders, got %d", len(expectedFolders), len(bm.ParentFolders))
	}
	for i, expected := range expectedFolders {
		if bm.ParentFolders[i] != expected {
			t.Errorf("parent folder %d: expected %q, got %q", i, expected, bm.ParentFolders[i])
		}
	}

	// Check GitHub (mid-level)
	bm2 := bookmarks[1]
	if bm2.Name != "GitHub" {
		t.Errorf("expected name 'GitHub', got %q", bm2.Name)
	}
	if bm2.Folder != "Tech" {
		t.Errorf("expected folder 'Tech', got %q", bm2.Folder)
	}
	expectedFolders2 := []string{"Bookmarks Bar", "Tech"}
	if len(bm2.ParentFolders) != len(expectedFolders2) {
		t.Fatalf("expected %d parent folders, got %d", len(expectedFolders2), len(bm2.ParentFolders))
	}

	// Check Hacker News (top-level under Bookmarks Bar)
	bm3 := bookmarks[2]
	if bm3.Name != "Hacker News" {
		t.Errorf("expected name 'Hacker News', got %q", bm3.Name)
	}
	if bm3.Folder != "Bookmarks Bar" {
		t.Errorf("expected folder 'Bookmarks Bar', got %q", bm3.Folder)
	}
}

func TestParseBookmarksHTML_EmptyFile(t *testing.T) {
	html := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
</DL><p>`

	bookmarks, err := ParseBookmarksHTML(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseBookmarksHTML failed: %v", err)
	}

	if len(bookmarks) != 0 {
		t.Errorf("expected 0 bookmarks, got %d", len(bookmarks))
	}
}

func TestParseBookmarksHTML_MissingAddDate(t *testing.T) {
	html := `<DL><p>
    <DT><A HREF="https://example.com">Example Without Date</A>
</DL><p>`

	bookmarks, err := ParseBookmarksHTML(strings.NewReader(html))
	if err != nil {
		t.Fatalf("ParseBookmarksHTML failed: %v", err)
	}

	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}

	bm := bookmarks[0]
	if !bm.DateAdded.IsZero() {
		t.Errorf("expected zero time for missing ADD_DATE, got %v", bm.DateAdded)
	}
}

// ── Browser History Parser Tests ──────────────────────────────────────────────

func TestParseBrowserHistoryJSON_Valid(t *testing.T) {
	json := `[
		{
			"favicon_url": "https://example.com/favicon.ico",
			"page_transition": "LINK",
			"title": "Example Domain",
			"url": "https://example.com",
			"client_id": "abc123",
			"time_usec": 1234567890000000
		},
		{
			"page_transition": "TYPED",
			"title": "Go Programming Language",
			"url": "https://golang.org",
			"time_usec": 1609459200000000
		}
	]`

	entries, err := ParseBrowserHistoryJSON(strings.NewReader(json))
	if err != nil {
		t.Fatalf("ParseBrowserHistoryJSON failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Check first entry
	entry := entries[0]
	if entry.Title != "Example Domain" {
		t.Errorf("expected title 'Example Domain', got %q", entry.Title)
	}
	if entry.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %q", entry.URL)
	}
	if entry.PageTransition != "LINK" {
		t.Errorf("expected transition 'LINK', got %q", entry.PageTransition)
	}

	// Check timestamp conversion (1234567890000000 microseconds = Feb 13, 2009)
	expectedTime := time.Unix(1234567890, 0)
	if !entry.LastVisited.Equal(expectedTime) {
		t.Errorf("expected time %v, got %v", expectedTime, entry.LastVisited)
	}
}

func TestParseBrowserHistoryJSON_EmptyArray(t *testing.T) {
	json := `[]`

	entries, err := ParseBrowserHistoryJSON(strings.NewReader(json))
	if err != nil {
		t.Fatalf("ParseBrowserHistoryJSON failed: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseBrowserHistoryJSON_InvalidJSON(t *testing.T) {
	json := `[{"title": "Incomplete`

	_, err := ParseBrowserHistoryJSON(strings.NewReader(json))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// ── SearchEngine Parser Tests ─────────────────────────────────────────────────

func TestParseSearchEnginesJSON_Valid(t *testing.T) {
	json := `[
		{
			"name": "Google",
			"keyword": "google.com",
			"url": "https://www.google.com/search?q={searchTerms}",
			"is_active": true
		},
		{
			"name": "DuckDuckGo",
			"keyword": "ddg",
			"url": "https://duckduckgo.com/?q={searchTerms}",
			"is_active": false
		}
	]`

	engines, err := ParseSearchEnginesJSON(strings.NewReader(json))
	if err != nil {
		t.Fatalf("ParseSearchEnginesJSON failed: %v", err)
	}

	if len(engines) != 2 {
		t.Fatalf("expected 2 engines, got %d", len(engines))
	}

	// Check Google
	eng := engines[0]
	if eng.Name != "Google" {
		t.Errorf("expected name 'Google', got %q", eng.Name)
	}
	if eng.Keyword != "google.com" {
		t.Errorf("expected keyword 'google.com', got %q", eng.Keyword)
	}
	if !eng.IsDefault {
		t.Error("expected IsDefault=true for active engine")
	}

	// Check DuckDuckGo
	eng2 := engines[1]
	if eng2.IsDefault {
		t.Error("expected IsDefault=false for inactive engine")
	}
}

// ── Autofill Parser Tests ─────────────────────────────────────────────────────

func TestParseAutofillJSON_Valid(t *testing.T) {
	json := `[
		{
			"name": "Email",
			"value": "user@example.com",
			"date_created": "2021-01-01T00:00:00Z"
		},
		{
			"name": "Phone",
			"value": "+1234567890",
			"date_created": "2021-06-15T12:30:00Z"
		}
	]`

	autofills, err := ParseAutofillJSON(strings.NewReader(json))
	if err != nil {
		t.Fatalf("ParseAutofillJSON failed: %v", err)
	}

	if len(autofills) != 2 {
		t.Fatalf("expected 2 autofills, got %d", len(autofills))
	}

	// Check first entry
	af := autofills[0]
	if af.Name != "Email" {
		t.Errorf("expected name 'Email', got %q", af.Name)
	}
	if af.Value != "user@example.com" {
		t.Errorf("expected value 'user@example.com', got %q", af.Value)
	}
}

// ── Timestamp Conversion Tests ────────────────────────────────────────────────

func TestMicrosToTime_Valid(t *testing.T) {
	// Test known timestamp: 1234567890000000 microseconds = Feb 13, 2009 23:31:30 UTC
	micros := int64(1234567890000000)
	result := microsToTime(micros)

	expected := time.Unix(1234567890, 0)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestMicrosToTime_Zero(t *testing.T) {
	result := microsToTime(0)
	if !result.IsZero() {
		t.Errorf("expected zero time for 0 microseconds, got %v", result)
	}
}

func TestMicrosToTime_EdgeCases(t *testing.T) {
	// Test large value (year 2050)
	micros := int64(2524608000000000) // Jan 1, 2050
	result := microsToTime(micros)

	if result.Year() != 2050 {
		t.Errorf("expected year 2050, got %d", result.Year())
	}
}

// ── Metadata Interface Tests ──────────────────────────────────────────────────

