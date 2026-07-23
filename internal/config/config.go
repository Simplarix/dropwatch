package config

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	value, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	d.Duration = value
	return nil
}

func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }

type Config struct {
	DatabasePath   string   `yaml:"database_path"`
	ConfigPath     string   `yaml:"-"`
	LogLevel       string   `yaml:"log_level"`
	LogFormat      string   `yaml:"log_format"`
	Timezone       string   `yaml:"timezone"`
	MaxConcurrency int      `yaml:"max_concurrency"`
	HTTPTimeout    Duration `yaml:"http_timeout"`
	UserAgent      string   `yaml:"user_agent"`

	RDAP struct {
		BootstrapURL string   `yaml:"bootstrap_url"`
		CachePath    string   `yaml:"cache_path"`
		CacheTTL     Duration `yaml:"cache_ttl"`
		MaxBodyBytes int64    `yaml:"max_body_bytes"`
	} `yaml:"rdap"`
	Availability struct {
		RequiredConfirmations int      `yaml:"required_confirmations"`
		MinimumSeparation     Duration `yaml:"minimum_separation"`
		ConfirmationWindow    Duration `yaml:"confirmation_window"`
	} `yaml:"availability"`
	Intervals struct {
		Registered        Duration `yaml:"registered"`
		Hold              Duration `yaml:"hold"`
		RedemptionPeriod  Duration `yaml:"redemption_period"`
		PendingDelete     Duration `yaml:"pending_delete"`
		PossiblyAvailable Duration `yaml:"possibly_available"`
		Available         Duration `yaml:"available"`
		MaxBackoff        Duration `yaml:"max_backoff"`
		JitterPercent     int      `yaml:"jitter_percent"`
	} `yaml:"intervals"`
	Notifications struct {
		MaxAttempts int      `yaml:"max_attempts"`
		RetryDelay  Duration `yaml:"retry_delay"`
		Discord     struct {
			WebhookURL string `yaml:"webhook_url"`
		} `yaml:"discord"`
		SMTP struct {
			Host       string   `yaml:"host"`
			Port       int      `yaml:"port"`
			Username   string   `yaml:"username"`
			Password   string   `yaml:"password"`
			From       string   `yaml:"from"`
			To         []string `yaml:"to"`
			RequireTLS bool     `yaml:"require_tls"`
		} `yaml:"smtp"`
	} `yaml:"notifications"`
	Scheduler struct {
		PollInterval    Duration `yaml:"poll_interval"`
		Heartbeat       Duration `yaml:"heartbeat_interval"`
		StaleAfter      Duration `yaml:"stale_after"`
		ShutdownTimeout Duration `yaml:"shutdown_timeout"`
	} `yaml:"scheduler"`
}

type Overrides struct {
	ConfigPath     string
	DatabasePath   string
	LogLevel       string
	LogFormat      string
	Timezone       string
	MaxConcurrency int
	HTTPTimeout    time.Duration
}

func Defaults() Config {
	var cfg Config
	cfg.DatabasePath = defaultDataPath()
	cfg.ConfigPath = defaultConfigPath()
	cfg.LogLevel = "info"
	cfg.LogFormat = "text"
	cfg.Timezone = "Local"
	cfg.MaxConcurrency = 4
	cfg.HTTPTimeout = Duration{15 * time.Second}
	cfg.UserAgent = "DropWatch/dev (+https://github.com/Simplarix/dropwatch)"
	cfg.RDAP.BootstrapURL = "https://data.iana.org/rdap/dns.json"
	cfg.RDAP.CachePath = ""
	cfg.RDAP.CacheTTL = Duration{24 * time.Hour}
	cfg.RDAP.MaxBodyBytes = 2 << 20
	cfg.Availability.RequiredConfirmations = 3
	cfg.Availability.MinimumSeparation = Duration{time.Minute}
	cfg.Availability.ConfirmationWindow = Duration{24 * time.Hour}
	cfg.Intervals.Registered = Duration{6 * time.Hour}
	cfg.Intervals.Hold = Duration{30 * time.Minute}
	cfg.Intervals.RedemptionPeriod = Duration{15 * time.Minute}
	cfg.Intervals.PendingDelete = Duration{2 * time.Minute}
	cfg.Intervals.PossiblyAvailable = Duration{time.Minute}
	cfg.Intervals.Available = Duration{time.Minute}
	cfg.Intervals.MaxBackoff = Duration{6 * time.Hour}
	cfg.Intervals.JitterPercent = 10
	cfg.Notifications.MaxAttempts = 3
	cfg.Notifications.RetryDelay = Duration{2 * time.Second}
	cfg.Notifications.SMTP.Port = 587
	cfg.Notifications.SMTP.RequireTLS = true
	cfg.Scheduler.PollInterval = Duration{10 * time.Second}
	cfg.Scheduler.Heartbeat = Duration{30 * time.Second}
	cfg.Scheduler.StaleAfter = Duration{2 * time.Minute}
	cfg.Scheduler.ShutdownTimeout = Duration{30 * time.Second}
	return cfg
}

