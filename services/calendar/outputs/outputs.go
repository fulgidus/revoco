// Package outputs provides Calendar export outputs.
package outputs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/calendar/metadata"
	"github.com/fulgidus/revoco/services/core"
)

func init() {
	// Auto-register outputs
	_ = core.RegisterOutput(NewICSOutput())
	_ = core.RegisterOutput(NewJSONOutput())
	_ = core.RegisterOutput(NewCSVOutput())
}

// ══════════════════════════════════════════════════════════════════════════════
// ICS Output - Export as clean ICS files
// ══════════════════════════════════════════════════════════════════════════════

// ICSOutput exports calendars as ICS files.
type ICSOutput struct {
	destDir            string
	oneFilePerCalendar bool
	mergeAll           bool
	calendars          map[string]*metadata.Calendar // Collect calendars during Export
}

// NewICSOutput creates a new ICS output.
func NewICSOutput() *ICSOutput {
	return &ICSOutput{
		oneFilePerCalendar: true,
		calendars:          make(map[string]*metadata.Calendar),
	}
}

func (o *ICSOutput) ID() string   { return "calendar-ics" }
func (o *ICSOutput) Name() string { return "Calendar ICS" }
func (o *ICSOutput) Description() string {
	return "Export calendars as clean ICS files (RFC 5545)"
}

func (o *ICSOutput) SupportedItemTypes() []string {
	return []string{"calendar_event"}
}

func (o *ICSOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for ICS files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "one_file_per_calendar",
			Name:        "One File Per Calendar",
			Description: "Create separate ICS file for each calendar",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "merge_all",
			Name:        "Merge All Events",
			Description: "Merge all events into a single ICS file",
			Type:        "bool",
			Default:     false,
		},
	}
}

func (o *ICSOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["one_file_per_calendar"].(bool); ok {
		o.oneFilePerCalendar = v
	}
	if v, ok := cfg.Settings["merge_all"].(bool); ok {
		o.mergeAll = v
	}

	o.calendars = make(map[string]*metadata.Calendar)
	return os.MkdirAll(o.destDir, 0o755)
}

func (o *ICSOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "calendar_event" {
		return nil
	}

	// Extract calendar and event from metadata
	var cal *metadata.Calendar
	var event *metadata.CalendarEvent

	if calData, ok := item.Metadata["calendar"]; ok {
		if c, ok := calData.(metadata.Calendar); ok {
			cal = &c
		}
	}

	if eventData, ok := item.Metadata["event"]; ok {
		if e, ok := eventData.(metadata.CalendarEvent); ok {
			event = &e
		}
	}

	// If we have a full calendar, store it
	if cal != nil {
		calName := cal.Name
		if calName == "" {
			calName = "Uncategorized"
		}
		if _, exists := o.calendars[calName]; !exists {
			o.calendars[calName] = cal
		}
		return nil
	}

	// If we have a standalone event, add it to the appropriate calendar
	if event != nil {
		calName := "Uncategorized"
		if cn, ok := item.Metadata["calendar"].(string); ok && cn != "" {
			calName = cn
		}

		if _, exists := o.calendars[calName]; !exists {
			o.calendars[calName] = &metadata.Calendar{
				Name:   calName,
				Events: []metadata.CalendarEvent{},
			}
		}

		// Avoid duplicates by UID
		found := false
		for _, e := range o.calendars[calName].Events {
			if e.UID == event.UID {
				found = true
				break
			}
		}
		if !found {
			o.calendars[calName].Events = append(o.calendars[calName].Events, *event)
		}
	}

	return nil
}

func (o *ICSOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil && (i+1)%100 == 0 {
			progress(i+1, len(items))
		}
	}
	if progress != nil {
		progress(len(items), len(items))
	}
	return nil
}

func (o *ICSOutput) Finalize(ctx context.Context) error {
	if o.mergeAll {
		return o.exportMerged(ctx)
	}
	return o.exportPerCalendar(ctx)
}

// exportPerCalendar creates one ICS file per calendar.
func (o *ICSOutput) exportPerCalendar(ctx context.Context) error {
	for _, cal := range o.calendars {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		filename := sanitizeFilename(cal.Name)
		if filename == "" {
			filename = "calendar"
		}
		filename += ".ics"

		icsPath := filepath.Join(o.destDir, filename)
		icsContent := metadata.BuildICS(cal)

		if err := os.WriteFile(icsPath, []byte(icsContent), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", filename, err)
		}
	}

	return nil
}

