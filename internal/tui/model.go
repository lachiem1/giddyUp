package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lachiem1/giddyUp/internal/auth"
	"github.com/lachiem1/giddyUp/internal/upapi"
)

type connectionState int

const (
	stateChecking connectionState = iota
	stateConnected
	stateDisconnected
)

type checkConnectionMsg struct {
	connected bool
	err       error
}

type savePATMsg struct {
	ok  bool
	err error
}

type model struct {
	width  int
	height int

	viewItems []string
	selected  int
	cmd       textinput.Model
	pat       textinput.Model

	status       connectionState
	statusDetail string

	showPATPrompt bool
	quitting      bool
}

func New() tea.Model {
	cmd := textinput.New()
	cmd.Prompt = "> "
	cmd.Placeholder = "/help"
	cmd.Focus()

	pat := textinput.New()
	pat.Prompt = "PAT: "
	pat.Placeholder = "up:..."
	pat.EchoMode = textinput.EchoPassword
	pat.EchoCharacter = '•'

	return model{
		viewItems: []string{
			"Accounts",
			"Transactions",
			"Spend Categories",
			"Pay Cycle Burndown",
			"Settings",
		},
		selected:     0,
		cmd:          cmd,
		pat:          pat,
		status:       stateChecking,
		statusDetail: "Checking connection...",
	}
}

func (m model) Init() tea.Cmd {
	return checkConnectionCmd
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case checkConnectionMsg:
		if msg.connected {
			m.status = stateConnected
			m.statusDetail = "Connected"
		} else {
			m.status = stateDisconnected
			m.statusDetail = "Not connected"
			if msg.err != nil {
				m.statusDetail = "Not connected: " + msg.err.Error()
			}
		}
		return m, nil

	case savePATMsg:
		m.showPATPrompt = false
		m.pat.SetValue("")
		m.cmd.Focus()
		if msg.err != nil {
			m.status = stateDisconnected
			m.statusDetail = "Failed to save PAT: " + msg.err.Error()
			return m, nil
		}
		m.status = stateChecking
		m.statusDetail = "Checking connection..."
		return m, checkConnectionCmd

	case tea.KeyMsg:
		if m.showPATPrompt {
			switch msg.String() {
			case "esc":
				m.showPATPrompt = false
				m.pat.Blur()
				m.cmd.Focus()
				return m, nil
			case "enter":
				pat := strings.TrimSpace(m.pat.Value())
				return m, savePATCmd(pat)
			}
			var cmd tea.Cmd
			m.pat, cmd = m.pat.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.viewItems)-1 {
				m.selected++
			}
		case "c":
			if m.status == stateDisconnected {
				m.showPATPrompt = true
				m.pat.Focus()
				m.cmd.Blur()
				return m, nil
			}
		case "enter":
			return m.runSlashCommand(strings.TrimSpace(m.cmd.Value()))
		}
	}

	var cmd tea.Cmd
	m.cmd, cmd = m.cmd.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	header := renderBlockTitle()
	if m.width > 0 {
		header = lipgloss.PlaceHorizontal(max(0, m.width-4), lipgloss.Center, header)
	}
	header = lipgloss.NewStyle().PaddingBottom(1).Render(header)

	statusDot := "●"
	statusColor := lipgloss.Color("#F15B5B")
	if m.status == stateConnected {
		statusColor = lipgloss.Color("#5CCB76")
	}
	statusLine := lipgloss.NewStyle().Bold(true).Render("Status: ") +
		lipgloss.NewStyle().Foreground(statusColor).Render(statusDot) + " " + m.statusDetail

	if m.status == stateDisconnected {
		statusLine += "  " + lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F47A60")).
			Bold(true).
			Render("[ Connect: press c ]")
	}

	listBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#F47A60")).
		Padding(0, 1).
		Render(renderViews(m.viewItems, m.selected))

	helpLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F6C34A")).
		Render("Type /help for commands")

	cmdBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#6CBFE6")).
		Padding(0, 1).
		Render(m.cmd.View())

	body := []string{
		header,
		"",
		statusLine,
		"",
		listBox,
		"",
		helpLine,
		cmdBox,
	}

	if m.showPATPrompt {
		prompt := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFD54A")).
			Padding(1, 2).
			Render("Connect to Up\n\n" + m.pat.View() + "\n\nEnter to save, Esc to cancel")
		body = append(body, "", prompt)
	}

	return lipgloss.NewStyle().
		Padding(2, 2).
		Render(strings.Join(body, "\n"))
}

