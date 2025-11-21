package rules

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLintReportsNextEvalAndConflicts(t *testing.T) {
	dir := t.TempDir()
	ruleFile := `
- name: cron_and_day
  when:
    schedule: "0 9 14 * *"
    day_of_month: [1]
    condition: account.balance("Checking") < 10
- name: empty_condition
  when: {}
`
	if err := os.WriteFile(filepath.Join(dir, "rules.yaml"), []byte(ruleFile), 0o644); err != nil {
		t.Fatalf("write tmp rule: %v", err)
	}

	now := time.Date(2024, time.March, 10, 9, 0, 0, 0, time.UTC)
	results, err := LintWithPoll(dir, now, time.Minute)
	if err != nil {
		t.Fatalf("lint error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	var cronRes, emptyRes LintResult
	for _, r := range results {
		if r.Name == "cron_and_day" {
			cronRes = r
		}
		if r.Name == "empty_condition" {
			emptyRes = r
		}
	}

	if !cronRes.HasNext || cronRes.NextEval.IsZero() {
		t.Fatalf("expected next eval for cron rule")
	}
	foundConflict := false
	for _, issue := range cronRes.Issues {
		if issue == "schedule present; day/week gates will be ignored" {
			foundConflict = true
		}
	}
	if !foundConflict {
		t.Fatalf("expected conflict warning for schedule vs day_of_month")
	}

	hasEmpty := false
	for _, issue := range emptyRes.Issues {
		if issue == "condition is empty; rule will never fire" {
			hasEmpty = true
		}
	}
	if !hasEmpty {
		t.Fatalf("expected empty condition warning")
	}
}
