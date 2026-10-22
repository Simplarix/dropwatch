package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Simplarix/dropwatch/internal/app"
	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type Model struct {
	app      *app.App
	domains  []domain.Domain
	history  []domain.HistoryItem
	selected int
	width    int
	height   int
	location *time.Location
	mode     string
	input    string
	status   string
	err      error
}

type loadedMsg struct {
	domains []domain.Domain
	err     error
}

type historyMsg struct {
	items []domain.HistoryItem
	err   error
}

type actionMsg struct {
	message string
	err     error
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	rowStyle    = lipgloss.NewStyle().Padding(0, 1)
	selectStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("255")).
			Bold(true).
			Padding(0, 1)
)

func New(application *app.App) Model {
	location, _ := time.LoadLocation(application.Config.Timezone)
	return Model{app: application, mode: "list", status: "Loading domains…", location: location}
}

func (m Model) Init() tea.Cmd { return m.loadDomains() }

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loadedMsg:
		m.domains, m.err = msg.domains, msg.err
		if m.selected >= len(m.domains) {
			m.selected = max(0, len(m.domains)-1)
		}
		if msg.err == nil {
			m.status = fmt.Sprintf("%d domains monitored", len(m.domains))
		}
	case historyMsg:
		m.history, m.err = msg.items, msg.err
		if msg.err == nil {
			m.mode = "history"
		}
	case actionMsg:
		m.err = msg.err
		if msg.err == nil {
			m.status = msg.message
			m.mode = "list"
			return m, m.loadDomains()
		}
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) View() string {
	var view string
	switch m.mode {
	case "detail":
		view = m.detailView()
	case "history":
		view = m.historyView()
	case "help":
		view = m.helpView()
	case "add":
		view = m.listView() + "\n\n" + titleStyle.Render("Add domain") + "\n> " + m.input + "█"
	case "delete":
		name := ""
		if selected := m.selectedDomain(); selected != nil {
			name = selected.ASCIIName
		}
		view = m.listView() + "\n\n" + warnStyle.Render("Remove "+name+"? Press y to confirm, n to cancel.")
	default:
		view = m.listView()
	}
	footer := m.status
	if m.err != nil {
		footer = errorStyle.Render(m.err.Error())
	} else {
		footer = dimStyle.Render(footer)
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(view + "\n\n" + footer)
}

func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	value := key.String()
	if m.mode == "add" {
		switch value {
		case "esc":
			m.mode, m.input = "list", ""
		case "enter":
			input := m.input
			m.input = ""
			return m, m.addDomain(input)
		case "backspace":
			if len(m.input) > 0 {
				_, size := utf8.DecodeLastRuneInString(m.input)
				m.input = m.input[:len(m.input)-size]
			}
		default:
			if key.Type == tea.KeyRunes {
				m.input += string(key.Runes)
			}
		}
		return m, nil
	}
	if m.mode == "delete" {
		switch value {
		case "y", "Y":
			if item := m.selectedDomain(); item != nil {
				return m, m.removeDomain(*item)
			}
		case "n", "N", "esc":
			m.mode = "list"
		}
		return m, nil
	}
	switch value {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = "list"
	case "up", "k":
		m.selected = max(0, m.selected-1)
	case "down", "j":
		m.selected = min(max(0, len(m.domains)-1), m.selected+1)
	case "enter":
		if m.selectedDomain() != nil {
			m.mode = "detail"
		}
	case "r":
		if item := m.selectedDomain(); item != nil {
			m.status = "Checking " + item.ASCIIName + "…"
			return m, m.checkDomain(*item)
		}
	case "R":
		m.status = "Checking all domains…"
		return m, m.checkAll()
	case "a":
		m.mode, m.input, m.err = "add", "", nil
	case "d":
		if m.selectedDomain() != nil {
			m.mode = "delete"
		}
	case "h":
		if item := m.selectedDomain(); item != nil {
			return m, m.loadHistory(item.ID)
		}
	case "?":
		m.mode = "help"
	}
	return m, nil
}

