package refs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	stateDetail
)

// ── Domain types ──────────────────────────────────────────────────────────────

type Reference struct {
	ID       string    `json:"id"`
	DOI      string    `json:"doi,omitempty"`
	PubMedID string    `json:"pubmed_id,omitempty"`
	Title    string    `json:"title"`
	Authors  []string  `json:"authors"`
	Year     int       `json:"year"`
	Journal  string    `json:"journal"`
	Abstract string    `json:"abstract"`
	OAURL    string    `json:"oa_url,omitempty"`
	AddedAt  time.Time `json:"added_at"`
}

// ── Messages ──────────────────────────────────────────────────────────────────

type refsLoadedMsg []Reference
type searchResultsMsg []Reference
type savedMsg struct{}
type errMsg struct{ err error }

// ── OpenAlex API structs ──────────────────────────────────────────────────────

type oaResponse struct {
	Results []oaWork `json:"results"`
}

type oaWork struct {
	ID                    string           `json:"id"`
	DOI                   string           `json:"doi"`
	Title                 string           `json:"title"`
	PublicationYear       int              `json:"publication_year"`
	PrimaryLocation       *oaLocation      `json:"primary_location"`
	Authorships           []oaAuthorship   `json:"authorships"`
	AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
	IDs                   oaIDs            `json:"ids"`
	OpenAccess            *oaOpenAccess    `json:"open_access"`
}

type oaOpenAccess struct {
	IsOA  bool   `json:"is_oa"`
	OAURL string `json:"oa_url"`
}

type oaLocation struct {
	Source *oaSource `json:"source"`
}

type oaSource struct {
	DisplayName string `json:"display_name"`
}

type oaAuthorship struct {
	Author oaAuthor `json:"author"`
}

type oaAuthor struct {
	DisplayName string `json:"display_name"`
}

type oaIDs struct {
	PMID string `json:"pmid"`
}

func (w oaWork) toReference() Reference {
	authors := make([]string, len(w.Authorships))
	for i, a := range w.Authorships {
		authors[i] = a.Author.DisplayName
	}

	journal := ""
	if w.PrimaryLocation != nil && w.PrimaryLocation.Source != nil {
		journal = w.PrimaryLocation.Source.DisplayName
	}

	doi := strings.TrimPrefix(w.DOI, "https://doi.org/")
	pmid := strings.TrimPrefix(w.IDs.PMID, "https://pubmed.ncbi.nlm.nih.gov/")

	oaURL := ""
	if w.OpenAccess != nil && w.OpenAccess.IsOA {
		oaURL = w.OpenAccess.OAURL
	}

	id := w.ID
	if id == "" {
		id = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	return Reference{
		ID:       id,
		DOI:      doi,
		PubMedID: pmid,
		Title:    w.Title,
		Authors:  authors,
		Year:     w.PublicationYear,
		Journal:  journal,
		Abstract: reconstructAbstract(w.AbstractInvertedIndex),
		OAURL:    oaURL,
		AddedAt:  time.Now(),
	}
}

// OpenAlex stores abstracts as an inverted index {word: [pos1, pos2, ...]}
func reconstructAbstract(inv map[string][]int) string {
	if len(inv) == 0 {
		return ""
	}
	maxPos := 0
	for _, positions := range inv {
		for _, p := range positions {
			if p > maxPos {
				maxPos = p
			}
		}
	}
	words := make([]string, maxPos+1)
	for word, positions := range inv {
		for _, p := range positions {
			words[p] = word
		}
	}
	return strings.Join(words, " ")
}

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	state             viewState
	refs              []Reference
	results           []Reference
	cursor            int
	resultCursor      int
	query             string
	dataDir           string
	width             int
	height            int
	err               error
	detailIdx         int
	detailFromResults bool
}

func New(dataDir string) Model {
	return Model{dataDir: dataDir}
}

func (m Model) Init() tea.Cmd {
	return loadRefs(m.dataDir)
}

func (m Model) IsEditing() bool {
	return m.state == stateSearch
}

