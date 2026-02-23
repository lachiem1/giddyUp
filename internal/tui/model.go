package tui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lachiem1/giddyUp/internal/auth"
	"github.com/lachiem1/giddyUp/internal/storage"
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

type deletePATMsg struct {
	err error
}

type wipeDBMsg struct {
	path string
	err  error
}

type accountPreviewRow struct {
	id           string
	displayName  string
	balanceValue string
}

type loadAccountsPreviewMsg struct {
	rows          []accountPreviewRow
	lastFetchedAt *time.Time
	err           error
}

type syncAccountsPreviewMsg struct {
	rows          []accountPreviewRow
	lastFetchedAt *time.Time
	err           error
}

type moveAccountMsg struct {
	err error
}

type accountsClockTickMsg struct {
	sessionID int
}

type accountsAutoRefreshTickMsg struct {
	sessionID int
}

type clearCommandTextMsg struct {
	id int
}

type clearButtonFlashMsg struct {
	id int
}

type commandSpec struct {
	name        string
	description string
}

type authDialogMode int

const (
	authDialogNone authDialogMode = iota
	authDialogConnect
	authDialogDisconnect
)

type screenMode int

const (
	screenHome screenMode = iota
	screenAccounts
)

type model struct {
	db *sql.DB

	width  int
	height int

	viewItems []string
	selected  int
	clicked   int
	clickedID int
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
	authDialog      authDialogMode
	screen          screenMode
	connectHint     string
	accountsRows    []accountPreviewRow
	accountsFetched *time.Time
	accountsErr     string
	accountsLoading bool
	accountsCursor  int
	accountsOffset  int
	accountsSession int
	quitting        bool
}

