package refs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"benchlog/internal/ui"
)

type viewState int

const (
	stateList    viewState = iota
	stateSearch
	stateLoading
	stateResults
)

// ── Domain types ──────────────────────────────────────────────────────────────

type Reference struct {
	ID       string    `json:"id"`
	S2ID     string    `json:"s2_id,omitempty"`
	DOI      string    `json:"doi,omitempty"`
	PubMedID string    `json:"pubmed_id,omitempty"`
	Title    string    `json:"title"`
	Authors  []string  `json:"authors"`
	Year     int       `json:"year"`
	Journal  string    `json:"journal"`
	Abstract string    `json:"abstract"`
	AddedAt  time.Time `json:"added_at"`
}

// ── Messages ──────────────────────────────────────────────────────────────────

type refsLoadedMsg []Reference
type searchResultsMsg []Reference
type savedMsg struct{}
type errMsg struct{ err error }

// ── Semantic Scholar API structs ──────────────────────────────────────────────

type s2SearchResponse struct {
	Data []s2Paper `json:"data"`
}

type s2Paper struct {
	PaperID     string            `json:"paperId"`
	Title       string            `json:"title"`
	Abstract    string            `json:"abstract"`
	Year        int               `json:"year"`
	Venue       string            `json:"venue"`
	Authors     []s2Author        `json:"authors"`
	ExternalIDs map[string]string `json:"externalIds"`
}

type s2Author struct {
	Name string `json:"name"`
}

func (p s2Paper) toReference() Reference {
	authors := make([]string, len(p.Authors))
	for i, a := range p.Authors {
		authors[i] = a.Name
	}
	id := p.PaperID
	if id == "" {
		id = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	doi := ""
	pmid := ""
	if p.ExternalIDs != nil {
		doi = p.ExternalIDs["DOI"]
		pmid = p.ExternalIDs["PubMed"]
	}
	return Reference{
		ID:       id,
		S2ID:     p.PaperID,
		DOI:      doi,
		PubMedID: pmid,
		Title:    p.Title,
		Authors:  authors,
		Year:     p.Year,
		Journal:  p.Venue,
		Abstract: p.Abstract,
		AddedAt:  time.Now(),
	}
}

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	state        viewState
	refs         []Reference
	results      []Reference
	cursor       int
	resultCursor int
	query        string
	dataDir      string
	width        int
	height       int
	err          error
}

func New(dataDir string) Model {
	return Model{dataDir: dataDir}
}

func (m Model) Init() tea.Cmd {
	return loadRefs(m.dataDir)
}

func (m Model) IsEditing() bool {
	return m.state == stateSearch || m.state == stateResults || m.state == stateLoading
}

func (m Model) FooterHint() string {
	switch m.state {
	case stateSearch:
		return " Enter to search · Esc to cancel"
	case stateLoading:
		return " Searching Semantic Scholar..."
	case stateResults:
		return " ↑/↓ navigate · Enter to save · Esc to search again"
	default:
		return " ↑/↓ navigate · a add · d delete · 1/2 switch view · q quit"
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refsLoadedMsg:
		m.refs = []Reference(msg)
		return m, nil

	case searchResultsMsg:
		m.results = []Reference(msg)
		m.resultCursor = 0
		m.state = stateResults
		return m, nil

	case savedMsg:
		return m, nil

	case errMsg:
		m.err = msg.err
		m.state = stateList
		return m, nil

	case tea.KeyMsg:
		m.err = nil
		switch m.state {
		case stateList:
			return m.updateList(msg)
		case stateSearch:
			return m.updateSearch(msg)
		case stateResults:
			return m.updateResults(msg)
		}
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.refs)-1 {
			m.cursor++
		}
	case "a":
		m.state = stateSearch
		m.query = ""
	case "d":
		if len(m.refs) > 0 {
			m.refs = append(m.refs[:m.cursor], m.refs[m.cursor+1:]...)
			if m.cursor >= len(m.refs) && m.cursor > 0 {
				m.cursor--
			}
			return m, saveRefs(m.dataDir, m.refs)
		}
	}
	return m, nil
}

func (m Model) updateSearch(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.state = stateList
		m.query = ""
	case tea.KeyEnter:
		if q := strings.TrimSpace(m.query); q != "" {
			m.state = stateLoading
			return m, fetchFromS2(q)
		}
	case tea.KeyBackspace, tea.KeyDelete:
		runes := []rune(m.query)
		if len(runes) > 0 {
			m.query = string(runes[:len(runes)-1])
		}
	case tea.KeyRunes:
		m.query += string(msg.Runes)
	}
	return m, nil
}

