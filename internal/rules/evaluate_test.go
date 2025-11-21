package rules

import (
	"context"
	"testing"
	"time"
)

func TestEvaluateDayOfMonthGate(t *testing.T) {
	r := Rule{
		Name: "day-gated",
		When: When{DayOfMonth: []int{14}, Condition: `account.balance("Checking") < 100`},
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
		Name: "day-gated",
		When: When{DayOfMonth: []int{14}, Condition: `account.balance("Checking") < 100`},
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
		Name: "dollars-literal",
		When: When{Condition: `account.balance("Checking") < 50.5`},
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
		Name: "cron-sched",
		When: When{
			Schedule:  "0 9 14 * *", // 09:00 on the 14th
			Condition: `account.balance("Checking") < 999999`,
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
		Name: "nth-weekday",
		When: When{
			NthWeekday: "1 Monday",
			Condition:  `account.balance("Checking") < 1000`,
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
		Name: "sched-wins",
		When: When{
			Schedule:   "0 9 14 * *",
			DayOfMonth: []int{1},
			Condition:  `account.balance("Checking") < 1000`,
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
