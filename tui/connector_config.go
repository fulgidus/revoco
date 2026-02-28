package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/session"
)

// ── Messages ─────────────────────────────────────────────────────────────────

type configConnSavedMsg struct {
	err error
}

type configConnAuthMsg struct {
	success bool
	message string
}

// ── Model ────────────────────────────────────────────────────────────────────

// ConfigConnectorModel is the screen for editing a connector's settings.
type ConfigConnectorModel struct {
	session    *session.Session
	instanceID string
	config     core.ConnectorConfig
	width      int
	height     int
	err        error

	// Editable fields
	nameInput    textinput.Model
	settingFocus int // Which setting is focused (-1 = name, -2 = actions)
	settings     []settingField

	// OAuth connectors
	isOAuth      bool
	authStatus   string // "not_authenticated", "authenticated", "checking"
	actionCursor int    // Which action is selected (0 = Authenticate, 1 = Test)

	// Loading
	loading        bool
	loadingMessage string
	spinner        spinner.Model
}

type settingField struct {
	key         string
	label       string
	value       string
	input       textinput.Model
	description string
	sensitive   bool   // If true, mask the value when not focused
	fieldType   string // "string", "password", "bool", "select"
}

// NewConfigConnectorModel creates the connector configuration screen.
func NewConfigConnectorModel(sess *session.Session, instanceID string) ConfigConnectorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	nameIn := textinput.New()
	nameIn.Placeholder = "Connector name"
	nameIn.CharLimit = 64

	m := ConfigConnectorModel{
		session:      sess,
		instanceID:   instanceID,
		spinner:      sp,
		nameInput:    nameIn,
		settingFocus: -1, // Start on name field
		authStatus:   "not_authenticated",
	}

	// Load existing config
	if cfg, ok := sess.GetConnector(instanceID); ok {
		m.config = cfg
		m.nameInput.SetValue(cfg.Name)
		m.loadSettings()

		// Check if this is an OAuth connector
		connInfo, ok := core.GetConnectorInfo(cfg.ConnectorID)
		if ok && connInfo.RequiresAuth && connInfo.AuthType == "oauth2" {
			m.isOAuth = true
		}
	}

	m.nameInput.Focus()
	return m
}

func (m *ConfigConnectorModel) loadSettings() {
	// Get connector info to understand what settings are available
	connInfo, ok := core.GetConnectorInfo(m.config.ConnectorID)
	if !ok {
		return
	}

	conn := connInfo.Factory()
	schema := conn.ConfigSchema()

	for _, opt := range schema {
		input := textinput.New()
		input.Placeholder = opt.Name
		input.CharLimit = 256

		// For password fields, use password echo mode
		if opt.Type == "password" || opt.Sensitive {
			input.EchoMode = textinput.EchoPassword
		}

		// Get current value
		val := ""
		if v, ok := m.config.Settings[opt.ID]; ok {
			val = fmt.Sprintf("%v", v)
		} else if opt.Default != nil {
			val = fmt.Sprintf("%v", opt.Default)
		}
		input.SetValue(val)

		m.settings = append(m.settings, settingField{
			key:         opt.ID,
			label:       opt.Name,
			value:       val,
			input:       input,
			description: opt.Description,
			sensitive:   opt.Sensitive || opt.Type == "password",
			fieldType:   opt.Type,
		})
	}
}

// Init implements tea.Model.
func (m ConfigConnectorModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
	)
}