func New(db *sql.DB) tea.Model {
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
		db: db,
		viewItems: []string{
			"accounts",
			"transactions",
			"spend categories",
			"pay cycle burndown",
		},
		selected:     0,
		clicked:      -1,
		cmd:          cmd,
		pat:          pat,
		status:       stateChecking,
		statusDetail: "not connected",
		authDialog:   authDialogNone,
		screen:       screenHome,
		commandText:  "",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(checkConnectionCmd, m.loadAccountsPreviewCmd())
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
			m.statusDetail = "connected"
		} else {
			m.status = stateDisconnected
			m.statusDetail = "not connected"
		}
		return m, nil

	case savePATMsg:
		m.authDialog = authDialogNone
		m.pat.SetValue("")
		m.pat.Blur()
		m.cmd.Focus()
		if msg.err != nil {
			m.status = stateDisconnected
			m.statusDetail = "not connected"
			return m, nil
		}
		m.status = stateDisconnected
		m.statusDetail = "not connected"
		next, cmd := m.withCommandFeedback("PAT saved to keychain.")
		return next, tea.Batch(cmd, checkConnectionCmd)

	case deletePATMsg:
		m.authDialog = authDialogNone
		m.pat.SetValue("")
		m.pat.Blur()
		m.cmd.Focus()
		if msg.err != nil {
			return m.withCommandFeedback("failed to remove PAT: " + msg.err.Error())
		}
		m.status = stateDisconnected
		m.statusDetail = "not connected"
		return m.withCommandFeedback("PAT removed from keychain.")

	case wipeDBMsg:
		if msg.err != nil {
			return m.withCommandFeedback("db wipe failed: " + msg.err.Error())
		}
		return m.withCommandFeedback("local database wiped: " + msg.path)

	case loadAccountsPreviewMsg:
		if msg.err != nil {
			if len(m.accountsRows) == 0 {
				m.accountsErr = msg.err.Error()
			}
			return m, nil
		}
		m.accountsErr = ""
		m.accountsRows = msg.rows
		m.accountsFetched = msg.lastFetchedAt
		if m.accountsCursor >= len(m.accountsRows) {
			m.accountsCursor = max(0, len(m.accountsRows)-1)
		}
		m.ensureAccountsScrollWindow()
		return m, nil

	case syncAccountsPreviewMsg:
		m.accountsLoading = false
		if msg.err != nil {
			if len(m.accountsRows) == 0 {
				m.accountsErr = msg.err.Error()
			}
			return m, nil
		}
		m.accountsErr = ""
		m.accountsRows = msg.rows
		m.accountsFetched = msg.lastFetchedAt
		if m.accountsCursor >= len(m.accountsRows) {
			m.accountsCursor = max(0, len(m.accountsRows)-1)
		}
		m.ensureAccountsScrollWindow()
		return m, nil

	case moveAccountMsg:
		if msg.err != nil {
			if len(m.accountsRows) == 0 {
				m.accountsErr = msg.err.Error()
			}
			return m, nil
		}
		return m, m.loadAccountsPreviewCmd()

	case accountsClockTickMsg:
		if msg.sessionID != m.accountsSession || m.screen != screenAccounts {
			return m, nil
		}
		return m, m.accountsClockTickCmd()

	case accountsAutoRefreshTickMsg:
		if msg.sessionID != m.accountsSession || m.screen != screenAccounts {
			return m, nil
		}
		return m, tea.Batch(
			m.syncAndReloadAccountsPreviewCmd(true),
			m.accountsAutoRefreshTickCmd(),
		)

	case clearCommandTextMsg:
		if msg.id == m.commandTextID {
			m.commandText = ""
		}
		return m, nil

	case clearButtonFlashMsg:
		if msg.id == m.clickedID {
			m.clicked = -1
		}
		return m, nil

	case tea.MouseMsg:
		if m.showHelpOverlay || m.authDialog != authDialogNone {
			return m, nil
		}
		if m.screen != screenHome {
			return m, nil
		}

		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			clicked := m.selectButtonAt(msg.X, msg.Y)
			switch clicked {
			case 0:
				m.clicked = 0
				m.clickedID++
				m.selected = 0
				next, cmd := m.withCommandFeedback("accounts view selected")
				return next, tea.Batch(cmd, clearButtonFlashCmd(m.clickedID))
			case 1:
				m.clicked = 1
				m.clickedID++
				m.selected = 1
				next, cmd := m.withCommandFeedback("transactions view selected")
				return next, tea.Batch(cmd, clearButtonFlashCmd(m.clickedID))
			}
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

		if m.authDialog != authDialogNone {
			switch msg.String() {
			case "esc":
				m.authDialog = authDialogNone
				m.pat.Blur()
				m.cmd.Focus()
				return m, nil
			case "enter":
				if m.authDialog == authDialogConnect {
					pat := strings.TrimSpace(m.pat.Value())
					return m, savePATCmd(pat)
				}
				if m.authDialog == authDialogDisconnect {
					return m, deletePATCmd
				}
			}
			if m.authDialog == authDialogDisconnect {
				return m, nil
			}
			var cmd tea.Cmd
			m.pat, cmd = m.pat.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "shift+up":
			if m.screen == screenAccounts && len(m.accountsRows) > 0 && m.accountsCursor > 0 {
				m.accountsCursor--
				m.ensureAccountsScrollWindow()
				id := m.accountsRows[m.accountsCursor+1].id
				return m, m.moveAccountCmd(id, -1)
			}
			return m, nil
		case "shift+down":
			if m.screen == screenAccounts && len(m.accountsRows) > 0 && m.accountsCursor < len(m.accountsRows)-1 {
				m.accountsCursor++
				m.ensureAccountsScrollWindow()
				id := m.accountsRows[m.accountsCursor-1].id
				return m, m.moveAccountCmd(id, +1)
			}
			return m, nil
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if m.screen == screenAccounts &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				m.screen = screenHome
				m.accountsSession++
				return m, nil
			}
			if strings.TrimSpace(m.cmd.Value()) != "" || m.shouldShowCommandSuggestions() {
				m.cmd.SetValue("")
				m.clearCommandSuggestions()
				return m, nil
			}
		case "up", "k":
			if m.screen == screenAccounts {
				if m.accountsCursor > 0 {
					m.accountsCursor--
				}
				m.ensureAccountsScrollWindow()
				return m, nil
			}
			if m.shouldShowCommandSuggestions() {
				if m.commandSuggestionIndex > 0 {
					m.commandSuggestionIndex--
				}
				m.adjustSuggestionWindow(2)
				return m, nil
			}
			if m.screen == screenHome && m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.screen == screenAccounts {
				if m.accountsCursor < len(m.accountsRows)-1 {
					m.accountsCursor++
				}
				m.ensureAccountsScrollWindow()
				return m, nil
			}
			if m.shouldShowCommandSuggestions() {
				if m.commandSuggestionIndex < len(m.commandSuggestions)-1 {
					m.commandSuggestionIndex++
				}
				m.adjustSuggestionWindow(2)
				return m, nil
			}
			if m.screen == screenHome && m.selected < len(m.viewItems)-1 {
				m.selected++
			}
		case "enter":
			if m.screen == screenHome &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() &&
				m.selected == 0 {
				return m.enterAccountsView()
			}
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
	if m.screen == screenAccounts {
		content := contentStyle.Render(m.renderAccountsScreen(layoutWidth))
		if m.showHelpOverlay {
			helpOverlay := renderHelpOverlay(layoutWidth)
			layoutHeight := max(1, m.height-frame.GetVerticalFrameSize()-contentStyle.GetVerticalFrameSize())
			centered := lipgloss.Place(layoutWidth, layoutHeight, lipgloss.Center, lipgloss.Center, helpOverlay)
			return frame.Render(contentStyle.Render(centered))
		}
		if m.authDialog != authDialogNone {
			authOverlay := m.renderAuthDialog(layoutWidth)
			layoutHeight := max(1, m.height-frame.GetVerticalFrameSize()-contentStyle.GetVerticalFrameSize())
			centered := lipgloss.Place(layoutWidth, layoutHeight, lipgloss.Center, lipgloss.Center, authOverlay)
			return frame.Render(contentStyle.Render(centered))
		}
		return frame.Render(content)
	}

	header := renderBlockTitle()
	if m.width > 0 {
		header = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, header)
	}
	header = lipgloss.NewStyle().PaddingBottom(1).Render(header)

	statusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#5FA8FF")).Bold(true).Render("status: ")
	statusValue := lipgloss.NewStyle().Foreground(lipgloss.Color("#F15B5B")).Bold(true).Render("not connected")
	if m.status == stateConnected {
		statusValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#5CCB76")).Bold(true).Render("connected")
	}
	statusLine := statusLabel + statusValue

	listWidth := 24
	rightWidth := max(36, m.width-listWidth-20)
	panelHeight := 14
	listBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#F47A60")).
		Padding(0, 1).
		Height(panelHeight).
		Width(listWidth).
		Render(renderViews(m.viewItems, m.selected, statusLine))

	panelWidth := max(18, (rightWidth-2)/2)
	pinnedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFD54A")).
		Padding(0, 1).
		Width(panelWidth).
		Height(panelHeight)
	pinTitle := pinIconOrFallback()
	leftSelect := renderSelectButton(m.clicked == 0)
	rightSelect := renderSelectButton(m.clicked == 1)
	leftHeader := pinTitle + " " + leftSelect
	rightHeader := pinTitle + " " + rightSelect
	pinnedOne := pinnedStyle.Render(leftHeader)
	pinnedTwo := pinnedStyle.Render(rightHeader)
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

		coreHeight := lipgloss.Height(header) + lipgloss.Height(mainPanelsRaw) + lipgloss.Height(bottomSection) + headerGap
		if hasMessage {
			coreHeight += messageGap + lipgloss.Height(messageArea)
		}

		// Keep the cards->CLI gap as the primary compression buffer.
		bridgeGap = max(bridgeMin, availableHeight-coreHeight)
		if coreHeight+bridgeGap > availableHeight {
			// Once bridge has reached its minimum, collapse secondary blank space.
			headerGap = 0
			coreHeight = lipgloss.Height(header) + lipgloss.Height(mainPanelsRaw) + lipgloss.Height(bottomSection)
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

	content := contentStyle.Render(bodyText)

	if m.showHelpOverlay {
		helpOverlay := renderHelpOverlay(layoutWidth)
		layoutHeight := max(1, m.height-frame.GetVerticalFrameSize()-contentStyle.GetVerticalFrameSize())
		centered := lipgloss.Place(layoutWidth, layoutHeight, lipgloss.Center, lipgloss.Center, helpOverlay)
		return frame.Render(contentStyle.Render(centered))
	}
	if m.authDialog != authDialogNone {
		authOverlay := m.renderAuthDialog(layoutWidth)
		layoutHeight := max(1, m.height-frame.GetVerticalFrameSize()-contentStyle.GetVerticalFrameSize())
		centered := lipgloss.Place(layoutWidth, layoutHeight, lipgloss.Center, lipgloss.Center, authOverlay)
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
		return m.enterAccountsView()
	case "/transactions":
		m.selected = 1
		m.screen = screenHome
		return m.withCommandFeedback("transactions view selected")
	case "/ping":
		next, cmd := m.withCommandFeedback("checking connection...")
		return next, tea.Batch(cmd, checkConnectionCmd)
	case "/db-wipe", "/db wipe":
		next, cmd := m.withCommandFeedback("wiping local database...")
		return next, tea.Batch(cmd, wipeDBCmd)
	case "/disconnect":
		m.authDialog = authDialogDisconnect
		m.pat.SetValue("")
		m.pat.Blur()
		m.cmd.Blur()
		m.clearCommandSuggestions()
		return m, nil
	case "/settings":
		return m.withCommandFeedback("settings has been removed from the views list.")
	case "/connect":
		hasPAT, err := auth.HasStoredPAT()
		if err != nil {
			return m.withCommandFeedback("failed to check stored PAT: " + err.Error())
		}
		m.connectHint = "Enter your PAT to save it to keychain."
		if hasPAT {
			m.connectHint = "A PAT already exists. Enter a new PAT to replace it."
		}
		m.authDialog = authDialogConnect
		m.pat.Focus()
		m.cmd.Blur()
		m.clearCommandSuggestions()
		return m, nil
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

func (m model) enterAccountsView() (tea.Model, tea.Cmd) {
	m.selected = 0
	m.screen = screenAccounts
	m.accountsErr = ""
	m.accountsLoading = true
	m.accountsSession++
	return m, tea.Batch(
		m.loadAccountsPreviewCmd(),
		m.syncAndReloadAccountsPreviewCmd(false),
		m.accountsClockTickCmd(),
		m.accountsAutoRefreshTickCmd(),
	)
}

func (m *model) ensureAccountsScrollWindow() {
	visible := m.accountsVisibleRows()
	if visible < 1 {
		visible = 1
	}
	if m.accountsCursor < m.accountsOffset {
		m.accountsOffset = m.accountsCursor
	}
	if m.accountsCursor >= m.accountsOffset+visible {
		m.accountsOffset = m.accountsCursor - visible + 1
	}
	maxOffset := max(0, len(m.accountsRows)-visible)
	if m.accountsOffset > maxOffset {
		m.accountsOffset = maxOffset
	}
	if m.accountsOffset < 0 {
		m.accountsOffset = 0
	}
}

func (m model) accountsVisibleRows() int {
	return 6
}

func (m model) accountsClockTickCmd() tea.Cmd {
	session := m.accountsSession
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return accountsClockTickMsg{sessionID: session}
	})
}

