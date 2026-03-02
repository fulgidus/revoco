// Package metadata provides ICS (iCalendar) parsing for Google Calendar Takeout.
package metadata

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// CalendarEvent represents a single calendar event (VEVENT).
type CalendarEvent struct {
	UID         string              `json:"uid"`
	Summary     string              `json:"summary"`
	Description string              `json:"description"`
	Location    string              `json:"location"`
	StartTime   time.Time           `json:"start_time"`
	EndTime     time.Time           `json:"end_time"`
	Timezone    string              `json:"timezone"`
	Recurrence  string              `json:"recurrence"` // RRULE value
	Attendees   []string            `json:"attendees"`
	Organizer   string              `json:"organizer"`
	Status      string              `json:"status"` // CONFIRMED, TENTATIVE, CANCELLED
	Created     time.Time           `json:"created"`
	Modified    time.Time           `json:"modified"`
	RawProps    map[string][]string `json:"raw_props"` // All properties for debugging
}

// Calendar represents a single calendar (VCALENDAR).
type Calendar struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Timezone    string          `json:"timezone"`
	Events      []CalendarEvent `json:"events"`
}

// CalendarLibrary represents all calendars parsed from Takeout.
type CalendarLibrary struct {
	Calendars []Calendar `json:"calendars"`
}

// ParseICS parses an ICS file (RFC 5545) and returns all calendars.
// Each ICS file may contain multiple VCALENDAR blocks.
func ParseICS(r io.Reader) ([]Calendar, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line

	var calendars []Calendar
	var currentCalendar *Calendar
	var currentEvent *CalendarEvent
	var inCalendar, inEvent bool
	var unfoldedLine strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// RFC 5545: Lines starting with space or tab are continuation lines
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			unfoldedLine.WriteString(line[1:])
			continue
		}

		// Process the previous unfolded line
		if unfoldedLine.Len() > 0 {
			processedLine := unfoldedLine.String()
			unfoldedLine.Reset()

			if err := processLine(processedLine, &inCalendar, &inEvent, &currentCalendar, &currentEvent, &calendars); err != nil {
				return nil, err
			}
		}

		// Start accumulating the current line
		unfoldedLine.WriteString(line)
	}

	// Process last line
	if unfoldedLine.Len() > 0 {
		processedLine := unfoldedLine.String()
		if err := processLine(processedLine, &inCalendar, &inEvent, &currentCalendar, &currentEvent, &calendars); err != nil {
			return nil, err
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan ics: %w", err)
	}

	return calendars, nil
}

// processLine handles a single unfolded ICS line.
func processLine(line string, inCalendar, inEvent *bool, currentCalendar **Calendar, currentEvent **CalendarEvent, calendars *[]Calendar) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	switch {
	case line == "BEGIN:VCALENDAR":
		*inCalendar = true
		*currentCalendar = &Calendar{
			Events: []CalendarEvent{},
		}

	case line == "END:VCALENDAR":
		if *currentCalendar != nil {
			*calendars = append(*calendars, **currentCalendar)
		}
		*inCalendar = false
		*currentCalendar = nil

	case line == "BEGIN:VEVENT":
		*inEvent = true
		*currentEvent = &CalendarEvent{
			RawProps: make(map[string][]string),
		}

	case line == "END:VEVENT":
		if *currentEvent != nil && *currentCalendar != nil {
			(*currentCalendar).Events = append((*currentCalendar).Events, **currentEvent)
		}
		*inEvent = false
		*currentEvent = nil

	case *inEvent && *currentEvent != nil:
		// Parse VEVENT properties
		parseEventProperty(line, *currentEvent)

	case *inCalendar && *currentCalendar != nil:
		// Parse VCALENDAR properties
		parseCalendarProperty(line, *currentCalendar)
	}

	return nil
}

// parseEventProperty parses a single VEVENT property line.
func parseEventProperty(line string, event *CalendarEvent) {
	// Split on first colon
	colonIdx := strings.Index(line, ":")
	if colonIdx == -1 {
		return
	}

	propLine := line[:colonIdx]
	value := line[colonIdx+1:]

	// Parse property name and parameters (e.g., "DTSTART;TZID=America/New_York")
	propParts := strings.Split(propLine, ";")
	propName := strings.ToUpper(propParts[0])
	params := make(map[string]string)

	for i := 1; i < len(propParts); i++ {
		paramParts := strings.SplitN(propParts[i], "=", 2)
		if len(paramParts) == 2 {
			params[strings.ToUpper(paramParts[0])] = paramParts[1]
		}
	}

	// Store raw property
	event.RawProps[propName] = append(event.RawProps[propName], value)

	// Parse specific properties
	switch propName {
	case "UID":
		event.UID = value
	case "SUMMARY":
		event.Summary = unescapeText(value)
	case "DESCRIPTION":
		event.Description = unescapeText(value)
	case "LOCATION":
		event.Location = unescapeText(value)
	case "DTSTART":
		if tz, ok := params["TZID"]; ok {
			event.Timezone = tz
		}
		if t, err := parseICSTime(value); err == nil {
			event.StartTime = t
		}
	case "DTEND":
		if t, err := parseICSTime(value); err == nil {
			event.EndTime = t
		}
	case "RRULE":
		event.Recurrence = value
	case "ATTENDEE":
		// Extract email from "mailto:user@example.com" or parameters
		attendee := strings.TrimPrefix(value, "mailto:")
		event.Attendees = append(event.Attendees, attendee)
	case "ORGANIZER":
		event.Organizer = strings.TrimPrefix(value, "mailto:")
	case "STATUS":
		event.Status = value
	case "CREATED":
		if t, err := parseICSTime(value); err == nil {
			event.Created = t
		}
	case "LAST-MODIFIED":
		if t, err := parseICSTime(value); err == nil {
			event.Modified = t
		}
	}
}

