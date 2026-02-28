package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/session"
)

// ── Dashboard Modes ──────────────────────────────────────────────────────────

type dashMode int

const (
	dashModeNormal        dashMode = iota
	dashModeConnectorMenu          // showing context menu for a connector
)

// ── Dashboard Actions ────────────────────────────────────────────────────────

type dashAction int

const (
	dashActionAddConnector dashAction = iota
	dashActionRetrieve
	dashActionProcess
	dashActionAnalyze
	dashActionRepair
	dashActionPush
	dashActionBack
)

var dashActions = []struct {
	action dashAction
	label  string
	key    string
}{
	{dashActionAddConnector, "Add Connector", "a"},
	{dashActionRetrieve, "Retrieve Data", "r"},
	{dashActionProcess, "Process", "p"},
	{dashActionAnalyze, "Analyze", "z"},
	{dashActionRepair, "Repair", "x"},
	{dashActionPush, "Push", "u"},
	{dashActionBack, "Back", "esc"},
}

// ── Connector Menu Actions ───────────────────────────────────────────────────

type connMenuAction int

const (
	connMenuConfigure connMenuAction = iota
	connMenuTest
	connMenuDisable
	connMenuEnable
	connMenuRemove
	connMenuCancel
)

// ── Messages ─────────────────────────────────────────────────────────────────

type dashConnectorRemovedMsg struct {
	instanceID string
	err        error
}

type dashConnectorToggledMsg struct {
	instanceID string
	enabled    bool
	err        error
}

type dashConnectorTestMsg struct {
	instanceID string
	success    bool
	message    string
}

// ── Model ────────────────────────────────────────────────────────────────────

// DashboardModel is the main session view showing configured connectors.
type DashboardModel struct {
	session *session.Session
	width   int
	height  int
	err     error

	// Connector list
	connectors      []core.ConnectorConfig
	connectorCursor int

	// Mode
	mode dashMode

	// Context menu
	menuCursor int
	menuItems  []connMenuAction

	// Test result (displayed temporarily)
	testResult    string
	testIsSuccess bool

	// Loading
	loading bool
	spinner spinner.Model
}

// NewDashboardModel creates a new dashboard for a session.
func NewDashboardModel(sess *session.Session) DashboardModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := DashboardModel{
		session: sess,
		mode:    dashModeNormal,
		spinner: sp,
	}

	if sess != nil {
		m.connectors = sess.ListConnectors()
	}

	return m
}

// Init implements tea.Model.
func (m DashboardModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model.
func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case dashConnectorRemovedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			// Refresh connector list
			m.connectors = m.session.ListConnectors()
			if m.connectorCursor >= len(m.connectors) {
				m.connectorCursor = max(0, len(m.connectors)-1)
			}
		}
		m.mode = dashModeNormal
		return m, nil

	case dashConnectorToggledMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.connectors = m.session.ListConnectors()
		}
		m.mode = dashModeNormal
		return m, nil

	case dashConnectorTestMsg:
		m.loading = false
		m.testResult = msg.message
		m.testIsSuccess = msg.success
		m.mode = dashModeNormal
		return m, nil
	}

	if m.loading {
		return m, nil
	}

	switch m.mode {
	case dashModeNormal:
		return m.updateNormal(msg)
	case dashModeConnectorMenu:
		return m.updateConnectorMenu(msg)
	}

	return m, nil
}

