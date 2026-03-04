// Package tui provides a Bubble Tea TUI for revoco.
package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fulgidus/revoco/cmd"
	"github.com/fulgidus/revoco/config"
	"github.com/fulgidus/revoco/session"
)

// Screen identifies which TUI screen is active.
type Screen int

const (
	ScreenSessions        Screen = iota // session management (landing)
	ScreenDashboard                     // main session view with connectors
	ScreenAddConnector                  // wizard to add a connector
	ScreenConfigConnector               // configure a connector instance
	ScreenRetrieve                      // data retrieval progress
	ScreenProcess                       // processing with processors
	ScreenAnalyze                       // statistics and analysis
	ScreenRepair                        // fallback-based repair
	ScreenPush                          // output to destinations
	ScreenUpdateConfirm                 // update confirmation dialog
	ScreenSettings                      // settings screen
	ScreenFirstRun                      // first-run channel selection
)

// App is the top-level Bubble Tea model that hosts all screens.
type App struct {
	screen          Screen
	sessions        SessionsModel
	dashboard       DashboardModel
	addConnector    AddConnectorModel
	configConnector ConfigConnectorModel
	retrieve        RetrieveModel
	process         ProcessModel
	analyze         AnalyzeModel
	repair          RepairModel
	push            PushModel
	updateConfirm   UpdateConfirmModel
	settings        SettingsModel
	firstRun        FirstRunModel
	width           int
	height          int

	// activeSession is set when a session is opened from the session list.
	activeSession *session.Session

	// updateState tracks update availability
	updateState UpdateState

	// previousScreen stores the screen to return to after update confirm
	previousScreen Screen
}

// NewApp creates the TUI application starting on the Sessions screen.
func NewApp() App {
	// Check if this is first run (no config file)
	configPath, _ := config.ConfigPath()
	isFirstRun := false
	if configPath != "" {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			isFirstRun = true
		}
	}

	initialScreen := ScreenSessions
	if isFirstRun {
		initialScreen = ScreenFirstRun
	}

	return App{
		screen:   initialScreen,
		sessions: NewSessionsModel(),
		firstRun: NewFirstRunModel(),
		updateState: UpdateState{
			Checking: true, // Will start checking on Init
		},
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	// Start update check in background along with session init
	return tea.Batch(
		a.sessions.Init(),
		CheckForUpdatesCmd(),
	)
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a.propagateSize(msg)

	case SwitchScreenMsg:
		return a.switchScreen(msg)

	case UpdateCheckMsg:
		return a.handleUpdateCheck(msg)

	case SelfUpdateCompleteMsg:
		return a.handleSelfUpdateComplete(msg)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}

		// Handle 'u' key for self-update (only on main screens, not during update confirm)
		if msg.String() == "u" && a.updateState.RevocoUpdateAvailable != "" && a.screen != ScreenUpdateConfirm {
			return a.showUpdateConfirm()
		}
	}

	// Handle update confirmation screen specially
	if a.screen == ScreenUpdateConfirm {
		return a.handleUpdateConfirmUpdate(msg)
	}

	// Delegate to active screen
	return a.delegateMsg(msg)
}

// handleUpdateCheck processes the update check result.
func (a App) handleUpdateCheck(msg UpdateCheckMsg) (tea.Model, tea.Cmd) {
	a.updateState.Checking = false
	a.updateState.RevocoUpdateAvailable = msg.RevocoUpdate
	a.updateState.PluginUpdatesAvailable = msg.PluginUpdates
	a.updateState.CheckError = msg.Error
	return a, nil
}

// handleSelfUpdateComplete processes the self-update result.
func (a App) handleSelfUpdateComplete(msg SelfUpdateCompleteMsg) (tea.Model, tea.Cmd) {
	a.updateState.Updating = false
	a.updateState.UpdateStage = ""
	a.updateState.UpdateProgress = 0

	if msg.Success {
		// Update completed - show message and quit so user can restart
		return a, tea.Quit
	}

	// Update failed - return to previous screen
	a.screen = a.previousScreen
	return a, nil
}

// showUpdateConfirm switches to the update confirmation screen.
func (a App) showUpdateConfirm() (tea.Model, tea.Cmd) {
	a.previousScreen = a.screen
	a.screen = ScreenUpdateConfirm
	a.updateConfirm = NewUpdateConfirmModel(
		strings.TrimPrefix(cmd.GetVersion(), "v"),
		a.updateState.RevocoUpdateAvailable,
	)
	return a, a.updateConfirm.Init()
}

