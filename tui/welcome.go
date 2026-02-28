package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fulgidus/revoco/cookies"
	"github.com/fulgidus/revoco/secrets"
	"github.com/fulgidus/revoco/session"
	"github.com/fulgidus/revoco/tui/components"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	// menuItemStyle / menuSelectedStyle: NO PaddingLeft — prefix string handles indent.
	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	menuSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205"))

	// descStyle: plain, no MarginTop so descriptions sit tight under items.
	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	panelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238"))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	browseButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("232")).
				Background(lipgloss.Color("63")).
				Padding(0, 1)

	selectedHighlightStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				Background(lipgloss.Color("57"))

	sessionNameStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))
)

// ── Menu items ────────────────────────────────────────────────────────────────

type menuItem struct {
	label string
	desc  string
}

var menuItems = []menuItem{
	{"Process Takeout", "Import and organise a Google Photos Takeout archive"},
	{"Recover Missing", "Download missing files using Chrome cookies"},
	{"Session Settings", "Configure output directories and options"},
	{"Back to Sessions", "Return to session list"},
	{"Quit", "Exit revoco"},
}

// ── Layout constants ──────────────────────────────────────────────────────────

// ── WelcomeModel ──────────────────────────────────────────────────────────────

// configField identifies which text input is currently focused in the config panel.
type configField int

const (
	fieldSourceDir configField = iota
	fieldDestDir
	fieldCookieJar
	fieldInputJSON
)

// WelcomeModel is the per-session menu screen with a two-panel layout.
// Left panel: clickable menu. Right panel: configuration for the selected item.
// A filepicker overlay slides in when the user clicks [Browse] or presses ctrl+o.
type WelcomeModel struct {
	session *session.Session

	cursor int
	width  int
	height int

	// Config inputs (one per required field)
	inputs  [4]textinput.Model // sourceDir, destDir, cookieJar, inputJSON
	focused configField

	// Whether the right config panel is shown (true once a non-Quit item is active)
	showRight bool

	// Filepicker overlay
	pickerOpen  bool
	pickerField configField // which field the picker will fill
	picker      components.FilePicker

	// Chrome cookie extraction
	chromePromptOpen bool            // password dialog is visible
	chromePromptStep int             // 0 = vault password, 1 = Chrome v11 password
	chromeVaultPass  string          // vault master password (held in RAM only)
	chromePassInput  textinput.Model // current password text input (masked)
	chromeStatus     string          // status message after extraction attempt
	chromeErr        string          // error message from last extraction
}

// NewWelcomeModel returns a fresh welcome screen for the given session.
func NewWelcomeModel(sess *session.Session) WelcomeModel {
	inputs := [4]textinput.Model{}
	placeholders := []string{
		"Takeout root directory",
		"Destination directory",
		"Cookie jar file (.txt Netscape format)",
		"missing-files.json path",
	}
	for i := range inputs {
		ti := textinput.New()
		ti.Placeholder = placeholders[i]
		ti.CharLimit = 512
		inputs[i] = ti
	}
	inputs[0].Focus()

	m := WelcomeModel{
		session: sess,
		inputs:  inputs,
	}

	// Chrome password input (masked)
	chromePass := textinput.New()
	chromePass.Placeholder = "Chrome v11 password (empty on most Linux)"
	chromePass.EchoMode = textinput.EchoPassword
	chromePass.EchoCharacter = '*'
	chromePass.CharLimit = 256
	m.chromePassInput = chromePass

	// Pre-fill from session config if available
	if sess != nil {
		if sess.SourcePath() != "" {
			m.inputs[fieldSourceDir].SetValue(sess.SourcePath())
		}
		if sess.OutputPath() != "" {
			m.inputs[fieldDestDir].SetValue(sess.OutputPath())
		}
		// Recovery fields
		if sess.Config.Recover.InputJSON != "" {
			m.inputs[fieldInputJSON].SetValue(sess.LogPath(sess.Config.Recover.InputJSON))
		}
	}

	return m
}

// Init implements tea.Model.
func (m WelcomeModel) Init() tea.Cmd { return textinput.Blink }

// ── Update ────────────────────────────────────────────────────────────────────

