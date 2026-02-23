package syncer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/lachiem1/giddyUp/internal/storage"
	"github.com/lachiem1/giddyUp/internal/upapi"
)

const defaultAccountWorkers = 4

type AccountsSyncer struct {
	client    *upapi.Client
	accounts  *storage.AccountsRepo
	syncState *storage.SyncStateRepo
	workers   int
}

func NewAccountsSyncer(
	client *upapi.Client,
	accounts *storage.AccountsRepo,
	syncState *storage.SyncStateRepo,
	workers int,
) *AccountsSyncer {
	if workers <= 0 {
		workers = defaultAccountWorkers
	}
	return &AccountsSyncer{
		client:    client,
		accounts:  accounts,
		syncState: syncState,
		workers:   workers,
	}
}

func (s *AccountsSyncer) Collection() string {
	return CollectionAccounts
}

func (s *AccountsSyncer) HasCachedData(ctx context.Context) (bool, error) {
	return s.accounts.HasActiveAccounts(ctx)
}

func (s *AccountsSyncer) LastSuccessAt(ctx context.Context) (time.Time, bool, error) {
	state, ok, err := s.syncState.Get(ctx, s.Collection())
	if err != nil {
		return time.Time{}, false, err
	}
	if !ok || state.LastSuccess == nil {
		return time.Time{}, false, nil
	}
	return state.LastSuccess.UTC(), true, nil
}

func (s *AccountsSyncer) Sync(ctx context.Context) error {
	attemptAt := time.Now().UTC()
	if err := s.syncState.RecordAttempt(ctx, s.Collection(), attemptAt); err != nil {
		return err
	}

	list, err := s.client.ListAccounts(ctx)
	if err != nil {
		_ = s.syncState.RecordError(context.Background(), s.Collection(), time.Now().UTC(), err)
		return err
	}

	ids := make([]string, 0, len(list.Data))
	for _, res := range list.Data {
		if res.ID == "" {
			continue
		}
		ids = append(ids, res.ID)
	}

	accounts, err := s.fetchAllAccounts(ctx, ids)
	if err != nil {
		_ = s.syncState.RecordError(context.Background(), s.Collection(), time.Now().UTC(), err)
		return err
	}

	fetchedAt := time.Now().UTC()
	if err := s.accounts.ReplaceSnapshot(ctx, accounts, fetchedAt); err != nil {
		_ = s.syncState.RecordError(context.Background(), s.Collection(), time.Now().UTC(), err)
		return err
	}
	if err := s.syncState.RecordSuccess(ctx, s.Collection(), fetchedAt); err != nil {
		return err
	}
	return nil
}

func (s *AccountsSyncer) fetchAllAccounts(ctx context.Context, ids []string) ([]storage.Account, error) {
	if len(ids) == 0 {
		return []storage.Account{}, nil
	}

	workers := s.workers
	if workers > len(ids) {
		workers = len(ids)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan string)
	results := make(chan storage.Account, len(ids))

	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	worker := func() {
		defer wg.Done()
		for id := range jobs {
			if ctx.Err() != nil {
				return
			}
			acct, err := s.fetchAccountByID(ctx, id)
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				errMu.Unlock()
				return
			}
			results <- acct
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}

	go func() {
		defer close(jobs)
		for _, id := range ids {
			select {
			case <-ctx.Done():
				return
			case jobs <- id:
			}
		}
	}()

	wg.Wait()
	close(results)

	errMu.Lock()
	err := firstErr
	errMu.Unlock()
	if err != nil {
		return nil, err
	}

	accounts := make([]storage.Account, 0, len(ids))
	for acct := range results {
		accounts = append(accounts, acct)
	}
	return accounts, nil
}

func (s *AccountsSyncer) fetchAccountByID(ctx context.Context, id string) (storage.Account, error) {
	resp, err := s.client.GetAccount(ctx, id)
	if err != nil {
		return storage.Account{}, fmt.Errorf("get account %q: %w", id, err)
	}
	return mapAccount(resp.Data)
}

func mapAccount(res upapi.Resource) (storage.Account, error) {
	attrs := res.Attributes
	if attrs == nil {
		return storage.Account{}, fmt.Errorf("account %q missing attributes", res.ID)
	}

	displayName, err := stringAttr(attrs, "displayName")
	if err != nil {
		return storage.Account{}, fmt.Errorf("account %q: %w", res.ID, err)
	}
	accountType, err := stringAttr(attrs, "accountType")
	if err != nil {
		return storage.Account{}, fmt.Errorf("account %q: %w", res.ID, err)
	}
	ownershipType, err := stringAttr(attrs, "ownershipType")
	if err != nil {
		return storage.Account{}, fmt.Errorf("account %q: %w", res.ID, err)
	}
	createdAt, err := stringAttr(attrs, "createdAt")
	if err != nil {
		return storage.Account{}, fmt.Errorf("account %q: %w", res.ID, err)
	}

	balanceRaw, ok := attrs["balance"]
	if !ok {
		return storage.Account{}, fmt.Errorf("account %q: missing balance", res.ID)
	}
	balance, ok := balanceRaw.(map[string]any)
	if !ok {
		return storage.Account{}, fmt.Errorf("account %q: invalid balance type %T", res.ID, balanceRaw)
	}
	currencyCode, err := stringAttr(balance, "currencyCode")
	if err != nil {
		return storage.Account{}, fmt.Errorf("account %q: %w", res.ID, err)
	}
	balanceValue, err := stringAttr(balance, "value")
	if err != nil {
		return storage.Account{}, fmt.Errorf("account %q: %w", res.ID, err)
	}
	baseUnits, err := int64Attr(balance, "valueInBaseUnits")
	if err != nil {
		return storage.Account{}, fmt.Errorf("account %q: %w", res.ID, err)
	}

	if res.ID == "" {
		return storage.Account{}, errors.New("account id is empty")
	}

	return storage.Account{
		ID:                      res.ID,
		DisplayName:             displayName,
		AccountType:             accountType,
		OwnershipType:           ownershipType,
		BalanceCurrencyCode:     currencyCode,
		BalanceValue:            balanceValue,
		BalanceValueInBaseUnits: baseUnits,
		CreatedAt:               createdAt,
	}, nil
}

func stringAttr(attrs map[string]any, key string) (string, error) {
	val, ok := attrs[key]
	if !ok {
		return "", fmt.Errorf("missing %s", key)
	}
	str, ok := val.(string)
	if !ok || str == "" {
		return "", fmt.Errorf("invalid %s", key)
	}
	return str, nil
}

func int64Attr(attrs map[string]any, key string) (int64, error) {
	val, ok := attrs[key]
	if !ok {
		return 0, fmt.Errorf("missing %s", key)
	}

	switch n := val.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, fmt.Errorf("invalid %s", key)
		}
		if n != math.Trunc(n) {
			return 0, fmt.Errorf("non-integer %s", key)
		}
		return int64(n), nil
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	default:
		return 0, fmt.Errorf("invalid %s type %T", key, val)
	}
}
