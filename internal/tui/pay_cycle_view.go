package tui

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lachiem1/giddyUp/internal/storage"
)

const (
	payCycleCellEmpty = iota
	payCycleCellAxis
	payCycleCellIdeal
	payCycleCellActual
	payCycleCellFutureActual
	payCycleCellToday
	payCycleCellNode
	payCycleCellNodeSelected
)

func renderPayCycleBurndownTitle() string {
	glyphs := map[rune][3]string{
		'A': {"▄▀█", "█▀█", "▀ ▀"},
		'B': {"█▀█", "█▀█", "▀▀▀"},
		'C': {"█▀▀", "█▄▄", "▀▀▀"},
		'D': {"█▀▄", "█ █", "▀▀ "},
		'E': {"█▀▀", "█▀▀", "▀▀▀"},
		'L': {"█  ", "█▄▄", "▀▀▀"},
		'N': {"█▄ █", "█ ▀█", "▀  ▀"},
		'O': {"█▀█", "█▄█", "▀▀▀"},
		'P': {"█▀█", "█▀▀", "▀  "},
		'R': {"█▀█", "█▀▄", "▀ ▀"},
		'U': {"█ █", "█▄█", "▀▀▀"},
		'W': {"█ █ █", "█ █ █", "▀▀▀▀▀"},
		'Y': {"█ █", " █ ", " ▀ "},
		' ': {" ", " ", " "},
	}
	title := "PAY CYCLE BURNDOWN"
	lines := [3][]string{{}, {}, {}}
	for _, ch := range title {
		g, ok := glyphs[ch]
		if !ok {
			continue
		}
		lines[0] = append(lines[0], g[0])
		lines[1] = append(lines[1], g[1])
		lines[2] = append(lines[2], g[2])
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true)
	out := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		out = append(out, style.Render(strings.Join(lines[i], " ")))
	}
	return strings.Join(out, "\n")
}

func (m model) enterPayCycleBurndownView() (tea.Model, tea.Cmd) {
	m.selected = 3
	m.screen = screenPayCycleBurndown
	m.payCycleErr = ""
	m.payCyclePromptErr = ""
	m.payCyclePromptMode = payCyclePromptNone
	m.payCycleSeries = nil
	m.payCycleTransactions = nil
	m.payCycleTxCursor = 0
	m.payCycleCurrentBalanceCents = 0
	m.payCycleGoalCents = 0
	m.payCycleStartDate = ""
	m.payCycleEndDate = ""
	m.payCycleInput.SetValue("")
	m.payCycleInput.Placeholder = ""
	m.payCycleInput.Blur()
	m.payCyclePaneOpen = false
	m.payCyclePaneFocus = payCyclePaneFocusMain
	m.payCycleConfigReturn = false
	m.payCyclePromptGoalAfterConfig = false
	m.cmd.Blur()
	next, syncCmd := m.maybeStartTransactionsSyncCmd(false)
	accountsSyncCmd := next.syncAndReloadAccountsPreviewCmd(false)
	if syncCmd != nil {
		return next, tea.Batch(
			next.loadPayCycleStateCmd(),
			accountsSyncCmd,
			syncCmd,
			next.transactionsReloadTickCmd(),
			next.transactionsClockTickCmd(),
			next.transactionsAutoRefreshTickCmd(),
		)
	}
	return next, tea.Batch(
		next.loadPayCycleStateCmd(),
		accountsSyncCmd,
		next.transactionsClockTickCmd(),
		next.transactionsAutoRefreshTickCmd(),
	)
}

func (m model) loadPayCycleStateCmd() tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return loadPayCycleStateMsg{err: fmt.Errorf("database is not initialized")}
		}
		accounts, nextPayDate, frequency, err := queryPayCycleState(context.Background(), m.db)
		if err != nil {
			return loadPayCycleStateMsg{err: err}
		}
		return loadPayCycleStateMsg{
			accounts:    accounts,
			nextPayDate: nextPayDate,
			frequency:   frequency,
		}
	}
}

