package notes

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"benchlog/internal/ui"
)

type Status string

const (
	StatusOngoing   Status = "ongoing"
	StatusDone      Status = "done"
	StatusAbandoned Status = "abandoned"
)

type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Tags      []string  `json:"tags"`
	Status    Status    `json:"status"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type notesLoadedMsg []Note
type errMsg error
type editorDoneMsg struct{ note Note }

type Model struct {
	notes   []Note
	cursor  int
	dataDir string
	width   int
	height  int
	err     error
}

func New(dataDir string) Model {
	return Model{dataDir: dataDir}
}

func (m Model) Init() tea.Cmd {
	return loadNotes(m.dataDir)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case notesLoadedMsg:
		m.notes = []Note(msg)
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil

	case editorDoneMsg:
		m.notes = append([]Note{msg.note}, m.notes...)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.notes)-1 {
				m.cursor++
			}
		case "n":
			return m, openEditor(m.dataDir)
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).
			Render(fmt.Sprintf("\n  error: %v", m.err))
	}
	if len(m.notes) == 0 {
		return ui.DimStyle.Render("\n  No notes yet — press [n] to create your first note.")
	}

	var b strings.Builder
	for i, note := range m.notes {
		date := note.CreatedAt.Format("2006-01-02")
		status := statusIcon(note.Status)
		tags := renderTags(note.Tags)
		line := fmt.Sprintf("  %s  %s  %-40s %s", date, status, note.Title, tags)

		if i == m.cursor {
			b.WriteString(ui.SelectedItemStyle.Render("▶" + line))
		} else {
			b.WriteString(ui.NormalItemStyle.Render(" " + line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) IsEditing() bool { return false }

func (m Model) FooterHint() string {
	return " ↑/↓ navigate · n new · e edit · / search · 1/2 switch view · q quit"
}

func statusIcon(s Status) string {
	switch s {
	case StatusOngoing:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F0C040")).Render("●")
	case StatusDone:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#73F59F")).Render("✓")
	case StatusAbandoned:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Render("✗")
	}
	return "○"
}

func renderTags(tags []string) string {
	var parts []string
	for _, t := range tags {
		parts = append(parts, ui.TagStyle.Render("#"+t))
	}
	return strings.Join(parts, " ")
}

func loadNotes(dir string) tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return notesLoadedMsg{}
			}
			return errMsg(err)
		}

		var notes []Note
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var n Note
			if err := json.Unmarshal(data, &n); err != nil {
				continue
			}
			notes = append(notes, n)
		}

		sort.Slice(notes, func(i, j int) bool {
			return notes[i].CreatedAt.After(notes[j].CreatedAt)
		})
		return notesLoadedMsg(notes)
	}
}

func openEditor(dir string) tea.Cmd {
	tmp, err := os.CreateTemp("", "benchlog-*.md")
	if err != nil {
		return func() tea.Msg { return errMsg(err) }
	}
	_, _ = tmp.WriteString("---\ntitle: \ntags: \nstatus: ongoing\n---\n\n")
	tmp.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "nano"
		}
	}

	c := exec.Command(editor, tmp.Name())
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return errMsg(err)
		}
		note, parseErr := parseNoteFile(tmp.Name(), dir)
		os.Remove(tmp.Name())
		if parseErr != nil {
			return errMsg(parseErr)
		}
		return editorDoneMsg{note: note}
	})
}

func parseNoteFile(path, dir string) (Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Note{}, err
	}

	content := string(data)
	note := Note{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Status:    StatusOngoing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			for _, line := range strings.Split(parts[1], "\n") {
				line = strings.TrimSpace(line)
				switch {
				case strings.HasPrefix(line, "title:"):
					note.Title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
				case strings.HasPrefix(line, "tags:"):
					raw := strings.TrimSpace(strings.TrimPrefix(line, "tags:"))
					for _, t := range strings.Split(raw, ",") {
						if t = strings.TrimSpace(t); t != "" {
							note.Tags = append(note.Tags, t)
						}
					}
				case strings.HasPrefix(line, "status:"):
					note.Status = Status(strings.TrimSpace(strings.TrimPrefix(line, "status:")))
				}
			}
			note.Body = strings.TrimSpace(parts[2])
		}
	} else {
		note.Body = strings.TrimSpace(content)
	}

	if note.Title == "" {
		note.Title = "Untitled " + time.Now().Format("2006-01-02 15:04")
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return Note{}, err
	}

	out, _ := json.MarshalIndent(note, "", "  ")
	return note, os.WriteFile(filepath.Join(dir, note.ID+".json"), out, 0644)
}
