package components

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	phaseActiveStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	phaseDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	phasePendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	phasePercentStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
)

// PhaseBar renders a progress bar for a single pipeline phase using
// bubbles/progress.ViewAs for smooth animated fills.
type PhaseBar struct {
	Label    string
	Done     int
	Total    int
	Width    int
	Active   bool
	Finished bool

	prog     progress.Model
	spin     spinner.Model
	initDone bool
}

// Init should be called once on the PhaseBar when it becomes active.
// Returns a tea.Cmd that drives the spinner animation.
func (p *PhaseBar) Init() tea.Cmd {
	if p.initDone {
		return nil
	}
	p.prog = progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)
	p.prog.Width = p.barWidth()

	p.spin = spinner.New()
	p.spin.Spinner = spinner.Dot
	p.spin.Style = phaseActiveStyle
	p.initDone = true
	return p.spin.Tick
}

// UpdateSpinner advances the spinner state. Call from the parent Update on
// spinner.TickMsg.
func (p *PhaseBar) UpdateSpinner(msg tea.Msg) tea.Cmd {
	if !p.Active || p.Finished {
		return nil
	}
	var cmd tea.Cmd
	p.spin, cmd = p.spin.Update(msg)
	return cmd
}

// barWidth calculates the available width for the progress bar segment.
func (p *PhaseBar) barWidth() int {
	// prefix(2) + label(18) + space(1) + bar + space(1) + percent(4) = 26 overhead
	w := p.Width - 26
	if w < 8 {
		w = 8
	}
	return w
}

// View renders the phase bar line.
func (p PhaseBar) View() string {
	p.prog.Width = p.barWidth()

	// Prefix symbol
	var prefix string
	switch {
	case p.Finished:
		prefix = phaseDoneStyle.Render("✓ ")
	case p.Active:
		if p.initDone {
			prefix = p.spin.View() + " "
		} else {
			prefix = phaseActiveStyle.Render("▶ ")
		}
	default:
		prefix = phasePendingStyle.Render("  ")
	}

	// Label fixed 18 chars
	label := p.Label
	if len([]rune(label)) > 18 {
		label = string([]rune(label)[:17]) + "…"
	}
	var labelStr string
	switch {
	case p.Finished:
		labelStr = phaseDoneStyle.Render(fmt.Sprintf("%-18s", label))
	case p.Active:
		labelStr = phaseActiveStyle.Render(fmt.Sprintf("%-18s", label))
	default:
		labelStr = phasePendingStyle.Render(fmt.Sprintf("%-18s", label))
	}

	// Percentage float
	var pct float64
	if p.Total > 0 {
		pct = float64(p.Done) / float64(p.Total)
		if pct > 1.0 {
			pct = 1.0
		}
	} else if p.Finished {
		pct = 1.0
	}

	bar := p.prog.ViewAs(pct)
	pctStr := phasePercentStyle.Render(fmt.Sprintf("%3d%%", int(pct*100)))

	return prefix + labelStr + " " + bar + " " + pctStr
}
