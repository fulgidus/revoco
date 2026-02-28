package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	logNormalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	logErrorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	logBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))
)

// LogLine is a single entry in the log panel.
type LogLine struct {
	Text    string
	IsError bool
}

// LogPanel is a scrollable log view backed by bubbles/viewport with
// auto-follow when at the bottom.
type LogPanel struct {
	Width    int
	Height   int
	MaxLines int

	lines    []LogLine
	vp       viewport.Model
	atBottom bool
}

// NewLogPanel creates a LogPanel with the given dimensions and max line capacity.
func NewLogPanel(width, height, maxLines int) LogPanel {
	vp := viewport.New(width, height)
	return LogPanel{
		Width:    width,
		Height:   height,
		MaxLines: maxLines,
		vp:       vp,
		atBottom: true,
	}
}

// Append adds a line, evicting the oldest if over capacity, and refreshes content.
func (l *LogPanel) Append(text string, isErr bool) {
	l.lines = append(l.lines, LogLine{Text: text, IsError: isErr})
	if len(l.lines) > l.MaxLines {
		l.lines = l.lines[len(l.lines)-l.MaxLines:]
	}
	l.vp.SetContent(l.buildContent())
	if l.atBottom {
		l.vp.GotoBottom()
	}
}

// Resize updates the viewport dimensions.
func (l *LogPanel) Resize(width, height int) {
	l.Width = width
	l.Height = height
	l.vp.Width = width
	l.vp.Height = height
	l.vp.SetContent(l.buildContent())
}

// Update handles keyboard events for the viewport.
func (l *LogPanel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	l.atBottom = l.vp.AtBottom()
	return cmd
}

// View renders the log panel with a rounded border.
func (l LogPanel) View() string {
	l.vp.Width = l.Width
	l.vp.Height = l.Height
	return logBorderStyle.Width(l.Width).Height(l.Height).Render(l.vp.View())
}

func (l *LogPanel) buildContent() string {
	var sb strings.Builder
	for _, line := range l.lines {
		text := line.Text
		if l.Width > 6 && len([]rune(text)) > l.Width-6 {
			runes := []rune(text)
			text = string(runes[:l.Width-9]) + "..."
		}
		if line.IsError {
			sb.WriteString(logErrorStyle.Render(text))
		} else {
			sb.WriteString(logNormalStyle.Render(text))
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
