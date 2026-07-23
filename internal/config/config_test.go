package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestExampleConfigurationParses(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(filepath.Join("..", "..", "config.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Intervals.PendingDelete.Duration != 2*time.Minute {
		t.Fatalf("pending delete interval = %s", cfg.Intervals.PendingDelete.Duration)
	}
}

func TestLoadPrecedence(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "config.yaml")
	if err := os.WriteFile(path, []byte("database_path: yaml.db\nlog_level: debug\nmax_concurrency: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DROPWATCH_DATABASE_PATH", "environment.db")
	t.Setenv("DROPWATCH_LOG_LEVEL", "warn")
	cfg, err := Load(Overrides{
		ConfigPath: path, DatabasePath: "flag.db", LogLevel: "error", MaxConcurrency: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabasePath != "flag.db" || cfg.LogLevel != "error" || cfg.MaxConcurrency != 8 {
		t.Fatalf("CLI overrides not applied: %#v", cfg)
	}
}

func TestSecureDefaults(t *testing.T) {
	t.Parallel()
	cfg := Defaults()
	if !cfg.Notifications.SMTP.RequireTLS {
		t.Fatal("SMTP TLS must be required by default")
	}
	if err := validate(cfg); err != nil {
		t.Fatalf("default configuration is invalid: %v", err)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("http_timout: 15s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(Overrides{ConfigPath: path})
	if err == nil || !strings.Contains(err.Error(), "field http_timout not found") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestValidateRejectsUnsafeValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*Config)
		match  string
	}{
		{
			name:   "zero HTTP timeout",
			change: func(cfg *Config) { cfg.HTTPTimeout = Duration{} },
			match:  "http_timeout",
		},
		{
			name:   "plaintext bootstrap",
			change: func(cfg *Config) { cfg.RDAP.BootstrapURL = "http://data.iana.org/rdap/dns.json" },
			match:  "HTTPS",
		},
		{
			name:   "negative poll interval",
			change: func(cfg *Config) { cfg.Scheduler.PollInterval = Duration{-time.Second} },
			match:  "poll_interval",
		},
		{
			name:   "unsafe jitter",
			change: func(cfg *Config) { cfg.Intervals.JitterPercent = 100 },
			match:  "jitter_percent",
		},
		{
			name: "short confirmation window",
			change: func(cfg *Config) {
				cfg.Availability.RequiredConfirmations = 3
				cfg.Availability.MinimumSeparation = Duration{time.Hour}
				cfg.Availability.ConfirmationWindow = Duration{time.Hour}
			},
			match: "confirmation_window",
		},
		{
			name: "stale period before heartbeat",
			change: func(cfg *Config) {
				cfg.Scheduler.Heartbeat = Duration{time.Minute}
				cfg.Scheduler.StaleAfter = Duration{time.Minute}
			},
			match: "stale_after",
		},
		{
			name: "plaintext remote SMTP",
			change: func(cfg *Config) {
				cfg.Notifications.SMTP.Host = "smtp.example.com"
				cfg.Notifications.SMTP.RequireTLS = false
			},
			match: "require_tls",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfg := Defaults()
			test.change(&cfg)
			err := validate(cfg)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("expected error containing %q, got %v", test.match, err)
			}
		})
	}
}
