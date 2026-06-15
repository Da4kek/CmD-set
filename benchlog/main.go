package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"benchlog/internal/app"
)

func main() {
	// benchlog add [text...] — quick-capture a note without opening the TUI
	if len(os.Args) >= 2 && os.Args[1] == "add" {
		quickAdd(os.Args[2:])
		return
	}

	p := tea.NewProgram(app.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func quickAdd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: benchlog add <text>")
		os.Exit(1)
	}
	text := strings.Join(args, " ")
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".benchlog", "notes")
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	now := time.Now()
	note := struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		Tags      []string  `json:"tags"`
		Status    string    `json:"status"`
		Folder    string    `json:"folder,omitempty"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}{
		ID:        fmt.Sprintf("%d", now.UnixNano()),
		Title:     "Quick note — " + now.Format("Jan 2, 15:04"),
		Tags:      []string{"quick"},
		Status:    "ongoing",
		Body:      text,
		CreatedAt: now,
		UpdatedAt: now,
	}
	data, _ := json.MarshalIndent(note, "", "  ")
	file := filepath.Join(dir, note.ID+".json")
	if err := os.WriteFile(file, data, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("✔ saved: %s\n", note.Title)
}
