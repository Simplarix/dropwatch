package scheduler

import (
	"math/rand"
	"testing"
	"time"

	"github.com/Simplarix/dropwatch/internal/config"
	"github.com/Simplarix/dropwatch/internal/domain"
)

func TestAdaptiveIntervals(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults()
	cfg.Intervals.JitterPercent = 0
	calculator := NewIntervalCalculator(cfg, rand.NewSource(1))
	tests := []struct {
		state domain.State
		want  time.Duration
	}{
		{domain.StateRegistered, 6 * time.Hour},
		{domain.StateClientHold, 30 * time.Minute},
		{domain.StateRedemptionPeriod, 15 * time.Minute},
		{domain.StatePendingDelete, 2 * time.Minute},
		{domain.StatePossiblyAvailable, time.Minute},
	}
	for _, test := range tests {
		if got := calculator.Next(test.state, 0, 0); got != test.want {
			t.Errorf("%s = %s, want %s", test.state, got, test.want)
		}
	}
}

func TestBackoffAndRetryAfter(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults()
	cfg.Intervals.JitterPercent = 0
	cfg.Intervals.MaxBackoff = config.Duration{Duration: 6 * time.Hour}
	calculator := NewIntervalCalculator(cfg, rand.NewSource(1))
	if got := calculator.Next(domain.StateError, 3, 0); got != 8*time.Minute {
		t.Fatalf("backoff = %s", got)
	}
	if got := calculator.Next(domain.StateError, 20, 0); got != 6*time.Hour {
		t.Fatalf("capped backoff = %s", got)
	}
	if got := calculator.Next(domain.StateError, 1, 20*time.Minute); got != 20*time.Minute {
		t.Fatalf("Retry-After = %s", got)
	}
}
