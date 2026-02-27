// Package tui provides a Bubble Tea TUI for revoco.
package tui

import (
	"github.com/charmbracelet/bubbletea"
)

// Screen identifies which TUI screen is active.
type Screen int

const (
	ScreenWelcome Screen = iota
	ScreenProcess
	ScreenRecover
	ScreenReport
)

// App is the top-level Bubble Tea model that hosts all screens.
type App struct {
	screen  Screen
	welcome WelcomeModel
	process ProcessModel
	recover RecoverModel
	report  ReportModel
	width   int
	height  int
}

// NewApp creates the TUI application starting on the Welcome screen.
func NewApp() App {
	return App{
		screen:  ScreenWelcome,
		welcome: NewWelcomeModel(),
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	return a.welcome.Init()
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// propagate to active sub-model
		switch a.screen {
		case ScreenWelcome:
			m, cmd := a.welcome.Update(msg)
			a.welcome = m.(WelcomeModel)
			return a, cmd
		case ScreenProcess:
			m, cmd := a.process.Update(msg)
			a.process = m.(ProcessModel)
			return a, cmd
		case ScreenRecover:
			m, cmd := a.recover.Update(msg)
			a.recover = m.(RecoverModel)
			return a, cmd
		case ScreenReport:
			m, cmd := a.report.Update(msg)
			a.report = m.(ReportModel)
			return a, cmd
		}

	case SwitchScreenMsg:
		return a.switchScreen(msg)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}

	// Delegate to active screen
	switch a.screen {
	case ScreenWelcome:
		m, cmd := a.welcome.Update(msg)
		a.welcome = m.(WelcomeModel)
		return a, cmd
	case ScreenProcess:
		m, cmd := a.process.Update(msg)
		a.process = m.(ProcessModel)
		return a, cmd
	case ScreenRecover:
		m, cmd := a.recover.Update(msg)
		a.recover = m.(RecoverModel)
		return a, cmd
	case ScreenReport:
		m, cmd := a.report.Update(msg)
		a.report = m.(ReportModel)
		return a, cmd
	}
	return a, nil
}

// View implements tea.Model.
func (a App) View() string {
	switch a.screen {
	case ScreenProcess:
		return a.process.View()
	case ScreenRecover:
		return a.recover.View()
	case ScreenReport:
		return a.report.View()
	default:
		return a.welcome.View()
	}
}

// SwitchScreenMsg is sent to navigate between screens.
type SwitchScreenMsg struct {
	To      Screen
	Process *ProcessModel
	Recover *RecoverModel
	Report  *ReportModel
}

func (a App) switchScreen(msg SwitchScreenMsg) (tea.Model, tea.Cmd) {
	a.screen = msg.To
	switch msg.To {
	case ScreenProcess:
		if msg.Process != nil {
			a.process = *msg.Process
		}
		return a, a.process.Init()
	case ScreenRecover:
		if msg.Recover != nil {
			a.recover = *msg.Recover
		}
		return a, a.recover.Init()
	case ScreenReport:
		if msg.Report != nil {
			a.report = *msg.Report
		}
		return a, a.report.Init()
	default:
		a.welcome = NewWelcomeModel()
		return a, a.welcome.Init()
	}
}
