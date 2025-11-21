package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings for the daemon.
type Config struct {
	APIToken     string
	BudgetID     string
	BaseURL      string
	RulesDir     string
	PollInterval time.Duration
	Notifier     string
	Pushover     PushoverConfig
	ObservePath  string
}

// PushoverConfig captures credentials for the default notifier.
type PushoverConfig struct {
	AppToken string
	UserKey  string
	Device   string
}

const (
	defaultBaseURL      = "https://api.ynab.com/v1"
	defaultRulesDir     = "rules"
	defaultPollInterval = time.Hour
	defaultNotifier     = "pushover"
)

// DefaultPollInterval returns the baseline daemon poll interval.
func DefaultPollInterval() time.Duration {
	return defaultPollInterval
}

// FromEnv builds a Config from environment variables.
func FromEnv() (Config, error) {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cacheDir = filepath.Join(home, ".cache")
		}
	}
	defaultObserve := filepath.Join(cacheDir, "ynab-alerts", "observations.json")

	cfg := Config{
		APIToken: strings.TrimSpace(os.Getenv("YNAB_TOKEN")),
		BudgetID: strings.TrimSpace(os.Getenv("YNAB_BUDGET_ID")),
		BaseURL:  valueOrDefault(strings.TrimSpace(os.Getenv("YNAB_BASE_URL")), defaultBaseURL),
		RulesDir: valueOrDefault(strings.TrimSpace(os.Getenv("YNAB_RULES_DIR")), defaultRulesDir),
		Notifier: valueOrDefault(strings.TrimSpace(os.Getenv("YNAB_NOTIFIER")), defaultNotifier),
		ObservePath: valueOrDefault(
			strings.TrimSpace(os.Getenv("YNAB_OBSERVATIONS_PATH")),
			defaultObserve,
		),
		Pushover: PushoverConfig{
			AppToken: strings.TrimSpace(os.Getenv("PUSHOVER_APP_TOKEN")),
			UserKey:  strings.TrimSpace(os.Getenv("PUSHOVER_USER_KEY")),
			Device:   strings.TrimSpace(os.Getenv("PUSHOVER_DEVICE")),
		},
		PollInterval: defaultPollInterval,
	}

	if poll := strings.TrimSpace(os.Getenv("YNAB_POLL_INTERVAL")); poll != "" {
		dur, err := time.ParseDuration(poll)
		if err != nil {
			return cfg, err
		}
		cfg.PollInterval = dur
	}

	return cfg, nil
}

// Validate performs consistency checks on the assembled config.
func (c Config) Validate() error {
	if c.APIToken == "" {
		return errors.New("YNAB_TOKEN is required")
	}
	if c.BudgetID == "" {
		return errors.New("YNAB_BUDGET_ID is required")
	}
	if c.Notifier == "pushover" {
		if c.Pushover.AppToken == "" || c.Pushover.UserKey == "" {
			return errors.New("PUSHOVER_APP_TOKEN and PUSHOVER_USER_KEY are required for Pushover")
		}
	}
	if c.PollInterval <= 0 {
		return errors.New("poll interval must be > 0")
	}
	return nil
}

func valueOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

// ParseMilliunits converts a string dollars amount to milliunits if given.
// This is a helper for reading numeric env values expressed in dollars.
func ParseMilliunits(v string) (int64, error) {
	if v == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return int64(f * 1000), nil
}
