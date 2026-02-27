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

// recoverEventMsg wraps a RecoverEvent.
type recoverEventMsg engine.RecoverEvent

// recoverDoneMsg signals recovery completed.
type recoverDoneMsg struct {
	result *engine.RecoverResult
	err    error
}

var (
	recoverHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).MarginBottom(1)
)

// RecoverModel is the TUI screen for a recovery run.
type RecoverModel struct {
	cfg       engine.RecoverConfig
	eventsCh  chan engine.RecoverEvent
	bar       components.PhaseBar
	log       components.LogPanel
	result    *engine.RecoverResult
	err       error
	done      bool
	width     int
	height    int
	startTime time.Time
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
	return RecoverModel{
		cfg:      cfg,
		eventsCh: ch,
		bar: components.PhaseBar{
			Label: "Downloading",
			Width: width - 4,
		},
		log:    components.NewLogPanel(width-4, 8, 200),
		width:  width,
		height: height,
	}
}

// Init implements tea.Model — starts the recovery pipeline and a tick.
func (m RecoverModel) Init() tea.Cmd {
	m.startTime = time.Now()
	cfg := m.cfg
	ch := m.eventsCh
	return tea.Batch(
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
		m.log.Width = msg.Width - 4
		m.bar.Width = msg.Width - 4
		return m, nil

	case recoverTickMsg:
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
			return m, tickRecover()
		}
		return m, nil

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

	case tea.KeyMsg:
		if m.done {
			switch msg.String() {
			case "q", "esc", "enter":
				return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenWelcome} }
			}
		}
	}
	return m, nil
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
	sb.WriteString("\n\n")

	if m.err != nil {
		sb.WriteString(errorMsgStyle.Render("Error: " + m.err.Error()))
		sb.WriteString("\n\n")
	} else if m.done && m.result != nil {
		r := m.result
		sb.WriteString(statValueStyle.Render(fmt.Sprintf(
			"Downloaded: %d  Skipped: %d  Failed: %d",
			r.Downloaded, r.Skipped, r.Failed,
		)))
		sb.WriteString("\n")
		elapsed := time.Since(m.startTime).Round(time.Second)
		sb.WriteString(statLabelStyle.Render(fmt.Sprintf("Elapsed: %s", elapsed)))
		if r.FailedPath != "" {
			sb.WriteString("\n")
			sb.WriteString(statLabelStyle.Render("Failed list: " + r.FailedPath))
		}
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("enter/q to return to menu"))
	}

	sb.WriteString(m.log.View())
	return sb.String()
}
