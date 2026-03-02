// Package metadata provides Google Passwords CSV parsing.
package metadata

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	conncore "github.com/fulgidus/revoco/connectors"
)

// PasswordEntry represents a single password entry from Google Passwords.
type PasswordEntry struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	Note     string `json:"note"`
}

// PasswordLibrary represents all parsed password entries.
type PasswordLibrary struct {
	Entries    []PasswordEntry `json:"entries"`
	SourcePath string          `json:"source_path"`
	Stats      PasswordStats   `json:"stats"`
}

// PasswordStats contains statistics about the password library.
type PasswordStats struct {
	TotalEntries      int            `json:"total_entries"`
	UniqueDomains     int            `json:"unique_domains"`
	EntriesWithNotes  int            `json:"entries_with_notes"`
	EntriesNoURL      int            `json:"entries_no_url"`
	EntriesNoUsername int            `json:"entries_no_username"`
	DomainBreakdown   map[string]int `json:"domain_breakdown"` // domain -> count
}

// ParsePasswordsCSV parses Google Passwords CSV format.
// Format: name,url,username,password,note (5 columns)
// Note: Some exports may not have "name" column, only: url,username,password,note
func ParsePasswordsCSV(r io.Reader) ([]PasswordEntry, error) {
	csvReader := csv.NewReader(r)

	// Read header
	header, err := csvReader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil // Empty CSV
		}
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Build column index map
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.ToLower(strings.TrimSpace(col))] = i
	}

	// Verify required columns (url, username, password are essential)
	requiredCols := []string{"url", "username", "password"}
	for _, col := range requiredCols {
		if _, ok := colMap[col]; !ok {
			return nil, fmt.Errorf("missing required column: %s", col)
		}
	}

	var entries []PasswordEntry

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read CSV record: %w", err)
		}

		entry := PasswordEntry{}

		// Parse Name (optional)
		if idx, ok := colMap["name"]; ok && idx < len(record) {
			entry.Name = strings.TrimSpace(record[idx])
		}

		// Parse URL (required)
		if idx := colMap["url"]; idx < len(record) {
			entry.URL = strings.TrimSpace(record[idx])
		}

		// Parse Username (required)
		if idx := colMap["username"]; idx < len(record) {
			entry.Username = strings.TrimSpace(record[idx])
		}

		// Parse Password (required)
		if idx := colMap["password"]; idx < len(record) {
			entry.Password = strings.TrimSpace(record[idx])
		}

		// Parse Note (optional)
		if idx, ok := colMap["note"]; ok && idx < len(record) {
			entry.Note = strings.TrimSpace(record[idx])
		}

		// If Name is empty, try to extract from URL
		if entry.Name == "" && entry.URL != "" {
			entry.Name = extractDomainFromURL(entry.URL)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// extractDomainFromURL extracts the domain from a URL for use as entry name.
func extractDomainFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Host == "" {
		return rawURL // Return original if parsing fails
	}
	// Remove "www." prefix
	domain := parsedURL.Host
	if strings.HasPrefix(domain, "www.") {
		domain = domain[4:]
	}
	return domain
}

// CalculateStats computes statistics for the password library.
func (p *PasswordLibrary) CalculateStats() {
	p.Stats = PasswordStats{
		TotalEntries:    len(p.Entries),
		DomainBreakdown: make(map[string]int),
	}

	domainSet := make(map[string]bool)

	for _, entry := range p.Entries {
		// Count entries with notes
		if entry.Note != "" {
			p.Stats.EntriesWithNotes++
		}

		// Count entries without URL
		if entry.URL == "" {
			p.Stats.EntriesNoURL++
		}

		// Count entries without username
		if entry.Username == "" {
			p.Stats.EntriesNoUsername++
		}

		// Track unique domains
		if entry.URL != "" {
			domain := extractDomainFromURL(entry.URL)
			if domain != "" {
				domainSet[domain] = true
				p.Stats.DomainBreakdown[domain]++
			}
		}
	}

	p.Stats.UniqueDomains = len(domainSet)
}

// GetID returns the library identifier.
func (p *PasswordLibrary) GetID() string {
	return fmt.Sprintf("passwords-%d-entries", len(p.Entries))
}