func (m DashboardModel) updateNormal(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.err = nil
		m.testResult = "" // Clear test result on any key press

		switch msg.String() {
		case "up", "k":
			if m.connectorCursor > 0 {
				m.connectorCursor--
			}
		case "down", "j":
			if m.connectorCursor < len(m.connectors)-1 {
				m.connectorCursor++
			}
		case "enter":
			// Open context menu for selected connector
			if len(m.connectors) > 0 {
				m.mode = dashModeConnectorMenu
				m.menuCursor = 0
				m.menuItems = m.buildMenuItems()
			}
			return m, nil

		case "a": // Add connector
			return m, func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenAddConnector,
					Session: m.session,
				}
			}

		case "r": // Retrieve
			if len(m.session.GetInputConnectors()) == 0 {
				m.err = fmt.Errorf("no input connectors configured")
				return m, nil
			}
			return m, func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenRetrieve,
					Session: m.session,
				}
			}

		case "p": // Process
			return m, func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenProcess,
					Session: m.session,
				}
			}

		case "z": // Analyze
			return m, func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenAnalyze,
					Session: m.session,
				}
			}

		case "x": // Repair
			if len(m.session.GetFallbackConnectors()) == 0 {
				m.err = fmt.Errorf("no fallback connectors configured")
				return m, nil
			}
			return m, func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenRepair,
					Session: m.session,
				}
			}

		case "u": // Push
			if len(m.session.GetOutputConnectors()) == 0 {
				m.err = fmt.Errorf("no output connectors configured")
				return m, nil
			}
			return m, func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenPush,
					Session: m.session,
				}
			}

		case "esc", "q":
			return m, func() tea.Msg {
				return SwitchScreenMsg{To: ScreenSessions}
			}
		}
	}
	return m, nil
}

func (m DashboardModel) buildMenuItems() []connMenuAction {
	if len(m.connectors) == 0 || m.connectorCursor >= len(m.connectors) {
		return []connMenuAction{connMenuCancel}
	}

	conn := m.connectors[m.connectorCursor]
	items := []connMenuAction{connMenuConfigure, connMenuTest}

	if conn.Enabled {
		items = append(items, connMenuDisable)
	} else {
		items = append(items, connMenuEnable)
	}

	items = append(items, connMenuRemove, connMenuCancel)
	return items
}

func (m DashboardModel) updateConnectorMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.menuCursor > 0 {
				m.menuCursor--
			}
		case "down", "j":
			if m.menuCursor < len(m.menuItems)-1 {
				m.menuCursor++
			}
		case "esc":
			m.mode = dashModeNormal
			return m, nil
		case "enter":
			return m.executeMenuAction()
		}
	}
	return m, nil
}

func (m DashboardModel) executeMenuAction() (tea.Model, tea.Cmd) {
	if m.menuCursor >= len(m.menuItems) {
		m.mode = dashModeNormal
		return m, nil
	}

	action := m.menuItems[m.menuCursor]
	conn := m.connectors[m.connectorCursor]

	switch action {
	case connMenuConfigure:
		configModel := NewConfigConnectorModel(m.session, conn.InstanceID)
		return m, func() tea.Msg {
			return SwitchScreenMsg{
				To:              ScreenConfigConnector,
				Session:         m.session,
				ConfigConnector: &configModel,
			}
		}

	case connMenuTest:
		m.loading = true
		m.testResult = "" // Clear previous result
		connCfg := conn
		return m, func() tea.Msg {
			return testConnector(connCfg)
		}

	case connMenuDisable:
		m.loading = true
		instanceID := conn.InstanceID
		return m, func() tea.Msg {
			m.session.Config.Connectors.DisableConnector(instanceID)
			err := m.session.Save()
			return dashConnectorToggledMsg{instanceID: instanceID, enabled: false, err: err}
		}

	case connMenuEnable:
		m.loading = true
		instanceID := conn.InstanceID
		return m, func() tea.Msg {
			m.session.Config.Connectors.EnableConnector(instanceID)
			err := m.session.Save()
			return dashConnectorToggledMsg{instanceID: instanceID, enabled: true, err: err}
		}

	case connMenuRemove:
		m.loading = true
		instanceID := conn.InstanceID
		return m, func() tea.Msg {
			m.session.RemoveConnector(instanceID)
			err := m.session.Save()
			return dashConnectorRemovedMsg{instanceID: instanceID, err: err}
		}

	case connMenuCancel:
		m.mode = dashModeNormal
	}

	return m, nil
}

