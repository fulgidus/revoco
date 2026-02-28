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
	"github.com/google/uuid"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/session"
	"github.com/fulgidus/revoco/tui/components"
)

// ── Wizard Steps ─────────────────────────────────────────────────────────────

type addConnStep int

const (
	addConnStepType     addConnStep = iota // Step 1: Select connector type
	addConnStepSetup                       // Step 1.5: Show setup instructions (if any)
	addConnStepRole                        // Step 2: Select role
	addConnStepSettings                    // Step 3: Configure settings
	addConnStepConfirm                     // Step 4: Confirm and save
	addConnStepTesting                     // Step 5: Test connection (for OAuth connectors)
)

// ── Messages ─────────────────────────────────────────────────────────────────

type addConnSavedMsg struct {
	err error
}

type addConnTestMsg struct {
	success bool
	message string
}

// ── Model ────────────────────────────────────────────────────────────────────

// AddConnectorModel is the wizard for adding a new connector.
type AddConnectorModel struct {
	session *session.Session
	width   int
	height  int
	err     error

	// Wizard state
	step addConnStep

	// Step 1: Type selection
	connectorTypes []*core.ConnectorInfo
	typeCursor     int

	// Step 1.5: Setup instructions
	setupInstructions string
	setupScroll       int

	// Step 2: Role selection (multi-select checkboxes)
	roleInput    bool
	roleOutput   bool
	roleFallback bool
	roleCursor   int // 0=input, 1=output, 2=fallback

	// Step 3: Settings
	// For local connectors, we need a path
	// For OAuth connectors, we need client credentials
	nameInput      textinput.Model
	pathInput      textinput.Model
	clientIDInput  textinput.Model
	secretInput    textinput.Model
	settingsCursor int // Which setting field is focused
	importModes    []core.ImportMode
	importCursor   int
	pickerOpen     bool
	picker         components.FilePicker

	// Final config being built
	config core.ConnectorConfig

	// Step 5: Test result
	testSuccess bool
	testMessage string

	// Loading
	loading bool
	spinner spinner.Model
}

// NewAddConnectorModel creates the add connector wizard.
func NewAddConnectorModel(sess *session.Session) AddConnectorModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	nameIn := textinput.New()
	nameIn.Placeholder = "Connector name"
	nameIn.CharLimit = 64

	pathIn := textinput.New()
	pathIn.Placeholder = "/path/to/data"
	pathIn.CharLimit = 256

	clientIDIn := textinput.New()
	clientIDIn.Placeholder = "OAuth Client ID"
	clientIDIn.CharLimit = 128

	secretIn := textinput.New()
	secretIn.Placeholder = "OAuth Client Secret"
	secretIn.CharLimit = 128
	secretIn.EchoMode = textinput.EchoPassword

	m := AddConnectorModel{
		session:        sess,
		step:           addConnStepType,
		spinner:        sp,
		nameInput:      nameIn,
		pathInput:      pathIn,
		clientIDInput:  clientIDIn,
		secretInput:    secretIn,
		connectorTypes: core.ListConnectors(),
		importModes: []core.ImportMode{
			core.ImportModeCopy,
			core.ImportModeMove,
			core.ImportModeReference,
		},
	}

	return m
}

// Init implements tea.Model.
func (m AddConnectorModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.spinner.Tick,
	)
}

// Update implements tea.Model.
func (m AddConnectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case addConnSavedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}

		// Check if this connector requires auth and supports testing
		connInfo, ok := core.GetConnectorInfo(m.config.ConnectorID)
		if ok && connInfo.RequiresAuth && connInfo.AuthType == "oauth2" {
			// Go to testing step for OAuth connectors
			m.step = addConnStepTesting
			m.loading = true
			config := m.config
			return m, func() tea.Msg {
				return runConnectorTest(config)
			}
		}

		// For non-OAuth connectors, go back to dashboard
		return m, func() tea.Msg {
			return SwitchScreenMsg{
				To:      ScreenDashboard,
				Session: m.session,
			}
		}

	case addConnTestMsg:
		m.loading = false
		m.testSuccess = msg.success
		m.testMessage = msg.message
		return m, nil
	}

	if m.loading {
		return m, nil
	}

	if m.pickerOpen {
		return m.updatePicker(msg)
	}

	switch m.step {
	case addConnStepType:
		return m.updateStepType(msg)
	case addConnStepSetup:
		return m.updateStepSetup(msg)
	case addConnStepRole:
		return m.updateStepRole(msg)
	case addConnStepSettings:
		return m.updateStepSettings(msg)
	case addConnStepConfirm:
		return m.updateStepConfirm(msg)
	case addConnStepTesting:
		return m.updateStepTesting(msg)
	}

	return m, nil
}

