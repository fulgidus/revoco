package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fulgidus/revoco/config"
)

// ── Messages ─────────────────────────────────────────────────────────────────

type firstRunCompletedMsg struct {
	err error
}

// ── Model ────────────────────────────────────────────────────────────────────

// FirstRunModel is shown once when no config file exists.
type FirstRunModel struct {
	width  int
	height int
	err    error

	// Channel selection
	cursor  int
	options []channelOption
}

// NewFirstRunModel creates the first-run channel selection prompt.
func NewFirstRunModel() FirstRunModel {
	return FirstRunModel{
		cursor:  0, // default to stable
		options: channelOptions,
	}
}

func (m FirstRunModel) Init() tea.Cmd {
	return nil
}

func (m FirstRunModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case firstRunCompletedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// First-run complete, switch to sessions screen
		return m, func() tea.Msg {
			return SwitchScreenMsg{To: ScreenSessions}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
			return m, nil

		case "enter":
			return m, m.saveAndContinue()
		}
	}

	return m, nil
}

func (m FirstRunModel) saveAndContinue() tea.Cmd {
	return func() tea.Msg {
		// Create default config with selected channel
		cfg := config.DefaultConfig()
		cfg.Updates.Channel = m.options[m.cursor].value

		if err := cfg.Save(); err != nil {
			return firstRunCompletedMsg{err: fmt.Errorf("failed to save config: %w", err)}
		}

		return firstRunCompletedMsg{err: nil}
	}
}

func (m FirstRunModel) View() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("Welcome to revoco!"))
	b.WriteString("\n\n")

	// Subtitle
	b.WriteString(subtitleStyle.Render("Select your update channel preference:"))
	b.WriteString("\n\n")

	// Error display
	if m.err != nil {
		b.WriteString(dangerStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	// Channel options
	for i, opt := range m.options {
		cursor := "  "
		style := itemStyle
		bullet := "○"

		if i == m.cursor {
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

	// Help text
	help := "↑↓/j/k navigate  enter confirm"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}