// GetTitle returns a human-readable title.
func (p *PasswordLibrary) GetTitle() string {
	return fmt.Sprintf("Google Passwords (%d entries)", p.Stats.TotalEntries)
}

// GetDataType returns the connector data type.
func (p *PasswordLibrary) GetDataType() conncore.DataType {
	return conncore.DataTypePassword
}

// GetCreatedAt returns a zero time (not applicable for passwords).
func (p *PasswordLibrary) GetCreatedAt() time.Time {
	return time.Time{}
}

// GetModifiedAt returns a zero time (not applicable for passwords).
func (p *PasswordLibrary) GetModifiedAt() time.Time {
	return time.Time{}
}

// GetSize returns the number of password entries.
func (p *PasswordLibrary) GetSize() int64 {
	return int64(len(p.Entries))
}

// GetMimeType returns the CSV MIME type.
func (p *PasswordLibrary) GetMimeType() string {
	return "text/csv"
}

// GetTags returns password-related tags.
func (p *PasswordLibrary) GetTags() []string {
	return []string{"passwords", "credentials", "security"}
}

// GetDescription returns a summary of the password library.
func (p *PasswordLibrary) GetDescription() string {
	return fmt.Sprintf("Google Passwords: %d total entries, %d unique domains",
		p.Stats.TotalEntries, p.Stats.UniqueDomains)
}

// GetMetadata returns all metadata as a map.
func (p *PasswordLibrary) GetMetadata() map[string]interface{} {
	return map[string]interface{}{
		"id":                   p.GetID(),
		"title":                p.GetTitle(),
		"data_type":            string(p.GetDataType()),
		"size":                 p.GetSize(),
		"mime_type":            p.GetMimeType(),
		"tags":                 p.GetTags(),
		"description":          p.GetDescription(),
		"total_entries":        p.Stats.TotalEntries,
		"unique_domains":       p.Stats.UniqueDomains,
		"entries_with_notes":   p.Stats.EntriesWithNotes,
		"entries_no_url":       p.Stats.EntriesNoURL,
		"entries_no_username":  p.Stats.EntriesNoUsername,
		"domain_breakdown_top": topDomains(p.Stats.DomainBreakdown, 10),
	}
}

// topDomains returns the top N domains by entry count.
func topDomains(breakdown map[string]int, n int) map[string]int {
	if len(breakdown) <= n {
		return breakdown
	}

	// Simple top-N extraction (not sorted, just first N)
	result := make(map[string]int)
	count := 0
	for domain, cnt := range breakdown {
		if count >= n {
			break
		}
		result[domain] = cnt
		count++
	}
	return result
}

// Metadata interface methods for PasswordEntry
func (e *PasswordEntry) GetID() string {
	// Use domain + username as ID (sanitized)
	domain := extractDomainFromURL(e.URL)
	if domain == "" {
		domain = "unknown"
	}
	return fmt.Sprintf("password-%s-%s", domain, e.Username)
}

func (e *PasswordEntry) GetTitle() string {
	if e.Name != "" {
		return e.Name
	}
	return extractDomainFromURL(e.URL)
}

func (e *PasswordEntry) GetDataType() conncore.DataType {
	return conncore.DataTypePassword
}

func (e *PasswordEntry) GetCreatedAt() time.Time {
	return time.Time{}
}

func (e *PasswordEntry) GetModifiedAt() time.Time {
	return time.Time{}
}

func (e *PasswordEntry) GetSize() int64 {
	return 1
}

func (e *PasswordEntry) GetMimeType() string {
	return "text/csv"
}

func (e *PasswordEntry) GetTags() []string {
	tags := []string{"password", "credential"}
	if e.URL != "" {
		domain := extractDomainFromURL(e.URL)
		if domain != "" {
			tags = append(tags, domain)
		}
	}
	return tags
}

func (e *PasswordEntry) GetDescription() string {
	return fmt.Sprintf("Password for %s (%s)", e.URL, e.Username)
}

func (e *PasswordEntry) GetMetadata() map[string]interface{} {
	return map[string]interface{}{
		"id":        e.GetID(),
		"title":     e.GetTitle(),
		"data_type": string(e.GetDataType()),
		"url":       e.URL,
		"username":  e.Username,
		"has_note":  e.Note != "",
		"has_name":  e.Name != "",
	}
}
