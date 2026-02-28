package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fulgidus/revoco/engine"
	"github.com/fulgidus/revoco/session"
)

var (
	analyzeHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).MarginBottom(1)
	analyzeKeyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Width(26)
	analyzeValStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	analyzeErrStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	analyzeButtonStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("232")).
				Background(lipgloss.Color("205")).
				Padding(0, 2).
				MarginRight(2)
	analyzeCancelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Padding(0, 2).
				MarginRight(2)
)

// analyzeResultMsg carries the result of the background Analyze scan.
type analyzeResultMsg struct {
	result *engine.AnalysisResult
	err    error
}

// analyzeProgressMsg carries incremental progress (done, total file count).
type analyzeProgressMsg struct {
	done  int
	total int
}

// AnalyzeMode tells the analyzer which operation to proceed to on [Start].
type AnalyzeMode int

const (
	AnalyzeModeProcess AnalyzeMode = iota
)

// AnalyzeModel is the pre-flight analyzer screen.
// It runs engine.Analyze in the background while showing a spinner, then
// displays the AnalysisResult summary and [Start] / [Cancel] buttons.
type AnalyzeModel struct {
	mode AnalyzeMode

	// Inputs forwarded from the welcome screen
	sourceDir  string
	destDir    string
	cookieJar  string
	inputJSON  string
	sessionDir string // session folder for logs; empty if no session

	spin     spinner.Model
	scanning bool
	result   *engine.AnalysisResult
	err      error

	// progress during scan
	scanDone  int
	scanTotal int

	// button focus: 0 = Start, 1 = Cancel
	focus int

	width  int
	height int
}

// NewAnalyzeModel creates a new AnalyzeModel for the given mode and inputs.
// sessionDir is the session folder for logs (empty string if no active session).
func NewAnalyzeModel(mode AnalyzeMode, source, dest, cookieJar, inputJSON, sessionDir string, width, height int) AnalyzeModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return AnalyzeModel{
		mode:       mode,
		sourceDir:  source,
		destDir:    dest,
		cookieJar:  cookieJar,
		inputJSON:  inputJSON,
		sessionDir: sessionDir,
		spin:       sp,
		scanning:   true,
		width:      width,
		height:     height,
	}
}

// NewAnalyzeModelFromSession creates an AnalyzeModel for a connector-based session.
// It analyzes the session's data directory where retrieved data is stored.
func NewAnalyzeModelFromSession(sess *session.Session) AnalyzeModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := AnalyzeModel{
		mode:     AnalyzeModeProcess,
		spin:     sp,
		scanning: true,
	}

	if sess != nil {
		m.sourceDir = sess.DataDir()
		m.destDir = sess.OutputPath()
		m.sessionDir = sess.Dir
	}

	return m
}

// Init implements tea.Model — starts the analyzer goroutine and the spinner.
func (m AnalyzeModel) Init() tea.Cmd {
	src := m.sourceDir
	return tea.Batch(
		m.spin.Tick,
		func() tea.Msg {
			// Run analysis; report progress via a channel trick using a
			// synchronous callback (progress is best-effort; we batch updates).
			result, err := engine.Analyze(src, nil)
			return analyzeResultMsg{result: result, err: err}
		},
	)
}

// Update implements tea.Model.
func (m AnalyzeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if m.scanning {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case analyzeResultMsg:
		m.scanning = false
		m.result = msg.result
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			m.focus = 0
		case "right", "l", "tab":
			m.focus = 1
		case "enter", " ":
			return m.activate()
		case "esc", "q":
			return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenDashboard} }
		}
	}
	return m, nil
}

func (m AnalyzeModel) activate() (tea.Model, tea.Cmd) {
	if m.focus == 1 || m.err != nil && m.focus == 0 {
		// Cancel — back to dashboard
		return m, func() tea.Msg { return SwitchScreenMsg{To: ScreenDashboard} }
	}
	// Start — launch the actual operation
	switch m.mode {
	case AnalyzeModeProcess:
		pm := NewProcessModel(m.sourceDir, m.destDir, m.sessionDir, m.width, m.height)
		return m, func() tea.Msg {
			return SwitchScreenMsg{To: ScreenProcess, Process: &pm}
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m AnalyzeModel) View() string {
	var sb strings.Builder

	sb.WriteString(analyzeHeaderStyle.Render("Pre-flight Analysis"))
	sb.WriteString("\n")
	sb.WriteString(analyzeKeyStyle.Render("Source"))
	sb.WriteString(analyzeValStyle.Render(m.sourceDir))
	sb.WriteString("\n\n")

	if m.scanning {
		sb.WriteString(m.spin.View())
		sb.WriteString("  Scanning source directory…")
		sb.WriteString("\n")
		return sb.String()
	}

	if m.err != nil {
		sb.WriteString(analyzeErrStyle.Render("Analysis failed: " + m.err.Error()))
		sb.WriteString("\n\n")
		sb.WriteString(analyzeCancelStyle.Render("[ Cancel ]"))
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("esc to return"))
		return sb.String()
	}

	r := m.result
	rows := [][2]string{
		{"Media files", fmt.Sprintf("%d", r.TotalMedia)},
		{"JSON sidecar files", fmt.Sprintf("%d", r.TotalJSON)},
		{"Match rate", fmt.Sprintf("%.0f%%", r.MatchRate*100)},
		{"Albums", fmt.Sprintf("%d", len(r.Albums))},
		{"Motion photos", fmt.Sprintf("%d", r.MotionPhotos)},
		{"Videos", fmt.Sprintf("%d", r.Videos)},
		{"Estimated size", humanizeBytes(r.TotalBytes)},
	}
	if !r.EarliestDate.IsZero() {
		rows = append(rows, [2]string{"Date range",
			r.EarliestDate.Format("2006-01-02") + " → " + r.LatestDate.Format("2006-01-02"),
		})
	}
	for _, row := range rows {
		sb.WriteString(analyzeKeyStyle.Render(row[0]))
		sb.WriteString(analyzeValStyle.Render(row[1]))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Buttons
	startLabel := "[ Start ]"
	cancelLabel := "[ Cancel ]"
	if m.focus == 0 {
		sb.WriteString(analyzeButtonStyle.Render(startLabel))
		sb.WriteString(analyzeCancelStyle.Render(cancelLabel))
	} else {
		sb.WriteString(analyzeButtonStyle.Copy().Background(lipgloss.Color("240")).Render(startLabel))
		sb.WriteString(analyzeButtonStyle.Copy().
			Background(lipgloss.Color("196")).
			Render(cancelLabel))
	}
	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render("←/→ select • enter confirm • esc cancel"))

	return sb.String()
}

// humanizeBytes returns a human-readable byte string (e.g. "4.2 GB").
func humanizeBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// analyzeTickMsg is unused but reserved for future progress reporting.
type analyzeTickMsg time.Time
