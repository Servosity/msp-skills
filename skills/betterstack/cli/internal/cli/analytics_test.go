// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Tests for the hand-built transcendence analytics helpers.

package cli

import (
	"math"
	"testing"
	"time"
)

func TestTruthy(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"1", true}, {"true", true}, {"TRUE", true},
		{"0", false}, {"false", false}, {"", false}, {"2", false},
	}
	for _, c := range cases {
		if got := truthy(c.in); got != c.want {
			t.Errorf("truthy(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestAtoiSafe(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0}, {"30", 30}, {"30.0", 30}, {"30.9", 30}, {"abc", 0}, {"-5", -5},
	}
	for _, c := range cases {
		if got := atoiSafe(c.in); got != c.want {
			t.Errorf("atoiSafe(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseTime(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2026-05-30T12:00:00Z", true},
		{"2026-05-30T12:00:00.123Z", true},
		{"2026-05-30T12:00:00+02:00", true},
		{"2026-05-30", true},
		{"", false},
		{"not-a-time", false},
	}
	for _, c := range cases {
		_, ok := parseTime(c.in)
		if ok != c.ok {
			t.Errorf("parseTime(%q) ok = %v, want %v", c.in, ok, c.ok)
		}
	}
}

func TestMeanMedian(t *testing.T) {
	if got := mean(nil); got != 0 {
		t.Errorf("mean(nil) = %v, want 0", got)
	}
	if got := mean([]float64{1, 2, 3, 4}); got != 2.5 {
		t.Errorf("mean = %v, want 2.5", got)
	}
	if got := median([]float64{3, 1, 2}); got != 2 {
		t.Errorf("median(odd) = %v, want 2", got)
	}
	if got := median([]float64{1, 2, 3, 4}); got != 2.5 {
		t.Errorf("median(even) = %v, want 2.5", got)
	}
}

func TestHumanizeSeconds(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "—"},
		{45, "45s"},
		{90, "1m30s"},
		{3600, "1h0m"},
		{90000, "1d1h"},
	}
	for _, c := range cases {
		if got := humanizeSeconds(c.in); got != c.want {
			t.Errorf("humanizeSeconds(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMonitorDown(t *testing.T) {
	for _, s := range []string{"down", "validating", "pending", "DOWN"} {
		if !monitorDown(s) {
			t.Errorf("monitorDown(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"up", "paused", "maintenance", ""} {
		if monitorDown(s) {
			t.Errorf("monitorDown(%q) = true, want false", s)
		}
	}
}

func TestHasAlertChannel(t *testing.T) {
	if (monitorRow{}).hasAlertChannel() {
		t.Error("empty monitor should have no alert channel")
	}
	if !(monitorRow{Email: true}).hasAlertChannel() {
		t.Error("monitor with email should have an alert channel")
	}
	if !(monitorRow{Push: true}).hasAlertChannel() {
		t.Error("monitor with push should have an alert channel")
	}
}

func TestMeanMTTRMathStable(t *testing.T) {
	// Guards the MTTR arithmetic used by the mttr command.
	vals := []float64{1800, 3600}
	if m := mean(vals); math.Abs(m-2700) > 0.001 {
		t.Errorf("mean MTTR = %v, want 2700", m)
	}
	if d := time.Duration(2700) * time.Second; d != 45*time.Minute {
		t.Errorf("2700s != 45m")
	}
}