func (m AddConnectorModel) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.pickerOpen = false
		// Handle multi-file selection for zip/tgz connectors
		if len(m.picker.MultiSelected) > 0 {
			m.pathInput.SetValue(strings.Join(m.picker.MultiSelected, ","))
		} else if m.picker.Selected != "" {
			m.pathInput.SetValue(m.picker.Selected)
		}
	}

	return m, cmd
}

func (m AddConnectorModel) updateStepType(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.err = nil
		switch msg.String() {
		case "up", "k":
			if m.typeCursor > 0 {
				m.typeCursor--
			}
		case "down", "j":
			if m.typeCursor < len(m.connectorTypes)-1 {
				m.typeCursor++
			}
		case "enter":
			if len(m.connectorTypes) > 0 {
				selected := m.connectorTypes[m.typeCursor]
				m.config.ConnectorID = selected.ID
				m.config.InstanceID = fmt.Sprintf("%s-%s", selected.ID, uuid.New().String()[:8])
				m.nameInput.SetValue(selected.Name)

				// Check if connector has setup instructions
				conn := selected.Factory()
				if setupConn, ok := conn.(core.ConnectorWithSetup); ok {
					m.setupInstructions = setupConn.SetupInstructions()
					m.setupScroll = 0
					m.step = addConnStepSetup
				} else {
					m.step = addConnStepRole
				}
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
	}
	return m, nil
}

func (m AddConnectorModel) updateStepSetup(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.setupScroll > 0 {
				m.setupScroll--
			}
		case "down", "j":
			m.setupScroll++
		case "pgup":
			m.setupScroll -= 10
			if m.setupScroll < 0 {
				m.setupScroll = 0
			}
		case "pgdown":
			m.setupScroll += 10
		case "enter":
			m.step = addConnStepRole
			return m, nil
		case "esc":
			m.step = addConnStepType
			m.setupInstructions = ""
			return m, nil
		}
	}
	return m, nil
}

func (m AddConnectorModel) updateStepRole(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.err = nil
		switch msg.String() {
		case "up", "k":
			if m.roleCursor > 0 {
				m.roleCursor--
			}
		case "down", "j":
			if m.roleCursor < 2 { // 3 options: input, output, fallback
				m.roleCursor++
			}
		case " ", "x": // Space or x to toggle
			switch m.roleCursor {
			case 0:
				m.roleInput = !m.roleInput
			case 1:
				m.roleOutput = !m.roleOutput
			case 2:
				m.roleFallback = !m.roleFallback
			}
			return m, nil
		case "enter":
			// Build roles from selections
			m.config.Roles = core.ConnectorRoles{
				IsInput:    m.roleInput,
				IsOutput:   m.roleOutput,
				IsFallback: m.roleFallback,
			}

			// Validate that at least one role is selected
			if !m.config.Roles.HasAnyRole() {
				m.err = fmt.Errorf("select at least one role")
				return m, nil
			}

			// Validate roles against connector capabilities
			connInfo, ok := core.GetConnectorInfo(m.config.ConnectorID)
			if ok {
				conn := connInfo.Factory()
				if err := core.ValidateRoles(conn, m.config.Roles); err != nil {
					m.err = err
					return m, nil
				}
			}

			m.nameInput.Focus()
			m.step = addConnStepSettings
			return m, nil
		case "esc":
			m.step = addConnStepType
			return m, nil
		}
	}
	return m, nil
}

