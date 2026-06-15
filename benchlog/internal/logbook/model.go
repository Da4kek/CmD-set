package logbook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"benchlog/internal/ui"
)

// ── Messages ──────────────────────────────────────────────────────────────────

type loadedMsg string
type savedMsg struct{}
type errMsg error

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	date    time.Time
	logDir  string
	area    textarea.Model
	width   int
	height  int
	err     error
	dirty   bool
	saveTip bool
}

func New(dataDir string) Model {
	ta := textarea.New()
	ta.Placeholder = "What happened today? Thoughts, ideas, plans..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	return Model{
		date:   time.Now(),
		logDir: filepath.Join(dataDir, "log"),
		area:   ta,
	}
}

func (m Model) Init() tea.Cmd {
	return m.loadDay(m.date)
}

func (m Model) IsEditing() bool { return true }

func (m Model) FooterHint() string {
	tip := ""
	if m.dirty {
		tip = ui.WarnStyle.Render(" ● unsaved") + "  "
	}
	return tip + " Ctrl+S save · [ prev day · ] next day · q quit"
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		m.area.SetValue(string(msg))
		m.dirty = false
		return m, m.area.Focus()
	case savedMsg:
		m.dirty = false
		return m, nil
	case errMsg:
		m.err = msg
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+s":
			return m, m.saveDay()
		case "[":
			if m.dirty {
				_ = m.saveDay()
			}
			m.date = m.date.AddDate(0, 0, -1)
			return m, m.loadDay(m.date)
		case "]":
			next := m.date.AddDate(0, 0, 1)
			if !next.After(time.Now()) {
				if m.dirty {
					_ = m.saveDay()
				}
				m.date = next
				return m, m.loadDay(m.date)
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	prev := m.area.Value()
	m.area, cmd = m.area.Update(msg)
	if m.area.Value() != prev {
		m.dirty = true
	}
	return m, cmd
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	var b strings.Builder

	// date header
	today := isToday(m.date)
	dateStr := m.date.Format("Mon, 02 Jan 2006")
	if today {
		dateStr += ui.NormalItemStyle.Render("  ← today")
	}
	b.WriteString("\n")
	b.WriteString("  " + ui.TitleStyle.Render(dateStr) + "\n")
	b.WriteString("  " + ui.DimStyle.Render("[ prev    ] next") + "\n\n")

	if m.err != nil {
		b.WriteString(ui.ErrorStyle.Render(fmt.Sprintf("  error: %v\n\n", m.err)))
	}

	// word count
	words := wordCount(m.area.Value())
	wc := ui.DimStyle.Render(fmt.Sprintf("  %d words", words))
	b.WriteString(wc + "\n")
	b.WriteString(ui.Sep(m.width) + "\n")
	b.WriteString(m.area.View())
	return b.String()
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	areaH := h - 7
	if areaH < 4 {
		areaH = 4
	}
	m.area.SetWidth(w - 2)
	m.area.SetHeight(areaH)
}

// ── Storage ───────────────────────────────────────────────────────────────────

func (m Model) dayFile() string {
	return filepath.Join(m.logDir, m.date.Format("2006-01-02")+".md")
}

func (m Model) loadDay(d time.Time) tea.Cmd {
	file := filepath.Join(m.logDir, d.Format("2006-01-02")+".md")
	return func() tea.Msg {
		data, err := os.ReadFile(file)
		if err != nil {
			if os.IsNotExist(err) {
				return loadedMsg("")
			}
			return errMsg(err)
		}
		return loadedMsg(string(data))
	}
}

func (m Model) saveDay() tea.Cmd {
	content := m.area.Value()
	file := m.dayFile()
	return func() tea.Msg {
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return errMsg(err)
		}
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			return errMsg(err)
		}
		return savedMsg{}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func isToday(d time.Time) bool {
	now := time.Now()
	return d.Year() == now.Year() && d.YearDay() == now.YearDay()
}

func wordCount(s string) int {
	return len(strings.Fields(s))
}

