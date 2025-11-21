package config

import (
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
