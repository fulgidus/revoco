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

// recoverTickMsg fires on a timer.
type recoverTickMsg time.Time

// recoverDoneMsg signals recovery completed.
type recoverDoneMsg struct {
	result *engine.RecoverResult
	err    error
}

var (
	recoverHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).MarginBottom(1)
)

// RecoverModel is the TUI screen for a recovery run.
// Layout: top — download progress bar + sparkline,
//
//	middle — live log panel,
//	bottom — stats bar.
type RecoverModel struct {
	cfg       engine.RecoverConfig
	eventsCh  chan engine.RecoverEvent
	bar       components.PhaseBar
	log       components.LogPanel
	sparkline components.Sparkline
	result    *engine.RecoverResult
	err       error
	done      bool
	width     int
	height    int
	startTime time.Time

	// throughput tracking
	lastItems int
	lastTick  time.Time
}

// NewRecoverModel builds a RecoverModel ready to run.
func NewRecoverModel(cookieJar, inputJSON string, width, height int) RecoverModel {
	cfg := engine.RecoverConfig{
		InputJSON:   inputJSON,
		OutputDir:   "./recovered",
		CookieJar:   cookieJar,
		Concurrency: 3,
		Delay:       1.0,
		MaxRetry:    3,
		StartFrom:   1,
	}
	ch := make(chan engine.RecoverEvent, 64)
	logH := height - 8
	if logH < 4 {
		logH = 4
	}
	return RecoverModel{
		cfg:       cfg,
		eventsCh:  ch,
		bar:       components.PhaseBar{Label: "Downloading", Width: width - 4},
		log:       components.NewLogPanel(width-4, logH, 500),
		sparkline: components.NewSparkline(width-4, "files/s"),
		width:     width,
		height:    height,
		startTime: time.Now(),
		lastTick:  time.Now(),
	}
}

// Init implements tea.Model — starts the recovery pipeline and a tick.
func (m RecoverModel) Init() tea.Cmd {
	cfg := m.cfg
	ch := m.eventsCh
	barCmd := m.bar.Init()
	return tea.Batch(
		barCmd,
		func() tea.Msg {
			result, err := engine.RunRecover(cfg, ch)
			return recoverDoneMsg{result: result, err: err}
		},
		tickRecover(),
	)
}

func tickRecover() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return recoverTickMsg(t)
	})
}

// Update implements tea.Model.
func (m RecoverModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		logH := msg.Height - 8
		if logH < 4 {
			logH = 4
		}
		m.log.Resize(msg.Width-4, logH)
		m.bar.Width = msg.Width - 4
		m.sparkline.Width = msg.Width - 4
		return m, nil

	case recoverTickMsg:
		var cmds []tea.Cmd
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
		// Throughput sparkline
		now := time.Time(msg)
		elapsed := now.Sub(m.lastTick).Seconds()
		if elapsed > 0 {
			delta := m.bar.Done - m.lastItems
			if delta < 0 {
				delta = 0
			}
			m.sparkline.Push(float64(delta) / elapsed)
			m.lastItems = m.bar.Done
			m.lastTick = now
		}

		// Update spinner
		spinCmd := m.bar.UpdateSpinner(msg)
		if spinCmd != nil {
			cmds = append(cmds, spinCmd)
		}

		logCmd := m.log.Update(msg)
		if logCmd != nil {
			cmds = append(cmds, logCmd)
		}

		if !m.done {
			cmds = append(cmds, tickRecover())
		}
		return m, tea.Batch(cmds...)

	case recoverDoneMsg:
		m.done = true
		m.result = msg.result
		m.err = msg.err
		m.bar.Active = false
		m.bar.Finished = msg.err == nil
		if msg.err != nil {
			m.log.Append("ERROR: "+msg.err.Error(), true)
		} else {
			m.log.Append("Recovery complete!", false)
		}
		return m, nil

	case tea.MouseMsg:
		logCmd := m.log.Update(msg)
		return m, logCmd

	case tea.KeyMsg:
		if m.done {
			switch msg.String() {
			case "q", "esc", "enter":
				return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenWelcome} }
			}
		}
	}

	logCmd := m.log.Update(msg)
	return m, logCmd
}

func (m *RecoverModel) applyEvent(ev engine.RecoverEvent) {
	m.bar.Done = ev.Done
	m.bar.Total = ev.Total
	m.bar.Active = !m.done
	if ev.Message != "" {
		m.log.Append(ev.Message, ev.IsError)
	}
}

// View implements tea.Model.
func (m RecoverModel) View() string {
	var sb strings.Builder

	sb.WriteString(recoverHeaderStyle.Render("Recovering Missing Files"))
	sb.WriteString("\n")
	sb.WriteString(statLabelStyle.Render(fmt.Sprintf("Input: %s  Output: %s", m.cfg.InputJSON, m.cfg.OutputDir)))
	sb.WriteString("\n\n")

	sb.WriteString(m.bar.View())
	sb.WriteString("\n")
	sb.WriteString(statLabelStyle.Render("Download rate"))
	sb.WriteString("\n")
	sb.WriteString(m.sparkline.View())
	sb.WriteString("\n\n")

	sb.WriteString(m.log.View())
	sb.WriteString("\n")

	// Stats bar at bottom
	sb.WriteString(m.buildStatsBar())

	return sb.String()
}

func (m RecoverModel) buildStatsBar() string {
	elapsed := time.Since(m.startTime).Round(time.Second)
	parts := []string{fmt.Sprintf("Elapsed: %s", elapsed)}

	if m.result != nil {
		r := m.result
		parts = append(parts,
			fmt.Sprintf("Downloaded: %d", r.Downloaded),
			fmt.Sprintf("Skipped: %d", r.Skipped),
			fmt.Sprintf("Failed: %d", r.Failed),
		)
		if r.FailedPath != "" {
			parts = append(parts, "Failed list: "+r.FailedPath)
		}
	} else {
		parts = append(parts,
			fmt.Sprintf("Downloaded: %d / %d", m.bar.Done, m.bar.Total),
		)
	}

	if m.done {
		parts = append(parts, "enter/q to return")
	}

	return statsBarStyle.Width(m.width).Render(strings.Join(parts, "  ·  "))
}
