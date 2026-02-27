package components

import (
	"os"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbletea"
)

// PickerMode controls what the filepicker allows the user to select.
type PickerMode int

const (
	// ModeDir only allows directory selection (for source/dest folder inputs).
	ModeDir PickerMode = iota
	// ModeFile allows file selection (for JSON/cookie file inputs).
	ModeFile
)

// FilePicker wraps bubbles/filepicker with a simpler API for the revoco TUI.
// After the user confirms a selection, Selected is set to the chosen path and
// Done is true.
type FilePicker struct {
	Mode     PickerMode
	Selected string
	Done     bool
	Err      error

	fp filepicker.Model
}

// NewFilePicker creates a FilePicker starting in the user's home directory
// (or the current directory if home cannot be determined).
func NewFilePicker(mode PickerMode) FilePicker {
	startDir, err := os.UserHomeDir()
	if err != nil {
		startDir = "."
	}

	fp := filepicker.New()
	fp.CurrentDirectory = startDir
	fp.ShowHidden = false
	fp.AutoHeight = true

	switch mode {
	case ModeDir:
		fp.DirAllowed = true
		fp.FileAllowed = false
	case ModeFile:
		fp.DirAllowed = false
		fp.FileAllowed = true
	}

	return FilePicker{
		Mode: mode,
		fp:   fp,
	}
}

// Init returns the filepicker's initial command (reads the start directory).
func (p FilePicker) Init() tea.Cmd {
	return p.fp.Init()
}

// Update forwards messages to the inner filepicker and detects selection.
// Returns (updated FilePicker, cmd).
func (p FilePicker) Update(msg tea.Msg) (FilePicker, tea.Cmd) {
	var cmd tea.Cmd
	p.fp, cmd = p.fp.Update(msg)

	if didSelect, path := p.fp.DidSelectFile(msg); didSelect {
		p.Selected = path
		p.Done = true
	}
	if didSelect, path := p.fp.DidSelectDisabledFile(msg); didSelect {
		// For dir-only mode, a "disabled file" selection means the user opened
		// a directory — treat it as the selected path.
		if p.Mode == ModeDir && path != "" {
			p.Selected = path
			p.Done = true
		}
	}

	return p, cmd
}

// View renders the filepicker.
func (p FilePicker) View() string {
	return p.fp.View()
}

// SetHeight updates the filepicker height.
func (p *FilePicker) SetHeight(h int) {
	p.fp.SetHeight(h)
}
