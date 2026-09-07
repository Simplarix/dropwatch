package scheduler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Simplarix/dropwatch/internal/config"
	"github.com/Simplarix/dropwatch/internal/domain"
)

type shutdownRepository struct {
	domains   []domain.Domain
	heartbeat func(context.Context) error
	release   func(context.Context) error
}

func (*shutdownRepository) AcquireWatcher(context.Context, string, time.Duration, time.Time) (bool, error) {
	return true, nil
}

func (r *shutdownRepository) Heartbeat(ctx context.Context, _ string, _ time.Time) error {
	if r.heartbeat != nil {
		return r.heartbeat(ctx)
	}
	return nil
}

func (r *shutdownRepository) ReleaseWatcher(ctx context.Context, _ string) error {
	return r.release(ctx)
}

func (r *shutdownRepository) ListDueDomains(context.Context, time.Time, int) ([]domain.Domain, error) {
	domains := r.domains
	r.domains = nil
	return domains, nil
}

type shutdownChecker func(context.Context, domain.Domain) error

func (c shutdownChecker) CheckScheduled(ctx context.Context, item domain.Domain) error {
	return c(ctx, item)
}

func newShutdownWatcher(repo *shutdownRepository, checker DomainChecker) *Watcher {
	cfg := config.Defaults()
	cfg.Scheduler.PollInterval.Duration = time.Hour
	cfg.Scheduler.Heartbeat.Duration = time.Second
	cfg.Scheduler.ShutdownTimeout.Duration = 3 * time.Second
	return NewWatcher(repo, checker, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestWatcherCancellationDuringHeartbeatWaitsForWorkers(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprintf("deadline=%t", deadline), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
				}
				defer cancel()
				var workerDone atomic.Bool
				released, releasedEarly := false, false
				repo := &shutdownRepository{
					domains: []domain.Domain{{ID: 1}},
					heartbeat: func(ctx context.Context) error {
						if !deadline {
							cancel()
						}
						<-ctx.Done()
						return fmt.Errorf("heartbeat query: %w", ctx.Err())
					},
					release: func(ctx context.Context) error {
						released = ctx.Err() == nil
						releasedEarly = !workerDone.Load()
						return ctx.Err()
					},
				}
				checker := shutdownChecker(func(ctx context.Context, _ domain.Domain) error {
					<-ctx.Done()
					time.Sleep(time.Second)
					workerDone.Store(true)
					return ctx.Err()
				})
				err := newShutdownWatcher(repo, checker).Run(ctx)
				// Let a prematurely abandoned worker finish even when the assertion fails.
				time.Sleep(time.Second)
				synctest.Wait()
				if err != nil {
					t.Errorf("graceful shutdown returned %v", err)
				}
				if !released || releasedEarly {
					t.Errorf("lease released=%t before worker finished=%t", released, releasedEarly)
				}
			})
		})
	}
}

func TestWatcherReleaseContextStartsAtShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		released := false
		repo := &shutdownRepository{release: func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			released = ctx.Err() == nil && ok && deadline.Sub(time.Now()) == 5*time.Second
			return ctx.Err()
		}}
		watcher := newShutdownWatcher(repo, nil)
		result := make(chan error, 1)
		go func() { result <- watcher.Run(ctx) }()
		synctest.Wait()
		time.Sleep(10 * time.Second)
		cancel()
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if !released {
			t.Fatal("lease release did not receive a fresh, bounded shutdown context")
		}
	})
}

func TestWatcherPreservesHeartbeatErrors(t *testing.T) {
	for _, heartbeatErr := range []error{errors.New("watcher lease lost"), context.Canceled, context.DeadlineExceeded} {
		t.Run(heartbeatErr.Error(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				released := false
				repo := &shutdownRepository{
					heartbeat: func(context.Context) error { return heartbeatErr },
					release: func(ctx context.Context) error {
						released = ctx.Err() == nil
						return ctx.Err()
					},
				}
				if err := newShutdownWatcher(repo, nil).Run(context.Background()); !errors.Is(err, heartbeatErr) {
					t.Errorf("heartbeat error = %v, want %v", err, heartbeatErr)
				}
				if !released {
					t.Fatal("watcher lease was not released")
				}
			})
		})
	}
}
