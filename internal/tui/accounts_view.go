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

func renderAccountsTitle() string {
	raw := []string{
		"▄▀█ █▀▀ █▀▀ █▀█ █ █ █▄ █ ▀█▀ █▀",
		"█▀█ █▄▄ █▄▄ █▄█ █▄█ █ ▀█  █  ▄█",
		"▀ ▀ ▀▀▀ ▀▀▀ ▀▀▀ ▀▀▀ ▀  ▀  ▀  ▀▀",
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

func (m model) renderAccountsScreen(layoutWidth int) string {
	title := renderAccountsTitle()
	title = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, title)

	if strings.TrimSpace(m.accountsErr) != "" {
		body := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F15B5B")).
			Render("error: " + m.accountsErr)
		body = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, body)
		return strings.Join([]string{title, "", body}, "\n")
	}
	if m.accountsLoading && len(m.accountsRows) == 0 {
		body := m.renderAccountsSkeletonCards(layoutWidth)
		return strings.Join([]string{title, "", body}, "\n")
	}
	if len(m.accountsRows) == 0 {
		body := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B9B4D0")).
			Render("no accounts found")
		body = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, body)
		return strings.Join([]string{title, "", body}, "\n")
	}

	cardWidth := min(layoutWidth-20, 56)
	cardWidth = max(32, cardWidth)
	visibleRows := m.accountsVisibleRows()
	start := max(0, min(m.accountsOffset, max(0, len(m.accountsRows)-1)))
	end := min(len(m.accountsRows), start+visibleRows)

	baseCard := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1).
		Width(cardWidth)
	selectedCard := baseCard.BorderForeground(lipgloss.Color("#FFD54A"))

	innerWidth := max(8, cardWidth-4)
	cards := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		row := m.accountsRows[i]

		display := row.displayName
		balance := "$" + strings.TrimSpace(row.balanceValue)
		if strings.TrimSpace(row.balanceValue) == "" {
			balance = "$0.00"
		}

		leftWidth := max(4, innerWidth-lipgloss.Width(balance)-1)
		left := lipgloss.NewStyle().
			Width(leftWidth).
			MaxWidth(leftWidth).
			Render(display)
		content := lipgloss.NewStyle().
			Width(innerWidth).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Render(left + " " + balance)
		card := baseCard
		if i == m.accountsCursor {
			card = selectedCard
		}
		cards = append(cards, card.Render(content))
	}

	body := strings.Join(cards, "\n")
	body = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, body)

	shownFrom := 0
	shownTo := 0
	if len(m.accountsRows) > 0 && end > start {
		shownFrom = start + 1
		shownTo = end
	}
	upArrow := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("↑")
	downArrow := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("↓")
	if start > 0 {
		upArrow = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true).Render("↑")
	}
	if end < len(m.accountsRows) {
		downArrow = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Bold(true).Render("↓")
	}
	statusLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		Render(fmt.Sprintf("showing %d-%d/%d   %s/%s to scroll", shownFrom, shownTo, len(m.accountsRows), upArrow, downArrow))
	statusLine = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, statusLine)

	totalLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#87CEEB")).
		Bold(true).
		Render("total " + formatTotalBalance(m.accountsRows))
	totalLine = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, totalLine)

	footer := ""
	if m.accountsFetched != nil {
		age := time.Since(m.accountsFetched.UTC()).Round(time.Second)
		if age < 0 {
			age = 0
		}
		staleTag := ""
		if age > 30*time.Second {
			staleTag = " (stale)"
		}
		footer = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			Render(fmt.Sprintf("last updated %s ago%s", age.String(), staleTag))
		footer = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, footer)
	}
	if footer == "" {
		return strings.Join([]string{title, "", body, "", statusLine, "", totalLine}, "\n")
	}
	return strings.Join([]string{title, "", body, "", statusLine, "", totalLine, "", footer}, "\n")
}

func (m model) loadAccountsPreviewCmd() tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return loadAccountsPreviewMsg{err: errors.New("database is not initialized")}
		}
		rows, fetchedAt, err := queryAccountsPreview(m.db)
		if err != nil {
			return loadAccountsPreviewMsg{err: err}
		}
		return loadAccountsPreviewMsg{rows: rows, lastFetchedAt: fetchedAt}
	}
}

func (m model) syncAndReloadAccountsPreviewCmd(force bool) tea.Cmd {
	return func() tea.Msg {
		if m.db == nil {
			return syncAccountsPreviewMsg{err: errors.New("database is not initialized")}
		}
		syncErr := syncAccountsIntoDB(m.db, force)
		rows, fetchedAt, queryErr := queryAccountsPreview(m.db)
		if queryErr != nil {
			return syncAccountsPreviewMsg{err: queryErr}
		}
		if syncErr != nil && len(rows) == 0 {
			return syncAccountsPreviewMsg{err: syncErr}
		}
		return syncAccountsPreviewMsg{rows: rows, lastFetchedAt: fetchedAt}
	}
}

