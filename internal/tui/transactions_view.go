package tui

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lachiem1/giddyUp/internal/auth"
	"github.com/lachiem1/giddyUp/internal/storage"
	"github.com/lachiem1/giddyUp/internal/syncer"
	"github.com/lachiem1/giddyUp/internal/upapi"
)

type transactionSortOption struct {
	label   string
	orderBy string
}

type transactionQuickRange struct {
	label string
	apply func(now time.Time) (time.Time, time.Time)
}

const (
	txFilterFromDateKey        = "transactions.filter.from_date"
	txFilterToDateKey          = "transactions.filter.to_date"
	txFilterModeKey            = "transactions.filter.mode"
	txFilterQuickIdxKey        = "transactions.filter.quick_idx"
	txFilterIncludeInternalKey = "transactions.filter.include_internal_transfers"
)

func renderTransactionsTitle() string {
	// Reuse exact accounts glyphs for shared letters: A, C, O, N, T, S.
	glyphs := map[rune][3]string{
		'A': {"▄▀█", "█▀█", "▀ ▀"},
		'C': {"█▀▀", "█▄▄", "▀▀▀"},
		'O': {"█▀█", "█▄█", "▀▀▀"},
		'N': {"█▄ █", "█ ▀█", "▀  ▀"},
		'T': {"▀█▀", " █ ", " ▀ "},
		'S': {"█▀", "▄█", "▀▀"},
		'R': {"█▀█", "█▀▄", "▀ ▀"},
		'I': {"█", "█", "▀"},
	}
	word := "TRANSACTIONS"
	lineParts := [3][]string{{}, {}, {}}
	for _, ch := range word {
		g, ok := glyphs[ch]
		if !ok {
			continue
		}
		lineParts[0] = append(lineParts[0], g[0])
		lineParts[1] = append(lineParts[1], g[1])
		lineParts[2] = append(lineParts[2], g[2])
	}
	raw := []string{
		strings.Join(lineParts[0], " "),
		strings.Join(lineParts[1], " "),
		strings.Join(lineParts[2], " "),
	}
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#87CEEB")).
		Bold(true)
	rows := make([]string, 0, len(raw))
	for _, line := range raw {
		rows = append(rows, style.Render(line))
	}
	return strings.Join(rows, "\n")
}

func (m model) loadTransactionsPreviewCmd() tea.Cmd {
	page := m.transactionsPage
	pageSize := m.transactionsPageSize
	fromDigits := m.transactionsFromDate
	toDigits := m.transactionsToDate
	includeInternal := m.transactionsIncludeInternal
	sortIdx := m.transactionsSortIdx
	viewMode := m.transactionsViewMode
	searchQuery := m.transactionsSearchApplied
	timeSeriesCategory := strings.TrimSpace(m.transactionsTimeSeriesCategory)
	return func() tea.Msg {
		if m.db == nil {
			return loadTransactionsPreviewMsg{err: fmt.Errorf("database is not initialized")}
		}
		if pageSize <= 0 {
			pageSize = max(1, m.transactionsVisibleRows())
		}
		if page < 0 {
			page = 0
		}
		sorts := transactionsSortOptions()
		orderBy := sorts[0].orderBy
		if viewMode == transactionsViewModeTable {
			if sortIdx < 0 || sortIdx >= len(sorts) {
				sortIdx = 0
			}
			orderBy = sorts[sortIdx].orderBy
		}
		rows, categorySpend, timeSeries, fetchedAt, total, clampedPage, err := queryTransactionsPreview(
			m.db,
			fromDigits,
			toDigits,
			includeInternal,
			searchQuery,
			timeSeriesCategory,
			orderBy,
			page,
			pageSize,
		)
		if err != nil {
			return loadTransactionsPreviewMsg{err: err}
		}
		return loadTransactionsPreviewMsg{
			rows:          rows,
			categorySpend: categorySpend,
			timeSeries:    timeSeries,
			lastFetchedAt: fetchedAt,
			totalCount:    total,
			page:          clampedPage,
		}
	}
}

func (m model) loadCategoryTransactionsCmd(category string, sortIdx int) tea.Cmd {
	fromDigits := m.transactionsFromDate
	toDigits := m.transactionsToDate
	includeInternal := m.transactionsIncludeInternal
	searchQuery := m.transactionsSearchApplied
	sorts := transactionsCategoryTransactionSortOptions()
	if len(sorts) == 0 {
		sorts = []transactionSortOption{
			{label: "amount ↑", orderBy: "t.amount_value_in_base_units ASC, t.created_at DESC, t.id DESC"},
		}
	}
	if sortIdx < 0 || sortIdx >= len(sorts) {
		sortIdx = 0
	}
	orderBy := sorts[sortIdx].orderBy
	return func() tea.Msg {
		if m.db == nil {
			return loadCategoryTransactionsMsg{err: fmt.Errorf("database is not initialized")}
		}
		rows, err := queryCategoryTransactions(
			m.db,
			fromDigits,
			toDigits,
			includeInternal,
			searchQuery,
			category,
			orderBy,
		)
		return loadCategoryTransactionsMsg{
			category: category,
			sortIdx:  sortIdx,
			rows:     rows,
			err:      err,
		}
	}
}

func (m model) loadTransactionsFiltersCmd() tea.Cmd {
	defaultFrom := m.transactionsFromDate
	defaultTo := m.transactionsToDate
	defaultMode := m.transactionsFilterMode
	defaultQuick := m.transactionsQuickIdx
	defaultIncludeInternal := m.transactionsIncludeInternal
	return func() tea.Msg {
		if m.db == nil {
			return loadTransactionsFiltersMsg{err: fmt.Errorf("database is not initialized")}
		}
		repo := storage.NewAppConfigRepo(m.db)
		ctx := context.Background()

		from, fromFound, err := repo.Get(ctx, txFilterFromDateKey)
		if err != nil {
			return loadTransactionsFiltersMsg{err: err}
		}
		to, toFound, err := repo.Get(ctx, txFilterToDateKey)
		if err != nil {
			return loadTransactionsFiltersMsg{err: err}
		}
		modeRaw, modeFound, err := repo.Get(ctx, txFilterModeKey)
		if err != nil {
			return loadTransactionsFiltersMsg{err: err}
		}
		quickRaw, quickFound, err := repo.Get(ctx, txFilterQuickIdxKey)
		if err != nil {
			return loadTransactionsFiltersMsg{err: err}
		}
		includeRaw, includeFound, err := repo.Get(ctx, txFilterIncludeInternalKey)
		if err != nil {
			return loadTransactionsFiltersMsg{err: err}
		}

		mode := defaultMode
		if modeFound {
			mode = transactionsFilterModeQuick
			if strings.TrimSpace(modeRaw) == "custom" {
				mode = transactionsFilterModeCustom
			}
		}
		quickIdx := defaultQuick
		if quickFound {
			if n, err := strconv.Atoi(strings.TrimSpace(quickRaw)); err == nil {
				quickIdx = n
			}
		}
		if !fromFound {
			from = defaultFrom
		}
		if !toFound {
			to = defaultTo
		}
		includeInternal := defaultIncludeInternal
		if includeFound {
			v := strings.ToLower(strings.TrimSpace(includeRaw))
			includeInternal = v == "1" || v == "true" || v == "yes" || v == "on"
		}
		return loadTransactionsFiltersMsg{
			fromDate:        strings.TrimSpace(from),
			toDate:          strings.TrimSpace(to),
			mode:            mode,
			quickIdx:        quickIdx,
			includeInternal: includeInternal,
		}
	}
}

func (m model) saveTransactionsFiltersCmd() tea.Cmd {
	from := strings.TrimSpace(m.transactionsFromDate)
	to := strings.TrimSpace(m.transactionsToDate)
	mode := "quick"
	if m.transactionsFilterMode == transactionsFilterModeCustom {
		mode = "custom"
	}
	quickIdx := m.transactionsQuickIdx
	includeInternal := m.transactionsIncludeInternal
	return func() tea.Msg {
		if m.db == nil {
			return saveTransactionsFiltersMsg{err: fmt.Errorf("database is not initialized")}
		}
		repo := storage.NewAppConfigRepo(m.db)
		err := repo.UpsertMany(context.Background(), map[string]string{
			txFilterFromDateKey:        from,
			txFilterToDateKey:          to,
			txFilterModeKey:            mode,
			txFilterQuickIdxKey:        strconv.Itoa(quickIdx),
			txFilterIncludeInternalKey: strconv.FormatBool(includeInternal),
		})
		return saveTransactionsFiltersMsg{err: err}
	}
}

func appendTransactionsSearchClauses(searchQuery string, where *[]string, args *[]any) error {
	if isTransactionsSearchHelpQuery(searchQuery) || isTransactionsSearchResetQuery(searchQuery) {
		return nil
	}
	normalized := normalizeTransactionsSearchQuery(searchQuery)
	if normalized == "" {
		return nil
	}

	parts := splitTransactionsSearchParts(normalized)
	lastField := ""
	for _, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			return fmt.Errorf("invalid search syntax")
		}

		field := ""
		value := ""
		colon := strings.Index(part, ":")
		switch {
		case colon > 0:
			if colon == len(part)-1 {
				return fmt.Errorf("invalid search syntax")
			}
			field = strings.ToLower(strings.TrimSpace(part[:colon]))
			value = strings.TrimSpace(part[colon+1:])
		case colon == -1 && lastField == "exclude-category":
			// Allow shorthand continuation for exclude-category:
			//   /exclude-category: uncat + hobb
			field = lastField
			value = part
		default:
			return fmt.Errorf("invalid search syntax")
		}
		if value == "" {
			return fmt.Errorf("invalid search syntax")
		}

		switch field {
		case "merchant":
			*where = append(*where, `LOWER(COALESCE(
				NULLIF(t.merchant_norm, ''),
				NULLIF(t.raw_text_norm, ''),
				NULLIF(t.description_norm, ''),
				COALESCE(t.raw_text, t.description, '')
			)) LIKE ?`)
			*args = append(*args, "%"+strings.ToLower(value)+"%")
		case "description":
			*where = append(*where, `LOWER(COALESCE(
				NULLIF(t.description_norm, ''),
				COALESCE(t.description, '')
			)) LIKE ?`)
			*args = append(*args, "%"+strings.ToLower(value)+"%")
		case "category":
			*where = append(*where, "LOWER(COALESCE(NULLIF(TRIM(t.category_id), ''), 'uncategorized')) LIKE ?")
			*args = append(*args, "%"+strings.ToLower(value)+"%")
		case "exclude-category":
			*where = append(*where, "LOWER(COALESCE(NULLIF(TRIM(t.category_id), ''), 'uncategorized')) NOT LIKE ?")
			*args = append(*args, "%"+strings.ToLower(value)+"%")
		case "type":
			sign, ok := parseTransactionTypeValue(value)
			if !ok {
				return fmt.Errorf("invalid search syntax")
			}
			if sign > 0 {
				*where = append(*where, "t.amount_value_in_base_units > 0")
			} else {
				*where = append(*where, "t.amount_value_in_base_units < 0")
			}
		case "amount":
			op, cents, ok := parseTransactionAmountValue(value)
			if !ok {
				return fmt.Errorf("invalid search syntax")
			}
			*where = append(*where, fmt.Sprintf("ABS(t.amount_value_in_base_units) %s ?", op))
			*args = append(*args, cents)
		default:
			return fmt.Errorf("invalid search syntax")
		}
		lastField = field
	}

	return nil
}

