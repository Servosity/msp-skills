// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestParseTLTime(t *testing.T) {
	cases := []struct {
		in     string
		wantOK bool
	}{
		{"", false},
		{"   ", false},
		{"not-a-date", false},
		{"2026-05-28T21:21:16Z", true},
		{"2026-05-28T21:21:16", true},
		{"2026-05-28 21:21:16", true},
		{"2026-05-28", true},
		{"05/28/2026", true},
	}
	for _, c := range cases {
		_, ok := parseTLTime(c.in)
		if ok != c.wantOK {
			t.Errorf("parseTLTime(%q) ok = %v, want %v", c.in, ok, c.wantOK)
		}
	}
}

func TestAgeBucketOf(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{-1 * time.Hour, "unknown"},
		{1 * time.Hour, "<24h"},
		{25 * time.Hour, ">24h"},
		{50 * time.Hour, ">48h"},
		{8 * 24 * time.Hour, ">7d"},
	}
	for _, c := range cases {
		if got := ageBucketOf(c.d); got != c.want {
			t.Errorf("ageBucketOf(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestResolveSince(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	if cutoff, ok := resolveSince("", now); !ok || !cutoff.IsZero() {
		t.Errorf("resolveSince(empty) = (%v,%v), want (zero,true)", cutoff, ok)
	}
	if cutoff, ok := resolveSince("7d", now); !ok || !cutoff.Equal(now.Add(-7*24*time.Hour)) {
		t.Errorf("resolveSince(7d) = (%v,%v), want 7 days before now", cutoff, ok)
	}
	if cutoff, ok := resolveSince("12h", now); !ok || !cutoff.Equal(now.Add(-12*time.Hour)) {
		t.Errorf("resolveSince(12h) = (%v,%v), want 12h before now", cutoff, ok)
	}
	if cutoff, ok := resolveSince("2026-05-01", now); !ok || cutoff.IsZero() {
		t.Errorf("resolveSince(date) = (%v,%v), want parsed date", cutoff, ok)
	}
	if _, ok := resolveSince("garbage", now); ok {
		t.Errorf("resolveSince(garbage) ok = true, want false")
	}
}

func TestTLString(t *testing.T) {
	if tlString(nil) != "" {
		t.Error("tlString(nil) should be empty")
	}
	v := "x"
	if tlString(&v) != "x" {
		t.Error("tlString(&v) should be the value")
	}
}
