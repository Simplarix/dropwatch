package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Simplarix/dropwatch/internal/availability"
	"github.com/Simplarix/dropwatch/internal/config"
	"github.com/Simplarix/dropwatch/internal/dnscheck"
	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/Simplarix/dropwatch/internal/notification"
	"github.com/Simplarix/dropwatch/internal/rdap"
	"github.com/Simplarix/dropwatch/internal/scheduler"
	"github.com/Simplarix/dropwatch/internal/storage"
)

type Checker struct {
	store      storage.DomainRepository
	primary    availability.AvailabilityProvider
	additional []availability.AvailabilityProvider
	dns        dnscheck.Checker
	intervals  *scheduler.IntervalCalculator
	notifier   *notification.Manager
	cfg        config.Config
	logger     *slog.Logger
	now        func() time.Time
}

type CheckOutcome struct {
	Domain            domain.Domain  `json:"domain"`
	Events            []domain.Event `json:"events"`
	NotificationError string         `json:"notification_error,omitempty"`
}

func NewChecker(
	store storage.DomainRepository,
	primary availability.AvailabilityProvider,
	additional []availability.AvailabilityProvider,
	dns dnscheck.Checker,
	intervals *scheduler.IntervalCalculator,
	notifier *notification.Manager,
	cfg config.Config,
	logger *slog.Logger,
) *Checker {
	return &Checker{
		store: store, primary: primary, additional: additional, dns: dns,
		intervals: intervals, notifier: notifier, cfg: cfg, logger: logger,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (c *Checker) CheckName(ctx context.Context, name string) (CheckOutcome, error) {
	normalized, err := domain.NormalizeName(name)
	if err != nil {
		return CheckOutcome{}, err
	}
	item, err := c.store.GetDomain(ctx, normalized.ASCII)
	if err != nil {
		return CheckOutcome{}, err
	}
	return c.CheckDomain(ctx, item)
}

func (c *Checker) CheckDomain(ctx context.Context, item domain.Domain) (CheckOutcome, error) {
	now := c.now()
	dnsChannel := make(chan domain.DNSResult, 1)
	go func() {
		dnsChannel <- c.dns.Check(ctx, item.ASCIIName)
	}()
	providerResult, providerErr := c.primary.Check(ctx, item.ASCIIName)
	dnsResult := <-dnsChannel
	if providerErr != nil && ctx.Err() != nil {
		return CheckOutcome{}, ctx.Err()
	}
	result := domain.CheckResult{
		State: domain.StateError, DNS: dnsResult, CheckedAt: now, Source: c.primary.Name(),
	}
	streak := 0
	retryAfter := time.Duration(0)
	if providerErr != nil {
		result.Error = safeCheckError(providerErr)
		var httpErr *rdap.HTTPError
		if errors.As(providerErr, &httpErr) {
			result.HTTPStatus = httpErr.StatusCode
			retryAfter = httpErr.RetryAfter
		}
	} else {
		result = resultFromProvider(providerResult, dnsResult, now)
		if providerResult.Registered {
			result.State = domain.StateFromRDAP(providerResult.RDAPStatuses, true)
			if err := c.store.AddAvailabilityConfirmation(ctx, domain.AvailabilityConfirmation{
				DomainID: item.ID, Source: c.primary.Name(), Available: false,
				Details: "RDAP domain object returned", CreatedAt: now,
			}); err != nil {
				return CheckOutcome{}, fmt.Errorf("record registration confirmation: %w", err)
			}
		} else if providerResult.Available && providerResult.Authoritative {
			count, err := c.recordAvailableConfirmation(ctx, item.ID, c.primary.Name(), now)
			if err != nil {
				return CheckOutcome{}, err
			}
			streak = count
			result.State = domain.StatePossiblyAvailable
			if count >= c.cfg.Availability.RequiredConfirmations {
				result.State = domain.StateAvailable
			}
		} else {
			result.State = domain.StateUnknown
		}
		result, streak = c.applyAdditionalProviders(ctx, item, result, streak, now)
	}
	errorsAfter := 0
	if result.Error != "" {
		errorsAfter = item.ConsecutiveErrors + 1
	}
	next := now.Add(c.intervals.Next(result.State, errorsAfter, retryAfter))
	events, err := c.store.RecordCheck(ctx, item.ID, storage.RecordCheckInput{
		Result: result, NextCheckAt: next, AvailabilityStreak: streak,
	})
	if err != nil {
		return CheckOutcome{}, err
	}
	updated, err := c.store.GetDomainByID(ctx, item.ID)
	if err != nil {
		return CheckOutcome{}, err
	}
	outcome := CheckOutcome{Domain: updated, Events: events}
	if len(events) > 0 && c.notifier != nil {
		if err := c.notifier.DispatchEvents(ctx, updated, events); err != nil {
			outcome.NotificationError = err.Error()
			c.logger.Error("event notification failed", "domain", item.ASCIIName, "error", err)
		}
	}
	if providerErr != nil {
		return outcome, fmt.Errorf("check %s: %w", item.ASCIIName, providerErr)
	}
	return outcome, nil
}

func (c *Checker) CheckScheduled(ctx context.Context, item domain.Domain) error {
	_, err := c.CheckDomain(ctx, item)
	return err
}

func (c *Checker) CheckAll(ctx context.Context, concurrency int) ([]CheckOutcome, []error) {
	items, err := c.store.ListDomains(ctx)
	if err != nil {
		return nil, []error{err}
	}
	if concurrency < 1 {
		concurrency = 1
	}
	type response struct {
		outcome CheckOutcome
		err     error
	}
	jobs := make(chan domain.Domain, len(items))
	results := make(chan response, len(items))
	for _, item := range items {
		jobs <- item
	}
	close(jobs)
	for range min(concurrency, len(items)) {
		go func() {
			for item := range jobs {
				outcome, checkErr := c.CheckDomain(ctx, item)
				results <- response{outcome: outcome, err: checkErr}
			}
		}()
	}
	var outcomes []CheckOutcome
	var checkErrors []error
	for range items {
		response := <-results
		if response.outcome.Domain.ID != 0 {
			outcomes = append(outcomes, response.outcome)
		}
		if response.err != nil {
			checkErrors = append(checkErrors, response.err)
		}
	}
	return outcomes, checkErrors
}

func (c *Checker) recordAvailableConfirmation(ctx context.Context, id int64, source string, now time.Time) (int, error) {
	since := now.Add(-c.cfg.Availability.ConfirmationWindow.Duration)
	existing, err := c.store.RecentAvailabilityConfirmations(ctx, id, since)
	if err != nil {
		return 0, err
	}
	count := 0
	var last time.Time
	for _, confirmation := range existing {
		if !confirmation.Available {
			count = 0
			last = time.Time{}
			continue
		}
		if last.IsZero() || confirmation.CreatedAt.Sub(last) >= c.cfg.Availability.MinimumSeparation.Duration {
			count++
			last = confirmation.CreatedAt
		}
	}
	if last.IsZero() || now.Sub(last) >= c.cfg.Availability.MinimumSeparation.Duration {
		if err := c.store.AddAvailabilityConfirmation(ctx, domain.AvailabilityConfirmation{
			DomainID: id, Source: source, Available: true,
			Details: "authoritative RDAP 404; registration is not guaranteed", CreatedAt: now,
		}); err != nil {
			return 0, fmt.Errorf("record availability confirmation: %w", err)
		}
		count++
	}
	return count, nil
}

func (c *Checker) applyAdditionalProviders(
	ctx context.Context,
	item domain.Domain,
	result domain.CheckResult,
	streak int,
	now time.Time,
) (domain.CheckResult, int) {
	for _, provider := range c.additional {
		extra, err := provider.Check(ctx, item.ASCIIName)
		if err != nil {
			c.logger.Warn("availability provider failed",
				"provider", provider.Name(), "domain", item.ASCIIName, "error", safeCheckError(err))
			continue
		}
		_ = c.store.AddAvailabilityConfirmation(ctx, domain.AvailabilityConfirmation{
			DomainID: item.ID, Source: provider.Name(), Available: extra.Available,
			Details: "additional provider result", CreatedAt: now,
		})
		if extra.Registered {
			// A registration signal always wins over a possible-availability signal.
			result.State = domain.StateRegistered
			streak = 0
		}
	}
	return result, streak
}

func resultFromProvider(provider availability.AvailabilityResult, dns domain.DNSResult, now time.Time) domain.CheckResult {
	return domain.CheckResult{
		RDAPStatuses: provider.RDAPStatuses, Registrar: provider.Registrar,
		RegisteredAt: provider.RegisteredAt, ExpiresAt: provider.ExpiresAt,
		ExternalUpdatedAt: provider.ExternalUpdatedAt, Nameservers: provider.Nameservers,
		DNS: dns, CheckedAt: now, Source: provider.Source, HTTPStatus: provider.HTTPStatus,
		RawResponse: provider.RawResponse,
	}
}

func safeCheckError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 500 {
		return value[:500]
	}
	return value
}
