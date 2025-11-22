package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fmt"
	"gopkg.in/yaml.v3"
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
	Debug        bool
	DayStart     time.Duration // offset from midnight (optional)
	DayEnd       time.Duration // offset from midnight (optional)
	Heartbeat    HeartbeatConfig
}

// PushoverConfig captures credentials for the default notifier.
type PushoverConfig struct {
	AppToken string
	UserKey  string
	Device   string
}

// HeartbeatConfig controls NATS heartbeat publishing for liveness monitoring.
type HeartbeatConfig struct {
	Enabled     bool
	NATSURL     string
	Subject     string
	Prefix      string
	Interval    time.Duration
	Skippable   *int
	GracePeriod *time.Duration
	Description string
}

const (
	defaultBaseURL      = "https://api.ynab.com/v1"
	defaultRulesDir     = "rules"
	defaultPollInterval = time.Hour
	defaultNotifier     = "pushover"
	defaultHBPfx        = "heartbeat"
	defaultHBNATSURL    = "nats://localhost:4222"
	defaultHBInterval   = time.Minute
	defaultHBDesc       = "YNAB Alerts"
)

// DefaultPollInterval returns the baseline daemon poll interval.
func DefaultPollInterval() time.Duration {
	return defaultPollInterval
}

// DefaultHeartbeatInterval returns the baseline heartbeat publish interval.
func DefaultHeartbeatInterval() time.Duration {
	return defaultHBInterval
}

// FromEnv builds a Config from environment variables.
// Deprecated: prefer Load with a config file path if available.
func FromEnv() (Config, error) {
	return Load("")
}

