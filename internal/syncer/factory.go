package syncer

import (
	"database/sql"
	"time"

	"github.com/lachiem1/giddyUp/internal/storage"
	"github.com/lachiem1/giddyUp/internal/upapi"
)

func NewAccountsService(db *sql.DB, client *upapi.Client) (*Service, error) {
	accountsRepo := storage.NewAccountsRepo(db)
	syncStateRepo := storage.NewSyncStateRepo(db)
	accountsSyncer := NewAccountsSyncer(client, accountsRepo, syncStateRepo, defaultAccountWorkers)

	engine, err := New(
		Config{
			StaleTTL:     30 * time.Second,
			PollInterval: 60 * time.Second,
			Backoff:      []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 60 * time.Second},
		},
		[]Syncer{accountsSyncer},
		nil,
	)
	if err != nil {
		return nil, err
	}
	return NewService(engine), nil
}