func (m AddConnectorModel) updateStepSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	connInfo, _ := core.GetConnectorInfo(m.config.ConnectorID)
	isOAuth := connInfo != nil && connInfo.RequiresAuth && connInfo.AuthType == "oauth2"

	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.err = nil
		switch msg.String() {
		case "tab", "shift+tab":
			// Cycle through input fields
			if isOAuth {
				// For OAuth connectors: name -> clientID -> secret
				switch {
				case m.nameInput.Focused():
					m.nameInput.Blur()
					m.clientIDInput.Focus()
					m.settingsCursor = 1
				case m.clientIDInput.Focused():
					m.clientIDInput.Blur()
					m.secretInput.Focus()
					m.settingsCursor = 2
				default:
					m.secretInput.Blur()
					m.nameInput.Focus()
					m.settingsCursor = 0
				}
			} else {
				// For local connectors: name -> path
				if m.nameInput.Focused() {
					m.nameInput.Blur()
					m.pathInput.Focus()
					m.settingsCursor = 1
				} else {
					m.pathInput.Blur()
					m.nameInput.Focus()
					m.settingsCursor = 0
				}
			}
			return m, nil

		case "ctrl+p":
			// Open file picker for path (only for local connectors)
			if !isOAuth && m.needsPath() {
				m.pickerOpen = true
				// Determine picker mode based on connector type
				if strings.Contains(m.config.ConnectorID, "folder") {
					m.picker = components.NewFilePicker(components.ModeDir)
				} else if strings.Contains(m.config.ConnectorID, "zip") {
					m.picker = components.NewFilePicker(components.ModeMultiFile)
					m.picker.AllowedExts = []string{".zip"}
				} else if strings.Contains(m.config.ConnectorID, "tgz") {
					m.picker = components.NewFilePicker(components.ModeMultiFile)
					m.picker.AllowedExts = []string{".tgz", ".tar.gz"}
				} else {
					m.picker = components.NewFilePicker(components.ModeFile)
				}
				return m, m.picker.Init()
			}
			return m, nil

		case "ctrl+i", "i":
			// Cycle import mode (only when input role is selected and not in text field)
			if m.roleInput && !m.nameInput.Focused() && !m.pathInput.Focused() &&
				!m.clientIDInput.Focused() && !m.secretInput.Focused() {
				m.importCursor = (m.importCursor + 1) % len(m.importModes)
			}
			return m, nil

		case "enter":
			// Validate and proceed
			name := strings.TrimSpace(m.nameInput.Value())
			if name == "" {
				m.err = fmt.Errorf("name is required")
				return m, nil
			}
			m.config.Name = name

			if isOAuth {
				// Validate OAuth credentials
				clientID := strings.TrimSpace(m.clientIDInput.Value())
				clientSecret := strings.TrimSpace(m.secretInput.Value())

				if clientID == "" {
					m.err = fmt.Errorf("Client ID is required")
					return m, nil
				}
				if clientSecret == "" {
					m.err = fmt.Errorf("Client Secret is required")
					return m, nil
				}

				m.config.Settings = map[string]any{
					"client_id":     clientID,
					"client_secret": clientSecret,
				}
			} else {
				// Validate path for local connectors
				path := strings.TrimSpace(m.pathInput.Value())
				if path == "" && m.needsPath() {
					m.err = fmt.Errorf("path is required for this connector")
					return m, nil
				}
				if path != "" {
					// For multi-file connectors, use "paths" array; for single, use "path"
					if m.isMultiPathConnector() && strings.Contains(path, ",") {
						paths := strings.Split(path, ",")
						for i := range paths {
							paths[i] = strings.TrimSpace(paths[i])
						}
						m.config.Settings = map[string]any{"paths": paths}
					} else {
						m.config.Settings = map[string]any{"path": path}
					}
				}
			}

			// Set import mode for input connectors
			if m.config.Roles.IsInput {
				m.config.ImportMode = m.importModes[m.importCursor]
			}

			m.config.Enabled = true
			m.step = addConnStepConfirm
			return m, nil

		case "esc":
			// Go back to setup if there were instructions, otherwise to role
			if m.setupInstructions != "" {
				m.step = addConnStepSetup
			} else {
				m.step = addConnStepRole
			}
			return m, nil
		}
	}

	// Update the focused text input
	var cmd tea.Cmd
	if isOAuth {
		switch {
		case m.nameInput.Focused():
			m.nameInput, cmd = m.nameInput.Update(msg)
		case m.clientIDInput.Focused():
			m.clientIDInput, cmd = m.clientIDInput.Update(msg)
		case m.secretInput.Focused():
			m.secretInput, cmd = m.secretInput.Update(msg)
		}
	} else {
		if m.nameInput.Focused() {
			m.nameInput, cmd = m.nameInput.Update(msg)
		} else {
			m.pathInput, cmd = m.pathInput.Update(msg)
		}
	}
	return m, cmd
}