// Load builds a Config from an optional YAML/JSON file and environment variables.
// CLI flags may further override the returned config.
func Load(filePath string) (Config, error) {
	cfg := defaultConfig()

	if filePath != "" {
		if err := applyFile(&cfg, filePath); err != nil {
			return cfg, err
		}
	}

	if err := applyEnv(&cfg); err != nil {
		return cfg, err
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
	if c.DayStart > 0 && c.DayEnd == 0 {
		c.DayEnd = 24*time.Hour - time.Second // assume end-of-day if only start provided
	}
	if (c.DayStart > 0 || c.DayEnd > 0) && c.DayStart >= c.DayEnd {
		return errors.New("day_start must be before day_end")
	}
	if c.HeartbeatEnabled() {
		if strings.TrimSpace(c.Heartbeat.NATSURL) == "" {
			return errors.New("heartbeat nats_url is required when enabled")
		}
		if strings.TrimSpace(c.Heartbeat.Subject) == "" {
			return errors.New("heartbeat subject is required when enabled")
		}
		if c.Heartbeat.Interval <= 0 {
			return errors.New("heartbeat interval must be > 0")
		}
		if c.Heartbeat.Prefix == "" {
			return errors.New("heartbeat prefix must be set")
		}
		if c.Heartbeat.Skippable != nil && *c.Heartbeat.Skippable < 0 {
			return errors.New("heartbeat skippable cannot be negative")
		}
		if c.Heartbeat.GracePeriod != nil && *c.Heartbeat.GracePeriod < 0 {
			return errors.New("heartbeat grace period cannot be negative")
		}
	}
	return nil
}

// HeartbeatEnabled reports whether heartbeat publishing should run.
func (c Config) HeartbeatEnabled() bool {
	hb := c.Heartbeat
	if hb.Enabled {
		return true
	}
	return strings.TrimSpace(hb.NATSURL) != "" && strings.TrimSpace(hb.Subject) != ""
}

func valueOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseBoolEnv(raw string, current bool) bool {
	if strings.TrimSpace(raw) == "" {
		return current
	}
	return parseBool(raw)
}

func intPtr(v int) *int {
	return &v
}

func durationPtr(v time.Duration) *time.Duration {
	return &v
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

// ParseTimeOfDay converts HH:MM (24h) to a duration offset from midnight.
func ParseTimeOfDay(val string) (time.Duration, error) {
	t, err := time.Parse("15:04", val)
	if err != nil {
		return 0, fmt.Errorf("invalid time of day %q, expected HH:MM", val)
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}

type fileConfig struct {
	Token        string         `yaml:"token"`
	BudgetID     string         `yaml:"budget_id"`
	BaseURL      string         `yaml:"base_url"`
	RulesDir     string         `yaml:"rules_dir"`
	PollInterval string         `yaml:"poll_interval"`
	Notifier     string         `yaml:"notifier"`
	ObservePath  string         `yaml:"observe_path"`
	Debug        *bool          `yaml:"debug"`
	DayStart     string         `yaml:"day_start"`
	DayEnd       string         `yaml:"day_end"`
	Pushover     pushoverBlock  `yaml:"pushover"`
	Heartbeat    heartbeatBlock `yaml:"heartbeat"`
}

type pushoverBlock struct {
	AppToken string `yaml:"app_token"`
	UserKey  string `yaml:"user_key"`
	Device   string `yaml:"device"`
}

type heartbeatBlock struct {
	Enabled     *bool  `yaml:"enabled"`
	NATSURL     string `yaml:"nats_url"`
	Subject     string `yaml:"subject"`
	Prefix      string `yaml:"prefix"`
	Interval    string `yaml:"interval"`
	Skippable   *int   `yaml:"skippable"`
	Grace       string `yaml:"grace"`
	Description string `yaml:"description"`
}

func defaultConfig() Config {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cacheDir = filepath.Join(home, ".cache")
		}
	}
	defaultObserve := filepath.Join(cacheDir, "ynab-alerts", "observations.json")

	return Config{
		APIToken:     "",
		BudgetID:     "",
		BaseURL:      defaultBaseURL,
		RulesDir:     defaultRulesDir,
		PollInterval: defaultPollInterval,
		Notifier:     defaultNotifier,
		Pushover:     PushoverConfig{},
		ObservePath:  defaultObserve,
		Debug:        false,
		DayStart:     0,
		DayEnd:       0,
		Heartbeat: HeartbeatConfig{
			Enabled:     false,
			NATSURL:     defaultHBNATSURL,
			Subject:     "ynab-alerts",
			Prefix:      defaultHBPfx,
			Interval:    defaultHBInterval,
			Skippable:   intPtr(5),
			GracePeriod: durationPtr(10 * time.Minute),
			Description: defaultHBDesc,
		},
	}
}

func applyEnv(cfg *Config) error {
	cfg.APIToken = valueOrDefault(strings.TrimSpace(os.Getenv("YNAB_TOKEN")), cfg.APIToken)
	cfg.BudgetID = valueOrDefault(strings.TrimSpace(os.Getenv("YNAB_BUDGET_ID")), cfg.BudgetID)
	cfg.BaseURL = valueOrDefault(strings.TrimSpace(os.Getenv("YNAB_BASE_URL")), cfg.BaseURL)
	cfg.RulesDir = valueOrDefault(strings.TrimSpace(os.Getenv("YNAB_RULES_DIR")), cfg.RulesDir)
	cfg.Notifier = valueOrDefault(strings.TrimSpace(os.Getenv("YNAB_NOTIFIER")), cfg.Notifier)
	cfg.ObservePath = valueOrDefault(strings.TrimSpace(os.Getenv("YNAB_OBSERVATIONS_PATH")), cfg.ObservePath)
	cfg.Pushover.AppToken = valueOrDefault(strings.TrimSpace(os.Getenv("PUSHOVER_APP_TOKEN")), cfg.Pushover.AppToken)
	cfg.Pushover.UserKey = valueOrDefault(strings.TrimSpace(os.Getenv("PUSHOVER_USER_KEY")), cfg.Pushover.UserKey)
	cfg.Pushover.Device = valueOrDefault(strings.TrimSpace(os.Getenv("PUSHOVER_DEVICE")), cfg.Pushover.Device)

	cfg.Debug = parseBoolEnv(os.Getenv("YNAB_DEBUG"), cfg.Debug)
	if v := strings.TrimSpace(os.Getenv("YNAB_DAY_START")); v != "" {
		if dur, err := ParseTimeOfDay(v); err == nil {
			cfg.DayStart = dur
		} else {
			return err
		}
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_DAY_END")); v != "" {
		if dur, err := ParseTimeOfDay(v); err == nil {
			cfg.DayEnd = dur
		} else {
			return err
		}
	}
	cfg.Heartbeat.Enabled = parseBoolEnv(os.Getenv("YNAB_HEARTBEAT_ENABLED"), cfg.Heartbeat.Enabled)
	if v := strings.TrimSpace(os.Getenv("YNAB_HEARTBEAT_NATS_URL")); v != "" {
		cfg.Heartbeat.NATSURL = v
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_HEARTBEAT_SUBJECT")); v != "" {
		cfg.Heartbeat.Subject = v
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_HEARTBEAT_PREFIX")); v != "" {
		cfg.Heartbeat.Prefix = v
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_HEARTBEAT_DESCRIPTION")); v != "" {
		cfg.Heartbeat.Description = v
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_HEARTBEAT_INTERVAL")); v != "" {
		dur, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		cfg.Heartbeat.Interval = dur
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_HEARTBEAT_SKIPPABLE")); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		cfg.Heartbeat.Skippable = &val
	}
	if v := strings.TrimSpace(os.Getenv("YNAB_HEARTBEAT_GRACE")); v != "" {
		dur, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		cfg.Heartbeat.GracePeriod = &dur
	}

	if poll := strings.TrimSpace(os.Getenv("YNAB_POLL_INTERVAL")); poll != "" {
		dur, err := time.ParseDuration(poll)
		if err != nil {
			return err
		}
		cfg.PollInterval = dur
	}
	return nil
}

func applyFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return err
	}

	if fc.Token != "" {
		cfg.APIToken = strings.TrimSpace(fc.Token)
	}
	if fc.BudgetID != "" {
		cfg.BudgetID = strings.TrimSpace(fc.BudgetID)
	}
	if fc.BaseURL != "" {
		cfg.BaseURL = strings.TrimSpace(fc.BaseURL)
	}
	if fc.RulesDir != "" {
		cfg.RulesDir = strings.TrimSpace(fc.RulesDir)
	}
	if fc.Notifier != "" {
		cfg.Notifier = strings.TrimSpace(fc.Notifier)
	}
	if fc.ObservePath != "" {
		cfg.ObservePath = strings.TrimSpace(fc.ObservePath)
	}
	if fc.PollInterval != "" {
		dur, err := time.ParseDuration(strings.TrimSpace(fc.PollInterval))
		if err != nil {
			return err
		}
		cfg.PollInterval = dur
	}
	if fc.Debug != nil {
		cfg.Debug = *fc.Debug
	}
	if fc.DayStart != "" {
		dur, err := ParseTimeOfDay(strings.TrimSpace(fc.DayStart))
		if err != nil {
			return err
		}
		cfg.DayStart = dur
	}
	if fc.DayEnd != "" {
		dur, err := ParseTimeOfDay(strings.TrimSpace(fc.DayEnd))
		if err != nil {
			return err
		}
		cfg.DayEnd = dur
	}
	if fc.Pushover.AppToken != "" {
		cfg.Pushover.AppToken = strings.TrimSpace(fc.Pushover.AppToken)
	}
	if fc.Pushover.UserKey != "" {
		cfg.Pushover.UserKey = strings.TrimSpace(fc.Pushover.UserKey)
	}
	if fc.Pushover.Device != "" {
		cfg.Pushover.Device = strings.TrimSpace(fc.Pushover.Device)
	}
	if fc.Heartbeat.Enabled != nil {
		cfg.Heartbeat.Enabled = *fc.Heartbeat.Enabled
	}
	if fc.Heartbeat.NATSURL != "" {
		cfg.Heartbeat.NATSURL = strings.TrimSpace(fc.Heartbeat.NATSURL)
	}
	if fc.Heartbeat.Subject != "" {
		cfg.Heartbeat.Subject = strings.TrimSpace(fc.Heartbeat.Subject)
	}
	if fc.Heartbeat.Prefix != "" {
		cfg.Heartbeat.Prefix = strings.TrimSpace(fc.Heartbeat.Prefix)
	}
	if fc.Heartbeat.Description != "" {
		cfg.Heartbeat.Description = strings.TrimSpace(fc.Heartbeat.Description)
	}
	if fc.Heartbeat.Interval != "" {
		dur, err := time.ParseDuration(strings.TrimSpace(fc.Heartbeat.Interval))
		if err != nil {
			return err
		}
		cfg.Heartbeat.Interval = dur
	}
	if fc.Heartbeat.Skippable != nil {
		cfg.Heartbeat.Skippable = fc.Heartbeat.Skippable
	}
	if fc.Heartbeat.Grace != "" {
		dur, err := time.ParseDuration(strings.TrimSpace(fc.Heartbeat.Grace))
		if err != nil {
			return err
		}
		cfg.Heartbeat.GracePeriod = &dur
	}
	return nil
}
