package tui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
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
	id              string
	displayName     string
	accountType     string
	ownershipType   string
	balanceCurrency string
	createdAt       string
	isActive        bool
	balanceValue    string
	goalBalance     string
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

type saveAccountGoalMsg struct {
	err error
}

type accountsClockTickMsg struct {
	sessionID int
}

type accountsAutoRefreshTickMsg struct {
	sessionID int
}

type loadConfigMsg struct {
	nextPayDate string
	frequency   string
	err         error
}

type saveConfigMsg struct {
	err    error
	silent bool
}

type transactionPreviewRow struct {
	createdAt   string
	merchant    string
	id          string
	rawText     string
	description string
	amountValue string
	status      string
	message     string
	categoryID  string
	cardMethod  string
	noteText    string
	accountName string
}

type transactionsCategorySpend struct {
	category       string
	spendCents     int64
	percentOfSpend float64
}

type loadTransactionsPreviewMsg struct {
	rows          []transactionPreviewRow
	categorySpend []transactionsCategorySpend
	lastFetchedAt *time.Time
	totalCount    int
	page          int
	err           error
}

type categoryTransactionRow struct {
	id          string
	createdAt   string
	merchant    string
	description string
	amountValue string
}

type loadCategoryTransactionsMsg struct {
	category string
	rows     []categoryTransactionRow
	err      error
}

type loadTransactionsFiltersMsg struct {
	fromDate        string
	toDate          string
	mode            int
	quickIdx        int
	includeInternal bool
	err             error
}

type saveTransactionsFiltersMsg struct {
	err error
}

type syncTransactionsDoneMsg struct {
	sessionID int
	err       error
}

type transactionsReloadTickMsg struct {
	sessionID int
}

type transactionsClockTickMsg struct {
	sessionID int
}

type transactionsAutoRefreshTickMsg struct {
	sessionID int
}

type transactionsPrewarmCheckMsg struct {
	empty bool
	err   error
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
	screenConfig
	screenTransactions
	screenTransactionsFilters
)

const (
	accountsFocusCards = iota
	accountsFocusPane
)

const (
	transactionsFocusFromDate = iota
	transactionsFocusToDate
	transactionsFocusQuickRange
	transactionsFocusIncludeInternal
)

const (
	transactionsFilterModeQuick = iota
	transactionsFilterModeCustom
)