func (m AddConnectorModel) needsPath() bool {
	// Local connectors need a path
	return strings.HasPrefix(m.config.ConnectorID, "local-")
}

func (m AddConnectorModel) isMultiPathConnector() bool {
	// Connectors that accept multiple paths
	return strings.Contains(m.config.ConnectorID, "multi-")
}

func (m AddConnectorModel) updateStepConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			m.loading = true
			config := m.config
			sess := m.session
			return m, func() tea.Msg {
				sess.AddConnector(config)
				err := sess.Save()
				return addConnSavedMsg{err: err}
			}
		case "n", "N", "esc":
			m.step = addConnStepSettings
			return m, nil
		}
	}
	return m, nil
}

func (m AddConnectorModel) updateStepTesting(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc":
			// Return to dashboard
			return m, func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenDashboard,
					Session: m.session,
				}
			}
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m AddConnectorModel) View() string {
	if m.loading {
		return m.viewLoading()
	}

	if m.pickerOpen {
		return m.viewPicker()
	}

	var sb strings.Builder

	// Header with step indicator
	sb.WriteString(titleStyle.Render("Add Connector"))
	sb.WriteString("\n")
	sb.WriteString(m.viewStepIndicator())
	sb.WriteString("\n\n")

	// Error
	if m.err != nil {
		sb.WriteString(dangerStyle.Render("Error: " + m.err.Error()))
		sb.WriteString("\n\n")
	}

	// Step content
	switch m.step {
	case addConnStepType:
		sb.WriteString(m.viewStepType())
	case addConnStepSetup:
		sb.WriteString(m.viewStepSetup())
	case addConnStepRole:
		sb.WriteString(m.viewStepRole())
	case addConnStepSettings:
		sb.WriteString(m.viewStepSettings())
	case addConnStepConfirm:
		sb.WriteString(m.viewStepConfirm())
	case addConnStepTesting:
		sb.WriteString(m.viewStepTesting())
	}

	return sb.String()
}

func (m AddConnectorModel) viewLoading() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Add Connector"))
	sb.WriteString("\n\n")
	sb.WriteString(m.spinner.View())
	sb.WriteString(" ")
	sb.WriteString(descStyle.Render("Saving connector..."))
	return sb.String()
}

func (m AddConnectorModel) viewPicker() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Select Path"))
	sb.WriteString("\n")
	sb.WriteString(descStyle.Render("Navigate and press Enter to select"))
	sb.WriteString("\n\n")
	sb.WriteString(m.picker.View())
	return sb.String()
}

func (m AddConnectorModel) viewStepIndicator() string {
	// Check if this is an OAuth connector (needs testing step)
	connInfo, _ := core.GetConnectorInfo(m.config.ConnectorID)
	isOAuth := connInfo != nil && connInfo.RequiresAuth && connInfo.AuthType == "oauth2"

	// Show different steps based on configuration
	var steps []string
	var currentIdx int

	if m.setupInstructions != "" {
		if isOAuth {
			steps = []string{"Type", "Setup", "Role", "Settings", "Confirm", "Test"}
		} else {
			steps = []string{"Type", "Setup", "Role", "Settings", "Confirm"}
		}
		switch m.step {
		case addConnStepType:
			currentIdx = 0
		case addConnStepSetup:
			currentIdx = 1
		case addConnStepRole:
			currentIdx = 2
		case addConnStepSettings:
			currentIdx = 3
		case addConnStepConfirm:
			currentIdx = 4
		case addConnStepTesting:
			currentIdx = 5
		}
	} else {
		if isOAuth {
			steps = []string{"Type", "Role", "Settings", "Confirm", "Test"}
		} else {
			steps = []string{"Type", "Role", "Settings", "Confirm"}
		}
		switch m.step {
		case addConnStepType:
			currentIdx = 0
		case addConnStepRole:
			currentIdx = 1
		case addConnStepSettings:
			currentIdx = 2
		case addConnStepConfirm:
			currentIdx = 3
		case addConnStepTesting:
			currentIdx = 4
		}
	}

	var parts []string
	for i, s := range steps {
		if i == currentIdx {
			parts = append(parts, selectedStyle.Render(fmt.Sprintf("[%d] %s", i+1, s)))
		} else if i < currentIdx {
			parts = append(parts, successStyle.Render(fmt.Sprintf("[%d] %s", i+1, s)))
		} else {
			parts = append(parts, descStyle.Render(fmt.Sprintf("[%d] %s", i+1, s)))
		}
	}

	return strings.Join(parts, " > ")
}

