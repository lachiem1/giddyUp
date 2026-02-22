package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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

type clearCommandTextMsg struct {
	id int
}

type commandSpec struct {
	name        string
	description string
}

type model struct {
	width  int
	height int

	viewItems []string
	selected  int
	cmd       textinput.Model
	pat       textinput.Model

	status                  connectionState
	statusDetail            string
	commandText             string
	commandTextID           int
	commandSuggestions      []commandSpec
	commandSuggestionIndex  int
	commandSuggestionOffset int

	showHelpOverlay bool
	showPATPrompt   bool
	quitting        bool
}

func New() tea.Model {
	cmd := textinput.New()
	cmd.Prompt = "> "
	cmd.Placeholder = "/help"
	cmd.Width = 72
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
		},
		selected:     0,
		cmd:          cmd,
		pat:          pat,
		status:       stateChecking,
		statusDetail: "Checking connection...",
		commandText:  "",
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
		m.cmd.Width = max(40, msg.Width-36)
		m.pat.Width = max(24, msg.Width-40)
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

	case clearCommandTextMsg:
		if msg.id == m.commandTextID {
			m.commandText = ""
		}
		return m, nil

	case tea.KeyMsg:
		if m.showHelpOverlay {
			switch msg.String() {
			case "esc":
				m.showHelpOverlay = false
				return m, nil
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}

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
			if m.shouldShowCommandSuggestions() {
				if m.commandSuggestionIndex > 0 {
					m.commandSuggestionIndex--
				}
				m.adjustSuggestionWindow(2)
				return m, nil
			}
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.shouldShowCommandSuggestions() {
				if m.commandSuggestionIndex < len(m.commandSuggestions)-1 {
					m.commandSuggestionIndex++
				}
				m.adjustSuggestionWindow(2)
				return m, nil
			}
			if m.selected < len(m.viewItems)-1 {
				m.selected++
			}
		case "c":
			if m.status == stateDisconnected {
				m.showPATPrompt = true
				m.pat.Focus()
				m.cmd.Blur()
				m.clearCommandSuggestions()
				return m, nil
			}
		case "enter":
			input := strings.TrimSpace(m.cmd.Value())
			if m.shouldShowCommandSuggestions() {
				input = m.commandSuggestions[m.commandSuggestionIndex].name
			}
			return m.runSlashCommand(input)
		}
		if m.commandText != "" {
			switch msg.Type {
			case tea.KeyRunes, tea.KeySpace, tea.KeyBackspace, tea.KeyDelete:
				m.commandText = ""
			}
		}
	}

	var cmd tea.Cmd
	m.cmd, cmd = m.cmd.Update(msg)
	m.refreshCommandSuggestions()
	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	frame := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#F47A60")).
		Padding(1, 1)
	// Keep top breathing room, but tighten bottom so CLI sits one row above the frame border.
	contentStyle := lipgloss.NewStyle().Padding(1, 1, 0, 1)
	if m.width > 0 {
		frame = frame.Width(max(1, m.width-frame.GetHorizontalBorderSize()))
	}
	if m.height > 0 {
		frame = frame.Height(max(1, m.height-frame.GetVerticalBorderSize()))
	}

	// Effective width available to body content after all outer frame and padding.
	layoutWidth := max(1, m.width-frame.GetHorizontalFrameSize()-contentStyle.GetHorizontalFrameSize())

	header := renderBlockTitle()
	if m.width > 0 {
		header = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, header)
	}
	header = lipgloss.NewStyle().PaddingBottom(1).Render(header)

	statusDot := "●"
	statusColor := lipgloss.Color("#F15B5B")
	if m.status == stateConnected {
		statusColor = lipgloss.Color("#5CCB76")
	}
	statusLine := lipgloss.NewStyle().Foreground(lipgloss.Color("#5FA8FF")).Bold(true).Render("Status: ") +
		lipgloss.NewStyle().Foreground(statusColor).Render(statusDot) + " " + m.statusDetail

	listWidth := 24
	rightWidth := max(36, m.width-listWidth-20)
	panelHeight := 14
	listBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#F47A60")).
		Padding(0, 1).
		Height(panelHeight).
		Width(listWidth).
		Render(renderViews(m.viewItems, m.selected, statusLine, m.status == stateDisconnected))

	panelWidth := max(18, (rightWidth-2)/2)
	pinnedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFD54A")).
		Padding(0, 1).
		Width(panelWidth).
		Height(panelHeight)
	pinTitle := pinIconOrFallback()
	selectButton := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD54A")).
		Bold(true).
		Render("[select view]")
	cardHeader := pinTitle + " " + selectButton
	pinnedOne := pinnedStyle.Render(cardHeader)
	pinnedTwo := pinnedStyle.Render(cardHeader)
	rightPanels := lipgloss.JoinHorizontal(lipgloss.Top, pinnedOne, "  ", pinnedTwo)
	canvasWidth := layoutWidth
	mainPanelsRaw := lipgloss.JoinHorizontal(lipgloss.Top, listBox, "  ", rightPanels)
	mainPanelsWidth := lipgloss.Width(mainPanelsRaw)
	mainPanels := lipgloss.PlaceHorizontal(canvasWidth, lipgloss.Center, mainPanelsRaw)

	messageArea := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6CBFE6")).
		Padding(0, 1).
		Foreground(lipgloss.Color("#D4CDE9")).
		Width(max(8, mainPanelsWidth-4)).
		Render(m.commandText)
	if strings.TrimSpace(m.commandText) == "" {
		messageArea = ""
	} else {
		messageArea = lipgloss.PlaceHorizontal(canvasWidth, lipgloss.Center, messageArea)
	}

	cmdOuterWidth := mainPanelsWidth
	cmdInnerWidth := max(8, cmdOuterWidth-4)
	cmdInput := m.cmd
	cmdInput.Width = max(6, cmdInnerWidth-2)
	cmdLines := []string{}
	if m.shouldShowCommandSuggestions() {
		cmdLines = append(cmdLines, renderCommandSuggestionRows(cmdInnerWidth, m.commandSuggestions, m.commandSuggestionIndex, m.commandSuggestionOffset))
	}
	cmdLines = append(cmdLines, lipgloss.NewStyle().Width(cmdInnerWidth).Render(cmdInput.View()))
	cmdInner := strings.Join(cmdLines, "\n")

	cmdBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6CBFE6")).
		Padding(0, 1).
		Render(cmdInner)
	cmdBox = lipgloss.PlaceHorizontal(canvasWidth, lipgloss.Center, cmdBox)
	bottomSection := cmdBox

	headerGap := 1
	hasMessage := strings.TrimSpace(m.commandText) != ""
	messageGap := 0
	if hasMessage {
		messageGap = 1
	}

	bridgeMin := 1
	bridgeGap := bridgeMin
	if m.height > 0 {
		availableHeight := max(0, m.height-frame.GetVerticalFrameSize()-contentStyle.GetVerticalFrameSize())

		coreHeight := lipgloss.Height(header) + lipgloss.Height(mainPanels) + lipgloss.Height(bottomSection) + headerGap
		if hasMessage {
			coreHeight += messageGap + lipgloss.Height(messageArea)
		}

		// Keep the cards->CLI gap as the primary compression buffer.
		bridgeGap = max(bridgeMin, availableHeight-coreHeight)
		if coreHeight+bridgeGap > availableHeight {
			// Once bridge has reached its minimum, collapse secondary blank space.
			headerGap = 0
			coreHeight = lipgloss.Height(header) + lipgloss.Height(mainPanels) + lipgloss.Height(bottomSection)
			if hasMessage {
				coreHeight += messageGap + lipgloss.Height(messageArea)
			}
		}
		if coreHeight+bridgeGap > availableHeight {
			// If space is still insufficient, allow bridge to collapse below 1.
			bridgeGap = max(0, availableHeight-coreHeight)
		}
	}

	topLines := []string{header}
	if headerGap > 0 {
		topLines = append(topLines, "")
	}
	topLines = append(topLines, mainPanels)
	if hasMessage {
		if messageGap > 0 {
			topLines = append(topLines, "")
		}
		topLines = append(topLines, messageArea)
	}
	topSection := strings.Join(topLines, "\n")

	bodyText := topSection
	if bridgeGap > 0 {
		// N blank rows are represented by N newlines between sections.
		bodyText += "\n" + strings.Repeat("\n", bridgeGap-1)
	}
	bodyText += "\n" + bottomSection

	if m.showPATPrompt {
		prompt := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFD54A")).
			Padding(1, 2).
			Render("Connect to Up\n\n" + m.pat.View() + "\n\nEnter to save, Esc to cancel")
		bodyText += "\n\n" + prompt
	}

	content := contentStyle.Render(bodyText)

	if m.showHelpOverlay {
		helpOverlay := renderHelpOverlay(layoutWidth)
		layoutHeight := max(1, m.height-frame.GetVerticalFrameSize()-contentStyle.GetVerticalFrameSize())
		centered := lipgloss.Place(layoutWidth, layoutHeight, lipgloss.Center, lipgloss.Center, helpOverlay)
		return frame.Render(contentStyle.Render(centered))
	}

	return frame.Render(content)
}

