package notes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"benchlog/internal/ui"
)

// ── Types ─────────────────────────────────────────────────────────────────────

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
	Folder    string    `json:"folder,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type viewState int

const (
	stateList    viewState = iota
	stateEditor
	stateDelete
	stateFolders
	stateImport
)

type editorField int

const (
	fieldTitle editorField = iota
	fieldTags
	fieldStatus
	fieldFolder
	fieldBody
)

// ── Messages ──────────────────────────────────────────────────────────────────

type notesLoadedMsg []Note
type errMsg error
type savedNoteMsg struct{ note Note }

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	state   viewState
	notes   []Note
	cursor  int
	dataDir string
	width   int
	height  int
	err     error

	// Editor
	editing     *Note
	titleInput  textinput.Model
	tagsInput   textinput.Model
	folderInput textinput.Model
	bodyArea    textarea.Model
	statusSel   int
	activeField editorField

	// Folder filter
	activeFolder string
	folders      []string
	folderCursor int

	// Import
	importInput textinput.Model
}

func New(dataDir string) Model {
	ti := textinput.New()
	ti.Placeholder = "Note title"

	tags := textinput.New()
	tags.Placeholder = "tag1, tag2"

	folder := textinput.New()
	folder.Placeholder = "folder (optional)"

	ta := textarea.New()
	ta.Placeholder = "Write your note here..."
	ta.ShowLineNumbers = false

	imp := textinput.New()
	imp.Placeholder = "/path/to/file.md"

	return Model{
		dataDir:     dataDir,
		titleInput:  ti,
		tagsInput:   tags,
		folderInput: folder,
		bodyArea:    ta,
		importInput: imp,
	}
}

func (m Model) Init() tea.Cmd {
	return loadNotes(m.dataDir)
}

func (m Model) IsEditing() bool {
	return m.state == stateEditor || m.state == stateImport
}

func (m Model) FooterHint() string {
	switch m.state {
	case stateEditor:
		return " Tab next field · Ctrl+S save · Esc cancel"
	case stateDelete:
		return " y confirm delete · n/Esc cancel"
	case stateFolders:
		return " ↑/↓ navigate · Enter select · Esc cancel"
	case stateImport:
		return " Enter import · Esc cancel"
	default:
		hint := " ↑/↓ navigate · n new · e edit · d delete · f folders · i import · 1/2 switch · q quit"
		if m.activeFolder != "" {
			hint = " [" + m.activeFolder + "]" + hint
		}
		return hint
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case notesLoadedMsg:
		m.notes = []Note(msg)
		m.folders = extractFolders(m.notes)
		return m, nil
	case savedNoteMsg:
		found := false
		for i, n := range m.notes {
			if n.ID == msg.note.ID {
				m.notes[i] = msg.note
				found = true
				break
			}
		}
		if !found {
			m.notes = append([]Note{msg.note}, m.notes...)
		}
		m.folders = extractFolders(m.notes)
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
	case stateEditor:
		return m.updateEditor(msg)
	case stateDelete:
		if km, ok := msg.(tea.KeyMsg); ok {
			return m.updateDelete(km)
		}
	case stateFolders:
		if km, ok := msg.(tea.KeyMsg); ok {
			return m.updateFolders(km)
		}
	case stateImport:
		return m.updateImport(msg)
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (Model, tea.Cmd) {
	filtered := m.filteredNotes()
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(filtered)-1 {
			m.cursor++
		}
	case "n":
		return m.openEditor(nil)
	case "e":
		if len(filtered) > 0 && m.cursor < len(filtered) {
			n := filtered[m.cursor]
			return m.openEditor(&n)
		}
	case "d":
		if len(filtered) > 0 {
			m.state = stateDelete
		}
	case "f":
		m.state = stateFolders
		m.folderCursor = 0
		for i, f := range m.folders {
			if f == m.activeFolder {
				m.folderCursor = i + 1
				break
			}
		}
	case "i":
		m.state = stateImport
		m.importInput.Reset()
		return m, m.importInput.Focus()
	}
	return m, nil
}

func (m Model) openEditor(note *Note) (Model, tea.Cmd) {
	m.state = stateEditor
	m.editing = note
	m.activeField = fieldTitle
	m.statusSel = 0

	m.titleInput.Reset()
	m.tagsInput.Reset()
	m.folderInput.Reset()
	m.bodyArea.Reset()
	m.tagsInput.Blur()
	m.folderInput.Blur()
	m.bodyArea.Blur()

	if note != nil {
		m.titleInput.SetValue(note.Title)
		m.tagsInput.SetValue(strings.Join(note.Tags, ", "))
		m.folderInput.SetValue(note.Folder)
		m.bodyArea.SetValue(note.Body)
		switch note.Status {
		case StatusDone:
			m.statusSel = 1
		case StatusAbandoned:
			m.statusSel = 2
		}
	}

	return m, m.titleInput.Focus()
}

func (m Model) updateEditor(msg tea.Msg) (Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "ctrl+s":
			return m.saveCurrentNote()
		case "esc":
			m.state = stateList
			return m, nil
		case "tab":
			return m.cycleField(true)
		case "shift+tab":
			return m.cycleField(false)
		}

		if m.activeField == fieldStatus {
			switch km.String() {
			case "left", "h":
				if m.statusSel > 0 {
					m.statusSel--
				}
			case "right", "l":
				if m.statusSel < 2 {
					m.statusSel++
				}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.activeField {
	case fieldTitle:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case fieldTags:
		m.tagsInput, cmd = m.tagsInput.Update(msg)
	case fieldFolder:
		m.folderInput, cmd = m.folderInput.Update(msg)
	case fieldBody:
		m.bodyArea, cmd = m.bodyArea.Update(msg)
	}
	return m, cmd
}

func (m Model) cycleField(forward bool) (Model, tea.Cmd) {
	fields := []editorField{fieldTitle, fieldTags, fieldStatus, fieldFolder, fieldBody}
	idx := 0
	for i, f := range fields {
		if f == m.activeField {
			idx = i
			break
		}
	}
	if forward {
		idx = (idx + 1) % len(fields)
	} else {
		idx = (idx - 1 + len(fields)) % len(fields)
	}

	switch m.activeField {
	case fieldTitle:
		m.titleInput.Blur()
	case fieldTags:
		m.tagsInput.Blur()
	case fieldFolder:
		m.folderInput.Blur()
	case fieldBody:
		m.bodyArea.Blur()
	}

	m.activeField = fields[idx]

	var cmd tea.Cmd
	switch m.activeField {
	case fieldTitle:
		cmd = m.titleInput.Focus()
	case fieldTags:
		cmd = m.tagsInput.Focus()
	case fieldFolder:
		cmd = m.folderInput.Focus()
	case fieldBody:
		cmd = m.bodyArea.Focus()
	}
	return m, cmd
}

func (m Model) saveCurrentNote() (Model, tea.Cmd) {
	title := strings.TrimSpace(m.titleInput.Value())
	if title == "" {
		title = "Untitled " + time.Now().Format("2006-01-02 15:04")
	}
	var tags []string
	for _, t := range strings.Split(m.tagsInput.Value(), ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	statuses := []Status{StatusOngoing, StatusDone, StatusAbandoned}
	now := time.Now()
	note := Note{
		Title:     title,
		Tags:      tags,
		Status:    statuses[m.statusSel],
		Folder:    strings.TrimSpace(m.folderInput.Value()),
		Body:      m.bodyArea.Value(),
		UpdatedAt: now,
	}
	if m.editing != nil {
		note.ID = m.editing.ID
		note.CreatedAt = m.editing.CreatedAt
	} else {
		note.ID = fmt.Sprintf("%d", now.UnixNano())
		note.CreatedAt = now
	}
	return m, saveNote(m.dataDir, note)
}

func (m Model) updateDelete(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		filtered := m.filteredNotes()
		if len(filtered) > 0 && m.cursor < len(filtered) {
			id := filtered[m.cursor].ID
			for i, n := range m.notes {
				if n.ID == id {
					m.notes = append(m.notes[:i], m.notes[i+1:]...)
					break
				}
			}
			if m.cursor >= len(m.filteredNotes()) && m.cursor > 0 {
				m.cursor--
			}
			m.folders = extractFolders(m.notes)
			m.state = stateList
			return m, deleteNote(m.dataDir, id)
		}
		m.state = stateList
	case "n", "esc":
		m.state = stateList
	}
	return m, nil
}

func (m Model) updateFolders(msg tea.KeyMsg) (Model, tea.Cmd) {
	total := len(m.folders) + 1
	switch msg.String() {
	case "up", "k":
		if m.folderCursor > 0 {
			m.folderCursor--
		}
	case "down", "j":
		if m.folderCursor < total-1 {
			m.folderCursor++
		}
	case "enter":
		if m.folderCursor == 0 {
			m.activeFolder = ""
		} else {
			m.activeFolder = m.folders[m.folderCursor-1]
		}
		m.cursor = 0
		m.state = stateList
	case "esc":
		m.state = stateList
	}
	return m, nil
}

func (m Model) updateImport(msg tea.Msg) (Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.Type {
		case tea.KeyEsc:
			m.state = stateList
			return m, nil
		case tea.KeyEnter:
			if path := strings.TrimSpace(m.importInput.Value()); path != "" {
				return m, importNote(path, m.dataDir)
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.importInput, cmd = m.importInput.Update(msg)
	return m, cmd
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	switch m.state {
	case stateEditor:
		return m.viewEditor()
	case stateDelete:
		return m.viewDelete()
	case stateFolders:
		return m.viewFolders()
	case stateImport:
		return m.viewImport()
	default:
		return m.viewList()
	}
}

func (m Model) viewList() string {
	var b strings.Builder
	if m.err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).
			Render(fmt.Sprintf("\n  error: %v\n\n", m.err)))
	}
	filtered := m.filteredNotes()
	if len(filtered) == 0 {
		if m.activeFolder != "" {
			b.WriteString(ui.DimStyle.Render(fmt.Sprintf("\n  No notes in %q — press [n] to create one.", m.activeFolder)))
		} else {
			b.WriteString(ui.DimStyle.Render("\n  No notes yet — press [n] to create your first note."))
		}
		return b.String()
	}
	for i, note := range filtered {
		date := note.CreatedAt.Format("2006-01-02")
		status := statusIcon(note.Status)
		tags := renderTags(note.Tags)
		folder := ""
		if note.Folder != "" && m.activeFolder == "" {
			folder = ui.DimStyle.Render("["+note.Folder+"] ")
		}
		line := fmt.Sprintf("  %s  %s  %s%-40s %s", date, status, folder, truncate(note.Title, 40), tags)
		if i == m.cursor {
			b.WriteString(ui.SelectedItemStyle.Render("▶" + line))
		} else {
			b.WriteString(ui.NormalItemStyle.Render(" " + line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) viewEditor() string {
	var b strings.Builder
	b.WriteString("\n")
	action := "New Note"
	if m.editing != nil {
		action = "Edit Note"
	}
	b.WriteString(ui.TitleStyle.Render("  "+action+"  ") + "\n\n")
	b.WriteString(fieldLabel("Title ", m.activeField == fieldTitle) + m.titleInput.View() + "\n\n")
	b.WriteString(fieldLabel("Tags  ", m.activeField == fieldTags) + m.tagsInput.View() + "\n\n")
	b.WriteString(fieldLabel("Status", m.activeField == fieldStatus))
	statuses := []Status{StatusOngoing, StatusDone, StatusAbandoned}
	for i, s := range statuses {
		if i == m.statusSel {
			b.WriteString(ui.SelectedItemStyle.Render(" [" + string(s) + "] "))
		} else {
			b.WriteString(ui.DimStyle.Render("  " + string(s) + "  "))
		}
	}
	if m.activeField == fieldStatus {
		b.WriteString(ui.DimStyle.Render("  ←/→ to change"))
	}
	b.WriteString("\n\n")
	b.WriteString(fieldLabel("Folder", m.activeField == fieldFolder) + m.folderInput.View() + "\n\n")
	b.WriteString(fieldLabel("Body  ", m.activeField == fieldBody) + "\n")
	b.WriteString(m.bodyArea.View())
	return b.String()
}

func fieldLabel(name string, active bool) string {
	label := fmt.Sprintf("  %-8s ", name)
	if active {
		return ui.ActiveTabStyle.Render(label)
	}
	return ui.DimStyle.Render(label)
}

func (m Model) viewDelete() string {
	filtered := m.filteredNotes()
	if m.cursor >= len(filtered) {
		return ""
	}
	return "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).
		Render(fmt.Sprintf(`  Delete "%s"? [y] yes  [n] cancel`, filtered[m.cursor].Title))
}

func (m Model) viewFolders() string {
	var b strings.Builder
	b.WriteString(ui.DimStyle.Render("\n  Select folder:\n\n"))
	items := append([]string{"All notes"}, m.folders...)
	for i, f := range items {
		label := "    " + f
		active := (i == 0 && m.activeFolder == "") || (i > 0 && m.folders[i-1] == m.activeFolder)
		if active {
			label += ui.DimStyle.Render(" ✓")
		}
		if i == m.folderCursor {
			b.WriteString(ui.SelectedItemStyle.Render("  ▶" + label))
		} else {
			b.WriteString(ui.NormalItemStyle.Render("   " + label))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) viewImport() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(ui.DimStyle.Render("  Import a note from file (.md or .txt, frontmatter optional):\n\n"))
	b.WriteString("  > " + m.importInput.View())
	return b.String()
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	inputW := w - 20
	if inputW < 10 {
		inputW = 10
	}
	m.titleInput.Width = inputW
	m.tagsInput.Width = inputW
	m.folderInput.Width = inputW
	m.importInput.Width = inputW
	bodyH := h/3
	if bodyH < 5 {
		bodyH = 5
	}
	m.bodyArea.SetWidth(w - 4)
	m.bodyArea.SetHeight(bodyH)
}

// ── Storage ───────────────────────────────────────────────────────────────────

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

func saveNote(dir string, note Note) tea.Cmd {
	return func() tea.Msg {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errMsg(err)
		}
		data, _ := json.MarshalIndent(note, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, note.ID+".json"), data, 0644); err != nil {
			return errMsg(err)
		}
		return savedNoteMsg{note}
	}
}

func deleteNote(dir, id string) tea.Cmd {
	return func() tea.Msg {
		_ = os.Remove(filepath.Join(dir, id+".json"))
		return nil
	}
}

func importNote(path, dir string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return errMsg(fmt.Errorf("cannot read %s: %w", path, err))
		}
		content := string(data)
		now := time.Now()
		note := Note{
			ID:        fmt.Sprintf("%d", now.UnixNano()),
			Status:    StatusOngoing,
			CreatedAt: now,
			UpdatedAt: now,
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
					case strings.HasPrefix(line, "folder:"):
						note.Folder = strings.TrimSpace(strings.TrimPrefix(line, "folder:"))
					}
				}
				note.Body = strings.TrimSpace(parts[2])
			}
		} else {
			base := filepath.Base(path)
			note.Title = strings.TrimSuffix(base, filepath.Ext(base))
			note.Body = strings.TrimSpace(content)
		}
		if note.Title == "" {
			note.Title = "Imported " + now.Format("2006-01-02 15:04")
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errMsg(err)
		}
		noteData, _ := json.MarshalIndent(note, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, note.ID+".json"), noteData, 0644); err != nil {
			return errMsg(err)
		}
		return savedNoteMsg{note}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m Model) filteredNotes() []Note {
	if m.activeFolder == "" {
		return m.notes
	}
	var out []Note
	for _, n := range m.notes {
		if n.Folder == m.activeFolder {
			out = append(out, n)
		}
	}
	return out
}

func extractFolders(notes []Note) []string {
	seen := map[string]bool{}
	var folders []string
	for _, n := range notes {
		if n.Folder != "" && !seen[n.Folder] {
			seen[n.Folder] = true
			folders = append(folders, n.Folder)
		}
	}
	sort.Strings(folders)
	return folders
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

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
