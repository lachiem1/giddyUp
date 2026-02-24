package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type TransactionTag struct {
	TagID    string
	TagType  string
	LinkSelf *string
}

type TransactionRecord struct {
	ID                                  string
	ResourceType                        string
	Status                              string
	RawText                             *string
	Description                         string
	Message                             *string
	IsCategorizable                     bool
	HoldAmountCurrencyCode              *string
	HoldAmountValue                     *string
	HoldAmountValueInBaseUnits          *int64
	HoldForeignAmountCurrencyCode       *string
	HoldForeignAmountValue              *string
	HoldForeignAmountValueInBaseUnits   *int64
	RoundUpAmountCurrencyCode           *string
	RoundUpAmountValue                  *string
	RoundUpAmountValueInBaseUnits       *int64
	RoundUpBoostPortionCurrencyCode     *string
	RoundUpBoostPortionValue            *string
	RoundUpBoostPortionValueInBaseUnits *int64
	CashbackDescription                 *string
	CashbackAmountCurrencyCode          *string
	CashbackAmountValue                 *string
	CashbackAmountValueInBaseUnits      *int64
	AmountCurrencyCode                  string
	AmountValue                         string
	AmountValueInBaseUnits              int64
	ForeignAmountCurrencyCode           *string
	ForeignAmountValue                  *string
	ForeignAmountValueInBaseUnits       *int64
	CardPurchaseMethodMethod            *string
	CardPurchaseMethodCardNumberSuffix  *string
	SettledAt                           *string
	CreatedAt                           string
	TransactionType                     *string
	NoteText                            *string
	PerformingCustomerDisplayName       *string
	DeepLinkURL                         *string

	AccountID                   string
	AccountResourceType         *string
	AccountLinkRelated          *string
	TransferAccountResourceType *string
	TransferAccountID           *string
	TransferAccountLinkRelated  *string
	CategoryResourceType        *string
	CategoryID                  *string
	CategoryLinkSelf            *string
	CategoryLinkRelated         *string
	ParentCategoryResourceType  *string
	ParentCategoryID            *string
	ParentCategoryLinkRelated   *string
	TagsLinkSelf                *string
	AttachmentResourceType      *string
	AttachmentID                *string
	AttachmentLinkRelated       *string
	ResourceLinkSelf            *string

	Tags []TransactionTag
}

type TransactionsRepo struct {
	db *sql.DB
}

func NewTransactionsRepo(db *sql.DB) *TransactionsRepo {
	return &TransactionsRepo{db: db}
}

func (r *TransactionsRepo) HasAny(ctx context.Context) (bool, error) {
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM transactions WHERE is_active = 1 LIMIT 1)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check active transactions: %w", err)
	}
	return exists == 1, nil
}

func (r *TransactionsRepo) KnownIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf("SELECT id FROM transactions WHERE id IN (%s)", strings.Join(placeholders, ","))
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query known transaction ids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan known transaction id: %w", err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate known transaction ids: %w", err)
	}
	return out, nil
}