func (m WelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If chrome password prompt is open, forward all messages to it first.
	if m.chromePromptOpen {
		return m.updateChromePrompt(msg)
	}

	// If filepicker overlay is open, forward all messages to it first.
	if m.pickerOpen {
		return m.updatePicker(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case chromeExtractResultMsg:
		m.chromePromptOpen = false
		if msg.err != nil {
			m.chromeErr = msg.err.Error()
			m.chromeStatus = ""
		} else {
			m.chromeErr = ""
			m.inputs[fieldCookieJar].SetValue(msg.jarPath)
			m.chromeStatus = fmt.Sprintf("%d cookies extracted to %s", msg.count, msg.jarPath)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward to focused input
	return m.updateActiveInput(msg)
}

func (m WelcomeModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "q":
		if !m.showRight {
			return m, tea.Quit
		}
		// q while config is open goes back to menu view
		m.showRight = false
		return m, nil

	case "esc":
		if m.showRight {
			m.showRight = false
			return m, nil
		}
		// Back to sessions list
		return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenSessions} }

	case "up", "k":
		if !m.showRight {
			if m.cursor > 0 {
				m.cursor--
			}
		} else {
			m.blurAll()
			minF := m.minField()
			if int(m.focused) > minF {
				m.focused--
			}
			m.inputs[m.focused].Focus()
		}
		return m, nil

	case "down", "j":
		if !m.showRight {
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		} else {
			m.blurAll()
			maxF := m.maxField()
			if int(m.focused) < maxF {
				m.focused++
			}
			m.inputs[m.focused].Focus()
		}
		return m, nil

	case "tab":
		if m.showRight {
			m.blurAll()
			minF := m.minField()
			maxF := m.maxField()
			next := int(m.focused) + 1
			if next > maxF {
				next = minF
			}
			m.focused = configField(next)
			m.inputs[m.focused].Focus()
		}
		return m, nil

	case "ctrl+o": // open filepicker for focused field
		if m.showRight {
			cmd := m.openPicker(m.focused)
			return m, cmd
		}
		return m, nil

	case "ctrl+e": // extract Chrome cookies (recover mode only)
		if m.showRight && m.cursor == 1 {
			cmd := m.openChromePrompt()
			return m, cmd
		}
		return m, nil

	case "enter":
		if !m.showRight {
			return m.activateMenuItem()
		}
		// On the last field, advance to pre-flight
		if int(m.focused) == m.maxField() {
			return m.launchAnalyze()
		}
		// Otherwise advance to next field
		m.blurAll()
		m.focused++
		m.inputs[m.focused].Focus()
		return m, nil
	}

	return m.updateActiveInput(msg)
}

func (m WelcomeModel) activateMenuItem() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0: // Process Takeout
		m.showRight = true
		m.focused = fieldSourceDir
		m.blurAll()
		m.inputs[fieldSourceDir].Focus()
	case 1: // Recover Missing
		m.showRight = true
		m.focused = fieldCookieJar
		m.blurAll()
		m.inputs[fieldCookieJar].Focus()
	case 2: // Session Settings
		// TODO: dedicated settings screen. For now, no-op.
		return m, nil
	case 3: // Back to Sessions
		return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenSessions} }
	case 4: // Quit
		return m, tea.Quit
	}
	return m, nil
}

func (m WelcomeModel) launchAnalyze() (tea.Model, tea.Cmd) {
	source := m.inputs[fieldSourceDir].Value()
	dest := m.inputs[fieldDestDir].Value()
	cookieJar := m.inputs[fieldCookieJar].Value()
	inputJSON := m.inputs[fieldInputJSON].Value()

	// Persist choices back to session config
	if m.session != nil {
		if m.cursor == 0 {
			// Process mode: update source + output
			if source != "" {
				m.session.Config.Source.OriginalPath = source
				m.session.Config.Source.Type = session.SourceFolder
			}
			if dest != "" {
				m.session.Config.OutputDir = dest
			}
			m.session.Config.Status = session.StatusProcessing
		} else {
			// Recover mode
			m.session.Config.Status = session.StatusRecovering
		}
		_ = m.session.Save()
	}

	var mode AnalyzeMode
	if m.cursor == 0 {
		mode = AnalyzeModeProcess
	} else {
		mode = AnalyzeModeRecover
	}

	var sessionDir string
	if m.session != nil {
		sessionDir = m.session.Dir
	}

	am := NewAnalyzeModel(mode, source, dest, cookieJar, inputJSON, sessionDir, m.width, m.height)
	return m, func() tea.Msg {
		return SwitchScreenMsg{To: ScreenAnalyze, Analyze: &am}
	}
}

// updatePicker forwards events to the filepicker and handles selection.
func (m WelcomeModel) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.pickerOpen = false
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	if m.picker.Done {
		m.inputs[m.pickerField].SetValue(m.picker.Selected)
		m.pickerOpen = false
	}
	return m, cmd
}

func (m WelcomeModel) updateActiveInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.showRight {
		return m, nil
	}
	var cmd tea.Cmd
	m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
	return m, cmd
}

