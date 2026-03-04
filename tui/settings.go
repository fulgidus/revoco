package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fulgidus/revoco/config"
)

// ── Messages ─────────────────────────────────────────────────────────────────

type settingsSavedMsg struct {
	err error
}

// ── Model ────────────────────────────────────────────────────────────────────

type channelOption struct {
	value       string
	name        string
	description string
}

var channelOptions = []channelOption{
	{
		value:       "stable",
		name:        "Stable (Recommended)",
		description: "Official releases - thoroughly tested",
	},
	{
		value:       "dev",
		name:        "Dev (Bleeding Edge)",
		description: "Latest development builds - may be unstable",
	},
}

// SettingsModel is the screen for managing application settings.
type SettingsModel struct {
	width  int
	height int
	err    error

	// Channel selection
	channelCursor   int
	currentChannel  string
	initialChannel  string
	saved           bool
	saveMessage     string
	restartRequired bool

	// Loading
	loading bool
	spinner spinner.Model
}

// NewSettingsModel creates the settings screen.
func NewSettingsModel() SettingsModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Load current channel from config
	cfg, err := config.Load()
	currentChannel := "stable"
	if err == nil {
		currentChannel = cfg.Updates.Channel
	}

	// Find cursor position for current channel
	cursor := 0
	for i, opt := range channelOptions {
		if opt.value == currentChannel {
			cursor = i
			break
		}
	}

	return SettingsModel{
		spinner:        sp,
		channelCursor:  cursor,
		currentChannel: currentChannel,
		initialChannel: currentChannel,
	}
}

func (m SettingsModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m SettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case settingsSavedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.saved = false
		} else {
			m.saved = true
			m.currentChannel = channelOptions[m.channelCursor].value
			m.restartRequired = m.currentChannel != m.initialChannel
			if m.restartRequired {
				m.saveMessage = fmt.Sprintf("Channel changed to %s. Restart revoco for changes to take effect.", m.currentChannel)
			} else {
				m.saveMessage = fmt.Sprintf("Channel set to %s (no change).", m.currentChannel)
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		switch msg.String() {
		case "q", "esc":
			return m, func() tea.Msg {
				return SwitchScreenMsg{To: ScreenDashboard}
			}

		case "up", "k":
			if m.channelCursor > 0 {
				m.channelCursor--
				m.saved = false
			}
			return m, nil

		case "down", "j":
			if m.channelCursor < len(channelOptions)-1 {
				m.channelCursor++
				m.saved = false
			}
			return m, nil

		case "enter":
			return m, m.save()
		}
	}

	return m, nil
}

func (m SettingsModel) save() tea.Cmd {
	m.loading = true
	m.err = nil
	m.saved = false

	return func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return settingsSavedMsg{err: fmt.Errorf("failed to load config: %w", err)}
		}

		cfg.Updates.Channel = channelOptions[m.channelCursor].value

		if err := cfg.Save(); err != nil {
			return settingsSavedMsg{err: fmt.Errorf("failed to save config: %w", err)}
		}

		return settingsSavedMsg{err: nil}
	}
}

func (m SettingsModel) View() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("Settings"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Configure update channel preference"))
	b.WriteString("\n\n")

	// Error display
	if m.err != nil {
		b.WriteString(dangerStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	// Section: Update Channel
	b.WriteString(labelStyle.Render("Update Channel"))
	b.WriteString("\n\n")

	// Channel options
	for i, opt := range channelOptions {
		cursor := "  "
		style := itemStyle
		bullet := "○"

		if i == m.channelCursor {
			cursor = "> "
			style = selectedStyle
			bullet = "●"
		}

		line := fmt.Sprintf("%s%s %s", cursor, bullet, opt.name)
		b.WriteString(style.Render(line))
		b.WriteString("\n")

		// Description (indented)
		desc := fmt.Sprintf("    %s", opt.description)
		b.WriteString(descStyle.Render(desc))
		b.WriteString("\n\n")
	}

	// Success message
	if m.saved && m.saveMessage != "" {
		style := successStyle
		if !m.restartRequired {
			style = descStyle
		}
		b.WriteString(style.Render("✓ " + m.saveMessage))
		b.WriteString("\n\n")
	}

	// Loading indicator
	if m.loading {
		b.WriteString(m.spinner.View())
		b.WriteString(" Saving...")
		b.WriteString("\n\n")
	}

	// Help text
	help := "↑↓/j/k navigate  enter save  esc cancel"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}
