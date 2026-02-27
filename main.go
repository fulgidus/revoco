package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbletea"

	"github.com/fulgidus/revoco/cmd"
	"github.com/fulgidus/revoco/tui"
)

func main() {
	// If invoked with no arguments, launch the TUI.
	if cmd.NeedsTUI() {
		app := tui.NewApp()
		p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Otherwise let Cobra handle the CLI.
	cmd.Execute()
}