// openPicker opens the filepicker overlay for a given field.
func (m *WelcomeModel) openPicker(field configField) tea.Cmd {
	var mode components.PickerMode
	if field == fieldCookieJar || field == fieldInputJSON {
		mode = components.ModeFile
	} else {
		mode = components.ModeDir
	}
	m.picker = components.NewFilePicker(mode)
	m.pickerField = field
	m.pickerOpen = true
	return m.picker.Init()
}

// ── Chrome cookie extraction ────────────────────────────────────────────────

// openChromePrompt opens the password dialog for Chrome cookie extraction.
// Step 0: vault master password, Step 1: Chrome v11 decryption password.
func (m *WelcomeModel) openChromePrompt() tea.Cmd {
	m.chromePromptOpen = true
	m.chromePromptStep = 0
	m.chromeVaultPass = ""
	m.chromeStatus = ""
	m.chromeErr = ""
	m.chromePassInput.SetValue("")
	m.chromePassInput.Placeholder = "Vault master password"
	m.chromePassInput.Focus()
	return textinput.Blink
}

// updateChromePrompt handles events while the Chrome password dialog is open.
func (m WelcomeModel) updateChromePrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.chromePromptOpen = false
			m.chromeVaultPass = ""
			return m, nil
		case "enter":
			if m.chromePromptStep == 0 {
				// Step 0 complete: capture vault password, advance to step 1
				m.chromeVaultPass = m.chromePassInput.Value()
				m.chromePromptStep = 1
				m.chromePassInput.SetValue("")
				m.chromePassInput.Placeholder = "Chrome v11 password (empty on most Linux)"
				m.chromePassInput.Focus()
				return m, textinput.Blink
			}
			// Step 1 complete: extract cookies
			return m.extractChromeCookies()
		}
	}
	var cmd tea.Cmd
	m.chromePassInput, cmd = m.chromePassInput.Update(msg)
	return m, cmd
}

// chromeExtractResultMsg carries the result of background Chrome extraction.
type chromeExtractResultMsg struct {
	jarPath string
	count   int
	err     error
}

// extractChromeCookies runs the Chrome cookie extraction.
func (m WelcomeModel) extractChromeCookies() (tea.Model, tea.Cmd) {
	chromePassword := m.chromePassInput.Value()
	vaultPassword := m.chromeVaultPass
	m.chromeStatus = "Extracting cookies..."
	m.chromeErr = ""

	// We need a session dir to write the jar into
	var jarPath string
	if m.session != nil {
		jarPath = filepath.Join(m.session.Dir, "cookies.txt")
	} else {
		jarPath = "cookies.txt"
	}

	// Store Chrome v11 password in vault (encrypted with vault master password)
	vaultPath, vaultErr := secrets.DefaultPath()
	if vaultErr == nil && vaultPassword != "" {
		_ = secrets.Store(vaultPath, vaultPassword, "chrome_v11_password", chromePassword)
	}

	// Clear vault password from memory
	m.chromeVaultPass = ""

	// Run extraction in a goroutine to avoid blocking TUI
	return m, func() tea.Msg {
		dbPath, err := cookies.DefaultChromeDBPath()
		if err != nil {
			return chromeExtractResultMsg{err: err}
		}
		count, err := cookies.ExtractToJar(dbPath, chromePassword, jarPath)
		if err != nil {
			return chromeExtractResultMsg{err: err}
		}
		return chromeExtractResultMsg{jarPath: jarPath, count: count}
	}
}

// blurAll removes focus from all inputs.
func (m *WelcomeModel) blurAll() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
}

// minField returns the index of the first visible config field for the active menu item.
func (m WelcomeModel) minField() int {
	if m.cursor == 0 {
		return int(fieldSourceDir)
	}
	return int(fieldCookieJar) // Recover mode: skip sourceDir and destDir
}

// maxField returns the index of the last visible config field for the active menu item.
func (m WelcomeModel) maxField() int {
	if m.cursor == 0 {
		return int(fieldDestDir)
	}
	return int(fieldInputJSON)
}

// leftWidth returns the *outer* width (including border) of the left menu panel.
func (m WelcomeModel) leftWidth() int {
	w := m.width * 40 / 100
	if w < 26 {
		w = 26
	}
	return w
}

// useTwoPanel returns true when the terminal is wide enough for a two-panel layout.
func (m WelcomeModel) useTwoPanel() bool {
	return m.width >= 60
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m WelcomeModel) View() string {
	if m.chromePromptOpen {
		return m.viewChromePrompt()
	}
	if m.pickerOpen {
		return m.viewPicker()
	}
	if m.useTwoPanel() {
		return m.viewTwoPanel()
	}
	return m.viewSinglePanel()
}