// Update implements tea.Model.
func (m ConfigConnectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case configConnSavedMsg:
		m.loading = false
		m.loadingMessage = ""
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		return m, func() tea.Msg {
			return SwitchScreenMsg{
				To:      ScreenDashboard,
				Session: m.session,
			}
		}

	case configConnAuthMsg:
		m.loading = false
		m.loadingMessage = ""
		if msg.success {
			m.authStatus = "authenticated"
		} else {
			m.err = fmt.Errorf("%s", msg.message)
			m.authStatus = "not_authenticated"
		}
		return m, nil
	}

	if m.loading {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.err = nil

		// Handle actions section navigation
		if m.settingFocus == -2 {
			switch msg.String() {
			case "up", "k":
				if m.actionCursor > 0 {
					m.actionCursor--
				}
			case "down", "j":
				if m.actionCursor < 1 { // 2 actions: Authenticate, Test
					m.actionCursor++
				}
			case "enter", " ":
				return m.executeAction()
			case "tab":
				// Go back to name
				m.settingFocus = -1
				m.nameInput.Focus()
				return m, nil
			case "shift+tab":
				// Go to last setting
				if len(m.settings) > 0 {
					m.settingFocus = len(m.settings) - 1
					m.settings[m.settingFocus].input.Focus()
				} else {
					m.settingFocus = -1
					m.nameInput.Focus()
				}
				return m, nil
			case "esc":
				return m, func() tea.Msg {
					return SwitchScreenMsg{
						To:      ScreenDashboard,
						Session: m.session,
					}
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "tab":
			// Cycle through fields
			m.nameInput.Blur()
			for i := range m.settings {
				m.settings[i].input.Blur()
			}

			if m.settingFocus == -1 {
				// Currently on name, go to first setting
				if len(m.settings) > 0 {
					m.settingFocus = 0
					m.settings[0].input.Focus()
				} else if m.isOAuth {
					// No settings, go to actions
					m.settingFocus = -2
				}
			} else if m.settingFocus < len(m.settings)-1 {
				m.settingFocus++
				m.settings[m.settingFocus].input.Focus()
			} else if m.isOAuth {
				// After last setting, go to actions
				m.settingFocus = -2
			} else {
				// Back to name
				m.settingFocus = -1
				m.nameInput.Focus()
			}
			return m, nil

		case "shift+tab":
			// Reverse cycle
			m.nameInput.Blur()
			for i := range m.settings {
				m.settings[i].input.Blur()
			}

			if m.settingFocus == -1 {
				// On name, go to actions or last setting
				if m.isOAuth {
					m.settingFocus = -2
				} else if len(m.settings) > 0 {
					m.settingFocus = len(m.settings) - 1
					m.settings[m.settingFocus].input.Focus()
				}
			} else if m.settingFocus <= 0 {
				m.settingFocus = -1
				m.nameInput.Focus()
			} else {
				m.settingFocus--
				m.settings[m.settingFocus].input.Focus()
			}
			return m, nil

		case "ctrl+s":
			return m.save()

		case "esc":
			return m, func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenDashboard,
					Session: m.session,
				}
			}
		}
	}

	// Update focused input
	var cmd tea.Cmd
	if m.settingFocus == -1 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else if m.settingFocus >= 0 && m.settingFocus < len(m.settings) {
		m.settings[m.settingFocus].input, cmd = m.settings[m.settingFocus].input.Update(msg)
	}
	return m, cmd
}

func (m ConfigConnectorModel) executeAction() (tea.Model, tea.Cmd) {
	switch m.actionCursor {
	case 0: // Authenticate
		m.loading = true
		m.loadingMessage = "Authenticating... (check your browser)"
		cfg := m.config

		// First save any changes to the settings
		if m.config.Settings == nil {
			m.config.Settings = make(map[string]any)
		}
		for _, sf := range m.settings {
			val := strings.TrimSpace(sf.input.Value())
			if val != "" {
				m.config.Settings[sf.key] = val
			}
		}

		return m, func() tea.Msg {
			return runConnectorAuth(cfg)
		}

	case 1: // Test Connection
		m.loading = true
		m.loadingMessage = "Testing connection..."
		cfg := m.config
		return m, func() tea.Msg {
			result := runConfigConnectorTest(cfg)
			return configConnAuthMsg{
				success: result.success,
				message: result.message,
			}
		}
	}
	return m, nil
}

