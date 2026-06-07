// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestTWindow(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 24 * time.Hour},
		{"2h", 2 * time.Hour},
		{"24h", 24 * time.Hour},
		{"7d", 168 * time.Hour},     // 7 * 24h, via ParseDurationLoose
		{"1w", 168 * time.Hour},     // 1 week == 168h, now supported
		{"-2h", 2 * time.Hour},      // negative clamped to absolute value
		{"garbage", 24 * time.Hour}, // unparseable falls back to default
	}
	for _, c := range cases {
		if got := tWindow(c.in); got != c.want {
			t.Errorf("tWindow(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