func Load(overrides Overrides) (Config, error) {
	cfg := Defaults()
	path := firstNonEmpty(overrides.ConfigPath, os.Getenv("DROPWATCH_CONFIG_PATH"), cfg.ConfigPath)
	cfg.ConfigPath = path
	if data, err := os.ReadFile(path); err == nil { // #nosec G304,G703 -- the local operator explicitly selects the config path.
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg.ConfigPath = path
	if err := applyEnvironment(&cfg); err != nil {
		return Config{}, err
	}
	applyOverrides(&cfg, overrides)
	if cfg.RDAP.CachePath == "" {
		cfg.RDAP.CachePath = filepath.Join(filepath.Dir(cfg.DatabasePath), "rdap-bootstrap.json")
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validate(cfg Config) error {
	if strings.TrimSpace(cfg.DatabasePath) == "" {
		return fmt.Errorf("database_path must not be empty")
	}
	switch strings.ToLower(cfg.LogLevel) {
	case "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("invalid log_level %q", cfg.LogLevel)
	}
	switch strings.ToLower(cfg.LogFormat) {
	case "text", "json":
	default:
		return fmt.Errorf("invalid log_format %q", cfg.LogFormat)
	}
	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", cfg.Timezone, err)
	}
	if cfg.MaxConcurrency < 1 || cfg.MaxConcurrency > 256 {
		return fmt.Errorf("max_concurrency must be between 1 and 256")
	}
	if cfg.HTTPTimeout.Duration <= 0 {
		return fmt.Errorf("http_timeout must be positive")
	}
	if err := validateHTTPSURL(cfg.RDAP.BootstrapURL); err != nil {
		return fmt.Errorf("rdap.bootstrap_url: %w", err)
	}
	if cfg.RDAP.CacheTTL.Duration <= 0 {
		return fmt.Errorf("rdap.cache_ttl must be positive")
	}
	if cfg.RDAP.MaxBodyBytes <= 0 || cfg.RDAP.MaxBodyBytes > 16<<20 {
		return fmt.Errorf("rdap.max_body_bytes must be between 1 and 16777216")
	}
	if cfg.Availability.RequiredConfirmations < 2 || cfg.Availability.RequiredConfirmations > 100 {
		return fmt.Errorf("availability.required_confirmations must be between 2 and 100")
	}
	if cfg.Availability.MinimumSeparation.Duration <= 0 {
		return fmt.Errorf("availability.minimum_separation must be positive")
	}
	if cfg.Availability.ConfirmationWindow.Duration <
		cfg.Availability.MinimumSeparation.Duration*time.Duration(cfg.Availability.RequiredConfirmations-1) {
		return fmt.Errorf("availability.confirmation_window is too short for the required confirmations")
	}
	intervals := map[string]time.Duration{
		"registered":         cfg.Intervals.Registered.Duration,
		"hold":               cfg.Intervals.Hold.Duration,
		"redemption_period":  cfg.Intervals.RedemptionPeriod.Duration,
		"pending_delete":     cfg.Intervals.PendingDelete.Duration,
		"possibly_available": cfg.Intervals.PossiblyAvailable.Duration,
		"available":          cfg.Intervals.Available.Duration,
		"max_backoff":        cfg.Intervals.MaxBackoff.Duration,
	}
	for name, interval := range intervals {
		if interval <= 0 {
			return fmt.Errorf("intervals.%s must be positive", name)
		}
	}
	if cfg.Intervals.JitterPercent < 0 || cfg.Intervals.JitterPercent > 50 {
		return fmt.Errorf("intervals.jitter_percent must be between 0 and 50")
	}
	if cfg.Notifications.MaxAttempts < 1 || cfg.Notifications.MaxAttempts > 20 {
		return fmt.Errorf("notifications.max_attempts must be between 1 and 20")
	}
	if cfg.Notifications.RetryDelay.Duration <= 0 {
		return fmt.Errorf("notifications.retry_delay must be positive")
	}
	if cfg.Notifications.SMTP.Port < 1 || cfg.Notifications.SMTP.Port > 65535 {
		return fmt.Errorf("notifications.smtp.port must be between 1 and 65535")
	}
	if cfg.Notifications.SMTP.Host != "" &&
		!cfg.Notifications.SMTP.RequireTLS &&
		!isLoopbackHost(cfg.Notifications.SMTP.Host) {
		return fmt.Errorf("notifications.smtp.require_tls may be false only for a loopback SMTP server")
	}
	if cfg.Scheduler.PollInterval.Duration <= 0 {
		return fmt.Errorf("scheduler.poll_interval must be positive")
	}
	if cfg.Scheduler.Heartbeat.Duration <= 0 {
		return fmt.Errorf("scheduler.heartbeat_interval must be positive")
	}
	if cfg.Scheduler.StaleAfter.Duration <= cfg.Scheduler.Heartbeat.Duration {
		return fmt.Errorf("scheduler.stale_after must be greater than heartbeat_interval")
	}
	if cfg.Scheduler.ShutdownTimeout.Duration <= 0 {
		return fmt.Errorf("scheduler.shutdown_timeout must be positive")
	}
	return nil
}

func validateHTTPSURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return fmt.Errorf("must be a valid HTTPS URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("must not contain credentials")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return fmt.Errorf("must use the default HTTPS port")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("must use a public host")
	}
	if address, parseErr := netip.ParseAddr(host); parseErr == nil &&
		(!address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast()) {
		return fmt.Errorf("must use a public host")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host).IsLoopback()
}

func applyEnvironment(cfg *Config) error {
	setString(&cfg.DatabasePath, "DROPWATCH_DATABASE_PATH")
	setString(&cfg.LogLevel, "DROPWATCH_LOG_LEVEL")
	setString(&cfg.LogFormat, "DROPWATCH_LOG_FORMAT")
	setString(&cfg.Timezone, "DROPWATCH_TIMEZONE")
	setString(&cfg.Notifications.Discord.WebhookURL, "DROPWATCH_DISCORD_WEBHOOK_URL")
	setString(&cfg.Notifications.SMTP.Host, "DROPWATCH_SMTP_HOST")
	setString(&cfg.Notifications.SMTP.Username, "DROPWATCH_SMTP_USERNAME")
	setString(&cfg.Notifications.SMTP.Password, "DROPWATCH_SMTP_PASSWORD")
	setString(&cfg.Notifications.SMTP.From, "DROPWATCH_SMTP_FROM")
	if value := os.Getenv("DROPWATCH_SMTP_TO"); value != "" {
		cfg.Notifications.SMTP.To = splitCSV(value)
	}
	var err error
	if cfg.MaxConcurrency, err = envInt("DROPWATCH_MAX_CONCURRENCY", cfg.MaxConcurrency); err != nil {
		return err
	}
	if cfg.Notifications.SMTP.Port, err = envInt("DROPWATCH_SMTP_PORT", cfg.Notifications.SMTP.Port); err != nil {
		return err
	}
	if cfg.Notifications.SMTP.RequireTLS, err = envBool("DROPWATCH_SMTP_REQUIRE_TLS", cfg.Notifications.SMTP.RequireTLS); err != nil {
		return err
	}
	if value := os.Getenv("DROPWATCH_HTTP_TIMEOUT"); value != "" {
		duration, parseErr := time.ParseDuration(value)
		if parseErr != nil {
			return fmt.Errorf("DROPWATCH_HTTP_TIMEOUT: %w", parseErr)
		}
		cfg.HTTPTimeout = Duration{duration}
	}
	return nil
}

func applyOverrides(cfg *Config, values Overrides) {
	if values.DatabasePath != "" {
		cfg.DatabasePath = values.DatabasePath
	}
	if values.LogLevel != "" {
		cfg.LogLevel = values.LogLevel
	}
	if values.LogFormat != "" {
		cfg.LogFormat = values.LogFormat
	}
	if values.Timezone != "" {
		cfg.Timezone = values.Timezone
	}
	if values.MaxConcurrency > 0 {
		cfg.MaxConcurrency = values.MaxConcurrency
	}
	if values.HTTPTimeout > 0 {
		cfg.HTTPTimeout = Duration{values.HTTPTimeout}
	}
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(dir, "dropwatch", "config.yaml")
}

func defaultDataPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "dropwatch.db"
	}
	return filepath.Join(dir, "dropwatch", "dropwatch.db")
}

func setString(target *string, key string) {
	if value := os.Getenv(key); value != "" {
		*target = value
	}
}

func envInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return number, nil
}

func envBool(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	result, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return result, nil
}

func splitCSV(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
