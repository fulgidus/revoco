// Package components provides reusable TUI widgets for revoco.
package components

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PickerMode controls what the filepicker allows the user to select.
type PickerMode int

const (
	// ModeDir allows directory selection. Enter navigates into directories,
	// Space confirms the current directory as the selection.
	ModeDir PickerMode = iota
	// ModeFile allows file selection. Enter navigates into directories or
	// selects the highlighted file.
	ModeFile
	// ModeMultiFile allows selecting multiple files. Space toggles selection,
	// Enter confirms all selections.
	ModeMultiFile
)

// fsEntry is one item in the file listing.
type fsEntry struct {
	name  string
	isDir bool
}

// readDirDoneMsg is sent when background directory reading completes.
type readDirDoneMsg struct {
	dir     string
	entries []fsEntry
	err     error
}

// FilePicker is a keyboard-driven file/directory picker.
//
// In ModeDir the user navigates with Enter and confirms the current directory
// with Space.  In ModeFile the user navigates into directories with Enter and
// selects a file with Enter. In ModeMultiFile, Space toggles selection and
// Enter confirms all selected files.
type FilePicker struct {
	Mode          PickerMode
	Selected      string   // Single selection (ModeDir/ModeFile)
	MultiSelected []string // Multiple selections (ModeMultiFile)
	Done          bool
	Err           error

	dir     string    // current directory being browsed
	entries []fsEntry // current listing (dirs first, then files)
	cursor  int       // highlighted entry
	offset  int       // scroll viewport top
	height  int       // visible rows

	// selected tracks toggled files in ModeMultiFile (full paths)
	selected map[string]bool

	// AllowedExts filters selectable files in ModeFile/ModeMultiFile (e.g. []string{".zip"}).
	// Empty means all files are allowed.
	AllowedExts []string

	// Folder creation (ModeDir only)
	creating    bool            // true when in folder creation mode
	createInput textinput.Model // text input for new folder name
}

// NewFilePicker creates a FilePicker starting in the user's home directory.
func NewFilePicker(mode PickerMode) FilePicker {
	startDir, _ := os.UserHomeDir()
	if startDir == "" {
		startDir = "."
	}

	ti := textinput.New()
	ti.Placeholder = "folder name"
	ti.CharLimit = 255

	return FilePicker{
		Mode:        mode,
		dir:         startDir,
		height:      20, // reasonable default; updated on WindowSizeMsg
		selected:    make(map[string]bool),
		createInput: ti,
	}
}

// Init returns the command that reads the initial directory.
func (p FilePicker) Init() tea.Cmd {
	return tea.Batch(
		p.readDirCmd(p.dir),
		textinput.Blink,
	)
}

func (p FilePicker) readDirCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		dir = filepath.Clean(dir)
		raw, err := os.ReadDir(dir)
		if err != nil {
			return readDirDoneMsg{dir: dir, err: err}
		}

		var entries []fsEntry
		for _, e := range raw {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue // skip hidden
			}
			entries = append(entries, fsEntry{name: name, isDir: e.IsDir()})
		}

		// Sort: directories first, then alphabetical
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].isDir != entries[j].isDir {
				return entries[i].isDir
			}
			return entries[i].name < entries[j].name
		})

		// Prepend ".." unless we're already at the filesystem root
		if filepath.Dir(dir) != dir {
			entries = append([]fsEntry{{name: "..", isDir: true}}, entries...)
		}

		return readDirDoneMsg{dir: dir, entries: entries, err: nil}
	}
}