func (m model) runSlashCommand(input string) (tea.Model, tea.Cmd) {
	switch input {
	case "":
		return m, nil
	case "/help":
		m.statusDetail = "Commands: /help /accounts /transactions /settings /connect"
	case "/accounts":
		m.selected = 0
		m.statusDetail = "Accounts view selected"
	case "/transactions":
		m.selected = 1
		m.statusDetail = "Transactions view selected"
	case "/settings":
		m.selected = 4
		m.statusDetail = "Settings view selected"
	case "/connect":
		m.showPATPrompt = true
		m.pat.Focus()
		m.cmd.Blur()
	default:
		m.statusDetail = fmt.Sprintf("Unknown command: %s", input)
	}
	m.cmd.SetValue("")
	return m, nil
}

func checkConnectionCmd() tea.Msg {
	pat, err := auth.LoadPAT()
	if err != nil {
		return checkConnectionMsg{connected: false, err: err}
	}

	client := upapi.New(pat)
	err = client.Ping(context.Background())
	return checkConnectionMsg{
		connected: err == nil,
		err:       err,
	}
}

func savePATCmd(pat string) tea.Cmd {
	return func() tea.Msg {
		if err := auth.SavePAT(pat); err != nil {
			return savePATMsg{ok: false, err: err}
		}
		return savePATMsg{ok: true}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func renderViews(items []string, selected int) string {
	lines := []string{"Views", ""}
	for i, item := range items {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		lines = append(lines, prefix+item)
	}
	return strings.Join(lines, "\n")
}

func renderBlockTitle() string {
	raw := []string{
		"  ██████╗ ██╗██████╗ ██████╗ ██╗   ██╗    ██╗   ██╗██████╗ ",
		" ██╔════╝ ██║██╔══██╗██╔══██╗╚██╗ ██╔╝    ██║   ██║██╔══██╗",
		" ██║  ███╗██║██║  ██║██║  ██║ ╚████╔╝     ██║   ██║██████╔╝",
		" ██║   ██║██║██║  ██║██║  ██║  ╚██╔╝      ██║   ██║██╔═══╝ ",
		" ╚██████╔╝██║██████╔╝██████╔╝   ██║       ╚██████╔╝██║     ",
		"  ╚═════╝ ╚═╝╚═════╝ ╚═════╝    ╚═╝        ╚═════╝ ╚═╝     ",
	}

	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("#5FA8FF")).Bold(true)
	coral := lipgloss.NewStyle().Foreground(lipgloss.Color("#F47A60")).Bold(true)
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD54A")).Bold(true)

	// Approximate letter regions for "GIDDYUP" across the title width.
	// Alternates coral/yellow per region.
	segments := [][2]int{
		{2, 8},   // G
		{10, 12}, // I
		{13, 19}, // D
		{21, 27}, // D
		{29, 37}, // Y
		{42, 50}, // U
		{51, 57}, // P
	}

	rows := make([]string, 0, len(raw))
	for _, line := range raw {
		runes := []rune(line)
		var out strings.Builder
		for idx, ch := range runes {
			if ch == ' ' {
				out.WriteRune(' ')
				continue
			}

			if isStrokeRune(ch) {
				out.WriteString(blue.Render(string(ch)))
				continue
			}

			seg := segmentForIndex(idx, segments)
			fill := coral
			if seg%2 == 1 {
				fill = yellow
			}
			out.WriteString(fill.Render(string(ch)))
		}
		rows = append(rows, out.String())
	}

	return strings.Join(rows, "\n")
}

func isStrokeRune(ch rune) bool {
	switch ch {
	case '╔', '╗', '╚', '╝', '║', '═':
		return true
	default:
		return false
	}
}

func segmentForIndex(index int, segments [][2]int) int {
	for i, s := range segments {
		if index >= s[0] && index <= s[1] {
			return i
		}
	}
	return 0
}