// handleUpdateConfirmUpdate handles updates while on the confirmation screen.
func (a App) handleUpdateConfirmUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := a.updateConfirm.Update(msg)
	a.updateConfirm = m.(UpdateConfirmModel)

	if a.updateConfirm.IsConfirmed() {
		// User confirmed - start the update
		// For now, we'll just print a message and return to previous screen
		// A full implementation would run the update in the background
		a.screen = a.previousScreen
		a.updateState.Updating = true
		a.updateState.UpdateStage = "preparing"
		// TODO: Actually run the self-update command
		// For now, just return to previous screen with a note
		a.updateState.Updating = false
		return a, nil
	}

	if a.updateConfirm.IsCancelled() {
		// User cancelled - return to previous screen
		a.screen = a.previousScreen
		return a, nil
	}

	return a, cmd
}

// propagateSize sends a WindowSizeMsg to the active screen.
func (a App) propagateSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	switch a.screen {
	case ScreenSessions:
		m, cmd := a.sessions.Update(msg)
		a.sessions = m.(SessionsModel)
		return a, cmd
	case ScreenDashboard:
		m, cmd := a.dashboard.Update(msg)
		a.dashboard = m.(DashboardModel)
		return a, cmd
	case ScreenAddConnector:
		m, cmd := a.addConnector.Update(msg)
		a.addConnector = m.(AddConnectorModel)
		return a, cmd
	case ScreenConfigConnector:
		m, cmd := a.configConnector.Update(msg)
		a.configConnector = m.(ConfigConnectorModel)
		return a, cmd
	case ScreenRetrieve:
		m, cmd := a.retrieve.Update(msg)
		a.retrieve = m.(RetrieveModel)
		return a, cmd
	case ScreenProcess:
		m, cmd := a.process.Update(msg)
		a.process = m.(ProcessModel)
		return a, cmd
	case ScreenAnalyze:
		m, cmd := a.analyze.Update(msg)
		a.analyze = m.(AnalyzeModel)
		return a, cmd
	case ScreenRepair:
		m, cmd := a.repair.Update(msg)
		a.repair = m.(RepairModel)
		return a, cmd
	case ScreenPush:
		m, cmd := a.push.Update(msg)
		a.push = m.(PushModel)
		return a, cmd
	case ScreenUpdateConfirm:
		m, cmd := a.updateConfirm.Update(msg)
		a.updateConfirm = m.(UpdateConfirmModel)
		return a, cmd
	case ScreenSettings:
		m, cmd := a.settings.Update(msg)
		a.settings = m.(SettingsModel)
		return a, cmd
	case ScreenFirstRun:
		m, cmd := a.firstRun.Update(msg)
		a.firstRun = m.(FirstRunModel)
		return a, cmd
	}
	return a, nil
}

// delegateMsg forwards any message to the currently active screen model.
func (a App) delegateMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch a.screen {
	case ScreenSessions:
		m, cmd := a.sessions.Update(msg)
		a.sessions = m.(SessionsModel)
		return a, cmd
	case ScreenDashboard:
		m, cmd := a.dashboard.Update(msg)
		a.dashboard = m.(DashboardModel)
		return a, cmd
	case ScreenAddConnector:
		m, cmd := a.addConnector.Update(msg)
		a.addConnector = m.(AddConnectorModel)
		return a, cmd
	case ScreenConfigConnector:
		m, cmd := a.configConnector.Update(msg)
		a.configConnector = m.(ConfigConnectorModel)
		return a, cmd
	case ScreenRetrieve:
		m, cmd := a.retrieve.Update(msg)
		a.retrieve = m.(RetrieveModel)
		return a, cmd
	case ScreenProcess:
		m, cmd := a.process.Update(msg)
		a.process = m.(ProcessModel)
		return a, cmd
	case ScreenAnalyze:
		m, cmd := a.analyze.Update(msg)
		a.analyze = m.(AnalyzeModel)
		return a, cmd
	case ScreenRepair:
		m, cmd := a.repair.Update(msg)
		a.repair = m.(RepairModel)
		return a, cmd
	case ScreenPush:
		m, cmd := a.push.Update(msg)
		a.push = m.(PushModel)
		return a, cmd
	case ScreenFirstRun:
		m, cmd := a.firstRun.Update(msg)
		a.firstRun = m.(FirstRunModel)
		return a, cmd
	case ScreenSettings:
		m, cmd := a.settings.Update(msg)
		a.settings = m.(SettingsModel)
		return a, cmd
	}
	return a, nil
}

