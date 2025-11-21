package service

import (
	"testing"
	"time"

	"ynab-alerts/internal/config"
)

func TestWithinEvalWindow(t *testing.T) {
	cfg := config.Config{
		DayStart: 6 * time.Hour,
		DayEnd:   22 * time.Hour,
	}
	svc := &Service{cfg: cfg}

	tc := []struct {
		hour   int
		min    int
		expect bool
	}{
		{5, 59, false},
		{6, 0, true},
		{21, 59, true},
		{22, 0, false},
	}

	for _, tt := range tc {
		now := time.Date(2024, time.January, 1, tt.hour, tt.min, 0, 0, time.UTC)
		if got := svc.withinEvalWindow(now); got != tt.expect {
			t.Fatalf("hour %02d:%02d expected %v got %v", tt.hour, tt.min, tt.expect, got)
		}
	}
}