// View implements tea.Model.
func (m DashboardModel) View() string {
	if m.loading {
		return m.viewLoading()
	}

	var sb strings.Builder

	// Header
	sessionName := "Unknown"
	if m.session != nil {
		sessionName = m.session.Config.Name
	}
	sb.WriteString(titleStyle.Render("Session: " + sessionName))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Connector-based data processing"))
	sb.WriteString("\n\n")

	// Error
	if m.err != nil {
		sb.WriteString(dangerStyle.Render("Error: " + m.err.Error()))
		sb.WriteString("\n\n")
	}

	// Test result
	if m.testResult != "" {
		if m.testIsSuccess {
			sb.WriteString(successStyle.Render("✓ " + m.testResult))
		} else {
			sb.WriteString(dangerStyle.Render("✗ " + m.testResult))
		}
		sb.WriteString("\n\n")
	}

	// Two-panel layout
	leftPanel := m.viewConnectorList()
	rightPanel := m.viewStats()

	// Calculate panel widths
	leftWidth := m.width / 2
	if leftWidth < 40 {
		leftWidth = 40
	}
	rightWidth := m.width - leftWidth - 4

	leftBox := lipgloss.NewStyle().
		Width(leftWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1).
		Render(leftPanel)

	rightBox := lipgloss.NewStyle().
		Width(rightWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1).
		Render(rightPanel)

	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox))
	sb.WriteString("\n\n")

	// Context menu overlay
	if m.mode == dashModeConnectorMenu {
		sb.WriteString(m.viewContextMenu())
		sb.WriteString("\n\n")
	}

	// Action bar
	sb.WriteString(m.viewActionBar())

	return sb.String()
}

func (m DashboardModel) viewLoading() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Dashboard"))
	sb.WriteString("\n\n")
	sb.WriteString(m.spinner.View())
	sb.WriteString(" ")
	sb.WriteString(descStyle.Render("Loading..."))
	return sb.String()
}

func (m DashboardModel) viewConnectorList() string {
	var sb strings.Builder

	sb.WriteString(labelStyle.Render("Connectors"))
	sb.WriteString("\n\n")

	if len(m.connectors) == 0 {
		sb.WriteString(descStyle.Render("No connectors configured"))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("Press 'a' to add one"))
		return sb.String()
	}

	for i, conn := range m.connectors {
		// Role badge
		roleBadge := m.renderRoleBadge(conn.Roles)

		// Enabled indicator
		enabledIndicator := ""
		if !conn.Enabled {
			enabledIndicator = " [disabled]"
		}

		line := fmt.Sprintf("%s %s%s", roleBadge, conn.Name, enabledIndicator)

		if i == m.connectorCursor {
			sb.WriteString(selectedStyle.Render("> " + line))
		} else {
			sb.WriteString(itemStyle.Render("  " + line))
		}
		sb.WriteString("\n")

		// Connector type
		sb.WriteString(descStyle.Render(fmt.Sprintf("    Type: %s", conn.ConnectorID)))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m DashboardModel) renderRoleBadge(roles core.ConnectorRoles) string {
	// Build badge based on active roles
	var parts []string

	if roles.IsInput {
		parts = append(parts, lipgloss.NewStyle().
			Background(lipgloss.Color("22")).
			Foreground(lipgloss.Color("46")).
			Render("IN"))
	}
	if roles.IsOutput {
		parts = append(parts, lipgloss.NewStyle().
			Background(lipgloss.Color("17")).
			Foreground(lipgloss.Color("39")).
			Render("OUT"))
	}
	if roles.IsFallback {
		parts = append(parts, lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("208")).
			Render("FB"))
	}

	if len(parts) == 0 {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("250")).
			Padding(0, 1).
			Render("???")
	}

	return strings.Join(parts, " ")
}

