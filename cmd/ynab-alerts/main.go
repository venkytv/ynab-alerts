package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"ynab-alerts/internal/config"
	"ynab-alerts/internal/heartbeat"
	"ynab-alerts/internal/notifier"
	"ynab-alerts/internal/rules"
	"ynab-alerts/internal/service"
	"ynab-alerts/internal/ynab"
)

var (
	flagToken        string
	flagBudget       string
	flagBaseURL      string
	flagRulesDir     string
	flagNotifier     string
	flagPollInterval string
	flagObservePath  string
	flagAccountsBud  string
	flagDebug        bool
	flagConfigPath   string
	flagDayStart     string
	flagDayEnd       string
	flagHBEnabled    bool
	flagHBNATSURL    string
	flagHBSubject    string
	flagHBPrefix     string
	flagHBInterval   string
	flagHBSkippable  int
	flagHBGrace      string
	flagHBDesc       string
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	rootCmd := &cobra.Command{
		Use:   "ynab-alerts",
		Short: "YNAB alerts daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(ctx, cmd)
		},
	}

	rootCmd.PersistentFlags().StringVar(&flagToken, "token", "", "YNAB API token (overrides YNAB_TOKEN)")
	rootCmd.PersistentFlags().StringVar(&flagBudget, "budget", "", "YNAB budget ID (overrides YNAB_BUDGET_ID)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", "", "YNAB API base URL")
	rootCmd.PersistentFlags().StringVar(&flagRulesDir, "rules", "", "Directory of YAML rule files")
	rootCmd.PersistentFlags().StringVar(&flagNotifier, "notifier", "", "Notifier kind (pushover|log)")
	rootCmd.PersistentFlags().StringVar(&flagPollInterval, "poll", "", "Poll interval (e.g. 1m)")
	rootCmd.PersistentFlags().StringVar(&flagObservePath, "observe-path", "", "Path to observation store (default XDG cache)")
	rootCmd.PersistentFlags().BoolVar(&flagDebug, "debug", false, "Enable debug logging")
	rootCmd.PersistentFlags().StringVar(&flagConfigPath, "config", "", "Path to config file (YAML/JSON)")
	rootCmd.PersistentFlags().StringVar(&flagDayStart, "day-start", "", "Earliest time of day to evaluate (HH:MM, 24h)")
	rootCmd.PersistentFlags().StringVar(&flagDayEnd, "day-end", "", "Latest time of day to evaluate (HH:MM, 24h)")
	rootCmd.PersistentFlags().BoolVar(&flagHBEnabled, "heartbeat", false, "Enable heartbeat publishing")
	rootCmd.PersistentFlags().StringVar(&flagHBNATSURL, "heartbeat-nats-url", "", "NATS URL to publish heartbeats")
	rootCmd.PersistentFlags().StringVar(&flagHBSubject, "heartbeat-subject", "", "Heartbeat subject (appended to prefix)")
	rootCmd.PersistentFlags().StringVar(&flagHBPrefix, "heartbeat-prefix", "", "Heartbeat subject prefix")
	rootCmd.PersistentFlags().StringVar(&flagHBInterval, "heartbeat-interval", "", "Heartbeat interval (e.g. 30s)")
	rootCmd.PersistentFlags().IntVar(&flagHBSkippable, "heartbeat-skippable", 0, "Heartbeats allowed to miss before alerting (0 to disable)")
	rootCmd.PersistentFlags().StringVar(&flagHBGrace, "heartbeat-grace", "", "Grace duration with no heartbeats before alerting (e.g. 2m)")
	rootCmd.PersistentFlags().StringVar(&flagHBDesc, "heartbeat-description", "", "Human-friendly heartbeat description")

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the alerts daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(ctx, cmd)
		},
	}

	listBudgetsCmd := &cobra.Command{
		Use:   "list-budgets",
		Short: "List budgets",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadBaseConfig(cmd)
			if err != nil {
				return err
			}
			token := resolveToken(cfg)
			baseURL := resolveBaseURL(cfg)
			client := ynab.NewClient(token, baseURL)
			return listBudgets(cmd.Context(), client)
		},
	}

	listAccountsCmd := &cobra.Command{
		Use:   "list-accounts",
		Short: "List accounts for a budget",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadBaseConfig(cmd)
			if err != nil {
				return err
			}
			token := resolveToken(cfg)
			baseURL := resolveBaseURL(cfg)
			budget := resolveBudget(cfg, flagAccountsBud)
			if budget == "" {
				return fmt.Errorf("budget ID required via --budget or YNAB_BUDGET_ID")
			}
			client := ynab.NewClient(token, baseURL)
			return listAccounts(cmd.Context(), client, budget)
		},
	}
	listAccountsCmd.Flags().StringVar(&flagAccountsBud, "budget", "", "Budget ID (defaults to persistent flag or env)")

	lintCmd := &cobra.Command{
		Use:   "lint",
		Short: "Lint rule files for common issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			rulesDir := resolveRulesDirForLint(cmd)
			pollInterval := resolvePollIntervalForLint(nil)
			now := time.Now()
			results, err := rules.LintWithPoll(rulesDir, now, pollInterval)
			if err != nil {
				return err
			}
			for _, r := range results {
				next := "unknown"
				if r.HasNext {
					next = r.NextEval.Format(time.RFC3339)
				}
				fmt.Printf("%s:\n  next: %s\n", r.Name, next)
				if len(r.Issues) == 0 {
					fmt.Println("  issues: none")
				} else {
					fmt.Println("  issues:")
					for _, i := range r.Issues {
						fmt.Printf("    - %s\n", i)
					}
				}
			}
			return nil
		},
	}

	rootCmd.AddCommand(runCmd, listBudgetsCmd, listAccountsCmd, lintCmd)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func runDaemon(ctx context.Context, cmd *cobra.Command) error {
	cfg, err := config.Load(resolveConfigPath(cmd))
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	if cmd.Flags().Changed("token") {
		cfg.APIToken = strings.TrimSpace(flagToken)
	}
	if cmd.Flags().Changed("budget") {
		cfg.BudgetID = strings.TrimSpace(flagBudget)
	}
	if cmd.Flags().Changed("base-url") {
		cfg.BaseURL = strings.TrimSpace(flagBaseURL)
	}
	if cmd.Flags().Changed("rules") {
		cfg.RulesDir = strings.TrimSpace(flagRulesDir)
	}
	if cmd.Flags().Changed("notifier") {
		cfg.Notifier = strings.TrimSpace(flagNotifier)
	}
	if cmd.Flags().Changed("observe-path") {
		cfg.ObservePath = strings.TrimSpace(flagObservePath)
	}
	if cmd.Flags().Changed("poll") {
		dur, err := time.ParseDuration(flagPollInterval)
		if err != nil {
			return fmt.Errorf("invalid poll interval: %w", err)
		}
		cfg.PollInterval = dur
	}
	if cmd.Flags().Changed("debug") {
		cfg.Debug = flagDebug
	}
	if cmd.Flags().Changed("day-start") {
		dur, err := config.ParseTimeOfDay(flagDayStart)
		if err != nil {
			return fmt.Errorf("invalid day-start: %w", err)
		}
		cfg.DayStart = dur
	}
	if cmd.Flags().Changed("day-end") {
		dur, err := config.ParseTimeOfDay(flagDayEnd)
		if err != nil {
			return fmt.Errorf("invalid day-end: %w", err)
		}
		cfg.DayEnd = dur
	}
	if cmd.Flags().Changed("heartbeat") {
		cfg.Heartbeat.Enabled = flagHBEnabled
	}
	if cmd.Flags().Changed("heartbeat-nats-url") {
		cfg.Heartbeat.NATSURL = strings.TrimSpace(flagHBNATSURL)
	}
	if cmd.Flags().Changed("heartbeat-subject") {
		cfg.Heartbeat.Subject = strings.TrimSpace(flagHBSubject)
	}
	if cmd.Flags().Changed("heartbeat-prefix") {
		cfg.Heartbeat.Prefix = strings.TrimSpace(flagHBPrefix)
	}
	if cmd.Flags().Changed("heartbeat-description") {
		cfg.Heartbeat.Description = strings.TrimSpace(flagHBDesc)
	}
	if cmd.Flags().Changed("heartbeat-interval") {
		dur, err := time.ParseDuration(flagHBInterval)
		if err != nil {
			return fmt.Errorf("invalid heartbeat-interval: %w", err)
		}
		cfg.Heartbeat.Interval = dur
	}
	if cmd.Flags().Changed("heartbeat-skippable") {
		val := flagHBSkippable
		cfg.Heartbeat.Skippable = &val
	}
	if cmd.Flags().Changed("heartbeat-grace") {
		dur, err := time.ParseDuration(flagHBGrace)
		if err != nil {
			return fmt.Errorf("invalid heartbeat-grace: %w", err)
		}
		cfg.Heartbeat.GracePeriod = &dur
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	daemonCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	store, err := rules.NewStore(cfg.ObservePath)
	if err != nil {
		return fmt.Errorf("observation store error: %w", err)
	}

	notif, err := notifier.Build(notifier.Options{
		Kind: cfg.Notifier,
		Pushover: notifier.PushoverConfig{
			AppToken: cfg.Pushover.AppToken,
			UserKey:  cfg.Pushover.UserKey,
			Device:   cfg.Pushover.Device,
		},
	})
	if err != nil {
		return fmt.Errorf("notifier error: %w", err)
	}

	ynabClient := ynab.NewClient(cfg.APIToken, cfg.BaseURL)
	if cfg.Debug {
		rules.SetDebugLogger(rules.LogDebugLogger{})
	} else {
		rules.SetDebugLogger(nil)
	}
	svc := service.New(cfg, ynabClient, notif, store)

	var stopHeartbeat func()
	if cfg.HeartbeatEnabled() {
		hbStop, err := heartbeat.Start(daemonCtx, cfg.Heartbeat)
		if err != nil {
			return fmt.Errorf("heartbeat error: %w", err)
		}
		stopHeartbeat = hbStop
	}
	if stopHeartbeat != nil {
		defer stopHeartbeat()
	}

	log.Println("ynab-alerts daemon starting")
	if err := svc.Run(daemonCtx); err != nil && daemonCtx.Err() == nil {
		return err
	}
	return nil
}

func resolveBudget(cfg config.Config, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if strings.TrimSpace(flagBudget) != "" {
		return strings.TrimSpace(flagBudget)
	}
	if cfg.BudgetID != "" {
		return cfg.BudgetID
	}
	return ""
}

func resolveRulesDir(cmd *cobra.Command, cfg config.Config) string {
	if cmd != nil && cmd.Flags().Changed("rules") {
		return strings.TrimSpace(flagRulesDir)
	}
	if strings.TrimSpace(flagRulesDir) != "" {
		return strings.TrimSpace(flagRulesDir)
	}
	if cfg.RulesDir != "" {
		return cfg.RulesDir
	}
	return "rules"
}

func resolveRulesDirForLint(cmd *cobra.Command) string {
	if cmd != nil && cmd.Flags().Changed("rules") {
		return strings.TrimSpace(flagRulesDir)
	}
	if strings.TrimSpace(flagRulesDir) != "" {
		return strings.TrimSpace(flagRulesDir)
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_RULES_DIR")); v != "" {
		return v
	}
	return "rules"
}

func resolvePollIntervalForLint(cfg *config.Config) time.Duration {
	if strings.TrimSpace(flagPollInterval) != "" {
		if dur, err := time.ParseDuration(flagPollInterval); err == nil {
			return dur
		}
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_POLL_INTERVAL")); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			return dur
		}
	}
	if cfg != nil && cfg.PollInterval > 0 {
		return cfg.PollInterval
	}
	return config.DefaultPollInterval()
}