func (m Model) listView() string {
	var body strings.Builder
	body.WriteString(titleStyle.Render("DropWatch Dashboard"))
	body.WriteString("\n\n")
	header := dashboardRow("DOMAIN", "STATE", "EXPIRES", "LAST CHECK", "NEXT CHECK", "ERRORS")
	body.WriteString(rowStyle.Render(dimStyle.Render(header)))
	body.WriteByte('\n')
	if len(m.domains) == 0 {
		body.WriteString("\nNo domains. Press a to add one.")
	} else {
		for index, item := range m.domains {
			expires := formatDate(item.ExpiresAt, m.location)
			last := formatDate(item.LastCheckAt, m.location)
			next := item.NextCheckAt.In(m.location).Format("2006-01-02 15:04")
			line := dashboardRow(
				truncate(item.UnicodeName, 24), styledState(item.State), expires, last, next, fmt.Sprint(item.ConsecutiveErrors))
			if index == m.selected {
				body.WriteString(selectStyle.Render(line))
			} else {
				body.WriteString(rowStyle.Render(line))
			}
			body.WriteByte('\n')
		}
	}
	body.WriteString("\n")
	body.WriteString(dimStyle.Render("↑/↓ j/k navigate • Enter details • r check • R check all • a add • d remove • h history • ? help • q quit"))
	return body.String()
}

func dashboardRow(cells ...string) string {
	widths := [...]int{25, 20, 17, 17, 17, 7}
	for index, cell := range cells {
		// Terminal cells exclude ANSI styling and account for wide Unicode characters.
		cells[index] = cell + strings.Repeat(" ", max(0, widths[index]-lipgloss.Width(cell)))
	}
	return strings.Join(cells, " ")
}

func (m Model) detailView() string {
	item := m.selectedDomain()
	if item == nil {
		return m.listView()
	}
	var body strings.Builder
	body.WriteString(titleStyle.Render("Domain details — " + item.UnicodeName))
	body.WriteString("\n\n")
	fmt.Fprintf(&body, "ASCII / Punycode   %s\n", item.ASCIIName)
	fmt.Fprintf(&body, "State              %s\n", styledState(item.State))
	fmt.Fprintf(&body, "RDAP status        %s\n", fallback(strings.Join(item.RDAPStatuses, ", "), "n/a"))
	fmt.Fprintf(&body, "Registrar          %s\n", fallback(item.Registrar, "n/a"))
	fmt.Fprintf(&body, "Registered         %s\n", formatDate(item.RegisteredAt, m.location))
	fmt.Fprintf(&body, "Expires            %s\n", formatDate(item.ExpiresAt, m.location))
	fmt.Fprintf(&body, "Nameservers        %s\n", fallback(strings.Join(item.Nameservers, ", "), "n/a"))
	fmt.Fprintf(&body, "DNS active         %t\n", item.DNSActive)
	fmt.Fprintf(&body, "Last check         %s\n", formatDate(item.LastCheckAt, m.location))
	fmt.Fprintf(&body, "Last success       %s\n", formatDate(item.LastSuccessAt, m.location))
	fmt.Fprintf(&body, "Next check         %s\n", item.NextCheckAt.In(m.location).Format(time.RFC3339))
	fmt.Fprintf(&body, "Consecutive errors %d\n", item.ConsecutiveErrors)
	fmt.Fprintf(&body, "Notifications      %s\n", fallback(item.LastNotification, "none"))
	if item.LastError != "" {
		fmt.Fprintf(&body, "Last error         %s\n", errorStyle.Render(item.LastError))
	}
	body.WriteString("\n" + dimStyle.Render("Esc returns • r checks now • h history • q quits"))
	return body.String()
}

