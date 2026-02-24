package syncer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lachiem1/giddyUp/internal/storage"
	"github.com/lachiem1/giddyUp/internal/upapi"
)

const defaultTransactionsMaxPages = 20

type TransactionsSyncer struct {
	client    *upapi.Client
	txRepo    *storage.TransactionsRepo
	syncState *storage.SyncStateRepo
	maxPages  int
}

func NewTransactionsSyncer(
	client *upapi.Client,
	txRepo *storage.TransactionsRepo,
	syncState *storage.SyncStateRepo,
	maxPages int,
) *TransactionsSyncer {
	if maxPages <= 0 {
		maxPages = defaultTransactionsMaxPages
	}
	return &TransactionsSyncer{
		client:    client,
		txRepo:    txRepo,
		syncState: syncState,
		maxPages:  maxPages,
	}
}

func (s *TransactionsSyncer) Collection() string {
	return CollectionTransactions
}

func (s *TransactionsSyncer) HasCachedData(ctx context.Context) (bool, error) {
	return s.txRepo.HasAny(ctx)
}

func (s *TransactionsSyncer) LastSuccessAt(ctx context.Context) (time.Time, bool, error) {
	state, ok, err := s.syncState.Get(ctx, s.Collection())
	if err != nil {
		return time.Time{}, false, err
	}
	if !ok || state.LastSuccess == nil {
		return time.Time{}, false, nil
	}
	return state.LastSuccess.UTC(), true, nil
}

func (s *TransactionsSyncer) Sync(ctx context.Context) error {
	hasCached, err := s.txRepo.HasAny(ctx)
	if err != nil {
		return err
	}

	return runSyncAttempt(ctx, s.syncState, s.Collection(), func(runCtx context.Context) (time.Time, error) {
		pageCount := 0
		knownSeen := 0
		next := ""
		fetchedAt := time.Now().UTC()

		for {
			var page *upapi.ListResponse
			if next == "" {
				page, err = s.client.ListTransactionsPage(runCtx, upapi.TransactionListOptions{})
			} else {
				page, err = s.client.ListTransactionsPageByURL(runCtx, next)
			}
			if err != nil {
				return time.Time{}, err
			}
			pageCount++
			if len(page.Data) == 0 {
				return fetchedAt, nil
			}

			ids := make([]string, 0, len(page.Data))
			for _, res := range page.Data {
				if res.ID != "" {
					ids = append(ids, res.ID)
				}
			}
			known := map[string]bool{}
			if hasCached && len(ids) > 0 {
				known, err = s.txRepo.KnownIDs(runCtx, ids)
				if err != nil {
					return time.Time{}, err
				}
			}

			batch := make([]storage.TransactionRecord, 0, len(page.Data))
			shouldStop := false
			for _, res := range page.Data {
				if res.ID == "" {
					continue
				}
				if hasCached && known[res.ID] {
					knownSeen++
					if knownSeen >= 2 {
						shouldStop = true
						break
					}
					continue
				}
				knownSeen = 0
				rec, mapErr := mapTransactionRecord(res)
				if mapErr != nil {
					return time.Time{}, mapErr
				}
				batch = append(batch, rec)
			}

			fetchedAt = time.Now().UTC()
			if len(batch) > 0 {
				if err := s.txRepo.UpsertBatch(runCtx, batch, fetchedAt); err != nil {
					return time.Time{}, err
				}
			}

			if shouldStop {
				return fetchedAt, nil
			}

			if page.Links.Next == nil || *page.Links.Next == "" {
				return fetchedAt, nil
			}
			next = *page.Links.Next

			// On incremental runs, cap page traversal.
			if hasCached && pageCount >= s.maxPages {
				return fetchedAt, nil
			}
		}
	})
}

