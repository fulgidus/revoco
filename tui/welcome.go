package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			PaddingLeft(2)

	menuSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				PaddingLeft(0)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238")).
			MarginTop(1)
)

// WelcomeModel is the landing screen with the main menu.
type WelcomeModel struct {
	cursor int
	width  int
	height int
	// Fields collected from the user before starting a run
	sourceDir  string
	destDir    string
	cookieJar  string
	inputJSON  string
	inputPhase inputPhase
}

type inputPhase int

const (
	phaseMenu inputPhase = iota
	phaseSourceDir
	phaseDestDir
	phaseCookieJar
	phaseInputJSON
)

type menuItem struct {
	label string
	desc  string
}

var menuItems = []menuItem{
	{"Process Takeout", "Import and organize a Google Photos Takeout archive"},
	{"Recover Missing", "Download missing files using Chrome cookies"},
	{"Quit", "Exit revoco"},
}

// NewWelcomeModel returns a fresh welcome screen.
func NewWelcomeModel() WelcomeModel {
	return WelcomeModel{inputPhase: phaseMenu}
}

// Init implements tea.Model.
func (m WelcomeModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m WelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.inputPhase {
		case phaseMenu:
			return m.updateMenu(msg)
		case phaseSourceDir, phaseDestDir, phaseCookieJar, phaseInputJSON:
			return m.updateInput(msg)
		}
	}
	return m, nil
}

func (m WelcomeModel) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(menuItems)-1 {
			m.cursor++
		}
	case "enter", " ":
		switch m.cursor {
		case 0: // Process Takeout
			m.inputPhase = phaseSourceDir
			m.sourceDir = ""
		case 1: // Recover Missing
			m.inputPhase = phaseCookieJar
			m.cookieJar = ""
		case 2: // Quit
			return m, tea.Quit
		}
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m WelcomeModel) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		switch m.inputPhase {
		case phaseSourceDir:
			m.inputPhase = phaseDestDir
		case phaseDestDir:
			// Launch process
			pm := NewProcessModel(m.sourceDir, m.destDir, m.width, m.height)
			return m, func() tea.Msg {
				return SwitchScreenMsg{To: ScreenProcess, Process: &pm}
			}
		case phaseCookieJar:
			m.inputPhase = phaseInputJSON
		case phaseInputJSON:
			rm := NewRecoverModel(m.cookieJar, m.inputJSON, m.width, m.height)
			return m, func() tea.Msg {
				return SwitchScreenMsg{To: ScreenRecover, Recover: &rm}
			}
		}
	case "esc":
		m.inputPhase = phaseMenu
	case "backspace":
		switch m.inputPhase {
		case phaseSourceDir:
			if len(m.sourceDir) > 0 {
				m.sourceDir = m.sourceDir[:len(m.sourceDir)-1]
			}
		case phaseDestDir:
			if len(m.destDir) > 0 {
				m.destDir = m.destDir[:len(m.destDir)-1]
			}
		case phaseCookieJar:
			if len(m.cookieJar) > 0 {
				m.cookieJar = m.cookieJar[:len(m.cookieJar)-1]
			}
		case phaseInputJSON:
			if len(m.inputJSON) > 0 {
				m.inputJSON = m.inputJSON[:len(m.inputJSON)-1]
			}
		}
	default:
		ch := msg.String()
		if len(ch) == 1 {
			switch m.inputPhase {
			case phaseSourceDir:
				m.sourceDir += ch
			case phaseDestDir:
				m.destDir += ch
			case phaseCookieJar:
				m.cookieJar += ch
			case phaseInputJSON:
				m.inputJSON += ch
			}
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m WelcomeModel) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("revoco"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Google Photos Takeout processor & recovery tool"))
	sb.WriteString("\n\n")

	switch m.inputPhase {
	case phaseMenu:
		for i, item := range menuItems {
			cursor := "  "
			style := menuItemStyle
			if i == m.cursor {
				cursor = "▶ "
				style = menuSelectedStyle
			}
			sb.WriteString(style.Render(fmt.Sprintf("%s%s", cursor, item.label)))
			sb.WriteString("\n")
			sb.WriteString(helpStyle.Render(fmt.Sprintf("    %s", item.desc)))
			sb.WriteString("\n\n")
		}
		sb.WriteString(helpStyle.Render("↑/↓ navigate • enter select • q quit"))

	case phaseSourceDir:
		sb.WriteString(subtitleStyle.Render("Process Takeout"))
		sb.WriteString("\n\n")
		sb.WriteString(menuItemStyle.Render("Source directory (Takeout root):"))
		sb.WriteString("\n")
		sb.WriteString(menuSelectedStyle.Render("  > " + m.sourceDir + "█"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("enter to continue • esc to go back"))

	case phaseDestDir:
		sb.WriteString(subtitleStyle.Render("Process Takeout"))
		sb.WriteString("\n\n")
		sb.WriteString(menuItemStyle.Render(fmt.Sprintf("Source: %s", m.sourceDir)))
		sb.WriteString("\n")
		sb.WriteString(menuItemStyle.Render("Destination directory:"))
		sb.WriteString("\n")
		sb.WriteString(menuSelectedStyle.Render("  > " + m.destDir + "█"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("enter to start • esc to go back"))

	case phaseCookieJar:
		sb.WriteString(subtitleStyle.Render("Recover Missing Files"))
		sb.WriteString("\n\n")
		sb.WriteString(menuItemStyle.Render("Cookie jar path (Netscape format):"))
		sb.WriteString("\n")
		sb.WriteString(menuSelectedStyle.Render("  > " + m.cookieJar + "█"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("enter to continue • esc to go back"))

	case phaseInputJSON:
		sb.WriteString(subtitleStyle.Render("Recover Missing Files"))
		sb.WriteString("\n\n")
		sb.WriteString(menuItemStyle.Render(fmt.Sprintf("Cookies: %s", m.cookieJar)))
		sb.WriteString("\n")
		sb.WriteString(menuItemStyle.Render("missing-files.json path:"))
		sb.WriteString("\n")
		sb.WriteString(menuSelectedStyle.Render("  > " + m.inputJSON + "█"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("enter to start • esc to go back"))
	}

	return sb.String()
}
