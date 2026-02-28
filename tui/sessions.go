package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fulgidus/revoco/session"
	"github.com/fulgidus/revoco/tui/components"
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
	sessModeList       sessMode = iota // browsing session list
	sessModeCreate                     // typing new session name
	sessModeRename                     // typing new name for selected session
	sessModeDelete                     // confirm delete
	sessModeImport                     // choose import type
	sessModeImportPick                 // filepicker for import (files)
)

// ── Action items in the bottom menu ─────────────────────────────────────────

type sessAction int

const (
	sessActionOpen sessAction = iota
	sessActionCreate
	sessActionRename
	sessActionImport
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
	{sessActionImport, "Import", "i"},
	{sessActionDelete, "Delete", "d"},
	{sessActionQuit, "Quit", "q"},
}

// ── Import choices ──────────────────────────────────────────────────────────

type importChoice int

const (
	importFolder importChoice = iota
	importZip
	importTGZ
	importCancel
)

var importChoices = []struct {
	choice importChoice
	label  string
}{
	{importFolder, "Import from folder"},
	{importZip, "Import from .zip"},
	{importTGZ, "Import from .tgz / .tar.gz"},
	{importCancel, "Cancel"},
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

type sessImportDoneMsg struct{ err error }

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

	// Import sub-mode
	importCursor int

	// Filepicker for import
	pickerOpen bool
	picker     components.FilePicker
	importType importChoice

	// Selected archive files (for multi-select)
	selectedFiles []string

	// After creating a session, we must import before opening it
	pendingOpen    bool
	pendingSession *session.Session

	// Loading state
	loading bool
}

// NewSessionsModel creates the session management screen.
func NewSessionsModel() SessionsModel {
	ti := textinput.New()
	ti.Placeholder = "Session name"
	ti.CharLimit = 64
	return SessionsModel{
		mode:    sessModeList,
		input:   ti,
		loading: true,
	}
}

// Init implements tea.Model.
func (m SessionsModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
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

	case sessListLoadedMsg:
		m.loading = false
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
		// After creation, require import before opening
		m.pendingOpen = true
		m.pendingSession = msg.session
		m.mode = sessModeImport
		m.importCursor = 0
		// Refresh list so new session appears
		return m, loadSessions

	case sessRenamedMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.mode = sessModeList
		return m, loadSessions

	case sessDeletedMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.mode = sessModeList
		m.cursor = 0
		return m, loadSessions

	case sessImportDoneMsg:
		m.pickerOpen = false
		if msg.err != nil {
			m.err = msg.err
			m.mode = sessModeList
			m.pendingOpen = false
			m.pendingSession = nil
			return m, loadSessions
		}
		// If we just created a session and import succeeded, open it
		if m.pendingOpen && m.pendingSession != nil {
			sess := m.pendingSession
			m.pendingOpen = false
			m.pendingSession = nil
			m.mode = sessModeList
			return m, m.openSession(sess)
		}
		m.mode = sessModeList
		return m, loadSessions
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
	case sessModeImport:
		return m.updateImportMenu(msg)
	case sessModeImportPick:
		return m.updateImportPick(msg)
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
		case "i":
			if len(m.sessions) > 0 {
				m.mode = sessModeImport
				m.importCursor = 0
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
		return SwitchScreenMsg{
			To:      ScreenWelcome,
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
				s, err := session.Create(name)
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
			return m, func() tea.Msg {
				err := session.Remove(name)
				return sessDeletedMsg{err: err}
			}
		case "n", "N", "esc":
			m.mode = sessModeList
			return m, nil
		}
	}
	return m, nil
}

// ── Import menu ──────────────────────────────────────────────────────────────

func (m SessionsModel) updateImportMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.importCursor > 0 {
				m.importCursor--
			}
		case "down", "j":
			if m.importCursor < len(importChoices)-1 {
				m.importCursor++
			}
		case "esc":
			m.mode = sessModeList
			m.pendingOpen = false
			m.pendingSession = nil
			return m, nil
		case "enter":
			choice := importChoices[m.importCursor].choice
			if choice == importCancel {
				m.mode = sessModeList
				m.pendingOpen = false
				m.pendingSession = nil
				return m, nil
			}
			m.importType = choice
			m.mode = sessModeImportPick
			m.selectedFiles = nil // Reset selected files

			var pickerMode components.PickerMode
			switch choice {
			case importFolder:
				pickerMode = components.ModeDir
			case importZip:
				pickerMode = components.ModeMultiFile
				m.picker = components.NewFilePicker(pickerMode)
				m.picker.AllowedExts = []string{".zip"}
			case importTGZ:
				pickerMode = components.ModeMultiFile
				m.picker = components.NewFilePicker(pickerMode)
				m.picker.AllowedExts = []string{".tgz", ".tar.gz"}
			default:
				pickerMode = components.ModeFile
				m.picker = components.NewFilePicker(pickerMode)
			}
			if choice == importFolder {
				m.picker = components.NewFilePicker(pickerMode)
			}
			m.pickerOpen = true
			return m, m.picker.Init()
		}
	}
	return m, nil
}