func (m Model) FooterHint() string {
	switch m.state {
	case stateSearch:
		return " Enter to search · Esc to cancel"
	case stateLoading:
		return " Searching OpenAlex..."
	case stateResults:
		return " ↑/↓ navigate · v abstract · Enter save · Esc search again"
	case stateDetail:
		return " o open PDF · Esc go back"
	default:
		return " ↑/↓ navigate · v/Enter view · a add · d delete · 1/2 switch · q quit"
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
		case stateDetail:
			return m.updateDetail(msg)
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
	case "v", "enter":
		if len(m.refs) > 0 {
			m.state = stateDetail
			m.detailIdx = m.cursor
			m.detailFromResults = false
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
			return m, fetchFromOpenAlex(q)
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
	case "v":
		if len(m.results) > 0 {
			m.state = stateDetail
			m.detailIdx = m.resultCursor
			m.detailFromResults = true
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

func (m Model) updateDetail(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.detailFromResults {
			m.state = stateResults
		} else {
			m.state = stateList
		}
	case "enter":
		if m.detailFromResults && m.detailIdx < len(m.results) {
			ref := m.results[m.detailIdx]
			m.refs = append([]Reference{ref}, m.refs...)
			m.state = stateList
			m.results = nil
			return m, saveRefs(m.dataDir, m.refs)
		}
	case "o":
		var ref *Reference
		if m.detailFromResults && m.detailIdx < len(m.results) {
			r := m.results[m.detailIdx]
			ref = &r
		} else if !m.detailFromResults && m.detailIdx < len(m.refs) {
			r := m.refs[m.detailIdx]
			ref = &r
		}
		if ref != nil && ref.OAURL != "" {
			openURL(ref.OAURL)
		}
	}
	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	switch m.state {
	case stateSearch:
		return m.viewSearch()
	case stateLoading:
		return ui.DimStyle.Render("\n  Searching OpenAlex...")
	case stateResults:
		return m.viewResults()
	case stateDetail:
		return m.viewDetail()
	default:
		return m.viewList()
	}
}

func (m Model) viewDetail() string {
	var ref Reference
	if m.detailFromResults {
		if m.detailIdx < len(m.results) {
			ref = m.results[m.detailIdx]
		}
	} else {
		if m.detailIdx < len(m.refs) {
			ref = m.refs[m.detailIdx]
		}
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(ui.SelectedItemStyle.Render("  "+ref.Title) + "\n\n")
	b.WriteString(ui.DimStyle.Render("  Authors:   ") + truncate(strings.Join(ref.Authors, ", "), m.width-14) + "\n")

	year := fmt.Sprintf("%d", ref.Year)
	if ref.Year == 0 {
		year = "unknown"
	}
	b.WriteString(ui.DimStyle.Render("  Published: ") + year)
	if ref.Journal != "" {
		b.WriteString(" · " + ref.Journal)
	}
	b.WriteString("\n")

	if ref.DOI != "" {
		b.WriteString(ui.DimStyle.Render("  DOI:       ") + ref.DOI + "\n")
	}
	if ref.PubMedID != "" {
		b.WriteString(ui.DimStyle.Render("  PubMed:    ") + ref.PubMedID + "\n")
	}

	if ref.OAURL != "" {
		b.WriteString(ui.DimStyle.Render("  PDF:       ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#43BF6D")).Render(ref.OAURL) + "\n")
	} else {
		b.WriteString(ui.DimStyle.Render("  PDF:       not open access\n"))
	}

	b.WriteString("\n" + ui.DimStyle.Render("  Abstract:\n"))
	if ref.Abstract != "" {
		b.WriteString("  " + wrapText(ref.Abstract, m.width-4) + "\n")
	} else {
		b.WriteString(ui.DimStyle.Render("  No abstract available.\n"))
	}

	b.WriteString("\n")
	if m.detailFromResults {
		b.WriteString(ui.DimStyle.Render("  Enter to save"))
		if ref.OAURL != "" {
			b.WriteString(ui.DimStyle.Render(" · o open PDF"))
		}
		b.WriteString(ui.DimStyle.Render(" · Esc go back"))
	} else {
		if ref.OAURL != "" {
			b.WriteString(ui.DimStyle.Render("  o open PDF · Esc go back"))
		} else {
			b.WriteString(ui.DimStyle.Render("  Esc go back"))
		}
	}
	return b.String()
}

func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	words := strings.Fields(text)
	var lines []string
	var line strings.Builder
	lineLen := 0
	for _, word := range words {
		wl := len([]rune(word))
		if lineLen > 0 && lineLen+1+wl > width {
			lines = append(lines, line.String())
			line.Reset()
			lineLen = 0
		}
		if lineLen > 0 {
			line.WriteString(" ")
			lineLen++
		}
		line.WriteString(word)
		lineLen += wl
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n  ")
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

// ── OpenAlex API ──────────────────────────────────────────────────────────────

const oaBase = "https://api.openalex.org/works"

func fetchFromOpenAlex(query string) tea.Cmd {
	var apiURL string

	switch {
	case strings.HasPrefix(query, "10."):
		apiURL = oaBase + "?filter=doi:https://doi.org/" + url.QueryEscape(query)
	case isAllDigits(query):
		apiURL = oaBase + "?filter=ids.pmid:" + query
	default:
		apiURL = oaBase + "?search=" + url.QueryEscape(query) + "&per-page=10"
	}

	return func() tea.Msg {
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return errMsg{err}
		}
		req.Header.Set("User-Agent", "benchlog/0.1 (https://github.com/Da4kek/CmD-set)")

		resp, err := client.Do(req)
		if err != nil {
			return errMsg{fmt.Errorf("network error: %w", err)}
		}
		defer resp.Body.Close()

		if resp.StatusCode == 404 {
			return errMsg{fmt.Errorf("not found")}
		}
		if resp.StatusCode != 200 {
			return errMsg{fmt.Errorf("OpenAlex error: status %d", resp.StatusCode)}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return errMsg{err}
		}

		var result oaResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return errMsg{err}
		}
		if len(result.Results) == 0 {
			return errMsg{fmt.Errorf("no results found for %q", query)}
		}

		refs := make([]Reference, len(result.Results))
		for i, w := range result.Results {
			refs[i] = w.toReference()
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