func (m model) runSlashCommand(input string) (tea.Model, tea.Cmd) {
	switch input {
	case "":
		return m, nil
	case "/help":
		m.showHelpOverlay = true
		m.commandText = ""
		m.cmd.SetValue("")
		m.clearCommandSuggestions()
		return m, nil
	case "/accounts":
		m.selected = 0
		return m.withCommandFeedback("Accounts view selected")
	case "/transactions":
		m.selected = 1
		return m.withCommandFeedback("Transactions view selected")
	case "/settings":
		return m.withCommandFeedback("Settings has been removed from the views list.")
	case "/connect":
		m.showPATPrompt = true
		m.pat.Focus()
		m.cmd.Blur()
		m.clearCommandSuggestions()
		return m.withCommandFeedback("Enter your PAT in the prompt to connect.")
	default:
		return m.withCommandFeedback(fmt.Sprintf("Unknown command: %s", input))
	}
}

func (m model) withCommandFeedback(text string) (tea.Model, tea.Cmd) {
	m.commandText = text
	m.commandTextID++
	m.cmd.SetValue("")
	m.clearCommandSuggestions()
	id := m.commandTextID
	return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg {
		return clearCommandTextMsg{id: id}
	})
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func canvasSafeWidth(width int) int {
	return max(20, width-10)
}

