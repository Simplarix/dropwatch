package scheduler

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/Simplarix/dropwatch/internal/config"
	"github.com/Simplarix/dropwatch/internal/domain"
)

type IntervalCalculator struct {
	cfg  config.Config
	rand *rand.Rand
	mu   sync.Mutex
}

func NewIntervalCalculator(cfg config.Config, source rand.Source) *IntervalCalculator {
	if source == nil {
		source = rand.NewSource(time.Now().UnixNano())
	}
	return &IntervalCalculator{cfg: cfg, rand: rand.New(source)} // #nosec G404 -- jitter is not used for a security decision.
}

func (c *IntervalCalculator) Next(state domain.State, consecutiveErrors int, retryAfter time.Duration) time.Duration {
	var interval time.Duration
	if consecutiveErrors > 0 || state == domain.StateError || state == domain.StateUnknown {
		exponent := min(consecutiveErrors, 16)
		interval = time.Minute * time.Duration(math.Pow(2, float64(exponent)))
		if interval > c.cfg.Intervals.MaxBackoff.Duration {
			interval = c.cfg.Intervals.MaxBackoff.Duration
		}
	} else {
		switch state {
		case domain.StateRegistered:
			interval = c.cfg.Intervals.Registered.Duration
		case domain.StateClientHold, domain.StateServerHold:
			interval = c.cfg.Intervals.Hold.Duration
		case domain.StateRedemptionPeriod:
			interval = c.cfg.Intervals.RedemptionPeriod.Duration
		case domain.StatePendingDelete:
			interval = c.cfg.Intervals.PendingDelete.Duration
		case domain.StatePossiblyAvailable:
			interval = c.cfg.Intervals.PossiblyAvailable.Duration
		case domain.StateAvailable:
			interval = c.cfg.Intervals.Available.Duration
		default:
			interval = time.Minute
		}
	}
	if retryAfter > interval {
		interval = retryAfter
	}
	return c.withJitter(interval)
}

func (c *IntervalCalculator) withJitter(interval time.Duration) time.Duration {
	percent := c.cfg.Intervals.JitterPercent
	if percent <= 0 || interval <= 0 {
		return interval
	}
	c.mu.Lock()
	factor := 1 + ((c.rand.Float64()*2 - 1) * float64(percent) / 100)
	c.mu.Unlock()
	return time.Duration(float64(interval) * factor)
}