// exportMerged creates a single ICS file with all events.
func (o *ICSOutput) exportMerged(ctx context.Context) error {
	merged := metadata.Calendar{
		Name:        "Merged Calendar",
		Description: "All calendars merged",
		Events:      []metadata.CalendarEvent{},
	}

	for _, cal := range o.calendars {
		merged.Events = append(merged.Events, cal.Events...)
	}

	icsPath := filepath.Join(o.destDir, "merged_calendar.ics")
	icsContent := metadata.BuildICS(&merged)

	return os.WriteFile(icsPath, []byte(icsContent), 0o644)
}

// ══════════════════════════════════════════════════════════════════════════════
// JSON Output - Export as JSON
// ══════════════════════════════════════════════════════════════════════════════

// JSONOutput exports calendars as JSON.
type JSONOutput struct {
	destDir         string
	prettyPrint     bool
	splitByCalendar bool
	calendars       map[string]*metadata.Calendar
}

// NewJSONOutput creates a new JSON output.
func NewJSONOutput() *JSONOutput {
	return &JSONOutput{
		prettyPrint: true,
		calendars:   make(map[string]*metadata.Calendar),
	}
}

func (o *JSONOutput) ID() string   { return "calendar-json" }
func (o *JSONOutput) Name() string { return "Calendar JSON" }
func (o *JSONOutput) Description() string {
	return "Export calendars and events as structured JSON"
}

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"calendar_event"}
}

func (o *JSONOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for JSON files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "pretty_print",
			Name:        "Pretty Print",
			Description: "Format JSON with indentation",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "split_by_calendar",
			Name:        "Split by Calendar",
			Description: "Create separate JSON file for each calendar",
			Type:        "bool",
			Default:     false,
		},
	}
}

func (o *JSONOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["pretty_print"].(bool); ok {
		o.prettyPrint = v
	}
	if v, ok := cfg.Settings["split_by_calendar"].(bool); ok {
		o.splitByCalendar = v
	}

	o.calendars = make(map[string]*metadata.Calendar)
	return os.MkdirAll(o.destDir, 0o755)
}

func (o *JSONOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "calendar_event" {
		return nil
	}

	// Collect calendar/event data (same logic as ICS output)
	var cal *metadata.Calendar
	var event *metadata.CalendarEvent

	if calData, ok := item.Metadata["calendar"]; ok {
		if c, ok := calData.(metadata.Calendar); ok {
			cal = &c
		}
	}

	if eventData, ok := item.Metadata["event"]; ok {
		if e, ok := eventData.(metadata.CalendarEvent); ok {
			event = &e
		}
	}

	if cal != nil {
		calName := cal.Name
		if calName == "" {
			calName = "Uncategorized"
		}
		if _, exists := o.calendars[calName]; !exists {
			o.calendars[calName] = cal
		}
		return nil
	}

	if event != nil {
		calName := "Uncategorized"
		if cn, ok := item.Metadata["calendar"].(string); ok && cn != "" {
			calName = cn
		}

		if _, exists := o.calendars[calName]; !exists {
			o.calendars[calName] = &metadata.Calendar{
				Name:   calName,
				Events: []metadata.CalendarEvent{},
			}
		}

		found := false
		for _, e := range o.calendars[calName].Events {
			if e.UID == event.UID {
				found = true
				break
			}
		}
		if !found {
			o.calendars[calName].Events = append(o.calendars[calName].Events, *event)
		}
	}

	return nil
}

func (o *JSONOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil && (i+1)%100 == 0 {
			progress(i+1, len(items))
		}
	}
	if progress != nil {
		progress(len(items), len(items))
	}
	return nil
}

func (o *JSONOutput) Finalize(ctx context.Context) error {
	if o.splitByCalendar {
		return o.exportSplit(ctx)
	}

	// Single JSON file with all calendars
	library := &metadata.CalendarLibrary{
		Calendars: make([]metadata.Calendar, 0, len(o.calendars)),
	}

	for _, cal := range o.calendars {
		library.Calendars = append(library.Calendars, *cal)
	}

	jsonPath := filepath.Join(o.destDir, "calendars.json")
	return o.writeJSON(jsonPath, library)
}

// exportSplit creates one JSON file per calendar.
func (o *JSONOutput) exportSplit(ctx context.Context) error {
	for _, cal := range o.calendars {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		filename := sanitizeFilename(cal.Name)
		if filename == "" {
			filename = "calendar"
		}
		filename += ".json"

		jsonPath := filepath.Join(o.destDir, filename)
		if err := o.writeJSON(jsonPath, cal); err != nil {
			return err
		}
	}

	return nil
}