func (m WelcomeModel) viewPicker() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Select path"))
	sb.WriteString("\n\n")
	sb.WriteString(m.picker.View())
	return sb.String()
}

func (m WelcomeModel) viewChromePrompt() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Extract Chrome Cookies"))
	sb.WriteString("\n\n")

	if m.chromePromptStep == 0 {
		sb.WriteString(descStyle.Render("Step 1/2: Enter your vault master password."))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("This encrypts secrets stored on disk."))
		sb.WriteString("\n\n")
		sb.WriteString(labelStyle.Render("Vault password:"))
	} else {
		sb.WriteString(descStyle.Render("Step 2/2: Enter your Chrome v11 decryption password."))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("On most Linux systems this is empty — just press Enter."))
		sb.WriteString("\n\n")
		sb.WriteString(labelStyle.Render("Chrome password:"))
	}

	sb.WriteString("\n")
	sb.WriteString(m.chromePassInput.View())
	sb.WriteString("\n\n")
	if m.chromeStatus != "" {
		sb.WriteString(statValueStyle.Render(m.chromeStatus))
		sb.WriteString("\n")
	}
	sb.WriteString(helpStyle.Render("enter continue • esc cancel"))
	return sb.String()
}

func (m WelcomeModel) sessionLabel() string {
	if m.session != nil {
		return m.session.Config.Name
	}
	return "(no session)"
}

func (m WelcomeModel) viewSinglePanel() string {
	var sb strings.Builder

	// When showRight is true in single-panel mode, show the config panel instead of menu
	if m.showRight {
		sb.WriteString(titleStyle.Render("revoco"))
		sb.WriteString("\n")
		sb.WriteString(sessionNameStyle.Render("Session: " + m.sessionLabel()))
		sb.WriteString("\n\n")
		sb.WriteString(m.viewConfigPanel(m.width - 4))
		return sb.String()
	}

	sb.WriteString(titleStyle.Render("revoco"))
	sb.WriteString("\n")
	sb.WriteString(sessionNameStyle.Render("Session: " + m.sessionLabel()))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Google Photos Takeout processor & recovery tool"))
	sb.WriteString("\n\n")

	for i, item := range menuItems {
		prefix := "  "
		style := menuItemStyle
		if i == m.cursor {
			prefix = "▶ "
			style = menuSelectedStyle
		}
		sb.WriteString(style.Render(prefix + item.label))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("  " + item.desc))
		sb.WriteString("\n\n")
	}
	sb.WriteString(helpStyle.Render("↑/↓ navigate • enter select • esc sessions • q quit"))
	return sb.String()
}

func (m WelcomeModel) viewTwoPanel() string {
	// Outer widths (including borders).
	leftOuterW := m.leftWidth()
	rightOuterW := m.width - leftOuterW - 1 // 1-char gap between panels
	if rightOuterW < 22 {
		rightOuterW = 22
	}

	// Inner content widths (subtract 2 for left+right border chars).
	leftInnerW := leftOuterW - 2
	rightInnerW := rightOuterW - 2

	panelH := m.height - 2
	if panelH < 10 {
		panelH = 10
	}

	// ── Left panel: menu ──────────────────────────────────────────────────────
	leftContent := m.viewMenuPanel(leftInnerW)
	leftPanel := panelBorderStyle.
		Width(leftInnerW).
		Height(panelH).
		Render(leftContent)

	// ── Right panel: configuration ────────────────────────────────────────────
	rightContent := m.viewConfigPanel(rightInnerW)
	rightPanel := panelBorderStyle.
		Width(rightInnerW).
		Height(panelH).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)
}

// viewMenuPanel renders the left-panel interior content.
func (m WelcomeModel) viewMenuPanel(innerW int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("revoco"))
	sb.WriteString("\n")
	sb.WriteString(sessionNameStyle.Render("Session: " + m.sessionLabel()))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("v0.1.0"))
	sb.WriteString("\n\n")

	for i, item := range menuItems {
		if i == m.cursor {
			// Full-width highlight bar for the selected item.
			label := "▶ " + item.label
			// Pad to innerW so the background stretches.
			for lipgloss.Width(label) < innerW {
				label += " "
			}
			sb.WriteString(selectedHighlightStyle.Render(label))
		} else {
			sb.WriteString(menuItemStyle.Render("  " + item.label))
		}
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("  " + item.desc))
		sb.WriteString("\n\n")
	}
	sb.WriteString(helpStyle.Render("↑/↓ • enter • esc • q"))
	return sb.String()
}

