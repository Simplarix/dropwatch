package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/Simplarix/dropwatch/migrations"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := "file:" + filepath.ToSlash(path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// PRAGMA settings are connection-local. A single long-lived connection keeps
	// them effective while network checks still run concurrently.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := configureDatabase(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o600); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("secure database permissions: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func configureDatabase(ctx context.Context, db *sql.DB) error {
	// Set the wait policy before any PRAGMA that may need a database lock. Health
	// checks and the watcher commonly open the same database at the same time.
	for _, setting := range []struct {
		name string
		sql  string
	}{
		{name: "busy timeout", sql: "PRAGMA busy_timeout = 5000"},
		{name: "foreign keys", sql: "PRAGMA foreign_keys = ON"},
		{name: "synchronous mode", sql: "PRAGMA synchronous = NORMAL"},
	} {
		if _, err := db.ExecContext(ctx, setting.sql); err != nil {
			return fmt.Errorf("configure database %s: %w", setting.name, err)
		}
	}

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("read database journal mode: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		if err := db.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
			return fmt.Errorf("enable database WAL mode: %w", err)
		}
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("enable database WAL mode: SQLite selected %q", journalMode)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}
	entries, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(entries)
	for _, name := range entries {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		var exists int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if exists > 0 {
			continue
		}
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations(version, name, applied_at) VALUES(?, ?, ?)",
			version, name, formatTime(time.Now().UTC())); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

func (s *Store) MigrationsCurrent(ctx context.Context) (bool, error) {
	entries, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return false, err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		return false, err
	}
	return count == len(entries), nil
}

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Writable(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx,
		"INSERT INTO scheduler_state(key, value, updated_at) VALUES(?, ?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at",
		"health_probe", "ok", formatTime(time.Now().UTC()))
	return err
}