func resolveConfigPath(cmd *cobra.Command) string {
	if cmd != nil && cmd.Flags().Changed("config") {
		return strings.TrimSpace(flagConfigPath)
	}
	if strings.TrimSpace(flagConfigPath) != "" {
		return strings.TrimSpace(flagConfigPath)
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_CONFIG")); v != "" {
		return v
	}
	return ""
}

func loadBaseConfig(cmd *cobra.Command) (config.Config, error) {
	cfg, err := config.Load(resolveConfigPath(cmd))
	if err != nil {
		return cfg, fmt.Errorf("config error: %w", err)
	}
	return cfg, nil
}

func resolveToken(cfg config.Config) string {
	if strings.TrimSpace(flagToken) != "" {
		return strings.TrimSpace(flagToken)
	}
	if cfg.APIToken != "" {
		return cfg.APIToken
	}
	log.Fatalf("YNAB_TOKEN is required")
	return ""
}

func resolveBaseURL(cfg config.Config) string {
	if strings.TrimSpace(flagBaseURL) != "" {
		return strings.TrimSpace(flagBaseURL)
	}
	if cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	return "https://api.ynab.com/v1"
}

func listBudgets(ctx context.Context, client *ynab.Client) error {
	budgets, err := client.GetBudgets(ctx)
	if err != nil {
		return err
	}
	for _, b := range budgets {
		sym := ""
		if b.CurrencyFormat != nil && b.CurrencyFormat.DisplaySymbol && b.CurrencyFormat.Symbol != "" {
			sym = b.CurrencyFormat.Symbol
		} else if b.CurrencyFormat != nil && b.CurrencyFormat.ISOCode != "" {
			sym = b.CurrencyFormat.ISOCode
		}
		fmt.Printf("%s\t%s\t%s\n", b.ID, b.Name, sym)
	}
	return nil
}