func (m WelcomeModel) viewConfigPanel(w int) string {
	var sb strings.Builder

	if !m.showRight {
		// Placeholder before any item is selected.
		sb.WriteString(subtitleStyle.Render("Select an action"))
		sb.WriteString("\n\n")
		sb.WriteString(descStyle.Render("Use ↑/↓ to navigate the menu,"))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("then press enter or click to configure."))

		// Show session source info if available
		if m.session != nil && m.session.Config.Source.OriginalPath != "" {
			sb.WriteString("\n\n")
			sb.WriteString(labelStyle.Render("Source:"))
			sb.WriteString("\n")
			sb.WriteString(descStyle.Render("  " + m.session.Config.Source.OriginalPath))
			sb.WriteString("\n")
			sb.WriteString(labelStyle.Render("Status: "))
			sb.WriteString(descStyle.Render(string(m.session.Config.Status)))
		}
		return sb.String()
	}

	switch m.cursor {
	case 0: // Process Takeout
		sb.WriteString(subtitleStyle.Render("Process Takeout"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderField(fieldSourceDir, "Source directory", w))
		sb.WriteString("\n")
		sb.WriteString(m.renderField(fieldDestDir, "Destination directory", w))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("tab/↑↓ navigate • ctrl+o browse • enter proceed"))

	case 1: // Recover Missing
		sb.WriteString(subtitleStyle.Render("Recover Missing Files"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderField(fieldCookieJar, "Cookie jar path", w))
		sb.WriteString("  ")
		sb.WriteString(browseButtonStyle.Render("[Extract from Chrome]"))
		sb.WriteString("\n")
		if m.chromeStatus != "" {
			sb.WriteString(statValueStyle.Render("  " + m.chromeStatus))
			sb.WriteString("\n")
		}
		if m.chromeErr != "" {
			sb.WriteString(errorMsgStyle.Render("  " + m.chromeErr))
			sb.WriteString("\n")
		}
		sb.WriteString(m.renderField(fieldInputJSON, "missing-files.json path", w))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("tab/↑↓ navigate • ctrl+o browse • ctrl+e chrome • enter proceed"))

	case 2: // Session Settings
		sb.WriteString(subtitleStyle.Render("Session Settings"))
		sb.WriteString("\n\n")
		if m.session != nil {
			sb.WriteString(labelStyle.Render("Name: "))
			sb.WriteString(descStyle.Render(m.session.Config.Name))
			sb.WriteString("\n")
			sb.WriteString(labelStyle.Render("Created: "))
			sb.WriteString(descStyle.Render(m.session.Config.Created.Format("2006-01-02 15:04")))
			sb.WriteString("\n")
			sb.WriteString(labelStyle.Render("Status: "))
			sb.WriteString(descStyle.Render(string(m.session.Config.Status)))
			sb.WriteString("\n")
			sb.WriteString(labelStyle.Render("Source: "))
			sb.WriteString(descStyle.Render(m.session.Config.Source.OriginalPath))
			sb.WriteString("\n")
			sb.WriteString(labelStyle.Render("Output: "))
			sb.WriteString(descStyle.Render(m.session.OutputPath()))
			sb.WriteString("\n")
			sb.WriteString(labelStyle.Render("Session dir: "))
			sb.WriteString(descStyle.Render(m.session.Dir))
			sb.WriteString("\n\n")

			// Secrets vault status
			vaultPath, vaultErr := secrets.DefaultPath()
			if vaultErr == nil && secrets.Exists(vaultPath) {
				sb.WriteString(labelStyle.Render("Secrets vault: "))
				sb.WriteString(statValueStyle.Render("configured"))
			} else {
				sb.WriteString(labelStyle.Render("Secrets vault: "))
				sb.WriteString(descStyle.Render("not set up"))
			}
		}
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("(settings editing coming soon)"))

	case 3: // Back to Sessions
		sb.WriteString(subtitleStyle.Render("Back"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("Press enter to return to the session list."))

	case 4: // Quit
		sb.WriteString(subtitleStyle.Render("Quit"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("Press enter to exit revoco."))
	}

	return sb.String()
}

func (m WelcomeModel) renderField(field configField, label string, _ int) string {
	var sb strings.Builder
	sb.WriteString(labelStyle.Render(label))
	sb.WriteString("\n")
	sb.WriteString(m.inputs[field].View())
	if field == m.focused {
		sb.WriteString("  ")
		sb.WriteString(browseButtonStyle.Render("[Browse]"))
	}
	sb.WriteString("\n")
	return sb.String()
}
