package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Simplarix/dropwatch/internal/app"
	"github.com/Simplarix/dropwatch/internal/config"
	"github.com/Simplarix/dropwatch/internal/domain"
	"github.com/Simplarix/dropwatch/internal/scheduler"
	"github.com/Simplarix/dropwatch/internal/storage"
	"github.com/Simplarix/dropwatch/internal/tui"
	"github.com/Simplarix/dropwatch/internal/version"
	"github.com/spf13/cobra"
)

type options struct {
	configPath     string
	databasePath   string
	logLevel       string
	logFormat      string
	timezone       string
	maxConcurrency int
	httpTimeout    time.Duration
	quiet          bool
}

type commandRuntime struct {
	app      *app.App
	location *time.Location
	out      io.Writer
}

func NewRootCommand() *cobra.Command {
	opts := &options{}
	root := &cobra.Command{
		Use:           "dropwatch",
		Short:         "Monitor domain registration and DNS state",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags := root.PersistentFlags()
	flags.StringVar(&opts.configPath, "config", "", "YAML configuration file")
	flags.StringVar(&opts.databasePath, "database", "", "SQLite database path")
	flags.StringVar(&opts.logLevel, "log-level", "", "debug, info, warn, or error")
	flags.StringVar(&opts.logFormat, "log-format", "", "text or json")
	flags.StringVar(&opts.timezone, "timezone", "", "IANA timezone for output")
	flags.IntVar(&opts.maxConcurrency, "max-concurrency", 0, "maximum concurrent checks")
	flags.DurationVar(&opts.httpTimeout, "http-timeout", 0, "network timeout")
	flags.BoolVar(&opts.quiet, "quiet", false, "suppress normal output")

	root.AddCommand(
		newInitCommand(opts),
		newDomainCommand(opts),
		newCheckCommand(opts),
		newHistoryCommand(opts),
		newWatchCommand(opts),
		newDashboardCommand(opts),
		newNotifyCommand(opts),
		newHealthCommand(opts),
		newVersionCommand(),
	)
	return root
}

func newInitCommand(opts *options) *cobra.Command {
	var writeConfig bool
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "init",
		Short: "Initialize directories, configuration, and database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(opts)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o700); err != nil {
				return fmt.Errorf("create data directory: %w", err)
			}
			store, err := storage.Open(cmd.Context(), cfg.DatabasePath)
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.Migrate(cmd.Context()); err != nil {
				return err
			}
			configCreated := false
			if writeConfig {
				configCreated, err = writeExampleConfig(cfg.ConfigPath)
				if err != nil {
					return err
				}
			}
			result := map[string]any{
				"database": cfg.DatabasePath, "config": cfg.ConfigPath,
				"config_created": configCreated, "rdap_cache": cfg.RDAP.CachePath,
			}
			if opts.quiet {
				return nil
			}
			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "DropWatch initialized\nDatabase:   %s\nConfig:     %s\nRDAP cache: %s\n",
				cfg.DatabasePath, cfg.ConfigPath, cfg.RDAP.CachePath)
			if writeConfig && !configCreated {
				fmt.Fprintln(cmd.OutOrStdout(), "Config already exists and was not overwritten.")
			}
			return nil
		},
	}
	command.Flags().BoolVar(&writeConfig, "write-config", false, "create a sample config if none exists")
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON")
	return command
}

func newDomainCommand(opts *options) *cobra.Command {
	command := &cobra.Command{Use: "domain", Short: "Manage monitored domains"}
	command.AddCommand(newDomainAddCommand(opts), newDomainRemoveCommand(opts), newDomainListCommand(opts), newDomainShowCommand(opts))
	return command
}

func newDomainAddCommand(opts *options) *cobra.Command {
	var noCheck, jsonOutput bool
	command := &cobra.Command{
		Use:   "add DOMAIN",
		Short: "Add a domain and run its first check",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			normalized, err := domain.NormalizeName(args[0])
			if err != nil {
				return err
			}
			item, err := runtime.app.Store.AddDomain(cmd.Context(), normalized, time.Now().UTC())
			if err != nil {
				return err
			}
			var outcome app.CheckOutcome
			if !noCheck {
				outcome, err = runtime.app.Checker.CheckDomain(cmd.Context(), item)
				if outcome.Domain.ID != 0 {
					item = outcome.Domain
				}
			}
			if !opts.quiet {
				if jsonOutput {
					if writeErr := writeJSON(cmd.OutOrStdout(), item); writeErr != nil {
						return writeErr
					}
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Added %s (%s), state: %s\n", item.UnicodeName, item.ASCIIName, item.State)
				}
			}
			return err
		},
	}
	command.Flags().BoolVar(&noCheck, "no-check", false, "skip the initial RDAP/DNS check")
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON")
	return command
}

func newDomainRemoveCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove DOMAIN",
		Short: "Stop monitoring a domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			normalized, err := domain.NormalizeName(args[0])
			if err != nil {
				return err
			}
			if err := runtime.app.Store.RemoveDomain(cmd.Context(), normalized.ASCII, time.Now().UTC()); err != nil {
				return err
			}
			if !opts.quiet {
				fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", normalized.ASCII)
			}
			return nil
		},
	}
}