func (s *Store) AddDomain(ctx context.Context, name domain.NormalizedName, now time.Time) (domain.Domain, error) {
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Domain{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `INSERT INTO domains(
		unicode_name, ascii_name, current_state, created_at, updated_at, next_check_at
	) VALUES(?, ?, ?, ?, ?, ?)`,
		name.Unicode, name.ASCII, domain.StateUnknown, formatTime(now), formatTime(now), formatTime(now))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return domain.Domain{}, fmt.Errorf("domain %s is already monitored", name.ASCII)
		}
		return domain.Domain{}, fmt.Errorf("insert domain: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.Domain{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO domain_events(
		domain_id, event_type, severity, message, new_value, created_at
	) VALUES(?, 'domain_added', 'info', ?, ?, ?)`,
		id, "Domain added", name.ASCII, formatTime(now)); err != nil {
		return domain.Domain{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Domain{}, err
	}
	return s.GetDomain(ctx, name.ASCII)
}

func (s *Store) RemoveDomain(ctx context.Context, asciiName string, now time.Time) error {
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	if err := tx.QueryRowContext(ctx,
		"SELECT id FROM domains WHERE ascii_name = ? AND deleted_at IS NULL", asciiName).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO domain_events(domain_id, event_type, severity, message, old_value, created_at) VALUES(?, 'domain_removed', 'info', 'Domain removed', ?, ?)",
		id, asciiName, formatTime(now)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE domains SET deleted_at = ?, updated_at = ? WHERE id = ?", formatTime(now), formatTime(now), id); err != nil {
		return err
	}
	return tx.Commit()
}

const domainSelect = `SELECT
	d.id, d.unicode_name, d.ascii_name, d.current_state, d.rdap_statuses,
	d.registrar, d.registered_at, d.expires_at, d.external_updated_at,
	d.nameservers, d.dns_active, d.created_at, d.updated_at, d.last_check_at,
	d.last_success_at, d.next_check_at, d.consecutive_errors,
	d.availability_streak, d.last_error,
	COALESCE((SELECT n.status FROM notifications n WHERE n.domain_id=d.id ORDER BY n.created_at DESC LIMIT 1), '')
	FROM domains d`

func (s *Store) GetDomain(ctx context.Context, asciiName string) (domain.Domain, error) {
	row := s.db.QueryRowContext(ctx, domainSelect+" WHERE d.ascii_name = ? AND d.deleted_at IS NULL", asciiName)
	item, err := scanDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Domain{}, ErrNotFound
	}
	return item, err
}

func (s *Store) GetDomainByID(ctx context.Context, id int64) (domain.Domain, error) {
	row := s.db.QueryRowContext(ctx, domainSelect+" WHERE d.id = ? AND d.deleted_at IS NULL", id)
	item, err := scanDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Domain{}, ErrNotFound
	}
	return item, err
}

func (s *Store) ListDomains(ctx context.Context) ([]domain.Domain, error) {
	rows, err := s.db.QueryContext(ctx, domainSelect+" WHERE d.deleted_at IS NULL ORDER BY d.ascii_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Domain
	for rows.Next() {
		item, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListDueDomains(ctx context.Context, now time.Time, limit int) ([]domain.Domain, error) {
	rows, err := s.db.QueryContext(ctx,
		domainSelect+" WHERE d.deleted_at IS NULL AND d.next_check_at <= ? ORDER BY d.next_check_at LIMIT ?",
		formatTime(now.UTC()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.Domain
	for rows.Next() {
		item, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type RecordCheckInput struct {
	Result             domain.CheckResult
	NextCheckAt        time.Time
	AvailabilityStreak int
}

func (s *Store) RecordCheck(ctx context.Context, id int64, input RecordCheckInput) ([]domain.Event, error) {
	before, err := s.GetDomainByID(ctx, id)
	if err != nil {
		return nil, err
	}
	result := input.Result
	result.CheckedAt = result.CheckedAt.UTC()
	if len(result.RawResponse) > 64*1024 {
		result.RawResponse = result.RawResponse[:64*1024]
	}
	statusJSON := marshalJSON(result.RDAPStatuses)
	nameserverJSON := marshalJSON(result.Nameservers)
	dnsJSON := marshalJSON(result.DNS)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	success := result.Error == ""
	if _, err := tx.ExecContext(ctx, `INSERT INTO domain_checks(
		domain_id, state, success, http_status, error, rdap_statuses, registrar,
		expires_at, nameservers, dns_result, raw_response, checked_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, result.State, success, nullableInt(result.HTTPStatus), safeExternalError(result.Error),
		statusJSON, result.Registrar, formatOptionalTime(result.ExpiresAt), nameserverJSON,
		dnsJSON, result.RawResponse, formatTime(result.CheckedAt)); err != nil {
		return nil, fmt.Errorf("record check: %w", err)
	}
	if success {
		if _, err := tx.ExecContext(ctx, `UPDATE domains SET
			current_state=?, rdap_statuses=?, registrar=?, registered_at=?, expires_at=?,
			external_updated_at=?, nameservers=?, dns_active=?, updated_at=?,
			last_check_at=?, last_success_at=?, next_check_at=?, consecutive_errors=0,
			availability_streak=?, last_error=''
			WHERE id=? AND deleted_at IS NULL`,
			result.State, statusJSON, result.Registrar, formatOptionalTime(result.RegisteredAt),
			formatOptionalTime(result.ExpiresAt), formatOptionalTime(result.ExternalUpdatedAt),
			nameserverJSON, result.DNS.Active, formatTime(result.CheckedAt), formatTime(result.CheckedAt),
			formatTime(result.CheckedAt), formatTime(input.NextCheckAt.UTC()), input.AvailabilityStreak, id); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE domains SET
			current_state=?, updated_at=?, last_check_at=?, next_check_at=?,
			consecutive_errors=consecutive_errors+1, last_error=?, availability_streak=0
			WHERE id=? AND deleted_at IS NULL`,
			domain.StateError, formatTime(result.CheckedAt), formatTime(result.CheckedAt),
			formatTime(input.NextCheckAt.UTC()), safeExternalError(result.Error), id); err != nil {
			return nil, err
		}
	}
	events := buildEvents(before, result)
	for index := range events {
		event := &events[index]
		event.DomainID = id
		event.CreatedAt = result.CheckedAt
		res, err := tx.ExecContext(ctx, `INSERT INTO domain_events(
			domain_id, event_type, severity, message, old_value, new_value, metadata, created_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			id, event.Type, event.Severity, event.Message, event.OldValue, event.NewValue,
			marshalJSON(event.Metadata), formatTime(event.CreatedAt))
		if err != nil {
			return nil, err
		}
		event.ID, _ = res.LastInsertId()
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *Store) AddAvailabilityConfirmation(ctx context.Context, confirmation domain.AvailabilityConfirmation) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO availability_confirmations(
		domain_id, source, available, details, created_at
	) VALUES(?, ?, ?, ?, ?)`, confirmation.DomainID, confirmation.Source, confirmation.Available,
		confirmation.Details, formatTime(confirmation.CreatedAt.UTC()))
	return err
}

func (s *Store) RecentAvailabilityConfirmations(ctx context.Context, domainID int64, since time.Time) ([]domain.AvailabilityConfirmation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, domain_id, source, available, details, created_at
		FROM availability_confirmations WHERE domain_id=? AND created_at>=? ORDER BY created_at`,
		domainID, formatTime(since.UTC()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.AvailabilityConfirmation
	for rows.Next() {
		var item domain.AvailabilityConfirmation
		var created string
		if err := rows.Scan(&item.ID, &item.DomainID, &item.Source, &item.Available, &item.Details, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTime(created)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) History(ctx context.Context, domainID int64, limit int) ([]domain.HistoryItem, error) {
	if limit < 1 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT kind, type, severity, message, status, metadata, created_at FROM (
		SELECT 'event' AS kind, event_type AS type, severity, message, '' AS status, metadata, created_at
		FROM domain_events WHERE domain_id=?
		UNION ALL
		SELECT 'notification', channel, '', subject, status, '{}', created_at
		FROM notifications WHERE domain_id=?
		UNION ALL
		SELECT 'check', 'check_failed', 'error', error, 'failed', '{}', checked_at
		FROM domain_checks WHERE domain_id=? AND success=0
	) ORDER BY created_at DESC LIMIT ?`, domainID, domainID, domainID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.HistoryItem
	for rows.Next() {
		var item domain.HistoryItem
		var metadata, created string
		if err := rows.Scan(&item.Kind, &item.Type, &item.Severity, &item.Message, &item.Status, &metadata, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metadata), &item.Metadata)
		item.CreatedAt, _ = parseTime(created)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) AcquireWatcher(ctx context.Context, owner string, staleAfter time.Duration, now time.Time) (bool, error) {
	cutoff := now.UTC().Add(-staleAfter)
	result, err := s.db.ExecContext(ctx, `INSERT INTO scheduler_state(key, value, updated_at)
		VALUES('watcher', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at
		WHERE scheduler_state.updated_at < ? OR scheduler_state.value = ?`,
		owner, formatTime(now.UTC()), formatTime(cutoff), owner)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (s *Store) Heartbeat(ctx context.Context, owner string, now time.Time) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE scheduler_state SET updated_at=? WHERE key='watcher' AND value=?",
		formatTime(now.UTC()), owner)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("watcher lock is no longer owned by this process")
	}
	return nil
}

func (s *Store) ReleaseWatcher(ctx context.Context, owner string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM scheduler_state WHERE key='watcher' AND value=?", owner)
	return err
}

func (s *Store) WatcherState(ctx context.Context) (string, time.Time, bool, error) {
	var owner, updated string
	err := s.db.QueryRowContext(ctx,
		"SELECT value, updated_at FROM scheduler_state WHERE key='watcher'").Scan(&owner, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, err
	}
	parsed, err := parseTime(updated)
	return owner, parsed, true, err
}

type NotificationRecord struct {
	ID        int64
	DomainID  int64
	EventID   int64
	Channel   string
	DedupeKey string
	Subject   string
	Body      string
	Status    string
}

func (s *Store) CreateNotification(ctx context.Context, record NotificationRecord, now time.Time) (int64, bool, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO notifications(
		domain_id, event_id, channel, dedupe_key, subject, body, status, created_at, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, 'pending', ?, ?)
	ON CONFLICT(dedupe_key) DO NOTHING`,
		nullableID(record.DomainID), nullableID(record.EventID), record.Channel, record.DedupeKey,
		record.Subject, record.Body, formatTime(now.UTC()), formatTime(now.UTC()))
	if err != nil {
		return 0, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return 0, false, err
	}
	id, err := result.LastInsertId()
	return id, true, err
}

func (s *Store) RecordNotificationAttempt(ctx context.Context, id int64, attempt int, sendErr error, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	success := sendErr == nil
	message := ""
	status := "sent"
	if sendErr != nil {
		message = safeExternalError(sendErr.Error())
		status = "failed"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_attempts(
		notification_id, attempt, success, error, created_at
	) VALUES(?, ?, ?, ?, ?)`, id, attempt, success, message, formatTime(now.UTC())); err != nil {
		return err
	}
	if success {
		_, err = tx.ExecContext(ctx,
			"UPDATE notifications SET status=?, last_error='', sent_at=?, updated_at=? WHERE id=?",
			status, formatTime(now.UTC()), formatTime(now.UTC()), id)
	} else {
		_, err = tx.ExecContext(ctx,
			"UPDATE notifications SET status=?, last_error=?, updated_at=? WHERE id=?",
			status, message, formatTime(now.UTC()), id)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDomain(row scanner) (domain.Domain, error) {
	var item domain.Domain
	var statuses, nameservers string
	var registered, expires, externalUpdated, lastCheck, lastSuccess sql.NullString
	var created, updated, next string
	if err := row.Scan(
		&item.ID, &item.UnicodeName, &item.ASCIIName, &item.State, &statuses,
		&item.Registrar, &registered, &expires, &externalUpdated, &nameservers,
		&item.DNSActive, &created, &updated, &lastCheck, &lastSuccess, &next,
		&item.ConsecutiveErrors, &item.AvailabilityStreak, &item.LastError, &item.LastNotification,
	); err != nil {
		return domain.Domain{}, err
	}
	_ = json.Unmarshal([]byte(statuses), &item.RDAPStatuses)
	_ = json.Unmarshal([]byte(nameservers), &item.Nameservers)
	item.CreatedAt, _ = parseTime(created)
	item.UpdatedAt, _ = parseTime(updated)
	item.NextCheckAt, _ = parseTime(next)
	item.RegisteredAt = parseOptionalTime(registered)
	item.ExpiresAt = parseOptionalTime(expires)
	item.ExternalUpdatedAt = parseOptionalTime(externalUpdated)
	item.LastCheckAt = parseOptionalTime(lastCheck)
	item.LastSuccessAt = parseOptionalTime(lastSuccess)
	return item, nil
}

func buildEvents(before domain.Domain, result domain.CheckResult) []domain.Event {
	var events []domain.Event
	if result.Error != "" {
		events = append(events, domain.Event{
			Type: "check_failed", Severity: "error", Message: "Domain check failed",
			OldValue: strconv.Itoa(before.ConsecutiveErrors), NewValue: safeExternalError(result.Error),
		})
		return events
	}
	if before.ConsecutiveErrors > 0 {
		events = append(events, domain.Event{Type: "check_recovered", Severity: "info", Message: "Domain checks recovered"})
	}
	if before.State != result.State {
		severity := "info"
		if result.State == domain.StatePendingDelete || domain.IsAvailabilityState(result.State) {
			severity = "critical"
		} else if result.State == domain.StateRedemptionPeriod {
			severity = "warning"
		}
		events = append(events, domain.Event{
			Type: "state_changed", Severity: severity, Message: "Domain state changed",
			OldValue: string(before.State), NewValue: string(result.State),
		})
		switch result.State {
		case domain.StateRedemptionPeriod:
			events = append(events, domain.Event{Type: "redemption_started", Severity: "warning", Message: "Redemption period started"})
		case domain.StatePendingDelete:
			events = append(events, domain.Event{Type: "pending_delete_started", Severity: "critical", Message: "Pending delete started"})
		case domain.StatePossiblyAvailable:
			events = append(events, domain.Event{Type: "possibly_available", Severity: "critical", Message: "Domain may be available; registrar confirmation is still required"})
		case domain.StateAvailable:
			events = append(events, domain.Event{Type: "availability_confirmed", Severity: "critical", Message: "Domain met the configured availability confirmation threshold"})
		case domain.StateRegistered:
			if domain.IsAvailabilityState(before.State) {
				events = append(events, domain.Event{Type: "domain_reregistered", Severity: "warning", Message: "Domain appears to be registered again"})
			}
		}
	}
	if marshalJSON(before.RDAPStatuses) != marshalJSON(result.RDAPStatuses) {
		events = append(events, domain.Event{
			Type: "rdap_status_changed", Severity: "info", Message: "RDAP statuses changed",
			OldValue: strings.Join(before.RDAPStatuses, ","), NewValue: strings.Join(result.RDAPStatuses, ","),
		})
	}
	if optionalTimeString(before.ExpiresAt) != optionalTimeString(result.ExpiresAt) {
		events = append(events, domain.Event{
			Type: "expiration_changed", Severity: "warning", Message: "Expiration date changed",
			OldValue: optionalTimeString(before.ExpiresAt), NewValue: optionalTimeString(result.ExpiresAt),
		})
		if before.ExpiresAt != nil && result.ExpiresAt != nil && result.ExpiresAt.After(*before.ExpiresAt) {
			events = append(events, domain.Event{
				Type: "domain_renewed", Severity: "info", Message: "Domain expiration was extended",
				OldValue: optionalTimeString(before.ExpiresAt), NewValue: optionalTimeString(result.ExpiresAt),
			})
		}
	}
	if before.Registrar != result.Registrar {
		events = append(events, domain.Event{
			Type: "registrar_changed", Severity: "info", Message: "Registrar changed",
			OldValue: before.Registrar, NewValue: result.Registrar,
		})
	}
	if marshalJSON(before.Nameservers) != marshalJSON(result.Nameservers) {
		events = append(events, domain.Event{
			Type: "nameservers_changed", Severity: "info", Message: "Nameservers changed",
			OldValue: strings.Join(before.Nameservers, ","), NewValue: strings.Join(result.Nameservers, ","),
		})
	}
	if before.DNSActive != result.DNS.Active {
		events = append(events, domain.Event{
			Type: "dns_changed", Severity: "warning", Message: "DNS activity changed",
			OldValue: strconv.FormatBool(before.DNSActive), NewValue: strconv.FormatBool(result.DNS.Active),
		})
	}
	return events
}

func migrationVersion(name string) (int, error) {
	prefix, _, found := strings.Cut(name, "_")
	if !found {
		return 0, fmt.Errorf("invalid migration name %q", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("invalid migration name %q", name)
	}
	return version, nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func parseOptionalTime(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

func optionalTimeString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func marshalJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func nullableID(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func safeExternalError(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if len(value) > 1000 {
		value = value[:1000]
	}
	return value
}
