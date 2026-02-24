package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lachiem1/giddyUp/internal/auth"
)

type Mode string

const (
	ModeSecure Mode = "secure"
)

const schemaVersion = 6

type Config struct {
	Mode Mode
	Path string
}

func Open(ctx context.Context) (*sql.DB, Config, error) {
	cfg, err := configFromEnv()
	if err != nil {
		return nil, Config{}, err
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o700); err != nil {
		return nil, Config{}, fmt.Errorf("create db directory: %w", err)
	}

	if !secureSQLiteSupported() {
		return nil, Config{}, fmt.Errorf(
			"secure mode requires a sqlcipher-enabled build; rebuild with '-tags sqlcipher'",
		)
	}

	key, created, err := ensureDBKey()
	if err != nil {
		return nil, Config{}, fmt.Errorf("ensure secure db key: %w", err)
	}
	if created {
		if err := resetLocalDBFiles(cfg.Path); err != nil {
			return nil, Config{}, fmt.Errorf("reset db after key creation: %w", err)
		}
	}

	db, err := openSecureSQLite(cfg.Path, key)
	if err != nil {
		return nil, Config{}, err
	}
	if err := runMigrations(ctx, db); err != nil {
		db.Close()
		return nil, Config{}, err
	}

	return db, cfg, nil
}

// Wipe removes local database files for the resolved DB path.
func Wipe() (Config, error) {
	cfg, err := configFromEnv()
	if err != nil {
		return Config{}, err
	}
	if err := resetLocalDBFiles(cfg.Path); err != nil {
		return Config{}, fmt.Errorf("wipe local db files: %w", err)
	}
	return cfg, nil
}

func configFromEnv() (Config, error) {
	if dbPath := strings.TrimSpace(os.Getenv("GIDDYUP_DB_PATH")); dbPath != "" {
		return Config{
			Mode: ModeSecure,
			Path: dbPath,
		}, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve user config directory: %w", err)
	}

	return Config{
		Mode: ModeSecure,
		Path: filepath.Join(configDir, "giddyup", "giddyup.db"),
	}, nil
}

func ensureDBKey() (key string, created bool, err error) {
	key, err = auth.LoadDBKey()
	if err == nil && strings.TrimSpace(key) != "" {
		return key, false, nil
	}

	newKey, err := generateRandomKey()
	if err != nil {
		return "", false, err
	}

	if err := auth.SaveDBKey(newKey); err != nil {
		return "", false, err
	}
	return newKey, true, nil
}

func generateRandomKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(buf), nil
}

func runMigrations(ctx context.Context, db *sql.DB) error {
	const bootstrapSchema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  version INTEGER NOT NULL
);

INSERT OR IGNORE INTO schema_migrations (id, version) VALUES (1, 1);
`
	if _, err := db.ExecContext(ctx, bootstrapSchema); err != nil {
		return fmt.Errorf("run sqlite migrations: %w", err)
	}

	var currentVersion int
	if err := db.QueryRowContext(ctx, "SELECT version FROM schema_migrations WHERE id = 1").Scan(&currentVersion); err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}

	if currentVersion < 2 {
		if err := applyV2Migrations(ctx, db); err != nil {
			return err
		}
		currentVersion = 2
	}
	if currentVersion < 3 {
		if err := applyV3Migrations(ctx, db); err != nil {
			return err
		}
		currentVersion = 3
	}
	if currentVersion < 4 {
		if err := applyV4Migrations(ctx, db); err != nil {
			return err
		}
		currentVersion = 4
	}
	if currentVersion < 5 {
		if err := applyV5Migrations(ctx, db); err != nil {
			return err
		}
		currentVersion = 5
	}
	if currentVersion < 6 {
		if err := applyV6Migrations(ctx, db); err != nil {
			return err
		}
		currentVersion = 6
	}

	if currentVersion > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", currentVersion, schemaVersion)
	}

	return nil
}

func applyV2Migrations(ctx context.Context, db *sql.DB) error {
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
  goal_balance TEXT,
  created_at TEXT NOT NULL,
  last_fetched_at TEXT NOT NULL,
  is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0,1))
);

CREATE INDEX IF NOT EXISTS idx_accounts_last_fetched_at ON accounts(last_fetched_at);
CREATE INDEX IF NOT EXISTS idx_accounts_account_type ON accounts(account_type);
CREATE INDEX IF NOT EXISTS idx_accounts_ownership_type ON accounts(ownership_type);

CREATE TABLE IF NOT EXISTS transactions (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  status TEXT NOT NULL,
  description TEXT NOT NULL,
  message TEXT,
  amount_currency_code TEXT NOT NULL,
  amount_value TEXT NOT NULL,
  amount_value_in_base_units INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  settled_at TEXT,
  last_fetched_at TEXT NOT NULL,
  is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0,1))
);

CREATE INDEX IF NOT EXISTS idx_transactions_account_id ON transactions(account_id);
CREATE INDEX IF NOT EXISTS idx_transactions_created_at ON transactions(created_at);
`
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite migration v2 transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("run sqlite v2 migrations: %w", err)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE schema_migrations SET version = 2 WHERE id = 1"); err != nil {
		return fmt.Errorf("update sqlite schema version to 2: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite v2 migrations: %w", err)
	}
	return nil
}

