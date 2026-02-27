package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	logNormal = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	logError  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	logBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))
)

// LogPanel is a scrolling log view that shows the last N lines.
type LogPanel struct {
	Lines    []LogLine
	MaxLines int
	Width    int
	Height   int
}

// LogLine is a single entry in the log panel.
type LogLine struct {
	Text    string
	IsError bool
}

// NewLogPanel creates a LogPanel with the given dimensions and max capacity.
func NewLogPanel(width, height, maxLines int) LogPanel {
	return LogPanel{Width: width, Height: height, MaxLines: maxLines}
}

// Append adds a line to the panel, evicting the oldest if over capacity.
func (l *LogPanel) Append(text string, isErr bool) {
	l.Lines = append(l.Lines, LogLine{Text: text, IsError: isErr})
	if len(l.Lines) > l.MaxLines {
		l.Lines = l.Lines[len(l.Lines)-l.MaxLines:]
	}
}

// View renders the log panel, showing the last Height lines.
func (l LogPanel) View() string {
	visible := l.Lines
	if len(visible) > l.Height {
		visible = visible[len(visible)-l.Height:]
	}

	var sb strings.Builder
	for _, line := range visible {
		text := line.Text
		if len([]rune(text)) > l.Width-4 {
			runes := []rune(text)
			text = string(runes[:l.Width-7]) + "..."
		}
		if line.IsError {
			sb.WriteString(logError.Render(text))
		} else {
			sb.WriteString(logNormal.Render(text))
		}
		sb.WriteString("\n")
	}

	content := strings.TrimRight(sb.String(), "\n")
	return logBorder.Width(l.Width).Height(l.Height).Render(content)
}
