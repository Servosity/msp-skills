// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestHumanDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                              "<1m",
		30 * time.Second:               "<1m",
		45 * time.Minute:               "45m",
		8 * time.Hour:                  "8h",
		3*time.Hour + 20*time.Minute:   "3h20m",
		40 * time.Hour:                 "1d16h",
		100 * time.Hour:                "4d4h",
		216 * time.Hour:                "9d",
		-(2*time.Hour + 5*time.Minute): "-2h5m",
	}
	for in, want := range cases {
		if got := humanDuration(in); got != want {
			t.Errorf("humanDuration(%v) = %q, want %q", in, got, want)
		}
	}
	// Boundaries: no overflow recursion, and both ends render in days.
	if got := humanDuration(math.MinInt64); got != "-106751d23h" {
		t.Errorf("humanDuration(MinInt64) = %q", got)
	}
	if got := humanDuration(math.MaxInt64); got != "106751d23h" {
		t.Errorf("humanDuration(MaxInt64) = %q", got)
	}
}

func TestPlural(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{0, "0 members"}, {1, "1 member"}, {2, "2 members"}} {
		if got := plural(c.n, "member"); got != c.want {
			t.Errorf("plural(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// Equal breach rates rank by breached count, then cycle count, then group id.
func TestSLARollupTieBreak(t *testing.T) {
	cycles := func(breached ...bool) jsmSLA {
		s := jsmSLA{Name: "TTR"}
		for _, b := range breached {
			s.CompletedCycles = append(s.CompletedCycles, jsmSLACycle{Breached: b, StopTime: &jsmDate{EpochMillis: 1000}})
		}
		return s
	}
	reqs := []localRequest{
		req("A-1", "1", "10", "u1", "DONE", cycles(true)),             // 1/1
		req("A-2", "2", "10", "u1", "DONE", cycles(true, true, true)), // 3/3
		req("A-3", "3", "10", "u1", "DONE", cycles(true, false)),      // 1/2
		req("A-4", "4", "10", "u1", "DONE", cycles(true, true)),       // 2/2
	}
	var order []string
	for _, g := range slaRollup(reqs, 0, "desk", nil) {
		order = append(order, g.Group)
	}
	want := []string{"2", "4", "1", "3"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("groups = %v, want %v", order, want)
	}
}