func renderViews(items []string, selected int, statusLine string, disconnected bool) string {
	lines := []string{statusLine, ""}
	for i, item := range items {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		lines = append(lines, prefix+item)
	}
	if disconnected {
		lines = append(lines, "", "[c to connect]")
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

func pinIconOrFallback() string {
	// Set GIDDYUP_DISABLE_NERD_FONT=1 to force ASCII fallback.
	if os.Getenv("GIDDYUP_DISABLE_NERD_FONT") == "1" {
		return "PIN"
	}
	return "\uf435"
}

func commandCatalog() []commandSpec {
	return []commandSpec{
		{name: "/help", description: "Show command help overlay"},
		{name: "/accounts", description: "Select the Accounts view"},
		{name: "/transactions", description: "Select the Transactions view"},
		{name: "/connect", description: "Open the PAT connect prompt"},
		{name: "/settings", description: "Legacy command (removed view)"},
	}
}

func (m *model) refreshCommandSuggestions() {
	input := strings.TrimSpace(m.cmd.Value())
	if !strings.HasPrefix(input, "/") {
		m.clearCommandSuggestions()
		return
	}

	prefix := strings.ToLower(input)
	all := commandCatalog()
	matches := make([]commandSpec, 0, len(all))
	for _, cmd := range all {
		if strings.HasPrefix(cmd.name, prefix) {
			matches = append(matches, cmd)
		}
	}
	if len(matches) == 0 {
		m.clearCommandSuggestions()
		return
	}

	m.commandSuggestions = matches
	if m.commandSuggestionIndex >= len(m.commandSuggestions) {
		m.commandSuggestionIndex = len(m.commandSuggestions) - 1
	}
	if m.commandSuggestionIndex < 0 {
		m.commandSuggestionIndex = 0
	}
	m.adjustSuggestionWindow(2)
}

func (m *model) clearCommandSuggestions() {
	m.commandSuggestions = nil
	m.commandSuggestionIndex = 0
	m.commandSuggestionOffset = 0
}

func (m model) shouldShowCommandSuggestions() bool {
	return strings.HasPrefix(strings.TrimSpace(m.cmd.Value()), "/") && len(m.commandSuggestions) > 0
}

func (m *model) adjustSuggestionWindow(visibleRows int) {
	if visibleRows < 1 {
		visibleRows = 1
	}
	if m.commandSuggestionIndex < m.commandSuggestionOffset {
		m.commandSuggestionOffset = m.commandSuggestionIndex
	}
	if m.commandSuggestionIndex >= m.commandSuggestionOffset+visibleRows {
		m.commandSuggestionOffset = m.commandSuggestionIndex - visibleRows + 1
	}
	maxOffset := max(0, len(m.commandSuggestions)-visibleRows)
	if m.commandSuggestionOffset > maxOffset {
		m.commandSuggestionOffset = maxOffset
	}
}

func renderCommandSuggestionRows(innerWidth int, matches []commandSpec, selectedIndex int, offset int) string {
	visibleRows := 2
	start := max(0, min(offset, max(0, len(matches)-1)))
	end := min(len(matches), start+visibleRows)

	rows := make([]string, 0, end-start)
	baseRow := lipgloss.NewStyle().
		Background(lipgloss.Color("#1B2330")).
		Width(innerWidth)
	selectedRow := lipgloss.NewStyle().
		Background(lipgloss.Color("#263249")).
		Width(innerWidth)
	for i := start; i < end; i++ {
		cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B9B4D0"))
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D88A8"))
		prefix := "  "
		rowStyle := baseRow
		if i == selectedIndex {
			prefix = "› "
			cmdStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD54A")).Bold(true)
			descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4CDE9"))
			rowStyle = selectedRow
		}
		row := prefix + cmdStyle.Render(matches[i].name) + "  " + descStyle.Render(matches[i].description)
		rows = append(rows, rowStyle.Render(row))
	}

	return strings.Join(rows, "\n")
}

func renderHelpOverlay(maxWidth int) string {
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#5FA8FF")).
		Bold(true).
		Render("Command Help")

	catalog := commandCatalog()
	commands := make([]string, 0, len(catalog))
	for _, cmd := range catalog {
		commands = append(commands, fmt.Sprintf("%-13s %s", cmd.name, cmd.description))
	}
	body := strings.Join(commands, "\n")
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD54A")).
		Bold(true).
		Render("Esc to close")

	content := strings.Join([]string{title, "", body, "", footer}, "\n")
	panelWidth := min(maxWidth-6, 64)
	panelWidth = max(36, panelWidth)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6CBFE6")).
		Padding(1, 2).
		Width(panelWidth).
		Render(content)
}