func mapTransactionRecord(res upapi.Resource) (storage.TransactionRecord, error) {
	if stringsTrim(res.ID) == "" {
		return storage.TransactionRecord{}, fmt.Errorf("transaction id is empty")
	}

	attrs := res.Attributes
	if attrs == nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q missing attributes", res.ID)
	}
	rels := res.Relationships

	status, err := stringAttr(attrs, "status")
	if err != nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: %w", res.ID, err)
	}
	description, err := stringAttr(attrs, "description")
	if err != nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: %w", res.ID, err)
	}
	createdAt, err := stringAttr(attrs, "createdAt")
	if err != nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: %w", res.ID, err)
	}
	isCategorizable, err := boolAttr(attrs, "isCategorizable")
	if err != nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: %w", res.ID, err)
	}

	amountObj, err := optionalObject(attrs, "amount")
	if err != nil || amountObj == nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: missing amount", res.ID)
	}
	amountCurrency, err := stringAttr(amountObj, "currencyCode")
	if err != nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: %w", res.ID, err)
	}
	amountValue, err := stringAttr(amountObj, "value")
	if err != nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: %w", res.ID, err)
	}
	amountBaseUnits, err := int64Attr(amountObj, "valueInBaseUnits")
	if err != nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: %w", res.ID, err)
	}

	accountID, accountType, accountRelated, err := parseAccountRelationship(rels, "account")
	if err != nil {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: %w", res.ID, err)
	}
	if stringsTrim(accountID) == "" {
		return storage.TransactionRecord{}, fmt.Errorf("transaction %q: missing relationships.account.data.id", res.ID)
	}

	rec := storage.TransactionRecord{
		ID:                     res.ID,
		ResourceType:           ifBlank(res.Type, "transactions"),
		Status:                 status,
		Description:            description,
		IsCategorizable:        isCategorizable,
		AmountCurrencyCode:     amountCurrency,
		AmountValue:            amountValue,
		AmountValueInBaseUnits: amountBaseUnits,
		CreatedAt:              createdAt,
		AccountID:              accountID,
		AccountResourceType:    accountType,
		AccountLinkRelated:     accountRelated,
		ResourceLinkSelf:       mapStringPtr(res.Links, "self"),
	}

	rec.RawText, _ = optionalString(attrs, "rawText")
	rec.Message, _ = optionalString(attrs, "message")
	rec.SettledAt, _ = optionalString(attrs, "settledAt")
	rec.TransactionType, _ = optionalString(attrs, "transactionType")
	rec.DeepLinkURL, _ = optionalString(attrs, "deepLinkURL")

	rec.ForeignAmountCurrencyCode, rec.ForeignAmountValue, rec.ForeignAmountValueInBaseUnits = parseMoneyAttr(attrs, "foreignAmount")
	rec.HoldAmountCurrencyCode, rec.HoldAmountValue, rec.HoldAmountValueInBaseUnits, rec.HoldForeignAmountCurrencyCode, rec.HoldForeignAmountValue, rec.HoldForeignAmountValueInBaseUnits = parseHoldInfo(attrs)
	rec.RoundUpAmountCurrencyCode, rec.RoundUpAmountValue, rec.RoundUpAmountValueInBaseUnits, rec.RoundUpBoostPortionCurrencyCode, rec.RoundUpBoostPortionValue, rec.RoundUpBoostPortionValueInBaseUnits = parseRoundUp(attrs)
	rec.CashbackDescription, rec.CashbackAmountCurrencyCode, rec.CashbackAmountValue, rec.CashbackAmountValueInBaseUnits = parseCashback(attrs)
	rec.CardPurchaseMethodMethod, rec.CardPurchaseMethodCardNumberSuffix = parseCardMethod(attrs)
	rec.NoteText = parseNestedString(attrs, "note", "text")
	rec.PerformingCustomerDisplayName = parseNestedString(attrs, "performingCustomer", "displayName")

	transferID, transferType, transferRelated, _ := parseAccountRelationship(rels, "transferAccount")
	rec.TransferAccountID = stringPtr(transferID)
	rec.TransferAccountResourceType = transferType
	rec.TransferAccountLinkRelated = transferRelated

	categoryID, categoryType, categoryRelated, categorySelf := parseRelWithSelf(rels, "category")
	rec.CategoryID = stringPtr(categoryID)
	rec.CategoryResourceType = categoryType
	rec.CategoryLinkRelated = categoryRelated
	rec.CategoryLinkSelf = categorySelf

	parentCategoryID, parentCategoryType, parentCategoryRelated, _ := parseRelWithSelf(rels, "parentCategory")
	rec.ParentCategoryID = stringPtr(parentCategoryID)
	rec.ParentCategoryResourceType = parentCategoryType
	rec.ParentCategoryLinkRelated = parentCategoryRelated

	rec.TagsLinkSelf, rec.Tags = parseTags(rels)
	attachmentID, attachmentType, attachmentRelated, _ := parseRelWithSelf(rels, "attachment")
	rec.AttachmentID = stringPtr(attachmentID)
	rec.AttachmentResourceType = attachmentType
	rec.AttachmentLinkRelated = attachmentRelated

	return rec, nil
}

