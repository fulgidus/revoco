package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fulgidus/revoco/session"
)

// ── Styles ───────────────────────────────────────────────────────────────────

var (
	sessionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				MarginBottom(1)

	sessionItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	sessionSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				Background(lipgloss.Color("57"))

	sessionStatusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244"))

	sessionDateStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("238"))

	sessionActionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("63"))

	sessionDangerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	sessionPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	sessionBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(1, 2)
)

// ── Modes ────────────────────────────────────────────────────────────────────

type sessMode int

const (
	sessModeList   sessMode = iota // browsing session list
	sessModeCreate                 // typing new session name
	sessModeRename                 // typing new name for selected session
	sessModeDelete                 // confirm delete
)

// ── Action items in the bottom menu ─────────────────────────────────────────

type sessAction int

const (
	sessActionOpen sessAction = iota
	sessActionCreate
	sessActionRename
	sessActionDelete
	sessActionQuit
)

var sessActions = []struct {
	action sessAction
	label  string
	key    string
}{
	{sessActionOpen, "Open", "enter"},
	{sessActionCreate, "New", "n"},
	{sessActionRename, "Rename", "r"},
	{sessActionDelete, "Delete", "d"},
	{sessActionQuit, "Quit", "q"},
}

// ── Messages ─────────────────────────────────────────────────────────────────

type sessListLoadedMsg struct {
	sessions []*session.Session
	err      error
}

type sessCreatedMsg struct {
	session *session.Session
	err     error
}

type sessRenamedMsg struct{ err error }

type sessDeletedMsg struct{ err error }

// ── Model ────────────────────────────────────────────────────────────────────

// SessionsModel is the TUI screen for managing sessions.
type SessionsModel struct {
	sessions []*session.Session
	cursor   int
	mode     sessMode
	width    int
	height   int
	err      error

	// Text input for create / rename
	input textinput.Model

	// Loading state
	loading        bool
	loadingMessage string
	spinner        spinner.Model
}

// NewSessionsModel creates the session management screen.
func NewSessionsModel() SessionsModel {
	ti := textinput.New()
	ti.Placeholder = "Session name"
	ti.CharLimit = 64

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return SessionsModel{
		mode:           sessModeList,
		input:          ti,
		spinner:        sp,
		loading:        true,
		loadingMessage: "Loading sessions...",
	}
}

// Init implements tea.Model.
func (m SessionsModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
		loadSessions,
	)
}

func loadSessions() tea.Msg {
	sessions, err := session.ListSessions()
	return sessListLoadedMsg{sessions: sessions, err: err}
}

// Update implements tea.Model.
func (m SessionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case sessListLoadedMsg:
		m.loading = false
		m.loadingMessage = ""
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.sessions = msg.sessions
		}
		return m, nil

	case sessCreatedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.mode = sessModeList
			return m, loadSessions
		}
		// V2 sessions go directly to Dashboard - connectors are added there
		sess := msg.session
		m.mode = sessModeList
		return m, tea.Batch(
			loadSessions,
			func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenDashboard,
					Session: sess,
				}
			},
		)

	case sessRenamedMsg:
		m.loading = false
		m.loadingMessage = ""
		if msg.err != nil {
			m.err = msg.err
		}
		m.mode = sessModeList
		return m, loadSessions

	case sessDeletedMsg:
		m.loading = false
		m.loadingMessage = ""
		if msg.err != nil {
			m.err = msg.err
		}
		m.mode = sessModeList
		m.cursor = 0
		return m, loadSessions
	}

	// If loading, only handle spinner updates
	if m.loading {
		return m, nil
	}

	// Delegate based on mode
	switch m.mode {
	case sessModeList:
		return m.updateList(msg)
	case sessModeCreate:
		return m.updateCreate(msg)
	case sessModeRename:
		return m.updateRename(msg)
	case sessModeDelete:
		return m.updateDelete(msg)
	}
	return m, nil
}

// ── List mode ────────────────────────────────────────────────────────────────

func (m SessionsModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.err = nil // clear previous error on any keypress
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.sessions) > 0 {
				return m, m.openSession(m.sessions[m.cursor])
			}
		case "n":
			m.mode = sessModeCreate
			m.input.SetValue("")
			m.input.Focus()
			return m, nil
		case "r":
			if len(m.sessions) > 0 {
				m.mode = sessModeRename
				m.input.SetValue(m.sessions[m.cursor].Config.Name)
				m.input.Focus()
			}
			return m, nil
		case "d":
			if len(m.sessions) > 0 {
				m.mode = sessModeDelete
			}
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	}
	return m, nil
}

func (m SessionsModel) openSession(s *session.Session) tea.Cmd {
	sess := s
	return func() tea.Msg {
		// All sessions now go to the Dashboard (connector-based view)
		return SwitchScreenMsg{
			To:      ScreenDashboard,
			Session: sess,
		}
	}
}

// ── Create mode ──────────────────────────────────────────────────────────────

