package rules

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// LintResult captures issues and metadata about a rule.
type LintResult struct {
	Name     string
	Issues   []string
	NextEval time.Time
	HasNext  bool
}

// Lint reads rules from dir and produces lint results.
func Lint(dir string, now time.Time) ([]LintResult, error) {
	return LintWithPoll(dir, now, time.Minute)
}

// LintWithPoll reads rules from dir and produces lint results using pollInterval to approximate next eval times.
func LintWithPoll(dir string, now time.Time, pollInterval time.Duration) ([]LintResult, error) {
	rules, err := LoadDir(dir)
	if err != nil {
		return nil, err
	}
	nameSeen := map[string]struct{}{}
	var results []LintResult
	for _, r := range rules {
		res := LintResult{Name: r.Name}
		if r.Name == "" {
			res.Issues = append(res.Issues, "rule has no name")
		}
		if _, exists := nameSeen[r.Name]; exists && r.Name != "" {
			res.Issues = append(res.Issues, "duplicate rule name")
		}
		nameSeen[r.Name] = struct{}{}

		res.Issues = append(res.Issues, lintWhen(r.When)...)
		res.NextEval, res.HasNext = nextEval(r.When, now, pollInterval)
		results = append(results, res)
	}
	return results, nil
}

func lintWhen(when When) []string {
	var issues []string
	if when.Condition == "" {
		issues = append(issues, "condition is empty; rule will never fire")
	}
	if len(when.DayOfMonth) > 0 {
		for _, d := range when.DayOfMonth {
			if d < 1 || d > 31 {
				issues = append(issues, fmt.Sprintf("day_of_month value %d is out of range 1-31", d))
			}
		}
	}
	for _, d := range when.DaysOfWeek {
		if _, ok := weekdayMap[strings.ToLower(strings.TrimSpace(d))]; !ok {
			issues = append(issues, fmt.Sprintf("days_of_week value %q is invalid", d))
		}
	}
	if when.NthWeekday != "" {
		if _, _, _, ok := parseNthWeekday(when.NthWeekday); !ok {
			issues = append(issues, fmt.Sprintf("nth_weekday value %q is invalid", when.NthWeekday))
		}
	}

	if when.Schedule != "" {
		if _, err := cron.ParseStandard(when.Schedule); err != nil {
			issues = append(issues, fmt.Sprintf("schedule invalid cron: %v", err))
		}
		if len(when.DayOfMonth) > 0 || len(when.DaysOfWeek) > 0 || when.NthWeekday != "" {
			issues = append(issues, "schedule present; day/week gates will be ignored")
		}
	}

	return issues
}

func nextEval(when When, now time.Time, pollInterval time.Duration) (time.Time, bool) {
	if when.Schedule != "" {
		sched, err := cron.ParseStandard(when.Schedule)
		if err != nil {
			return time.Time{}, false
		}
		return sched.Next(now), true
	}
	// no explicit gates: next tick is now + poll interval
	if len(when.DayOfMonth) == 0 && len(when.DaysOfWeek) == 0 && when.NthWeekday == "" {
		return now.Add(pollInterval), true
	}
	// search ahead for the first matching day gate.
	for i := 0; i <= 365; i++ {
		t := now.AddDate(0, 0, i)
		if matchesDayOfMonth(when.DayOfMonth, t.Day()) &&
			matchesDayOfWeek(when.DaysOfWeek, t.Weekday()) &&
			matchNth(when.NthWeekday, t) {
			approx := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()).Add(pollInterval)
			if approx.Before(now) {
				approx = now.Add(pollInterval)
			}
			return approx, true
		}
	}
	return time.Time{}, false
}

func matchNth(expr string, t time.Time) bool {
	if expr == "" {
		return true
	}
	return matchesNthWeekday(expr, t)
}

func nthWeekdayParsable(expr string) bool {
	return matchesNthWeekday(expr, time.Now())
}
