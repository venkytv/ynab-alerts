# ynab-alerts

Daemon that polls YNAB and emits alerts when rule conditions are met.

## Quick start
1. Configure credentials and defaults (config file or env):
   - Config file (YAML/JSON) via `--config` or `YNAB_CONFIG`, e.g.:
     ```yaml
     token: $YNAB_TOKEN
     budget_id: your-budget-id
     rules_dir: rules/
     poll_interval: 1h
     observe_path: ~/.cache/ynab-alerts/observations.json
     notifier: pushover
     debug: false
     day_start: "06:00"
     day_end: "22:00"
     pushover:
       app_token: ""
       user_key: ""
       device: ""
     heartbeat:
       enabled: true
       nats_url: "nats://localhost:4222"
       subject: "ynab-alerts"
       prefix: "heartbeat"
       interval: 1m
       grace: 10m
       description: "YNAB Alerts daemon"
     ```
   - Or environment:
     - `YNAB_TOKEN` — personal access token.
     - `YNAB_BUDGET_ID` — target budget ID.
     - `YNAB_POLL_INTERVAL` — optional, defaults to `1h` (e.g. `30m`).
     - `YNAB_RULES_DIR` — optional, defaults to `rules/`.
     - `YNAB_OBSERVATIONS_PATH` — optional, defaults to `$XDG_CACHE_HOME/ynab-alerts/observations.json`.
     - `YNAB_DEBUG` — optional, set to `true` to emit debug logs (captures, matches).
     - `YNAB_DAY_START`, `YNAB_DAY_END` — optional, HH:MM (24h) window to limit evaluations (e.g., `06:00` / `22:00`).
     - `PUSHOVER_APP_TOKEN`, `PUSHOVER_USER_KEY`, `PUSHOVER_DEVICE` — Pushover credentials (default notifier).
     - Heartbeat (optional; defaults in parentheses): `YNAB_HEARTBEAT_ENABLED` (`false`), `YNAB_HEARTBEAT_NATS_URL` (`nats://localhost:4222`), `YNAB_HEARTBEAT_SUBJECT` (`ynab-alerts`), `YNAB_HEARTBEAT_PREFIX` (`heartbeat`), `YNAB_HEARTBEAT_INTERVAL` (`1m`), `YNAB_HEARTBEAT_GRACE` (`10m`), `YNAB_HEARTBEAT_DESCRIPTION` (`YNAB Alerts`).
2. Inspect data to write rules:
   - List budgets: `go run ./cmd/ynab-alerts list-budgets`
   - List accounts for a budget: `go run ./cmd/ynab-alerts list-accounts --budget <budget-id>`
3. Lint rules: `go run ./cmd/ynab-alerts lint` (shows issues and next evaluation time for each rule).
4. Define rules in YAML (see `rules/sample.yaml`).
5. Run: `go run ./cmd/ynab-alerts run` (add `--notifier=log` to debug without sending).

CLI overrides (persistent flags): `--config`, `--token`, `--budget`, `--base-url`, `--rules`, `--poll`, `--notifier=pushover|log`, `--observe-path`, `--debug`, `--day-start`, `--day-end`, heartbeat-specific flags (`--heartbeat`, `--heartbeat-nats-url`, `--heartbeat-subject`, `--heartbeat-prefix`, `--heartbeat-interval`, `--heartbeat-grace`, `--heartbeat-description`). Subcommands: `run`, `list-budgets`, `list-accounts`, `lint`. Precedence: flags > env vars > config file > defaults. Enable verbose capture/condition logs with `--debug` or `YNAB_DEBUG=true`. Set `--day-start`/`--day-end` (HH:MM) to avoid alerts outside a daily window.

## Rule DSL (brief)
```yaml
- name: checking_vs_cc_due
  when:
    condition: account.balance("Checking") < account.due("CC_Main")
  notify: [pushover]
- name: cc_payment_readiness
  observe:
    - capture_on: "5" # day-of-month to capture CC due
      variable: cc_due_capture
      value: account.due("CC_Main")
  when:
    - day_of_month: [14] # only evaluate on the 14th (weekday agnostic)
      condition: account.balance("Checking") < 0.8 * var.cc_due_capture
  notify: [pushover]
- name: first_monday_buffer
  when:
    nth_weekday: "1 Monday"
    condition: account.balance("Checking") < 100000
  notify: [log]
- name: weekly_friday_sweep
  when:
    days_of_week: ["Fri"]
    condition: account.balance("Savings") > 500
  notify: [log]
- name: cron_based_check
  when:
    schedule: "0 9 14 * *" # 09:00 on the 14th monthly (cron standard)
    condition: account.balance("Checking") < account.due("CC_Main")
  notify: [pushover]
- name: billing_window_readiness
  observe:
    - capture_on: "28"
      variable: bill_due
      value: 500
  when:
    - day_of_month_range: ["27-5"] # spans month boundary
      condition: account.balance("Checking") < (var.bill_due + 200)
  notify: [log]
- name: month_end_check
  when:
    - day_of_month: [-1] # last day of the month
      condition: account.balance("Checking") < 100
  notify: [log]
- name: two_cards_cover
  observe:
    - capture_on: "10"
      variable: card1_due
      value: account.due("Card1")
    - capture_on: "15"
      variable: card2_due
      value: account.due("Card2")
  when:
    - day_of_month: [-1]
      condition: account.balance("Checking") < (var.card1_due + var.card2_due + 100)
  notify: [pushover]
- name: paycheck_buffer
  observe:
    - capture_on: "1"
      variable: rent_due
      value: 2000
  when:
    - days_of_week: ["Fri"]
      condition: account.balance("Checking") < (var.rent_due * 0.5)
    - day_of_month: [25]
      condition: account.balance("Checking") < (var.rent_due + 300)
  notify: [log]
- name: elevated_due_check
  observe:
    - capture_on: "10"
      variable: cc_due_capture
      value: account.due("Primary Card")
  when:
    - day_of_month: [14]
      condition: account.balance("Checking") < var.cc_due_capture
  notify: [pushover]
```

Supported primitives: `account.balance("Name")`, `account.due("Name")` (alias of balance), numeric literals in dollars (e.g., `50` or `50.5`), full arithmetic (`+`, `-`, `*`, `/`, parentheses, unary minus), and `var.<name>` for captured values. You can provide multiple `observe` and `when` entries per rule; schedule gates: `day_of_month` (supports negatives, e.g., `-1` = last day), `day_of_month_range` (e.g., `27-5` to span months), `days_of_week` (Mon-Sun), `nth_weekday` (`1 Monday`, `last Friday`), or `schedule` (cron `min hour dom mon dow`). Observations persist in the cache (`$XDG_CACHE_HOME/ynab-alerts/observations.json` by default, override with `YNAB_OBSERVATIONS_PATH`).