// View implements tea.Model.
func (a App) View() string {
	// Build header with update status
	header := a.buildHeader()

	// Build main content
	var content string
	switch a.screen {
	case ScreenSessions:
		content = a.sessions.View()
	case ScreenDashboard:
		content = a.dashboard.View()
	case ScreenAddConnector:
		content = a.addConnector.View()
	case ScreenConfigConnector:
		content = a.configConnector.View()
	case ScreenRetrieve:
		content = a.retrieve.View()
	case ScreenProcess:
		content = a.process.View()
	case ScreenAnalyze:
		content = a.analyze.View()
	case ScreenRepair:
		content = a.repair.View()
	case ScreenPush:
		content = a.push.View()
	case ScreenUpdateConfirm:
		content = a.updateConfirm.View()
	case ScreenSettings:
		content = a.settings.View()
	case ScreenFirstRun:
		content = a.firstRun.View()
	default:
		content = a.sessions.View()
	}

	// Combine header and content
	if header != "" {
		return header + "\n" + content
	}
	return content
}

// buildHeader builds the header line with version and update status.
func (a App) buildHeader() string {
	version := cmd.GetVersion()
	badge := a.updateState.UpdateBadge()

	if badge != "" {
		return fmt.Sprintf("revoco v%s  %s", strings.TrimPrefix(version, "v"), badge)
	}
	return ""
}

// SwitchScreenMsg is sent to navigate between screens.
type SwitchScreenMsg struct {
	To      Screen
	Session *session.Session // set when opening a session

	// Screen-specific data for initialization
	Dashboard       *DashboardModel
	AddConnector    *AddConnectorModel
	ConfigConnector *ConfigConnectorModel
	Retrieve        *RetrieveModel
	Process         *ProcessModel
	Analyze         *AnalyzeModel
	Repair          *RepairModel
	Push            *PushModel
}

func (a App) switchScreen(msg SwitchScreenMsg) (tea.Model, tea.Cmd) {
	a.screen = msg.To

	// Capture session context when transitioning
	if msg.Session != nil {
		a.activeSession = msg.Session
	}

	// Helper to send current window size to newly created screen
	sizeCmd := func() tea.Msg {
		return tea.WindowSizeMsg{Width: a.width, Height: a.height}
	}

	switch msg.To {
	case ScreenSessions:
		a.sessions = NewSessionsModel()
		a.activeSession = nil
		return a, tea.Batch(a.sessions.Init(), sizeCmd)

	case ScreenDashboard:
		if msg.Dashboard != nil {
			a.dashboard = *msg.Dashboard
		} else {
			a.dashboard = NewDashboardModel(a.activeSession)
		}
		return a, tea.Batch(a.dashboard.Init(), sizeCmd)

	case ScreenAddConnector:
		if msg.AddConnector != nil {
			a.addConnector = *msg.AddConnector
		} else {
			a.addConnector = NewAddConnectorModel(a.activeSession)
		}
		return a, tea.Batch(a.addConnector.Init(), sizeCmd)

	case ScreenConfigConnector:
		if msg.ConfigConnector != nil {
			a.configConnector = *msg.ConfigConnector
		}
		return a, tea.Batch(a.configConnector.Init(), sizeCmd)

	case ScreenRetrieve:
		if msg.Retrieve != nil {
			a.retrieve = *msg.Retrieve
		} else {
			a.retrieve = NewRetrieveModel(a.activeSession)
		}
		return a, tea.Batch(a.retrieve.Init(), sizeCmd)

	case ScreenProcess:
		if msg.Process != nil {
			a.process = *msg.Process
		} else {
			a.process = NewProcessModelFromSession(a.activeSession)
		}
		return a, tea.Batch(a.process.Init(), sizeCmd)

	case ScreenAnalyze:
		if msg.Analyze != nil {
			a.analyze = *msg.Analyze
		} else {
			a.analyze = NewAnalyzeModelFromSession(a.activeSession)
		}
		return a, tea.Batch(a.analyze.Init(), sizeCmd)

	case ScreenRepair:
		if msg.Repair != nil {
			a.repair = *msg.Repair
		} else {
			a.repair = NewRepairModel(a.activeSession)
		}
		return a, tea.Batch(a.repair.Init(), sizeCmd)

	case ScreenPush:
		if msg.Push != nil {
			a.push = *msg.Push
		} else {
			a.push = NewPushModel(a.activeSession)
		}
		return a, tea.Batch(a.push.Init(), sizeCmd)

	case ScreenSettings:
		a.settings = NewSettingsModel()
		return a, tea.Batch(a.settings.Init(), sizeCmd)

	case ScreenFirstRun:
		a.firstRun = NewFirstRunModel()
		return a, tea.Batch(a.firstRun.Init(), sizeCmd)

	default:
		a.sessions = NewSessionsModel()
		return a, tea.Batch(a.sessions.Init(), sizeCmd)
	}
}
