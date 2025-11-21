# Repository Guidelines

## Project Structure & Module Organization
- Go-first layout; initialize `go.mod` when code lands.
- Executable entrypoint in `cmd/ynab-alerts/`; shared logic in `internal/`.
- Fixtures in `testdata/`; configs in `.env` with `.env.example` for safe defaults.

## Build, Test, and Development Commands
- `go fmt ./...` — format all Go files; run before committing.
- `go vet ./...` — static checks for common mistakes.
- `go test ./...` — run the full test suite; add `-cover` for coverage.
- `go run ./cmd/ynab-alerts run` — execute the daemon; add `--notifier=log` to dry-run alerts.
- `go run ./cmd/ynab-alerts list-budgets` / `list-accounts --budget <id>` — discovery helpers.
- `go run ./cmd/ynab-alerts lint` — sanity-check rules; shows issues and next evaluation time.
- Configuration sources: `--config`/`YNAB_CONFIG` (YAML/JSON file) < env vars < CLI flags. Common flags: `--config`, `--token`, `--budget`, `--base-url`, `--rules`, `--poll`, `--notifier=pushover|log`, `--observe-path`, `--debug` (verbose capture/condition logs).

## Coding Style & Naming Conventions
- Follow Go idioms: tabs, `gofmt`, focused files.
- Package names are short and lowercase; exported identifiers use PascalCase with doc comments.

## Testing Guidelines
- Use `testing` with table-driven cases; name tests `TestFeatureBehavior`.
- Place mocks/fixtures under `testdata/`; keep golden files small.
- Add integration tests around YNAB API boundaries; gate network calls behind interfaces.
- Run `go test ./...` before pushing; cover core alert rules.

## Commit & Pull Request Guidelines
- Write imperative, scoped commit messages (`Add alert rule parser`, `Fix budget client retries`) and keep commits cohesive.
- Pull requests: problem/solution summary, related issues, manual-test steps or sample output, config changes.
- Before opening a PR, ensure formatting, vetting, and tests pass; include logs or screenshots for user-facing or alert output changes.

## Architecture Overview
- Daemon-style Go service that polls YNAB on a schedule, normalizes responses, and runs rules to emit alerts.
- API access stays behind interfaces; scheduling and dispatch are centralized so alert logic stays pure; log with rule IDs and guard double-processing with windows or cursors.
- Notifications default to Pushover; keep notifier drivers pluggable/configurable (e.g., Pushover, Slack, email).
- Observations persist to XDG cache (`XDG_CACHE_HOME/ynab-alerts/observations.json`); override with `YNAB_OBSERVATIONS_PATH`.
- Enable debug traces for captures/condition matches with `YNAB_DEBUG=true` or `--debug`.
- Config files are supported via `--config` or `YNAB_CONFIG` (YAML/JSON); flags > env > file > defaults.

## Rule Authoring (DSL)
- Rules must be human-readable; use YAML (example):
  ```yaml
  - name: checking_vs_cc_due
    when: account.balance("Checking") < account.due("CC_Main")
    notify: slack
  - name: cc_payment_readiness
    observe:
      - capture_on: "5" # day-of-month to record the due amount
        variable: cc_due_capture
        value: account.due("CC_Main")
    when:
      - day_of_month: [14] # weekday agnostic
        window: payment_day -3d..0d
        condition: account.balance("Checking") < 0.8 * var.cc_due_capture
    notify: email
  - name: first_monday_buffer
    when:
      nth_weekday: "1 Monday"
      condition: account.balance("Checking") < 100
    notify: log
  - name: cron_based_check
    when:
      schedule: "0 9 14 * *"
      condition: account.balance("Checking") < account.due("CC_Main")
    notify: pushover
  - name: billing_window_readiness
    observe:
      - capture_on: "28"
        variable: bill_due
        value: 500
    when:
      - day_of_month_range: ["27-5"] # spans month boundary
        condition: account.balance("Checking") < (var.bill_due + 200)
    notify: log
  - name: month_end_check
    when:
      - day_of_month: [-1]
        condition: account.balance("Checking") < 100
    notify: log
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
    notify: pushover
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
    notify: log
  ```
- Primitives: `account.balance`, `account.due`, simple math, named variables per day; numeric literals are dollars (e.g., `50` or `50.5`) and converted to milliunits. Multiple `observe` and `when` blocks are supported. Schedule via `day_of_month` (supports negatives, e.g., `-1` for last day), `day_of_month_range` (e.g., `27-5` to span months), `days_of_week`, `nth_weekday` (e.g., `1 Monday`, `last Friday`), or cron-style `schedule`. Store rules in `rules/`; validate on startup and lint unknown accounts/vars.

## Security & Configuration Tips
- Never commit real YNAB API tokens; load them from `.env` or your shell environment.
- If adding new secrets, document the variable names and expected format in `.env.example`.
- Avoid writing sensitive values to logs; mask token-like strings and redact budget identifiers when sharing diagnostics.
- Observation storage defaults to XDG cache; set `YNAB_OBSERVATIONS_PATH` if you need a specific location (avoid shared or world-readable paths).