// writeJSON writes JSON data to a file.
func (o *JSONOutput) writeJSON(path string, data any) error {
	var jsonData []byte
	var err error

	if o.prettyPrint {
		jsonData, err = json.MarshalIndent(data, "", "  ")
	} else {
		jsonData, err = json.Marshal(data)
	}

	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	return os.WriteFile(path, jsonData, 0o644)
}

// ══════════════════════════════════════════════════════════════════════════════
// CSV Output - Export as flat CSV
// ══════════════════════════════════════════════════════════════════════════════

// CSVOutput exports events as CSV.
type CSVOutput struct {
	destDir            string
	csvPath            string
	writer             *csv.Writer
	file               *os.File
	includeDescription bool
	includeAttendees   bool
}

// NewCSVOutput creates a new CSV output.
func NewCSVOutput() *CSVOutput {
	return &CSVOutput{}
}

func (o *CSVOutput) ID() string   { return "calendar-csv" }
func (o *CSVOutput) Name() string { return "Calendar CSV" }
func (o *CSVOutput) Description() string {
	return "Export events as flat CSV (date, time, title, location)"
}

func (o *CSVOutput) SupportedItemTypes() []string {
	return []string{"calendar_event"}
}

func (o *CSVOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for CSV file",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "include_description",
			Name:        "Include Description",
			Description: "Add event description column to CSV",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "include_attendees",
			Name:        "Include Attendees",
			Description: "Add attendees column to CSV",
			Type:        "bool",
			Default:     false,
		},
	}
}

func (o *CSVOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["include_description"].(bool); ok {
		o.includeDescription = v
	}
	if v, ok := cfg.Settings["include_attendees"].(bool); ok {
		o.includeAttendees = v
	}

	if err := os.MkdirAll(o.destDir, 0o755); err != nil {
		return err
	}

	// Create CSV file and writer
	o.csvPath = filepath.Join(o.destDir, "events.csv")
	file, err := os.Create(o.csvPath)
	if err != nil {
		return fmt.Errorf("create csv: %w", err)
	}
	o.file = file
	o.writer = csv.NewWriter(file)

	// Write header
	header := []string{"Calendar", "UID", "Summary", "Start Date", "Start Time", "End Date", "End Time", "Location", "Status", "Recurrence"}
	if o.includeDescription {
		header = append(header, "Description")
	}
	if o.includeAttendees {
		header = append(header, "Organizer", "Attendees")
	}

	return o.writer.Write(header)
}

func (o *CSVOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "calendar_event" {
		return nil
	}

	// Extract event from metadata
	eventData, ok := item.Metadata["event"]
	if !ok {
		return nil
	}

	event, ok := eventData.(metadata.CalendarEvent)
	if !ok {
		return nil
	}

	calName := "Uncategorized"
	if cn, ok := item.Metadata["calendar"].(string); ok && cn != "" {
		calName = cn
	}

	row := []string{
		calName,
		event.UID,
		event.Summary,
		event.StartTime.Format("2006-01-02"),
		event.StartTime.Format("15:04:05"),
		event.EndTime.Format("2006-01-02"),
		event.EndTime.Format("15:04:05"),
		event.Location,
		event.Status,
		event.Recurrence,
	}

	if o.includeDescription {
		row = append(row, event.Description)
	}

	if o.includeAttendees {
		attendeeList := ""
		if len(event.Attendees) > 0 {
			attendeeList = strings.Join(event.Attendees, "; ")
		}
		row = append(row, event.Organizer, attendeeList)
	}

	return o.writer.Write(row)
}

func (o *CSVOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil && (i+1)%100 == 0 {
			progress(i+1, len(items))
		}
	}
	if progress != nil {
		progress(len(items), len(items))
	}
	return nil
}

func (o *CSVOutput) Finalize(ctx context.Context) error {
	if o.writer != nil {
		o.writer.Flush()
	}
	if o.file != nil {
		return o.file.Close()
	}
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func sanitizeFilename(name string) string {
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	s := result.String()
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// Ensure outputs implement core.Output interface.
var (
	_ core.Output = (*ICSOutput)(nil)
	_ core.Output = (*JSONOutput)(nil)
	_ core.Output = (*CSVOutput)(nil)

	// Silence unused import warning
	_ = io.EOF
)