func (m Model) historyView() string {
	item := m.selectedDomain()
	name := ""
	if item != nil {
		name = item.ASCIIName
	}
	var body strings.Builder
	body.WriteString(titleStyle.Render("History — " + name))
	body.WriteString("\n\n")
	for _, event := range m.history {
		fmt.Fprintf(&body, "%s  %-22s %s\n",
			event.CreatedAt.In(m.location).Format("2006-01-02 15:04:05"), event.Type, event.Message)
	}
	if len(m.history) == 0 {
		body.WriteString("No history recorded.\n")
	}
	body.WriteString("\n" + dimStyle.Render("Esc returns • q quits"))
	return body.String()
}

func (m Model) helpView() string {
	return titleStyle.Render("DropWatch Help") + `

↑/↓ or j/k   Navigate domains
Enter         Open details
r             Check selected domain
R             Check every domain
a             Add a domain
d             Remove with confirmation
h             Show history
?             Toggle help
Esc           Return to list
q             Quit`
}

func (m Model) selectedDomain() *domain.Domain {
	if m.selected < 0 || m.selected >= len(m.domains) {
		return nil
	}
	return &m.domains[m.selected]
}

func (m Model) loadDomains() tea.Cmd {
	return func() tea.Msg {
		items, err := m.app.Store.ListDomains(context.Background())
		return loadedMsg{domains: items, err: err}
	}
}

func (m Model) loadHistory(id int64) tea.Cmd {
	return func() tea.Msg {
		items, err := m.app.Store.History(context.Background(), id, 100)
		return historyMsg{items: items, err: err}
	}
}

func (m Model) addDomain(input string) tea.Cmd {
	return func() tea.Msg {
		normalized, err := domain.NormalizeName(input)
		if err != nil {
			return actionMsg{err: err}
		}
		item, err := m.app.Store.AddDomain(context.Background(), normalized, time.Now().UTC())
		if err != nil {
			return actionMsg{err: err}
		}
		_, checkErr := m.app.Checker.CheckDomain(context.Background(), item)
		if checkErr != nil {
			return actionMsg{message: "Domain added; initial check failed", err: checkErr}
		}
		return actionMsg{message: "Added and checked " + normalized.ASCII}
	}
}

func (m Model) removeDomain(item domain.Domain) tea.Cmd {
	return func() tea.Msg {
		err := m.app.Store.RemoveDomain(context.Background(), item.ASCIIName, time.Now().UTC())
		return actionMsg{message: "Removed " + item.ASCIIName, err: err}
	}
}

func (m Model) checkDomain(item domain.Domain) tea.Cmd {
	return func() tea.Msg {
		_, err := m.app.Checker.CheckDomain(context.Background(), item)
		return actionMsg{message: "Checked " + item.ASCIIName, err: err}
	}
}

func (m Model) checkAll() tea.Cmd {
	return func() tea.Msg {
		_, errors := m.app.Checker.CheckAll(context.Background(), m.app.Config.MaxConcurrency)
		if len(errors) > 0 {
			return actionMsg{err: fmt.Errorf("%d domain checks failed", len(errors))}
		}
		return actionMsg{message: "Checked all domains"}
	}
}

func styledState(state domain.State) string {
	switch state {
	case domain.StateRegistered:
		return okStyle.Render(string(state))
	case domain.StateAvailable, domain.StatePossiblyAvailable:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true).Render(string(state))
	case domain.StateError, domain.StatePendingDelete:
		return errorStyle.Render(string(state))
	case domain.StateClientHold, domain.StateServerHold, domain.StateRedemptionPeriod:
		return warnStyle.Render(string(state))
	default:
		return dimStyle.Render(string(state))
	}
}

func formatDate(value *time.Time, location *time.Location) string {
	if value == nil {
		return "n/a"
	}
	return value.In(location).Format("2006-01-02 15:04")
}

func truncate(value string, maxLength int) string {
	return ansi.Truncate(value, maxLength, "…")
}

func fallback(value, replacement string) string {
	if value == "" {
		return replacement
	}
	return value
}

func Run(application *app.App) error {
	_, err := tea.NewProgram(New(application), tea.WithAltScreen()).Run()
	return err
}
