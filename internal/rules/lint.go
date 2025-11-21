package rules

import (
	"fmt"
	"regexp"
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
		variables := map[string]struct{}{}
		res := LintResult{Name: r.Name}
		if r.Name == "" {
			res.Issues = append(res.Issues, "rule has no name")
		}
		if _, exists := nameSeen[r.Name]; exists && r.Name != "" {
			res.Issues = append(res.Issues, "duplicate rule name")
		}
		nameSeen[r.Name] = struct{}{}

		for _, obs := range r.Observe {
			if obs.Variable == "" {
				res.Issues = append(res.Issues, "observe variable is empty")
			} else {
				variables[obs.Variable] = struct{}{}
			}
		}

		res.Issues = append(res.Issues, lintWhen(r.When, variables)...)
		res.NextEval, res.HasNext = nextEval(r.When, now, pollInterval)
		results = append(results, res)
	}
	return results, nil
}

func lintWhen(whens WhenList, vars map[string]struct{}) []string {
	var issues []string

	if len(whens) == 0 {
		issues = append(issues, "no when clause defined; rule will never run")
	}

	for _, when := range whens {
		if when.Condition == "" {
			issues = append(issues, "condition is empty; rule will never fire")
		}
		if len(when.DayOfMonth) > 0 {
			for _, d := range when.DayOfMonth {
				if d == 0 || d < -31 || d > 31 {
					issues = append(issues, fmt.Sprintf("day_of_month value %d is out of range -31..-1 or 1..31", d))
				}
			}
		}
		for _, r := range when.DayOfMonthRanges {
			s, e, ok := parseRange(r)
			if !ok {
				issues = append(issues, fmt.Sprintf("day_of_month_range value %q is invalid", r))
				continue
			}
			if s < 1 || s > 31 || e < 1 || e > 31 {
				issues = append(issues, fmt.Sprintf("day_of_month_range %q values must be within 1..31", r))
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
			if len(when.DayOfMonth) > 0 || len(when.DaysOfWeek) > 0 || when.NthWeekday != "" || len(when.DayOfMonthRanges) > 0 {
				issues = append(issues, "schedule present; day/week gates will be ignored")
			}
		}

		for _, ref := range varRefs(when.Condition) {
			if _, ok := vars[ref]; !ok {
				issues = append(issues, fmt.Sprintf("condition references unknown variable %q", ref))
			}
		}
	}

	return issues
}

var varRefPattern = regexp.MustCompile(`var\.([A-Za-z0-9_]+)`)

func varRefs(cond string) []string {
	matches := varRefPattern.FindAllStringSubmatch(cond, -1)
	var out []string
	for _, m := range matches {
		if len(m) == 2 {
			out = append(out, m[1])
		}
	}
	return out
}

func nextEval(whens WhenList, now time.Time, pollInterval time.Duration) (time.Time, bool) {
	if len(whens) == 0 {
		return time.Time{}, false
	}
	// schedule wins if present on any when; pick the soonest
	var best time.Time
	for _, when := range whens {
		if when.Schedule != "" {
			sched, err := cron.ParseStandard(when.Schedule)
			if err != nil {
				continue
			}
			next := sched.Next(now)
			if best.IsZero() || next.Before(best) {
				best = next
			}
		}
	}
	if !best.IsZero() {
		return best, true
	}

	// if no explicit gates anywhere: now + poll
	allUngated := true
	for _, when := range whens {
		if len(when.DayOfMonth) > 0 || len(when.DaysOfWeek) > 0 || when.NthWeekday != "" || len(when.DayOfMonthRanges) > 0 {
			allUngated = false
			break
		}
	}
	if allUngated {
		return now.Add(pollInterval), true
	}

	for i := 0; i <= 365; i++ {
		t := now.AddDate(0, 0, i)
		for _, when := range whens {
			if matchesDayOfMonth(when.DayOfMonth, t.Day(), daysInMonth(t)) &&
				matchesDayOfMonthRange(when.DayOfMonthRanges, t.Day(), daysInMonth(t)) &&
				matchesDayOfWeek(when.DaysOfWeek, t.Weekday()) &&
				matchNth(when.NthWeekday, t) {
				approx := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()).Add(pollInterval)
				if approx.Before(now) {
					approx = now.Add(pollInterval)
				}
				return approx, true
			}
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
