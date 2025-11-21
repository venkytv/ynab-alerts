# ynab-alerts

Daemon that polls YNAB and emits alerts when rule conditions are met.

## Quick start
1. Set environment:
   - `YNAB_TOKEN` — personal access token.
   - `YNAB_BUDGET_ID` — target budget ID.
   - `YNAB_POLL_INTERVAL` — optional, defaults to `1h` (e.g. `30m`).
   - `YNAB_RULES_DIR` — optional, defaults to `rules/`.
   - `YNAB_OBSERVATIONS_PATH` — optional, defaults to `$XDG_CACHE_HOME/ynab-alerts/observations.json`.
   - `PUSHOVER_APP_TOKEN`, `PUSHOVER_USER_KEY`, `PUSHOVER_DEVICE` — Pushover credentials (default notifier).
2. Inspect data to write rules:
   - List budgets: `go run ./cmd/ynab-alerts list-budgets`
   - List accounts for a budget: `go run ./cmd/ynab-alerts list-accounts --budget <budget-id>`
3. Lint rules: `go run ./cmd/ynab-alerts lint` (shows issues and next evaluation time for each rule).
4. Define rules in YAML (see `rules/sample.yaml`).
5. Run: `go run ./cmd/ynab-alerts run` (add `--notifier=log` to debug without sending).

CLI overrides (persistent flags): `--token`, `--budget`, `--base-url`, `--rules`, `--poll`, `--notifier=pushover|log`, `--observe-path`. Subcommands: `run`, `list-budgets`, `list-accounts`, `lint`.

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

Supported primitives: `account.balance("Name")`, `account.due("Name")` (alias of balance), numeric literals in dollars (e.g., `50` or `50.5`), simple math with `*` and `+`, and `var.<name>` for captured values. You can provide multiple `observe` and `when` entries per rule; schedule gates: `day_of_month` (supports negatives, e.g., `-1` = last day), `days_of_week` (Mon-Sun), `nth_weekday` (`1 Monday`, `last Friday`), or `schedule` (cron `min hour dom mon dow`). Observations persist in the cache (`$XDG_CACHE_HOME/ynab-alerts/observations.json` by default, override with `YNAB_OBSERVATIONS_PATH`).