func (m model) accountsAutoRefreshTickCmd() tea.Cmd {
	session := m.accountsSession
	return tea.Tick(60*time.Second, func(time.Time) tea.Msg {
		return accountsAutoRefreshTickMsg{sessionID: session}
	})
}

func (m model) moveAccountCmd(accountID string, delta int) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return moveAccountMsg{err: fmt.Errorf("database is not initialized")}
		}
		if err := moveAccountDisplayOrder(context.Background(), m.db, accountID, delta); err != nil {
			return moveAccountMsg{err: err}
		}
		return moveAccountMsg{}
	}
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

func clearButtonFlashCmd(id int) tea.Cmd {
	return tea.Tick(450*time.Millisecond, func(time.Time) tea.Msg {
		return clearButtonFlashMsg{id: id}
	})
}

func deletePATCmd() tea.Msg {
	return deletePATMsg{err: auth.RemovePAT()}
}

func wipeDBCmd() tea.Msg {
	cfg, err := storage.Wipe()
	if err != nil {
		return wipeDBMsg{err: err}
	}

	db, _, err := storage.Open(context.Background())
	if err != nil {
		return wipeDBMsg{err: fmt.Errorf("reinitialize database: %w", err)}
	}
	_ = db.Close()

	return wipeDBMsg{path: cfg.Path}
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

func renderViews(items []string, selected int, statusLine string) string {
	lines := []string{statusLine, ""}
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
	return renderStyledBlockTitle(raw, segments)
}

func isStrokeRune(ch rune) bool {
	switch ch {
	case '╔', '╗', '╚', '╝', '║', '═', '┌', '┐', '└', '┘', '│', '─':
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
		{name: "/help", description: "show command help overlay"},
		{name: "/accounts", description: "select the accounts view"},
		{name: "/transactions", description: "select the transactions view"},
		{name: "/ping", description: "check Up API connectivity"},
		{name: "/disconnect", description: "remove saved PAT from keychain"},
		{name: "/db-wipe", description: "wipe and reinitialize the local database"},
		{name: "/connect", description: "open the PAT connect prompt"},
		{name: "/settings", description: "legacy command (removed view)"},
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

func (m model) renderAuthDialog(maxWidth int) string {
	panelWidth := min(maxWidth-6, 64)
	panelWidth = max(44, panelWidth)

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6CBFE6")).
		Padding(1, 2).
		Width(panelWidth)

	switch m.authDialog {
	case authDialogConnect:
		title := "Connect to Up"
		hint := m.connectHint
		if strings.TrimSpace(hint) == "" {
			hint = "Enter your PAT to save it to keychain."
		}

		patInput := m.pat
		patInput.Width = max(18, panelWidth-8)

		content := strings.Join([]string{
			title,
			"",
			hint,
			"",
			patInput.View(),
			"",
			"Enter to save, Esc to cancel",
		}, "\n")
		return panel.Render(content)
	case authDialogDisconnect:
		content := strings.Join([]string{
			"Disconnect from Up",
			"",
			"This will remove your saved PAT from keychain.",
			"",
			"Enter to remove PAT, Esc to cancel",
		}, "\n")
		return panel.Render(content)
	default:
		return ""
	}
}

type hitRect struct {
	x int
	y int
	w int
	h int
}

func (r hitRect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

func renderSelectButton(clicked bool) string {
	bracketStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD54A")).
		Bold(true)
	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD54A")).
		Bold(true)
	if clicked {
		textStyle = textStyle.Underline(true)
	}
	return bracketStyle.Render("[") + textStyle.Render("select view") + bracketStyle.Render("]")
}

func (m model) selectButtonAt(x, y int) int {
	rects := m.selectButtonRects()
	for i, rect := range rects {
		if rect.contains(x, y) {
			return i
		}
	}
	return -1
}

func (m model) selectButtonRects() []hitRect {
	if m.width < 1 || m.height < 1 {
		return nil
	}

	frame := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#F47A60")).
		Padding(1, 1)
	contentStyle := lipgloss.NewStyle().Padding(1, 1, 0, 1)
	frame = frame.Width(max(1, m.width-frame.GetHorizontalBorderSize()))
	frame = frame.Height(max(1, m.height-frame.GetVerticalBorderSize()))
	layoutWidth := max(1, m.width-frame.GetHorizontalFrameSize()-contentStyle.GetHorizontalFrameSize())

	header := renderBlockTitle()
	header = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, header)
	header = lipgloss.NewStyle().PaddingBottom(1).Render(header)

	listWidth := 24
	rightWidth := max(36, m.width-listWidth-20)
	panelHeight := 14
	listBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#F47A60")).
		Padding(0, 1).
		Height(panelHeight).
		Width(listWidth).
		Render(renderViews(m.viewItems, m.selected, ""))

	panelWidth := max(18, (rightWidth-2)/2)
	pinnedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFD54A")).
		Padding(0, 1).
		Width(panelWidth).
		Height(panelHeight)
	pinTitle := pinIconOrFallback()
	pinnedOne := pinnedStyle.Render(pinTitle + " " + renderSelectButton(false))
	pinnedTwo := pinnedStyle.Render(pinTitle + " " + renderSelectButton(false))
	rightPanels := lipgloss.JoinHorizontal(lipgloss.Top, pinnedOne, "  ", pinnedTwo)

	mainPanelsRaw := lipgloss.JoinHorizontal(lipgloss.Top, listBox, "  ", rightPanels)
	mainPanelsWidth := lipgloss.Width(mainPanelsRaw)

	mainPanelsTopGap := 1
	if m.height > 0 {
		cmdOuterWidth := lipgloss.Width(mainPanelsRaw)
		cmdInnerWidth := max(8, cmdOuterWidth-4)
		cmdInput := m.cmd
		cmdInput.Width = max(6, cmdInnerWidth-2)
		cmdLines := []string{}
		if m.shouldShowCommandSuggestions() {
			cmdLines = append(cmdLines, renderCommandSuggestionRows(cmdInnerWidth, m.commandSuggestions, m.commandSuggestionIndex, m.commandSuggestionOffset))
		}
		cmdLines = append(cmdLines, lipgloss.NewStyle().Width(cmdInnerWidth).Render(cmdInput.View()))
		cmdInner := strings.Join(cmdLines, "\n")
		bottomSection := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#6CBFE6")).
			Padding(0, 1).
			Render(cmdInner)

		headerGap := 1
		hasMessage := strings.TrimSpace(m.commandText) != ""
		messageGap := 0
		if hasMessage {
			messageGap = 1
		}

		availableHeight := max(0, m.height-frame.GetVerticalFrameSize()-contentStyle.GetVerticalFrameSize())
		coreHeight := lipgloss.Height(header) + lipgloss.Height(mainPanelsRaw) + lipgloss.Height(bottomSection) + headerGap
		if hasMessage {
			messageArea := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#6CBFE6")).
				Padding(0, 1).
				Foreground(lipgloss.Color("#D4CDE9")).
				Width(max(8, lipgloss.Width(mainPanelsRaw)-4)).
				Render(m.commandText)
			coreHeight += messageGap + lipgloss.Height(messageArea)
		}
		bridgeGap := max(1, availableHeight-coreHeight)
		if coreHeight+bridgeGap > availableHeight {
			headerGap = 0
		}
		mainPanelsTopGap = headerGap
	}

	bodyX := frame.GetHorizontalFrameSize()/2 + contentStyle.GetPaddingLeft()
	bodyY := frame.GetVerticalFrameSize()/2 + contentStyle.GetPaddingTop()
	mainPanelsX := bodyX + max(0, (layoutWidth-mainPanelsWidth)/2)
	mainPanelsY := bodyY + lipgloss.Height(header) + mainPanelsTopGap

	leftPanelX := mainPanelsX + lipgloss.Width(listBox) + 2
	secondPanelX := leftPanelX + lipgloss.Width(pinnedOne) + 2
	buttonXOffset := 2 + lipgloss.Width(pinTitle+" ")
	buttonY := mainPanelsY + 1
	buttonW := lipgloss.Width("[select view]")

	return []hitRect{
		{x: leftPanelX + buttonXOffset, y: buttonY, w: buttonW, h: 1},
		{x: secondPanelX + buttonXOffset, y: buttonY, w: buttonW, h: 1},
	}
}
