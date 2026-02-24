package syncer

import (
	"context"
	"time"

	"github.com/lachiem1/giddyUp/internal/storage"
)

// runSyncAttempt wraps collection sync work with sync_state bookkeeping.
// The work function returns the timestamp that should be recorded as success.
func runSyncAttempt(
	ctx context.Context,
	syncState *storage.SyncStateRepo,
	collection string,
	work func(context.Context) (time.Time, error),
) error {
	attemptAt := time.Now().UTC()
	if err := syncState.RecordAttempt(ctx, collection, attemptAt); err != nil {
		return err
	}

	successAt, err := work(ctx)
	if err != nil {
		_ = syncState.RecordError(context.Background(), collection, time.Now().UTC(), err)
		return err
	}
	if successAt.IsZero() {
		successAt = time.Now().UTC()
	}
	return syncState.RecordSuccess(ctx, collection, successAt.UTC())
}
