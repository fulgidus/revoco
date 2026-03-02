// Package metadata provides Chrome Takeout parsing for bookmarks and browser history.
package metadata

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	conncore "github.com/fulgidus/revoco/connectors"
)

// Bookmark represents a Chrome bookmark with folder hierarchy.
type Bookmark struct {
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	DateAdded     time.Time `json:"date_added"`
	Folder        string    `json:"folder"`         // Immediate parent folder
	ParentFolders []string  `json:"parent_folders"` // Full folder path (e.g., ["Bookmarks Bar", "Tech", "Go"])
}

// HistoryEntry represents a single browser history entry.
type HistoryEntry struct {
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	VisitCount     int       `json:"visit_count"`
	LastVisited    time.Time `json:"last_visited"`
	TypedCount     int       `json:"typed_count"`
	PageTransition string    `json:"page_transition"`
}

// SearchEngine represents a Chrome search engine.
type SearchEngine struct {
	Name      string `json:"name"`
	Keyword   string `json:"keyword"`
	URL       string `json:"url"`
	IsDefault bool   `json:"is_default"`
}

// Autofill represents a Chrome autofill entry.
type Autofill struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	DateCreated string `json:"date_created"`
}

// ChromeLibrary represents all parsed Chrome data.
type ChromeLibrary struct {
	Bookmarks     []Bookmark     `json:"bookmarks"`
	History       []HistoryEntry `json:"history"`
	SearchEngines []SearchEngine `json:"search_engines,omitempty"`
	Autofills     []Autofill     `json:"autofills,omitempty"`
	BookmarksPath string         `json:"bookmarks_path"`
	HistoryPath   string         `json:"history_path"`
	Stats         map[string]int `json:"stats"`
	FolderCounts  map[string]int `json:"folder_counts"` // Count of bookmarks per folder
}

// browserHistoryJSON represents the raw JSON structure from BrowserHistory.json.
type browserHistoryJSON []struct {
	FaviconURL     string `json:"favicon_url"`
	PageTransition string `json:"page_transition"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	ClientID       string `json:"client_id"`
	TimeUsec       int64  `json:"time_usec"` // Microseconds since Unix epoch
}

// searchEngineJSON represents the raw JSON structure from SearchEngines.json.
type searchEngineJSON []struct {
	Name               string `json:"name"`
	Keyword            string `json:"keyword"`
	URL                string `json:"url"`
	IsActive           bool   `json:"is_active"`
	DateCreated        string `json:"date_created,omitempty"`
	LastModified       string `json:"last_modified,omitempty"`
	PrepopulateID      int    `json:"prepopulate_id,omitempty"`
	SafeForAutoreplace bool   `json:"safe_for_autoreplace,omitempty"`
}

// autofillJSON represents the raw JSON structure from Autofill.json.
type autofillJSON []struct {
	Name         string `json:"name"`
	Value        string `json:"value"`
	DateCreated  string `json:"date_created"`
	DateModified string `json:"date_modified,omitempty"`
	Count        int    `json:"count,omitempty"`
}

// ParseBookmarksHTML parses Netscape Bookmark HTML format and returns bookmarks with folder hierarchy.
func ParseBookmarksHTML(r io.Reader) ([]Bookmark, error) {
	var bookmarks []Bookmark
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line

	// Regex patterns
	bookmarkPattern := regexp.MustCompile(`<DT><A\s+HREF="([^"]+)"(?:\s+ADD_DATE="(\d+)")?[^>]*>([^<]+)</A>`)
	folderStartPattern := regexp.MustCompile(`<DT><H3(?:\s+ADD_DATE="(\d+)")?(?:\s+LAST_MODIFIED="(\d+)")?[^>]*>([^<]+)</H3>`)
	folderEndPattern := regexp.MustCompile(`</DL><p>`)

	var folderStack []string // Track current folder hierarchy

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check for bookmark
		if matches := bookmarkPattern.FindStringSubmatch(line); matches != nil {
			url := matches[1]
			dateAddedStr := matches[2]
			name := matches[3]

			var dateAdded time.Time
			if dateAddedStr != "" {
				if unixSec, err := strconv.ParseInt(dateAddedStr, 10, 64); err == nil {
					dateAdded = time.Unix(unixSec, 0)
				}
			}

			folder := ""
			if len(folderStack) > 0 {
				folder = folderStack[len(folderStack)-1]
			}

			bookmarks = append(bookmarks, Bookmark{
				Name:          name,
				URL:           url,
				DateAdded:     dateAdded,
				Folder:        folder,
				ParentFolders: append([]string{}, folderStack...), // Copy stack
			})
			continue
		}

		// Check for folder start
		if matches := folderStartPattern.FindStringSubmatch(line); matches != nil {
			folderName := matches[3]
			folderStack = append(folderStack, folderName)
			continue
		}

		// Check for folder end
		if folderEndPattern.MatchString(line) {
			if len(folderStack) > 0 {
				folderStack = folderStack[:len(folderStack)-1]
			}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan bookmarks: %w", err)
	}

	return bookmarks, nil
}