func normalizeTransactionsSearchQuery(searchQuery string) string {
	trimmed := strings.TrimSpace(searchQuery)
	if strings.HasPrefix(trimmed, "/") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "/"))
	}
	return trimmed
}

func isTransactionsSearchHelpQuery(searchQuery string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(searchQuery))
	return trimmed == "/help" || trimmed == "help"
}

func isTransactionsSearchResetQuery(searchQuery string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(searchQuery))
	return trimmed == "/reset" || trimmed == "reset"
}

func splitTransactionsSearchParts(searchQuery string) []string {
	trimmed := strings.TrimSpace(searchQuery)
	if trimmed == "" {
		return nil
	}

	parts := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != '+' {
			continue
		}
		if i == 0 || i == len(trimmed)-1 {
			continue
		}
		if !isWhitespaceByte(trimmed[i-1]) || !isWhitespaceByte(trimmed[i+1]) {
			continue
		}
		parts = append(parts, strings.TrimSpace(trimmed[start:i]))
		start = i + 1
	}
	parts = append(parts, strings.TrimSpace(trimmed[start:]))
	return parts
}

func isWhitespaceByte(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func parseTransactionTypeValue(value string) (int, bool) {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "+ve", "positive", "credit", "income":
		return 1, true
	case "-ve", "negative", "debit", "expense", "spend":
		return -1, true
	default:
		return 0, false
	}
}

func parseTransactionAmountValue(value string) (string, int64, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", 0, false
	}

	op := "="
	for _, candidate := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(v, candidate) {
			op = candidate
			v = strings.TrimSpace(strings.TrimPrefix(v, candidate))
			break
		}
	}
	if v == "" {
		return "", 0, false
	}

	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return "", 0, false
	}
	cents := int64(math.Round(math.Abs(n) * 100))
	return op, cents, true
}

