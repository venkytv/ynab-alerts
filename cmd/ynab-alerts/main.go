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
			token := resolveToken()
			baseURL := resolveBaseURL()
			client := ynab.NewClient(token, baseURL)
			return listBudgets(cmd.Context(), client)
		},
	}

	listAccountsCmd := &cobra.Command{
		Use:   "list-accounts",
		Short: "List accounts for a budget",
		RunE: func(cmd *cobra.Command, args []string) error {
			token := resolveToken()
			baseURL := resolveBaseURL()
			budget := resolveBudget(flagAccountsBud)
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
			rulesDir := resolveRulesDir(cmd)
			pollInterval := resolvePollIntervalForLint()
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
	cfg, err := config.FromEnv()
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

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config error: %w", err)
	}

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

	log.Println("ynab-alerts daemon starting")
	if err := svc.Run(ctx); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func resolveToken() string {
	if strings.TrimSpace(flagToken) != "" {
		return strings.TrimSpace(flagToken)
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_TOKEN")); v != "" {
		return v
	}
	log.Fatalf("YNAB_TOKEN is required")
	return ""
}

func resolveBaseURL() string {
	if strings.TrimSpace(flagBaseURL) != "" {
		return strings.TrimSpace(flagBaseURL)
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_BASE_URL")); v != "" {
		return v
	}
	return "https://api.ynab.com/v1"
}

func resolveBudget(override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if strings.TrimSpace(flagBudget) != "" {
		return strings.TrimSpace(flagBudget)
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_BUDGET_ID")); v != "" {
		return v
	}
	return ""
}

func resolveRulesDir(cmd *cobra.Command) string {
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

func resolvePollIntervalForLint() time.Duration {
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
	return config.DefaultPollInterval()
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
