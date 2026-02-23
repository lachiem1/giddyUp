//go:build integration
// +build integration

package syncer

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/lachiem1/giddyUp/internal/storage"
	"github.com/lachiem1/giddyUp/internal/upapi"
)

func TestEnterAccountsViewSyncsPaginatedListAndAccountDetails(t *testing.T) {
	server := newAccountsStubServer(t)
	defer server.Close()

	db := openTestDB(t)
	defer db.Close()
	createSyncTables(t, db)

	client := upapi.NewWithBaseURL("test-token", server.URL())
	accountsRepo := storage.NewAccountsRepo(db)
	syncStateRepo := storage.NewSyncStateRepo(db)
	accountsSyncer := NewAccountsSyncer(client, accountsRepo, syncStateRepo, 4)

	engine, err := New(
		Config{
			StaleTTL:     30 * time.Second,
			PollInterval: time.Hour,
			Backoff:      []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 60 * time.Second},
		},
		[]Syncer{accountsSyncer},
		nil,
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	svc := NewService(engine)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer svc.LeaveView()

	if err := svc.EnterAccountsView(ctx); err != nil {
		t.Fatalf("EnterAccountsView() unexpected error: %v", err)
	}

	waitForCondition(t, 5*time.Second, func() bool {
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE is_active = 1`).Scan(&count)
		return err == nil && count == 3
	})

	assertAccountsPersisted(t, db)
	server.Assert(t)

	state, found, err := syncStateRepo.Get(context.Background(), CollectionAccounts)
	if err != nil {
		t.Fatalf("sync state get error: %v", err)
	}
	if !found {
		t.Fatal("sync state not found for accounts")
	}
	if state.LastSuccess == nil {
		t.Fatal("sync state last_success_at is nil")
	}
}

type accountsStubServer struct {
	server *httptest.Server

	mu               sync.Mutex
	listFirstHits    int
	listSecondHits   int
	accountIDRequest map[string]int
}

func newAccountsStubServer(t *testing.T) *accountsStubServer {
	t.Helper()

	s := &accountsStubServer{accountIDRequest: make(map[string]int)}
	mux := http.NewServeMux()

	mux.HandleFunc("/accounts", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		after := q.Get("page[after]")
		if after == "" {
			if q.Get("page[size]") != "50" {
				t.Fatalf("page[size] = %q, want 50", q.Get("page[size]"))
			}
			s.mu.Lock()
			s.listFirstHits++
			s.mu.Unlock()
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{
					{"type": "accounts", "id": "acc-1"},
					{"type": "accounts", "id": "acc-2"},
				},
				"links": map[string]any{
					"prev": nil,
					"next": "/accounts?page[after]=cursor-2",
				},
			})
			return
		}

		if after == "cursor-2" {
			s.mu.Lock()
			s.listSecondHits++
			s.mu.Unlock()
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{
					{"type": "accounts", "id": "acc-3"},
				},
				"links": map[string]any{
					"prev": nil,
					"next": nil,
				},
			})
			return
		}

		http.NotFound(w, r)
	})

	mux.HandleFunc("/accounts/", func(w http.ResponseWriter, r *http.Request) {
		id := filepath.Base(r.URL.Path)
		s.mu.Lock()
		s.accountIDRequest[id]++
		s.mu.Unlock()

		createdAt := "2026-02-17T12:13:27+11:00"
		baseUnits := int64(100)
		display := "Spending"
		switch id {
		case "acc-1":
			display = "Spending"
			baseUnits = 100
		case "acc-2":
			display = "Savers"
			baseUnits = 2050
		case "acc-3":
			display = "Joint"
			baseUnits = 5000
		default:
			http.NotFound(w, r)
			return
		}

		writeJSON(t, w, map[string]any{
			"data": map[string]any{
				"type": "accounts",
				"id":   id,
				"attributes": map[string]any{
					"displayName":   display,
					"accountType":   "TRANSACTIONAL",
					"ownershipType": "INDIVIDUAL",
					"balance": map[string]any{
						"currencyCode":     "AUD",
						"value":            "1.00",
						"valueInBaseUnits": baseUnits,
					},
					"createdAt": createdAt,
				},
			},
		})
	})

	s.server = httptest.NewServer(mux)
	return s
}

func (s *accountsStubServer) Close() {
	s.server.Close()
}

func (s *accountsStubServer) URL() string {
	return s.server.URL
}

func (s *accountsStubServer) Assert(t *testing.T) {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listFirstHits != 1 {
		t.Fatalf("first list page hits = %d, want 1", s.listFirstHits)
	}
	if s.listSecondHits != 1 {
		t.Fatalf("second list page hits = %d, want 1", s.listSecondHits)
	}
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		if s.accountIDRequest[id] != 1 {
			t.Fatalf("account %s request count = %d, want 1", id, s.accountIDRequest[id])
		}
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() unexpected error: %v", err)
	}
	return db
}

func createSyncTables(t *testing.T, db *sql.DB) {
	t.Helper()

	const schema = `
CREATE TABLE IF NOT EXISTS sync_state (
  collection TEXT PRIMARY KEY,
  last_success_at TEXT,
  last_attempt_at TEXT,
  last_error TEXT
);

CREATE TABLE IF NOT EXISTS accounts (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  account_type TEXT NOT NULL,
  ownership_type TEXT NOT NULL,
  balance_currency_code TEXT NOT NULL,
  balance_value TEXT NOT NULL,
  balance_value_in_base_units INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  last_fetched_at TEXT NOT NULL,
  is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0,1))
);
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
}

