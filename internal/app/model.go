package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"benchlog/internal/notes"
	"benchlog/internal/refs"
	"benchlog/internal/ui"
)

type view int

const (
	viewNotes view = iota
	viewRefs
)

type Model struct {
	active view
	notes  notes.Model
	refs   refs.Model
	width  int
	height int
	ready  bool
}

func dataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".benchlog")
}

func New() Model {
	dir := dataDir()
	return Model{
		active: viewNotes,
		notes:  notes.New(filepath.Join(dir, "notes")),
		refs:   refs.New(dir),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.notes.Init(), m.refs.Init())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.notes.SetSize(msg.Width, msg.Height-4)
		m.refs.SetSize(msg.Width, msg.Height-4)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			// Only quit from base list states — not while typing or browsing results
			if !m.notes.IsEditing() && !m.refs.IsEditing() {
				return m, tea.Quit
			}
		case "1":
			if !m.refs.IsEditing() && !m.notes.IsEditing() {
				m.active = viewNotes
				return m, nil
			}
		case "2":
			if !m.refs.IsEditing() && !m.notes.IsEditing() {
				m.active = viewRefs
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
	}
	return m, cmd
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
	}

	body := lipgloss.NewStyle().
		Height(contentH).
		Width(m.width).
		Render(content)

	return fmt.Sprintf("%s\n%s\n%s", header, body, footer)
}

func (m Model) header() string {
	title := ui.TitleStyle.Render("benchlog")

	notesTab := ui.InactiveTabStyle.Render("1 Notes")
	refsTab := ui.InactiveTabStyle.Render("2 References")
	if m.active == viewNotes {
		notesTab = ui.ActiveTabStyle.Render("1 Notes")
	} else {
		refsTab = ui.ActiveTabStyle.Render("2 References")
	}

	tabs := lipgloss.JoinHorizontal(lipgloss.Top, notesTab, refsTab)
	used := lipgloss.Width(title) + 1 + lipgloss.Width(tabs)
	gap := strings.Repeat(" ", max(0, m.width-used))

	return lipgloss.JoinHorizontal(lipgloss.Top, title, " ", tabs, gap)
}

func (m Model) footer() string {
	var hint string
	switch m.active {
	case viewNotes:
		hint = m.notes.FooterHint()
	case viewRefs:
		hint = m.refs.FooterHint()
	}
	return ui.StatusBarStyle.Width(m.width).Render(hint)
}
