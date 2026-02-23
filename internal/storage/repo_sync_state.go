package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type SyncState struct {
	Collection   string
	LastSuccess  *time.Time
	LastAttempt  *time.Time
	LastErrorMsg string
}

type SyncStateRepo struct {
	db *sql.DB
}

func NewSyncStateRepo(db *sql.DB) *SyncStateRepo {
	return &SyncStateRepo{db: db}
}

func (r *SyncStateRepo) Get(ctx context.Context, collection string) (SyncState, bool, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT collection, last_success_at, last_attempt_at, COALESCE(last_error, '')
		 FROM sync_state WHERE collection = ?`,
		collection,
	)

	var state SyncState
	var lastSuccess sql.NullString
	var lastAttempt sql.NullString
	if err := row.Scan(&state.Collection, &lastSuccess, &lastAttempt, &state.LastErrorMsg); err != nil {
		if err == sql.ErrNoRows {
			return SyncState{}, false, nil
		}
		return SyncState{}, false, fmt.Errorf("query sync state for %q: %w", collection, err)
	}

	if strings.TrimSpace(lastSuccess.String) != "" {
		t, err := time.Parse(time.RFC3339Nano, lastSuccess.String)
		if err != nil {
			return SyncState{}, false, fmt.Errorf("parse last_success_at for %q: %w", collection, err)
		}
		state.LastSuccess = &t
	}
	if strings.TrimSpace(lastAttempt.String) != "" {
		t, err := time.Parse(time.RFC3339Nano, lastAttempt.String)
		if err != nil {
			return SyncState{}, false, fmt.Errorf("parse last_attempt_at for %q: %w", collection, err)
		}
		state.LastAttempt = &t
	}

	return state, true, nil
}

func (r *SyncStateRepo) RecordAttempt(ctx context.Context, collection string, at time.Time) error {
	// Clear previous error at the start of a new attempt.
	msg := ""
	return r.upsert(ctx, collection, at, nil, &msg)
}

func (r *SyncStateRepo) RecordSuccess(ctx context.Context, collection string, at time.Time) error {
	msg := ""
	return r.upsert(ctx, collection, at, &at, &msg)
}

func (r *SyncStateRepo) RecordError(ctx context.Context, collection string, at time.Time, syncErr error) error {
	msg := ""
	if syncErr != nil {
		msg = syncErr.Error()
	}
	return r.upsert(ctx, collection, at, nil, &msg)
}

func (r *SyncStateRepo) upsert(
	ctx context.Context,
	collection string,
	attemptAt time.Time,
	successAt *time.Time,
	errorMsg *string,
) error {
	attemptValue := attemptAt.UTC().Format(time.RFC3339Nano)
	var successValue any
	if successAt != nil {
		successValue = successAt.UTC().Format(time.RFC3339Nano)
	}
	var errorValue any
	if errorMsg != nil {
		errorValue = *errorMsg
	}

	const q = `
INSERT INTO sync_state (collection, last_attempt_at, last_success_at, last_error)
VALUES (?, ?, ?, ?)
ON CONFLICT(collection) DO UPDATE SET
  last_attempt_at = excluded.last_attempt_at,
  last_success_at = COALESCE(excluded.last_success_at, sync_state.last_success_at),
  last_error = CASE
    WHEN excluded.last_error IS NULL THEN sync_state.last_error
    ELSE excluded.last_error
  END
`
	if _, err := r.db.ExecContext(ctx, q, collection, attemptValue, successValue, errorValue); err != nil {
		return fmt.Errorf("upsert sync state for %q: %w", collection, err)
	}
	return nil
}
