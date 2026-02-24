package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type AppConfigRepo struct {
	db *sql.DB
}

func NewAppConfigRepo(db *sql.DB) *AppConfigRepo {
	return &AppConfigRepo{db: db}
}

func (r *AppConfigRepo) Get(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := r.db.QueryRowContext(ctx, "SELECT value FROM app_config WHERE key = ?", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("get app config %q: %w", key, err)
	}
	return value, true, nil
}

func (r *AppConfigRepo) UpsertMany(ctx context.Context, values map[string]string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin app config upsert transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for key, value := range values {
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO app_config (key, value, updated_at) VALUES (?, ?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			key,
			value,
			now,
		); err != nil {
			return fmt.Errorf("upsert app config %q: %w", key, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit app config upsert transaction: %w", err)
	}
	return nil
}
