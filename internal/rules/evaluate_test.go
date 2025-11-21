package rules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEvaluateDayOfMonthGate(t *testing.T) {
	r := Rule{
		Name:    "day-gated",
		When:    WhenList{{DayOfMonth: []int{14}, Condition: `account.balance("Checking") < 100`}},
		Observe: ObserveList{},
	}
	data := Data{
		Accounts: map[string]int64{"Checking": 50_000},
		Vars:     map[string]int64{},
		Now:      time.Date(2024, time.January, 14, 12, 0, 0, 0, time.UTC),
	}
	trigs, err := Evaluate(context.Background(), []Rule{r}, nil, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(trigs))
	}
}

func TestEvaluateDoesNotTriggerOnOtherDay(t *testing.T) {
	r := Rule{
		Name:    "day-gated",
		When:    WhenList{{DayOfMonth: []int{14}, Condition: `account.balance("Checking") < 100`}},
		Observe: ObserveList{},
	}
	data := Data{
		Accounts: map[string]int64{"Checking": 50_000},
		Vars:     map[string]int64{},
		Now:      time.Date(2024, time.January, 15, 12, 0, 0, 0, time.UTC),
	}
	trigs, err := Evaluate(context.Background(), []Rule{r}, nil, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 0 {
		t.Fatalf("expected 0 triggers, got %d", len(trigs))
	}
}

func TestEvaluateDollarsLiteral(t *testing.T) {
	r := Rule{
		Name:    "dollars-literal",
		When:    WhenList{{Condition: `account.balance("Checking") < 50.5`}},
		Observe: ObserveList{},
	}
	data := Data{
		Accounts: map[string]int64{"Checking": 50_000}, // $50.00
		Vars:     map[string]int64{},
		Now:      time.Now(),
	}
	trigs, err := Evaluate(context.Background(), []Rule{r}, nil, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 1 {
		t.Fatalf("expected trigger because 50.00 < 50.50, got %d", len(trigs))
	}
}

func TestEvaluateCronSchedule(t *testing.T) {
	r := Rule{
		Name:    "cron-sched",
		Observe: ObserveList{},
		When: WhenList{
			{
				Schedule:  "0 9 14 * *", // 09:00 on the 14th
				Condition: `account.balance("Checking") < 999999`,
			},
		},
	}
	now := time.Date(2024, time.March, 14, 9, 0, 0, 0, time.UTC)
	data := Data{
		Accounts: map[string]int64{"Checking": 10},
		Vars:     map[string]int64{},
		Now:      now,
	}
	trigs, err := Evaluate(context.Background(), []Rule{r}, nil, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 1 {
		t.Fatalf("expected cron match trigger, got %d", len(trigs))
	}
}

func TestEvaluateNthWeekday(t *testing.T) {
	r := Rule{
		Name:    "nth-weekday",
		Observe: ObserveList{},
		When: WhenList{
			{
				NthWeekday: "1 Monday",
				Condition:  `account.balance("Checking") < 1000`,
			},
		},
	}
	now := time.Date(2024, time.July, 1, 10, 0, 0, 0, time.UTC) // Monday and first of month
	data := Data{
		Accounts: map[string]int64{"Checking": 10},
		Vars:     map[string]int64{},
		Now:      now,
	}
	trigs, err := Evaluate(context.Background(), []Rule{r}, nil, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 1 {
		t.Fatalf("expected nth weekday trigger, got %d", len(trigs))
	}
}

func TestEvaluateScheduleOverridesOtherGates(t *testing.T) {
	r := Rule{
		Name:    "sched-wins",
		Observe: ObserveList{},
		When: WhenList{
			{
				Schedule:   "0 9 14 * *",
				DayOfMonth: []int{1},
				Condition:  `account.balance("Checking") < 1000`,
			},
		},
	}
	now := time.Date(2024, time.March, 14, 9, 0, 0, 0, time.UTC)
	data := Data{
		Accounts: map[string]int64{"Checking": 10},
		Vars:     map[string]int64{},
		Now:      now,
	}
	trigs, err := Evaluate(context.Background(), []Rule{r}, nil, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 1 {
		t.Fatalf("expected schedule to match even with conflicting day_of_month, got %d", len(trigs))
	}
}

func TestEvaluateNegativeDayOfMonth(t *testing.T) {
	r := Rule{
		Name:    "end-of-month",
		Observe: ObserveList{},
		When: WhenList{
			{
				DayOfMonth: []int{-1},
				Condition:  `account.balance("Checking") < 200`,
			},
		},
	}
	now := time.Date(2024, time.January, 31, 12, 0, 0, 0, time.UTC) // last day
	data := Data{
		Accounts: map[string]int64{"Checking": 150_000},
		Vars:     map[string]int64{},
		Now:      now,
	}
	trigs, err := Evaluate(context.Background(), []Rule{r}, nil, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 1 {
		t.Fatalf("expected trigger on last day, got %d", len(trigs))
	}
}

func TestEvaluateDayOfMonthRangeWrap(t *testing.T) {
	r := Rule{
		Name:    "billing-window",
		Observe: ObserveList{},
		When: WhenList{
			{
				DayOfMonthRanges: []string{"27-5"},
				Condition:        `account.balance("Checking") < 999999`,
			},
		},
	}
	// March 2 should match (wrap range 27..end + 1..5)
	now := time.Date(2024, time.March, 2, 10, 0, 0, 0, time.UTC)
	data := Data{
		Accounts: map[string]int64{"Checking": 1},
		Vars:     map[string]int64{},
		Now:      now,
	}
	trigs, err := Evaluate(context.Background(), []Rule{r}, nil, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 1 {
		t.Fatalf("expected trigger inside range, got %d", len(trigs))
	}

	// March 10 should not match
	data.Now = time.Date(2024, time.March, 10, 10, 0, 0, 0, time.UTC)
	trigs, err = Evaluate(context.Background(), []Rule{r}, nil, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 0 {
		t.Fatalf("expected no trigger outside range, got %d", len(trigs))
	}
}

func TestEvaluateCapturesMultipleObservations(t *testing.T) {
	storePath := t.TempDir() + "/obs.json"
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("store error: %v", err)
	}
	r := Rule{
		Name: "multi-observe",
		Observe: ObserveList{
			{CaptureOn: "", Variable: "a", Value: "10"},
			{CaptureOn: "", Variable: "b", Value: "20"},
		},
		When: WhenList{
			{Condition: "var.a < var.b"},
		},
	}
	data := Data{
		Accounts: map[string]int64{},
		Vars:     map[string]int64{},
		Now:      time.Now(),
	}
	trigs, err := Evaluate(context.Background(), []Rule{r}, store, data)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(trigs) != 1 {
		t.Fatalf("expected trigger with captured vars, got %d", len(trigs))
	}
	snap := store.Snapshot()
	if snap["a"] != 10_000 || snap["b"] != 20_000 {
		t.Fatalf("unexpected captured values (milliunits): %+v", snap)
	}
}

func TestEvaluateEmitsDebugLogs(t *testing.T) {
	storePath := t.TempDir() + "/obs.json"
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("store error: %v", err)
	}

	logger := &capturingDebug{}
	defer SetDebugLogger(nil)
	SetDebugLogger(logger)

	r := Rule{
		Name: "debug-capture",
		Observe: ObserveList{
			{Variable: "due_cc", Value: "account.balance(\"Card\")"},
		},
		When: WhenList{
			{Condition: "var.due_cc > 0"},
		},
	}
	data := Data{
		Accounts: map[string]int64{"Card": 123_000},
		Vars:     map[string]int64{},
		Now:      time.Now(),
	}
	if _, err := Evaluate(context.Background(), []Rule{r}, store, data); err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if len(logger.msgs) == 0 {
		t.Fatalf("expected debug logs to be recorded")
	}
	foundCapture := false
	foundMatch := false
	for _, m := range logger.msgs {
		if containsAll(m, []string{"captured", "due_cc"}) {
			foundCapture = true
		}
		if containsAll(m, []string{"condition matched", "debug-capture"}) {
			foundMatch = true
		}
	}
	if !foundCapture || !foundMatch {
		t.Fatalf("expected capture and match debug logs, got: %v", logger.msgs)
	}
}

type capturingDebug struct {
	msgs []string
}

func (c *capturingDebug) Debugf(format string, args ...interface{}) {
	c.msgs = append(c.msgs, fmt.Sprintf(format, args...))
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
