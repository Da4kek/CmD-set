package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"benchlog/internal/app"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "add":
			quickAdd(os.Args[2:])
			return
		case "--diag":
			runDiag()
			return
		}
	}

	home := homeDir()
	writeLog(home, fmt.Sprintf("start GOOS=%s GOARCH=%s HOME=%s\n", runtime.GOOS, runtime.GOARCH, home))

	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("PANIC: %v\n\n%s", r, debug.Stack())
			writeLog(home, msg)
			fmt.Fprintln(os.Stderr, "\n\n--- benchlog panic ---")
			fmt.Fprintln(os.Stderr, msg)
			time.Sleep(12 * time.Second)
		}
	}()

	writeLog(home, "creating model\n")
	m := app.New()
	writeLog(home, "running program\n")
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		writeLog(home, "error: "+err.Error()+"\n")
		fmt.Fprintln(os.Stderr, "\n--- benchlog error ---\n"+err.Error())
		time.Sleep(8 * time.Second)
		os.Exit(1)
	}
	writeLog(home, "exit ok\n")
}

// runDiag prints environment info then sleeps so the user can read it.
func runDiag() {
	home := homeDir()
	lines := []string{
		"=== benchlog diagnostics ===",
		"GOOS:        " + runtime.GOOS,
		"GOARCH:      " + runtime.GOARCH,
		"NumCPU:      " + fmt.Sprint(runtime.NumCPU()),
		"HOME (env):  " + os.Getenv("HOME"),
		"HOME (func): " + home,
		"TERM:        " + os.Getenv("TERM"),
		"",
	}
	dataDir := filepath.Join(home, ".benchlog")
	err := os.MkdirAll(dataDir, 0755)
	lines = append(lines, fmt.Sprintf("MkdirAll(%s): %v", dataDir, err))

	// check if we can write a file
	testFile := filepath.Join(dataDir, "diag_test.tmp")
	werr := os.WriteFile(testFile, []byte("ok"), 0644)
	lines = append(lines, fmt.Sprintf("WriteFile test: %v", werr))
	os.Remove(testFile)

	// print run.log if it exists
	if data, err := os.ReadFile(filepath.Join(dataDir, "run.log")); err == nil {
		lines = append(lines, "", "=== run.log ===")
		lines = append(lines, strings.Split(string(data), "\n")...)
	} else {
		lines = append(lines, "", "run.log: not found (main() was never reached)")
	}

	lines = append(lines, "", "=== sleeping 60s — take a screenshot ===")
	for _, l := range lines {
		fmt.Println(l)
	}
	time.Sleep(60 * time.Second)
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func writeLog(home, msg string) {
	if home == "" {
		return
	}
	dir := filepath.Join(home, ".benchlog")
	_ = os.MkdirAll(dir, 0755)
	f, err := os.OpenFile(filepath.Join(dir, "run.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(time.Now().Format("15:04:05 ") + msg)
}

func quickAdd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: benchlog add <text>")
		os.Exit(1)
	}
	text := strings.Join(args, " ")
	home := homeDir()
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
	fmt.Printf("saved: %s\n", note.Title)
}