// ParseBrowserHistoryJSON parses BrowserHistory.json and converts microsecond timestamps.
func ParseBrowserHistoryJSON(r io.Reader) ([]HistoryEntry, error) {
	var raw browserHistoryJSON
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode history JSON: %w", err)
	}

	entries := make([]HistoryEntry, 0, len(raw))
	for _, item := range raw {
		entries = append(entries, HistoryEntry{
			Title:          item.Title,
			URL:            item.URL,
			VisitCount:     1, // Default, actual count not in Takeout format
			LastVisited:    microsToTime(item.TimeUsec),
			TypedCount:     0, // Not available in Takeout
			PageTransition: item.PageTransition,
		})
	}

	return entries, nil
}

// ParseSearchEnginesJSON parses SearchEngines.json.
func ParseSearchEnginesJSON(r io.Reader) ([]SearchEngine, error) {
	var raw searchEngineJSON
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode search engines JSON: %w", err)
	}

	engines := make([]SearchEngine, 0, len(raw))
	for _, item := range raw {
		engines = append(engines, SearchEngine{
			Name:      item.Name,
			Keyword:   item.Keyword,
			URL:       item.URL,
			IsDefault: item.IsActive, // IsActive typically means default
		})
	}

	return engines, nil
}

// ParseAutofillJSON parses Autofill.json.
func ParseAutofillJSON(r io.Reader) ([]Autofill, error) {
	var raw autofillJSON
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode autofill JSON: %w", err)
	}

	autofills := make([]Autofill, 0, len(raw))
	for _, item := range raw {
		autofills = append(autofills, Autofill{
			Name:        item.Name,
			Value:       item.Value,
			DateCreated: item.DateCreated,
		})
	}

	return autofills, nil
}

// microsToTime converts Chrome's microsecond timestamp to time.Time.
func microsToTime(micros int64) time.Time {
	if micros == 0 {
		return time.Time{}
	}
	return time.Unix(0, micros*1000)
}


// ── Metadata interface implementation for Bookmark ───────────────────────────

// GetTitle returns the title or name of the item.
func (b Bookmark) GetTitle() string {
	return b.Name
}

// GetDescription returns a description or summary.
func (b Bookmark) GetDescription() string {
	return fmt.Sprintf("Bookmark: %s", b.URL)
}

// GetDate returns the primary date/time.
func (b Bookmark) GetDate() time.Time {
	return b.DateAdded
}

// GetLocation returns any location information.
func (b Bookmark) GetLocation() string {
	if len(b.ParentFolders) > 0 {
		return strings.Join(b.ParentFolders, "/")
	}
	return ""
}

// GetTags returns tags or labels.
func (b Bookmark) GetTags() []string {
	// Use folder path as tags
	return b.ParentFolders
}