func parseMoneyAttr(attrs map[string]any, key string) (*string, *string, *int64) {
	obj, err := optionalObject(attrs, key)
	if err != nil || obj == nil {
		return nil, nil, nil
	}
	c, _ := optionalString(obj, "currencyCode")
	v, _ := optionalString(obj, "value")
	u, _ := optionalInt64(obj, "valueInBaseUnits")
	return c, v, u
}

func parseHoldInfo(attrs map[string]any) (*string, *string, *int64, *string, *string, *int64) {
	hold, err := optionalObject(attrs, "holdInfo")
	if err != nil || hold == nil {
		return nil, nil, nil, nil, nil, nil
	}
	amountObj, _ := optionalObject(hold, "amount")
	fObj, _ := optionalObject(hold, "foreignAmount")
	var ac, av *string
	var au *int64
	var fc, fv *string
	var fu *int64
	if amountObj != nil {
		ac, _ = optionalString(amountObj, "currencyCode")
		av, _ = optionalString(amountObj, "value")
		au, _ = optionalInt64(amountObj, "valueInBaseUnits")
	}
	if fObj != nil {
		fc, _ = optionalString(fObj, "currencyCode")
		fv, _ = optionalString(fObj, "value")
		fu, _ = optionalInt64(fObj, "valueInBaseUnits")
	}
	return ac, av, au, fc, fv, fu
}

func parseRoundUp(attrs map[string]any) (*string, *string, *int64, *string, *string, *int64) {
	ru, err := optionalObject(attrs, "roundUp")
	if err != nil || ru == nil {
		return nil, nil, nil, nil, nil, nil
	}
	amountObj, _ := optionalObject(ru, "amount")
	boostObj, _ := optionalObject(ru, "boostPortion")
	var ac, av *string
	var au *int64
	var bc, bv *string
	var bu *int64
	if amountObj != nil {
		ac, _ = optionalString(amountObj, "currencyCode")
		av, _ = optionalString(amountObj, "value")
		au, _ = optionalInt64(amountObj, "valueInBaseUnits")
	}
	if boostObj != nil {
		bc, _ = optionalString(boostObj, "currencyCode")
		bv, _ = optionalString(boostObj, "value")
		bu, _ = optionalInt64(boostObj, "valueInBaseUnits")
	}
	return ac, av, au, bc, bv, bu
}

func parseCashback(attrs map[string]any) (*string, *string, *string, *int64) {
	cb, err := optionalObject(attrs, "cashback")
	if err != nil || cb == nil {
		return nil, nil, nil, nil
	}
	desc, _ := optionalString(cb, "description")
	amountObj, _ := optionalObject(cb, "amount")
	if amountObj == nil {
		return desc, nil, nil, nil
	}
	ac, _ := optionalString(amountObj, "currencyCode")
	av, _ := optionalString(amountObj, "value")
	au, _ := optionalInt64(amountObj, "valueInBaseUnits")
	return desc, ac, av, au
}

func parseCardMethod(attrs map[string]any) (*string, *string) {
	obj, err := optionalObject(attrs, "cardPurchaseMethod")
	if err != nil || obj == nil {
		return nil, nil
	}
	method, _ := optionalString(obj, "method")
	suffix, _ := optionalString(obj, "cardNumberSuffix")
	return method, suffix
}

func parseNestedString(attrs map[string]any, objectKey, key string) *string {
	obj, err := optionalObject(attrs, objectKey)
	if err != nil || obj == nil {
		return nil
	}
	v, _ := optionalString(obj, key)
	return v
}

