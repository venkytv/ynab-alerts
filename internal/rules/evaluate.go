package rules

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Evaluate applies all rules against the provided data, capturing observations as needed.
func Evaluate(ctx context.Context, rules []Rule, store *Store, data Data) ([]Trigger, error) {
	var triggers []Trigger

	for _, rule := range rules {
		select {
		case <-ctx.Done():
			return triggers, ctx.Err()
		default:
		}

		for _, obs := range rule.Observe {
			if store == nil {
				break
			}
			if err := captureObservation(obs, store, data); err != nil {
				return triggers, fmt.Errorf("capture %s: %w", rule.Name, err)
			}
			// refresh vars after capture
			data.Vars = store.Snapshot()
		}

		if len(rule.When) == 0 {
			continue
		}

		for _, when := range rule.When {
			if when.Condition == "" {
				continue
			}
			if !shouldEvaluate(when, data.Now) {
				continue
			}
			ok, err := evaluateCondition(when.Condition, data)
			if err != nil {
				return triggers, fmt.Errorf("rule %s: %w", rule.Name, err)
			}
			if ok {
				triggers = append(triggers, Trigger{
					Rule:    rule,
					Message: fmt.Sprintf("Rule %s triggered: %s", rule.Name, when.Condition),
				})
			}
		}
	}
	return triggers, nil
}

func captureObservation(obs Observe, store *Store, data Data) error {
	if obs.Variable == "" || obs.Value == "" {
		return errors.New("observation missing variable or value")
	}

	shouldCapture := false
	now := data.Now
	if obs.CaptureOn == "" || strings.EqualFold(obs.CaptureOn, "always") {
		shouldCapture = true
	} else if day, err := strconv.Atoi(obs.CaptureOn); err == nil {
		if now.Day() == day {
			if existing, ok := store.Get(obs.Variable); !ok || !sameCalendarDay(existing.RecordedAt, now) {
				shouldCapture = true
			}
		}
	}

	if !shouldCapture {
		return nil
	}

	val, err := resolveValue(obs.Value, data)
	if err != nil {
		return err
	}
	return store.Set(obs.Variable, ObservedValue{
		Value:      val,
		RecordedAt: now,
	})
}

var condPattern = regexp.MustCompile(`^\s*(.+?)\s*(<=|>=|==|!=|<|>)\s*(.+?)\s*$`)

func evaluateCondition(cond string, data Data) (bool, error) {
	m := condPattern.FindStringSubmatch(cond)
	if len(m) != 4 {
		return false, fmt.Errorf("unable to parse condition %q", cond)
	}
	left, op, right := strings.TrimSpace(m[1]), m[2], strings.TrimSpace(m[3])

	lv, err := resolveValue(left, data)
	if err != nil {
		return false, fmt.Errorf("left side: %w", err)
	}
	rv, err := resolveValue(right, data)
	if err != nil {
		return false, fmt.Errorf("right side: %w", err)
	}

	switch op {
	case "<":
		return lv < rv, nil
	case "<=":
		return lv <= rv, nil
	case ">":
		return lv > rv, nil
	case ">=":
		return lv >= rv, nil
	case "==":
		return lv == rv, nil
	case "!=":
		return lv != rv, nil
	default:
		return false, fmt.Errorf("unknown operator %q", op)
	}
}

func resolveValue(expr string, data Data) (int64, error) {
	expr = strings.TrimSpace(expr)

	// simple multiplier pattern: a * b
	if parts := strings.Split(expr, "*"); len(parts) == 2 {
		factorStr := strings.TrimSpace(parts[0])
		rest := strings.TrimSpace(parts[1])

		factor, err := strconv.ParseFloat(factorStr, 64)
		if err == nil {
			val, err := resolveValue(rest, data)
			if err != nil {
				return 0, err
			}
			return int64(math.Round(float64(val) * factor)), nil
		}
	}

	// account.balance("Name")
	if strings.HasPrefix(expr, "account.balance(") {
		name := extractArg(expr, "account.balance")
		if name == "" {
			return 0, fmt.Errorf("account balance missing name")
		}
		val, ok := data.Accounts[name]
		if !ok {
			return 0, fmt.Errorf("account %q not found", name)
		}
		return val, nil
	}

	// account.due("Name") currently treated as balance
	if strings.HasPrefix(expr, "account.due(") {
		name := extractArg(expr, "account.due")
		if name == "" {
			return 0, fmt.Errorf("account due missing name")
		}
		val, ok := data.Accounts[name]
		if !ok {
			return 0, fmt.Errorf("account %q not found", name)
		}
		return val, nil
	}

	// variable reference var.foo
	if strings.HasPrefix(expr, "var.") {
		key := strings.TrimPrefix(expr, "var.")
		val, ok := data.Vars[key]
		if !ok {
			return 0, fmt.Errorf("variable %q not found", key)
		}
		return val, nil
	}

	// numeric literal (dollars) -> milliunits
	if num, err := strconv.ParseFloat(expr, 64); err == nil {
		return int64(math.Round(num * 1000)), nil
	}

	return 0, fmt.Errorf("unsupported expression %q", expr)
}