// parseCalendarProperty parses a single VCALENDAR property line.
func parseCalendarProperty(line string, cal *Calendar) {
	colonIdx := strings.Index(line, ":")
	if colonIdx == -1 {
		return
	}

	propName := strings.ToUpper(line[:colonIdx])
	value := line[colonIdx+1:]

	switch propName {
	case "X-WR-CALNAME":
		cal.Name = unescapeText(value)
	case "X-WR-CALDESC":
		cal.Description = unescapeText(value)
	case "X-WR-TIMEZONE":
		cal.Timezone = value
	}
}

// parseICSTime parses an ICS date-time value.
// Formats:
// - 20060102T150405Z (UTC)
// - 20060102T150405 (floating/local time)
// - 20060102 (date only)
func parseICSTime(dt string) (time.Time, error) {
	dt = strings.TrimSpace(dt)

	// UTC format
	if strings.HasSuffix(dt, "Z") {
		return time.Parse("20060102T150405Z", dt)
	}

	// Date-time format (floating)
	if len(dt) == 15 && dt[8] == 'T' {
		return time.Parse("20060102T150405", dt)
	}

	// Date-only format
	if len(dt) == 8 {
		return time.Parse("20060102", dt)
	}

	return time.Time{}, fmt.Errorf("unsupported date format: %s", dt)
}

// unescapeText unescapes ICS text values per RFC 5545.
// Escaped characters: \, \; \, \n \N
func unescapeText(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\N", "\n")
	s = strings.ReplaceAll(s, "\\,", ",")
	s = strings.ReplaceAll(s, "\\;", ";")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// FormatICSTime formats a time value as ICS UTC format.
func FormatICSTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("20060102T150405Z")
}

// FormatICSDate formats a time value as ICS date-only format.
func FormatICSDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("20060102")
}

// EscapeText escapes text values for ICS output per RFC 5545.
func EscapeText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// FoldLine folds long ICS lines to 75 octets per RFC 5545.
func FoldLine(line string) string {
	if len(line) <= 75 {
		return line
	}

	var result strings.Builder
	for len(line) > 75 {
		result.WriteString(line[:75])
		result.WriteString("\r\n ")
		line = line[75:]
	}
	result.WriteString(line)
	return result.String()
}

// BuildICS builds a valid ICS file from a Calendar.
func BuildICS(cal *Calendar) string {
	var b strings.Builder

	// VCALENDAR header
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//revoco//Calendar Export//EN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")

	if cal.Name != "" {
		b.WriteString(FoldLine(fmt.Sprintf("X-WR-CALNAME:%s", EscapeText(cal.Name))))
		b.WriteString("\r\n")
	}
	if cal.Description != "" {
		b.WriteString(FoldLine(fmt.Sprintf("X-WR-CALDESC:%s", EscapeText(cal.Description))))
		b.WriteString("\r\n")
	}
	if cal.Timezone != "" {
		b.WriteString(FoldLine(fmt.Sprintf("X-WR-TIMEZONE:%s", cal.Timezone)))
		b.WriteString("\r\n")
	}

	// VEVENT blocks
	for _, event := range cal.Events {
		b.WriteString("BEGIN:VEVENT\r\n")

		if event.UID != "" {
			b.WriteString(FoldLine(fmt.Sprintf("UID:%s", event.UID)))
			b.WriteString("\r\n")
		}
		if !event.StartTime.IsZero() {
			b.WriteString(FoldLine(fmt.Sprintf("DTSTART:%s", FormatICSTime(event.StartTime))))
			b.WriteString("\r\n")
		}
		if !event.EndTime.IsZero() {
			b.WriteString(FoldLine(fmt.Sprintf("DTEND:%s", FormatICSTime(event.EndTime))))
			b.WriteString("\r\n")
		}
		if event.Summary != "" {
			b.WriteString(FoldLine(fmt.Sprintf("SUMMARY:%s", EscapeText(event.Summary))))
			b.WriteString("\r\n")
		}
		if event.Description != "" {
			b.WriteString(FoldLine(fmt.Sprintf("DESCRIPTION:%s", EscapeText(event.Description))))
			b.WriteString("\r\n")
		}
		if event.Location != "" {
			b.WriteString(FoldLine(fmt.Sprintf("LOCATION:%s", EscapeText(event.Location))))
			b.WriteString("\r\n")
		}
		if event.Status != "" {
			b.WriteString(FoldLine(fmt.Sprintf("STATUS:%s", event.Status)))
			b.WriteString("\r\n")
		}
		if event.Recurrence != "" {
			b.WriteString(FoldLine(fmt.Sprintf("RRULE:%s", event.Recurrence)))
			b.WriteString("\r\n")
		}
		if event.Organizer != "" {
			b.WriteString(FoldLine(fmt.Sprintf("ORGANIZER:mailto:%s", event.Organizer)))
			b.WriteString("\r\n")
		}
		for _, attendee := range event.Attendees {
			b.WriteString(FoldLine(fmt.Sprintf("ATTENDEE:mailto:%s", attendee)))
			b.WriteString("\r\n")
		}
		if !event.Created.IsZero() {
			b.WriteString(FoldLine(fmt.Sprintf("CREATED:%s", FormatICSTime(event.Created))))
			b.WriteString("\r\n")
		}
		if !event.Modified.IsZero() {
			b.WriteString(FoldLine(fmt.Sprintf("LAST-MODIFIED:%s", FormatICSTime(event.Modified))))
			b.WriteString("\r\n")
		}

		b.WriteString("END:VEVENT\r\n")
	}

	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}
