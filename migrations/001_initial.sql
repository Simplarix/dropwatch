CREATE TABLE IF NOT EXISTS domains (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    unicode_name TEXT NOT NULL,
    ascii_name TEXT NOT NULL UNIQUE,
    current_state TEXT NOT NULL DEFAULT 'unknown',
    rdap_statuses TEXT NOT NULL DEFAULT '[]',
    registrar TEXT NOT NULL DEFAULT '',
    registered_at TEXT,
    expires_at TEXT,
    external_updated_at TEXT,
    nameservers TEXT NOT NULL DEFAULT '[]',
    dns_active INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_check_at TEXT,
    last_success_at TEXT,
    next_check_at TEXT NOT NULL,
    consecutive_errors INTEGER NOT NULL DEFAULT 0,
    availability_streak INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    deleted_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_domains_due
    ON domains(next_check_at) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS domain_checks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    state TEXT NOT NULL,
    success INTEGER NOT NULL,
    http_status INTEGER,
    error TEXT NOT NULL DEFAULT '',
    rdap_statuses TEXT NOT NULL DEFAULT '[]',
    registrar TEXT NOT NULL DEFAULT '',
    expires_at TEXT,
    nameservers TEXT NOT NULL DEFAULT '[]',
    dns_result TEXT NOT NULL DEFAULT '{}',
    raw_response TEXT NOT NULL DEFAULT '',
    checked_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_domain_checks_domain_time
    ON domain_checks(domain_id, checked_at DESC);

CREATE TABLE IF NOT EXISTS domain_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    severity TEXT NOT NULL,
    message TEXT NOT NULL,
    old_value TEXT NOT NULL DEFAULT '',
    new_value TEXT NOT NULL DEFAULT '',
    metadata TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_domain_events_domain_time
    ON domain_events(domain_id, created_at DESC);

CREATE TABLE IF NOT EXISTS availability_confirmations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    available INTEGER NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_availability_domain_time
    ON availability_confirmations(domain_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    domain_id INTEGER REFERENCES domains(id) ON DELETE SET NULL,
    event_id INTEGER REFERENCES domain_events(id) ON DELETE SET NULL,
    channel TEXT NOT NULL,
    dedupe_key TEXT NOT NULL UNIQUE,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    sent_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_notifications_status
    ON notifications(status, created_at);

CREATE TABLE IF NOT EXISTS notification_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    notification_id INTEGER NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    attempt INTEGER NOT NULL,
    success INTEGER NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS scheduler_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