func applyV3Migrations(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite migration v3 transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	hasDisplayOrder, err := tableHasColumn(ctx, tx, "accounts", "display_order")
	if err != nil {
		return err
	}
	if !hasDisplayOrder {
		if _, err = tx.ExecContext(
			ctx,
			"ALTER TABLE accounts ADD COLUMN display_order INTEGER NOT NULL DEFAULT 2147483647",
		); err != nil {
			return fmt.Errorf("add accounts.display_order column: %w", err)
		}
	}
	if _, err = tx.ExecContext(
		ctx,
		"CREATE INDEX IF NOT EXISTS idx_accounts_display_order ON accounts(display_order)",
	); err != nil {
		return fmt.Errorf("create accounts display_order index: %w", err)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE schema_migrations SET version = 3 WHERE id = 1"); err != nil {
		return fmt.Errorf("update sqlite schema version to 3: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite v3 migrations: %w", err)
	}
	return nil
}

func applyV4Migrations(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite migration v4 transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	hasGoalBalance, err := tableHasColumn(ctx, tx, "accounts", "goal_balance")
	if err != nil {
		return err
	}
	if !hasGoalBalance {
		if _, err = tx.ExecContext(ctx, "ALTER TABLE accounts ADD COLUMN goal_balance TEXT"); err != nil {
			return fmt.Errorf("add accounts.goal_balance column: %w", err)
		}
	}

	// Up's spending account is TRANSACTIONAL; keep goal values for savers only.
	if _, err = tx.ExecContext(ctx, `
CREATE TRIGGER IF NOT EXISTS trg_accounts_goal_balance_insert
AFTER INSERT ON accounts
WHEN NEW.account_type = 'TRANSACTIONAL'
BEGIN
  UPDATE accounts
  SET goal_balance = NULL
  WHERE id = NEW.id;
END;
`); err != nil {
		return fmt.Errorf("create accounts goal_balance insert trigger: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
CREATE TRIGGER IF NOT EXISTS trg_accounts_goal_balance_update
AFTER UPDATE OF account_type, goal_balance ON accounts
WHEN NEW.account_type = 'TRANSACTIONAL' AND NEW.goal_balance IS NOT NULL
BEGIN
  UPDATE accounts
  SET goal_balance = NULL
  WHERE id = NEW.id;
END;
`); err != nil {
		return fmt.Errorf("create accounts goal_balance update trigger: %w", err)
	}
	if _, err = tx.ExecContext(
		ctx,
		"UPDATE accounts SET goal_balance = NULL WHERE account_type = 'TRANSACTIONAL'",
	); err != nil {
		return fmt.Errorf("clear transactional accounts goal_balance: %w", err)
	}

	if _, err = tx.ExecContext(ctx, "UPDATE schema_migrations SET version = 4 WHERE id = 1"); err != nil {
		return fmt.Errorf("update sqlite schema version to 4: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite v4 migrations: %w", err)
	}
	return nil
}

func applyV5Migrations(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite migration v5 transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS app_config (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`); err != nil {
		return fmt.Errorf("create app_config table: %w", err)
	}

	if _, err = tx.ExecContext(ctx, "UPDATE schema_migrations SET version = 5 WHERE id = 1"); err != nil {
		return fmt.Errorf("update sqlite schema version to 5: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite v5 migrations: %w", err)
	}
	return nil
}

func applyV6Migrations(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite migration v6 transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	type colDef struct {
		name string
		ddl  string
	}
	cols := []colDef{
		{name: "resource_type", ddl: "TEXT NOT NULL DEFAULT 'transactions'"},
		{name: "raw_text", ddl: "TEXT"},
		{name: "is_categorizable", ddl: "INTEGER NOT NULL DEFAULT 0 CHECK (is_categorizable IN (0,1))"},
		{name: "hold_amount_currency_code", ddl: "TEXT"},
		{name: "hold_amount_value", ddl: "TEXT"},
		{name: "hold_amount_value_in_base_units", ddl: "INTEGER"},
		{name: "hold_foreign_amount_currency_code", ddl: "TEXT"},
		{name: "hold_foreign_amount_value", ddl: "TEXT"},
		{name: "hold_foreign_amount_value_in_base_units", ddl: "INTEGER"},
		{name: "round_up_amount_currency_code", ddl: "TEXT"},
		{name: "round_up_amount_value", ddl: "TEXT"},
		{name: "round_up_amount_value_in_base_units", ddl: "INTEGER"},
		{name: "round_up_boost_portion_currency_code", ddl: "TEXT"},
		{name: "round_up_boost_portion_value", ddl: "TEXT"},
		{name: "round_up_boost_portion_value_in_base_units", ddl: "INTEGER"},
		{name: "cashback_description", ddl: "TEXT"},
		{name: "cashback_amount_currency_code", ddl: "TEXT"},
		{name: "cashback_amount_value", ddl: "TEXT"},
		{name: "cashback_amount_value_in_base_units", ddl: "INTEGER"},
		{name: "foreign_amount_currency_code", ddl: "TEXT"},
		{name: "foreign_amount_value", ddl: "TEXT"},
		{name: "foreign_amount_value_in_base_units", ddl: "INTEGER"},
		{name: "card_purchase_method_method", ddl: "TEXT"},
		{name: "card_purchase_method_card_number_suffix", ddl: "TEXT"},
		{name: "transaction_type", ddl: "TEXT"},
		{name: "note_text", ddl: "TEXT"},
		{name: "performing_customer_display_name", ddl: "TEXT"},
		{name: "deep_link_url", ddl: "TEXT"},
		{name: "account_resource_type", ddl: "TEXT"},
		{name: "account_link_related", ddl: "TEXT"},
		{name: "transfer_account_resource_type", ddl: "TEXT"},
		{name: "transfer_account_id", ddl: "TEXT"},
		{name: "transfer_account_link_related", ddl: "TEXT"},
		{name: "category_resource_type", ddl: "TEXT"},
		{name: "category_id", ddl: "TEXT"},
		{name: "category_link_self", ddl: "TEXT"},
		{name: "category_link_related", ddl: "TEXT"},
		{name: "parent_category_resource_type", ddl: "TEXT"},
		{name: "parent_category_id", ddl: "TEXT"},
		{name: "parent_category_link_related", ddl: "TEXT"},
		{name: "tags_link_self", ddl: "TEXT"},
		{name: "attachment_resource_type", ddl: "TEXT"},
		{name: "attachment_id", ddl: "TEXT"},
		{name: "attachment_link_related", ddl: "TEXT"},
		{name: "resource_link_self", ddl: "TEXT"},
	}

	for _, c := range cols {
		hasCol, colErr := tableHasColumn(ctx, tx, "transactions", c.name)
		if colErr != nil {
			return colErr
		}
		if hasCol {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE transactions ADD COLUMN %s %s", c.name, c.ddl)
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("add transactions.%s column: %w", c.name, err)
		}
	}

	if _, err = tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS transaction_tags (
  transaction_id TEXT NOT NULL,
  tag_id TEXT NOT NULL,
  tag_type TEXT NOT NULL DEFAULT 'tags',
  relationship_link_self TEXT,
  last_fetched_at TEXT NOT NULL,
  is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0,1)),
  PRIMARY KEY (transaction_id, tag_id),
  FOREIGN KEY (transaction_id) REFERENCES transactions(id) ON DELETE CASCADE
);
`); err != nil {
		return fmt.Errorf("create transaction_tags table: %w", err)
	}
	if _, err = tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_transaction_tags_transaction_id ON transaction_tags(transaction_id)"); err != nil {
		return fmt.Errorf("create transaction_tags transaction_id index: %w", err)
	}
	if _, err = tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_transactions_category_id ON transactions(category_id)"); err != nil {
		return fmt.Errorf("create transactions category_id index: %w", err)
	}
	if _, err = tx.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS idx_transactions_settled_at ON transactions(settled_at)"); err != nil {
		return fmt.Errorf("create transactions settled_at index: %w", err)
	}

	if _, err = tx.ExecContext(ctx, "UPDATE schema_migrations SET version = 6 WHERE id = 1"); err != nil {
		return fmt.Errorf("update sqlite schema version to 6: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite v6 migrations: %w", err)
	}
	return nil
}

func tableHasColumn(ctx context.Context, tx *sql.Tx, tableName, columnName string) (bool, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, fmt.Errorf("query table info for %s: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var ctype sql.NullString
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan table info for %s: %w", tableName, err)
		}
		if name == columnName {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read table info rows for %s: %w", tableName, err)
	}
	return false, nil
}

func resetLocalDBFiles(path string) error {
	paths := []string{
		path,
		path + "-wal",
		path + "-shm",
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