func (m DashboardModel) viewStats() string {
	var sb strings.Builder

	sb.WriteString(labelStyle.Render("Session Status"))
	sb.WriteString("\n\n")

	// Count connectors by role
	inputs := len(m.session.GetInputConnectors())
	outputs := len(m.session.GetOutputConnectors())
	fallbacks := len(m.session.GetFallbackConnectors())

	sb.WriteString(fmt.Sprintf("Input connectors:    %d\n", inputs))
	sb.WriteString(fmt.Sprintf("Output connectors:   %d\n", outputs))
	sb.WriteString(fmt.Sprintf("Fallback connectors: %d\n", fallbacks))
	sb.WriteString("\n")

	// Session settings
	sb.WriteString(labelStyle.Render("Settings"))
	sb.WriteString("\n")
	parallelStr := "No"
	if m.session.Config.Connectors.ParallelRetrieval {
		parallelStr = "Yes"
	}
	autoStr := "No"
	if m.session.Config.Connectors.AutoProcess {
		autoStr = "Yes"
	}
	sb.WriteString(fmt.Sprintf("Parallel retrieval: %s\n", parallelStr))
	sb.WriteString(fmt.Sprintf("Auto process:       %s\n", autoStr))

	// Stats if available
	if stats := m.session.Config.Connectors.Stats; stats != nil {
		sb.WriteString("\n")
		sb.WriteString(labelStyle.Render("Data Statistics"))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("Total items:   %d\n", stats.TotalItems))
		sb.WriteString(fmt.Sprintf("Processed OK:  %d\n", stats.ProcessedOK))
		sb.WriteString(fmt.Sprintf("Failed:        %d\n", stats.ProcessedFail))
		sb.WriteString(fmt.Sprintf("Missing:       %d\n", stats.Missing))
		sb.WriteString(fmt.Sprintf("Repairable:    %d\n", stats.Repairable))
	}

	return sb.String()
}

func (m DashboardModel) viewContextMenu() string {
	if len(m.connectors) == 0 || m.connectorCursor >= len(m.connectors) {
		return ""
	}

	conn := m.connectors[m.connectorCursor]
	var sb strings.Builder

	sb.WriteString(labelStyle.Render("Connector: " + conn.Name))
	sb.WriteString("\n\n")

	menuLabels := map[connMenuAction]string{
		connMenuConfigure: "Configure",
		connMenuTest:      "Test Connection",
		connMenuDisable:   "Disable",
		connMenuEnable:    "Enable",
		connMenuRemove:    "Remove",
		connMenuCancel:    "Cancel",
	}

	for i, action := range m.menuItems {
		label := menuLabels[action]
		if i == m.menuCursor {
			sb.WriteString(selectedStyle.Render("> " + label))
		} else {
			sb.WriteString(itemStyle.Render("  " + label))
		}
		sb.WriteString("\n")
	}

	return boxStyle.Render(sb.String())
}

func (m DashboardModel) viewActionBar() string {
	var parts []string
	for _, a := range dashActions {
		// Only show certain actions if applicable
		switch a.action {
		case dashActionRepair:
			if len(m.session.GetFallbackConnectors()) == 0 {
				continue
			}
		case dashActionPush:
			if len(m.session.GetOutputConnectors()) == 0 {
				continue
			}
		case dashActionRetrieve:
			if len(m.session.GetInputConnectors()) == 0 {
				continue
			}
		}

		parts = append(parts, actionStyle.Render(a.key)+helpStyle.Render(" "+a.label))
	}
	return strings.Join(parts, "  ")
}

// testConnector tests a connector's connection and returns a result message.
func testConnector(cfg core.ConnectorConfig) dashConnectorTestMsg {
	// Get the connector info from registry
	connInfo, ok := core.GetConnectorInfo(cfg.ConnectorID)
	if !ok {
		return dashConnectorTestMsg{
			instanceID: cfg.InstanceID,
			success:    false,
			message:    "Unknown connector type: " + cfg.ConnectorID,
		}
	}

	// Create connector instance
	conn := connInfo.Factory()

	// Check if connector supports testing
	tester, ok := conn.(core.ConnectorTester)
	if !ok {
		return dashConnectorTestMsg{
			instanceID: cfg.InstanceID,
			success:    false,
			message:    "Connector does not support connection testing",
		}
	}

	// Run the test with a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := tester.TestConnection(ctx, cfg)
	if err != nil {
		return dashConnectorTestMsg{
			instanceID: cfg.InstanceID,
			success:    false,
			message:    "Test failed: " + err.Error(),
		}
	}

	return dashConnectorTestMsg{
		instanceID: cfg.InstanceID,
		success:    true,
		message:    "Connection successful!",
	}
}
