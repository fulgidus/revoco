package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fulgidus/revoco/tui/components"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	// menuItemStyle / menuSelectedStyle: NO PaddingLeft — prefix string handles indent.
	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	menuSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205"))

	// descStyle: plain, no MarginTop so descriptions sit tight under items.
	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("238"))

	panelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238"))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	browseButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("232")).
				Background(lipgloss.Color("63")).
				Padding(0, 1)

	selectedHighlightStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				Background(lipgloss.Color("57"))
)

// ── Menu items ────────────────────────────────────────────────────────────────

type menuItem struct {
	label string
	desc  string
}

var menuItems = []menuItem{
	{"Process Takeout", "Import and organise a Google Photos Takeout archive"},
	{"Recover Missing", "Download missing files using Chrome cookies"},
	{"Quit", "Exit revoco"},
}

// ── Layout constants ──────────────────────────────────────────────────────────

// menuItemStartRow returns the terminal row (0-indexed) where menu item i
// begins, assuming the left panel starts at row 0 of the terminal.
//
// Left panel interior layout (rows inside the border):
//
//	row 0: title
//	row 1: blank (MarginBottom(1) on titleStyle)
//	row 2: subtitle ("v0.1.0")
//	row 3: blank line
//	row 4+: menu items, 2 rows each (label + desc)
//
// The rounded border adds 1 row at the top, so terminal row = interior + 1.
func menuItemStartRow(i int) int {
	return 1 + 4 + i*2 // top_border(1) + header_rows(4) + item_offset
}

// ── WelcomeModel ──────────────────────────────────────────────────────────────

// configField identifies which text input is currently focused in the config panel.
type configField int

const (
	fieldSourceDir configField = iota
	fieldDestDir
	fieldCookieJar
	fieldInputJSON
)

// WelcomeModel is the landing screen with a two-panel layout.
// Left panel: clickable menu. Right panel: configuration for the selected item.
// A filepicker overlay slides in when the user clicks [Browse] or presses ctrl+o.
type WelcomeModel struct {
	cursor int
	width  int
	height int

	// Config inputs (one per required field)
	inputs  [4]textinput.Model // sourceDir, destDir, cookieJar, inputJSON
	focused configField

	// Whether the right config panel is shown (true once a non-Quit item is active)
	showRight bool

	// Filepicker overlay
	pickerOpen  bool
	pickerField configField // which field the picker will fill
	picker      components.FilePicker
}

// NewWelcomeModel returns a fresh welcome screen.
func NewWelcomeModel() WelcomeModel {
	inputs := [4]textinput.Model{}
	placeholders := []string{
		"Takeout root directory",
		"Destination directory",
		"Cookie jar file (.txt Netscape format)",
		"missing-files.json path",
	}
	for i := range inputs {
		ti := textinput.New()
		ti.Placeholder = placeholders[i]
		ti.CharLimit = 512
		inputs[i] = ti
	}
	inputs[0].Focus()
	return WelcomeModel{
		inputs: inputs,
	}
}

// Init implements tea.Model.
func (m WelcomeModel) Init() tea.Cmd { return textinput.Blink }

// ── Update ────────────────────────────────────────────────────────────────────

func (m WelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If filepicker overlay is open, forward all messages to it first.
	if m.pickerOpen {
		return m.updatePicker(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			return m.handleClick(msg.X, msg.Y)
		}

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward to focused input
	return m.updateActiveInput(msg)
}

func (m WelcomeModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "q":
		if !m.showRight {
			return m, tea.Quit
		}
		// q while config is open goes back to menu view
		m.showRight = false
		return m, nil

	case "esc":
		if m.showRight {
			m.showRight = false
			return m, nil
		}
		return m, tea.Quit

	case "up", "k":
		if !m.showRight {
			if m.cursor > 0 {
				m.cursor--
			}
		} else {
			m.blurAll()
			if int(m.focused) > 0 {
				m.focused--
			}
			m.inputs[m.focused].Focus()
		}
		return m, nil

	case "down", "j":
		if !m.showRight {
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		} else {
			m.blurAll()
			maxF := m.maxField()
			if int(m.focused) < maxF {
				m.focused++
			}
			m.inputs[m.focused].Focus()
		}
		return m, nil

	case "tab":
		if m.showRight {
			m.blurAll()
			maxF := m.maxField()
			next := int(m.focused) + 1
			if next > maxF {
				next = 0
			}
			m.focused = configField(next)
			m.inputs[m.focused].Focus()
		}
		return m, nil

	case "ctrl+o": // open filepicker for focused field
		if m.showRight {
			cmd := m.openPicker(m.focused)
			return m, cmd
		}
		return m, nil

	case "enter":
		if !m.showRight {
			return m.activateMenuItem()
		}
		// On the last field, advance to pre-flight
		if int(m.focused) == m.maxField() {
			return m.launchAnalyze()
		}
		// Otherwise advance to next field
		m.blurAll()
		m.focused++
		m.inputs[m.focused].Focus()
		return m, nil
	}

	return m.updateActiveInput(msg)
}

