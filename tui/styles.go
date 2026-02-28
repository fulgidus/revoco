package tui

import "github.com/charmbracelet/lipgloss"

// ══════════════════════════════════════════════════════════════════════════════
// Shared TUI Styles
// ══════════════════════════════════════════════════════════════════════════════

var (
	// ── Title & Headers ───────────────────────────────────────────────────────
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			Italic(true)

	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	// ── List Items ────────────────────────────────────────────────────────────
	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Background(lipgloss.Color("57"))

	// ── Text Styles ───────────────────────────────────────────────────────────
	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	// ── Status Styles ─────────────────────────────────────────────────────────
	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208"))

	dangerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	// ── Action Bar ────────────────────────────────────────────────────────────
	actionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("63"))

	// ── Boxes & Panels ────────────────────────────────────────────────────────
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(1, 2)

	panelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238"))

	// ── Prompt Styles ─────────────────────────────────────────────────────────
	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	// ── Role Badge Colors ─────────────────────────────────────────────────────
	inputBadgeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("22")).
			Foreground(lipgloss.Color("46")).
			Padding(0, 1)

	outputBadgeStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("17")).
				Foreground(lipgloss.Color("39")).
				Padding(0, 1)

	fallbackBadgeStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("52")).
				Foreground(lipgloss.Color("208")).
				Padding(0, 1)

	bothBadgeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("53")).
			Foreground(lipgloss.Color("213")).
			Padding(0, 1)
)
