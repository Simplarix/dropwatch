package notification

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/Simplarix/dropwatch/internal/storage"
)

type Notification struct {
	Domain     string
	Previous   domain.State
	Current    domain.State
	RDAPStatus []string
	ExpiresAt  *time.Time
	Time       time.Time
	Urgency    string
	Subject    string
	Message    string
	Error      string
}

type Notifier interface {
	Name() string
	Send(ctx context.Context, notification Notification) error
}

type Manager struct {
	store       storage.NotificationRepository
	notifiers   []Notifier
	maxAttempts int
	retryDelay  time.Duration
	logger      *slog.Logger
}

func NewManager(store storage.NotificationRepository, notifiers []Notifier, maxAttempts int, retryDelay time.Duration, logger *slog.Logger) *Manager {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return &Manager{
		store: store, notifiers: notifiers, maxAttempts: maxAttempts,
		retryDelay: retryDelay, logger: logger,
	}
}

func (m *Manager) Enabled() []string {
	names := make([]string, 0, len(m.notifiers))
	for _, notifier := range m.notifiers {
		names = append(names, notifier.Name())
	}
	return names
}

func (m *Manager) DispatchEvents(ctx context.Context, item domain.Domain, events []domain.Event) error {
	var allErrors []string
	for _, event := range events {
		notification := eventNotification(item, event)
		for _, notifier := range m.notifiers {
			dedupe := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", event.ID, notifier.Name(), event.Type))))
			record := storage.NotificationRecord{
				DomainID: item.ID, EventID: event.ID, Channel: notifier.Name(),
				DedupeKey: dedupe, Subject: notification.Subject, Body: notification.Message,
			}
			id, created, err := m.store.CreateNotification(ctx, record, time.Now().UTC())
			if err != nil {
				allErrors = append(allErrors, notifier.Name()+": persist notification")
				continue
			}
			if !created {
				continue
			}
			if err := m.sendWithRetry(ctx, notifier, id, notification); err != nil {
				allErrors = append(allErrors, notifier.Name()+": "+safeError(err))
			}
		}
	}
	if len(allErrors) > 0 {
		return fmt.Errorf("notification delivery failed: %s", strings.Join(allErrors, "; "))
	}
	return nil
}

func (m *Manager) Test(ctx context.Context) map[string]error {
	results := make(map[string]error, len(m.notifiers))
	now := time.Now().UTC()
	for _, notifier := range m.notifiers {
		message := Notification{
			Domain: "example.com", Previous: domain.StateUnknown, Current: domain.StateRegistered,
			Time: now, Urgency: "info", Subject: "DropWatch test notification",
			Message: "This is a DropWatch test notification.",
		}
		key := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("test:%s:%d", notifier.Name(), now.UnixNano()))))
		id, _, err := m.store.CreateNotification(ctx, storage.NotificationRecord{
			Channel: notifier.Name(), DedupeKey: key, Subject: message.Subject, Body: message.Message,
		}, now)
		if err == nil {
			err = m.sendWithRetry(ctx, notifier, id, message)
		}
		results[notifier.Name()] = err
	}
	return results
}

func (m *Manager) sendWithRetry(ctx context.Context, notifier Notifier, id int64, message Notification) error {
	var lastErr error
	for attempt := 1; attempt <= m.maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = notifier.Send(ctx, message)
		recordErr := m.store.RecordNotificationAttempt(ctx, id, attempt, lastErr, time.Now().UTC())
		if recordErr != nil {
			return fmt.Errorf("record %s delivery attempt: %w", notifier.Name(), recordErr)
		}
		if lastErr == nil {
			return nil
		}
		m.logger.Warn("notification attempt failed",
			"channel", notifier.Name(), "attempt", attempt, "error", safeError(lastErr))
		if attempt < m.maxAttempts {
			timer := time.NewTimer(m.retryDelay * time.Duration(attempt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return lastErr
}

func eventNotification(item domain.Domain, event domain.Event) Notification {
	previous := item.State
	if event.Type == "state_changed" {
		previous = domain.State(event.OldValue)
	}
	current := item.State
	message := fmt.Sprintf("%s: %s", item.ASCIIName, event.Message)
	if event.OldValue != "" || event.NewValue != "" {
		message += fmt.Sprintf(" (%s → %s)", event.OldValue, event.NewValue)
	}
	result := Notification{
		Domain: item.ASCIIName, Previous: previous, Current: current,
		RDAPStatus: item.RDAPStatuses, ExpiresAt: item.ExpiresAt, Time: event.CreatedAt,
		Urgency: event.Severity, Subject: fmt.Sprintf("DropWatch [%s] %s", strings.ToUpper(event.Severity), item.ASCIIName),
		Message: message,
	}
	if event.Type == "check_failed" {
		result.Error = event.NewValue
	}
	return result
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(value) > 300 {
		return value[:300]
	}
	return value
}
