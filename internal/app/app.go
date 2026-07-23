package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/Simplarix/dropwatch/internal/availability"
	"github.com/Simplarix/dropwatch/internal/config"
	"github.com/Simplarix/dropwatch/internal/dnscheck"
	"github.com/Simplarix/dropwatch/internal/notification"
	"github.com/Simplarix/dropwatch/internal/rdap"
	"github.com/Simplarix/dropwatch/internal/scheduler"
	"github.com/Simplarix/dropwatch/internal/storage"
)

type App struct {
	Config        config.Config
	Store         *storage.Store
	Checker       *Checker
	Notifications *notification.Manager
	Logger        *slog.Logger
}

func New(ctx context.Context, cfg config.Config, output io.Writer) (*App, error) {
	logger, err := NewLogger(cfg.LogLevel, cfg.LogFormat, output)
	if err != nil {
		return nil, err
	}
	store, err := storage.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	rdapClient := rdap.NewClient(
		cfg.HTTPTimeout.Duration, cfg.RDAP.BootstrapURL, cfg.RDAP.CachePath,
		cfg.RDAP.CacheTTL.Duration, cfg.RDAP.MaxBodyBytes, cfg.UserAgent,
	)
	provider := availability.NewRDAPProvider(rdapClient)
	dns := dnscheck.New(cfg.HTTPTimeout.Duration)
	notifiers, err := configuredNotifiers(cfg)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	manager := notification.NewManager(
		store, notifiers, cfg.Notifications.MaxAttempts,
		cfg.Notifications.RetryDelay.Duration, logger,
	)
	intervals := scheduler.NewIntervalCalculator(cfg, nil)
	checker := NewChecker(store, provider, nil, dns, intervals, manager, cfg, logger)
	return &App{
		Config: cfg, Store: store, Checker: checker,
		Notifications: manager, Logger: logger,
	}, nil
}

func (a *App) Close() error { return a.Store.Close() }

func NewLogger(levelName, format string, output io.Writer) (*slog.Logger, error) {
	var level slog.Level
	switch strings.ToLower(levelName) {
	case "debug":
		level = slog.LevelDebug
	case "", "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q", levelName)
	}
	options := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch strings.ToLower(format) {
	case "", "text":
		handler = slog.NewTextHandler(output, options)
	case "json":
		handler = slog.NewJSONHandler(output, options)
	default:
		return nil, fmt.Errorf("invalid log format %q", format)
	}
	return slog.New(handler), nil
}

func configuredNotifiers(cfg config.Config) ([]notification.Notifier, error) {
	var notifiers []notification.Notifier
	if cfg.Notifications.Discord.WebhookURL != "" {
		discord, err := notification.NewDiscord(cfg.Notifications.Discord.WebhookURL, cfg.HTTPTimeout.Duration)
		if err != nil {
			return nil, fmt.Errorf("configure Discord: %w", err)
		}
		notifiers = append(notifiers, discord)
	}
	if cfg.Notifications.SMTP.Host != "" {
		smtpNotifier, err := notification.NewSMTP(notification.SMTPConfig{
			Host: cfg.Notifications.SMTP.Host, Port: cfg.Notifications.SMTP.Port,
			Username: cfg.Notifications.SMTP.Username, Password: cfg.Notifications.SMTP.Password,
			From: cfg.Notifications.SMTP.From, To: cfg.Notifications.SMTP.To,
			RequireTLS: cfg.Notifications.SMTP.RequireTLS, Timeout: cfg.HTTPTimeout.Duration,
		})
		if err != nil {
			return nil, fmt.Errorf("configure SMTP: %w", err)
		}
		notifiers = append(notifiers, smtpNotifier)
	}
	return notifiers, nil
}

func DefaultOutput() io.Writer { return os.Stderr }