func queryPayCycleState(ctx context.Context, db *sql.DB) ([]payCycleAccountRow, string, string, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT
			id,
			display_name,
			account_type,
			balance_value_in_base_units,
			COALESCE(goal_balance, '')
		 FROM accounts
		 WHERE is_active = 1
		   AND UPPER(account_type) != 'TRANSACTIONAL'
		 ORDER BY display_order ASC, display_name ASC, id ASC`,
	)
	if err != nil {
		return nil, "", "", err
	}
	defer rows.Close()

	out := make([]payCycleAccountRow, 0, 8)
	for rows.Next() {
		var r payCycleAccountRow
		if err := rows.Scan(
			&r.id,
			&r.displayName,
			&r.accountType,
			&r.balanceCents,
			&r.goalBalance,
		); err != nil {
			return nil, "", "", err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", "", err
	}

	repo := storage.NewAppConfigRepo(db)
	nextPayDate, _, err := repo.Get(ctx, "pay_cycle.next_date")
	if err != nil {
		return nil, "", "", err
	}
	frequency, _, err := repo.Get(ctx, "pay_cycle.frequency")
	if err != nil {
		return nil, "", "", err
	}
	return out, strings.TrimSpace(nextPayDate), strings.TrimSpace(frequency), nil
}

func (m model) savePayCycleGoalCmd(accountID, goalBalance string) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return savePayCycleGoalMsg{err: fmt.Errorf("database is not initialized")}
		}
		if err := saveAccountGoalBalance(context.Background(), m.db, accountID, goalBalance); err != nil {
			return savePayCycleGoalMsg{err: err}
		}
		return savePayCycleGoalMsg{}
	}
}

func (m model) savePayCycleConfigValueCmd(values map[string]string) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return savePayCycleConfigMsg{err: fmt.Errorf("database is not initialized")}
		}
		repo := storage.NewAppConfigRepo(m.db)
		if err := repo.UpsertMany(context.Background(), values); err != nil {
			return savePayCycleConfigMsg{err: err}
		}
		return savePayCycleConfigMsg{}
	}
}

func normalizePayCycleFrequency(raw string) (string, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	for _, opt := range configFrequencyOptions() {
		if trimmed == opt {
			return opt, true
		}
	}
	return "", false
}

func parsePayCycleDate(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("next pay date is required")
	}
	t, err := time.ParseInLocation("2006-01-02", trimmed, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("next pay date must be YYYY-MM-DD")
	}
	return t, nil
}

func computePayCycleWindow(nextDate, frequency string) (time.Time, time.Time, error) {
	nextPayDate, err := parsePayCycleDate(nextDate)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	freq, ok := normalizePayCycleFrequency(frequency)
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("pay cycle frequency is required")
	}

	lastPayDate := nextPayDate
	switch freq {
	case "weekly":
		lastPayDate = nextPayDate.AddDate(0, 0, -7)
	case "fortnightly":
		lastPayDate = nextPayDate.AddDate(0, 0, -14)
	case "monthly":
		lastPayDate = nextPayDate.AddDate(0, -1, 0)
	case "quarterly":
		lastPayDate = nextPayDate.AddDate(0, -3, 0)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported pay cycle frequency")
	}
	return lastPayDate, nextPayDate, nil
}

func parseGoalBalanceCents(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("goal balance is required")
	}
	n, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("goal balance is invalid")
	}
	if n <= 0 {
		return 0, fmt.Errorf("goal balance must be greater than 0")
	}
	return int64(math.Round(n * 100)), nil
}

func digitsOnly(raw string) string {
	var b strings.Builder
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func limitDigits(raw string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(raw) <= maxLen {
		return raw
	}
	return raw[:maxLen]
}

func (m *model) clampPayCycleCursor() {
	if len(m.payCycleAccounts) == 0 {
		m.payCycleCursor = 0
		return
	}
	if m.payCycleCursor < 0 {
		m.payCycleCursor = 0
	}
	if m.payCycleCursor >= len(m.payCycleAccounts) {
		m.payCycleCursor = len(m.payCycleAccounts) - 1
	}
}

func (m model) payCycleSelectedAccount() (payCycleAccountRow, bool) {
	if len(m.payCycleAccounts) == 0 {
		return payCycleAccountRow{}, false
	}
	if m.payCycleCursor < 0 || m.payCycleCursor >= len(m.payCycleAccounts) {
		return payCycleAccountRow{}, false
	}
	return m.payCycleAccounts[m.payCycleCursor], true
}

func (m model) loadPayCycleSeriesCmd() tea.Cmd {
	account, ok := m.payCycleSelectedAccount()
	if !ok {
		return nil
	}
	goalCents, err := parseGoalBalanceCents(account.goalBalance)
	if err != nil {
		return nil
	}
	currentBalanceCents := account.balanceCents
	startDate, endDate, err := computePayCycleWindow(m.payCycleNextDate, m.payCycleFrequency)
	if err != nil {
		return nil
	}
	accountID := account.id
	startDateStr := startDate.Format("2006-01-02")
	endDateStr := endDate.Format("2006-01-02")
	return func() tea.Msg {
		if m.db == nil {
			return loadPayCycleSeriesMsg{err: fmt.Errorf("database is not initialized")}
		}
		points, transactions, err := queryPayCycleBurndownSeries(
			context.Background(),
			m.db,
			accountID,
			startDate,
			endDate,
			currentBalanceCents,
			goalCents,
		)
		return loadPayCycleSeriesMsg{
			accountID:           accountID,
			startDate:           startDateStr,
			endDate:             endDateStr,
			goalCents:           goalCents,
			currentBalanceCents: currentBalanceCents,
			points:              points,
			transactions:        transactions,
			err:                 err,
		}
	}
}

func queryPayCycleBurndownSeries(
	ctx context.Context,
	db *sql.DB,
	accountID string,
	startDate time.Time,
	endDate time.Time,
	currentBalanceCents int64,
	goalCents int64,
) ([]payCycleBurndownPoint, []payCycleTransactionRow, error) {
	if strings.TrimSpace(accountID) == "" {
		return nil, nil, fmt.Errorf("account id is required")
	}
	if endDate.Before(startDate) {
		return nil, nil, fmt.Errorf("next pay date must be after last pay date")
	}

	startDateStr := startDate.Format("2006-01-02")
	endDateStr := endDate.Format("2006-01-02")
	rows, err := db.QueryContext(
		ctx,
		`SELECT
			t.id,
			t.created_at,
			COALESCE(
				NULLIF(t.merchant_norm, ''),
				COALESCE(
					NULLIF(t.raw_text_norm, ''),
					NULLIF(t.description_norm, ''),
					COALESCE(t.raw_text, t.description, '')
				)
			) AS merchant,
			COALESCE(NULLIF(t.raw_text_norm, ''), COALESCE(t.raw_text, '')) AS raw_text,
			COALESCE(NULLIF(t.description_norm, ''), COALESCE(t.description, '')) AS description,
			t.amount_value,
			COALESCE(-t.amount_value_in_base_units, 0) AS spend_cents,
			COALESCE(t.status, ''),
			COALESCE(t.message, ''),
			COALESCE(t.category_id, ''),
			COALESCE(t.card_purchase_method_method, ''),
			COALESCE(t.note_text, ''),
			COALESCE(a.display_name, '')
		 FROM transactions t
		 LEFT JOIN accounts a ON a.id = t.account_id
		 WHERE t.is_active = 1
		   AND t.account_id = ?
		   AND t.amount_value_in_base_units != 0
		   AND date(t.created_at) >= date(?)
		   AND date(t.created_at) <= date(?)
		 ORDER BY t.created_at ASC, t.id ASC`,
		accountID,
		startDateStr,
		endDateStr,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	allRows := make([]payCycleTransactionRow, 0, 64)
	for rows.Next() {
		var row payCycleTransactionRow
		var spend sql.NullInt64
		if err := rows.Scan(
			&row.id,
			&row.createdAt,
			&row.merchant,
			&row.rawText,
			&row.description,
			&row.amountValue,
			&spend,
			&row.status,
			&row.message,
			&row.categoryID,
			&row.cardMethod,
			&row.noteText,
			&row.accountName,
		); err != nil {
			return nil, nil, err
		}
		if spend.Valid {
			row.spendCents = spend.Int64
		}
		allRows = append(allRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	fundingIdx := -1
	if goalCents > 0 {
		for i := range allRows {
			// spendCents is negative for inflows; funding is a +ve transaction >= goal.
			if allRows[i].spendCents <= -goalCents {
				fundingIdx = i
				break
			}
		}
	}

	transactions := make([]payCycleTransactionRow, 0, len(allRows))
	totalSpendCents := int64(0)
	for i := range allRows {
		if fundingIdx >= 0 && i < fundingIdx {
			continue
		}
		// Ignore likely funding spikes (e.g. salary/seed top-ups) that exceed the goal balance.
		if goalCents > 0 && absInt64(allRows[i].spendCents) >= goalCents {
			continue
		}
		totalSpendCents += allRows[i].spendCents
		transactions = append(transactions, allRows[i])
	}

	startBalanceCents := currentBalanceCents + totalSpendCents
	points := make([]payCycleBurndownPoint, 0, len(transactions)+2)
	points = append(points, payCycleBurndownPoint{
		date:           startDateStr,
		createdAt:      startDate.Format("2006-01-02T00:00:00"),
		remainingCents: startBalanceCents,
		hasTransaction: false,
	})

	remaining := startBalanceCents
	for i := range transactions {
		remaining -= transactions[i].spendCents
		t := strings.TrimSpace(transactions[i].createdAt)
		datePart := formatTransactionDate(t)
		points = append(points, payCycleBurndownPoint{
			date:           datePart,
			createdAt:      t,
			remainingCents: remaining,
			hasTransaction: true,
			transactionID:  transactions[i].id,
		})
	}
	points = append(points, payCycleBurndownPoint{
		date:           endDateStr,
		createdAt:      endDate.Format("2006-01-02T23:59:59"),
		remainingCents: currentBalanceCents,
		hasTransaction: false,
	})
	return points, transactions, nil
}

func (m *model) refreshPayCyclePrompt() {
	m.clampPayCycleCursor()

	nextPayDate := strings.TrimSpace(m.payCycleNextDate)
	frequency := strings.TrimSpace(m.payCycleFrequency)
	if _, err := parsePayCycleDate(nextPayDate); err != nil {
		m.payCyclePromptMode = payCyclePromptNextDate
		m.payCycleInput.Placeholder = "YYYYMMDD"
		m.payCycleInput.SetValue(dateToDigits(nextPayDate))
		m.payCycleInput.Focus()
		return
	}
	if _, ok := normalizePayCycleFrequency(frequency); !ok {
		m.payCyclePromptMode = payCyclePromptFrequency
		m.payCycleInput.Placeholder = "weekly|fortnightly|monthly|quarterly"
		m.payCycleInput.SetValue(frequency)
		m.payCycleInput.Focus()
		return
	}
	if _, ok := m.payCycleSelectedAccount(); !ok {
		m.payCyclePromptMode = payCyclePromptNone
		m.payCycleInput.SetValue("")
		m.payCycleInput.Blur()
		return
	}
	m.payCyclePromptMode = payCyclePromptNone
	m.payCycleInput.SetValue("")
	m.payCycleInput.Blur()
}

func renderPayCyclePromptLabel(mode int, accountName string) string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true)
	switch mode {
	case payCyclePromptNextDate:
		return labelStyle.Render("Enter next pay date (YYYYMMDD):")
	case payCyclePromptFrequency:
		return labelStyle.Render("Enter pay cycle frequency: ") +
			valueStyle.Render("weekly / fortnightly / monthly / quarterly")
	case payCyclePromptGoal:
		return labelStyle.Render("Set goal balance for ") + valueStyle.Render(accountName) + labelStyle.Render(":")
	default:
		return ""
	}
}

func renderPayCycleDollars(cents int64) string {
	return formatTimeSeriesDollar(cents)
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func renderPayCycleBurndownLines(
	points []payCycleBurndownPoint,
	contentWidth int,
	goalCents int64,
	currentBalanceCents int64,
	accountColor lipgloss.Color,
	startDateRaw string,
	endDateRaw string,
	selectedTransactionID string,
) []string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	idealStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	lineStyle := lipgloss.NewStyle().Foreground(accountColor)
	todayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	nodeStyle := lipgloss.NewStyle().Foreground(accountColor).Bold(true)
	selectedNodeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD54A")).Bold(true)
	out := []string{titleStyle.Render("pay cycle burndown")}
	if len(points) == 0 {
		return append(out, labelStyle.Render("no pay cycle data - press enter to configure burndown"))
	}
	if goalCents <= 0 {
		return append(out, labelStyle.Render("goal balance required"))
	}

	innerWidth := max(16, contentWidth-2)
	plotHeight := 8
	if contentWidth >= 58 {
		plotHeight = 9
	}
	if contentWidth >= 72 {
		plotHeight = 10
	}
	yTickCount := min(5, max(3, plotHeight-1))
	yTickByRow := make(map[int]int64, yTickCount)
	for i := 0; i < yTickCount; i++ {
		row := int(math.Round(float64(i) * float64((plotHeight-1)-1) / float64(yTickCount-1)))
		ratio := float64((plotHeight-1)-row) / float64(plotHeight-1)
		yTickByRow[row] = int64(math.Round(ratio * float64(goalCents)))
	}
	yTickByRow[plotHeight-1] = 0
	yLabelWidth := 1
	for _, cents := range yTickByRow {
		w := lipgloss.Width(renderPayCycleDollars(cents))
		if w > yLabelWidth {
			yLabelWidth = w
		}
	}

	graphWidth := max(12, innerWidth-yLabelWidth-1)
	dataCols := max(10, graphWidth-1)
	graphWidth = dataCols + 1
	xAxisRow := plotHeight - 1
	startDate, endDate, hasWindow := parsePayCycleWindowDates(startDateRaw, endDateRaw)

	grid := make([][]rune, plotHeight)
	codes := make([][]int, plotHeight)
	for i := range grid {
		grid[i] = make([]rune, graphWidth)
		codes[i] = make([]int, graphWidth)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
		setPayCycleCell(grid, codes, 0, i, '|', payCycleCellAxis)
	}
	for x := 0; x < graphWidth; x++ {
		setPayCycleCell(grid, codes, x, xAxisRow, '—', payCycleCellAxis)
	}
	setPayCycleCell(grid, codes, 0, xAxisRow, '└', payCycleCellAxis)

	// Straight dotted benchmark line from top of y-axis to end of x-axis.
	drawPayCycleSegment(grid, codes, 1, 0, dataCols, xAxisRow, '·', payCycleCellIdeal, xAxisRow, -1, false)

	pointX := make([]int, len(points))
	pointY := make([]int, len(points))
	for i := range points {
		pointX[i] = payCyclePointColumn(points[i], startDate, endDate, hasWindow, dataCols)
	}
	todayCol := payCycleTodayColumn(startDate, endDate, hasWindow, dataCols)
	if todayCol > 0 {
		for y := 0; y <= xAxisRow; y++ {
			setPayCycleCell(grid, codes, todayCol, y, '·', payCycleCellToday)
		}
	}

	prevX, prevY := -1, -1
	for i, p := range points {
		ratio := float64(p.remainingCents) / float64(goalCents)
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		y := xAxisRow - int(math.Round(ratio*float64(xAxisRow)))
		y = max(0, min(plotHeight-1, y))
		if y == xAxisRow && xAxisRow > 0 {
			y = xAxisRow - 1
		}
		x := pointX[i] + 1
		pointY[i] = y
		if prevX >= 0 {
			// Render burndown as a step graph:
			// hold previous balance horizontally until the transaction time, then jump vertically.
			if x != prevX {
				skipFutureTail := todayCol > 0 && prevX <= todayCol && x > todayCol
				drawPayCycleSegment(grid, codes, prevX, prevY, x, prevY, '.', payCycleCellActual, xAxisRow, todayCol, skipFutureTail)
			}
			if y != prevY {
				skipFutureTail := false
				if todayCol > 0 && x > todayCol {
					skipFutureTail = true
				}
				drawPayCycleSegment(grid, codes, x, prevY, x, y, '.', payCycleCellActual, xAxisRow, todayCol, skipFutureTail)
			}
		}
		prevX, prevY = x, y
	}
	for i := range points {
		if !points[i].hasTransaction {
			continue
		}
		node := '●'
		cellCode := payCycleCellNode
		if todayCol > 0 && pointX[i]+1 > todayCol {
			cellCode = payCycleCellFutureActual
		}
		if strings.TrimSpace(selectedTransactionID) != "" &&
			strings.TrimSpace(points[i].transactionID) == strings.TrimSpace(selectedTransactionID) {
			node = '◉'
			cellCode = payCycleCellNodeSelected
		}
		setPayCycleCell(grid, codes, pointX[i]+1, pointY[i], node, cellCode)
	}

	for row := 0; row < plotHeight; row++ {
		axisLabel := ""
		if cents, ok := yTickByRow[row]; ok {
			axisLabel = renderPayCycleDollars(cents)
		}
		prefix := fmt.Sprintf("%*s ", yLabelWidth, axisLabel)
		graphPart := renderPayCycleGraphRow(
			grid[row],
			codes[row],
			max(1, innerWidth-lipgloss.Width(prefix)),
			labelStyle,
			idealStyle,
			lineStyle,
			todayStyle,
			nodeStyle,
			selectedNodeStyle,
		)
		out = append(out, labelStyle.Render(prefix)+graphPart)
	}

	axisPrefix := strings.Repeat(" ", yLabelWidth+1)
	tickPositions := []int{0, dataCols / 2, dataCols - 1}
	if dataCols >= 32 {
		tickPositions = []int{0, dataCols / 3, (2 * dataCols) / 3, dataCols - 1}
	}
	tickLabels := make([]string, 0, len(tickPositions))
	tickRunes := make([]rune, graphWidth)
	for i := range tickRunes {
		tickRunes[i] = ' '
	}
	for _, pos := range tickPositions {
		if pos < 0 || pos >= dataCols {
			continue
		}
		tickRunes[pos+1] = '|'
		tickLabels = append(tickLabels, payCycleTickLabel(startDate, endDate, hasWindow, pos, dataCols))
	}
	out = append(out, labelStyle.Render(truncateDisplayWidth(axisPrefix+string(tickRunes), innerWidth)))
	shiftedTicks := make([]int, 0, len(tickPositions))
	for _, pos := range tickPositions {
		shiftedTicks = append(shiftedTicks, pos+1)
	}
	out = append(out, labelStyle.Render(truncateDisplayWidth(axisPrefix+renderTimeSeriesLabelRow(graphWidth, shiftedTicks, tickLabels), innerWidth)))
	xAxisLabel := lipgloss.NewStyle().Width(graphWidth).Align(lipgloss.Center).Render("date")
	out = append(out, labelStyle.Render(truncateDisplayWidth(axisPrefix+xAxisLabel, innerWidth)))
	daysLeft := payCycleDaysLeft(endDateRaw)
	out = append(out, labelStyle.Render(
		truncateDisplayWidth(
			fmt.Sprintf(
				"goal: %s  |  remaining: %s  |  days left in cycle: %d",
				renderPayCycleDollars(goalCents),
				renderPayCycleDollars(currentBalanceCents),
				daysLeft,
			),
			innerWidth,
		),
	))
	return out
}

func payCycleDaysLeft(endDateRaw string) int {
	endDateRaw = strings.TrimSpace(endDateRaw)
	if endDateRaw == "" {
		return 0
	}
	endDate, err := time.ParseInLocation("2006-01-02", endDateRaw, time.Local)
	if err != nil {
		return 0
	}
	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if endDate.Before(today) {
		return 0
	}
	return int(endDate.Sub(today).Hours() / 24)
}

func payCycleTickLabel(startDate time.Time, endDate time.Time, hasWindow bool, pos int, colCount int) string {
	if pos < 0 {
		pos = 0
	}
	if pos >= colCount {
		pos = colCount - 1
	}
	if !hasWindow || colCount <= 1 {
		return formatPayCycleDateLabel(startDate.Format("2006-01-02"))
	}
	ratio := float64(pos) / float64(colCount-1)
	seconds := int64(math.Round(ratio * endDate.Sub(startDate).Seconds()))
	tickDate := startDate.Add(time.Duration(seconds) * time.Second)
	return tickDate.Format("02 Jan")
}

func formatPayCycleDateLabel(raw string) string {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(raw), time.Local)
	if err != nil {
		return truncateDisplayWidth(strings.TrimSpace(raw), 10)
	}
	return t.Format("02 Jan")
}

func parsePayCycleWindowDates(startRaw string, endRaw string) (time.Time, time.Time, bool) {
	start, errStart := time.ParseInLocation("2006-01-02", strings.TrimSpace(startRaw), time.Local)
	end, errEnd := time.ParseInLocation("2006-01-02", strings.TrimSpace(endRaw), time.Local)
	if errStart != nil || errEnd != nil {
		now := time.Now().In(time.Local)
		base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		return base, base.AddDate(0, 0, 1), false
	}
	// Use end-of-day for the end bound so intra-day transaction times map cleanly.
	end = end.Add(24*time.Hour - time.Second)
	if end.Before(start) {
		return start, start.AddDate(0, 0, 1), false
	}
	return start, end, true
}

func payCyclePointTime(point payCycleBurndownPoint) (time.Time, bool) {
	rawCreated := strings.TrimSpace(point.createdAt)
	if rawCreated != "" {
		if ts, err := time.Parse(time.RFC3339Nano, rawCreated); err == nil {
			return ts.In(time.Local), true
		}
		if ts, err := time.ParseInLocation("2006-01-02T15:04:05", rawCreated, time.Local); err == nil {
			return ts, true
		}
	}
	rawDate := strings.TrimSpace(point.date)
	if rawDate == "" {
		return time.Time{}, false
	}
	if d, err := time.ParseInLocation("2006-01-02", rawDate, time.Local); err == nil {
		return d, true
	}
	return time.Time{}, false
}

func payCycleTimeColumn(ts time.Time, startDate time.Time, endDate time.Time, hasWindow bool, dataCols int) int {
	if dataCols <= 1 {
		return 0
	}
	if !hasWindow || !endDate.After(startDate) {
		return 0
	}
	if ts.Before(startDate) {
		ts = startDate
	}
	if ts.After(endDate) {
		ts = endDate
	}
	totalSeconds := endDate.Sub(startDate).Seconds()
	if totalSeconds <= 0 {
		return 0
	}
	ratio := ts.Sub(startDate).Seconds() / totalSeconds
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return int(math.Round(ratio * float64(dataCols-1)))
}

func payCyclePointColumn(
	point payCycleBurndownPoint,
	startDate time.Time,
	endDate time.Time,
	hasWindow bool,
	dataCols int,
) int {
	ts, ok := payCyclePointTime(point)
	if !ok {
		return 0
	}
	return payCycleTimeColumn(ts, startDate, endDate, hasWindow, dataCols)
}

func payCycleTodayColumn(startDate time.Time, endDate time.Time, hasWindow bool, dataCols int) int {
	if !hasWindow {
		return -1
	}
	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local)
	if today.Before(startDate) || today.After(endDate) {
		return -1
	}
	return payCycleTimeColumn(today, startDate, endDate, hasWindow, dataCols) + 1
}

func renderPayCycleGraphRow(
	rowRunes []rune,
	rowCodes []int,
	maxWidth int,
	axisStyle lipgloss.Style,
	idealStyle lipgloss.Style,
	lineStyle lipgloss.Style,
	todayStyle lipgloss.Style,
	nodeStyle lipgloss.Style,
	selectedNodeStyle lipgloss.Style,
) string {
	if maxWidth <= 0 || len(rowRunes) == 0 {
		return ""
	}
	limit := min(len(rowRunes), maxWidth)
	var b strings.Builder
	for i := 0; i < limit; i++ {
		ch := string(rowRunes[i])
		switch rowCodes[i] {
		case payCycleCellAxis:
			b.WriteString(axisStyle.Render(ch))
		case payCycleCellIdeal:
			b.WriteString(idealStyle.Render(ch))
		case payCycleCellActual:
			b.WriteString(lineStyle.Render(ch))
		case payCycleCellFutureActual:
			b.WriteString(idealStyle.Render(ch))
		case payCycleCellToday:
			b.WriteString(todayStyle.Render(ch))
		case payCycleCellNode:
			b.WriteString(nodeStyle.Render(ch))
		case payCycleCellNodeSelected:
			b.WriteString(selectedNodeStyle.Render(ch))
		default:
			b.WriteString(ch)
		}
	}
	return b.String()
}

func setPayCycleCell(grid [][]rune, codes [][]int, x int, y int, ch rune, code int) {
	if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
		return
	}
	if code >= codes[y][x] {
		grid[y][x] = ch
		codes[y][x] = code
	}
}

func drawPayCycleSegment(
	grid [][]rune,
	codes [][]int,
	x0 int,
	y0 int,
	x1 int,
	y1 int,
	ch rune,
	code int,
	xAxisRow int,
	todayCol int,
	skipFutureTail bool,
) {
	dx := x1 - x0
	dy := y1 - y0
	steps := max(absInt(dx), absInt(dy))
	if steps <= 0 {
		setPayCycleCell(grid, codes, x0, y0, ch, code)
		return
	}
	for step := 0; step <= steps; step++ {
		x := x0 + int(math.Round(float64(step*dx)/float64(steps)))
		y := y0 + int(math.Round(float64(step*dy)/float64(steps)))
		if x <= 0 {
			continue
		}
		if y == xAxisRow && code == payCycleCellActual {
			continue
		}
		cellCode := code
		if code == payCycleCellActual && todayCol > 0 && x > todayCol {
			if skipFutureTail {
				continue
			}
			cellCode = payCycleCellFutureActual
		}
		setPayCycleCell(grid, codes, x, y, ch, cellCode)
	}
}

func (m model) renderPayCycleBurndownScreen(layoutWidth int) string {
	title := renderPayCycleBurndownTitle()
	title = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, title)

	paneWidth := max(30, min(40, layoutWidth/3))
	gapWidth := 3
	hasPane := m.payCyclePaneOpen && len(m.payCycleTransactions) > 0
	mainBorder := lipgloss.Color("#FFFFFF")
	paneBorder := lipgloss.Color("#FFD54A")
	if hasPane {
		if m.payCyclePaneFocus == payCyclePaneFocusMain {
			mainBorder = lipgloss.Color("#FFD54A")
			paneBorder = lipgloss.Color("#FFFFFF")
		} else {
			mainBorder = lipgloss.Color("#FFFFFF")
			paneBorder = lipgloss.Color("#FFD54A")
		}
	}

	mainCap := int(math.Round(float64(layoutWidth) * 0.78))
	maxMainWidth := min(layoutWidth-8, max(36, mainCap))
	if maxMainWidth < 20 {
		maxMainWidth = max(20, layoutWidth-8)
	}
	if hasPane {
		maxLeft := layoutWidth - paneWidth - gapWidth - 2
		maxMainWidth = min(maxMainWidth, max(36, maxLeft))
	}
	const (
		txPrefixWidth       = 2
		txDateWidth         = 10
		txGapWidth          = 2
		txAmountWidth       = 10
		txLineSlack         = 4
		txBaseMerchantWidth = 30
	)
	fixedColumnsWidth := txPrefixWidth + txDateWidth + txGapWidth + txGapWidth + txAmountWidth + txLineSlack
	baseMainContentWidth := fixedColumnsWidth + txBaseMerchantWidth
	if baseMainContentWidth > maxMainWidth {
		baseMainContentWidth = maxMainWidth
	}
	wideMainContentWidth := min(maxMainWidth, max(baseMainContentWidth, int(math.Round(float64(baseMainContentWidth)*1.5))))
	cardContentWidth := wideMainContentWidth
	if hasPane {
		totalContent := max(20, layoutWidth-gapWidth-8)
		paneWidth = int(math.Round(float64(totalContent) * 0.40))
		cardContentWidth = totalContent - paneWidth

		minPane := min(28, max(12, totalContent/3))
		minMain := min(24, max(12, totalContent/3))
		if paneWidth < minPane {
			paneWidth = minPane
			cardContentWidth = totalContent - paneWidth
		}
		if cardContentWidth < minMain {
			cardContentWidth = minMain
			paneWidth = totalContent - cardContentWidth
		}
		if paneWidth < 12 {
			paneWidth = 12
			cardContentWidth = totalContent - paneWidth
		}
		if cardContentWidth < 12 {
			cardContentWidth = 12
			paneWidth = totalContent - cardContentWidth
			if paneWidth < 8 {
				paneWidth = 8
			}
		}
	} else {
		cardContentWidth = min(cardContentWidth, maxMainWidth)
	}
	cardBodyHeight := m.transactionsChartVisibleRows() + 1

	account, hasAccount := m.payCycleSelectedAccount()
	accountColor := lipgloss.Color("#6CBFE6")
	if hasAccount {
		accountColor = transactionsCategoryColor(m.payCycleCursor)
	}

	selectedTransactionID := ""
	if len(m.payCycleTransactions) > 0 {
		txCursor := m.payCycleTxCursor
		if txCursor < 0 || txCursor >= len(m.payCycleTransactions) {
			txCursor = len(m.payCycleTransactions) - 1
		}
		selectedTransactionID = strings.TrimSpace(m.payCycleTransactions[txCursor].id)
	}
	cardLines := renderPayCycleBurndownLines(
		m.payCycleSeries,
		cardContentWidth,
		m.payCycleGoalCents,
		m.payCycleCurrentBalanceCents,
		accountColor,
		m.payCycleStartDate,
		m.payCycleEndDate,
		selectedTransactionID,
	)
	if len(m.payCycleAccounts) == 0 {
		cardLines = []string{
			lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true).Render("pay cycle burndown"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("No non-transactional accounts found."),
		}
	}
	cardLines = padTransactionsBodyLines(cardLines, cardBodyHeight)
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(mainBorder).
		Padding(0, 1).
		Height(cardBodyHeight).
		Width(cardContentWidth).
		Render(strings.Join(cardLines, "\n"))

	mainBlock := card
	if hasPane {
		txCursor := m.payCycleTxCursor
		if txCursor < 0 || txCursor >= len(m.payCycleTransactions) {
			txCursor = len(m.payCycleTransactions) - 1
		}
		selected := m.payCycleTransactions[txCursor]
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true)
		paneLines := []string{lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true).Render("transaction details")}
		valueWidth := max(10, paneWidth-16)
		paneLines = append(paneLines, renderDetailLines("amount", selected.amountValue, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("date", formatTransactionDate(selected.createdAt), valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("time", formatTransactionTime(selected.createdAt), valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("category", selected.categoryID, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("raw text", selected.rawText, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("status", selected.status, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("message", selected.message, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("description", selected.description, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("merchant", selected.merchant, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("card method", selected.cardMethod, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("note text", selected.noteText, valueWidth, labelStyle, valueStyle)...)
		paneLines = padTransactionsBodyLines(paneLines, cardBodyHeight)

		pane := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(paneBorder).
			Padding(0, 1).
			Height(cardBodyHeight).
			Width(paneWidth).
			Render(strings.Join(paneLines, "\n"))
		mainBlock = lipgloss.JoinHorizontal(lipgloss.Top, card, strings.Repeat(" ", gapWidth), pane)
	}
	mainBlock = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, mainBlock)

	metaLines := []string{}
	metaLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	metaAccountStyle := lipgloss.NewStyle().Foreground(accountColor).Bold(true)
	metaValueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true)
	if hasAccount {
		metaLines = append(metaLines, metaLabelStyle.Render("account: ")+metaAccountStyle.Render(account.displayName))
	}
	if strings.TrimSpace(m.payCycleStartDate) != "" && strings.TrimSpace(m.payCycleEndDate) != "" {
		metaLines = append(metaLines, metaLabelStyle.Render("cycle: ")+metaValueStyle.Render(m.payCycleStartDate+" to "+m.payCycleEndDate))
	}
	metaBlock := ""
	if len(metaLines) > 0 {
		aligned := make([]string, 0, len(metaLines))
		for _, line := range metaLines {
			aligned = append(aligned, lipgloss.NewStyle().Width(lipgloss.Width(mainBlock)).Align(lipgloss.Center).Render(line))
		}
		metaBlock = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, strings.Join(aligned, "\n"))
	}

	hint := "↑/↓ account  enter details  g set goal  esc back"
	if m.payCyclePromptMode != payCyclePromptNone {
		hint = "enter save  esc back"
	} else if hasAccount && hasPane {
		hint = "↑/↓ account  ←/→ transaction  tab focus  g set goal  esc close"
	}
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		Width(lipgloss.Width(mainBlock)).
		Align(lipgloss.Center).
		Render(hint)
	footer = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, footer)
	statusLines := []string{}
	if m.transactionsFetched != nil {
		age := time.Since(m.transactionsFetched.UTC()).Round(time.Second)
		if age < 0 {
			age = 0
		}
		statusLines = append(statusLines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			Width(lipgloss.Width(mainBlock)).
			Align(lipgloss.Center).
			Render(fmt.Sprintf("last updated %s ago", age.String())))
	}

	parts := []string{title}
	parts = append(parts, "", mainBlock)

	if m.payCyclePromptMode != payCyclePromptNone {
		label := renderPayCyclePromptLabel(m.payCyclePromptMode, account.displayName)
		input := m.payCycleInput
		input.Width = max(18, cardContentWidth-8)
		promptBody := []string{
			label,
			input.View(),
		}
		if strings.TrimSpace(m.payCyclePromptErr) != "" {
			promptBody = append(promptBody, lipgloss.NewStyle().Foreground(lipgloss.Color("#F15B5B")).Render(m.payCyclePromptErr))
		}
		promptCard := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFD54A")).
			Padding(0, 1).
			Width(cardContentWidth).
			Render(strings.Join(promptBody, "\n"))
		parts = append(parts, "", lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, promptCard))
	}
	if strings.TrimSpace(m.payCycleErr) != "" {
		parts = append(parts, "", lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, lipgloss.NewStyle().Foreground(lipgloss.Color("#F15B5B")).Render("error: "+m.payCycleErr)))
	}
	if metaBlock != "" {
		parts = append(parts, "", metaBlock)
	}
	parts = append(parts, "", footer)
	if len(statusLines) > 0 {
		parts = append(parts, "", lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, strings.Join(statusLines, "\n")))
	}
	return strings.Join(parts, "\n")
}
