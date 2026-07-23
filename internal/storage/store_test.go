package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Simplarix/dropwatch/internal/domain"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "dropwatch.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestMigrationsAndDomainRepository(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()
	current, err := store.MigrationsCurrent(ctx)
	if err != nil || !current {
		t.Fatalf("migrations current=%t err=%v", current, err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	item, err := store.AddDomain(ctx, domain.NormalizedName{Unicode: "bücher.de", ASCII: "xn--bcher-kva.de"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if item.ASCIIName != "xn--bcher-kva.de" || item.State != domain.StateUnknown {
		t.Fatalf("unexpected domain: %#v", item)
	}
	if _, err := store.AddDomain(ctx, domain.NormalizedName{Unicode: "bücher.de", ASCII: "xn--bcher-kva.de"}, now); err == nil {
		t.Fatal("duplicate insert succeeded")
	}
	items, err := store.ListDomains(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("list len=%d err=%v", len(items), err)
	}
	if err := store.RemoveDomain(ctx, item.ASCIIName, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDomain(ctx, item.ASCIIName); err != ErrNotFound {
		t.Fatalf("get removed domain error = %v", err)
	}
}

func TestRecordCheckTransitionsAndNotificationDedupe(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	item, err := store.AddDomain(ctx, domain.NormalizedName{Unicode: "example.com", ASCII: "example.com"}, now)
	if err != nil {
		t.Fatal(err)
	}
	expiration := now.Add(365 * 24 * time.Hour)
	events, err := store.RecordCheck(ctx, item.ID, RecordCheckInput{
		Result: domain.CheckResult{
			State: domain.StateRegistered, RDAPStatuses: []string{"active"},
			ExpiresAt: &expiration, DNS: domain.DNSResult{Active: true},
			CheckedAt: now.Add(time.Second),
		},
		NextCheckAt: now.Add(6 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected transition events")
	}
	record := NotificationRecord{DomainID: item.ID, EventID: events[0].ID, Channel: "mock", DedupeKey: "same", Subject: "s", Body: "b"}
	_, created, err := store.CreateNotification(ctx, record, now)
	if err != nil || !created {
		t.Fatalf("first notification created=%t err=%v", created, err)
	}
	_, created, err = store.CreateNotification(ctx, record, now)
	if err != nil || created {
		t.Fatalf("duplicate notification created=%t err=%v", created, err)
	}
}

func TestWatcherLock(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	acquired, err := store.AcquireWatcher(ctx, "one", time.Minute, now)
	if err != nil || !acquired {
		t.Fatalf("first lock acquired=%t err=%v", acquired, err)
	}
	acquired, err = store.AcquireWatcher(ctx, "two", time.Minute, now.Add(time.Second))
	if err != nil || acquired {
		t.Fatalf("second lock acquired=%t err=%v", acquired, err)
	}
	acquired, err = store.AcquireWatcher(ctx, "two", time.Minute, now.Add(2*time.Minute))
	if err != nil || !acquired {
		t.Fatalf("stale lock acquired=%t err=%v", acquired, err)
	}
}

func TestConcurrentOpenOnWALDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	ctx := context.Background()

	initial, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer initial.Close()
	if err := initial.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	const workers = 16
	start := make(chan struct{})
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			store, openErr := Open(ctx, path)
			if openErr != nil {
				errors <- openErr
				return
			}
			defer store.Close()
			if migrateErr := store.Migrate(ctx); migrateErr != nil {
				errors <- migrateErr
				return
			}
			if writableErr := store.Writable(ctx); writableErr != nil {
				errors <- writableErr
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errors)

	for workerErr := range errors {
		t.Errorf("concurrent database open failed: %v", workerErr)
	}
}
