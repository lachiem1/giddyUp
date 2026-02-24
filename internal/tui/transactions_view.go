package tui

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lachiem1/giddyUp/internal/auth"
	"github.com/lachiem1/giddyUp/internal/storage"
	"github.com/lachiem1/giddyUp/internal/syncer"
	"github.com/lachiem1/giddyUp/internal/upapi"
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
	return func() tea.Msg {
		if m.db == nil {
			return loadTransactionsPreviewMsg{err: fmt.Errorf("database is not initialized")}
		}
		rows, fetchedAt, err := queryTransactionsPreview(m.db)
		if err != nil {
			return loadTransactionsPreviewMsg{err: err}
		}
		return loadTransactionsPreviewMsg{rows: rows, lastFetchedAt: fetchedAt}
	}
}

func queryTransactionsPreview(db *sql.DB) ([]transactionPreviewRow, *time.Time, error) {
	rows, err := db.QueryContext(
		context.Background(),
		`SELECT id, COALESCE(raw_text, ''), description, amount_value
		 FROM transactions
		 ORDER BY created_at DESC, id DESC`,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := make([]transactionPreviewRow, 0, 64)
	for rows.Next() {
		var r transactionPreviewRow
		if err := rows.Scan(&r.id, &r.rawText, &r.description, &r.amountValue); err != nil {
			return nil, nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var lastSuccess *time.Time
	stateRepo := storage.NewSyncStateRepo(db)
	state, found, err := stateRepo.Get(context.Background(), syncer.CollectionTransactions)
	if err != nil {
		return nil, nil, err
	}
	if found && state.LastSuccess != nil {
		t := state.LastSuccess.UTC()
		lastSuccess = &t
	}

	return out, lastSuccess, nil
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

func (m model) renderTransactionsScreen(layoutWidth int) string {
	title := renderTransactionsTitle()
	title = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, title)

	statusLines := []string{}
	if strings.TrimSpace(m.transactionsErr) != "" {
		statusLines = append(statusLines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F15B5B")).
			Render("error: "+m.transactionsErr))
	}
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
	if len(statusLines) == 0 {
		return title
	}
	for i, line := range statusLines {
		statusLines[i] = lipgloss.PlaceHorizontal(layoutWidth, lipgloss.Center, line)
	}
	return strings.Join([]string{title, "", strings.Join(statusLines, "\n")}, "\n")
}
