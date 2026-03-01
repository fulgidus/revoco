package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbletea"

	"github.com/fulgidus/revoco/cmd"
	"github.com/fulgidus/revoco/plugins"
	"github.com/fulgidus/revoco/tui"
)

func main() {
	// Ensure plugins are cleaned up on exit
	defer plugins.ShutdownPlugins()

	// Set version info for CLI
	cmd.SetVersionInfo(VersionInfo, FullVersionInfo)

	// If invoked with no arguments, launch the TUI.
	if cmd.NeedsTUI() {
		app := tui.NewApp()
		p := tea.NewProgram(app, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Otherwise let Cobra handle the CLI.
	cmd.Execute()
}