func newDomainListCommand(opts *options) *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List monitored domains",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			items, err := runtime.app.Store.ListDomains(cmd.Context())
			if err != nil || opts.quiet {
				return err
			}
			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), items)
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "DOMAIN\tSTATE\tRDAP STATUS\tEXPIRES\tNEXT CHECK\tLAST SUCCESS\tERRORS")
			for _, item := range items {
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
					item.UnicodeName, item.State, emptyFallback(strings.Join(item.RDAPStatuses, ",")),
					formatOptional(item.ExpiresAt, runtime.location), formatTime(item.NextCheckAt, runtime.location),
					formatOptional(item.LastSuccessAt, runtime.location), item.ConsecutiveErrors)
			}
			return writer.Flush()
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON")
	return command
}

func newDomainShowCommand(opts *options) *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "show DOMAIN",
		Short: "Show one monitored domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			item, err := getDomain(cmd.Context(), runtime.app.Store, args[0])
			if err != nil || opts.quiet {
				return err
			}
			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), item)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Domain: %s\nASCII: %s\nState: %s\nRDAP status: %s\nRegistrar: %s\nExpires: %s\nNameservers: %s\nDNS active: %t\nNext check: %s\nLast error: %s\n",
				item.UnicodeName, item.ASCIIName, item.State, emptyFallback(strings.Join(item.RDAPStatuses, ", ")),
				emptyFallback(item.Registrar), formatOptional(item.ExpiresAt, runtime.location),
				emptyFallback(strings.Join(item.Nameservers, ", ")), item.DNSActive,
				formatTime(item.NextCheckAt, runtime.location), emptyFallback(item.LastError))
			return nil
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON")
	return command
}

func newCheckCommand(opts *options) *cobra.Command {
	var all, jsonOutput bool
	command := &cobra.Command{
		Use:   "check [DOMAIN]",
		Short: "Run a domain check immediately",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) == 1) {
				return fmt.Errorf("provide exactly one DOMAIN or use --all")
			}
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			var outcomes []app.CheckOutcome
			var checkErrors []error
			if all {
				outcomes, checkErrors = runtime.app.Checker.CheckAll(cmd.Context(), runtime.app.Config.MaxConcurrency)
			} else {
				outcome, checkErr := runtime.app.Checker.CheckName(cmd.Context(), args[0])
				if outcome.Domain.ID != 0 {
					outcomes = append(outcomes, outcome)
				}
				if checkErr != nil {
					checkErrors = append(checkErrors, checkErr)
				}
			}
			if !opts.quiet {
				if jsonOutput {
					if err := writeJSON(cmd.OutOrStdout(), outcomes); err != nil {
						return err
					}
				} else {
					for _, outcome := range outcomes {
						fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (next: %s)\n",
							outcome.Domain.ASCIIName, outcome.Domain.State,
							formatTime(outcome.Domain.NextCheckAt, runtime.location))
					}
				}
			}
			if len(checkErrors) > 0 {
				return fmt.Errorf("%d check(s) failed: %w", len(checkErrors), checkErrors[0])
			}
			return nil
		},
	}
	command.Flags().BoolVar(&all, "all", false, "check every monitored domain")
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON")
	return command
}

func newHistoryCommand(opts *options) *cobra.Command {
	var limit int
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "history DOMAIN",
		Short: "Show domain events, errors, and notification delivery",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			item, err := getDomain(cmd.Context(), runtime.app.Store, args[0])
			if err != nil {
				return err
			}
			items, err := runtime.app.Store.History(cmd.Context(), item.ID, limit)
			if err != nil || opts.quiet {
				return err
			}
			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), items)
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "TIME\tKIND\tTYPE\tSTATUS\tMESSAGE")
			for _, historyItem := range items {
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
					formatTime(historyItem.CreatedAt, runtime.location), historyItem.Kind,
					historyItem.Type, historyItem.Status, historyItem.Message)
			}
			return writer.Flush()
		},
	}
	command.Flags().IntVar(&limit, "limit", 50, "maximum number of history entries")
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON")
	return command
}

func newWatchCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Run the persistent scheduler",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			watcher := scheduler.NewWatcher(runtime.app.Store, runtime.app.Checker, runtime.app.Config, runtime.app.Logger)
			return watcher.Run(cmd.Context())
		},
	}
}

func newDashboardCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Open the interactive terminal dashboard",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			return tui.Run(runtime.app)
		},
	}
}

func newNotifyCommand(opts *options) *cobra.Command {
	parent := &cobra.Command{Use: "notify", Short: "Test notification channels"}
	parent.AddCommand(&cobra.Command{
		Use:   "test",
		Short: "Send a test through every enabled channel",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			enabled := runtime.app.Notifications.Enabled()
			if len(enabled) == 0 {
				return fmt.Errorf("no notification channels are enabled")
			}
			results := runtime.app.Notifications.Test(cmd.Context())
			var failures []string
			for _, name := range enabled {
				if sendErr := results[name]; sendErr != nil {
					failures = append(failures, name)
					if !opts.quiet {
						fmt.Fprintf(cmd.OutOrStdout(), "%s: failed\n", name)
					}
				} else if !opts.quiet {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: sent\n", name)
				}
			}
			if len(failures) > 0 {
				return fmt.Errorf("notification test failed for: %s", strings.Join(failures, ", "))
			}
			return nil
		},
	})
	return parent
}