func assertAccountsPersisted(t *testing.T, db *sql.DB) {
	t.Helper()

	rows, err := db.Query(`
SELECT id, display_name, account_type, ownership_type, balance_currency_code, balance_value, balance_value_in_base_units, is_active
FROM accounts
ORDER BY id
`)
	if err != nil {
		t.Fatalf("query accounts: %v", err)
	}
	defer rows.Close()

	type row struct {
		id          string
		displayName string
		acctType    string
		ownerType   string
		currency    string
		value       string
		baseUnits   int64
		createdAt   string
		lastFetched string
		active      int
	}
	got := make([]row, 0, 3)
	for rows.Next() {
		var r row
		if err := rows.Scan(
			&r.id,
			&r.displayName,
			&r.acctType,
			&r.ownerType,
			&r.currency,
			&r.value,
			&r.baseUnits,
			&r.active,
		); err != nil {
			t.Fatalf("scan account row: %v", err)
		}

		if err := db.QueryRow(
			`SELECT created_at, last_fetched_at FROM accounts WHERE id = ?`,
			r.id,
		).Scan(&r.createdAt, &r.lastFetched); err != nil {
			t.Fatalf("query created/last fetched for %s: %v", r.id, err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("accounts row count = %d, want 3", len(got))
	}
	expected := map[string]row{
		"acc-1": {
			id:          "acc-1",
			displayName: "Spending",
			acctType:    "TRANSACTIONAL",
			ownerType:   "INDIVIDUAL",
			currency:    "AUD",
			value:       "1.00",
			baseUnits:   100,
			createdAt:   "2026-02-17T12:13:27+11:00",
			active:      1,
		},
		"acc-2": {
			id:          "acc-2",
			displayName: "Savers",
			acctType:    "TRANSACTIONAL",
			ownerType:   "INDIVIDUAL",
			currency:    "AUD",
			value:       "1.00",
			baseUnits:   2050,
			createdAt:   "2026-02-17T12:13:27+11:00",
			active:      1,
		},
		"acc-3": {
			id:          "acc-3",
			displayName: "Joint",
			acctType:    "TRANSACTIONAL",
			ownerType:   "INDIVIDUAL",
			currency:    "AUD",
			value:       "1.00",
			baseUnits:   5000,
			createdAt:   "2026-02-17T12:13:27+11:00",
			active:      1,
		},
	}

	for _, r := range got {
		want, ok := expected[r.id]
		if !ok {
			t.Fatalf("unexpected account row id %q", r.id)
		}
		if r.displayName != want.displayName {
			t.Fatalf("account %s display_name = %q, want %q", r.id, r.displayName, want.displayName)
		}
		if r.acctType != want.acctType {
			t.Fatalf("account %s account_type = %q, want %q", r.id, r.acctType, want.acctType)
		}
		if r.ownerType != want.ownerType {
			t.Fatalf("account %s ownership_type = %q, want %q", r.id, r.ownerType, want.ownerType)
		}
		if r.currency != want.currency {
			t.Fatalf("account %s balance_currency_code = %q, want %q", r.id, r.currency, want.currency)
		}
		if r.value != want.value {
			t.Fatalf("account %s balance_value = %q, want %q", r.id, r.value, want.value)
		}
		if r.baseUnits != want.baseUnits {
			t.Fatalf(
				"account %s balance_value_in_base_units = %d, want %d",
				r.id,
				r.baseUnits,
				want.baseUnits,
			)
		}
		if r.createdAt != want.createdAt {
			t.Fatalf("account %s created_at = %q, want %q", r.id, r.createdAt, want.createdAt)
		}
		if _, err := time.Parse(time.RFC3339Nano, r.lastFetched); err != nil {
			t.Fatalf("account %s last_fetched_at invalid RFC3339: %q (%v)", r.id, r.lastFetched, err)
		}
		if r.active != want.active {
			t.Fatalf("account %s is_active = %d, want %d", r.id, r.active, want.active)
		}
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func writeJSON(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode json response: %v", err)
	}
}