// GetAuthor returns the author or creator.
func (b Bookmark) GetAuthor() string {
	return ""
}

// GetSource returns the source or origin.
func (b Bookmark) GetSource() string {
	return "Chrome Bookmarks"
}

// GetURL returns any associated URL.
func (b Bookmark) GetURL() string {
	return b.URL
}

// GetAttachments returns paths to related files.
func (b Bookmark) GetAttachments() []string {
	return nil
}

// GetMetadata returns all metadata as a map.
func (b Bookmark) GetMetadata() map[string]any {
	return map[string]any{
		"name":           b.Name,
		"url":            b.URL,
		"date_added":     b.DateAdded,
		"folder":         b.Folder,
		"parent_folders": b.ParentFolders,
	}
}

// GetExportName returns the suggested filename for export.
func (b Bookmark) GetExportName() string {
	// Sanitize name for filename
	name := strings.ReplaceAll(b.Name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	return name + ".url"
}

// ── Metadata interface implementation for HistoryEntry ────────────────────────

// GetTitle returns the title or name of the item.
func (h HistoryEntry) GetTitle() string {
	return h.Title
}

// GetDescription returns a description or summary.
func (h HistoryEntry) GetDescription() string {
	return fmt.Sprintf("Visited %d times", h.VisitCount)
}

// GetDate returns the primary date/time.
func (h HistoryEntry) GetDate() time.Time {
	return h.LastVisited
}

// GetLocation returns any location information.
func (h HistoryEntry) GetLocation() string {
	return ""
}

// GetTags returns tags or labels.
func (h HistoryEntry) GetTags() []string {
	if h.PageTransition != "" {
		return []string{h.PageTransition}
	}
	return nil
}

// GetAuthor returns the author or creator.
func (h HistoryEntry) GetAuthor() string {
	return ""
}

// GetSource returns the source or origin.
func (h HistoryEntry) GetSource() string {
	return "Chrome History"
}

// GetURL returns any associated URL.
func (h HistoryEntry) GetURL() string {
	return h.URL
}

// GetAttachments returns paths to related files.
func (h HistoryEntry) GetAttachments() []string {
	return nil
}

// GetMetadata returns all metadata as a map.
func (h HistoryEntry) GetMetadata() map[string]any {
	return map[string]any{
		"title":           h.Title,
		"url":             h.URL,
		"visit_count":     h.VisitCount,
		"last_visited":    h.LastVisited,
		"typed_count":     h.TypedCount,
		"page_transition": h.PageTransition,
	}
}

// GetExportName returns the suggested filename for export.
func (h HistoryEntry) GetExportName() string {
	// Use title or URL as filename
	if h.Title != "" {
		title := strings.ReplaceAll(h.Title, "/", "-")
		title = strings.ReplaceAll(title, "\\", "-")
		return title + ".url"
	}
	return "history-entry.url"
}
// ── conncore.Metadata interface implementation for ChromeLibrary ─────────────

// GetTitle returns the title for display.
func (l *ChromeLibrary) GetTitle() string {
	return "Google Chrome Browser Data"
}

// GetDescription returns a description of the data.
func (l *ChromeLibrary) GetDescription() string {
	return fmt.Sprintf("%d bookmarks, %d history entries, %d search engines, %d autofill entries",
		len(l.Bookmarks), len(l.History), len(l.SearchEngines), len(l.Autofills))
}

// GetCreatedDate returns the earliest bookmark/history timestamp.
func (l *ChromeLibrary) GetCreatedDate() time.Time {
	var earliest time.Time

	// Check bookmarks
	for _, bm := range l.Bookmarks {
		if !bm.DateAdded.IsZero() && (earliest.IsZero() || bm.DateAdded.Before(earliest)) {
			earliest = bm.DateAdded
		}
	}

	// Check history
	for _, entry := range l.History {
		if !entry.LastVisited.IsZero() && (earliest.IsZero() || entry.LastVisited.Before(earliest)) {
			earliest = entry.LastVisited
		}
	}

	return earliest
}

// GetModifiedDate returns the latest bookmark/history timestamp.
func (l *ChromeLibrary) GetModifiedDate() time.Time {
	var latest time.Time

	// Check bookmarks
	for _, bm := range l.Bookmarks {
		if bm.DateAdded.After(latest) {
			latest = bm.DateAdded
		}
	}

	// Check history
	for _, entry := range l.History {
		if entry.LastVisited.After(latest) {
			latest = entry.LastVisited
		}
	}

	return latest
}

// GetSize returns the total number of items (bookmarks + history + other data).
func (l *ChromeLibrary) GetSize() int64 {
	return int64(len(l.Bookmarks) + len(l.History) + len(l.SearchEngines) + len(l.Autofills))
}

// GetDataType returns the data type.
func (l *ChromeLibrary) GetDataType() string {
	// Chrome data is mixed type, use bookmark as primary
	return string(conncore.DataTypeBookmark)
}

// GetMediaType returns the media type.
func (l *ChromeLibrary) GetMediaType() string {
	return "application/json"
}

// GetMetadata returns all metadata as a map.
func (l *ChromeLibrary) GetMetadata() map[string]any {
	metadata := map[string]any{
		"bookmarks_count":      len(l.Bookmarks),
		"history_count":        len(l.History),
		"search_engines_count": len(l.SearchEngines),
		"autofill_count":       len(l.Autofills),
		"stats":                l.Stats,
		"folder_counts":        l.FolderCounts,
	}

	// Add date range if available
	created := l.GetCreatedDate()
	modified := l.GetModifiedDate()
	if !created.IsZero() {
		metadata["date_range_start"] = created.Format(time.RFC3339)
	}
	if !modified.IsZero() {
		metadata["date_range_end"] = modified.Format(time.RFC3339)
	}

	return metadata
}

// SetMetadata updates metadata (no-op for read-only fields).
func (l *ChromeLibrary) SetMetadata(key string, value any) error {
	// Chrome library data is read-only after parsing
	return fmt.Errorf("metadata is read-only")
}

// GetTags returns tags/labels (not applicable for Chrome data).
func (l *ChromeLibrary) GetTags() []string {
	return []string{}
}

// SetTags sets tags/labels (not supported).
func (l *ChromeLibrary) SetTags(tags []string) error {
	return fmt.Errorf("tags not supported for Chrome data")
}

// ── Helper methods ────────────────────────────────────────────────────────────

// GetBookmarkFolders returns unique folder names from bookmarks.
func (l *ChromeLibrary) GetBookmarkFolders() []string {
	folderSet := make(map[string]bool)
	for _, bm := range l.Bookmarks {
		if bm.Folder != "" {
			folderSet[bm.Folder] = true
		}
		for _, parent := range bm.ParentFolders {
			folderSet[parent] = true
		}
	}

	folders := make([]string, 0, len(folderSet))
	for folder := range folderSet {
		folders = append(folders, folder)
	}
	return folders
}

// GetHistoryByDomain groups history entries by domain.
func (l *ChromeLibrary) GetHistoryByDomain() map[string][]HistoryEntry {
	byDomain := make(map[string][]HistoryEntry)
	domainRegex := regexp.MustCompile(`^(?:https?://)?(?:www\.)?([^/]+)`)

	for _, entry := range l.History {
		matches := domainRegex.FindStringSubmatch(entry.URL)
		domain := "unknown"
		if len(matches) > 1 {
			domain = matches[1]
		}
		byDomain[domain] = append(byDomain[domain], entry)
	}

	return byDomain
}

// CountByTransitionType counts history entries by page transition type.
func (l *ChromeLibrary) CountByTransitionType() map[string]int {
	counts := make(map[string]int)
	for _, entry := range l.History {
		if entry.PageTransition != "" {
			counts[entry.PageTransition]++
		} else {
			counts["unknown"]++
		}
	}
	return counts
}
