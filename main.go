package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to run application: %v\n", err)
		os.Exit(1)
	}

	finalModel, ok := result.(model)
	if !ok {
		fmt.Fprintln(os.Stderr, "failed to read application state")
		os.Exit(1)
	}

	if finalModel.exitAction == exitActionOpenShell {
		if err := openShellInWorktree(finalModel.exitPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to open shell: %v\n", err)
			os.Exit(1)
		}
	}
}
