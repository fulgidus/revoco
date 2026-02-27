package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fulgidus/revoco/engine"
	"github.com/fulgidus/revoco/tui/components"
)

// processTickMsg fires on a timer to refresh the display.
type processTickMsg time.Time

// processEventMsg wraps a ProgressEvent received from the pipeline.
type processEventMsg engine.ProgressEvent

// processDoneMsg signals the pipeline completed.
type processDoneMsg struct {
	result *engine.PipelineResult
	err    error
}

var (
	processHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).MarginBottom(1)
	statLabelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	statValueStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	errorMsgStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

// phaseLabel maps phase numbers to names.
var phaseLabels = []string{
	"Setup",
	"Indexing",
	"Albums",
	"Dedup",
	"Transfer",
	"Motion photos",
	"EXIF metadata",
	"Report",
}

// ProcessModel is the TUI screen for a processing run.
type ProcessModel struct {
	cfg       engine.PipelineConfig
	eventsCh  chan engine.ProgressEvent
	phases    [8]components.PhaseBar
	log       components.LogPanel
	result    *engine.PipelineResult
	err       error
	done      bool
	width     int
	height    int
	startTime time.Time
}

// NewProcessModel builds a ProcessModel ready to run.
func NewProcessModel(source, dest string, width, height int) ProcessModel {
	cfg := engine.PipelineConfig{
		SourceDir: source,
		DestDir:   dest,
	}
	ch := make(chan engine.ProgressEvent, 64)
	m := ProcessModel{
		cfg:      cfg,
		eventsCh: ch,
		width:    width,
		height:   height,
		log:      components.NewLogPanel(width-4, 8, 200),
	}
	for i, label := range phaseLabels {
		m.phases[i] = components.PhaseBar{
			Label: label,
			Width: width - 4,
		}
	}
	return m
}

// Init implements tea.Model — starts the pipeline and a tick.
func (m ProcessModel) Init() tea.Cmd {
	m.startTime = time.Now()
	cfg := m.cfg
	ch := m.eventsCh
	return tea.Batch(
		func() tea.Msg {
			result, err := engine.Run(cfg, ch)
			return processDoneMsg{result: result, err: err}
		},
		tickProcess(),
	)
}

func tickProcess() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return processTickMsg(t)
	})
}

// Update implements tea.Model.
func (m ProcessModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.log.Width = msg.Width - 4
		return m, nil

	case processTickMsg:
		// Drain all available events
		for {
			select {
			case ev, ok := <-m.eventsCh:
				if !ok {
					return m, nil
				}
				m.applyEvent(ev)
			default:
				goto done
			}
		}
	done:
		if !m.done {
			return m, tickProcess()
		}
		return m, nil

	case processDoneMsg:
		m.done = true
		m.result = msg.result
		m.err = msg.err
		if msg.err != nil {
			m.log.Append("ERROR: "+msg.err.Error(), true)
		} else {
			m.log.Append("Run complete!", false)
		}
		// Mark all phases done
		for i := range m.phases {
			m.phases[i].Active = false
			if msg.err == nil {
				m.phases[i].Finished = true
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.done {
			switch msg.String() {
			case "q", "esc", "enter":
				if m.result != nil {
					rm := NewReportModel(m.result, m.width, m.height)
					return m, func() tea.Msg {
						return SwitchScreenMsg{To: ScreenReport, Report: &rm}
					}
				}
				return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenWelcome} }
			}
		}
	}
	return m, nil
}

func (m *ProcessModel) applyEvent(ev engine.ProgressEvent) {
	if ev.Phase >= 0 && ev.Phase < len(m.phases) {
		for i := range m.phases {
			m.phases[i].Active = (i == ev.Phase)
			if i < ev.Phase {
				m.phases[i].Finished = true
			}
		}
		m.phases[ev.Phase].Done = ev.Done
		m.phases[ev.Phase].Total = ev.Total
		m.phases[ev.Phase].Label = ev.Label
	}
	if ev.Message != "" {
		m.log.Append(ev.Message, false)
	}
}

// View implements tea.Model.
func (m ProcessModel) View() string {
	var sb strings.Builder

	sb.WriteString(processHeaderStyle.Render("Processing Takeout"))
	sb.WriteString("\n")
	sb.WriteString(statLabelStyle.Render(fmt.Sprintf("Source: %s  Dest: %s", m.cfg.SourceDir, m.cfg.DestDir)))
	sb.WriteString("\n\n")

	for _, bar := range m.phases {
		sb.WriteString(bar.View())
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	if m.err != nil {
		sb.WriteString(errorMsgStyle.Render("Error: " + m.err.Error()))
		sb.WriteString("\n\n")
	} else if m.done && m.result != nil {
		s := m.result.Stats
		sb.WriteString(statLabelStyle.Render(fmt.Sprintf(
			"Files: %d  Albums: %d  Dedup: %d  Transferred: %d  EXIF: %d",
			s.MediaFound, s.Albums, s.DuplicatesRemoved, s.FilesTransferred, s.EXIFApplied,
		)))
		sb.WriteString("\n")
		if m.result.Report != nil && len(m.result.Report.Entries) > 0 {
			sb.WriteString(statLabelStyle.Render(fmt.Sprintf("Missing: %d  (see missing-files.json)", len(m.result.Report.Entries))))
			sb.WriteString("\n")
		}
		elapsed := time.Since(m.startTime).Round(time.Second)
		sb.WriteString(statLabelStyle.Render(fmt.Sprintf("Elapsed: %s", elapsed)))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("enter/q to continue"))
	}

	sb.WriteString(m.log.View())

	return sb.String()
}