func parseAccountRelationship(rels map[string]map[string]interface{}, key string) (id string, rType *string, related *string, err error) {
	rel, ok := rels[key]
	if !ok {
		return "", nil, nil, nil
	}
	dataObj, _ := rel["data"].(map[string]any)
	if dataObj != nil {
		if v, ok := dataObj["id"].(string); ok {
			id = v
		}
		if t, ok := dataObj["type"].(string); ok {
			rType = &t
		}
	}
	if links, ok := rel["links"].(map[string]any); ok {
		if v, ok := links["related"].(string); ok {
			related = &v
		}
	}
	return id, rType, related, nil
}

func parseRelWithSelf(rels map[string]map[string]interface{}, key string) (id string, rType *string, related *string, self *string) {
	rel, ok := rels[key]
	if !ok {
		return "", nil, nil, nil
	}
	if dataObj, ok := rel["data"].(map[string]any); ok {
		if v, ok := dataObj["id"].(string); ok {
			id = v
		}
		if t, ok := dataObj["type"].(string); ok {
			rType = &t
		}
	}
	if links, ok := rel["links"].(map[string]any); ok {
		if v, ok := links["related"].(string); ok {
			related = &v
		}
		if v, ok := links["self"].(string); ok {
			self = &v
		}
	}
	return id, rType, related, self
}

func parseTags(rels map[string]map[string]interface{}) (*string, []storage.TransactionTag) {
	rel, ok := rels["tags"]
	if !ok {
		return nil, nil
	}
	var self *string
	if links, ok := rel["links"].(map[string]any); ok {
		if v, ok := links["self"].(string); ok {
			self = &v
		}
	}

	data, ok := rel["data"].([]any)
	if !ok || len(data) == 0 {
		return self, nil
	}
	tags := make([]storage.TransactionTag, 0, len(data))
	for _, item := range data {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := obj["id"].(string)
		if stringsTrim(id) == "" {
			continue
		}
		tagType, _ := obj["type"].(string)
		tags = append(tags, storage.TransactionTag{
			TagID:    id,
			TagType:  tagType,
			LinkSelf: self,
		})
	}
	return self, tags
}

func optionalObject(attrs map[string]any, key string) (map[string]any, error) {
	val, ok := attrs[key]
	if !ok || val == nil {
		return nil, nil
	}
	obj, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid %s type %T", key, val)
	}
	return obj, nil
}

func optionalString(attrs map[string]any, key string) (*string, error) {
	val, ok := attrs[key]
	if !ok || val == nil {
		return nil, nil
	}
	str, ok := val.(string)
	if !ok {
		return nil, fmt.Errorf("invalid %s type %T", key, val)
	}
	return &str, nil
}

func optionalInt64(attrs map[string]any, key string) (*int64, error) {
	val, ok := attrs[key]
	if !ok || val == nil {
		return nil, nil
	}
	switch n := val.(type) {
	case float64:
		if n != float64(int64(n)) {
			return nil, fmt.Errorf("non-integer %s", key)
		}
		v := int64(n)
		return &v, nil
	case int64:
		v := n
		return &v, nil
	case int:
		v := int64(n)
		return &v, nil
	default:
		return nil, fmt.Errorf("invalid %s type %T", key, val)
	}
}

func boolAttr(attrs map[string]any, key string) (bool, error) {
	val, ok := attrs[key]
	if !ok {
		return false, fmt.Errorf("missing %s", key)
	}
	b, ok := val.(bool)
	if !ok {
		return false, fmt.Errorf("invalid %s type %T", key, val)
	}
	return b, nil
}

func mapStringPtr(m map[string]string, key string) *string {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok {
		return nil
	}
	return &v
}

func stringsTrim(v string) string {
	return strings.TrimSpace(v)
}

func ifBlank(v, fallback string) string {
	if stringsTrim(v) == "" {
		return fallback
	}
	return v
}

func stringPtr(v string) *string {
	if stringsTrim(v) == "" {
		return nil
	}
	return &v
}