func queryAccountsPreview(db *sql.DB) ([]accountPreviewRow, *time.Time, error) {
	rows, err := db.QueryContext(
		context.Background(),
		`SELECT id, display_name, balance_value, last_fetched_at
		 FROM accounts
		 WHERE is_active = 1
		 ORDER BY display_order ASC, display_name ASC, id ASC`,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := make([]accountPreviewRow, 0, 5)
	var newest *time.Time
	for rows.Next() {
		var row accountPreviewRow
		var fetchedAtRaw string
		if err := rows.Scan(&row.id, &row.displayName, &row.balanceValue, &fetchedAtRaw); err != nil {
			return nil, nil, err
		}
		if t, err := time.Parse(time.RFC3339Nano, fetchedAtRaw); err == nil {
			tt := t.UTC()
			if newest == nil || tt.After(*newest) {
				newest = &tt
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, newest, nil
}

func syncAccountsIntoDB(sqlDB *sql.DB, force bool) error {
	pat, err := auth.LoadPAT()
	if err != nil {
		return err
	}

	client := upapi.New(pat)
	service, err := syncer.NewAccountsService(sqlDB, client)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	defer service.LeaveView()

	repo := storage.NewSyncStateRepo(sqlDB)
	accountsRepo := storage.NewAccountsRepo(sqlDB)

	hasCachedRows, err := accountsRepo.HasActiveAccounts(ctx)
	if err != nil {
		return err
	}

	var prevAttempt *time.Time
	var prevSuccess *time.Time
	if state, found, err := repo.Get(ctx, syncer.CollectionAccounts); err == nil && found {
		if state.LastAttempt != nil {
			t := state.LastAttempt.UTC()
			prevAttempt = &t
		}
		if state.LastSuccess != nil {
			t := state.LastSuccess.UTC()
			prevSuccess = &t
		}
	}

	if err := service.EnterAccountsView(ctx); err != nil {
		return err
	}

	if force {
		if err := service.RefreshAccounts(); err != nil {
			return err
		}
		waitForRows := !hasCachedRows
		return waitForAccountsSyncResult(ctx, repo, accountsRepo, prevAttempt, prevSuccess, waitForRows)
	}

	isStale := prevSuccess == nil || time.Since(prevSuccess.UTC()) > 30*time.Second
	if hasCachedRows && !isStale {
		return nil
	}
	waitForRows := !hasCachedRows
	return waitForAccountsSyncResult(ctx, repo, accountsRepo, prevAttempt, prevSuccess, waitForRows)
}

func (m model) renderAccountsSkeletonCards(layoutWidth int) string {
	cardWidth := min(layoutWidth-20, 56)
	cardWidth = max(32, cardWidth)
	count := min(3, m.accountsVisibleRows())
	if count < 1 {
		count = 1
	}
	innerWidth := max(8, cardWidth-4)

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C7C7C")).
		Padding(0, 1).
		Width(cardWidth)
	lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))

	rows := make([]string, 0, count)
	for i := 0; i < count; i++ {
		placeholder := lineStyle.Render(strings.Repeat("·", innerWidth-2))
		rows = append(rows, cardStyle.Render(placeholder))
	}
	body := strings.Join(rows, "\n")
	return lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, body)
}

func waitForAccountsSyncResult(
	ctx context.Context,
	repo *storage.SyncStateRepo,
	accountsRepo *storage.AccountsRepo,
	previousAttempt *time.Time,
	previousSuccess *time.Time,
	waitForRows bool,
) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		if waitForRows {
			hasRows, err := accountsRepo.HasActiveAccounts(ctx)
			if err == nil && hasRows {
				return nil
			}
		}

		state, found, err := repo.Get(ctx, syncer.CollectionAccounts)
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
			return errors.New("accounts sync timed out")
		case <-ticker.C:
		}
	}
}

func renderStyledBlockTitle(raw []string, segments [][2]int) string {
	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("#5FA8FF")).Bold(true)
	coral := lipgloss.NewStyle().Foreground(lipgloss.Color("#F47A60")).Bold(true)
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD54A")).Bold(true)

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

func segmentRuns(line string) [][2]int {
	runes := []rune(line)
	segments := make([][2]int, 0, 8)
	start := -1
	for i, ch := range runes {
		if ch != ' ' {
			if start == -1 {
				start = i
			}
			continue
		}
		if start != -1 {
			segments = append(segments, [2]int{start, i - 1})
			start = -1
		}
	}
	if start != -1 {
		segments = append(segments, [2]int{start, len(runes) - 1})
	}
	if len(segments) == 0 {
		return [][2]int{{0, max(0, len(runes)-1)}}
	}
	return segments
}

func formatTotalBalance(rows []accountPreviewRow) string {
	if len(rows) == 0 {
		return "$0.00"
	}
	total := 0.0
	for _, row := range rows {
		v := strings.TrimSpace(row.balanceValue)
		if v == "" {
			continue
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			continue
		}
		total += n
	}
	if math.Abs(total) < 0.0000001 {
		total = 0
	}
	return fmt.Sprintf("$%.2f", total)
}

func moveAccountDisplayOrder(ctx context.Context, db *sql.DB, accountID string, delta int) error {
	if delta == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(
		ctx,
		`SELECT id
		 FROM accounts
		 WHERE is_active = 1
		 ORDER BY display_order ASC, display_name ASC, id ASC`,
	)
	if err != nil {
		return err
	}

	ids := make([]string, 0, 32)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(ids) == 0 {
		if err = tx.Commit(); err != nil {
			return err
		}
		return nil
	}

	// Keep contiguous order values for deterministic swaps.
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, "UPDATE accounts SET display_order = ? WHERE id = ?", i, id); err != nil {
			return err
		}
	}

	current := -1
	for i, id := range ids {
		if id == accountID {
			current = i
			break
		}
	}
	if current == -1 {
		if err = tx.Commit(); err != nil {
			return err
		}
		return nil
	}

	target := current + delta
	if target < 0 || target >= len(ids) {
		if err = tx.Commit(); err != nil {
			return err
		}
		return nil
	}

	ids[current], ids[target] = ids[target], ids[current]
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, "UPDATE accounts SET display_order = ? WHERE id = ?", i, id); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}
