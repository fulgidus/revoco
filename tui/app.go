// Package tui provides a Bubble Tea TUI for revoco.
package tui

import (
	"github.com/charmbracelet/bubbletea"

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
	width           int
	height          int

	// activeSession is set when a session is opened from the session list.
	activeSession *session.Session
}

// NewApp creates the TUI application starting on the Sessions screen.
func NewApp() App {
	return App{
		screen:   ScreenSessions,
		sessions: NewSessionsModel(),
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	return a.sessions.Init()
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

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}

	// Delegate to active screen
	return a.delegateMsg(msg)
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
	}
	return a, nil
}

// View implements tea.Model.
func (a App) View() string {
	switch a.screen {
	case ScreenSessions:
		return a.sessions.View()
	case ScreenDashboard:
		return a.dashboard.View()
	case ScreenAddConnector:
		return a.addConnector.View()
	case ScreenConfigConnector:
		return a.configConnector.View()
	case ScreenRetrieve:
		return a.retrieve.View()
	case ScreenProcess:
		return a.process.View()
	case ScreenAnalyze:
		return a.analyze.View()
	case ScreenRepair:
		return a.repair.View()
	case ScreenPush:
		return a.push.View()
	default:
		return a.sessions.View()
	}
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

	default:
		a.sessions = NewSessionsModel()
		return a, tea.Batch(a.sessions.Init(), sizeCmd)
	}
}
