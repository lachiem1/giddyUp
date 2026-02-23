package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Account struct {
	ID                      string
	DisplayName             string
	AccountType             string
	OwnershipType           string
	BalanceCurrencyCode     string
	BalanceValue            string
	BalanceValueInBaseUnits int64
	CreatedAt               string
}

type AccountsRepo struct {
	db *sql.DB
}

func NewAccountsRepo(db *sql.DB) *AccountsRepo {
	return &AccountsRepo{db: db}
}

func (r *AccountsRepo) HasActiveAccounts(ctx context.Context) (bool, error) {
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE is_active = 1 LIMIT 1)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check active accounts: %w", err)
	}
	return exists == 1, nil
}

func (r *AccountsRepo) ReplaceSnapshot(ctx context.Context, accounts []Account, fetchedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin accounts snapshot transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	fetchedValue := fetchedAt.UTC().Format(time.RFC3339Nano)
	const upsert = `
INSERT INTO accounts (
	id,
	display_name,
	account_type,
	ownership_type,
	balance_currency_code,
	balance_value,
	balance_value_in_base_units,
	created_at,
	last_fetched_at,
	is_active
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
ON CONFLICT(id) DO UPDATE SET
	display_name = excluded.display_name,
	account_type = excluded.account_type,
	ownership_type = excluded.ownership_type,
	balance_currency_code = excluded.balance_currency_code,
	balance_value = excluded.balance_value,
	balance_value_in_base_units = excluded.balance_value_in_base_units,
	created_at = excluded.created_at,
	last_fetched_at = excluded.last_fetched_at,
	is_active = 1
`
	for _, acct := range accounts {
		if _, err = tx.ExecContext(
			ctx,
			upsert,
			acct.ID,
			acct.DisplayName,
			acct.AccountType,
			acct.OwnershipType,
			acct.BalanceCurrencyCode,
			acct.BalanceValue,
			acct.BalanceValueInBaseUnits,
			acct.CreatedAt,
			fetchedValue,
		); err != nil {
			return fmt.Errorf("upsert account %q: %w", acct.ID, err)
		}
	}

	if err = deactivateMissingAccounts(ctx, tx, accounts); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit accounts snapshot transaction: %w", err)
	}
	return nil
}

func deactivateMissingAccounts(ctx context.Context, tx *sql.Tx, accounts []Account) error {
	if len(accounts) == 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET is_active = 0`); err != nil {
			return fmt.Errorf("deactivate all accounts: %w", err)
		}
		return nil
	}

	placeholders := make([]string, len(accounts))
	args := make([]any, len(accounts))
	for i, acct := range accounts {
		placeholders[i] = "?"
		args[i] = acct.ID
	}

	q := fmt.Sprintf(
		"UPDATE accounts SET is_active = 0 WHERE id NOT IN (%s)",
		strings.Join(placeholders, ","),
	)
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("deactivate missing accounts: %w", err)
	}

	return nil
}