type healthReport struct {
	Healthy          bool   `json:"healthy"`
	Database         string `json:"database"`
	Migrations       string `json:"migrations"`
	Writable         string `json:"writable"`
	Watcher          string `json:"watcher"`
	HeartbeatAge     string `json:"heartbeat_age,omitempty"`
	SchedulerBlocked bool   `json:"scheduler_blocked"`
}

func newHealthCommand(opts *options) *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "health",
		Short: "Check database and watcher health",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := openRuntime(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer runtime.app.Close()
			report := healthReport{Healthy: true, Database: "ok", Migrations: "ok", Writable: "ok", Watcher: "ok"}
			if err := runtime.app.Store.Ping(cmd.Context()); err != nil {
				report.Healthy, report.Database = false, "failed"
			}
			current, err := runtime.app.Store.MigrationsCurrent(cmd.Context())
			if err != nil || !current {
				report.Healthy, report.Migrations = false, "outdated"
			}
			if err := runtime.app.Store.Writable(cmd.Context()); err != nil {
				report.Healthy, report.Writable = false, "failed"
			}
			_, heartbeat, exists, err := runtime.app.Store.WatcherState(cmd.Context())
			if err != nil || !exists {
				report.Healthy, report.Watcher = false, "missing"
			} else {
				age := time.Since(heartbeat)
				report.HeartbeatAge = age.Round(time.Second).String()
				if age > runtime.app.Config.Scheduler.StaleAfter.Duration {
					report.Healthy, report.Watcher, report.SchedulerBlocked = false, "stale", true
				}
			}
			if !opts.quiet {
				if jsonOutput {
					if err := writeJSON(cmd.OutOrStdout(), report); err != nil {
						return err
					}
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "healthy=%t database=%s migrations=%s writable=%s watcher=%s heartbeat_age=%s\n",
						report.Healthy, report.Database, report.Migrations, report.Writable, report.Watcher, report.HeartbeatAge)
				}
			}
			if !report.Healthy {
				return fmt.Errorf("DropWatch is unhealthy")
			}
			return nil
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON")
	return command
}

func newVersionCommand() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := version.Current()
			if jsonOutput {
				return writeJSON(cmd.OutOrStdout(), info)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "DropWatch %s (commit %s, built %s, %s)\n",
				info.Version, info.Commit, info.BuildTime, info.Go)
			return nil
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "emit JSON")
	return command
}

func openRuntime(ctx context.Context, opts *options) (*commandRuntime, error) {
	cfg, err := loadConfig(opts)
	if err != nil {
		return nil, err
	}
	logOutput := io.Writer(os.Stderr)
	if opts.quiet {
		logOutput = io.Discard
	}
	application, err := app.New(ctx, cfg, logOutput)
	if err != nil {
		return nil, err
	}
	location, _ := time.LoadLocation(cfg.Timezone)
	return &commandRuntime{app: application, location: location, out: os.Stdout}, nil
}

func loadConfig(opts *options) (config.Config, error) {
	return config.Load(config.Overrides{
		ConfigPath: opts.configPath, DatabasePath: opts.databasePath,
		LogLevel: opts.logLevel, LogFormat: opts.logFormat, Timezone: opts.timezone,
		MaxConcurrency: opts.maxConcurrency, HTTPTimeout: opts.httpTimeout,
	})
}

func getDomain(ctx context.Context, store *storage.Store, name string) (domain.Domain, error) {
	normalized, err := domain.NormalizeName(name)
	if err != nil {
		return domain.Domain{}, err
	}
	return store.GetDomain(ctx, normalized.ASCII)
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func formatTime(value time.Time, location *time.Location) string {
	return value.In(location).Format(time.RFC3339)
}

func formatOptional(value *time.Time, location *time.Location) string {
	if value == nil {
		return "-"
	}
	return formatTime(*value, location)
}

func emptyFallback(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func writeExampleConfig(path string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- the local operator explicitly selects this new config path.
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create config: %w", err)
	}
	defer file.Close()
	if _, err := io.WriteString(file, exampleConfig); err != nil {
		return false, fmt.Errorf("write config: %w", err)
	}
	return true, file.Sync()
}

const exampleConfig = `# DropWatch configuration
log_level: info
log_format: text
timezone: Local
max_concurrency: 4
http_timeout: 15s

availability:
  required_confirmations: 3
  minimum_separation: 1m
  confirmation_window: 24h

notifications:
  max_attempts: 3
  retry_delay: 2s
  discord:
    webhook_url: ""
  smtp:
    host: ""
    port: 587
    username: ""
    password: ""
    from: ""
    to: []
    require_tls: true
`