// ── Import filepicker ────────────────────────────────────────────────────────

func (m SessionsModel) updateImportPick(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.mode = sessModeImport
			m.pickerOpen = false
			m.selectedFiles = nil
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)

	if m.picker.Done {
		m.pickerOpen = false

		// Determine target session
		var sess *session.Session
		if m.pendingOpen && m.pendingSession != nil {
			sess = m.pendingSession
		} else if len(m.sessions) > 0 && m.cursor < len(m.sessions) {
			sess = m.sessions[m.cursor]
		}
		if sess == nil {
			m.mode = sessModeList
			return m, nil
		}

		// Handle based on import type
		switch m.importType {
		case importFolder:
			// Single folder selection - import directly
			selected := m.picker.Selected
			return m, func() tea.Msg {
				err := sess.ImportFolder(selected)
				return sessImportDoneMsg{err: err}
			}

		case importZip:
			// Multi-file selection - extract to session source folder
			selectedFiles := m.picker.MultiSelected
			if len(selectedFiles) == 0 {
				m.mode = sessModeImport
				return m, nil
			}
			return m, func() tea.Msg {
				err := sess.ImportZipMulti(selectedFiles, "")
				return sessImportDoneMsg{err: err}
			}

		case importTGZ:
			// Multi-file selection - extract to session source folder
			selectedFiles := m.picker.MultiSelected
			if len(selectedFiles) == 0 {
				m.mode = sessModeImport
				return m, nil
			}
			return m, func() tea.Msg {
				err := sess.ImportTGZMulti(selectedFiles, "")
				return sessImportDoneMsg{err: err}
			}
		}
	}
	return m, cmd
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m SessionsModel) View() string {
	if m.pickerOpen {
		return m.viewPicker()
	}

	switch m.mode {
	case sessModeCreate:
		return m.viewCreate()
	case sessModeRename:
		return m.viewRename()
	case sessModeDelete:
		return m.viewDelete()
	case sessModeImport:
		return m.viewImportMenu()
	}

	return m.viewList()
}

func (m SessionsModel) viewList() string {
	var sb strings.Builder

	sb.WriteString(sessionTitleStyle.Render("revoco"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Google Photos Takeout processor & recovery tool"))
	sb.WriteString("\n\n")

	if m.loading {
		sb.WriteString(descStyle.Render("Loading sessions..."))
		return sb.String()
	}

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
		// Only show "Open/Rename/Import/Delete" if sessions exist
		switch a.action {
		case sessActionOpen, sessActionRename, sessActionImport, sessActionDelete:
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

func (m SessionsModel) viewImportMenu() string {
	var sb strings.Builder
	name := ""
	if m.cursor < len(m.sessions) {
		name = m.sessions[m.cursor].Config.Name
	}
	sb.WriteString(sessionTitleStyle.Render("Import Takeout"))
	sb.WriteString("\n")
	sb.WriteString(descStyle.Render("Into session: " + name))
	sb.WriteString("\n\n")

	for i, c := range importChoices {
		prefix := "  "
		style := sessionItemStyle
		if i == m.importCursor {
			prefix = "> "
			style = sessionSelectedStyle
		}
		sb.WriteString(style.Render(prefix + c.label))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("up/down navigate  enter select  esc cancel"))
	return sessionBoxStyle.Width(m.width - 8).Render(sb.String())
}

func (m SessionsModel) viewPicker() string {
	var sb strings.Builder

	// Show different titles based on mode
	switch m.mode {
	case sessModeImportPick:
		switch m.importType {
		case importZip:
			sb.WriteString(titleStyle.Render("Select .zip archive(s)"))
			sb.WriteString("\n")
			sb.WriteString(descStyle.Render("Space to toggle, Enter to confirm"))
		case importTGZ:
			sb.WriteString(titleStyle.Render("Select .tgz archive(s)"))
			sb.WriteString("\n")
			sb.WriteString(descStyle.Render("Space to toggle, Enter to confirm"))
		default:
			sb.WriteString(titleStyle.Render("Select path"))
		}
	default:
		sb.WriteString(titleStyle.Render("Select path"))
	}

	sb.WriteString("\n\n")
	sb.WriteString(m.picker.View())
	return sb.String()
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
