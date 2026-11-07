// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

// Codex r1 class wrong-requested-time-window: a day-count preset asks Cost
// Explorer for exactly that many whole days ending before today (End is
// exclusive: the AWS SDK GetCostAndUsageInput TimePeriod), never a day more.
func TestR1DayPresetsSpanExactlyTheirDays(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	for preset, days := range map[string]int{"7d": 7, "7days": 7, "week": 7, "30d": 30, "30days": 30, "yesterday": 1} {
		pr, err := resolvePeriod(preset)
		if err != nil {
			t.Fatalf("%s: %v", preset, err)
		}
		s, _ := time.Parse("2006-01-02", pr.Start)
		e, _ := time.Parse("2006-01-02", pr.End)
		if got := int(e.Sub(s).Hours() / 24); got != days {
			t.Errorf("--period %s asks for %d days [%s, %s), want %d", preset, got, pr.Start, pr.End, days)
		}
		if pr.End != today {
			t.Errorf("--period %s ends %s (exclusive), want today %s: today's partial day is not one of the last %d days", preset, pr.End, today, days)
		}
	}
}
