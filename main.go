package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"benchlog/internal/app"
)

func main() {
	p := tea.NewProgram(app.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
