package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"benchlog/internal/bookmarks"
	"benchlog/internal/logbook"
	"benchlog/internal/notes"
	"benchlog/internal/refs"
	"benchlog/internal/ui"
)

type view int

const (
	viewNotes view = iota
	viewRefs
	viewBookmarks
	viewLog
)

type Model struct {
	active    view
	notes     notes.Model
	refs      refs.Model
	bookmarks bookmarks.Model
	log       logbook.Model
	width     int
	height    int
	ready     bool
}

func dataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".benchlog")
}

func New() Model {
	dir := dataDir()
	return Model{
		active:    viewNotes,
		notes:     notes.New(filepath.Join(dir, "notes")),
		refs:      refs.New(dir),
		bookmarks: bookmarks.New(dir),
		log:       logbook.New(dir),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.notes.Init(),
		m.refs.Init(),
		m.bookmarks.Init(),
		m.log.Init(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		ch := msg.Height - 4
		if ch < 0 {
			ch = 0
		}
		m.notes.SetSize(msg.Width, ch)
		m.refs.SetSize(msg.Width, ch)
		m.bookmarks.SetSize(msg.Width, ch)
		m.log.SetSize(msg.Width, ch)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			editing := m.notes.IsEditing() || m.refs.IsEditing() ||
				m.bookmarks.IsEditing() || m.log.IsEditing()
			if !editing {
				return m, tea.Quit
			}
		case "1":
			if !m.anyEditing() {
				m.active = viewNotes
				return m, nil
			}
		case "2":
			if !m.anyEditing() {
				m.active = viewRefs
				return m, nil
			}
		case "3":
			if !m.anyEditing() {
				m.active = viewBookmarks
				return m, nil
			}
		case "4":
			if !m.anyEditing() {
				m.active = viewLog
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	switch m.active {
	case viewNotes:
		m.notes, cmd = m.notes.Update(msg)
	case viewRefs:
		m.refs, cmd = m.refs.Update(msg)
	case viewBookmarks:
		m.bookmarks, cmd = m.bookmarks.Update(msg)
	case viewLog:
		m.log, cmd = m.log.Update(msg)
	}
	return m, cmd
}

func (m Model) anyEditing() bool {
	return m.notes.IsEditing() || m.refs.IsEditing() ||
		m.bookmarks.IsEditing()
}

func (m Model) View() string {
	if !m.ready {
		return ""
	}
	header := m.header()
	footer := m.footer()
	contentH := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if contentH < 0 {
		contentH = 0
	}

	var content string
	switch m.active {
	case viewNotes:
		content = m.notes.View()
	case viewRefs:
		content = m.refs.View()
	case viewBookmarks:
		content = m.bookmarks.View()
	case viewLog:
		content = m.log.View()
	}

	body := lipgloss.NewStyle().
		Height(contentH).
		Width(m.width).
		Render(content)

	return fmt.Sprintf("%s\n%s\n%s", header, body, footer)
}

func (m Model) header() string {
	logo := ui.TitleStyle.Render("◉ benchlog")

	tabs := []struct {
		key   string
		label string
		view  view
	}{
		{"1", "notes", viewNotes},
		{"2", "refs", viewRefs},
		{"3", "bookmarks", viewBookmarks},
		{"4", "log", viewLog},
	}

	var tabParts []string
	for _, t := range tabs {
		label := t.key + " " + t.label
		if m.active == t.view {
			tabParts = append(tabParts, ui.ActiveTabStyle.Render(label))
		} else {
			tabParts = append(tabParts, ui.InactiveTabStyle.Render(label))
		}
	}
	tabBar := strings.Join(tabParts, ui.DimStyle.Render("│"))

	clock := ui.DimStyle.Render(time.Now().Format("15:04"))

	used := lipgloss.Width(logo) + 2 + lipgloss.Width(tabBar) + 2 + lipgloss.Width(clock)
	gap := strings.Repeat(" ", max(0, m.width-used))

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		logo, "  ", tabBar, gap, clock, " ")

	sep := ui.Sep(m.width)
	return top + "\n" + sep
}

func (m Model) footer() string {
	var hint string
	switch m.active {
	case viewNotes:
		hint = m.notes.FooterHint()
	case viewRefs:
		hint = m.refs.FooterHint()
	case viewBookmarks:
		hint = m.bookmarks.FooterHint()
	case viewLog:
		hint = m.log.FooterHint()
	}
	return ui.StatusBarStyle.Width(m.width).Render(hint)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
