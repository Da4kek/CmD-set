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

// tickMsg drives 500ms animation frames.
type tickMsg time.Time

func doTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type Model struct {
	active    view
	notes     notes.Model
	refs      refs.Model
	bookmarks bookmarks.Model
	log       logbook.Model
	width     int
	height    int
	ready     bool
	tick      int  // monotonic frame counter
	blink     bool // toggles every tick — drives cursor and pulse effects
	tabFlash  int  // counts down after a tab switch for the neon flash
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
		doTick(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.tick++
		m.blink = !m.blink
		if m.tabFlash > 0 {
			m.tabFlash--
		}
		return m, doTick()

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
			if !m.anyEditing() && m.active != viewNotes {
				m.active = viewNotes
				m.tabFlash = 4
				return m, nil
			}
		case "2":
			if !m.anyEditing() && m.active != viewRefs {
				m.active = viewRefs
				m.tabFlash = 4
				return m, nil
			}
		case "3":
			if !m.anyEditing() && m.active != viewBookmarks {
				m.active = viewBookmarks
				m.tabFlash = 4
				return m, nil
			}
		case "4":
			if !m.anyEditing() && m.active != viewLog {
				m.active = viewLog
				m.tabFlash = 4
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

// ── Tab definitions ────────────────────────────────────────────────────────────

var tabDefs = []struct {
	key   string
	label string
	id    view
}{
	{"1", "NOTES", viewNotes},
	{"2", "REFS", viewRefs},
	{"3", "BKMRK", viewBookmarks},
	{"4", "LOG", viewLog},
}

// ── Glitch effect ──────────────────────────────────────────────────────────────

var glitchPairs = [][2]rune{
	{'E', '3'}, {'N', '╬'}, {'H', '#'}, {'L', '|'}, {'O', '0'}, {'G', '6'},
}

// glitchStr replaces the first two glitchable chars — fires for one 500ms
// frame every ~6 seconds to give the logo a brief data-corruption flicker.
func glitchStr(s string) string {
	r := []rune(s)
	applied := 0
	for i := range r {
		for _, pair := range glitchPairs {
			if r[i] == pair[0] {
				r[i] = pair[1]
				applied++
				break
			}
		}
		if applied >= 2 {
			break
		}
	}
	return string(r)
}

// ── Header ─────────────────────────────────────────────────────────────────────

func (m Model) header() string {
	// Logo: glitches for exactly one frame every ~6 seconds
	logoText := "◉ BENCHLOG"
	if m.tick%12 == 1 {
		logoText = glitchStr(logoText)
	}
	// Blinking block cursor after logo — pulses while app is alive
	cursor := " "
	if m.blink {
		cursor = ui.AmberDimStyle.Render("█")
	}
	logo := ui.TitleStyle.Render(logoText) + cursor

	// Tab bar — active tab flashes neon green for 2s after switching
	var tabParts []string
	for _, t := range tabDefs {
		if m.active == t.id {
			label := "▸ " + t.key + ":" + t.label + " ◂"
			if m.tabFlash > 0 {
				tabParts = append(tabParts, ui.FlashTabStyle.Render(label))
			} else {
				tabParts = append(tabParts, ui.ActiveTabStyle.Render(label))
			}
		} else {
			tabParts = append(tabParts,
				ui.InactiveTabStyle.Render(" "+t.key+":"+strings.ToLower(t.label)+" "))
		}
	}
	tabBar := strings.Join(tabParts, ui.DimStyle.Render("·"))

	clock := ui.DimStyle.Render(time.Now().Format("15:04"))

	used := lipgloss.Width(logo) + 2 +
		lipgloss.Width(tabBar) + 2 +
		lipgloss.Width(clock)
	gap := strings.Repeat(" ", max(0, m.width-used))

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		logo, "  ", tabBar, gap, clock, " ")

	return top + "\n" + ui.Sep(m.width)
}

// ── Footer ─────────────────────────────────────────────────────────────────────

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
