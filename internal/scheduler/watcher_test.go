package scheduler

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Simplarix/dropwatch/internal/config"
	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/Simplarix/dropwatch/internal/storage"
)

type countingChecker struct {
	calls  atomic.Int32
	cancel context.CancelFunc
}

func (c *countingChecker) CheckScheduled(context.Context, domain.Domain) error {
	if c.calls.Add(1) == 1 {
		c.cancel()
	}
	return nil
}

func TestWatcherChecksDueDomainAndReleasesLock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddDomain(ctx,
		domain.NormalizedName{Unicode: "example.com", ASCII: "example.com"},
		time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	checker := &countingChecker{cancel: cancel}
	cfg := config.Defaults()
	cfg.Scheduler.PollInterval = config.Duration{Duration: 5 * time.Millisecond}
	cfg.Scheduler.Heartbeat = config.Duration{Duration: 10 * time.Millisecond}
	cfg.Scheduler.ShutdownTimeout = config.Duration{Duration: time.Second}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	watcher := NewWatcher(store, checker, cfg, logger)
	if err := watcher.Run(runCtx); err != nil {
		t.Fatal(err)
	}
	if checker.calls.Load() != 1 {
		t.Fatalf("calls = %d", checker.calls.Load())
	}
	_, _, exists, err := store.WatcherState(ctx)
	if err != nil || exists {
		t.Fatalf("watcher lock still exists=%t err=%v", exists, err)
	}
}