func (m Model) updateResults(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.resultCursor > 0 {
			m.resultCursor--
		}
	case "down", "j":
		if m.resultCursor < len(m.results)-1 {
			m.resultCursor++
		}
	case "enter":
		if len(m.results) > 0 {
			ref := m.results[m.resultCursor]
			m.refs = append([]Reference{ref}, m.refs...)
			m.state = stateList
			m.results = nil
			return m, saveRefs(m.dataDir, m.refs)
		}
	case "esc":
		m.state = stateSearch
		m.results = nil
	}
	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	switch m.state {
	case stateSearch:
		return m.viewSearch()
	case stateLoading:
		return ui.DimStyle.Render("\n  Searching Semantic Scholar...")
	case stateResults:
		return m.viewResults()
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

	if len(m.refs) == 0 {
		b.WriteString(ui.DimStyle.Render("\n  No references yet — press [a] to add one."))
		return b.String()
	}

	for i, ref := range m.refs {
		year := fmt.Sprintf("%d", ref.Year)
		if ref.Year == 0 {
			year = "????"
		}
		line := fmt.Sprintf("  %s  %-18s  %-46s  %s",
			year,
			shortAuthors(ref.Authors),
			truncate(ref.Title, 46),
			truncate(ref.Journal, 18),
		)
		if i == m.cursor {
			b.WriteString(ui.SelectedItemStyle.Render("▶" + line))
		} else {
			b.WriteString(ui.NormalItemStyle.Render(" " + line))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) viewSearch() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(ui.DimStyle.Render("  Enter a title, DOI (10.xxx/...), or PubMed ID:\n\n"))
	b.WriteString("  > ")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA")).Render(m.query))
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Render("█"))
	b.WriteString("\n\n")
	b.WriteString(ui.DimStyle.Render("  Enter to search · Esc to cancel"))
	return b.String()
}

func (m Model) viewResults() string {
	var b strings.Builder
	b.WriteString(ui.DimStyle.Render(fmt.Sprintf("\n  %d results for %q\n\n", len(m.results), m.query)))

	for i, ref := range m.results {
		year := fmt.Sprintf("%d", ref.Year)
		if ref.Year == 0 {
			year = "????"
		}
		line := fmt.Sprintf("  %s  %-18s  %-46s  %s",
			year,
			shortAuthors(ref.Authors),
			truncate(ref.Title, 46),
			truncate(ref.Journal, 18),
		)
		if i == m.resultCursor {
			b.WriteString(ui.SelectedItemStyle.Render("▶" + line))
		} else {
			b.WriteString(ui.NormalItemStyle.Render(" " + line))
		}
		b.WriteString("\n")
	}

	if len(m.results) > 0 {
		b.WriteString("\n")
		preview := m.results[m.resultCursor].Abstract
		if preview != "" {
			b.WriteString(ui.DimStyle.Render("  " + truncate(preview, m.width-4) + "\n"))
		}
	}
	return b.String()
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// ── Storage ───────────────────────────────────────────────────────────────────

func loadRefs(dir string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(filepath.Join(dir, "refs.json"))
		if err != nil {
			if os.IsNotExist(err) {
				return refsLoadedMsg{}
			}
			return errMsg{err}
		}
		var refs []Reference
		if err := json.Unmarshal(data, &refs); err != nil {
			return errMsg{err}
		}
		return refsLoadedMsg(refs)
	}
}

func saveRefs(dir string, refs []Reference) tea.Cmd {
	return func() tea.Msg {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errMsg{err}
		}
		data, _ := json.MarshalIndent(refs, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "refs.json"), data, 0644); err != nil {
			return errMsg{err}
		}
		return savedMsg{}
	}
}

// ── Semantic Scholar API ──────────────────────────────────────────────────────

const s2Fields = "title,authors,year,venue,abstract,externalIds"

func fetchFromS2(query string) tea.Cmd {
	var apiURL string
	var single bool

	switch {
	case strings.HasPrefix(query, "10."):
		apiURL = "https://api.semanticscholar.org/graph/v1/paper/DOI:" +
			url.PathEscape(query) + "?fields=" + s2Fields
		single = true
	case isAllDigits(query):
		apiURL = "https://api.semanticscholar.org/graph/v1/paper/PMID:" +
			query + "?fields=" + s2Fields
		single = true
	default:
		apiURL = "https://api.semanticscholar.org/graph/v1/paper/search?query=" +
			url.QueryEscape(query) + "&fields=" + s2Fields + "&limit=10"
	}

	return func() tea.Msg {
		resp, err := http.Get(apiURL)
		if err != nil {
			return errMsg{fmt.Errorf("network error: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode == 404 {
			return errMsg{fmt.Errorf("not found")}
		}
		if resp.StatusCode != 200 {
			return errMsg{fmt.Errorf("API returned status %d", resp.StatusCode)}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return errMsg{err}
		}

		if single {
			var paper s2Paper
			if err := json.Unmarshal(body, &paper); err != nil {
				return errMsg{err}
			}
			if paper.Title == "" {
				return errMsg{fmt.Errorf("paper not found")}
			}
			return searchResultsMsg{paper.toReference()}
		}

		var result s2SearchResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return errMsg{err}
		}
		if len(result.Data) == 0 {
			return errMsg{fmt.Errorf("no results found for %q", query)}
		}
		refs := make([]Reference, len(result.Data))
		for i, p := range result.Data {
			refs[i] = p.toReference()
		}
		return searchResultsMsg(refs)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func shortAuthors(authors []string) string {
	if len(authors) == 0 {
		return "Unknown"
	}
	last := lastName(authors[0])
	if len(authors) == 1 {
		return last
	}
	return last + " et al."
}

func lastName(full string) string {
	parts := strings.Fields(full)
	if len(parts) == 0 {
		return full
	}
	return parts[len(parts)-1]
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
