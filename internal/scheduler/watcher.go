package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Simplarix/dropwatch/internal/config"
	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/Simplarix/dropwatch/internal/storage"
)

type DomainChecker interface {
	CheckScheduled(context.Context, domain.Domain) error
}

type Watcher struct {
	store   storage.SchedulerRepository
	checker DomainChecker
	cfg     config.Config
	logger  *slog.Logger
	owner   string

	mu       sync.Mutex
	inFlight map[int64]struct{}
	wg       sync.WaitGroup
}

func NewWatcher(store storage.SchedulerRepository, checker DomainChecker, cfg config.Config, logger *slog.Logger) *Watcher {
	host, _ := os.Hostname()
	owner := host + ":" + strconv.Itoa(os.Getpid()) + ":" + strconv.FormatInt(time.Now().UnixNano(), 36)
	return &Watcher{
		store: store, checker: checker, cfg: cfg, logger: logger,
		owner: owner, inFlight: make(map[int64]struct{}),
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	acquired, err := w.store.AcquireWatcher(ctx, w.owner, w.cfg.Scheduler.StaleAfter.Duration, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("acquire watcher lock: %w", err)
	}
	if !acquired {
		return fmt.Errorf("another watcher owns this database")
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.store.ReleaseWatcher(releaseCtx, w.owner); err != nil {
			w.logger.Error("release watcher lock", "error", err)
		}
	}()
	w.logger.Info("watcher started", "max_concurrency", w.cfg.MaxConcurrency)
	poll := time.NewTicker(w.cfg.Scheduler.PollInterval.Duration)
	heartbeat := time.NewTicker(w.cfg.Scheduler.Heartbeat.Duration)
	defer poll.Stop()
	defer heartbeat.Stop()
	if err := w.dispatchDue(ctx); err != nil {
		w.logger.Error("initial scheduler poll failed", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return w.waitForWorkers()
		case <-heartbeat.C:
			if err := w.store.Heartbeat(ctx, w.owner, time.Now().UTC()); err != nil {
				if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
					return w.waitForWorkers()
				}
				return fmt.Errorf("watcher heartbeat: %w", err)
			}
		case <-poll.C:
			if err := w.dispatchDue(ctx); err != nil {
				w.logger.Error("scheduler poll failed", "error", err)
			}
		}
	}
}

func (w *Watcher) dispatchDue(ctx context.Context) error {
	w.mu.Lock()
	available := w.cfg.MaxConcurrency - len(w.inFlight)
	w.mu.Unlock()
	if available <= 0 {
		return nil
	}
	items, err := w.store.ListDueDomains(ctx, time.Now().UTC(), available)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !w.markStarted(item.ID) {
			continue
		}
		w.wg.Add(1)
		go w.runCheck(ctx, item)
	}
	return nil
}

func (w *Watcher) runCheck(ctx context.Context, item domain.Domain) {
	defer w.wg.Done()
	defer w.markFinished(item.ID)
	start := time.Now()
	err := w.checker.CheckScheduled(ctx, item)
	if err != nil {
		w.logger.Warn("scheduled domain check failed",
			"domain", item.ASCIIName, "duration", time.Since(start), "error", err)
		return
	}
	w.logger.Info("scheduled domain check completed",
		"domain", item.ASCIIName, "duration", time.Since(start))
}

func (w *Watcher) markStarted(id int64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.inFlight[id]; exists {
		return false
	}
	w.inFlight[id] = struct{}{}
	return true
}

func (w *Watcher) markFinished(id int64) {
	w.mu.Lock()
	delete(w.inFlight, id)
	w.mu.Unlock()
}

func (w *Watcher) waitForWorkers() error {
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(w.cfg.Scheduler.ShutdownTimeout.Duration)
	defer timer.Stop()
	select {
	case <-done:
		w.logger.Info("watcher stopped")
		return nil
	case <-timer.C:
		return fmt.Errorf("watcher shutdown timed out with checks still running")
	}
}