// handleClick handles left-button mouse clicks anywhere on screen.
//
// Hit regions:
//  1. Menu items — detected mathematically via menuItemStartRow(i).
//     Each item occupies 2 rows (label + desc). Left panel starts at x=0.
//     Left border is at x=0, interior starts at x=1.
//  2. [Browse] button — detected when right panel is shown and click is in
//     the right half of the terminal at the known button Y positions.
func (m WelcomeModel) handleClick(x, y int) (tea.Model, tea.Cmd) {
	leftOuterW := m.leftWidth()

	// ── Menu item clicks ──────────────────────────────────────────────────────
	// The left panel spans x: 0 .. leftOuterW-1
	if x < leftOuterW {
		for i := range menuItems {
			startRow := menuItemStartRow(i)
			if y == startRow || y == startRow+1 {
				m.cursor = i
				return m.activateMenuItem()
			}
		}
	}

	// ── [Browse] button clicks ────────────────────────────────────────────────
	// Only relevant when the right config panel is visible.
	if m.showRight && x >= leftOuterW {
		// Right panel interior starts at x = leftOuterW + 1 (gap) + 1 (border) = leftOuterW+2
		// Config panel layout inside right panel:
		//   row 0 (interior): title
		//   row 1: blank
		//   row 2: field 0 label
		//   row 3: field 0 input + [Browse]   ← clickable
		//   row 4: blank
		//   row 5: field 1 label
		//   row 6: field 1 input + [Browse]   ← clickable
		//
		// Terminal row = 1 (top border) + interior_row
		// So field-0 [Browse] is at terminal row 4, field-1 at row 7.

		firstField := configField(0)
		if m.cursor == 1 {
			firstField = fieldCookieJar
		}

		browseRows := [2]int{
			1 + 1 + 2,     // top_border + title_rows(2) + label(1) = row 4 inside terminal (top_border=1, title+blank=2, label=1 → input row=4)
			1 + 1 + 2 + 3, // same + gap(1) + label(1) + input: row 7
		}

		for i, brow := range browseRows {
			if y == brow {
				targetField := configField(int(firstField) + i)
				if int(targetField) <= m.maxField() {
					cmd := m.openPicker(targetField)
					return m, cmd
				}
			}
		}
	}

	return m, nil
}

func (m WelcomeModel) activateMenuItem() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0: // Process Takeout
		m.showRight = true
		m.focused = fieldSourceDir
		m.blurAll()
		m.inputs[fieldSourceDir].Focus()
	case 1: // Recover Missing
		m.showRight = true
		m.focused = fieldCookieJar
		m.blurAll()
		m.inputs[fieldCookieJar].Focus()
	case 2: // Quit
		return m, tea.Quit
	}
	return m, nil
}

func (m WelcomeModel) launchAnalyze() (tea.Model, tea.Cmd) {
	source := m.inputs[fieldSourceDir].Value()
	dest := m.inputs[fieldDestDir].Value()
	cookieJar := m.inputs[fieldCookieJar].Value()
	inputJSON := m.inputs[fieldInputJSON].Value()

	var mode AnalyzeMode
	if m.cursor == 0 {
		mode = AnalyzeModeProcess
	} else {
		mode = AnalyzeModeRecover
	}

	am := NewAnalyzeModel(mode, source, dest, cookieJar, inputJSON, m.width, m.height)
	return m, func() tea.Msg {
		return SwitchScreenMsg{To: ScreenAnalyze, Analyze: &am}
	}
}

// updatePicker forwards events to the filepicker and handles selection.
func (m WelcomeModel) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.pickerOpen = false
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	if m.picker.Done {
		m.inputs[m.pickerField].SetValue(m.picker.Selected)
		m.pickerOpen = false
	}
	return m, cmd
}

func (m WelcomeModel) updateActiveInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.showRight {
		return m, nil
	}
	var cmd tea.Cmd
	m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
	return m, cmd
}

// openPicker opens the filepicker overlay for a given field.
func (m *WelcomeModel) openPicker(field configField) tea.Cmd {
	var mode components.PickerMode
	if field == fieldCookieJar || field == fieldInputJSON {
		mode = components.ModeFile
	} else {
		mode = components.ModeDir
	}
	m.picker = components.NewFilePicker(mode)
	m.pickerField = field
	m.pickerOpen = true
	return m.picker.Init()
}

// blurAll removes focus from all inputs.
func (m *WelcomeModel) blurAll() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
}

// maxField returns the index of the last visible config field for the active menu item.
func (m WelcomeModel) maxField() int {
	if m.cursor == 0 {
		return int(fieldDestDir)
	}
	return int(fieldInputJSON)
}

// leftWidth returns the *outer* width (including border) of the left menu panel.
func (m WelcomeModel) leftWidth() int {
	w := m.width * 40 / 100
	if w < 26 {
		w = 26
	}
	return w
}