func listAccounts(ctx context.Context, client *ynab.Client, budgetID string) error {
	var cf *ynab.CurrencyFormat
	if budget, err := client.GetBudget(ctx, budgetID); err == nil {
		cf = budget.CurrencyFormat
	} else {
		log.Printf("warning: could not fetch budget metadata for %s: %v", budgetID, err)
	}
	accounts, err := client.GetAccounts(ctx, budgetID)
	if err != nil {
		return err
	}
	fmt.Printf("Budget: %s\n", budgetID)
	for _, a := range accounts {
		fmt.Printf("%s\t%s\t%s\n", a.ID, a.Name, formatMoney(a.Balance, cf))
	}
	return nil
}

func formatMoney(milli int64, cf *ynab.CurrencyFormat) string {
	sign := ""
	if milli < 0 {
		sign = "-"
		milli = -milli
	}
	decimals := 2
	if cf != nil && cf.DecimalDigits != 0 {
		decimals = cf.DecimalDigits
	}
	unit := float64(milli) / 1000
	formatted := fmt.Sprintf("%s%.*f", sign, decimals, unit)
	if cf != nil && cf.DisplaySymbol && cf.Symbol != "" {
		if cf.SymbolFirst {
			return cf.Symbol + formatted
		}
		return formatted + " " + cf.Symbol
	}
	return formatted
}
