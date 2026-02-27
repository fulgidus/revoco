package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fulgidus/revoco/engine"
)

var (
	reportHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")).MarginBottom(1)
	reportRowStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	reportEmptyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	reportStatKey     = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Width(24)
	reportStatVal     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
)

// ReportModel shows the final stats and missing-file summary.
type ReportModel struct {
	result *engine.PipelineResult
	offset int // scroll offset in missing entries list
	width  int
	height int
}

// NewReportModel builds a ReportModel from a pipeline result.
func NewReportModel(result *engine.PipelineResult, width, height int) ReportModel {
	return ReportModel{result: result, width: width, height: height}
}

// Init implements tea.Model.
func (m ReportModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m ReportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.offset > 0 {
				m.offset--
			}
		case "down", "j":
			if m.result != nil && m.result.Report != nil {
				max := len(m.result.Report.Entries) - 1
				if m.offset < max {
					m.offset++
				}
			}
		case "q", "esc", "enter":
			return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenWelcome} }
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m ReportModel) View() string {
	var sb strings.Builder

	sb.WriteString(reportHeaderStyle.Render("Run Summary"))
	sb.WriteString("\n\n")

	if m.result == nil {
		sb.WriteString(reportEmptyStyle.Render("No result available."))
		return sb.String()
	}

	s := m.result.Stats
	rows := [][2]string{
		{"Media found", fmt.Sprintf("%d", s.MediaFound)},
		{"JSON matched", fmt.Sprintf("%d", s.JSONMatched)},
		{"JSON orphans", fmt.Sprintf("%d", s.JSONOrphans)},
		{"Albums", fmt.Sprintf("%d", s.Albums)},
		{"Duplicates removed", fmt.Sprintf("%d", s.DuplicatesRemoved)},
		{"Files transferred", fmt.Sprintf("%d", s.FilesTransferred)},
		{"Conflicts resolved", fmt.Sprintf("%d", s.ConflictsResolved)},
		{"Motion photos", fmt.Sprintf("%d", s.MPConverted)},
		{"EXIF applied", fmt.Sprintf("%d", s.EXIFApplied)},
		{"Date from filename", fmt.Sprintf("%d", s.DateFromFilename)},
		{"Errors", fmt.Sprintf("%d", s.Errors)},
	}
	for _, row := range rows {
		sb.WriteString(reportStatKey.Render(row[0]))
		sb.WriteString(reportStatVal.Render(row[1]))
		sb.WriteString("\n")
	}

	if m.result.LogPath != "" {
		sb.WriteString("\n")
		sb.WriteString(statLabelStyle.Render("Log: " + m.result.LogPath))
		sb.WriteString("\n")
	}

	if m.result.Report != nil && len(m.result.Report.Entries) > 0 {
		sb.WriteString("\n")
		sb.WriteString(reportHeaderStyle.Render(fmt.Sprintf("Missing files (%d)", len(m.result.Report.Entries))))
		sb.WriteString("\n")

		entries := m.result.Report.Entries
		visibleLines := m.height - 20
		if visibleLines < 3 {
			visibleLines = 3
		}

		end := m.offset + visibleLines
		if end > len(entries) {
			end = len(entries)
		}
		for i := m.offset; i < end; i++ {
			e := entries[i]
			line := fmt.Sprintf("[%d] %s  %s", i+1, e.PhotoTakenDate, e.Title)
			if len(line) > m.width-4 {
				line = line[:m.width-7] + "..."
			}
			sb.WriteString(reportRowStyle.Render(line))
			sb.WriteString("\n")
		}
		if len(entries) > visibleLines {
			sb.WriteString(helpStyle.Render(fmt.Sprintf("↑/↓ scroll (%d/%d)", m.offset+1, len(entries))))
			sb.WriteString("\n")
		}
		if m.result.Report.Path != "" {
			sb.WriteString(statLabelStyle.Render("Written to: " + m.result.Report.Path))
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("\n")
		sb.WriteString(reportEmptyStyle.Render("No missing files — all media was successfully processed."))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("enter/q/esc to return to menu"))
	return sb.String()
}
