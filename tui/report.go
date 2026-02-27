package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fulgidus/revoco/engine"
)

var (
	reportHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")).MarginBottom(1)
	reportEmptyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	reportStatKey     = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Width(24)
	reportStatVal     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	tableBaseStyle    = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240"))
)

// ReportModel shows the final stats and a filterable missing-file table.
type ReportModel struct {
	result *engine.PipelineResult
	width  int
	height int

	// filter input
	filter textinput.Model

	// table
	tbl      table.Model
	allRows  []table.Row // all rows, pre-built
	filtered []table.Row // rows matching the current filter
}

// NewReportModel builds a ReportModel from a pipeline result.
func NewReportModel(result *engine.PipelineResult, width, height int) ReportModel {
	fi := textinput.New()
	fi.Placeholder = "Filter by title, date, folder…"
	fi.CharLimit = 128

	m := ReportModel{
		result: result,
		width:  width,
		height: height,
		filter: fi,
	}
	m.buildRows()
	m.buildTable()
	return m
}

func (m *ReportModel) buildRows() {
	if m.result == nil || m.result.Report == nil {
		return
	}
	m.allRows = make([]table.Row, 0, len(m.result.Report.Entries))
	for i, e := range m.result.Report.Entries {
		m.allRows = append(m.allRows, table.Row{
			fmt.Sprintf("%d", i+1),
			e.PhotoTakenDate,
			e.SourceFolder,
			e.Title,
		})
	}
	m.filtered = m.allRows
}

func (m *ReportModel) applyFilter(q string) {
	if q == "" {
		m.filtered = m.allRows
		return
	}
	lower := strings.ToLower(q)
	m.filtered = m.filtered[:0]
	for _, row := range m.allRows {
		for _, cell := range row {
			if strings.Contains(strings.ToLower(cell), lower) {
				m.filtered = append(m.filtered, row)
				break
			}
		}
	}
}

func (m *ReportModel) buildTable() {
	titleW := m.width - 4 - 6 - 12 - 20 // total - margins - # - date - folder
	if titleW < 20 {
		titleW = 20
	}
	cols := []table.Column{
		{Title: "#", Width: 5},
		{Title: "Date", Width: 11},
		{Title: "Folder", Width: 18},
		{Title: "Title", Width: titleW},
	}

	tableH := m.height - 20 // stats area + filter row
	if tableH < 4 {
		tableH = 4
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithRows(m.filtered),
		table.WithFocused(true),
		table.WithHeight(tableH),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)
	m.tbl = t
}

// Init implements tea.Model.
func (m ReportModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m ReportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.buildTable()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			if m.filter.Focused() {
				m.filter.Blur()
				return m, nil
			}
			return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenWelcome} }
		case "enter":
			if m.filter.Focused() {
				m.filter.Blur()
				return m, nil
			}
			return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenWelcome} }
		case "/":
			if !m.filter.Focused() {
				m.filter.Focus()
				return m, textinput.Blink
			}
		}

		if m.filter.Focused() {
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.applyFilter(m.filter.Value())
			m.tbl.SetRows(m.filtered)
			return m, cmd
		}

		// Table navigation
		var cmd tea.Cmd
		m.tbl, cmd = m.tbl.Update(msg)
		return m, cmd
	}

	// Forward to table for mouse scroll
	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	return m, cmd
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
		sb.WriteString(reportHeaderStyle.Render(
			fmt.Sprintf("Missing files (%d total, %d shown)", len(m.allRows), len(m.filtered)),
		))
		sb.WriteString("\n")

		// Filter input
		sb.WriteString(m.filter.View())
		sb.WriteString("  ")
		sb.WriteString(helpStyle.Render("/ to filter"))
		sb.WriteString("\n")

		sb.WriteString(tableBaseStyle.Render(m.tbl.View()))
		sb.WriteString("\n")

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
	sb.WriteString(helpStyle.Render("↑/↓ scroll  /  filter  q/esc return to menu"))
	return sb.String()
}