func (r *TransactionsRepo) UpsertBatch(ctx context.Context, records []TransactionRecord, fetchedAt time.Time) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transactions upsert transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	fetchedValue := fetchedAt.UTC().Format(time.RFC3339Nano)
	accountNames, err := loadAccountDisplayNameByID(ctx, tx)
	if err != nil {
		return err
	}

	const upsert = `
INSERT INTO transactions (
  id, account_id, status, description, message,
  amount_currency_code, amount_value, amount_value_in_base_units,
  created_at, settled_at, last_fetched_at, is_active,
  resource_type, raw_text, is_categorizable,
  hold_amount_currency_code, hold_amount_value, hold_amount_value_in_base_units,
  hold_foreign_amount_currency_code, hold_foreign_amount_value, hold_foreign_amount_value_in_base_units,
  round_up_amount_currency_code, round_up_amount_value, round_up_amount_value_in_base_units,
  round_up_boost_portion_currency_code, round_up_boost_portion_value, round_up_boost_portion_value_in_base_units,
  cashback_description, cashback_amount_currency_code, cashback_amount_value, cashback_amount_value_in_base_units,
  foreign_amount_currency_code, foreign_amount_value, foreign_amount_value_in_base_units,
  card_purchase_method_method, card_purchase_method_card_number_suffix,
  transaction_type, note_text, performing_customer_display_name, deep_link_url,
  account_resource_type, account_link_related,
  transfer_account_resource_type, transfer_account_id, transfer_account_link_related,
  category_resource_type, category_id, category_link_self, category_link_related,
  parent_category_resource_type, parent_category_id, parent_category_link_related,
  tags_link_self, attachment_resource_type, attachment_id, attachment_link_related, resource_link_self,
  raw_text_norm, description_norm, merchant_norm
) VALUES (
  ?, ?, ?, ?, ?,
  ?, ?, ?,
  ?, ?, ?, 1,
  ?, ?, ?,
  ?, ?, ?,
  ?, ?, ?,
  ?, ?, ?,
  ?, ?, ?,
  ?, ?, ?, ?,
  ?, ?, ?,
  ?, ?,
  ?, ?, ?, ?,
  ?, ?,
  ?, ?, ?,
  ?, ?, ?, ?,
  ?, ?, ?,
  ?, ?, ?, ?, ?,
  ?, ?, ?
)
ON CONFLICT(id) DO UPDATE SET
  account_id = excluded.account_id,
  status = excluded.status,
  description = excluded.description,
  message = excluded.message,
  amount_currency_code = excluded.amount_currency_code,
  amount_value = excluded.amount_value,
  amount_value_in_base_units = excluded.amount_value_in_base_units,
  created_at = excluded.created_at,
  settled_at = excluded.settled_at,
  last_fetched_at = excluded.last_fetched_at,
  is_active = 1,
  resource_type = excluded.resource_type,
  raw_text = excluded.raw_text,
  is_categorizable = excluded.is_categorizable,
  hold_amount_currency_code = excluded.hold_amount_currency_code,
  hold_amount_value = excluded.hold_amount_value,
  hold_amount_value_in_base_units = excluded.hold_amount_value_in_base_units,
  hold_foreign_amount_currency_code = excluded.hold_foreign_amount_currency_code,
  hold_foreign_amount_value = excluded.hold_foreign_amount_value,
  hold_foreign_amount_value_in_base_units = excluded.hold_foreign_amount_value_in_base_units,
  round_up_amount_currency_code = excluded.round_up_amount_currency_code,
  round_up_amount_value = excluded.round_up_amount_value,
  round_up_amount_value_in_base_units = excluded.round_up_amount_value_in_base_units,
  round_up_boost_portion_currency_code = excluded.round_up_boost_portion_currency_code,
  round_up_boost_portion_value = excluded.round_up_boost_portion_value,
  round_up_boost_portion_value_in_base_units = excluded.round_up_boost_portion_value_in_base_units,
  cashback_description = excluded.cashback_description,
  cashback_amount_currency_code = excluded.cashback_amount_currency_code,
  cashback_amount_value = excluded.cashback_amount_value,
  cashback_amount_value_in_base_units = excluded.cashback_amount_value_in_base_units,
  foreign_amount_currency_code = excluded.foreign_amount_currency_code,
  foreign_amount_value = excluded.foreign_amount_value,
  foreign_amount_value_in_base_units = excluded.foreign_amount_value_in_base_units,
  card_purchase_method_method = excluded.card_purchase_method_method,
  card_purchase_method_card_number_suffix = excluded.card_purchase_method_card_number_suffix,
  transaction_type = excluded.transaction_type,
  note_text = excluded.note_text,
  performing_customer_display_name = excluded.performing_customer_display_name,
  deep_link_url = excluded.deep_link_url,
  account_resource_type = excluded.account_resource_type,
  account_link_related = excluded.account_link_related,
  transfer_account_resource_type = excluded.transfer_account_resource_type,
  transfer_account_id = excluded.transfer_account_id,
  transfer_account_link_related = excluded.transfer_account_link_related,
  category_resource_type = excluded.category_resource_type,
  category_id = excluded.category_id,
  category_link_self = excluded.category_link_self,
  category_link_related = excluded.category_link_related,
  parent_category_resource_type = excluded.parent_category_resource_type,
  parent_category_id = excluded.parent_category_id,
  parent_category_link_related = excluded.parent_category_link_related,
  tags_link_self = excluded.tags_link_self,
  attachment_resource_type = excluded.attachment_resource_type,
  attachment_id = excluded.attachment_id,
  attachment_link_related = excluded.attachment_link_related,
  resource_link_self = excluded.resource_link_self,
  raw_text_norm = excluded.raw_text_norm,
  description_norm = excluded.description_norm,
  merchant_norm = excluded.merchant_norm
`

	for _, rcd := range records {
		isCategorizable := 0
		if rcd.IsCategorizable {
			isCategorizable = 1
		}
		rawText := ptrStringValue(rcd.RawText)
		rawTextNorm := normalizeTransactionText(rawText)
		descriptionNorm := normalizeTransactionText(rcd.Description)
		merchantNorm := normalizeTransactionMerchant(rawText, rcd.Description)
		if rcd.TransferAccountID != nil && strings.TrimSpace(*rcd.TransferAccountID) != "" {
			accountName := accountNames[rcd.AccountID]
			transferName := accountNames[*rcd.TransferAccountID]
			if normalizedTransfer, ok := normalizeInternalTransferMerchant(
				accountName,
				transferName,
				rcd.AmountValueInBaseUnits,
				rawText,
				rcd.Description,
			); ok {
				merchantNorm = normalizedTransfer
			}
		}

		if _, err = tx.ExecContext(
			ctx,
			upsert,
			rcd.ID, rcd.AccountID, rcd.Status, rcd.Description, ptrString(rcd.Message),
			rcd.AmountCurrencyCode, rcd.AmountValue, rcd.AmountValueInBaseUnits,
			rcd.CreatedAt, ptrString(rcd.SettledAt), fetchedValue,
			emptyIfBlank(rcd.ResourceType), ptrString(rcd.RawText), isCategorizable,
			ptrString(rcd.HoldAmountCurrencyCode), ptrString(rcd.HoldAmountValue), ptrInt64(rcd.HoldAmountValueInBaseUnits),
			ptrString(rcd.HoldForeignAmountCurrencyCode), ptrString(rcd.HoldForeignAmountValue), ptrInt64(rcd.HoldForeignAmountValueInBaseUnits),
			ptrString(rcd.RoundUpAmountCurrencyCode), ptrString(rcd.RoundUpAmountValue), ptrInt64(rcd.RoundUpAmountValueInBaseUnits),
			ptrString(rcd.RoundUpBoostPortionCurrencyCode), ptrString(rcd.RoundUpBoostPortionValue), ptrInt64(rcd.RoundUpBoostPortionValueInBaseUnits),
			ptrString(rcd.CashbackDescription), ptrString(rcd.CashbackAmountCurrencyCode), ptrString(rcd.CashbackAmountValue), ptrInt64(rcd.CashbackAmountValueInBaseUnits),
			ptrString(rcd.ForeignAmountCurrencyCode), ptrString(rcd.ForeignAmountValue), ptrInt64(rcd.ForeignAmountValueInBaseUnits),
			ptrString(rcd.CardPurchaseMethodMethod), ptrString(rcd.CardPurchaseMethodCardNumberSuffix),
			ptrString(rcd.TransactionType), ptrString(rcd.NoteText), ptrString(rcd.PerformingCustomerDisplayName), ptrString(rcd.DeepLinkURL),
			ptrString(rcd.AccountResourceType), ptrString(rcd.AccountLinkRelated),
			ptrString(rcd.TransferAccountResourceType), ptrString(rcd.TransferAccountID), ptrString(rcd.TransferAccountLinkRelated),
			ptrString(rcd.CategoryResourceType), ptrString(rcd.CategoryID), ptrString(rcd.CategoryLinkSelf), ptrString(rcd.CategoryLinkRelated),
			ptrString(rcd.ParentCategoryResourceType), ptrString(rcd.ParentCategoryID), ptrString(rcd.ParentCategoryLinkRelated),
			ptrString(rcd.TagsLinkSelf), ptrString(rcd.AttachmentResourceType), ptrString(rcd.AttachmentID), ptrString(rcd.AttachmentLinkRelated), ptrString(rcd.ResourceLinkSelf),
			rawTextNorm, descriptionNorm, merchantNorm,
		); err != nil {
			return fmt.Errorf("upsert transaction %q: %w", rcd.ID, err)
		}

		if _, err = tx.ExecContext(ctx, "UPDATE transaction_tags SET is_active = 0 WHERE transaction_id = ?", rcd.ID); err != nil {
			return fmt.Errorf("deactivate transaction tags %q: %w", rcd.ID, err)
		}
		for _, tag := range rcd.Tags {
			tagType := tag.TagType
			if strings.TrimSpace(tagType) == "" {
				tagType = "tags"
			}
			if _, err = tx.ExecContext(
				ctx,
				`INSERT INTO transaction_tags (transaction_id, tag_id, tag_type, relationship_link_self, last_fetched_at, is_active)
				 VALUES (?, ?, ?, ?, ?, 1)
				 ON CONFLICT(transaction_id, tag_id) DO UPDATE SET
				   tag_type = excluded.tag_type,
				   relationship_link_self = excluded.relationship_link_self,
				   last_fetched_at = excluded.last_fetched_at,
				   is_active = 1`,
				rcd.ID,
				tag.TagID,
				tagType,
				ptrString(tag.LinkSelf),
				fetchedValue,
			); err != nil {
				return fmt.Errorf("upsert transaction tag %q/%q: %w", rcd.ID, tag.TagID, err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transactions upsert transaction: %w", err)
	}
	return nil
}

func ptrString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func ptrInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func ptrStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func loadAccountDisplayNameByID(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, COALESCE(display_name, '') FROM accounts`)
	if err != nil {
		return nil, fmt.Errorf("query accounts for transaction merchant normalization: %w", err)
	}
	defer rows.Close()

	names := make(map[string]string, 64)
	for rows.Next() {
		var id string
		var displayName string
		if err := rows.Scan(&id, &displayName); err != nil {
			return nil, fmt.Errorf("scan accounts for transaction merchant normalization: %w", err)
		}
		names[id] = displayName
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts for transaction merchant normalization: %w", err)
	}
	return names, nil
}

func emptyIfBlank(s string) string {
	if strings.TrimSpace(s) == "" {
		return "transactions"
	}
	return s
}
