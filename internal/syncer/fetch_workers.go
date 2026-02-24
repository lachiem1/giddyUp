package syncer

import (
	"context"
	"sync"
)

// fetchAllByID executes fetch concurrently across ids using a bounded worker pool.
func fetchAllByID[T any](
	ctx context.Context,
	ids []string,
	workers int,
	fetch func(context.Context, string) (T, error),
) ([]T, error) {
	if len(ids) == 0 {
		return []T{}, nil
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > len(ids) {
		workers = len(ids)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan string)
	results := make(chan T, len(ids))

	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	worker := func() {
		defer wg.Done()
		for id := range jobs {
			if ctx.Err() != nil {
				return
			}
			v, err := fetch(ctx, id)
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				errMu.Unlock()
				return
			}
			results <- v
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

	out := make([]T, 0, len(ids))
	for v := range results {
		out = append(out, v)
	}
	return out, nil
}
