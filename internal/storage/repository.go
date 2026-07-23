package storage

import (
	"context"
	"time"

	"github.com/Simplarix/dropwatch/internal/domain"
)

// DomainRepository is the persistence contract used by domain-checking logic.
// Store is the SQLite implementation; tests and future backends can substitute it.
type DomainRepository interface {
	GetDomain(context.Context, string) (domain.Domain, error)
	GetDomainByID(context.Context, int64) (domain.Domain, error)
	ListDomains(context.Context) ([]domain.Domain, error)
	RecordCheck(context.Context, int64, RecordCheckInput) ([]domain.Event, error)
	AddAvailabilityConfirmation(context.Context, domain.AvailabilityConfirmation) error
	RecentAvailabilityConfirmations(context.Context, int64, time.Time) ([]domain.AvailabilityConfirmation, error)
}

// SchedulerRepository isolates the persistent watcher lease and due-domain queue.
type SchedulerRepository interface {
	AcquireWatcher(context.Context, string, time.Duration, time.Time) (bool, error)
	Heartbeat(context.Context, string, time.Time) error
	ReleaseWatcher(context.Context, string) error
	ListDueDomains(context.Context, time.Time, int) ([]domain.Domain, error)
}

// NotificationRepository is the durable outbox contract.
type NotificationRepository interface {
	CreateNotification(context.Context, NotificationRecord, time.Time) (int64, bool, error)
	RecordNotificationAttempt(context.Context, int64, int, error, time.Time) error
}
