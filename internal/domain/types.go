package domain

import "time"

type State string

const (
	StateRegistered        State = "registered"
	StateClientHold        State = "client_hold"
	StateServerHold        State = "server_hold"
	StateRedemptionPeriod  State = "redemption_period"
	StatePendingDelete     State = "pending_delete"
	StatePossiblyAvailable State = "possibly_available"
	StateAvailable         State = "available"
	StateUnknown           State = "unknown"
	StateError             State = "error"
)

type Domain struct {
	ID                 int64      `json:"id"`
	UnicodeName        string     `json:"domain"`
	ASCIIName          string     `json:"ascii_domain"`
	State              State      `json:"state"`
	RDAPStatuses       []string   `json:"rdap_statuses"`
	Registrar          string     `json:"registrar,omitempty"`
	RegisteredAt       *time.Time `json:"registered_at,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	ExternalUpdatedAt  *time.Time `json:"external_updated_at,omitempty"`
	Nameservers        []string   `json:"nameservers"`
	DNSActive          bool       `json:"dns_active"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	LastCheckAt        *time.Time `json:"last_check_at,omitempty"`
	LastSuccessAt      *time.Time `json:"last_success_at,omitempty"`
	NextCheckAt        time.Time  `json:"next_check_at"`
	ConsecutiveErrors  int        `json:"consecutive_errors"`
	AvailabilityStreak int        `json:"availability_streak"`
	LastError          string     `json:"last_error,omitempty"`
	LastNotification   string     `json:"last_notification_status,omitempty"`
}

type DNSResult struct {
	NS     []string `json:"ns"`
	SOA    []string `json:"soa"`
	A      []string `json:"a"`
	AAAA   []string `json:"aaaa"`
	MX     []string `json:"mx"`
	Active bool     `json:"active"`
	Error  string   `json:"error,omitempty"`
}

type CheckResult struct {
	State             State         `json:"state"`
	RDAPStatuses      []string      `json:"rdap_statuses"`
	Registrar         string        `json:"registrar,omitempty"`
	RegisteredAt      *time.Time    `json:"registered_at,omitempty"`
	ExpiresAt         *time.Time    `json:"expires_at,omitempty"`
	ExternalUpdatedAt *time.Time    `json:"external_updated_at,omitempty"`
	Nameservers       []string      `json:"nameservers"`
	DNS               DNSResult     `json:"dns"`
	CheckedAt         time.Time     `json:"checked_at"`
	Source            string        `json:"source"`
	HTTPStatus        int           `json:"http_status,omitempty"`
	RawResponse       string        `json:"-"`
	Error             string        `json:"error,omitempty"`
	RetryAfter        time.Duration `json:"-"`
}

type Event struct {
	ID        int64          `json:"id"`
	DomainID  int64          `json:"domain_id"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	Message   string         `json:"message"`
	OldValue  string         `json:"old_value,omitempty"`
	NewValue  string         `json:"new_value,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type CheckRecord struct {
	ID         int64     `json:"id"`
	DomainID   int64     `json:"domain_id"`
	State      State     `json:"state"`
	Success    bool      `json:"success"`
	HTTPStatus int       `json:"http_status,omitempty"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type AvailabilityConfirmation struct {
	ID        int64     `json:"id"`
	DomainID  int64     `json:"domain_id"`
	Source    string    `json:"source"`
	Available bool      `json:"available"`
	Details   string    `json:"details,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type HistoryItem struct {
	Kind      string         `json:"kind"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity,omitempty"`
	Message   string         `json:"message"`
	Status    string         `json:"status,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}
