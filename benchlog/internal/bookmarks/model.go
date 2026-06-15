package bookmarks

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

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"benchlog/internal/ui"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type Bookmark struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	AddedAt     time.Time `json:"added_at"`
}

type viewState int

const (
	stateList viewState = iota
	stateAdd
	stateDetail
	stateDelete
)

type addField int

const (
	fieldURL addField = iota
	fieldTitle
	fieldDesc
	fieldTags
)

// ── Messages ──────────────────────────────────────────────────────────────────

type loadedMsg []Bookmark
type savedMsg Bookmark
type errMsg error

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	state     viewState
	items     []Bookmark
	cursor    int
	dataFile  string
	width     int
	height    int
	err       error

	urlInput   textinput.Model
	titleInput textinput.Model
	descInput  textinput.Model
	tagsInput  textinput.Model
	activeField addField
}

func New(dataDir string) Model {
	url := textinput.New()
	url.Placeholder = "https://..."

	title := textinput.New()
	title.Placeholder = "Page title"

	desc := textinput.New()
	desc.Placeholder = "Short description"

	tags := textinput.New()
	tags.Placeholder = "tag1, tag2"

	return Model{
		dataFile:   filepath.Join(dataDir, "bookmarks.json"),
		urlInput:   url,
		titleInput: title,
		descInput:  desc,
		tagsInput:  tags,
	}
}

func (m Model) Init() tea.Cmd { return load(m.dataFile) }

func (m Model) IsEditing() bool { return m.state == stateAdd }

func (m Model) FooterHint() string {
	switch m.state {
	case stateAdd:
		return " Tab next field · Ctrl+S save · Esc cancel"
	case stateDetail:
		return " o open url · Esc back"
	case stateDelete:
		return " y confirm · n/Esc cancel"
	default:
		return " ↑/↓ navigate · a add · v detail · d delete · q quit"
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		m.items = []Bookmark(msg)
		return m, nil
	case savedMsg:
		b := Bookmark(msg)
		found := false
		for i, x := range m.items {
			if x.ID == b.ID {
				m.items[i] = b
				found = true
				break
			}
		}
		if !found {
			m.items = append([]Bookmark{b}, m.items...)
		}
		m.state = stateList
		return m, nil
	case errMsg:
		m.err = msg
		return m, nil
	}

	switch m.state {
	case stateList:
		if km, ok := msg.(tea.KeyMsg); ok {
			return m.updateList(km)
		}
	case stateAdd:
		return m.updateAdd(msg)
	case stateDetail:
		if km, ok := msg.(tea.KeyMsg); ok {
			return m.updateDetail(km)
		}
	case stateDelete:
		if km, ok := msg.(tea.KeyMsg); ok {
			return m.updateDelete(km)
		}
	}
	return m, nil
}

func (m Model) updateList(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "a":
		return m.openAdd()
	case "v", "enter":
		if len(m.items) > 0 {
			m.state = stateDetail
		}
	case "d":
		if len(m.items) > 0 {
			m.state = stateDelete
		}
	case "o":
		if len(m.items) > 0 {
			openURL(m.items[m.cursor].URL)
		}
	}
	return m, nil
}

func (m Model) openAdd() (Model, tea.Cmd) {
	m.state = stateAdd
	m.activeField = fieldURL
	m.urlInput.Reset()
	m.titleInput.Reset()
	m.descInput.Reset()
	m.tagsInput.Reset()
	m.titleInput.Blur()
	m.descInput.Blur()
	m.tagsInput.Blur()
	return m, m.urlInput.Focus()
}

func (m Model) updateAdd(msg tea.Msg) (Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+s":
			return m.save()
		case "esc":
			m.state = stateList
			return m, nil
		case "tab":
			return m.cycleField(true)
		case "shift+tab":
			return m.cycleField(false)
		}
	}
	var cmd tea.Cmd
	switch m.activeField {
	case fieldURL:
		m.urlInput, cmd = m.urlInput.Update(msg)
	case fieldTitle:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case fieldDesc:
		m.descInput, cmd = m.descInput.Update(msg)
	case fieldTags:
		m.tagsInput, cmd = m.tagsInput.Update(msg)
	}
	return m, cmd
}

func (m Model) cycleField(forward bool) (Model, tea.Cmd) {
	fields := []addField{fieldURL, fieldTitle, fieldDesc, fieldTags}
	idx := int(m.activeField)
	if forward {
		idx = (idx + 1) % len(fields)
	} else {
		idx = (idx - 1 + len(fields)) % len(fields)
	}

	// blur current
	switch m.activeField {
	case fieldURL:
		m.urlInput.Blur()
	case fieldTitle:
		m.titleInput.Blur()
	case fieldDesc:
		m.descInput.Blur()
	case fieldTags:
		m.tagsInput.Blur()
	}
	m.activeField = fields[idx]

	var cmd tea.Cmd
	switch m.activeField {
	case fieldURL:
		cmd = m.urlInput.Focus()
	case fieldTitle:
		cmd = m.titleInput.Focus()
	case fieldDesc:
		cmd = m.descInput.Focus()
	case fieldTags:
		cmd = m.tagsInput.Focus()
	}
	return m, cmd
}