func queryTransactionsPreview(
	db *sql.DB,
	fromDigits string,
	toDigits string,
	includeInternal bool,
	searchQuery string,
	timeSeriesCategory string,
	orderBy string,
	page int,
	pageSize int,
) ([]transactionPreviewRow, []transactionsCategorySpend, []transactionsTimeSeriesPoint, *time.Time, int, int, error) {
	where := []string{"t.is_active = 1"}
	args := make([]any, 0, 8)
	if !includeInternal {
		where = append(where, "t.transfer_account_id IS NULL")
	}
	if err := appendTransactionsSearchClauses(strings.TrimSpace(searchQuery), &where, &args); err != nil {
		return nil, nil, nil, nil, 0, 0, err
	}

	if len(strings.TrimSpace(fromDigits)) == 8 {
		fromDate, err := parseTransactionsDateDigits(fromDigits)
		if err != nil {
			return nil, nil, nil, nil, 0, 0, err
		}
		where = append(where, "date(t.created_at) >= date(?)")
		args = append(args, fromDate)
	}
	if len(strings.TrimSpace(toDigits)) == 8 {
		toDate, err := parseTransactionsDateDigits(toDigits)
		if err != nil {
			return nil, nil, nil, nil, 0, 0, err
		}
		where = append(where, "date(t.created_at) <= date(?)")
		args = append(args, toDate)
	}
	if len(strings.TrimSpace(fromDigits)) == 8 && len(strings.TrimSpace(toDigits)) == 8 {
		fromDate, _ := parseTransactionsDateDigits(fromDigits)
		toDate, _ := parseTransactionsDateDigits(toDigits)
		if fromDate > toDate {
			return nil, nil, nil, nil, 0, 0, fmt.Errorf("from date cannot be after to date")
		}
	}

	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(
		context.Background(),
		fmt.Sprintf("SELECT COUNT(*) FROM transactions t WHERE %s", whereSQL),
		args...,
	).Scan(&total); err != nil {
		return nil, nil, nil, nil, 0, 0, err
	}

	if pageSize <= 0 {
		pageSize = 12
	}
	if page < 0 {
		page = 0
	}
	maxPage := 0
	if total > 0 {
		maxPage = (total - 1) / pageSize
	}
	if page > maxPage {
		page = maxPage
	}
	offset := page * pageSize

	q := fmt.Sprintf(
		`SELECT
			t.created_at,
			t.id,
			COALESCE(NULLIF(t.raw_text_norm, ''), COALESCE(t.raw_text, '')),
			COALESCE(NULLIF(t.description_norm, ''), COALESCE(t.description, '')),
			t.amount_value,
			COALESCE(
				NULLIF(t.merchant_norm, ''),
				COALESCE(
					NULLIF(t.raw_text_norm, ''),
					NULLIF(t.description_norm, ''),
					COALESCE(t.raw_text, t.description, '')
				)
			),
			t.status,
			COALESCE(t.message, ''),
			COALESCE(t.category_id, ''),
			COALESCE(t.card_purchase_method_method, ''),
			COALESCE(t.note_text, ''),
			COALESCE(a.display_name, '')
		 FROM transactions t
		 LEFT JOIN accounts a ON a.id = t.account_id
		 WHERE %s
		 ORDER BY %s
		 LIMIT ? OFFSET ?`,
		whereSQL,
		orderBy,
	)
	pageArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := db.QueryContext(context.Background(), q, pageArgs...)
	if err != nil {
		return nil, nil, nil, nil, 0, 0, err
	}
	defer rows.Close()

	out := make([]transactionPreviewRow, 0, 64)
	for rows.Next() {
		var r transactionPreviewRow
		if err := rows.Scan(
			&r.createdAt,
			&r.id,
			&r.rawText,
			&r.description,
			&r.amountValue,
			&r.merchant,
			&r.status,
			&r.message,
			&r.categoryID,
			&r.cardMethod,
			&r.noteText,
			&r.accountName,
		); err != nil {
			return nil, nil, nil, nil, 0, 0, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, nil, 0, 0, err
	}

	categorySpend, err := queryCategorySpend(context.Background(), db, whereSQL, args)
	if err != nil {
		return nil, nil, nil, nil, 0, 0, err
	}

	timeSeries, err := querySpendTimeSeries(context.Background(), db, whereSQL, args, fromDigits, toDigits, timeSeriesCategory)
	if err != nil {
		return nil, nil, nil, nil, 0, 0, err
	}

	var lastSuccess *time.Time
	stateRepo := storage.NewSyncStateRepo(db)
	state, found, err := stateRepo.Get(context.Background(), syncer.CollectionTransactions)
	if err != nil {
		return nil, nil, nil, nil, 0, 0, err
	}
	if found && state.LastSuccess != nil {
		t := state.LastSuccess.UTC()
		lastSuccess = &t
	}

	return out, categorySpend, timeSeries, lastSuccess, total, page, nil
}

func queryCategoryTransactions(
	db *sql.DB,
	fromDigits string,
	toDigits string,
	includeInternal bool,
	searchQuery string,
	category string,
	orderBy string,
) ([]categoryTransactionRow, error) {
	where := []string{"t.is_active = 1"}
	args := make([]any, 0, 10)
	if !includeInternal {
		where = append(where, "t.transfer_account_id IS NULL")
	}
	if err := appendTransactionsSearchClauses(strings.TrimSpace(searchQuery), &where, &args); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(fromDigits)) == 8 {
		fromDate, err := parseTransactionsDateDigits(fromDigits)
		if err != nil {
			return nil, err
		}
		where = append(where, "date(t.created_at) >= date(?)")
		args = append(args, fromDate)
	}
	if len(strings.TrimSpace(toDigits)) == 8 {
		toDate, err := parseTransactionsDateDigits(toDigits)
		if err != nil {
			return nil, err
		}
		where = append(where, "date(t.created_at) <= date(?)")
		args = append(args, toDate)
	}
	categoryNorm := strings.ToLower(strings.TrimSpace(category))
	where = append(where, "LOWER(COALESCE(NULLIF(TRIM(t.category_id), ''), 'uncategorized')) = ?")
	args = append(args, categoryNorm)

	whereSQL := strings.Join(where, " AND ")
	if strings.TrimSpace(orderBy) == "" {
		sorts := transactionsCategoryTransactionSortOptions()
		if len(sorts) == 0 {
			sorts = []transactionSortOption{
				{label: "amount ↑", orderBy: "t.amount_value_in_base_units ASC, t.created_at DESC, t.id DESC"},
			}
		}
		orderBy = sorts[0].orderBy
	}
	q := fmt.Sprintf(
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
			COALESCE(NULLIF(t.description_norm, ''), COALESCE(t.description, '')) AS description,
			t.amount_value,
			COALESCE(NULLIF(t.raw_text_norm, ''), COALESCE(t.raw_text, '')) AS raw_text,
			COALESCE(t.status, ''),
			COALESCE(t.message, ''),
			COALESCE(t.category_id, ''),
			COALESCE(t.card_purchase_method_method, ''),
			COALESCE(t.note_text, ''),
			COALESCE(a.display_name, '')
		 FROM transactions t
		 LEFT JOIN accounts a ON a.id = t.account_id
		 WHERE %s
		 ORDER BY %s`,
		whereSQL,
		orderBy,
	)

	rows, err := db.QueryContext(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]categoryTransactionRow, 0, 64)
	for rows.Next() {
		var r categoryTransactionRow
		if err := rows.Scan(
			&r.id,
			&r.createdAt,
			&r.merchant,
			&r.description,
			&r.amountValue,
			&r.rawText,
			&r.status,
			&r.message,
			&r.categoryID,
			&r.cardMethod,
			&r.noteText,
			&r.accountName,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func queryCategorySpend(ctx context.Context, db *sql.DB, whereSQL string, args []any) ([]transactionsCategorySpend, error) {
	q := fmt.Sprintf(
		`SELECT
			COALESCE(NULLIF(TRIM(t.category_id), ''), 'uncategorized') AS category,
			SUM(CASE WHEN t.amount_value_in_base_units < 0 THEN -t.amount_value_in_base_units ELSE 0 END) AS spend_cents
		 FROM transactions t
		 WHERE %s
		 GROUP BY category
		 HAVING spend_cents > 0
		 ORDER BY spend_cents DESC, category ASC`,
		whereSQL,
	)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]transactionsCategorySpend, 0, 16)
	var total int64
	for rows.Next() {
		var r transactionsCategorySpend
		if err := rows.Scan(&r.category, &r.spendCents); err != nil {
			return nil, err
		}
		total += r.spendCents
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if total <= 0 {
		return out, nil
	}
	for i := range out {
		out[i].percentOfSpend = (float64(out[i].spendCents) / float64(total)) * 100.0
	}
	return out, nil
}

func querySpendTimeSeries(
	ctx context.Context,
	db *sql.DB,
	whereSQL string,
	args []any,
	fromDigits string,
	toDigits string,
	timeSeriesCategory string,
) ([]transactionsTimeSeriesPoint, error) {
	_ = fromDigits
	_ = toDigits

	timeSeriesWhere := whereSQL
	timeSeriesArgs := append([]any{}, args...)
	timeSeriesWhere += " AND t.amount_value_in_base_units < 0"
	if strings.TrimSpace(timeSeriesCategory) != "" {
		timeSeriesWhere += " AND LOWER(COALESCE(NULLIF(TRIM(t.category_id), ''), 'uncategorized')) = ?"
		timeSeriesArgs = append(timeSeriesArgs, strings.ToLower(strings.TrimSpace(timeSeriesCategory)))
	}
	q := fmt.Sprintf(
		`SELECT
			t.created_at,
			date(t.created_at) AS day,
			t.id,
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
		 WHERE %s
		 ORDER BY t.created_at ASC, t.id ASC`,
		timeSeriesWhere,
	)
	rows, err := db.QueryContext(ctx, q, timeSeriesArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	raw := make([]transactionsTimeSeriesPoint, 0, 32)
	for rows.Next() {
		var p transactionsTimeSeriesPoint
		var spend sql.NullInt64
		if err := rows.Scan(
			&p.createdAt,
			&p.date,
			&p.id,
			&p.merchant,
			&p.rawText,
			&p.description,
			&p.amountValue,
			&spend,
			&p.status,
			&p.message,
			&p.categoryID,
			&p.cardMethod,
			&p.noteText,
			&p.accountName,
		); err != nil {
			return nil, err
		}
		if spend.Valid {
			p.spendCents = spend.Int64
		}
		raw = append(raw, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return raw, nil
}

func chartFooterHelpText(mode int) string {
	if mode == transactionsViewModeTable {
		return "/ search  f filters  s sort"
	}
	if mode == transactionsViewModeTimeSeries {
		return "↑/↓ category  ←/→ node/pan  +/- zoom  enter details  f filters"
	}
	return "/ search  f filters"
}

func (m model) syncTransactionsCmd(sessionID int, force bool) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return syncTransactionsDoneMsg{sessionID: sessionID, err: errors.New("database is not initialized")}
		}
		err := syncTransactionsIntoDB(m.db, force)
		return syncTransactionsDoneMsg{sessionID: sessionID, err: err}
	}
}

func syncTransactionsIntoDB(sqlDB *sql.DB, force bool) error {
	pat, err := auth.LoadPAT()
	if err != nil {
		return err
	}
	client := upapi.New(pat)
	service, err := syncer.NewTransactionsService(sqlDB, client)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	defer service.LeaveView()

	repo := storage.NewSyncStateRepo(sqlDB)
	txRepo := storage.NewTransactionsRepo(sqlDB)
	hasCached, err := txRepo.HasAny(ctx)
	if err != nil {
		return err
	}

	var prevAttempt *time.Time
	var prevSuccess *time.Time
	if state, found, err := repo.Get(ctx, syncer.CollectionTransactions); err == nil && found {
		if state.LastAttempt != nil {
			t := state.LastAttempt.UTC()
			prevAttempt = &t
		}
		if state.LastSuccess != nil {
			t := state.LastSuccess.UTC()
			prevSuccess = &t
		}
	}

	if err := service.EnterTransactionsView(ctx); err != nil {
		return err
	}
	if force {
		if err := service.RefreshTransactions(); err != nil {
			return err
		}
		return waitForTransactionsSyncResult(ctx, repo, prevAttempt, prevSuccess)
	}
	isStale := prevSuccess == nil || time.Since(prevSuccess.UTC()) > 30*time.Second
	if hasCached && !isStale {
		return nil
	}
	return waitForTransactionsSyncResult(ctx, repo, prevAttempt, prevSuccess)
}

func waitForTransactionsSyncResult(
	ctx context.Context,
	repo *storage.SyncStateRepo,
	previousAttempt *time.Time,
	previousSuccess *time.Time,
) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, found, err := repo.Get(ctx, syncer.CollectionTransactions)
		if err == nil && found && state.LastAttempt != nil {
			attemptChanged := previousAttempt == nil || state.LastAttempt.After(*previousAttempt)
			successChanged := false
			if state.LastSuccess != nil {
				successChanged = previousSuccess == nil || state.LastSuccess.After(*previousSuccess)
			}
			if attemptChanged && strings.TrimSpace(state.LastErrorMsg) != "" {
				return errors.New(state.LastErrorMsg)
			}
			if successChanged {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("transactions sync timed out")
		case <-ticker.C:
		}
	}
}

func renderTransactionsViewModeSelector(mode int) string {
	item := func(label string, active bool) string {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		if active {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
		}
		return style.Render(label)
	}
	return "view: " +
		item("table [1]", mode == transactionsViewModeTable) +
		"  | " +
		item("chart [2]", mode == transactionsViewModeChart) +
		"  | " +
		item("time series [3]", mode == transactionsViewModeTimeSeries)
}

func renderTransactionsBodyLines(
	mode int,
	rows []transactionPreviewRow,
	categorySpend []transactionsCategorySpend,
	timeSeries []transactionsTimeSeriesPoint,
	timeSeriesCategory string,
	timeSeriesColor lipgloss.Color,
	timeSeriesSelected int,
	cursor int,
	merchantW int,
	contentWidth int,
	chartCursor int,
	chartShowAmount bool,
) []string {
	switch mode {
	case transactionsViewModeChart:
		return renderTransactionsChartLines(categorySpend, contentWidth, chartCursor, chartShowAmount)
	case transactionsViewModeTimeSeries:
		return renderTransactionsTimeSeriesLines(timeSeries, contentWidth, timeSeriesCategory, timeSeriesColor, timeSeriesSelected)
	default:
		return renderTransactionsTableLines(rows, cursor, merchantW)
	}
}

func padTransactionsBodyLines(lines []string, target int) []string {
	if target <= 0 {
		return lines
	}
	if len(lines) >= target {
		return lines[:target]
	}
	out := append([]string{}, lines...)
	for len(out) < target {
		out = append(out, "")
	}
	return out
}

func transactionsCategoryPalette() []lipgloss.Color {
	return []lipgloss.Color{
		"#E53935", "#1E88E5", "#43A047", "#FB8C00", "#8E24AA", "#00897B",
		"#F4511E", "#3949AB", "#7CB342", "#D81B60", "#00ACC1", "#6D4C41",
		"#5E35B1", "#C0CA33", "#039BE5", "#FDD835", "#8D6E63", "#00C853",
		"#FF7043", "#546E7A", "#AEEA00", "#26A69A", "#EF5350", "#7E57C2",
		"#29B6F6", "#9CCC65", "#FFCA28", "#AB47BC", "#66BB6A", "#EC407A",
	}
}

func transactionsCategoryColor(rank int) lipgloss.Color {
	palette := transactionsCategoryPalette()
	if len(palette) == 0 {
		return lipgloss.Color("#D1D5DB")
	}
	if rank < 0 {
		rank = 0
	}
	return palette[rank%len(palette)]
}

func renderTransactionsTableLines(rows []transactionPreviewRow, cursor int, merchantW int) []string {
	header := fmt.Sprintf("  %-10s  %-"+strconv.Itoa(merchantW)+"s  %10s", "date", "merchant", "amount")
	out := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true).Render(header),
	}
	if len(rows) == 0 {
		return append(out, lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("no transactions found"))
	}
	for i, row := range rows {
		prefix := "  "
		if i == cursor {
			prefix = "› "
		}
		date := formatTransactionDate(row.createdAt)
		merchant := truncateDisplayWidth(strings.TrimSpace(row.merchant), merchantW)
		line := fmt.Sprintf("%s%-10s  %-"+strconv.Itoa(merchantW)+"s  %10s", prefix, date, merchant, row.amountValue)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
		if i == cursor {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
		}
		out = append(out, style.Render(line))
	}
	return out
}

func renderTransactionsChartLines(categorySpend []transactionsCategorySpend, contentWidth int, chartCursor int, showAmount bool) []string {
	out := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true).Render("spend by category"),
	}
	if len(categorySpend) == 0 {
		return append(out, lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("no transactions found"))
	}

	maxSpendCents := int64(1)
	for _, c := range categorySpend {
		if c.spendCents > maxSpendCents {
			maxSpendCents = c.spendCents
		}
	}
	if maxSpendCents <= 0 {
		maxSpendCents = 1
	}
	// Fit each chart row to the current content width to avoid wrapping.
	fixed := 12 // prefix + spaces + pct
	if showAmount {
		fixed = 22 // adds 9.2f amount column and spacing
	}
	const rightSlack = 5
	available := max(6, contentWidth-fixed-rightSlack)
	labelWidth := min(32, max(6, int(math.Round(float64(available)*0.58))))
	barWidth := max(3, available-labelWidth)
	rows := categorySpend
	for i, row := range rows {
		dollars := float64(row.spendCents) / 100.0
		barLen := int(math.Round((float64(row.spendCents) / float64(maxSpendCents)) * float64(barWidth)))
		barLen = max(1, barLen)
		if barWidth > 1 {
			barLen = min(barLen, barWidth-1)
		}
		bar := strings.Repeat("█", barLen)
		label := truncateDisplayWidth(strings.TrimSpace(row.category), labelWidth)
		prefix := "  "
		if i == chartCursor {
			prefix = "› "
		}
		line := fmt.Sprintf("%s%-"+strconv.Itoa(labelWidth)+"s  %s  %5.1f%%", prefix, label, bar, row.percentOfSpend)
		if showAmount {
			line = fmt.Sprintf("%s%-"+strconv.Itoa(labelWidth)+"s  %9.2f  %s  %5.1f%%", prefix, label, dollars, bar, row.percentOfSpend)
		}
		line = truncateDisplayWidth(line, max(8, contentWidth))
		style := lipgloss.NewStyle().Foreground(transactionsCategoryColor(i))
		if i == chartCursor {
			style = lipgloss.NewStyle().Foreground(transactionsCategoryColor(i)).Bold(true)
		}
		out = append(out, style.Render(line))
	}
	return out
}

func renderTransactionsTimeSeriesLines(
	points []transactionsTimeSeriesPoint,
	contentWidth int,
	categoryLabel string,
	seriesColor lipgloss.Color,
	selectedPoint int,
) []string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	if strings.TrimSpace(string(seriesColor)) == "" {
		seriesColor = lipgloss.Color("#6CBFE6")
	}
	lineStyle := lipgloss.NewStyle().Foreground(seriesColor)
	focusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD54A")).Bold(true)
	seriesLabelStyle := lipgloss.NewStyle().Foreground(seriesColor).Bold(true)

	out := []string{titleStyle.Render("spend over time")}
	seriesName := "all"
	if strings.TrimSpace(categoryLabel) != "" {
		seriesName = strings.TrimSpace(categoryLabel)
	}
	out = append(out, seriesLabelStyle.Render("category: "+seriesName))
	if len(points) == 0 {
		return append(out, labelStyle.Render("no transactions found"))
	}
	if selectedPoint < 0 || selectedPoint >= len(points) {
		selectedPoint = len(points) - 1
	}

	maxSpend := int64(0)
	totalSpend := int64(0)
	for _, p := range points {
		if p.spendCents > maxSpend {
			maxSpend = p.spendCents
		}
		totalSpend += p.spendCents
	}
	if maxSpend <= 0 {
		maxSpend = 1
	}

	innerWidth := max(12, contentWidth-2)
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
		yTickByRow[row] = int64(math.Round(ratio * float64(maxSpend)))
	}
	yTickByRow[plotHeight-1] = 0
	yLabelWidth := 1
	for _, cents := range yTickByRow {
		w := lipgloss.Width(formatTimeSeriesDollar(cents))
		if w > yLabelWidth {
			yLabelWidth = w
		}
	}

	graphWidth := max(10, innerWidth-yLabelWidth-1)
	dataCols := max(8, graphWidth-1) // keep first column for y-axis
	graphWidth = dataCols + 1
	xAxisRow := plotHeight - 1
	grid := make([][]rune, plotHeight)
	for i := range grid {
		grid[i] = make([]rune, graphWidth)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
		grid[i][0] = '|'
	}
	for x := 0; x < graphWidth; x++ {
		grid[xAxisRow][x] = '—'
	}
	grid[xAxisRow][0] = '└'

	pointX := make([]int, len(points))
	pointY := make([]int, len(points))
	for i := range points {
		pointX[i] = timeSeriesPointColumn(i, len(points), dataCols)
	}

	prevGridX, prevGridY := -1, -1
	for i, p := range points {
		ratio := 0.0
		if maxSpend > 0 {
			ratio = float64(p.spendCents) / float64(maxSpend)
		}
		y := xAxisRow - int(math.Round(ratio*float64(xAxisRow)))
		y = max(0, min(plotHeight-1, y))
		if y == xAxisRow && xAxisRow > 0 {
			// Keep x-axis visually continuous; show near-zero points just above axis.
			y = xAxisRow - 1
		}
		gridX := pointX[i] + 1 // shift right of y-axis
		pointY[i] = y
		if prevGridX >= 0 {
			drawTransactionsSeriesSegment(grid, prevGridX, prevGridY, gridX, y, xAxisRow)
		}
		prevGridX, prevGridY = gridX, y
	}

	showLargeNodes := len(points) <= max(14, dataCols/2)
	nodeChar := '·'
	selectedNodeChar := '•'
	if showLargeNodes {
		nodeChar = '●'
		selectedNodeChar = '◉'
	}
	selectedGridX, selectedGridY := -1, -1
	for i := range points {
		gridX := pointX[i] + 1
		pointNode := nodeChar
		if i == selectedPoint {
			pointNode = selectedNodeChar
			selectedGridX = gridX
			selectedGridY = pointY[i]
		}
		grid[pointY[i]][gridX] = pointNode
	}

	for row := 0; row < plotHeight; row++ {
		axisLabel := ""
		if cents, ok := yTickByRow[row]; ok {
			axisLabel = formatTimeSeriesDollar(cents)
		}
		prefix := fmt.Sprintf("%*s ", yLabelWidth, axisLabel)
		maxGraphWidth := max(1, innerWidth-lipgloss.Width(prefix))
		graphRunes := grid[row]
		if len(graphRunes) > maxGraphWidth {
			graphRunes = graphRunes[:maxGraphWidth]
		}
		rowSelectedCol := -1
		if row == selectedGridY && selectedGridX >= 0 && selectedGridX < len(graphRunes) {
			rowSelectedCol = selectedGridX
		}
		graphPart := renderTimeSeriesGraphRow(graphRunes, rowSelectedCol, lineStyle, focusStyle)
		out = append(out, labelStyle.Render(prefix)+graphPart)
	}

	axisPrefix := strings.Repeat(" ", yLabelWidth+1)

	spanDays := timeSeriesDateSpanDays(points)
	tickPositions := buildTimeSeriesTickPositions(dataCols, innerWidth, spanDays)
	tickLabels := make([]string, 0, len(tickPositions))
	tickRunes := make([]rune, graphWidth)
	for i := range tickRunes {
		tickRunes[i] = ' '
	}
	lastTickDay := ""
	for _, pos := range tickPositions {
		if pos < 0 || pos >= dataCols {
			continue
		}
		tickRunes[pos+1] = '|'
		tickTS, tickDate := timestampForTimeSeriesTick(points, pos, dataCols)
		label := formatTimeSeriesTickLabel(tickDate, spanDays)
		if !tickTS.IsZero() {
			day := tickTS.Format("2006-01-02")
			if day == lastTickDay && lastTickDay != "" {
				label = tickTS.Format("15:04")
			}
			lastTickDay = day
		}
		tickLabels = append(tickLabels, label)
	}
	out = append(out, labelStyle.Render(truncateDisplayWidth(axisPrefix+string(tickRunes), innerWidth)))
	shiftedTickPositions := make([]int, 0, len(tickPositions))
	for _, pos := range tickPositions {
		shiftedTickPositions = append(shiftedTickPositions, pos+1)
	}
	out = append(out, labelStyle.Render(truncateDisplayWidth(axisPrefix+renderTimeSeriesLabelRow(graphWidth, shiftedTickPositions, tickLabels), innerWidth)))
	xAxisLabel := lipgloss.NewStyle().Width(graphWidth).Align(lipgloss.Center).Render("date")
	out = append(out, labelStyle.Render(truncateDisplayWidth(axisPrefix+xAxisLabel, innerWidth)))

	out = append(out, labelStyle.Render(truncateDisplayWidth(fmt.Sprintf("total spend: %s", formatTimeSeriesDollar(totalSpend)), innerWidth)))
	return out
}

func renderTimeSeriesGraphRow(graphRunes []rune, selectedCol int, lineStyle lipgloss.Style, focusStyle lipgloss.Style) string {
	if len(graphRunes) == 0 {
		return ""
	}
	if selectedCol < 0 || selectedCol >= len(graphRunes) {
		return lineStyle.Render(string(graphRunes))
	}
	left := string(graphRunes[:selectedCol])
	mid := string(graphRunes[selectedCol : selectedCol+1])
	right := string(graphRunes[selectedCol+1:])
	return lineStyle.Render(left) + focusStyle.Render(mid) + lineStyle.Render(right)
}

func drawTransactionsSeriesSegment(grid [][]rune, x0 int, y0 int, x1 int, y1 int, xAxisRow int) {
	if len(grid) == 0 || len(grid[0]) == 0 {
		return
	}
	dx := x1 - x0
	dy := y1 - y0
	steps := max(absInt(dx), absInt(dy))
	if steps <= 0 {
		return
	}
	for step := 1; step < steps; step++ {
		x := x0 + int(math.Round(float64(step*dx)/float64(steps)))
		y := y0 + int(math.Round(float64(step*dy)/float64(steps)))
		if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
			continue
		}
		if x == 0 || y == xAxisRow {
			continue
		}
		if grid[y][x] == ' ' || grid[y][x] == '—' {
			grid[y][x] = '.'
		}
	}
}

func resampleTransactionsTimeSeries(points []transactionsTimeSeriesPoint, maxWidth int) []transactionsTimeSeriesPoint {
	if len(points) == 0 {
		return nil
	}
	if maxWidth <= 0 {
		return nil
	}
	if maxWidth == 1 {
		return []transactionsTimeSeriesPoint{points[len(points)-1]}
	}
	if len(points) == maxWidth {
		out := make([]transactionsTimeSeriesPoint, len(points))
		copy(out, points)
		return out
	}
	if len(points) == 1 {
		out := make([]transactionsTimeSeriesPoint, maxWidth)
		for i := range out {
			out[i] = points[0]
		}
		return out
	}

	// Downsample using bucket averages to preserve broad trend across wide ranges.
	if len(points) > maxWidth {
		out := make([]transactionsTimeSeriesPoint, 0, maxWidth)
		bucket := float64(len(points)) / float64(maxWidth)
		for i := 0; i < maxWidth; i++ {
			start := int(math.Floor(float64(i) * bucket))
			end := int(math.Floor(float64(i+1) * bucket))
			if end <= start {
				end = start + 1
			}
			start = max(0, min(start, len(points)-1))
			end = max(start+1, min(end, len(points)))

			var sum int64
			for j := start; j < end; j++ {
				sum += points[j].spendCents
			}
			avg := sum / int64(end-start)
			out = append(out, transactionsTimeSeriesPoint{
				date:       points[end-1].date,
				spendCents: avg,
			})
		}
		return out
	}

	// Upsample to fill the whole plot width so axis length remains stable while zooming.
	out := make([]transactionsTimeSeriesPoint, 0, maxWidth)
	srcLast := len(points) - 1
	for i := 0; i < maxWidth; i++ {
		pos := float64(i) * float64(srcLast) / float64(maxWidth-1)
		lo := int(math.Floor(pos))
		hi := int(math.Ceil(pos))
		if hi > srcLast {
			hi = srcLast
		}
		if lo < 0 {
			lo = 0
		}
		if lo > srcLast {
			lo = srcLast
		}

		frac := pos - float64(lo)
		spend := points[lo].spendCents
		if hi != lo {
			delta := points[hi].spendCents - points[lo].spendCents
			spend = int64(math.Round(float64(points[lo].spendCents) + float64(delta)*frac))
		}
		dateIdx := int(math.Round(pos))
		if dateIdx < 0 {
			dateIdx = 0
		}
		if dateIdx > srcLast {
			dateIdx = srcLast
		}
		out = append(out, transactionsTimeSeriesPoint{
			date:       points[dateIdx].date,
			spendCents: spend,
		})
	}
	return out
}

func formatTimeSeriesDollar(cents int64) string {
	dollars := float64(cents) / 100.0
	value := fmt.Sprintf("$%.2f", dollars)
	if strings.HasSuffix(value, ".00") {
		value = strings.TrimSuffix(value, ".00")
	}
	return value
}

func timeSeriesDateSpanDays(points []transactionsTimeSeriesPoint) int {
	if len(points) < 2 {
		if len(points) == 1 {
			return 1
		}
		return 0
	}
	start, okStart := parseTimeSeriesDate(points[0].date)
	end, okEnd := parseTimeSeriesDate(points[len(points)-1].date)
	if !okStart || !okEnd || end.Before(start) {
		return len(points)
	}
	return int(end.Sub(start).Hours()/24) + 1
}

func parseTimeSeriesDate(raw string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(raw), time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func formatTimeSeriesTickLabel(raw string, spanDays int) string {
	t, ok := parseTimeSeriesDate(raw)
	if !ok {
		return truncateDisplayWidth(strings.TrimSpace(raw), 10)
	}
	switch {
	case spanDays <= 31:
		return t.Format("02 Jan")
	case spanDays <= 92:
		return t.Format("Jan 02")
	case spanDays <= 1095:
		return t.Format("Jan 2006")
	default:
		return t.Format("2006")
	}
}

func timeSeriesPointColumn(pointIdx int, pointCount int, colCount int) int {
	if pointCount <= 1 || colCount <= 1 {
		return 0
	}
	if pointIdx < 0 {
		pointIdx = 0
	}
	if pointIdx >= pointCount {
		pointIdx = pointCount - 1
	}
	return int(math.Round(float64(pointIdx) * float64(colCount-1) / float64(pointCount-1)))
}

func timestampForTimeSeriesTick(points []transactionsTimeSeriesPoint, pos int, colCount int) (time.Time, string) {
	if len(points) == 0 {
		return time.Time{}, ""
	}
	if len(points) == 1 || colCount <= 1 {
		last := points[len(points)-1]
		if ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(last.createdAt)); err == nil {
			return ts.In(time.Local), ts.In(time.Local).Format("2006-01-02")
		}
		return time.Time{}, last.date
	}
	if pos < 0 {
		pos = 0
	}
	if pos >= colCount {
		pos = colCount - 1
	}
	ratio := float64(pos) / float64(colCount-1)
	idx := int(math.Round(ratio * float64(len(points)-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(points) {
		idx = len(points) - 1
	}
	point := points[idx]
	if ts, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(point.createdAt)); err == nil {
		local := ts.In(time.Local)
		return local, local.Format("2006-01-02")
	}
	if d, ok := parseTimeSeriesDate(point.date); ok {
		return d, d.Format("2006-01-02")
	}
	return time.Time{}, point.date
}

func buildTimeSeriesTickPositions(pointCount int, contentWidth int, spanDays int) []int {
	if pointCount <= 0 {
		return nil
	}
	target := max(2, min(4, contentWidth/20))
	switch {
	case spanDays <= 14:
		target = max(target, min(10, contentWidth/9))
	case spanDays <= 31:
		target = max(target, min(8, contentWidth/11))
	case spanDays <= 92:
		target = max(target, min(6, contentWidth/14))
	}
	target = min(target, pointCount)
	if target == 1 {
		return []int{0}
	}
	out := make([]int, 0, target)
	last := -1
	for i := 0; i < target; i++ {
		pos := int(math.Round(float64(i) * float64(pointCount-1) / float64(target-1)))
		if pos == last {
			continue
		}
		out = append(out, pos)
		last = pos
	}
	if len(out) == 0 {
		return []int{0}
	}
	if out[0] != 0 {
		out = append([]int{0}, out...)
	}
	if out[len(out)-1] != pointCount-1 {
		out = append(out, pointCount-1)
	}
	return out
}

func renderTimeSeriesLabelRow(width int, tickPositions []int, labels []string) string {
	if width <= 0 {
		return ""
	}
	row := make([]rune, width)
	for i := range row {
		row[i] = ' '
	}
	lastEnd := -2
	for i := 0; i < len(tickPositions) && i < len(labels); i++ {
		label := strings.TrimSpace(labels[i])
		if label == "" {
			continue
		}
		pos := tickPositions[i]
		if pos < 0 || pos >= width {
			continue
		}
		runes := []rune(label)
		if len(runes) > width {
			runes = runes[:width]
		}
		start := pos - (len(runes) / 2)
		if start < 0 {
			start = 0
		}
		if start+len(runes) > width {
			start = width - len(runes)
		}
		end := start + len(runes) - 1
		if start <= lastEnd+1 {
			continue
		}
		for j, r := range runes {
			row[start+j] = r
		}
		if end > lastEnd {
			lastEnd = end
		}
	}
	return string(row)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (m model) renderTransactionsScreen(layoutWidth int) string {
	title := renderTransactionsTitle()
	title = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, title)

	sorts := transactionsSortOptions()
	sortLabel := sorts[0].label
	if m.transactionsSortIdx >= 0 && m.transactionsSortIdx < len(sorts) {
		sortLabel = sorts[m.transactionsSortIdx].label
	}
	rangeLabel := transactionsRangeLabel(m.transactionsFromDate, m.transactionsToDate)

	tableBorder := lipgloss.Color("#FFFFFF")
	chartPaneBorder := lipgloss.Color("#FFD54A")
	mainCap := int(math.Round(float64(layoutWidth) * 0.78))
	maxMainWidth := min(layoutWidth-8, max(36, mainCap))
	if maxMainWidth < 20 {
		maxMainWidth = max(20, layoutWidth-8)
	}
	paneWidth := max(30, min(40, layoutWidth/3))
	gapWidth := 3
	hasTablePane := m.transactionsViewMode == transactionsViewModeTable && m.transactionsPaneOpen
	hasTimeSeriesPane := m.transactionsViewMode == transactionsViewModeTimeSeries &&
		m.transactionsPaneOpen &&
		len(m.transactionsTimeSeries) > 0
	hasChartPane := m.transactionsViewMode == transactionsViewModeChart && m.transactionsChartPaneOpen
	if hasChartPane {
		if m.transactionsChartPaneFocus == transactionsChartFocusMain {
			tableBorder = lipgloss.Color("#FFD54A")
			chartPaneBorder = lipgloss.Color("#FFFFFF")
		} else {
			tableBorder = lipgloss.Color("#FFFFFF")
			chartPaneBorder = lipgloss.Color("#FFD54A")
		}
	}
	if hasTablePane || hasTimeSeriesPane || hasChartPane {
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
	tableContentWidth := wideMainContentWidth
	if hasTablePane {
		tableContentWidth = baseMainContentWidth
	}
	if hasChartPane || hasTimeSeriesPane {
		// Allocate widths from available layout space (responsive), prioritizing single-line rows.
		totalContent := max(20, layoutWidth-gapWidth-8) // subtract two cards' border+padding overhead.
		paneRatio := 0.40
		minPane := min(28, max(12, totalContent/3))
		minMain := min(24, max(12, totalContent/3))
		if hasTimeSeriesPane {
			// When the details pane is open in time-series, reserve more space for details
			// and compress the graph card further to prevent wrapping artifacts.
			paneRatio = 0.44
			minPane = min(44, max(24, totalContent/3))
			minMain = min(36, max(20, totalContent/3))
		}
		paneWidth = int(math.Round(float64(totalContent) * paneRatio))
		tableContentWidth = totalContent - paneWidth

		if paneWidth < minPane {
			paneWidth = minPane
			tableContentWidth = totalContent - paneWidth
		}
		if tableContentWidth < minMain {
			tableContentWidth = minMain
			paneWidth = totalContent - tableContentWidth
		}
		if paneWidth < 12 {
			paneWidth = 12
			tableContentWidth = totalContent - paneWidth
		}
		if tableContentWidth < 12 {
			tableContentWidth = 12
			paneWidth = totalContent - tableContentWidth
			if paneWidth < 8 {
				paneWidth = 8
			}
		}
	} else {
		tableContentWidth = min(tableContentWidth, maxMainWidth)
	}
	merchantW := max(6, tableContentWidth-fixedColumnsWidth)
	chartSpendForCard := m.transactionsCategorySpend
	chartCursorInWindow := m.transactionsChartCursor
	if m.transactionsViewMode == transactionsViewModeChart {
		startIdx := max(0, min(m.transactionsChartOffset, max(0, len(m.transactionsCategorySpend)-1)))
		endIdx := min(len(m.transactionsCategorySpend), startIdx+m.transactionsChartVisibleRows())
		if endIdx < startIdx {
			endIdx = startIdx
		}
		chartSpendForCard = m.transactionsCategorySpend[startIdx:endIdx]
		chartCursorInWindow = m.transactionsChartCursor - startIdx
	}
	chartShowAmount := !hasChartPane
	timeSeriesCategoryLabel := ""
	timeSeriesColor := lipgloss.Color("#6CBFE6")
	timeSeriesForCard := m.transactionsTimeSeries
	timeSeriesSelectedLocal := -1
	if m.transactionsViewMode == transactionsViewModeTimeSeries {
		startIdx, endIdx := m.transactionsTimeSeriesBounds(len(m.transactionsTimeSeries))
		if endIdx < startIdx {
			endIdx = startIdx
		}
		timeSeriesForCard = m.transactionsTimeSeries[startIdx:endIdx]
		if len(m.transactionsTimeSeries) > 0 && len(timeSeriesForCard) > 0 {
			selectedAbs := m.transactionsTimeSeriesSelection
			if selectedAbs < 0 || selectedAbs >= len(m.transactionsTimeSeries) {
				selectedAbs = len(m.transactionsTimeSeries) - 1
			}
			if selectedAbs < startIdx {
				selectedAbs = startIdx
			}
			if selectedAbs >= endIdx {
				selectedAbs = endIdx - 1
			}
			timeSeriesSelectedLocal = selectedAbs - startIdx
		}
	}
	if selectedCategory := strings.TrimSpace(m.transactionsTimeSeriesCategory); selectedCategory != "" {
		timeSeriesCategoryLabel = selectedCategory
		for i := range m.transactionsCategorySpend {
			category := strings.TrimSpace(m.transactionsCategorySpend[i].category)
			if strings.EqualFold(category, selectedCategory) {
				timeSeriesCategoryLabel = category
				timeSeriesColor = transactionsCategoryColor(i)
				break
			}
		}
	}
	tableLines := renderTransactionsBodyLines(
		m.transactionsViewMode,
		m.transactionsRows,
		chartSpendForCard,
		timeSeriesForCard,
		timeSeriesCategoryLabel,
		timeSeriesColor,
		timeSeriesSelectedLocal,
		m.transactionsCursor,
		merchantW,
		tableContentWidth,
		chartCursorInWindow,
		chartShowAmount,
	)
	timeSeriesCardExtraHeight := 0
	if m.transactionsViewMode == transactionsViewModeTimeSeries {
		// Match chart-view card+search combined height using measured search box height.
		searchShell := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#6CBFE6")).
			Padding(0, 1).
			Width(tableContentWidth).
			Render(" ")
		timeSeriesCardExtraHeight = 1 + lipgloss.Height(searchShell)
	}
	tableBodyHeight := m.transactionsChartVisibleRows() + 1 + timeSeriesCardExtraHeight
	tableLines = padTransactionsBodyLines(tableLines, tableBodyHeight)
	table := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tableBorder).
		Padding(0, 1).
		Height(tableBodyHeight).
		Width(tableContentWidth).
		Render(strings.Join(tableLines, "\n"))
	tableOuterWidth := lipgloss.Width(table)
	viewModeLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		Width(tableOuterWidth).
		Align(lipgloss.Center).
		Render(renderTransactionsViewModeSelector(m.transactionsViewMode))
	sortLineLabel := "dates: " + rangeLabel
	if m.transactionsViewMode == transactionsViewModeTable {
		sortLineLabel = "sort: " + sortLabel + "  |  " + sortLineLabel
	}
	sortLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		Width(tableOuterWidth).
		Align(lipgloss.Center).
		Render(sortLineLabel)
	viewModeHeader := lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, viewModeLine)
	sortHeader := lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, sortLine)

	start := 0
	end := 0
	if len(m.transactionsRows) > 0 {
		start = m.transactionsPage*m.transactionsPageSize + 1
		end = start + len(m.transactionsRows) - 1
	}
	totalPages := 0
	if m.transactionsPageSize > 0 && m.transactionsTotal > 0 {
		totalPages = (m.transactionsTotal-1)/m.transactionsPageSize + 1
	}
	showSearchHelp := isTransactionsSearchHelpQuery(m.transactionsSearchApplied)
	footer := []string{}
	if showSearchHelp {
		footer = []string{
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Width(tableOuterWidth).Render("Search format: field: value + field: value"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Width(tableOuterWidth).Render("Example 1: merchant: WOOL + amount: >60"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Width(tableOuterWidth).Render("Example 2: category: groceries + type: -ve"),
			"",
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Width(tableOuterWidth).Render("merchant: case-insensitive match on merchant text"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Width(tableOuterWidth).Render("description: case-insensitive match on description"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Width(tableOuterWidth).Render("category: case-insensitive match on category id"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Width(tableOuterWidth).Render("exclude-category: exclude matches (repeat key or append + term)"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Width(tableOuterWidth).Render("amount: numeric compare, e.g. >60, <=12.50, =25"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Width(tableOuterWidth).Render("type: +ve (credits) or -ve (debits)"),
		}
	} else {
		if m.transactionsViewMode == transactionsViewModeTable {
			footer = []string{
				lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).
					Width(tableOuterWidth).
					Align(lipgloss.Center).
					Render(fmt.Sprintf("showing %d-%d/%d  |  page %d/%d", start, end, m.transactionsTotal, m.transactionsPage+1, max(1, totalPages))),
				lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).
					Width(tableOuterWidth).
					Align(lipgloss.Center).
					Render(chartFooterHelpText(m.transactionsViewMode)),
			}
		} else {
			footer = []string{
				lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).
					Width(tableOuterWidth).
					Align(lipgloss.Center).
					Render(chartFooterHelpText(m.transactionsViewMode)),
			}
		}
	}

	statusLines := []string{}
	if m.transactionsSyncing {
		statusLines = append(statusLines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			Render("syncing..."))
	}
	if m.transactionsFetched != nil {
		age := time.Since(m.transactionsFetched.UTC()).Round(time.Second)
		if age < 0 {
			age = 0
		}
		statusLines = append(statusLines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			Render(fmt.Sprintf("last updated %s ago", age.String())))
	}
	if strings.TrimSpace(m.transactionsDateErr) != "" {
		statusLines = append(statusLines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F15B5B")).
			Render(m.transactionsDateErr))
	}
	if strings.TrimSpace(m.transactionsErr) != "" {
		statusLines = append(statusLines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F15B5B")).
			Render("error: "+m.transactionsErr))
	}
	if strings.TrimSpace(m.transactionsSearchErr) != "" {
		statusLines = append(statusLines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F15B5B")).
			Render(m.transactionsSearchErr))
	}

	showSearchBar := m.transactionsViewMode != transactionsViewModeTimeSeries
	searchInput := m.transactionsSearchInput
	if hasChartPane || !showSearchBar {
		searchInput.SetValue("")
		searchInput.Placeholder = ""
	}
	searchInput.Width = max(6, tableContentWidth)
	searchBorder := lipgloss.Color("#6CBFE6")
	if m.transactionsSearchActive {
		searchBorder = lipgloss.Color("#FFD54A")
	}
	searchBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(searchBorder).
		Padding(0, 1).
		Width(tableContentWidth).
		Render(searchInput.View())

	headerBlock := strings.Join([]string{viewModeHeader, sortHeader}, "\n")
	leftTop := table
	footerBlock := strings.Join(footer, "\n")
	statusBlock := ""
	if len(statusLines) > 0 {
		centeredStatus := make([]string, 0, len(statusLines))
		for _, line := range statusLines {
			centeredStatus = append(centeredStatus, lipgloss.NewStyle().Width(tableOuterWidth).Align(lipgloss.Center).Render(line))
		}
		statusBlock = strings.Join(centeredStatus, "\n")
	}

	if hasChartPane {
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true)
		titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true)
		chartPaneSorts := transactionsCategoryTransactionSortOptions()
		chartPaneSortLabel := ""
		if len(chartPaneSorts) > 0 {
			sortIdx := m.transactionsChartPaneSortIdx
			if sortIdx < 0 || sortIdx >= len(chartPaneSorts) {
				sortIdx = 0
			}
			chartPaneSortLabel = chartPaneSorts[sortIdx].label
		}
		if strings.TrimSpace(chartPaneSortLabel) == "" {
			chartPaneSortLabel = "amount ↑"
		}

		paneOuterHeight := tableBodyHeight + 2
		// Match main chart card vertical alignment: no top padding so titles share the same row.
		paneInnerHeight := max(1, paneOuterHeight-2)
		amountWidth := 9
		merchantWidth := min(15, max(6, paneWidth-(amountWidth+6)))

		paneLines := []string{}
		if m.transactionsChartPaneMode == transactionsChartPaneModeDetails {
			paneLines = append(paneLines, titleStyle.Render("transaction details"))
			selectedIdx := m.transactionsChartPaneCursor
			if strings.TrimSpace(m.transactionsChartPaneDetailTxID) != "" {
				if idx := findCategoryTransactionRowIndex(m.transactionsChartPaneRows, m.transactionsChartPaneDetailTxID); idx >= 0 {
					selectedIdx = idx
				}
			}
			if selectedIdx < 0 || selectedIdx >= len(m.transactionsChartPaneRows) {
				paneLines = append(paneLines, labelStyle.Render("transaction not found"))
			} else {
				selected := m.transactionsChartPaneRows[selectedIdx]
				valueWidth := max(10, paneWidth-16)
				paneLines = append(paneLines, renderDetailLines("account", selected.accountName, valueWidth, labelStyle, valueStyle)...)
				paneLines = append(paneLines, renderDetailLines("time", formatTransactionTime(selected.createdAt), valueWidth, labelStyle, valueStyle)...)
				paneLines = append(paneLines, renderDetailLines("category", selected.categoryID, valueWidth, labelStyle, valueStyle)...)
				paneLines = append(paneLines, renderDetailLines("raw text", selected.rawText, valueWidth, labelStyle, valueStyle)...)
				paneLines = append(paneLines, renderDetailLines("status", selected.status, valueWidth, labelStyle, valueStyle)...)
				paneLines = append(paneLines, renderDetailLines("message", selected.message, valueWidth, labelStyle, valueStyle)...)
				paneLines = append(paneLines, renderDetailLines("description", selected.description, valueWidth, labelStyle, valueStyle)...)
				paneLines = append(paneLines, renderDetailLines("merchant", selected.merchant, valueWidth, labelStyle, valueStyle)...)
				paneLines = append(paneLines, renderDetailLines("card method", selected.cardMethod, valueWidth, labelStyle, valueStyle)...)
				paneLines = append(paneLines, renderDetailLines("note text", selected.noteText, valueWidth, labelStyle, valueStyle)...)
			}
			paneLines = padTransactionsBodyLines(paneLines, paneInnerHeight)
		} else {
			paneLines = make([]string, paneInnerHeight)
			if paneInnerHeight > 0 {
				paneLines[0] = titleStyle.Render("category transactions")
			}
			if paneInnerHeight > 1 {
				paneLines[1] = labelStyle.Render(fmt.Sprintf("  %-"+strconv.Itoa(amountWidth)+"s %-"+strconv.Itoa(merchantWidth)+"s", "amount", "merchant"))
			}
			sortRow := paneInnerHeight - 1
			if sortRow >= 0 {
				paneLines[sortRow] = labelStyle.Render("sort: " + chartPaneSortLabel)
			}

			listStartRow := 2
			listEndRow := max(listStartRow, sortRow-1) // keep one blank row above sort at the bottom
			availableRows := max(0, listEndRow-listStartRow)
			txVisible := min(m.transactionsChartPaneVisibleRowsForInnerHeight(paneInnerHeight), availableRows)
			if txVisible > 0 {
				start := max(0, min(m.transactionsChartPaneOffset, len(m.transactionsChartPaneRows)))
				end := min(len(m.transactionsChartPaneRows), start+txVisible)
				if start >= end {
					paneLines[listStartRow] = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("no transactions")
				} else {
					for i := start; i < end; i++ {
						row := m.transactionsChartPaneRows[i]
						prefix := " "
						if i == m.transactionsChartPaneCursor {
							prefix = "›"
						}
						merchant := strings.TrimSpace(row.merchant)
						if merchant == "" {
							merchant = strings.TrimSpace(row.description)
						}
						merchant = truncateDisplayWidth(merchant, merchantWidth)
						line := fmt.Sprintf("%s %-"+strconv.Itoa(amountWidth)+"s %-"+strconv.Itoa(merchantWidth)+"s", prefix, row.amountValue, merchant)
						line = truncateDisplayWidth(line, max(8, paneWidth))
						style := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
						if i == m.transactionsChartPaneCursor {
							style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
						}
						targetRow := listStartRow + (i - start)
						if targetRow >= 0 && targetRow < len(paneLines) {
							paneLines[targetRow] = style.Render(line)
						}
					}
				}
			}
		}

		pane := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(chartPaneBorder).
			Padding(0, 1).
			Height(paneInnerHeight).
			Width(paneWidth).
			Render(strings.Join(paneLines, "\n"))

		cardsRow := lipgloss.JoinHorizontal(lipgloss.Top, table, strings.Repeat(" ", gapWidth), pane)
		cardsRow = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, cardsRow)
		footerRow := lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, footerBlock)
		bodyLines := []string{viewModeHeader, sortHeader, "", cardsRow, "", footerRow}
		if statusBlock != "" {
			bodyLines = append(bodyLines, "", lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, statusBlock))
		}
		return strings.Join([]string{title, "", strings.Join(bodyLines, "\n")}, "\n")
	}

	hasTableDetailsPane := m.transactionsViewMode == transactionsViewModeTable &&
		m.transactionsPaneOpen &&
		len(m.transactionsRows) > 0 &&
		m.transactionsCursor >= 0 &&
		m.transactionsCursor < len(m.transactionsRows)
	hasTimeSeriesDetailsPane := m.transactionsViewMode == transactionsViewModeTimeSeries &&
		m.transactionsPaneOpen &&
		len(m.transactionsTimeSeries) > 0
	hasPane := hasTableDetailsPane || hasTimeSeriesDetailsPane
	pane := ""
	leftBeforeFooter := leftTop
	if showSearchBar {
		leftBeforeFooter = strings.Join([]string{leftTop, "", searchBox}, "\n")
	}
	if hasTableDetailsPane {
		selected := m.transactionsRows[m.transactionsCursor]
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true)
		paneLines := []string{lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true).Render("transaction details")}
		valueWidth := max(10, paneWidth-16)
		paneLines = append(paneLines, renderDetailLines("account", selected.accountName, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("time", formatTransactionTime(selected.createdAt), valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("category", selected.categoryID, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("raw text", selected.rawText, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("status", selected.status, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("message", selected.message, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("description", selected.description, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("merchant", selected.merchant, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("card method", selected.cardMethod, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("note text", selected.noteText, valueWidth, labelStyle, valueStyle)...)
		paneInnerHeight := max(1, lipgloss.Height(leftBeforeFooter)-2)
		paneLines = padTransactionsBodyLines(paneLines, paneInnerHeight)

		pane = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFD54A")).
			Padding(0, 1).
			Height(paneInnerHeight).
			Width(paneWidth).
			Render(strings.Join(paneLines, "\n"))
	} else if hasTimeSeriesDetailsPane {
		selectedIdx := m.transactionsTimeSeriesSelection
		if selectedIdx < 0 || selectedIdx >= len(m.transactionsTimeSeries) {
			selectedIdx = len(m.transactionsTimeSeries) - 1
		}
		selected := m.transactionsTimeSeries[selectedIdx]
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true)
		paneLines := []string{lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true).Render("transaction details")}
		valueWidth := max(10, paneWidth-16)
		paneLines = append(paneLines, renderDetailLines("account", selected.accountName, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("time", formatTransactionTime(selected.createdAt), valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("category", selected.categoryID, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("raw text", selected.rawText, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("status", selected.status, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("message", selected.message, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("description", selected.description, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("merchant", selected.merchant, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("card method", selected.cardMethod, valueWidth, labelStyle, valueStyle)...)
		paneLines = append(paneLines, renderDetailLines("note text", selected.noteText, valueWidth, labelStyle, valueStyle)...)
		paneInnerHeight := max(1, lipgloss.Height(leftBeforeFooter)-2)
		paneLines = padTransactionsBodyLines(paneLines, paneInnerHeight)

		pane = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFD54A")).
			Padding(0, 1).
			Height(paneInnerHeight).
			Width(paneWidth).
			Render(strings.Join(paneLines, "\n"))
	}
	footerRow := lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, footerBlock)
	if !hasPane {
		leftContent := lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, leftBeforeFooter)
		bodyLines := []string{leftContent, "", footerRow}
		if statusBlock != "" {
			bodyLines = append(bodyLines, "", lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, statusBlock))
		}
		return strings.Join([]string{title, "", headerBlock, "", strings.Join(bodyLines, "\n")}, "\n")
	}

	cardsRow := lipgloss.JoinHorizontal(lipgloss.Top, leftBeforeFooter, strings.Repeat(" ", gapWidth), pane)
	cardsRow = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, cardsRow)
	bodyLines := []string{cardsRow, "", footerRow}
	if statusBlock != "" {
		bodyLines = append(bodyLines, "", lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, statusBlock))
	}
	return strings.Join([]string{title, "", headerBlock, "", strings.Join(bodyLines, "\n")}, "\n")
}

func (m model) renderTransactionsFiltersScreen(layoutWidth int) string {
	title := renderTransactionsTitle()
	title = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, title)

	isQuick := m.transactionsFilterMode == transactionsFilterModeQuick
	isCustom := m.transactionsFilterMode == transactionsFilterModeCustom

	dateBorderBase := lipgloss.Color("#5CCB76")
	quickBorderBase := lipgloss.Color("#5CCB76")
	dateLabelColor := lipgloss.Color("#D1D5DB")
	quickLabelColor := lipgloss.Color("#D1D5DB")
	if isQuick {
		dateBorderBase = lipgloss.Color("#4B5563")
		dateLabelColor = lipgloss.Color("#6B7280")
	}
	if isCustom {
		quickBorderBase = lipgloss.Color("#4B5563")
		quickLabelColor = lipgloss.Color("#6B7280")
	}

	fromBorder := dateBorderBase
	toBorder := dateBorderBase
	quickBorder := quickBorderBase
	includeBorder := lipgloss.Color("#FFFFFF")
	if m.transactionsFocus == transactionsFocusFromDate {
		fromBorder = lipgloss.Color("#FFD54A")
	}
	if m.transactionsFocus == transactionsFocusToDate {
		toBorder = lipgloss.Color("#FFD54A")
	}
	if m.transactionsFocus == transactionsFocusQuickRange {
		quickBorder = lipgloss.Color("#FFD54A")
	}
	if m.transactionsFocus == transactionsFocusIncludeInternal {
		includeBorder = lipgloss.Color("#FFD54A")
	}

	fromField := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(fromBorder).Padding(0, 1).Render(renderDateMask(m.transactionsFromDate))
	toField := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(toBorder).Padding(0, 1).Render(renderDateMask(m.transactionsToDate))

	ranges := transactionsQuickRanges()
	rangeParts := make([]string, 0, len(ranges))
	for i, r := range ranges {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		if isQuick {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		}
		if i == m.transactionsQuickIdx {
			if isQuick {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
			} else {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Bold(true)
			}
		}
		rangeParts = append(rangeParts, style.Render(r.label))
	}
	rangeField := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(quickBorder).
		Padding(0, 1).
		Render(strings.Join(rangeParts, "  "))

	switchOff := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("off")
	switchOn := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("on")
	if m.transactionsIncludeInternal {
		switchOn = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render("on")
	} else {
		switchOff = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render("off")
	}
	includeSwitch := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(includeBorder).
		Padding(0, 1).
		Render(switchOff + "  |  " + switchOn)

	dateLabel := lipgloss.NewStyle().Foreground(dateLabelColor).Bold(true).Render("custom range")
	dateFields := lipgloss.JoinHorizontal(
		lipgloss.Center,
		fromField,
		lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render(" - "),
		toField,
	)

	modeValue := lipgloss.NewStyle().Foreground(lipgloss.Color("#5CCB76")).Bold(true).Render("quick range")
	if isCustom {
		modeValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#5CCB76")).Bold(true).Render("custom range")
	}
	modeLine := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true).Render("mode: ") + modeValue

	lines := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true).Render("filters"),
		"",
		modeLine,
		"",
		dateLabel,
		dateFields,
		"",
		lipgloss.NewStyle().Foreground(quickLabelColor).Bold(true).Render("quick range"),
		rangeField,
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true).Render("include internal transfers"),
		includeSwitch,
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("tab switch field  ←/→ change value"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("type date or c calendar  enter save/apply  esc back"),
	}
	if strings.TrimSpace(m.transactionsDateErr) != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("#F15B5B")).Render(m.transactionsDateErr))
	}
	panel := lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(lines, "\n"))
	panel = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, panel)
	content := strings.Join([]string{title, "", panel}, "\n")
	if !m.transactionsCalendarOpen {
		return content
	}
	overlay := renderTransactionsCalendarOverlay(
		m.transactionsCalendarMonth,
		m.transactionsCalendarCursor,
		m.transactionsCalendarTarget == transactionsFocusFromDate,
	)
	overlay = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, overlay)
	return strings.Join([]string{content, "", overlay}, "\n")
}

func transactionsSortOptions() []transactionSortOption {
	return []transactionSortOption{
		{label: "date ↓", orderBy: "t.created_at DESC, t.id DESC"},
		{label: "date ↑", orderBy: "t.created_at ASC, t.id ASC"},
		{label: "merchant A-Z", orderBy: "COALESCE(t.merchant_norm, COALESCE(t.raw_text_norm, t.description_norm, t.raw_text, t.description, '')) ASC, t.created_at DESC, t.id DESC"},
		{label: "merchant Z-A", orderBy: "COALESCE(t.merchant_norm, COALESCE(t.raw_text_norm, t.description_norm, t.raw_text, t.description, '')) DESC, t.created_at DESC, t.id DESC"},
		{label: "amount ↓", orderBy: "t.amount_value_in_base_units DESC, t.created_at DESC, t.id DESC"},
		{label: "amount ↑", orderBy: "t.amount_value_in_base_units ASC, t.created_at DESC, t.id DESC"},
	}
}

func transactionsCategoryTransactionSortOptions() []transactionSortOption {
	return []transactionSortOption{
		{label: "amount ↑", orderBy: "t.amount_value_in_base_units ASC, t.created_at DESC, t.id DESC"},
		{label: "amount ↓", orderBy: "t.amount_value_in_base_units DESC, t.created_at DESC, t.id DESC"},
		{label: "merchant A-Z", orderBy: "merchant ASC, t.amount_value_in_base_units ASC, t.created_at DESC, t.id DESC"},
		{label: "merchant Z-A", orderBy: "merchant DESC, t.amount_value_in_base_units ASC, t.created_at DESC, t.id DESC"},
		{label: "date ↓", orderBy: "t.created_at DESC, t.id DESC"},
		{label: "date ↑", orderBy: "t.created_at ASC, t.id ASC"},
	}
}

func transactionsQuickRanges() []transactionQuickRange {
	return []transactionQuickRange{
		{
			label: "today",
			apply: func(now time.Time) (time.Time, time.Time) { return now, now },
		},
		{
			label: "this week",
			apply: func(now time.Time) (time.Time, time.Time) {
				weekday := int(now.Weekday())
				if weekday == 0 {
					weekday = 7
				}
				from := now.AddDate(0, 0, -(weekday - 1))
				return from, now
			},
		},
		{
			label: "3m",
			apply: func(now time.Time) (time.Time, time.Time) { return now.AddDate(0, -3, 0), now },
		},
		{
			label: "6m",
			apply: func(now time.Time) (time.Time, time.Time) { return now.AddDate(0, -6, 0), now },
		},
		{
			label: "1y",
			apply: func(now time.Time) (time.Time, time.Time) { return now.AddDate(-1, 0, 0), now },
		},
		{
			label: "all",
			apply: func(now time.Time) (time.Time, time.Time) { return time.Time{}, time.Time{} },
		},
	}
}

func (m *model) applyTransactionsQuickRange(idx int) {
	ranges := transactionsQuickRanges()
	if idx < 0 || idx >= len(ranges) {
		idx = 0
	}
	m.transactionsQuickIdx = idx
	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	from, to := ranges[idx].apply(today)
	if from.IsZero() && to.IsZero() {
		m.transactionsFromDate = ""
		m.transactionsToDate = ""
		return
	}
	m.transactionsFromDate = fmt.Sprintf("%04d%02d%02d", from.Year(), int(from.Month()), from.Day())
	m.transactionsToDate = fmt.Sprintf("%04d%02d%02d", to.Year(), int(to.Month()), to.Day())
}

func appendDateDigit(raw string, d rune) string {
	if len(raw) >= 8 {
		return string(d)
	}
	return raw + string(d)
}

func backspaceDateDigit(raw string) string {
	if len(raw) == 0 {
		return raw
	}
	return raw[:len(raw)-1]
}

func validateTransactionsDateRange(fromDigits, toDigits string) error {
	if strings.TrimSpace(fromDigits) != "" {
		if _, err := parseTransactionsDateDigits(fromDigits); err != nil {
			return fmt.Errorf("invalid from date: %w", err)
		}
	}
	if strings.TrimSpace(toDigits) != "" {
		if _, err := parseTransactionsDateDigits(toDigits); err != nil {
			return fmt.Errorf("invalid to date: %w", err)
		}
	}
	if strings.TrimSpace(fromDigits) != "" && strings.TrimSpace(toDigits) != "" {
		from, _ := parseTransactionsDateDigits(fromDigits)
		to, _ := parseTransactionsDateDigits(toDigits)
		if from > to {
			return fmt.Errorf("from date cannot be after to date")
		}
	}
	return nil
}

func parseTransactionsDateDigits(digits string) (string, error) {
	if len(digits) != 8 {
		return "", fmt.Errorf("date must be YYYY / MM / DD")
	}
	year, err := strconv.Atoi(digits[0:4])
	if err != nil || year < 1900 || year > 9999 {
		return "", fmt.Errorf("year must be 1900-9999")
	}
	month, err := strconv.Atoi(digits[4:6])
	if err != nil || month < 1 || month > 12 {
		return "", fmt.Errorf("month must be 01-12")
	}
	day, err := strconv.Atoi(digits[6:8])
	if err != nil || day < 1 || day > 31 {
		return "", fmt.Errorf("day must be 01-31")
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
	if date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return "", fmt.Errorf("date is not valid in calendar")
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day), nil
}

func formatTransactionDate(raw string) string {
	ts := strings.TrimSpace(raw)
	if ts == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		if len(ts) >= 10 {
			return ts[:10]
		}
		return ts
	}
	return t.In(time.Local).Format("2006-01-02")
}

func formatTransactionTime(raw string) string {
	ts := strings.TrimSpace(raw)
	if ts == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err == nil {
		return t.In(time.Local).Format("15:04")
	}
	if parsed, err2 := time.ParseInLocation("2006-01-02T15:04:05", ts, time.Local); err2 == nil {
		return parsed.Format("15:04")
	}
	if parsed, err2 := time.ParseInLocation("2006-01-02 15:04:05", ts, time.Local); err2 == nil {
		return parsed.Format("15:04")
	}
	if idx := strings.Index(ts, "T"); idx >= 0 && idx+6 <= len(ts) {
		return ts[idx+1 : idx+6]
	}
	if len(ts) >= 5 {
		return ts[:5]
	}
	return ts
}

func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(r[:maxLen])
	}
	return string(r[:maxLen-3]) + "..."
}

func truncateDisplayWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	if maxWidth <= 3 {
		return strings.Repeat(".", maxWidth)
	}
	ellipsis := "..."
	r := []rune(s)
	out := make([]rune, 0, len(r))
	for _, ch := range r {
		candidate := string(append(out, ch))
		if lipgloss.Width(candidate)+lipgloss.Width(ellipsis) > maxWidth {
			break
		}
		out = append(out, ch)
	}
	if len(out) == 0 {
		return ellipsis
	}
	return string(out) + ellipsis
}

func emptyDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func renderDetailLines(label string, value string, width int, labelStyle lipgloss.Style, valueStyle lipgloss.Style) []string {
	v := truncateRunes(emptyDash(value), 50)
	segments := wrapRunes(v, width)
	if len(segments) == 0 {
		segments = []string{"-"}
	}
	lines := []string{
		labelStyle.Render(label+": ") + valueStyle.Render(segments[0]),
	}
	indent := strings.Repeat(" ", len(label)+2)
	for _, seg := range segments[1:] {
		lines = append(lines, labelStyle.Render(indent)+valueStyle.Render(seg))
	}
	return lines
}

func wrapRunes(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	r := []rune(s)
	if len(r) == 0 {
		return nil
	}
	lines := make([]string, 0, (len(r)/width)+1)
	for len(r) > width {
		lines = append(lines, string(r[:width]))
		r = r[width:]
	}
	if len(r) > 0 {
		lines = append(lines, string(r))
	}
	return lines
}

func transactionsRangeLabel(fromDigits, toDigits string) string {
	from := transactionsDateForDisplay(fromDigits)
	to := transactionsDateForDisplay(toDigits)
	if from == "" && to == "" {
		return "all time"
	}
	if from == "" {
		return "until " + to
	}
	if to == "" {
		return "from " + from
	}
	return from + " to " + to
}

func transactionsDateForDisplay(digits string) string {
	if len(strings.TrimSpace(digits)) != 8 {
		return ""
	}
	v, err := parseTransactionsDateDigits(digits)
	if err != nil {
		return ""
	}
	return v
}

func renderTransactionsCalendarOverlay(month time.Time, selected time.Time, isFrom bool) string {
	title := "calendar (to)"
	if isFrom {
		title = "calendar (from)"
	}
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#87CEEB")).Bold(true)
	header := titleStyle.Render(title + "  " + month.Format("January 2006"))

	weekHeader := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("Mo Tu We Th Fr Sa Su")
	first := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.Local)
	startOffset := int(first.Weekday())
	if startOffset == 0 {
		startOffset = 7
	}
	start := first.AddDate(0, 0, -(startOffset - 1))

	lines := []string{header, "", weekHeader}
	day := start
	for w := 0; w < 6; w++ {
		cells := make([]string, 0, 7)
		for d := 0; d < 7; d++ {
			cell := fmt.Sprintf("%2d", day.Day())
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
			if day.Month() == month.Month() {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
			}
			if day.Year() == selected.Year() && day.Month() == selected.Month() && day.Day() == selected.Day() {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).
					Background(lipgloss.Color("#2D3748"))
			}
			cells = append(cells, style.Render(cell))
			day = day.AddDate(0, 0, 1)
		}
		lines = append(lines, strings.Join(cells, " "))
	}
	lines = append(lines, "", lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("←/→/↑/↓ move  enter select  esc close"))
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Render("shift+←/→ month  shift+↑/↓ year"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFFFFF")).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

func calendarAnchorFromPartial(raw string) (time.Time, bool) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return time.Time{}, false
	}
	// Accept typed variants like YYYY, YYYYMM, YYYY-MM, YYYY/MM, YYYYMMDD.
	clean = strings.ReplaceAll(clean, "-", "")
	clean = strings.ReplaceAll(clean, "/", "")
	clean = strings.ReplaceAll(clean, " ", "")
	if len(clean) < 4 {
		return time.Time{}, false
	}
	for _, ch := range clean {
		if ch < '0' || ch > '9' {
			return time.Time{}, false
		}
	}
	year, err := strconv.Atoi(clean[:4])
	if err != nil || year < 1900 || year > 9999 {
		return time.Time{}, false
	}
	month := 1
	day := 1
	if len(clean) >= 6 {
		month, err = strconv.Atoi(clean[4:6])
		if err != nil || month < 1 || month > 12 {
			return time.Time{}, false
		}
	}
	if len(clean) >= 8 {
		day, err = strconv.Atoi(clean[6:8])
		if err != nil || day < 1 || day > 31 {
			return time.Time{}, false
		}
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		// For YYYY or YYYYMM, use first day of month.
		if len(clean) < 8 {
			return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local), true
		}
		return time.Time{}, false
	}
	return t, true
}

func shiftCalendarByMonths(current time.Time, delta int) time.Time {
	y, m, d := current.Date()
	target := time.Date(y, m, 1, 0, 0, 0, 0, time.Local).AddDate(0, delta, 0)
	lastDay := daysInMonth(target.Year(), target.Month())
	if d > lastDay {
		d = lastDay
	}
	return time.Date(target.Year(), target.Month(), d, 0, 0, 0, 0, time.Local)
}

func shiftCalendarByYears(current time.Time, delta int) time.Time {
	y, m, d := current.Date()
	targetYear := y + delta
	lastDay := daysInMonth(targetYear, m)
	if d > lastDay {
		d = lastDay
	}
	return time.Date(targetYear, m, d, 0, 0, 0, 0, time.Local)
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
}