func (m AddConnectorModel) viewStepType() string {
	var sb strings.Builder

	sb.WriteString(labelStyle.Render("Select Connector Type"))
	sb.WriteString("\n\n")

	if len(m.connectorTypes) == 0 {
		sb.WriteString(descStyle.Render("No connectors available"))
		return sb.String()
	}

	for i, ct := range m.connectorTypes {
		// Capabilities badges
		var caps []string
		for _, c := range ct.Capabilities {
			caps = append(caps, string(c))
		}
		capsStr := strings.Join(caps, ", ")

		line := fmt.Sprintf("%-20s [%s]", ct.Name, capsStr)

		if i == m.typeCursor {
			sb.WriteString(selectedStyle.Render("> " + line))
		} else {
			sb.WriteString(itemStyle.Render("  " + line))
		}
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("    " + ct.Description))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("up/down navigate  enter select  esc cancel"))

	return sb.String()
}

func (m AddConnectorModel) viewStepSetup() string {
	var sb strings.Builder

	connInfo, _ := core.GetConnectorInfo(m.config.ConnectorID)
	sb.WriteString(labelStyle.Render("Setup: " + connInfo.Name))
	sb.WriteString("\n")
	sb.WriteString(descStyle.Render("Follow these instructions to configure the connector"))
	sb.WriteString("\n\n")

	// Calculate available height for instructions
	availableHeight := m.height - 12 // Leave room for header and footer
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Split instructions into lines and apply scroll
	lines := strings.Split(m.setupInstructions, "\n")

	// Clamp scroll
	maxScroll := len(lines) - availableHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.setupScroll > maxScroll {
		m.setupScroll = maxScroll
	}

	// Display visible lines with simple markdown rendering
	endLine := m.setupScroll + availableHeight
	if endLine > len(lines) {
		endLine = len(lines)
	}

	for _, line := range lines[m.setupScroll:endLine] {
		// Simple markdown-style rendering
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# "):
			sb.WriteString(titleStyle.Render(strings.TrimPrefix(trimmed, "# ")))
		case strings.HasPrefix(trimmed, "## "):
			sb.WriteString(labelStyle.Render(strings.TrimPrefix(trimmed, "## ")))
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			sb.WriteString(itemStyle.Render("  " + trimmed))
		case strings.HasPrefix(trimmed, "Note:"):
			sb.WriteString(warningStyle.Render(trimmed))
		case strings.Contains(trimmed, "http"):
			sb.WriteString(actionStyle.Render(trimmed))
		default:
			sb.WriteString(descStyle.Render(line))
		}
		sb.WriteString("\n")
	}

	// Show scroll indicator
	if len(lines) > availableHeight {
		scrollPercent := 0
		if maxScroll > 0 {
			scrollPercent = (m.setupScroll * 100) / maxScroll
		}
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render(fmt.Sprintf("─── %d%% ───", scrollPercent)))
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("up/down/pgup/pgdown scroll  enter continue  esc back"))

	return sb.String()
}

func (m AddConnectorModel) viewStepRole() string {
	var sb strings.Builder

	connInfo, _ := core.GetConnectorInfo(m.config.ConnectorID)
	sb.WriteString(labelStyle.Render("Select Roles for: " + connInfo.Name))
	sb.WriteString("\n")
	sb.WriteString(descStyle.Render("Use space to toggle, select one or more roles"))
	sb.WriteString("\n\n")

	type roleOption struct {
		name        string
		description string
		selected    *bool
	}

	options := []roleOption{
		{"Input", "Primary data source - data will be retrieved from here", &m.roleInput},
		{"Output", "Primary destination - processed data will be pushed here", &m.roleOutput},
		{"Fallback", "Fallback source - used only to repair missing data", &m.roleFallback},
	}

	for i, opt := range options {
		// Checkbox indicator
		checkbox := "[ ]"
		if *opt.selected {
			checkbox = "[x]"
		}

		line := fmt.Sprintf("%s %s", checkbox, opt.name)
		if i == m.roleCursor {
			sb.WriteString(selectedStyle.Render("> " + line))
		} else {
			sb.WriteString(itemStyle.Render("  " + line))
		}
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("      " + opt.description))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("up/down navigate  space toggle  enter confirm  esc back"))

	return sb.String()
}

