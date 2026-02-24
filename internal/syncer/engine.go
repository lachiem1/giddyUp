package syncer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Syncer interface {
	Collection() string
	HasCachedData(ctx context.Context) (bool, error)
	LastSuccessAt(ctx context.Context) (time.Time, bool, error)
	Sync(ctx context.Context) error
}

type EventType string

const (
	EventSyncStarted EventType = "sync_started"
	EventSyncOK      EventType = "sync_ok"
	EventSyncFailed  EventType = "sync_failed"
)

type Event struct {
	Type       EventType
	Collection string
	At         time.Time
	Err        error
	RetryIn    time.Duration
}

type Config struct {
	StaleTTL     time.Duration
	PollInterval time.Duration
	Backoff      []time.Duration
}

type Engine struct {
	cfg     Config
	syncer  map[string]Syncer
	onEvent func(Event)

	mu     sync.Mutex
	active *activeRun
}

type activeRun struct {
	collection string
	cancel     context.CancelFunc
	manual     chan struct{}
	done       chan struct{}
}

func New(cfg Config, syncers []Syncer, onEvent func(Event)) (*Engine, error) {
	if cfg.StaleTTL <= 0 {
		cfg.StaleTTL = 30 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Minute
	}
	if len(cfg.Backoff) == 0 {
		cfg.Backoff = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 60 * time.Second}
	}

	registry := make(map[string]Syncer, len(syncers))
	for _, s := range syncers {
		if s == nil {
			continue
		}
		collection := s.Collection()
		if collection == "" {
			return nil, errors.New("syncer has empty collection")
		}
		if _, exists := registry[collection]; exists {
			return nil, fmt.Errorf("duplicate syncer for collection %q", collection)
		}
		registry[collection] = s
	}
	if len(registry) == 0 {
		return nil, errors.New("at least one syncer is required")
	}

	return &Engine{cfg: cfg, syncer: registry, onEvent: onEvent}, nil
}

func (e *Engine) EnterView(ctx context.Context, collection string) error {
	e.mu.Lock()
	s, ok := e.syncer[collection]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("no syncer for collection %q", collection)
	}

	if e.active != nil {
		e.active.cancel()
	}

	runCtx, cancel := context.WithCancel(ctx)
	state := &activeRun{
		collection: collection,
		cancel:     cancel,
		manual:     make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
	e.active = state
	e.mu.Unlock()

	go e.runLoop(runCtx, state, s)
	return nil
}

func (e *Engine) LeaveView() {
	e.mu.Lock()
	state := e.active
	e.active = nil
	e.mu.Unlock()

	if state != nil {
		state.cancel()
		<-state.done
	}
}

func (e *Engine) ManualRefresh(collection string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.active == nil {
		return errors.New("no active view")
	}
	if e.active.collection != collection {
		return fmt.Errorf("active view is %q, not %q", e.active.collection, collection)
	}

	select {
	case e.active.manual <- struct{}{}:
	default:
	}
	return nil
}

func (e *Engine) ActiveCollection() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active == nil {
		return ""
	}
	return e.active.collection
}

func (e *Engine) runLoop(ctx context.Context, state *activeRun, s Syncer) {
	defer close(state.done)

	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	var retryTimer *time.Timer
	var retryC <-chan time.Time
	backoffIdx := 0

	shouldSyncNow, err := e.shouldSyncOnEnter(ctx, s)
	if err != nil {
		e.emit(Event{Type: EventSyncFailed, Collection: s.Collection(), At: time.Now().UTC(), Err: err})
	}
	if shouldSyncNow {
		if err := e.attemptSync(ctx, s); err != nil {
			retryTimer, retryC, backoffIdx = scheduleRetry(retryTimer, e.cfg.Backoff, backoffIdx)
		} else {
			backoffIdx = 0
		}
	}

	for {
		select {
		case <-ctx.Done():
			if retryTimer != nil {
				retryTimer.Stop()
			}
			return
		case <-state.manual:
			if retryTimer != nil {
				retryTimer.Stop()
				retryTimer = nil
				retryC = nil
			}
			if err := e.attemptSync(ctx, s); err != nil {
				retryTimer, retryC, backoffIdx = scheduleRetry(retryTimer, e.cfg.Backoff, backoffIdx)
			} else {
				backoffIdx = 0
			}
		case <-ticker.C:
			if retryC != nil {
				continue
			}
			if err := e.attemptSync(ctx, s); err != nil {
				retryTimer, retryC, backoffIdx = scheduleRetry(retryTimer, e.cfg.Backoff, backoffIdx)
			} else {
				backoffIdx = 0
			}
		case <-retryC:
			retryTimer = nil
			retryC = nil
			if err := e.attemptSync(ctx, s); err != nil {
				retryTimer, retryC, backoffIdx = scheduleRetry(retryTimer, e.cfg.Backoff, backoffIdx)
			} else {
				backoffIdx = 0
			}
		}
	}
}

func scheduleRetry(current *time.Timer, backoff []time.Duration, index int) (*time.Timer, <-chan time.Time, int) {
	if current != nil {
		current.Stop()
	}
	if index >= len(backoff) {
		index = len(backoff) - 1
	}
	t := time.NewTimer(backoff[index])
	nextIdx := index + 1
	if nextIdx >= len(backoff) {
		nextIdx = len(backoff) - 1
	}
	return t, t.C, nextIdx
}

func (e *Engine) shouldSyncOnEnter(ctx context.Context, s Syncer) (bool, error) {
	hasData, err := s.HasCachedData(ctx)
	if err != nil {
		return false, err
	}
	if !hasData {
		return true, nil
	}

	lastSuccess, ok, err := s.LastSuccessAt(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return time.Since(lastSuccess) > e.cfg.StaleTTL, nil
}

func (e *Engine) attemptSync(ctx context.Context, s Syncer) error {
	e.emit(Event{Type: EventSyncStarted, Collection: s.Collection(), At: time.Now().UTC()})
	err := s.Sync(ctx)
	if err != nil {
		e.emit(Event{Type: EventSyncFailed, Collection: s.Collection(), At: time.Now().UTC(), Err: err})
		return err
	}
	e.emit(Event{Type: EventSyncOK, Collection: s.Collection(), At: time.Now().UTC()})
	return nil
}

func (e *Engine) emit(evt Event) {
	if e.onEvent == nil {
		return
	}
	e.onEvent(evt)
}
