package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fulgidus/revoco/metadata"
)

// MissingReport is the output of Phase 8.
type MissingReport struct {
	Entries []metadata.MissingEntry
	Path    string
}

// GenerateReport builds missing-files.json from orphan JSON entries and writes it to destDir.
func GenerateReport(orphans []OrphanJSON, destDir string) (*MissingReport, error) {
	entries := make([]metadata.MissingEntry, 0, len(orphans))
	for _, o := range orphans {
		taken := ""
		if o.PhotoTakenTS > 0 {
			taken = time.Unix(o.PhotoTakenTS, 0).UTC().Format("2006-01-02")
		} else if o.CreationTS > 0 {
			taken = time.Unix(o.CreationTS, 0).UTC().Format("2006-01-02")
		}
		entries = append(entries, metadata.MissingEntry{
			Title:          o.Title,
			URL:            o.URL,
			PhotoTakenDate: taken,
			SourceFolder:   o.SourceFolder,
		})
	}

	report := &MissingReport{Entries: entries}

	if len(entries) == 0 {
		return report, nil
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return report, fmt.Errorf("marshal report: %w", err)
	}

	reportPath := filepath.Join(destDir, "missing-files.json")
	if err := os.WriteFile(reportPath, data, 0o644); err != nil {
		return report, fmt.Errorf("write report: %w", err)
	}
	report.Path = reportPath
	return report, nil
}

// Stats is the final summary of a processing run.
type Stats struct {
	MediaFound        int
	JSONMatched       int
	JSONOrphans       int
	DuplicatesRemoved int
	FilesTransferred  int
	ConflictsResolved int
	MPConverted       int
	EXIFApplied       int
	DateFromFilename  int
	Errors            int
	Albums            int
}

// FormatTimestamp formats a Unix timestamp as "YYYY:MM:DD HH:MM:SS" for EXIF.
func FormatTimestamp(unix int64) string {
	return time.Unix(unix, 0).Local().Format("2006:01:02 15:04:05")
}

// FormatDate formats a Unix timestamp as a date-only string.
func FormatDate(unix int64) string {
	return time.Unix(unix, 0).UTC().Format("2006-01-02")
}

// ParseUnixString parses a string Unix timestamp, returning 0 on error.
func ParseUnixString(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