func (m Model) save() (Model, tea.Cmd) {
	rawURL := strings.TrimSpace(m.urlInput.Value())
	if rawURL == "" {
		return m, nil
	}
	title := strings.TrimSpace(m.titleInput.Value())
	if title == "" {
		// use domain as fallback title
		title = urlHost(rawURL)
	}
	var tags []string
	for _, t := range strings.Split(m.tagsInput.Value(), ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	now := time.Now()
	b := Bookmark{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		URL:         rawURL,
		Title:       title,
		Description: strings.TrimSpace(m.descInput.Value()),
		Tags:        tags,
		AddedAt:     now,
	}
	return m, saveBookmark(m.dataFile, m.items, b)
}

func (m Model) updateDetail(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "esc", "q":
		m.state = stateList
	case "o":
		if len(m.items) > 0 {
			openURL(m.items[m.cursor].URL)
		}
	}
	return m, nil
}

func (m Model) updateDelete(key tea.KeyMsg) (Model, tea.Cmd) {
	switch key.String() {
	case "y":
		if len(m.items) > 0 {
			m.items = append(m.items[:m.cursor], m.items[m.cursor+1:]...)
			if m.cursor >= len(m.items) && m.cursor > 0 {
				m.cursor--
			}
			m.state = stateList
			return m, persist(m.dataFile, m.items)
		}
		m.state = stateList
	case "n", "esc":
		m.state = stateList
	}
	return m, nil
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	iw := w - 20
	if iw < 10 {
		iw = 10
	}
	m.urlInput.Width = iw
	m.titleInput.Width = iw
	m.descInput.Width = iw
	m.tagsInput.Width = iw
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	switch m.state {
	case stateAdd:
		return m.viewAdd()
	case stateDetail:
		return m.viewDetail()
	case stateDelete:
		return m.viewDeleteConfirm()
	default:
		return m.viewList()
	}
}

func (m Model) viewList() string {
	var b strings.Builder
	if m.err != nil {
		b.WriteString(ui.ErrorStyle.Render(fmt.Sprintf("\n  error: %v\n\n", m.err)))
	}
	if len(m.items) == 0 {
		b.WriteString(ui.DimStyle.Render("\n  No bookmarks yet — press [a] to add one."))
		return b.String()
	}
	for i, bm := range m.items {
		host := urlHost(bm.URL)
		tags := renderTags(bm.Tags)
		title := truncate(bm.Title, 36)
		line := fmt.Sprintf("  %-24s  %-36s  %s", host, title, tags)
		if i == m.cursor {
			b.WriteString(ui.SelectedItemStyle.Render("▶" + line))
		} else {
			b.WriteString(ui.NormalItemStyle.Render(" " + line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) viewAdd() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(ui.TitleStyle.Render("  Add Bookmark  ") + "\n\n")
	b.WriteString(ui.FieldLabel("URL  ", m.activeField == fieldURL) + m.urlInput.View() + "\n\n")
	b.WriteString(ui.FieldLabel("Title", m.activeField == fieldTitle) + m.titleInput.View() + "\n\n")
	b.WriteString(ui.FieldLabel("Desc ", m.activeField == fieldDesc) + m.descInput.View() + "\n\n")
	b.WriteString(ui.FieldLabel("Tags ", m.activeField == fieldTags) + m.tagsInput.View() + "\n")
	return b.String()
}

func (m Model) viewDetail() string {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return ""
	}
	bm := m.items[m.cursor]
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(ui.TitleStyle.Render("  "+bm.Title+"  ") + "\n\n")
	b.WriteString(ui.AccentStyle.Render("  "+bm.URL) + "\n\n")
	if bm.Description != "" {
		b.WriteString(ui.NormalItemStyle.Render("  "+bm.Description) + "\n\n")
	}
	if len(bm.Tags) > 0 {
		b.WriteString("  " + renderTags(bm.Tags) + "\n\n")
	}
	b.WriteString(ui.DimStyle.Render("  Added "+bm.AddedAt.Format("2006-01-02")) + "\n")
	return b.String()
}

func (m Model) viewDeleteConfirm() string {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return ""
	}
	return "\n" + ui.ErrorStyle.Render(fmt.Sprintf(`  Delete "%s"? [y] yes  [n] cancel`, m.items[m.cursor].Title))
}

// ── Storage ───────────────────────────────────────────────────────────────────

func load(file string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(file)
		if err != nil {
			if os.IsNotExist(err) {
				return loadedMsg{}
			}
			return errMsg(err)
		}
		var items []Bookmark
		if err := json.Unmarshal(data, &items); err != nil {
			return errMsg(err)
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].AddedAt.After(items[j].AddedAt)
		})
		return loadedMsg(items)
	}
}

func saveBookmark(file string, existing []Bookmark, b Bookmark) tea.Cmd {
	return func() tea.Msg {
		all := append([]Bookmark{b}, existing...)
		if err := write(file, all); err != nil {
			return errMsg(err)
		}
		return savedMsg(b)
	}
}

func persist(file string, items []Bookmark) tea.Cmd {
	return func() tea.Msg {
		if err := write(file, items); err != nil {
			return errMsg(err)
		}
		return nil
	}
}

func write(file string, items []Bookmark) error {
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(items, "", "  ")
	return os.WriteFile(file, data, 0644)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func renderTags(tags []string) string {
	var parts []string
	for _, t := range tags {
		parts = append(parts, ui.TagStyle.Render("#"+t))
	}
	return strings.Join(parts, " ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func urlHost(rawURL string) string {
	s := strings.TrimPrefix(rawURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	if i := strings.IndexByte(s, '/'); i != -1 {
		s = s[:i]
	}
	return s
}

func openURL(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	_ = cmd.Start()
}