// runConnectorAuth triggers the OAuth flow for a connector.
func runConnectorAuth(cfg core.ConnectorConfig) configConnAuthMsg {
	connInfo, ok := core.GetConnectorInfo(cfg.ConnectorID)
	if !ok {
		return configConnAuthMsg{
			success: false,
			message: "Unknown connector type",
		}
	}

	conn := connInfo.Factory()

	// Check if it supports testing (which triggers auth)
	tester, ok := conn.(core.ConnectorTester)
	if !ok {
		return configConnAuthMsg{
			success: false,
			message: "Connector does not support authentication",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	err := tester.TestConnection(ctx, cfg)
	if err != nil {
		return configConnAuthMsg{
			success: false,
			message: err.Error(),
		}
	}

	return configConnAuthMsg{
		success: true,
		message: "Authentication successful!",
	}
}

// runConfigConnectorTest tests the connector without triggering new auth.
func runConfigConnectorTest(cfg core.ConnectorConfig) configConnAuthMsg {
	connInfo, ok := core.GetConnectorInfo(cfg.ConnectorID)
	if !ok {
		return configConnAuthMsg{
			success: false,
			message: "Unknown connector type",
		}
	}

	conn := connInfo.Factory()

	tester, ok := conn.(core.ConnectorTester)
	if !ok {
		return configConnAuthMsg{
			success: false,
			message: "Connector does not support testing",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := tester.TestConnection(ctx, cfg)
	if err != nil {
		return configConnAuthMsg{
			success: false,
			message: err.Error(),
		}
	}

	return configConnAuthMsg{
		success: true,
		message: "Connection test passed!",
	}
}

func (m ConfigConnectorModel) save() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		m.err = fmt.Errorf("name is required")
		return m, nil
	}

	m.config.Name = name

	// Update settings
	if m.config.Settings == nil {
		m.config.Settings = make(map[string]any)
	}
	for _, sf := range m.settings {
		val := strings.TrimSpace(sf.input.Value())
		if val != "" {
			m.config.Settings[sf.key] = val
		}
	}

	m.loading = true
	cfg := m.config
	sess := m.session
	return m, func() tea.Msg {
		sess.UpdateConnector(cfg)
		err := sess.Save()
		return configConnSavedMsg{err: err}
	}
}

// View implements tea.Model.
func (m ConfigConnectorModel) View() string {
	if m.loading {
		var sb strings.Builder
		sb.WriteString(titleStyle.Render("Configure Connector"))
		sb.WriteString("\n\n")
		sb.WriteString(m.spinner.View())
		sb.WriteString(" ")
		msg := "Saving..."
		if m.loadingMessage != "" {
			msg = m.loadingMessage
		}
		sb.WriteString(descStyle.Render(msg))
		return sb.String()
	}

	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Configure Connector"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render(m.config.ConnectorID))
	sb.WriteString("\n\n")

	if m.err != nil {
		sb.WriteString(dangerStyle.Render("Error: " + m.err.Error()))
		sb.WriteString("\n\n")
	}

	// Show auth status for OAuth connectors
	if m.isOAuth {
		statusLabel := "Auth Status: "
		switch m.authStatus {
		case "authenticated":
			sb.WriteString(promptStyle.Render(statusLabel))
			sb.WriteString(successStyle.Render("Authenticated"))
		case "checking":
			sb.WriteString(promptStyle.Render(statusLabel))
			sb.WriteString(warningStyle.Render("Checking..."))
		default:
			sb.WriteString(promptStyle.Render(statusLabel))
			sb.WriteString(warningStyle.Render("Not Authenticated"))
		}
		sb.WriteString("\n\n")
	}

	// Name field
	nameLabel := "Name:"
	if m.settingFocus == -1 {
		nameLabel = "> Name:"
	}
	sb.WriteString(promptStyle.Render(nameLabel))
	sb.WriteString("\n")
	sb.WriteString(m.nameInput.View())
	sb.WriteString("\n\n")

	// Role (read-only)
	sb.WriteString(promptStyle.Render("Roles: "))
	sb.WriteString(m.config.Roles.String())
	sb.WriteString("\n\n")

	// Dynamic settings
	if len(m.settings) > 0 {
		sb.WriteString(labelStyle.Render("Settings"))
		sb.WriteString("\n\n")

		for i, sf := range m.settings {
			sb.WriteString(promptStyle.Render(sf.label + ":"))
			if sf.description != "" {
				sb.WriteString(" ")
				sb.WriteString(helpStyle.Render("(" + sf.description + ")"))
			}
			sb.WriteString("\n")

			if i == m.settingFocus {
				// Focused - show the input (password fields already handle masking)
				sb.WriteString(sf.input.View())
			} else {
				// Not focused - mask sensitive values
				displayVal := sf.input.Value()
				if sf.sensitive && displayVal != "" {
					displayVal = maskConfigValue(displayVal)
				}
				sb.WriteString(descStyle.Render(displayVal))
			}
			sb.WriteString("\n\n")
		}
	}

	// OAuth Actions section
	if m.isOAuth {
		sb.WriteString(labelStyle.Render("Actions"))
		sb.WriteString("\n\n")

		actions := []struct {
			name string
			desc string
		}{
			{"Authenticate", "Open browser to authorize access"},
			{"Test Connection", "Verify API access is working"},
		}

		for i, action := range actions {
			prefix := "  "
			if m.settingFocus == -2 && i == m.actionCursor {
				prefix = "> "
				sb.WriteString(selectedStyle.Render(prefix + action.name))
			} else {
				sb.WriteString(itemStyle.Render(prefix + action.name))
			}
			sb.WriteString("\n")
			sb.WriteString(descStyle.Render("    " + action.desc))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(helpStyle.Render("tab cycle fields  ctrl+s save  esc cancel"))

	return sb.String()
}

// maskConfigValue masks a sensitive value for display.
func maskConfigValue(val string) string {
	if len(val) <= 8 {
		return "********"
	}
	return val[:4] + strings.Repeat("*", len(val)-8) + val[len(val)-4:]
}