// Update processes messages.
func (p FilePicker) Update(msg tea.Msg) (FilePicker, tea.Cmd) {
	// Handle folder creation mode separately
	if p.creating {
		return p.updateCreating(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.height = msg.Height - 6 // room for path header + help footer
		if p.height < 5 {
			p.height = 5
		}

	case readDirDoneMsg:
		if msg.err != nil {
			p.Err = msg.err
			return p, nil
		}
		p.dir = msg.dir
		p.entries = msg.entries
		p.cursor = 0
		p.offset = 0
		p.Err = nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
				if p.cursor < p.offset {
					p.offset = p.cursor
				}
			}
		case "down", "j":
			if p.cursor < len(p.entries)-1 {
				p.cursor++
				if p.cursor >= p.offset+p.height {
					p.offset = p.cursor - p.height + 1
				}
			}
		case "pgup":
			p.cursor -= p.height
			if p.cursor < 0 {
				p.cursor = 0
			}
			p.offset -= p.height
			if p.offset < 0 {
				p.offset = 0
			}
		case "pgdown":
			p.cursor += p.height
			if p.cursor >= len(p.entries) {
				p.cursor = len(p.entries) - 1
			}
			if p.cursor < 0 {
				p.cursor = 0
			}
			p.offset += p.height
			if p.offset+p.height > len(p.entries) {
				p.offset = len(p.entries) - p.height
				if p.offset < 0 {
					p.offset = 0
				}
			}
		case "enter", "l", "right":
			return p.handleOpen()
		case "backspace", "h", "left":
			return p.goParent()
		case " ": // Space selects the current directory in ModeDir, toggles in ModeMultiFile
			if p.Mode == ModeDir {
				p.Selected = p.dir
				p.Done = true
			} else if p.Mode == ModeMultiFile && len(p.entries) > 0 {
				e := p.entries[p.cursor]
				if !e.isDir && e.name != ".." {
					path := filepath.Join(p.dir, e.name)
					if p.matchesFilter(path) {
						if p.selected[path] {
							delete(p.selected, path)
						} else {
							p.selected[path] = true
						}
					}
				}
			}
		case "n": // Create new folder (ModeDir only)
			if p.Mode == ModeDir {
				p.creating = true
				p.createInput.SetValue("")
				p.createInput.Focus()
				return p, nil
			}
		case "~": // Jump to home
			home, _ := os.UserHomeDir()
			if home != "" {
				return p, p.readDirCmd(home)
			}
		case "/": // Jump to root
			return p, p.readDirCmd("/")
		case "g": // Jump to top of listing
			p.cursor = 0
			p.offset = 0
		case "G": // Jump to bottom of listing
			if len(p.entries) > 0 {
				p.cursor = len(p.entries) - 1
				if p.cursor >= p.offset+p.height {
					p.offset = p.cursor - p.height + 1
				}
			}
		}
	}
	return p, nil
}

// updateCreating handles input when in folder creation mode.
func (p FilePicker) updateCreating(msg tea.Msg) (FilePicker, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			p.creating = false
			p.createInput.Blur()
			return p, nil
		case "enter":
			name := strings.TrimSpace(p.createInput.Value())
			if name == "" {
				p.creating = false
				p.createInput.Blur()
				return p, nil
			}
			// Create the folder
			newPath := filepath.Join(p.dir, name)
			if err := os.MkdirAll(newPath, 0o755); err != nil {
				p.Err = fmt.Errorf("failed to create folder: %w", err)
				p.creating = false
				p.createInput.Blur()
				return p, nil
			}
			// Success - refresh the directory listing and navigate into new folder
			p.creating = false
			p.createInput.Blur()
			p.Err = nil
			return p, p.readDirCmd(newPath)
		}
	}

	var cmd tea.Cmd
	p.createInput, cmd = p.createInput.Update(msg)
	return p, cmd
}

func (p FilePicker) handleOpen() (FilePicker, tea.Cmd) {
	if len(p.entries) == 0 {
		return p, nil
	}
	e := p.entries[p.cursor]

	// ".." goes to parent
	if e.name == ".." {
		return p.goParent()
	}

	if e.isDir {
		newDir := filepath.Join(p.dir, e.name)
		return p, p.readDirCmd(newDir)
	}

	// File selection in ModeFile
	if p.Mode == ModeFile {
		path := filepath.Join(p.dir, e.name)
		if p.matchesFilter(path) {
			p.Selected = path
			p.Done = true
		}
	}

	// In ModeMultiFile, Enter confirms all selections (if any)
	if p.Mode == ModeMultiFile {
		if len(p.selected) > 0 {
			p.MultiSelected = make([]string, 0, len(p.selected))
			for path := range p.selected {
				p.MultiSelected = append(p.MultiSelected, path)
			}
			p.Done = true
		}
	}
	return p, nil
}