const (
	transactionsViewModeTable = iota
	transactionsViewModeChart
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

	showHelpOverlay             bool
	authDialog                  authDialogMode
	screen                      screenMode
	connectHint                 string
	accountsRows                []accountPreviewRow
	accountsFetched             *time.Time
	accountsErr                 string
	accountsLoading             bool
	accountsCursor              int
	accountsOffset              int
	accountsSession             int
	accountsPaneOpen            bool
	accountsPaneFocus           int
	accountsAction              int
	accountsGoalEditing         bool
	accountsGoalErr             string
	accountsGoalInput           textinput.Model
	configNextPayDigits         string
	configFrequencyIndex        int
	configLastSavedDate         string
	configDateDirty             bool
	configFocus                 int
	configErr                   string
	transactionsRows            []transactionPreviewRow
	transactionsCategorySpend   []transactionsCategorySpend
	transactionsCursor          int
	transactionsOffset          int
	transactionsErr             string
	transactionsFetched         *time.Time
	transactionsSyncing         bool
	transactionsSession         int
	transactionsLastSync        *time.Time
	transactionsPage            int
	transactionsPageSize        int
	transactionsTotal           int
	transactionsFromDate        string
	transactionsToDate          string
	transactionsQuickIdx        int
	transactionsSortIdx         int
	transactionsViewMode        int
	transactionsFocus           int
	transactionsDateErr         string
	transactionsFilterMode      int
	transactionsIncludeInternal bool
	transactionsPaneOpen        bool
	transactionsSearchInput     textinput.Model
	transactionsSearchApplied   string
	transactionsSearchErr       string
	transactionsSearchActive    bool
	transactionsChartCursor     int
	transactionsChartOffset     int
	transactionsChartPaneOpen   bool
	transactionsChartPaneRows   []categoryTransactionRow
	transactionsChartPaneCursor int
	transactionsChartPaneOffset int
	transactionsChartPaneTitle  string
	transactionsCalendarOpen    bool
	transactionsCalendarMonth   time.Time
	transactionsCalendarCursor  time.Time
	transactionsCalendarTarget  int
	quitting                    bool
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

	goalInput := textinput.New()
	goalInput.Prompt = "$ "
	goalInput.Placeholder = "0.00"
	goalInput.Width = 20

	transactionsSearchInput := textinput.New()
	transactionsSearchInput.Prompt = ""
	transactionsSearchInput.Placeholder = "e.g. /merchant: WOOL + amount: >60 + type: -ve"
	transactionsSearchInput.Width = 72

	return model{
		db: db,
		viewItems: []string{
			"config",
			"accounts",
			"transactions",
			"spend categories",
			"pay cycle burndown",
		},
		selected:                    0,
		clicked:                     -1,
		cmd:                         cmd,
		pat:                         pat,
		status:                      stateChecking,
		statusDetail:                "not connected",
		authDialog:                  authDialogNone,
		screen:                      screenHome,
		commandText:                 "",
		accountsGoalInput:           goalInput,
		configFrequencyIndex:        0,
		transactionsPageSize:        8,
		transactionsFilterMode:      transactionsFilterModeQuick,
		transactionsIncludeInternal: true,
		transactionsViewMode:        transactionsViewModeTable,
		transactionsSearchInput:     transactionsSearchInput,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		checkConnectionCmd,
		m.loadAccountsPreviewCmd(),
		m.transactionsPrewarmCheckCmd(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.cmd.Width = max(40, msg.Width-36)
		m.pat.Width = max(24, msg.Width-40)
		m.transactionsSearchInput.Width = max(24, msg.Width-36)
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
		m.clampAccountsAction()
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
		m.clampAccountsAction()
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

	case saveAccountGoalMsg:
		if msg.err != nil {
			m.accountsGoalErr = msg.err.Error()
			return m, nil
		}
		m.accountsGoalErr = ""
		m.accountsGoalEditing = false
		m.accountsGoalInput.Blur()
		m.accountsGoalInput.SetValue("")
		next, cmd := m.withCommandFeedback("goal balance saved")
		return next, tea.Batch(cmd, m.loadAccountsPreviewCmd())

	case loadConfigMsg:
		if msg.err != nil {
			m.configErr = msg.err.Error()
			return m, nil
		}
		m.configErr = ""
		m.configNextPayDigits = dateToDigits(msg.nextPayDate)
		m.configFrequencyIndex = frequencyIndexFromValue(msg.frequency)
		m.configLastSavedDate = msg.nextPayDate
		m.configDateDirty = false
		return m, nil

	case saveConfigMsg:
		if msg.err != nil {
			m.configErr = msg.err.Error()
			return m, nil
		}
		m.configErr = ""
		return m, nil

	case loadTransactionsPreviewMsg:
		if msg.err != nil {
			m.transactionsErr = msg.err.Error()
			return m, nil
		}
		m.transactionsErr = ""
		m.transactionsRows = msg.rows
		m.transactionsCategorySpend = msg.categorySpend
		if len(m.transactionsCategorySpend) == 0 {
			m.transactionsChartCursor = 0
		} else if m.transactionsChartCursor >= len(m.transactionsCategorySpend) {
			m.transactionsChartCursor = len(m.transactionsCategorySpend) - 1
		}
		if m.transactionsChartCursor < 0 {
			m.transactionsChartCursor = 0
		}
		m.ensureTransactionsChartScrollWindow()
		if m.transactionsChartPaneOpen {
			m.transactionsChartPaneOpen = false
			m.transactionsChartPaneRows = nil
			m.transactionsChartPaneCursor = 0
			m.transactionsChartPaneOffset = 0
			m.transactionsChartPaneTitle = ""
		}
		m.transactionsFetched = msg.lastFetchedAt
		m.transactionsTotal = msg.totalCount
		if msg.page >= 0 {
			m.transactionsPage = msg.page
		}
		if m.transactionsCursor >= len(m.transactionsRows) {
			m.transactionsCursor = max(0, len(m.transactionsRows)-1)
		}
		m.ensureTransactionsScrollWindow()
		return m, nil

	case loadCategoryTransactionsMsg:
		if msg.err != nil {
			m.transactionsErr = msg.err.Error()
			return m, nil
		}
		m.transactionsErr = ""
		m.transactionsChartPaneOpen = true
		m.transactionsChartPaneTitle = msg.category
		m.transactionsChartPaneRows = msg.rows
		m.transactionsChartPaneCursor = 0
		m.transactionsChartPaneOffset = 0
		m.ensureTransactionsChartPaneScrollWindow()
		return m, nil

	case loadTransactionsFiltersMsg:
		if msg.err == nil {
			m.transactionsFromDate = strings.TrimSpace(msg.fromDate)
			m.transactionsToDate = strings.TrimSpace(msg.toDate)
			if msg.mode == transactionsFilterModeCustom {
				m.transactionsFilterMode = transactionsFilterModeCustom
			} else {
				m.transactionsFilterMode = transactionsFilterModeQuick
			}
			ranges := transactionsQuickRanges()
			if msg.quickIdx >= 0 && msg.quickIdx < len(ranges) {
				m.transactionsQuickIdx = msg.quickIdx
			}
			m.transactionsIncludeInternal = msg.includeInternal
		}
		return m, m.loadTransactionsPreviewCmd()

	case saveTransactionsFiltersMsg:
		if msg.err != nil {
			m.transactionsErr = msg.err.Error()
		}
		return m, nil

	case syncTransactionsDoneMsg:
		if msg.sessionID != m.transactionsSession {
			return m, nil
		}
		m.transactionsSyncing = false
		now := time.Now().UTC()
		m.transactionsLastSync = &now
		return m, m.loadTransactionsPreviewCmd()

	case transactionsReloadTickMsg:
		if msg.sessionID != m.transactionsSession || (m.screen != screenTransactions && m.screen != screenTransactionsFilters) || !m.transactionsSyncing {
			return m, nil
		}
		return m, tea.Batch(m.loadTransactionsPreviewCmd(), m.transactionsReloadTickCmd())

	case transactionsClockTickMsg:
		if msg.sessionID != m.transactionsSession || (m.screen != screenTransactions && m.screen != screenTransactionsFilters) {
			return m, nil
		}
		return m, m.transactionsClockTickCmd()

	case transactionsAutoRefreshTickMsg:
		if msg.sessionID != m.transactionsSession || (m.screen != screenTransactions && m.screen != screenTransactionsFilters) {
			return m, nil
		}
		next, syncCmd := m.maybeStartTransactionsSyncCmd(false)
		return next, tea.Batch(syncCmd, m.transactionsAutoRefreshTickCmd())

	case transactionsPrewarmCheckMsg:
		if msg.err != nil || !msg.empty {
			return m, nil
		}
		next, cmd := m.maybeStartTransactionsSyncCmd(false)
		return next, cmd

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
				m.selected = 1
				return m, clearButtonFlashCmd(m.clickedID)
			case 1:
				m.clicked = 1
				m.clickedID++
				m.selected = 2
				return m, clearButtonFlashCmd(m.clickedID)
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
		if m.screen == screenConfig {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			case "esc":
				m.screen = screenHome
				m.configErr = ""
				m.cmd.Focus()
				return m, nil
			case "tab", "up", "down", "j", "k":
				if m.configFocus == 0 {
					m.configFocus = 1
				} else {
					m.configFocus = 0
				}
				return m, nil
			case "left", "h":
				if m.configFocus == 1 {
					opts := configFrequencyOptions()
					m.configFrequencyIndex = (m.configFrequencyIndex - 1 + len(opts)) % len(opts)
					return m, nil
				}
			case "right", "l":
				if m.configFocus == 1 {
					opts := configFrequencyOptions()
					m.configFrequencyIndex = (m.configFrequencyIndex + 1) % len(opts)
					return m, nil
				}
			case "enter":
				date, err := validateAndFormatDateDigits(m.configNextPayDigits, m.configDateDirty)
				if err != nil {
					m.configErr = err.Error()
					return m, nil
				}
				freq := configFrequencyOptions()[m.configFrequencyIndex]
				m.configErr = ""
				return m, m.saveConfigCmd(date, freq)
			case "backspace", "delete":
				if m.configFocus == 0 {
					if len(m.configNextPayDigits) > 0 {
						m.configNextPayDigits = m.configNextPayDigits[:len(m.configNextPayDigits)-1]
						m.configDateDirty = true
					}
					m.configErr = ""
					return m, nil
				}
			}

			var cmd tea.Cmd
			if m.configFocus == 0 {
				if msg.Type == tea.KeyRunes {
					for _, ch := range msg.Runes {
						if ch >= '0' && ch <= '9' && len(m.configNextPayDigits) < 8 {
							m.configNextPayDigits += string(ch)
							m.configDateDirty = true
						}
					}
				}
				if !m.configDateDirty {
					return m, nil
				}
				formatted, err := validateAndFormatDateDigits(m.configNextPayDigits, true)
				if err != nil {
					// Show warnings only when full date is present.
					if len(m.configNextPayDigits) == 8 {
						m.configErr = err.Error()
					} else {
						m.configErr = ""
					}
					return m, nil
				}
				m.configErr = ""
				if len(m.configNextPayDigits) == 8 && formatted != m.configLastSavedDate {
					m.configLastSavedDate = formatted
					return m, m.saveConfigDateCmd(formatted)
				}
			}
			return m, cmd
		}
		if m.screen == screenAccounts && m.accountsGoalEditing {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			case "esc":
				m.accountsGoalEditing = false
				m.accountsGoalErr = ""
				m.accountsGoalInput.SetValue("")
				m.accountsGoalInput.Blur()
				return m, nil
			case "enter":
				raw := strings.TrimSpace(m.accountsGoalInput.Value())
				if raw == "" {
					m.accountsGoalErr = "enter a number"
					return m, nil
				}
				n, err := strconv.ParseFloat(raw, 64)
				if err != nil || n < 0 {
					m.accountsGoalErr = "invalid amount"
					return m, nil
				}
				if len(m.accountsRows) == 0 || m.accountsCursor >= len(m.accountsRows) {
					m.accountsGoalErr = "no account selected"
					return m, nil
				}
				m.accountsGoalErr = ""
				formatted := fmt.Sprintf("%.2f", n)
				return m, m.saveAccountGoalCmd(m.accountsRows[m.accountsCursor].id, formatted)
			}

			var cmd tea.Cmd
			m.accountsGoalInput, cmd = m.accountsGoalInput.Update(msg)
			m.accountsGoalInput.SetValue(normalizeGoalInput(m.accountsGoalInput.Value()))
			return m, cmd
		}

		if m.screen == screenTransactionsFilters &&
			strings.TrimSpace(m.cmd.Value()) == "" &&
			!m.shouldShowCommandSuggestions() &&
			(m.transactionsFocus == transactionsFocusFromDate || m.transactionsFocus == transactionsFocusToDate) {
			if m.transactionsCalendarOpen {
				switch msg.String() {
				case "shift+left":
					m.transactionsCalendarCursor = shiftCalendarByMonths(m.transactionsCalendarCursor, -1)
					m.transactionsCalendarMonth = time.Date(
						m.transactionsCalendarCursor.Year(),
						m.transactionsCalendarCursor.Month(),
						1, 0, 0, 0, 0, time.Local,
					)
					return m, nil
				case "shift+right":
					m.transactionsCalendarCursor = shiftCalendarByMonths(m.transactionsCalendarCursor, 1)
					m.transactionsCalendarMonth = time.Date(
						m.transactionsCalendarCursor.Year(),
						m.transactionsCalendarCursor.Month(),
						1, 0, 0, 0, 0, time.Local,
					)
					return m, nil
				case "shift+up":
					m.transactionsCalendarCursor = shiftCalendarByYears(m.transactionsCalendarCursor, -1)
					m.transactionsCalendarMonth = time.Date(
						m.transactionsCalendarCursor.Year(),
						m.transactionsCalendarCursor.Month(),
						1, 0, 0, 0, 0, time.Local,
					)
					return m, nil
				case "shift+down":
					m.transactionsCalendarCursor = shiftCalendarByYears(m.transactionsCalendarCursor, 1)
					m.transactionsCalendarMonth = time.Date(
						m.transactionsCalendarCursor.Year(),
						m.transactionsCalendarCursor.Month(),
						1, 0, 0, 0, 0, time.Local,
					)
					return m, nil
				case "left":
					m.transactionsCalendarCursor = m.transactionsCalendarCursor.AddDate(0, 0, -1)
					m.transactionsCalendarMonth = time.Date(
						m.transactionsCalendarCursor.Year(),
						m.transactionsCalendarCursor.Month(),
						1, 0, 0, 0, 0, time.Local,
					)
					return m, nil
				case "right":
					m.transactionsCalendarCursor = m.transactionsCalendarCursor.AddDate(0, 0, 1)
					m.transactionsCalendarMonth = time.Date(
						m.transactionsCalendarCursor.Year(),
						m.transactionsCalendarCursor.Month(),
						1, 0, 0, 0, 0, time.Local,
					)
					return m, nil
				case "up":
					m.transactionsCalendarCursor = m.transactionsCalendarCursor.AddDate(0, 0, -7)
					m.transactionsCalendarMonth = time.Date(
						m.transactionsCalendarCursor.Year(),
						m.transactionsCalendarCursor.Month(),
						1, 0, 0, 0, 0, time.Local,
					)
					return m, nil
				case "down":
					m.transactionsCalendarCursor = m.transactionsCalendarCursor.AddDate(0, 0, 7)
					m.transactionsCalendarMonth = time.Date(
						m.transactionsCalendarCursor.Year(),
						m.transactionsCalendarCursor.Month(),
						1, 0, 0, 0, 0, time.Local,
					)
					return m, nil
				case "enter":
					digits := fmt.Sprintf("%04d%02d%02d",
						m.transactionsCalendarCursor.Year(),
						int(m.transactionsCalendarCursor.Month()),
						m.transactionsCalendarCursor.Day(),
					)
					if m.transactionsCalendarTarget == transactionsFocusFromDate {
						m.transactionsFromDate = digits
					} else {
						m.transactionsToDate = digits
					}
					m.transactionsFilterMode = transactionsFilterModeCustom
					m.transactionsCalendarOpen = false
					m.transactionsDateErr = ""
					return m, nil
				case "esc":
					m.transactionsCalendarOpen = false
					return m, nil
				}
			}
			switch msg.Type {
			case tea.KeyRunes:
				if len(msg.Runes) > 0 {
					d := msg.Runes[0]
					if d >= '0' && d <= '9' {
						if m.transactionsFocus == transactionsFocusFromDate {
							m.transactionsFromDate = appendDateDigit(m.transactionsFromDate, d)
						} else {
							m.transactionsToDate = appendDateDigit(m.transactionsToDate, d)
						}
						m.transactionsFilterMode = transactionsFilterModeCustom
						m.transactionsDateErr = ""
						return m, nil
					}
				}
			case tea.KeyBackspace, tea.KeyDelete:
				if m.transactionsFocus == transactionsFocusFromDate {
					m.transactionsFromDate = backspaceDateDigit(m.transactionsFromDate)
				} else {
					m.transactionsToDate = backspaceDateDigit(m.transactionsToDate)
				}
				m.transactionsFilterMode = transactionsFilterModeCustom
				m.transactionsDateErr = ""
				return m, nil
			}
		}

		if m.screen == screenTransactions {
			if m.transactionsSearchActive {
				switch msg.String() {
				case "enter":
					searchInput := strings.TrimSpace(m.transactionsSearchInput.Value())
					appliedSearch := strings.TrimSpace(m.transactionsSearchApplied)
					if isTransactionsSearchResetQuery(searchInput) {
						m.transactionsSearchInput.SetValue("")
						m.transactionsSearchApplied = ""
						m.transactionsSearchErr = ""
						m.transactionsSearchActive = false
						m.transactionsSearchInput.Blur()
						m.transactionsPage = 0
						m.transactionsCursor = 0
						return m, m.loadTransactionsPreviewCmd()
					}
					isHelp := isTransactionsSearchHelpQuery(searchInput)
					if searchInput != appliedSearch {
						if !isHelp {
							if err := validateTransactionsSearchSyntax(searchInput); err != nil {
								m.transactionsSearchErr = "invalid search syntax, type /help for info"
								return m, nil
							}
						}
						m.transactionsSearchApplied = searchInput
						m.transactionsSearchErr = ""
						if isHelp {
							// Keep focus in search mode while help is displayed.
							m.transactionsSearchActive = true
							m.transactionsSearchInput.Focus()
							return m, m.loadTransactionsPreviewCmd()
						}
						m.transactionsSearchActive = false
						m.transactionsSearchInput.Blur()
						m.transactionsPage = 0
						m.transactionsCursor = 0
						return m, m.loadTransactionsPreviewCmd()
					}
					if isHelp {
						// Enter should not leave search mode while help instructions are active.
						m.transactionsSearchActive = true
						m.transactionsSearchInput.Focus()
						return m, nil
					}
					m.transactionsSearchErr = ""
					m.transactionsSearchActive = false
					m.transactionsSearchInput.Blur()
					return m, nil
				case "esc":
					if isTransactionsSearchHelpQuery(m.transactionsSearchApplied) {
						m.transactionsSearchInput.SetValue("")
						m.transactionsSearchApplied = ""
						m.transactionsSearchErr = ""
						m.transactionsSearchActive = false
						m.transactionsSearchInput.Blur()
						m.transactionsPage = 0
						m.transactionsCursor = 0
						return m, m.loadTransactionsPreviewCmd()
					}
					m.transactionsSearchActive = false
					m.transactionsSearchInput.Blur()
					return m, nil
				default:
					var cmd tea.Cmd
					m.transactionsSearchInput, cmd = m.transactionsSearchInput.Update(msg)
					m.transactionsSearchErr = ""
					return m, cmd
				}
			}
			if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == '/' {
				m.cmd.SetValue("")
				m.clearCommandSuggestions()
				m.transactionsSearchActive = true
				if strings.TrimSpace(m.transactionsSearchInput.Value()) == "" {
					m.transactionsSearchInput.SetValue("/")
				}
				m.transactionsSearchInput.Focus()
				m.transactionsSearchInput.CursorEnd()
				m.transactionsSearchErr = ""
				return m, nil
			}
		}

		switch msg.String() {
		case "shift+up":
			if m.screen == screenAccounts &&
				(!m.accountsPaneOpen || m.accountsPaneFocus == accountsFocusCards) &&
				len(m.accountsRows) > 0 &&
				m.accountsCursor > 0 {
				m.accountsCursor--
				m.clampAccountsAction()
				m.ensureAccountsScrollWindow()
				id := m.accountsRows[m.accountsCursor+1].id
				return m, m.moveAccountCmd(id, -1)
			}
			return m, nil
		case "shift+down":
			if m.screen == screenAccounts &&
				(!m.accountsPaneOpen || m.accountsPaneFocus == accountsFocusCards) &&
				len(m.accountsRows) > 0 &&
				m.accountsCursor < len(m.accountsRows)-1 {
				m.accountsCursor++
				m.clampAccountsAction()
				m.ensureAccountsScrollWindow()
				id := m.accountsRows[m.accountsCursor-1].id
				return m, m.moveAccountCmd(id, +1)
			}
			return m, nil
		case "tab":
			if m.screen == screenTransactionsFilters {
				m.transactionsFocus = (m.transactionsFocus + 1) % 4
				return m, nil
			}
			if m.screen == screenAccounts && m.accountsPaneOpen {
				if m.accountsPaneFocus == accountsFocusCards {
					m.accountsPaneFocus = accountsFocusPane
				} else {
					m.accountsPaneFocus = accountsFocusCards
				}
				return m, nil
			}
			return m, nil
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if m.screen == screenTransactions && m.transactionsChartPaneOpen {
				m.transactionsChartPaneOpen = false
				m.transactionsChartPaneRows = nil
				m.transactionsChartPaneCursor = 0
				m.transactionsChartPaneOffset = 0
				m.transactionsChartPaneTitle = ""
				return m, nil
			}
			if m.screen == screenTransactionsFilters && m.transactionsCalendarOpen {
				m.transactionsCalendarOpen = false
				return m, nil
			}
			if m.screen == screenTransactionsFilters {
				m.screen = screenTransactions
				return m, nil
			}
			if m.screen == screenAccounts && m.accountsPaneOpen {
				m.accountsPaneOpen = false
				m.accountsPaneFocus = accountsFocusCards
				return m, nil
			}
			if m.screen == screenTransactions &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				if m.transactionsPaneOpen {
					m.transactionsPaneOpen = false
					return m, nil
				}
				m.screen = screenHome
				m.transactionsSession++
				m.transactionsSyncing = false
				return m, nil
			}
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
			if m.screen == screenTransactions {
				if m.transactionsViewMode == transactionsViewModeChart {
					if m.transactionsChartPaneOpen {
						if m.transactionsChartPaneCursor > 0 {
							m.transactionsChartPaneCursor--
							m.ensureTransactionsChartPaneScrollWindow()
						}
						return m, nil
					}
					if m.transactionsChartCursor > 0 {
						m.transactionsChartCursor--
						m.ensureTransactionsChartScrollWindow()
					}
					return m, nil
				}
				if m.transactionsViewMode != transactionsViewModeTable {
					return m, nil
				}
				if m.transactionsCursor > 0 {
					m.transactionsCursor--
					m.ensureTransactionsScrollWindow()
					return m, nil
				}
				if m.transactionsPage > 0 {
					m.transactionsPage--
					if m.transactionsPageSize > 0 {
						m.transactionsCursor = m.transactionsPageSize - 1
					} else {
						m.transactionsCursor = 0
					}
					return m, m.loadTransactionsPreviewCmd()
				}
				return m, nil
			}
			if m.screen == screenAccounts {
				if m.accountsPaneOpen && m.accountsPaneFocus == accountsFocusPane {
					if m.accountsAction > 0 {
						m.accountsAction--
					}
					return m, nil
				}
				if m.accountsCursor > 0 {
					m.accountsCursor--
				}
				m.clampAccountsAction()
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
			if m.screen == screenTransactions {
				if m.transactionsViewMode == transactionsViewModeChart {
					if m.transactionsChartPaneOpen {
						if m.transactionsChartPaneCursor < len(m.transactionsChartPaneRows)-1 {
							m.transactionsChartPaneCursor++
							m.ensureTransactionsChartPaneScrollWindow()
						}
						return m, nil
					}
					if m.transactionsChartCursor < len(m.transactionsCategorySpend)-1 {
						m.transactionsChartCursor++
						m.ensureTransactionsChartScrollWindow()
					}
					return m, nil
				}
				if m.transactionsViewMode != transactionsViewModeTable {
					return m, nil
				}
				if m.transactionsCursor < len(m.transactionsRows)-1 {
					m.transactionsCursor++
					m.ensureTransactionsScrollWindow()
					return m, nil
				}
				maxPage := 0
				if m.transactionsPageSize > 0 && m.transactionsTotal > 0 {
					maxPage = (m.transactionsTotal - 1) / m.transactionsPageSize
				}
				if m.transactionsPage < maxPage {
					m.transactionsPage++
					m.transactionsCursor = 0
					return m, m.loadTransactionsPreviewCmd()
				}
				return m, nil
			}
			if m.screen == screenAccounts {
				if m.accountsPaneOpen && m.accountsPaneFocus == accountsFocusPane {
					if m.accountsAction < len(m.currentAccountActionItems())-1 {
						m.accountsAction++
					}
					return m, nil
				}
				if m.accountsCursor < len(m.accountsRows)-1 {
					m.accountsCursor++
				}
				m.clampAccountsAction()
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
		case "left":
			if m.screen == screenTransactionsFilters &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				if m.transactionsFocus == transactionsFocusQuickRange {
					ranges := transactionsQuickRanges()
					m.transactionsQuickIdx = (m.transactionsQuickIdx - 1 + len(ranges)) % len(ranges)
					return m, nil
				}
				if m.transactionsFocus == transactionsFocusIncludeInternal {
					m.transactionsIncludeInternal = false
					return m, nil
				}
			}
			if m.screen == screenTransactions &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() &&
				m.transactionsViewMode == transactionsViewModeTable &&
				m.transactionsPage > 0 {
				m.transactionsPage--
				m.transactionsCursor = 0
				return m, m.loadTransactionsPreviewCmd()
			}
		case "right":
			if m.screen == screenTransactionsFilters &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				if m.transactionsFocus == transactionsFocusQuickRange {
					ranges := transactionsQuickRanges()
					m.transactionsQuickIdx = (m.transactionsQuickIdx + 1) % len(ranges)
					return m, nil
				}
				if m.transactionsFocus == transactionsFocusIncludeInternal {
					m.transactionsIncludeInternal = true
					return m, nil
				}
			}
			if m.screen == screenTransactions &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() &&
				m.transactionsViewMode == transactionsViewModeTable {
				maxPage := 0
				if m.transactionsPageSize > 0 && m.transactionsTotal > 0 {
					maxPage = (m.transactionsTotal - 1) / m.transactionsPageSize
				}
				if m.transactionsPage < maxPage {
					m.transactionsPage++
					m.transactionsCursor = 0
					return m, m.loadTransactionsPreviewCmd()
				}
			}
		case "f":
			if m.screen == screenTransactions &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				m.screen = screenTransactionsFilters
				m.transactionsFocus = transactionsFocusFromDate
				return m, nil
			}
		case "c":
			if m.screen == screenTransactionsFilters &&
				(m.transactionsFocus == transactionsFocusFromDate || m.transactionsFocus == transactionsFocusToDate) &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				m.transactionsCalendarTarget = m.transactionsFocus
				selected := time.Now().In(time.Local)
				digits := m.transactionsFromDate
				if m.transactionsCalendarTarget == transactionsFocusToDate {
					digits = m.transactionsToDate
				}
				if strings.TrimSpace(digits) != "" {
					if t, ok := calendarAnchorFromPartial(digits); ok {
						selected = t
					}
				}
				m.transactionsCalendarCursor = time.Date(selected.Year(), selected.Month(), selected.Day(), 0, 0, 0, 0, time.Local)
				m.transactionsCalendarMonth = time.Date(selected.Year(), selected.Month(), 1, 0, 0, 0, 0, time.Local)
				m.transactionsCalendarOpen = true
				return m, nil
			}
		case "s":
			if m.screen == screenTransactions &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() &&
				m.transactionsViewMode == transactionsViewModeTable {
				sorts := transactionsSortOptions()
				m.transactionsSortIdx = (m.transactionsSortIdx + 1) % len(sorts)
				m.transactionsPage = 0
				return m, m.loadTransactionsPreviewCmd()
			}
		case "1":
			if m.screen == screenTransactions &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				m.transactionsViewMode = transactionsViewModeTable
				return m, nil
			}
		case "2":
			if m.screen == screenTransactions &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				m.transactionsViewMode = transactionsViewModeChart
				m.transactionsPaneOpen = false
				m.transactionsChartPaneOpen = false
				m.transactionsChartPaneRows = nil
				m.transactionsChartPaneCursor = 0
				m.transactionsChartPaneOffset = 0
				m.transactionsChartPaneTitle = ""
				return m, nil
			}
		case "3":
			if m.screen == screenTransactions &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				return m, nil
			}
		case "enter":
			if m.screen == screenTransactions &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				if m.transactionsViewMode == transactionsViewModeChart {
					if len(m.transactionsCategorySpend) == 0 || m.transactionsChartCursor < 0 || m.transactionsChartCursor >= len(m.transactionsCategorySpend) {
						return m, nil
					}
					category := m.transactionsCategorySpend[m.transactionsChartCursor].category
					return m, m.loadCategoryTransactionsCmd(category)
				}
				if m.transactionsViewMode != transactionsViewModeTable {
					return m, nil
				}
				if len(m.transactionsRows) == 0 {
					return m, nil
				}
				m.transactionsPaneOpen = !m.transactionsPaneOpen
				return m, nil
			}
			if m.screen == screenTransactionsFilters &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				switch m.transactionsFocus {
				case transactionsFocusQuickRange:
					m.applyTransactionsQuickRange(m.transactionsQuickIdx)
					m.transactionsFilterMode = transactionsFilterModeQuick
					m.transactionsPage = 0
					m.transactionsDateErr = ""
					return m, tea.Batch(m.saveTransactionsFiltersCmd(), m.loadTransactionsPreviewCmd())
				case transactionsFocusFromDate, transactionsFocusToDate:
					if err := validateTransactionsDateRange(m.transactionsFromDate, m.transactionsToDate); err != nil {
						m.transactionsDateErr = err.Error()
						return m, nil
					}
					m.transactionsFilterMode = transactionsFilterModeCustom
					m.transactionsDateErr = ""
					m.transactionsPage = 0
					return m, tea.Batch(m.saveTransactionsFiltersCmd(), m.loadTransactionsPreviewCmd())
				case transactionsFocusIncludeInternal:
					return m, tea.Batch(m.saveTransactionsFiltersCmd(), m.loadTransactionsPreviewCmd())
				}
			}
			if m.screen == screenAccounts &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				if len(m.accountsRows) == 0 {
					return m, nil
				}
				if !m.accountsPaneOpen {
					m.accountsPaneOpen = true
					m.accountsPaneFocus = accountsFocusPane
					m.accountsAction = 0
					return m, nil
				}
				if m.accountsPaneFocus == accountsFocusCards {
					m.accountsPaneFocus = accountsFocusPane
					return m, nil
				}
				actions := m.currentAccountActionItems()
				if len(actions) == 0 {
					return m, nil
				}
				selectedAction := actions[m.accountsAction]
				if selectedAction == "enter goal balance" {
					m.accountsGoalEditing = true
					m.accountsGoalErr = ""
					m.accountsGoalInput.SetValue("")
					m.accountsGoalInput.Focus()
					return m, nil
				}
				return m.withCommandFeedback(fmt.Sprintf("%s: coming soon", selectedAction))
			}
			if m.screen == screenHome &&
				strings.TrimSpace(m.cmd.Value()) == "" &&
				!m.shouldShowCommandSuggestions() {
				switch m.viewItems[m.selected] {
				case "config":
					return m.enterConfigView()
				case "accounts":
					return m.enterAccountsView()
				case "transactions":
					return m.enterTransactionsView()
				default:
					return m, nil
				}
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
	if m.screen == screenTransactions && !m.transactionsSearchActive {
		return m, nil
	}
	if m.screen == screenTransactionsFilters {
		return m, nil
	}
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
	if m.screen == screenConfig {
		content := contentStyle.Render(m.renderConfigScreen(layoutWidth))
		layoutHeight := max(1, m.height-frame.GetVerticalFrameSize()-contentStyle.GetVerticalFrameSize())
		if m.showHelpOverlay {
			helpOverlay := renderHelpOverlay(layoutWidth)
			centered := lipgloss.Place(layoutWidth, layoutHeight, lipgloss.Center, lipgloss.Center, helpOverlay)
			return frame.Render(contentStyle.Render(centered))
		}
		if m.authDialog != authDialogNone {
			authOverlay := m.renderAuthDialog(layoutWidth)
			centered := lipgloss.Place(layoutWidth, layoutHeight, lipgloss.Center, lipgloss.Center, authOverlay)
			return frame.Render(contentStyle.Render(centered))
		}
		return frame.Render(content)
	}
	if m.screen == screenTransactions {
		content := contentStyle.Render(m.renderTransactionsScreen(layoutWidth))
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
	if m.screen == screenTransactionsFilters {
		content := contentStyle.Render(m.renderTransactionsFiltersScreen(layoutWidth))
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

	statusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true).Render("status: ")
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
	case "/config":
		return m.enterConfigView()
	case "/accounts":
		return m.enterAccountsView()
	case "/transactions":
		return m.enterTransactionsView()
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
	m.selected = 1
	m.screen = screenAccounts
	m.accountsErr = ""
	m.accountsLoading = true
	m.accountsPaneOpen = false
	m.accountsPaneFocus = accountsFocusCards
	m.accountsAction = 0
	m.accountsGoalEditing = false
	m.accountsGoalErr = ""
	m.accountsGoalInput.SetValue("")
	m.accountsGoalInput.Blur()
	m.accountsSession++
	return m, tea.Batch(
		m.loadAccountsPreviewCmd(),
		m.syncAndReloadAccountsPreviewCmd(false),
		m.accountsClockTickCmd(),
		m.accountsAutoRefreshTickCmd(),
	)
}

func (m model) enterTransactionsView() (tea.Model, tea.Cmd) {
	m.selected = 2
	m.screen = screenTransactions
	m.transactionsErr = ""
	m.transactionsDateErr = ""
	m.transactionsFocus = transactionsFocusFromDate
	m.transactionsPaneOpen = false
	m.transactionsSearchErr = ""
	m.transactionsSearchActive = false
	m.transactionsSearchInput.Blur()
	m.transactionsSearchInput.SetValue("")
	m.transactionsSearchApplied = ""
	m.transactionsChartCursor = 0
	m.transactionsChartOffset = 0
	m.transactionsChartPaneOpen = false
	m.transactionsChartPaneRows = nil
	m.transactionsChartPaneCursor = 0
	m.transactionsChartPaneOffset = 0
	m.transactionsChartPaneTitle = ""
	m.cmd.SetValue("")
	m.clearCommandSuggestions()
	m.transactionsCursor = 0
	m.transactionsOffset = 0
	m.transactionsPage = 0
	if m.transactionsPageSize <= 0 {
		m.transactionsPageSize = 8
	}
	if m.transactionsFromDate == "" && m.transactionsToDate == "" {
		m.transactionsQuickIdx = 2 // last 3 months
		m.applyTransactionsQuickRange(m.transactionsQuickIdx)
		m.transactionsFilterMode = transactionsFilterModeQuick
	} else {
		m.transactionsFilterMode = transactionsFilterModeCustom
	}
	m.transactionsSession++
	m.transactionsSyncing = false
	next, syncCmd := m.maybeStartTransactionsSyncCmd(false)
	return next, tea.Batch(
		next.loadTransactionsFiltersCmd(),
		syncCmd,
		next.transactionsReloadTickCmd(),
		next.transactionsClockTickCmd(),
		next.transactionsAutoRefreshTickCmd(),
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

func (m *model) ensureTransactionsScrollWindow() {
	visible := m.transactionsVisibleRows()
	if visible < 1 {
		visible = 1
	}
	if m.transactionsCursor < m.transactionsOffset {
		m.transactionsOffset = m.transactionsCursor
	}
	if m.transactionsCursor >= m.transactionsOffset+visible {
		m.transactionsOffset = m.transactionsCursor - visible + 1
	}
	maxOffset := max(0, len(m.transactionsRows)-visible)
	if m.transactionsOffset > maxOffset {
		m.transactionsOffset = maxOffset
	}
	if m.transactionsOffset < 0 {
		m.transactionsOffset = 0
	}
}

func (m *model) ensureTransactionsChartScrollWindow() {
	visible := m.transactionsChartVisibleRows()
	if visible < 1 {
		visible = 1
	}
	if m.transactionsChartCursor < m.transactionsChartOffset {
		m.transactionsChartOffset = m.transactionsChartCursor
	}
	if m.transactionsChartCursor >= m.transactionsChartOffset+visible {
		m.transactionsChartOffset = m.transactionsChartCursor - visible + 1
	}
	maxOffset := max(0, len(m.transactionsCategorySpend)-visible)
	if m.transactionsChartOffset > maxOffset {
		m.transactionsChartOffset = maxOffset
	}
	if m.transactionsChartOffset < 0 {
		m.transactionsChartOffset = 0
	}
}

func (m *model) ensureTransactionsChartPaneScrollWindow() {
	visible := m.transactionsChartPaneVisibleRows()
	if visible < 1 {
		visible = 1
	}
	if m.transactionsChartPaneCursor < m.transactionsChartPaneOffset {
		m.transactionsChartPaneOffset = m.transactionsChartPaneCursor
	}
	if m.transactionsChartPaneCursor >= m.transactionsChartPaneOffset+visible {
		m.transactionsChartPaneOffset = m.transactionsChartPaneCursor - visible + 1
	}
	maxOffset := max(0, len(m.transactionsChartPaneRows)-visible)
	if m.transactionsChartPaneOffset > maxOffset {
		m.transactionsChartPaneOffset = maxOffset
	}
	if m.transactionsChartPaneOffset < 0 {
		m.transactionsChartPaneOffset = 0
	}
}

func (m model) transactionsVisibleRows() int {
	return 12
}

func (m model) transactionsChartVisibleRows() int {
	return 15
}

func (m model) transactionsChartPaneVisibleRows() int {
	return 16
}

func (m model) maybeStartTransactionsSyncCmd(force bool) (model, tea.Cmd) {
	if m.transactionsSyncing {
		return m, nil
	}
	if !force && m.transactionsLastSync != nil && time.Since(m.transactionsLastSync.UTC()) < 15*time.Second {
		return m, nil
	}
	m.transactionsSyncing = true
	session := m.transactionsSession
	return m, m.syncTransactionsCmd(session, force)
}

func (m model) transactionsReloadTickCmd() tea.Cmd {
	session := m.transactionsSession
	return tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg {
		return transactionsReloadTickMsg{sessionID: session}
	})
}

func (m model) transactionsClockTickCmd() tea.Cmd {
	session := m.transactionsSession
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return transactionsClockTickMsg{sessionID: session}
	})
}