func (m AddConnectorModel) viewStepSettings() string {
	var sb strings.Builder

	connInfo, _ := core.GetConnectorInfo(m.config.ConnectorID)
	isOAuth := connInfo != nil && connInfo.RequiresAuth && connInfo.AuthType == "oauth2"

	sb.WriteString(labelStyle.Render("Configure Connector"))
	sb.WriteString("\n\n")

	// Name
	nameLabel := "Name:"
	if m.settingsCursor == 0 {
		nameLabel = "> Name:"
	}
	sb.WriteString(promptStyle.Render(nameLabel))
	sb.WriteString("\n")
	sb.WriteString(m.nameInput.View())
	sb.WriteString("\n\n")

	if isOAuth {
		// OAuth credentials
		clientIDLabel := "Client ID:"
		if m.settingsCursor == 1 {
			clientIDLabel = "> Client ID:"
		}
		sb.WriteString(promptStyle.Render(clientIDLabel))
		sb.WriteString("\n")
		sb.WriteString(m.clientIDInput.View())
		sb.WriteString("\n\n")

		secretLabel := "Client Secret:"
		if m.settingsCursor == 2 {
			secretLabel = "> Client Secret:"
		}
		sb.WriteString(promptStyle.Render(secretLabel))
		sb.WriteString("\n")
		sb.WriteString(m.secretInput.View())
		sb.WriteString("\n\n")

		sb.WriteString(descStyle.Render("After saving, a browser window will open for authorization."))
		sb.WriteString("\n\n")
	} else {
		// Path (if needed for local connectors)
		if m.needsPath() {
			pathLabel := "Path:"
			if m.settingsCursor == 1 {
				pathLabel = "> Path:"
			}
			if m.isMultiPathConnector() {
				pathLabel = "Paths:"
				if m.settingsCursor == 1 {
					pathLabel = "> Paths:"
				}
			}
			sb.WriteString(promptStyle.Render(pathLabel))
			sb.WriteString(" ")
			sb.WriteString(helpStyle.Render("(Ctrl+P to browse)"))
			sb.WriteString("\n")
			sb.WriteString(m.pathInput.View())
			if m.isMultiPathConnector() {
				sb.WriteString("\n")
				sb.WriteString(descStyle.Render("    Use file picker to select multiple files"))
			}
			sb.WriteString("\n\n")
		}
	}

	// Import mode (for input connectors) - always show to make it clear
	if m.roleInput {
		sb.WriteString(promptStyle.Render("Import Mode:"))
		sb.WriteString(" ")
		sb.WriteString(helpStyle.Render("(Ctrl+I or i to cycle)"))
		sb.WriteString("\n\n")

		modeDescriptions := map[core.ImportMode]string{
			core.ImportModeCopy:      "Copy files to session folder (source unchanged)",
			core.ImportModeMove:      "Move files to session folder (source deleted)",
			core.ImportModeReference: "Reference files in place (no copy, source unchanged)",
		}

		for i, mode := range m.importModes {
			prefix := "  "
			if i == m.importCursor {
				prefix = "> "
				sb.WriteString(selectedStyle.Render(prefix + string(mode)))
			} else {
				sb.WriteString(itemStyle.Render(prefix + string(mode)))
			}
			sb.WriteString("\n")
			sb.WriteString(descStyle.Render("    " + modeDescriptions[mode]))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(helpStyle.Render("tab switch fields  enter continue  esc back"))

	return sb.String()
}

func (m AddConnectorModel) viewStepConfirm() string {
	var sb strings.Builder

	sb.WriteString(labelStyle.Render("Confirm Connector"))
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("Type:        %s\n", m.config.ConnectorID))
	sb.WriteString(fmt.Sprintf("Name:        %s\n", m.config.Name))
	sb.WriteString(fmt.Sprintf("Roles:       %s\n", m.config.Roles.String()))
	if m.config.ImportMode != "" {
		sb.WriteString(fmt.Sprintf("Import Mode: %s\n", m.config.ImportMode))
	}

	// Show OAuth credentials (masked secret)
	if clientID, ok := m.config.Settings["client_id"].(string); ok {
		sb.WriteString(fmt.Sprintf("Client ID:   %s\n", clientID))
	}
	if secret, ok := m.config.Settings["client_secret"].(string); ok {
		// Mask the secret, showing only first 4 and last 4 chars
		maskedSecret := maskSecret(secret)
		sb.WriteString(fmt.Sprintf("Secret:      %s\n", maskedSecret))
	}

	// Show path for local connectors
	if path, ok := m.config.Settings["path"]; ok {
		sb.WriteString(fmt.Sprintf("Path:        %s\n", path))
	}
	if paths, ok := m.config.Settings["paths"].([]string); ok {
		sb.WriteString(fmt.Sprintf("Paths:       %d file(s)\n", len(paths)))
		for _, p := range paths {
			sb.WriteString(fmt.Sprintf("             - %s\n", p))
		}
	}

	sb.WriteString("\n")

	// Show note about OAuth flow for OAuth connectors
	connInfo, _ := core.GetConnectorInfo(m.config.ConnectorID)
	if connInfo != nil && connInfo.RequiresAuth && connInfo.AuthType == "oauth2" {
		sb.WriteString(warningStyle.Render("Note: A browser will open for authorization after saving."))
		sb.WriteString("\n\n")
	}

	sb.WriteString(promptStyle.Render("Add this connector? "))
	sb.WriteString(actionStyle.Render("y") + helpStyle.Render(" yes  "))
	sb.WriteString(actionStyle.Render("n") + helpStyle.Render(" no"))

	return sb.String()
}

func (m AddConnectorModel) viewStepTesting() string {
	var sb strings.Builder

	connInfo, _ := core.GetConnectorInfo(m.config.ConnectorID)
	connName := m.config.ConnectorID
	if connInfo != nil {
		connName = connInfo.Name
	}

	sb.WriteString(labelStyle.Render("Testing Connection: " + connName))
	sb.WriteString("\n\n")

	if m.testSuccess {
		sb.WriteString(successStyle.Render("Connection Test Passed!"))
		sb.WriteString("\n\n")
		sb.WriteString(descStyle.Render(m.testMessage))
		sb.WriteString("\n\n")
		sb.WriteString(descStyle.Render("The connector \"" + m.config.Name + "\" has been added and is ready to use."))
	} else {
		sb.WriteString(dangerStyle.Render("Connection Test Failed"))
		sb.WriteString("\n\n")
		sb.WriteString(descStyle.Render(m.testMessage))
		sb.WriteString("\n\n")
		sb.WriteString(warningStyle.Render("The connector has been added but may not work correctly."))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("You can test it again from the Dashboard using the contextual menu."))
	}

	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render("enter/esc continue to dashboard"))

	return sb.String()
}

// maskSecret masks the middle portion of a secret string for display
func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return "********"
	}
	return secret[:4] + strings.Repeat("*", len(secret)-8) + secret[len(secret)-4:]
}

// runConnectorTest tests the connector and returns a test message.
func runConnectorTest(cfg core.ConnectorConfig) addConnTestMsg {
	// Get the connector info from registry
	connInfo, ok := core.GetConnectorInfo(cfg.ConnectorID)
	if !ok {
		return addConnTestMsg{
			success: false,
			message: "Unknown connector type: " + cfg.ConnectorID,
		}
	}

	// Create connector instance
	conn := connInfo.Factory()

	// Check if connector supports testing
	tester, ok := conn.(core.ConnectorTester)
	if !ok {
		return addConnTestMsg{
			success: false,
			message: "Connector does not support connection testing",
		}
	}

	// Run the test with a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := tester.TestConnection(ctx, cfg)
	if err != nil {
		return addConnTestMsg{
			success: false,
			message: "Test failed: " + err.Error(),
		}
	}

	return addConnTestMsg{
		success: true,
		message: "Connection successful!",
	}
}