func (p FilePicker) goParent() (FilePicker, tea.Cmd) {
	parent := filepath.Dir(p.dir)
	if parent == p.dir {
		return p, nil // already at root
	}
	return p, p.readDirCmd(parent)
}

func (p FilePicker) matchesFilter(path string) bool {
	if len(p.AllowedExts) == 0 {
		return true
	}
	lower := strings.ToLower(path)
	for _, ext := range p.AllowedExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// SetHeight sets the visible row count.
func (p *FilePicker) SetHeight(h int) {
	p.height = h
}

// ── Styles ───────────────────────────────────────────────────────────────────

var (
	fpPathStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	fpDirStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	fpFileStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	fpDisabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	fpCursorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	fpSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82")) // green for selected
	fpHelpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	fpErrStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	fpCountStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
)

// View renders the picker.
func (p FilePicker) View() string {
	var sb strings.Builder

	// Current path
	sb.WriteString(fpPathStyle.Render(p.dir))
	sb.WriteString("\n")
	rulerLen := len(p.dir) + 2
	if rulerLen > 72 {
		rulerLen = 72
	}
	sb.WriteString(fpDisabledStyle.Render(strings.Repeat("─", rulerLen)))
	sb.WriteString("\n")

	// Show folder creation prompt
	if p.creating {
		sb.WriteString(fpCountStyle.Render("New folder name:"))
		sb.WriteString("\n")
		sb.WriteString(p.createInput.View())
		sb.WriteString("\n\n")
		sb.WriteString(fpHelpStyle.Render("enter create  esc cancel"))
		return sb.String()
	}

	// Show selection count in multi-file mode
	if p.Mode == ModeMultiFile && len(p.selected) > 0 {
		sb.WriteString(fpCountStyle.Render(fmt.Sprintf("%d file(s) selected", len(p.selected))))
		sb.WriteString("\n")
	}

	if p.Err != nil {
		sb.WriteString(fpErrStyle.Render("Error: " + p.Err.Error()))
		sb.WriteString("\n")
	}

	if len(p.entries) == 0 {
		sb.WriteString(fpDisabledStyle.Render("  (empty directory)"))
		sb.WriteString("\n")
	}

	// List entries within viewport
	end := p.offset + p.height
	if end > len(p.entries) {
		end = len(p.entries)
	}

	for i := p.offset; i < end; i++ {
		e := p.entries[i]
		fullPath := filepath.Join(p.dir, e.name)
		isSelected := p.selected[fullPath]

		// Build prefix: cursor indicator + selection checkbox
		var prefix string
		if p.Mode == ModeMultiFile && !e.isDir && e.name != ".." {
			if isSelected {
				prefix = "[x] "
			} else {
				prefix = "[ ] "
			}
		} else {
			prefix = "  "
		}
		if i == p.cursor {
			prefix = "> " + prefix[2:] // Replace leading spaces with cursor
		}

		name := e.name
		if e.isDir && name != ".." {
			name += "/"
		}

		var style lipgloss.Style
		if isSelected {
			style = fpSelectedStyle
		} else if i == p.cursor {
			style = fpCursorStyle
		} else if e.isDir {
			style = fpDirStyle
		} else if p.Mode == ModeDir {
			// Files are not selectable in directory mode
			style = fpDisabledStyle
		} else if !p.matchesFilter(filepath.Join(p.dir, e.name)) {
			style = fpDisabledStyle
		} else {
			style = fpFileStyle
		}

		sb.WriteString(style.Render(prefix + name))
		sb.WriteString("\n")
	}

	// Help footer
	sb.WriteString("\n")
	switch p.Mode {
	case ModeDir:
		sb.WriteString(fpHelpStyle.Render("enter open  space select  n new folder  backspace parent  ~ home  esc cancel"))
	case ModeMultiFile:
		sb.WriteString(fpHelpStyle.Render("space toggle  enter confirm  backspace parent  ~ home  esc cancel"))
	default:
		sb.WriteString(fpHelpStyle.Render("enter open/select  backspace parent  ~ home  esc cancel"))
	}

	return sb.String()
}