func (m model) transactionsAutoRefreshTickCmd() tea.Cmd {
	session := m.transactionsSession
	return tea.Tick(2*time.Minute, func(time.Time) tea.Msg {
		return transactionsAutoRefreshTickMsg{sessionID: session}
	})
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
	return tea.Tick(2*time.Minute, func(time.Time) tea.Msg {
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

func (m model) accountsActionItems() []string {
	return []string{
		"enter goal balance",
		"burndown chart",
	}
}

func (m model) currentAccountActionItems() []string {
	items := m.accountsActionItems()
	if len(m.accountsRows) == 0 || m.accountsCursor < 0 || m.accountsCursor >= len(m.accountsRows) {
		return items
	}
	if m.accountsRows[m.accountsCursor].accountType == "TRANSACTIONAL" {
		return []string{"burndown chart"}
	}
	return items
}

func (m *model) clampAccountsAction() {
	items := m.currentAccountActionItems()
	if len(items) == 0 {
		m.accountsAction = 0
		return
	}
	if m.accountsAction < 0 {
		m.accountsAction = 0
		return
	}
	if m.accountsAction >= len(items) {
		m.accountsAction = len(items) - 1
	}
}

func (m model) saveAccountGoalCmd(accountID, goalBalance string) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return saveAccountGoalMsg{err: fmt.Errorf("database is not initialized")}
		}
		if err := saveAccountGoalBalance(context.Background(), m.db, accountID, goalBalance); err != nil {
			return saveAccountGoalMsg{err: err}
		}
		return saveAccountGoalMsg{}
	}
}

