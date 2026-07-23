package app

import (
	"context"
	"io"
	"log/slog"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/Simplarix/dropwatch/internal/availability"
	"github.com/Simplarix/dropwatch/internal/config"
	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/Simplarix/dropwatch/internal/scheduler"
	"github.com/Simplarix/dropwatch/internal/storage"
)

type fakeDNS struct {
	result domain.DNSResult
}

func (f fakeDNS) Check(context.Context, string) domain.DNSResult { return f.result }

func TestAvailabilityRequiresSeparatedConfirmations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Availability.RequiredConfirmations = 3
	cfg.Availability.MinimumSeparation = config.Duration{Duration: time.Minute}
	cfg.Intervals.JitterPercent = 0
	provider := &availability.MockProvider{Result: availability.AvailabilityResult{
		Source: "test-rdap", Available: true, Authoritative: true, HTTPStatus: 404,
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := NewChecker(store, provider, nil, fakeDNS{}, scheduler.NewIntervalCalculator(cfg, rand.NewSource(1)), nil, cfg, logger)
	item, err := store.AddDomain(ctx, domain.NormalizedName{Unicode: "example.com", ASCII: "example.com"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	checker.now = func() time.Time { return base }
	first, err := checker.CheckDomain(ctx, item)
	if err != nil || first.Domain.State != domain.StatePossiblyAvailable {
		t.Fatalf("first state=%s err=%v", first.Domain.State, err)
	}
	checker.now = func() time.Time { return base.Add(30 * time.Second) }
	second, err := checker.CheckDomain(ctx, first.Domain)
	if err != nil || second.Domain.AvailabilityStreak != 1 {
		t.Fatalf("too-close streak=%d err=%v", second.Domain.AvailabilityStreak, err)
	}
	checker.now = func() time.Time { return base.Add(time.Minute) }
	third, _ := checker.CheckDomain(ctx, second.Domain)
	checker.now = func() time.Time { return base.Add(2 * time.Minute) }
	final, err := checker.CheckDomain(ctx, third.Domain)
	if err != nil || final.Domain.State != domain.StateAvailable || final.Domain.AvailabilityStreak != 3 {
		t.Fatalf("final state=%s streak=%d err=%v", final.Domain.State, final.Domain.AvailabilityStreak, err)
	}
}

func TestRegisteredSignalIsSaferThanDNSFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Intervals.JitterPercent = 0
	provider := &availability.MockProvider{Result: availability.AvailabilityResult{
		Registered: true, Authoritative: true, RDAPStatuses: []string{"clientHold"}, HTTPStatus: 200,
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	checker := NewChecker(store, provider, nil,
		fakeDNS{result: domain.DNSResult{Active: false, Error: "NXDOMAIN"}},
		scheduler.NewIntervalCalculator(cfg, rand.NewSource(1)), nil, cfg, logger)
	item, _ := store.AddDomain(ctx, domain.NormalizedName{Unicode: "example.com", ASCII: "example.com"}, time.Now().UTC())
	outcome, err := checker.CheckDomain(ctx, item)
	if err != nil || outcome.Domain.State != domain.StateClientHold {
		t.Fatalf("state=%s err=%v", outcome.Domain.State, err)
	}
}
