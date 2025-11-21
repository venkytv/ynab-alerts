package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFromEnvDefaultsAndOverrides(t *testing.T) {
	t.Setenv("YNAB_TOKEN", "t123")
	t.Setenv("YNAB_BUDGET_ID", "b123")
	t.Setenv("XDG_CACHE_HOME", "/tmp/testcache")
	t.Setenv("PUSHOVER_APP_TOKEN", "app")
	t.Setenv("PUSHOVER_USER_KEY", "user")
	t.Setenv("YNAB_DEBUG", "true")
	t.Setenv("YNAB_DAY_START", "06:00")
	t.Setenv("YNAB_DAY_END", "22:00")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
	if cfg.APIToken != "t123" || cfg.BudgetID != "b123" {
		t.Fatalf("token/budget not set from env")
	}
	expectedObserve := filepath.Join("/tmp/testcache", "ynab-alerts", "observations.json")
	if cfg.ObservePath != expectedObserve {
		t.Fatalf("observe path mismatch: %s", cfg.ObservePath)
	}
	if cfg.PollInterval != time.Hour {
		t.Fatalf("expected default poll interval 1h, got %s", cfg.PollInterval)
	}
	if !cfg.Debug {
		t.Fatalf("expected debug to be true from env")
	}
	if cfg.DayStart != 6*time.Hour || cfg.DayEnd != 22*time.Hour {
		t.Fatalf("expected day window 06:00-22:00, got %s-%s", cfg.DayStart, cfg.DayEnd)
	}
}

func TestPollIntervalOverride(t *testing.T) {
	t.Setenv("YNAB_TOKEN", "t123")
	t.Setenv("YNAB_BUDGET_ID", "b123")
	t.Setenv("YNAB_POLL_INTERVAL", "30s")
	t.Setenv("PUSHOVER_APP_TOKEN", "app")
	t.Setenv("PUSHOVER_USER_KEY", "user")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PollInterval != 30*time.Second {
		t.Fatalf("expected poll interval 30s, got %s", cfg.PollInterval)
	}
}

func TestObservePathOverride(t *testing.T) {
	t.Setenv("YNAB_TOKEN", "t123")
	t.Setenv("YNAB_BUDGET_ID", "b123")
	t.Setenv("YNAB_OBSERVATIONS_PATH", "/tmp/custom/obs.json")
	t.Setenv("PUSHOVER_APP_TOKEN", "app")
	t.Setenv("PUSHOVER_USER_KEY", "user")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ObservePath != "/tmp/custom/obs.json" {
		t.Fatalf("observe path override failed: %s", cfg.ObservePath)
	}
}

func TestValidateRespectsNotifierKind(t *testing.T) {
	cfg := Config{
		APIToken:     "token",
		BudgetID:     "budget",
		Notifier:     "log",
		PollInterval: time.Hour,
		Debug:        true,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("log notifier should not require pushover creds: %v", err)
	}

	cfg.Notifier = "pushover"
	cfg.Pushover = PushoverConfig{}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error when pushover creds are missing")
	}
}

func TestLoadFromFileAndEnvOverride(t *testing.T) {
	file := t.TempDir() + "/config.yaml"
	content := `
token: file-token
budget_id: file-budget
base_url: https://example.com
rules_dir: conf-rules
poll_interval: 2m
notifier: log
observe_path: /tmp/obs.json
debug: true
day_start: "06:00"
day_end: "21:00"
pushover:
  app_token: file-app
  user_key: file-user
  device: file-device
`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	t.Setenv("YNAB_POLL_INTERVAL", "45s") // env wins

	cfg, err := Load(file)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if cfg.APIToken != "file-token" || cfg.BudgetID != "file-budget" {
		t.Fatalf("expected token/budget from file, got %s/%s", cfg.APIToken, cfg.BudgetID)
	}
	if cfg.BaseURL != "https://example.com" || cfg.RulesDir != "conf-rules" {
		t.Fatalf("file overrides not applied: base %s rules %s", cfg.BaseURL, cfg.RulesDir)
	}
	if cfg.Notifier != "log" || !cfg.Debug {
		t.Fatalf("notifier/debug not applied: %s debug=%v", cfg.Notifier, cfg.Debug)
	}
	if cfg.PollInterval != 45*time.Second {
		t.Fatalf("env poll should win over file: %s", cfg.PollInterval)
	}
	if cfg.ObservePath != "/tmp/obs.json" {
		t.Fatalf("observe path not applied: %s", cfg.ObservePath)
	}
	if cfg.DayStart != 6*time.Hour || cfg.DayEnd != 21*time.Hour {
		t.Fatalf("day window not applied from file: %s-%s", cfg.DayStart, cfg.DayEnd)
	}
	if cfg.Pushover.AppToken != "file-app" || cfg.Pushover.UserKey != "file-user" || cfg.Pushover.Device != "file-device" {
		t.Fatalf("pushover block not loaded: %+v", cfg.Pushover)
	}
}
