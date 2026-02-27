package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	phaseActive  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	phaseDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	phasePending = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	barFilled    = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	barEmpty     = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
	percentStyle = lipgloss.NewStyle().Bold(true)
)

// PhaseBar renders a compact progress bar with label, filled/empty blocks, and percent.
type PhaseBar struct {
	Label    string
	Done     int
	Total    int
	Width    int
	Active   bool
	Finished bool
}

// View returns the rendered string of the phase bar.
func (p PhaseBar) View() string {
	label := p.Label
	switch {
	case p.Finished:
		label = phaseDone.Render("✓ " + label)
	case p.Active:
		label = phaseActive.Render("▶ " + label)
	default:
		label = phasePending.Render("  " + label)
	}

	barWidth := p.Width - len([]rune(p.Label)) - 16
	if barWidth < 4 {
		barWidth = 4
	}

	var pct int
	if p.Total > 0 {
		pct = p.Done * 100 / p.Total
	} else if p.Finished {
		pct = 100
	}

	filled := barWidth * pct / 100
	empty := barWidth - filled

	bar := barFilled.Render(strings.Repeat("█", filled)) +
		barEmpty.Render(strings.Repeat("░", empty))

	return fmt.Sprintf("%s %s %s",
		label,
		bar,
		percentStyle.Render(fmt.Sprintf("%3d%%", pct)),
	)
}
