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
	statsBarStyle      = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Background(lipgloss.Color("235")).
				Padding(0, 1)
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
// Layout: left panel (~40%) shows phase bars + sparkline,
//
//	right panel (~60%) shows the live log,
//	bottom bar shows stats.
type ProcessModel struct {
	cfg       engine.PipelineConfig
	eventsCh  chan engine.ProgressEvent
	phases    [8]components.PhaseBar
	log       components.LogPanel
	sparkline components.Sparkline
	result    *engine.PipelineResult
	err       error
	done      bool
	width     int
	height    int
	startTime time.Time

	// throughput tracking: items processed per tick
	lastItems int
	lastTick  time.Time
}

// NewProcessModel builds a ProcessModel ready to run.
// sessionDir, if non-empty, directs logs to the session folder.
func NewProcessModel(source, dest, sessionDir string, width, height int) ProcessModel {
	cfg := engine.PipelineConfig{
		SourceDir:  source,
		DestDir:    dest,
		SessionDir: sessionDir,
	}
	ch := make(chan engine.ProgressEvent, 64)

	leftW, rightW, logH := dashboardDims(width, height)

	m := ProcessModel{
		cfg:       cfg,
		eventsCh:  ch,
		width:     width,
		height:    height,
		log:       components.NewLogPanel(rightW-4, logH, 500),
		sparkline: components.NewSparkline(leftW-4, "items/s"),
		startTime: time.Now(),
		lastTick:  time.Now(),
	}
	for i, label := range phaseLabels {
		m.phases[i] = components.PhaseBar{
			Label: label,
			Width: leftW - 4,
		}
	}
	return m
}

// dashboardDims returns (leftW, rightW, logH) for the dashboard layout.
func dashboardDims(width, height int) (leftW, rightW, logH int) {
	leftW = width * 40 / 100
	if leftW < 30 {
		leftW = 30
	}
	rightW = width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}
	logH = height - 4 // minus header + stats bar
	if logH < 4 {
		logH = 4
	}
	return
}

// Init implements tea.Model — starts the pipeline and a tick.
func (m ProcessModel) Init() tea.Cmd {
	cfg := m.cfg
	ch := m.eventsCh

	// Also init spinner for each phase bar
	var spinCmds []tea.Cmd
	for i := range m.phases {
		cmd := m.phases[i].Init()
		if cmd != nil {
			spinCmds = append(spinCmds, cmd)
		}
	}

	cmds := append(spinCmds,
		func() tea.Msg {
			result, err := engine.Run(cfg, ch)
			return processDoneMsg{result: result, err: err}
		},
		tickProcess(),
	)
	return tea.Batch(cmds...)
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
		leftW, rightW, logH := dashboardDims(msg.Width, msg.Height)
		m.log.Resize(rightW-4, logH)
		m.sparkline.Width = leftW - 4
		for i := range m.phases {
			m.phases[i].Width = leftW - 4
		}
		return m, nil

	case processTickMsg:
		var cmds []tea.Cmd
		// Drain all available events
		for {
			select {
			case ev, ok := <-m.eventsCh:
				if !ok {
					goto drained
				}
				m.applyEvent(ev)
			default:
				goto drained
			}
		}
	drained:
		// Update throughput sparkline (items/s = sum of Done across active phases per tick)
		now := time.Time(msg)
		elapsed := now.Sub(m.lastTick).Seconds()
		if elapsed > 0 {
			var totalDone int
			for i := range m.phases {
				totalDone += m.phases[i].Done
			}
			delta := totalDone - m.lastItems
			if delta < 0 {
				delta = 0
			}
			rate := float64(delta) / elapsed
			m.sparkline.Push(rate)
			m.lastItems = totalDone
			m.lastTick = now
		}

		// Update spinner for active phase
		for i := range m.phases {
			if m.phases[i].Active {
				cmd := m.phases[i].UpdateSpinner(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

		// Also forward tick to log panel (mouse scroll support)
		logCmd := m.log.Update(msg)
		if logCmd != nil {
			cmds = append(cmds, logCmd)
		}

		if !m.done {
			cmds = append(cmds, tickProcess())
		}
		return m, tea.Batch(cmds...)

	case processDoneMsg:
		m.done = true
		m.result = msg.result
		m.err = msg.err
		if msg.err != nil {
			m.log.Append("ERROR: "+msg.err.Error(), true)
		} else {
			m.log.Append("Run complete!", false)
		}
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
				return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenSessions} }
			}
		}
	}

	// Forward to log for scroll handling
	logCmd := m.log.Update(msg)
	return m, logCmd
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

// View implements tea.Model — two-panel dashboard.
func (m ProcessModel) View() string {
	leftW, rightW, logH := dashboardDims(m.width, m.height)

	// ── Left panel: phases + sparkline ────────────────────────────────────────
	var leftSB strings.Builder
	leftSB.WriteString(processHeaderStyle.Render("Processing Takeout"))
	leftSB.WriteString("\n")
	for _, bar := range m.phases {
		leftSB.WriteString(bar.View())
		leftSB.WriteString("\n")
	}
	leftSB.WriteString("\n")
	leftSB.WriteString(statLabelStyle.Render("Throughput"))
	leftSB.WriteString("\n")
	leftSB.WriteString(m.sparkline.View())

	if m.done {
		leftSB.WriteString("\n\n")
		if m.err != nil {
			leftSB.WriteString(errorMsgStyle.Render("Error: " + m.err.Error()))
		} else {
			leftSB.WriteString(helpStyle.Render("enter/q → report"))
		}
	}

	leftPanel := panelBorderStyle.Width(leftW - 2).Height(logH + 2).Render(leftSB.String())

	// ── Right panel: log ──────────────────────────────────────────────────────
	_ = rightW
	rightPanel := m.log.View()

	// ── Stats bar ─────────────────────────────────────────────────────────────
	statsBar := m.buildStatsBar()

	top := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)
	return top + "\n" + statsBar
}

func (m ProcessModel) buildStatsBar() string {
	elapsed := time.Since(m.startTime).Round(time.Second)
	parts := []string{
		fmt.Sprintf("Elapsed: %s", elapsed),
	}
	if m.result != nil {
		s := m.result.Stats
		parts = append(parts,
			fmt.Sprintf("Files: %d", s.MediaFound),
			fmt.Sprintf("Albums: %d", s.Albums),
			fmt.Sprintf("Dedup: %d", s.DuplicatesRemoved),
			fmt.Sprintf("Transferred: %d", s.FilesTransferred),
			fmt.Sprintf("EXIF: %d", s.EXIFApplied),
		)
		if m.result.Report != nil && len(m.result.Report.Entries) > 0 {
			parts = append(parts, fmt.Sprintf("Missing: %d", len(m.result.Report.Entries)))
		}
	}
	return statsBarStyle.Width(m.width).Render(strings.Join(parts, "  ·  "))
}