// useTwoPanel returns true when the terminal is wide enough for a two-panel layout.
func (m WelcomeModel) useTwoPanel() bool {
	return m.width >= 60
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m WelcomeModel) View() string {
	if m.pickerOpen {
		return m.viewPicker()
	}
	if m.useTwoPanel() {
		return m.viewTwoPanel()
	}
	return m.viewSinglePanel()
}

func (m WelcomeModel) viewPicker() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Select path"))
	sb.WriteString("\n\n")
	sb.WriteString(m.picker.View())
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("↑/↓/enter navigate • esc cancel"))
	return sb.String()
}

func (m WelcomeModel) viewSinglePanel() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("revoco"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Google Photos Takeout processor & recovery tool"))
	sb.WriteString("\n\n")

	for i, item := range menuItems {
		prefix := "  "
		style := menuItemStyle
		if i == m.cursor {
			prefix = "▶ "
			style = menuSelectedStyle
		}
		sb.WriteString(style.Render(prefix + item.label))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("  " + item.desc))
		sb.WriteString("\n\n")
	}
	sb.WriteString(helpStyle.Render("↑/↓ navigate • enter select • q quit"))
	return sb.String()
}

func (m WelcomeModel) viewTwoPanel() string {
	// Outer widths (including borders).
	leftOuterW := m.leftWidth()
	rightOuterW := m.width - leftOuterW - 1 // 1-char gap between panels
	if rightOuterW < 22 {
		rightOuterW = 22
	}

	// Inner content widths (subtract 2 for left+right border chars).
	leftInnerW := leftOuterW - 2
	rightInnerW := rightOuterW - 2

	panelH := m.height - 2
	if panelH < 10 {
		panelH = 10
	}

	// ── Left panel: menu ──────────────────────────────────────────────────────
	leftContent := m.viewMenuPanel(leftInnerW)
	leftPanel := panelBorderStyle.
		Width(leftInnerW).
		Height(panelH).
		Render(leftContent)

	// ── Right panel: configuration ────────────────────────────────────────────
	rightContent := m.viewConfigPanel(rightInnerW)
	rightPanel := panelBorderStyle.
		Width(rightInnerW).
		Height(panelH).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)
}

// viewMenuPanel renders the left-panel interior content.
func (m WelcomeModel) viewMenuPanel(innerW int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("revoco"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("v0.1.0"))
	sb.WriteString("\n\n")

	for i, item := range menuItems {
		if i == m.cursor {
			// Full-width highlight bar for the selected item.
			label := "▶ " + item.label
			// Pad to innerW so the background stretches.
			for lipgloss.Width(label) < innerW {
				label += " "
			}
			sb.WriteString(selectedHighlightStyle.Render(label))
		} else {
			sb.WriteString(menuItemStyle.Render("  " + item.label))
		}
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("  " + item.desc))
		sb.WriteString("\n\n")
	}
	sb.WriteString(helpStyle.Render("↑/↓ • enter • q"))
	return sb.String()
}

func (m WelcomeModel) viewConfigPanel(w int) string {
	var sb strings.Builder

	if !m.showRight {
		// Placeholder before any item is selected.
		sb.WriteString(subtitleStyle.Render("Select an action"))
		sb.WriteString("\n\n")
		sb.WriteString(descStyle.Render("Use ↑/↓ to navigate the menu,"))
		sb.WriteString("\n")
		sb.WriteString(descStyle.Render("then press enter or click to configure."))
		return sb.String()
	}

	switch m.cursor {
	case 0: // Process Takeout
		sb.WriteString(subtitleStyle.Render("Process Takeout"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderField(fieldSourceDir, "Source directory", w))
		sb.WriteString("\n")
		sb.WriteString(m.renderField(fieldDestDir, "Destination directory", w))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("tab/↑↓ navigate • ctrl+o browse • enter proceed"))

	case 1: // Recover Missing
		sb.WriteString(subtitleStyle.Render("Recover Missing Files"))
		sb.WriteString("\n\n")
		sb.WriteString(m.renderField(fieldCookieJar, "Cookie jar path", w))
		sb.WriteString("\n")
		sb.WriteString(m.renderField(fieldInputJSON, "missing-files.json path", w))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("tab/↑↓ navigate • ctrl+o browse • enter proceed"))

	case 2: // Quit
		sb.WriteString(subtitleStyle.Render("Quit"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("Press enter to exit revoco."))
	}

	return sb.String()
}

func (m WelcomeModel) renderField(field configField, label string, _ int) string {
	var sb strings.Builder
	sb.WriteString(labelStyle.Render(label))
	sb.WriteString("\n")
	sb.WriteString(m.inputs[field].View())
	if field == m.focused {
		sb.WriteString("  ")
		sb.WriteString(browseButtonStyle.Render("[Browse]"))
	}
	sb.WriteString("\n")
	return sb.String()
}