func (m SessionsModel) updateCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.mode = sessModeList
			return m, nil
		case "enter":
			name := strings.TrimSpace(m.input.Value())
			if name == "" {
				m.mode = sessModeList
				return m, nil
			}
			return m, func() tea.Msg {
				s, err := session.CreateV2(name)
				return sessCreatedMsg{session: s, err: err}
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// ── Rename mode ──────────────────────────────────────────────────────────────

func (m SessionsModel) updateRename(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.mode = sessModeList
			return m, nil
		case "enter":
			newName := strings.TrimSpace(m.input.Value())
			if newName == "" || newName == m.sessions[m.cursor].Config.Name {
				m.mode = sessModeList
				return m, nil
			}
			oldName := m.sessions[m.cursor].Config.Name
			return m, func() tea.Msg {
				err := session.Rename(oldName, newName)
				return sessRenamedMsg{err: err}
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// ── Delete mode ──────────────────────────────────────────────────────────────

func (m SessionsModel) updateDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			name := m.sessions[m.cursor].Config.Name
			m.loading = true
			m.loadingMessage = "Deleting session..."
			return m, tea.Batch(
				m.spinner.Tick,
				func() tea.Msg {
					err := session.Remove(name)
					return sessDeletedMsg{err: err}
				},
			)
		case "n", "N", "esc":
			m.mode = sessModeList
			return m, nil
		}
	}
	return m, nil
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m SessionsModel) View() string {
	// Show loading spinner for any long operation
	if m.loading {
		return m.viewLoading()
	}

	switch m.mode {
	case sessModeCreate:
		return m.viewCreate()
	case sessModeRename:
		return m.viewRename()
	case sessModeDelete:
		return m.viewDelete()
	}

	return m.viewList()
}

func (m SessionsModel) viewLoading() string {
	var sb strings.Builder
	sb.WriteString(sessionTitleStyle.Render("revoco"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Google Photos Takeout processor & recovery tool"))
	sb.WriteString("\n\n")

	msg := m.loadingMessage
	if msg == "" {
		msg = "Loading..."
	}
	sb.WriteString(m.spinner.View())
	sb.WriteString(" ")
	sb.WriteString(descStyle.Render(msg))
	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render("Please wait..."))
	return sb.String()
}

func (m SessionsModel) viewList() string {
	var sb strings.Builder

	sb.WriteString(sessionTitleStyle.Render("revoco"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Google Photos Takeout processor & recovery tool"))
	sb.WriteString("\n\n")

	if m.err != nil {
		sb.WriteString(sessionDangerStyle.Render("Error: " + m.err.Error()))
		sb.WriteString("\n\n")
	}

	if len(m.sessions) == 0 {
		sb.WriteString(descStyle.Render("No sessions yet. Press 'n' to create one."))
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(labelStyle.Render("Sessions"))
		sb.WriteString("\n\n")

		maxW := m.width - 4
		if maxW < 40 {
			maxW = 40
		}

		for i, s := range m.sessions {
			name := s.Config.Name
			status := string(s.Config.Status)
			date := s.Config.Updated.Format("2006-01-02 15:04")

			// Source info
			sourceInfo := ""
			if s.Config.Source.OriginalPath != "" {
				sourceInfo = " <- " + filepath.Base(s.Config.Source.OriginalPath)
			}

			line := fmt.Sprintf("%-30s  [%s]%s", name, status, sourceInfo)
			if len(line) > maxW {
				line = line[:maxW]
			}

			if i == m.cursor {
				// Pad for highlight
				for lipgloss.Width(line) < maxW {
					line += " "
				}
				sb.WriteString(sessionSelectedStyle.Render(line))
			} else {
				sb.WriteString(sessionItemStyle.Render(line))
			}
			sb.WriteString("\n")
			sb.WriteString(sessionDateStyle.Render(fmt.Sprintf("  Updated: %s", date)))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(m.viewActionBar())
	return sb.String()
}

func (m SessionsModel) viewActionBar() string {
	var parts []string
	for _, a := range sessActions {
		// Only show "Open/Rename/Delete" if sessions exist
		switch a.action {
		case sessActionOpen, sessActionRename, sessActionDelete:
			if len(m.sessions) == 0 {
				continue
			}
		}
		parts = append(parts, sessionActionStyle.Render(a.key)+
			helpStyle.Render(" "+a.label))
	}
	return strings.Join(parts, "  ")
}

func (m SessionsModel) viewCreate() string {
	var sb strings.Builder
	sb.WriteString(sessionTitleStyle.Render("New Session"))
	sb.WriteString("\n\n")
	sb.WriteString(sessionPromptStyle.Render("Session name:"))
	sb.WriteString("\n")
	sb.WriteString(m.input.View())
	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render("enter confirm  esc cancel"))
	return sessionBoxStyle.Width(m.width - 8).Render(sb.String())
}

func (m SessionsModel) viewRename() string {
	var sb strings.Builder
	oldName := ""
	if m.cursor < len(m.sessions) {
		oldName = m.sessions[m.cursor].Config.Name
	}
	sb.WriteString(sessionTitleStyle.Render("Rename Session"))
	sb.WriteString("\n\n")
	sb.WriteString(descStyle.Render("Current: " + oldName))
	sb.WriteString("\n")
	sb.WriteString(sessionPromptStyle.Render("New name:"))
	sb.WriteString("\n")
	sb.WriteString(m.input.View())
	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render("enter confirm  esc cancel"))
	return sessionBoxStyle.Width(m.width - 8).Render(sb.String())
}

func (m SessionsModel) viewDelete() string {
	var sb strings.Builder
	name := ""
	if m.cursor < len(m.sessions) {
		name = m.sessions[m.cursor].Config.Name
	}
	sb.WriteString(sessionDangerStyle.Render("Delete Session"))
	sb.WriteString("\n\n")
	sb.WriteString(descStyle.Render("This will permanently delete session:"))
	sb.WriteString("\n")
	sb.WriteString(sessionPromptStyle.Render("  " + name))
	sb.WriteString("\n")
	sb.WriteString(descStyle.Render("and ALL its files (output, logs, imported data)."))
	sb.WriteString("\n\n")
	sb.WriteString(sessionDangerStyle.Render("y") + helpStyle.Render(" confirm  ") +
		sessionActionStyle.Render("n/esc") + helpStyle.Render(" cancel"))
	return sessionBoxStyle.Width(m.width - 8).Render(sb.String())
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