func extractArg(expr, prefix string) string {
	start := strings.Index(expr, "(")
	end := strings.LastIndex(expr, ")")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	arg := strings.TrimSpace(expr[start+1 : end])
	arg = strings.Trim(arg, `"`)
	arg = strings.Trim(arg, `'`)
	if strings.HasPrefix(expr, prefix) {
		return arg
	}
	return ""
}

func sameCalendarDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func shouldEvaluate(when When, now time.Time) bool {
	// schedule (cron) wins if set
	if when.Schedule != "" {
		sched, err := cron.ParseStandard(when.Schedule)
		if err != nil {
			return false
		}
		// check if now matches the schedule tick
		prev := sched.Next(now.Add(-time.Minute * 2))
		return sameMinute(prev, now)
	}

	if len(when.DayOfMonth) > 0 && !matchesDayOfMonth(when.DayOfMonth, now.Day()) {
		return false
	}
	if len(when.DaysOfWeek) > 0 && !matchesDayOfWeek(when.DaysOfWeek, now.Weekday()) {
		return false
	}
	if when.NthWeekday != "" && !matchesNthWeekday(when.NthWeekday, now) {
		return false
	}
	return true
}

func sameMinute(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day() && a.Hour() == b.Hour() && a.Minute() == b.Minute()
}

func matchesDayOfMonth(days []int, today int) bool {
	if len(days) == 0 {
		return true
	}
	for _, d := range days {
		if d == today {
			return true
		}
	}
	return false
}

var weekdayMap = map[string]time.Weekday{
	"sun":       time.Sunday,
	"sunday":    time.Sunday,
	"mon":       time.Monday,
	"monday":    time.Monday,
	"tue":       time.Tuesday,
	"tues":      time.Tuesday,
	"tuesday":   time.Tuesday,
	"wed":       time.Wednesday,
	"wednesday": time.Wednesday,
	"thu":       time.Thursday,
	"thur":      time.Thursday,
	"thurs":     time.Thursday,
	"thursday":  time.Thursday,
	"fri":       time.Friday,
	"friday":    time.Friday,
	"sat":       time.Saturday,
	"saturday":  time.Saturday,
}

func matchesDayOfWeek(days []string, today time.Weekday) bool {
	if len(days) == 0 {
		return true
	}
	for _, d := range days {
		if wd, ok := weekdayMap[strings.ToLower(strings.TrimSpace(d))]; ok && wd == today {
			return true
		}
	}
	return false
}

func matchesNthWeekday(expr string, now time.Time) bool {
	n, wd, last, ok := parseNthWeekday(expr)
	if !ok {
		return false
	}
	matchDates := []time.Time{}
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	for t := firstOfMonth; t.Month() == now.Month(); t = t.AddDate(0, 0, 1) {
		if t.Weekday() == wd {
			matchDates = append(matchDates, t)
		}
	}

	if last && len(matchDates) > 0 {
		return sameCalendarDay(now, matchDates[len(matchDates)-1])
	}
	if n > 0 && len(matchDates) >= n {
		return sameCalendarDay(now, matchDates[n-1])
	}
	return false
}

func parseNthWeekday(expr string) (n int, wd time.Weekday, last bool, ok bool) {
	parts := strings.Fields(strings.ToLower(expr))
	if len(parts) != 2 {
		return 0, 0, false, false
	}
	nthStr, dayStr := parts[0], parts[1]

	if nthStr == "last" {
		last = true
	} else {
		val, err := strconv.Atoi(nthStr)
		if err != nil || val < 1 {
			return 0, 0, false, false
		}
		n = val
	}

	var okDay bool
	wd, okDay = weekdayMap[dayStr]
	if !okDay {
		return 0, 0, false, false
	}
	return n, wd, last, true
}