func normalizeGoalInput(raw string) string {
	var b strings.Builder
	hasDot := false
	decimals := 0
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			if hasDot {
				if decimals >= 2 {
					continue
				}
				decimals++
			}
			b.WriteRune(ch)
			continue
		}
		if ch == '.' && !hasDot {
			hasDot = true
			b.WriteRune(ch)
		}
	}
	return b.String()
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

func (m model) transactionsPrewarmCheckCmd() tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return transactionsPrewarmCheckMsg{err: fmt.Errorf("database is not initialized")}
		}
		var count int
		if err := m.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM transactions").Scan(&count); err != nil {
			return transactionsPrewarmCheckMsg{err: err}
		}
		return transactionsPrewarmCheckMsg{empty: count == 0}
	}
}

func validateTransactionsSearchSyntax(query string) error {
	where := []string{}
	args := []any{}
	return appendTransactionsSearchClauses(strings.TrimSpace(query), &where, &args)
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
	itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Underline(true)
	prefixStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F47A60")).Bold(true)
	for i, item := range items {
		prefix := "  "
		style := itemStyle
		if i == selected {
			prefix = "> "
			style = selectedStyle
		}
		if i == selected {
			lines = append(lines, prefixStyle.Render("> ")+style.Render(item))
			continue
		}
		lines = append(lines, style.Render(prefix+item))
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
		{name: "/config", description: "open app config"},
		{name: "/accounts", description: "select the accounts view"},
		{name: "/transactions", description: "select the transactions view"},
		{name: "/ping", description: "check Up API connectivity"},
		{name: "/disconnect", description: "remove saved PAT from keychain"},
		{name: "/db-wipe", description: "wipe and reinitialize the local database"},
		{name: "/connect", description: "open the PAT connect prompt"},
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
	searchHelp := []string{
		"",
		"transactions search:",
		"merchant: WOO + amount: >60 + category: groceries",
		"type: +ve or type: -ve",
	}
	body := strings.Join(append(commands, searchHelp...), "\n")
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
