// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Tests for the reprint-added novel helpers (config-diff, sla-breach, alert-noise).

package cli

import (
	"testing"
	"time"
)

func TestNovelConfigHash(t *testing.T) {
	tests := []struct {
		name string
		a    map[string]any
		b    map[string]any
		same bool
	}{
		{"identical maps hash equal", map[string]any{"name": "svc", "n": 1.0}, map[string]any{"n": 1.0, "name": "svc"}, true},
		{"changed value hashes differ", map[string]any{"name": "svc"}, map[string]any{"name": "svc2"}, false},
		{"added key hashes differ", map[string]any{"name": "svc"}, map[string]any{"name": "svc", "x": true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ha, hb := novelConfigHash(tt.a), novelConfigHash(tt.b)
			if ha == "" || hb == "" {
				t.Fatalf("empty hash: %q %q", ha, hb)
			}
			if (ha == hb) != tt.same {
				t.Errorf("hash equality = %v, want %v (%q vs %q)", ha == hb, tt.same, ha, hb)
			}
		})
	}
}

func TestSlaTargetDuration(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]any
		want  time.Duration
		ok    bool
	}{
		{"duration string", map[string]any{"duration": "2h"}, 2 * time.Hour, true},
		{"days shorthand", map[string]any{"target": "1d"}, 24 * time.Hour, true},
		{"bare minutes number", map[string]any{"minutes": 90.0}, 90 * time.Minute, true},
		{"large bare number is seconds", map[string]any{"time": 7200.0}, 2 * time.Hour, true},
		{"nothing parseable", map[string]any{"name": "gold"}, 0, false},
		{"empty attrs", map[string]any{}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := slaTargetDuration(tt.attrs)
			if ok != tt.ok || got != tt.want {
				t.Errorf("slaTargetDuration() = (%v, %v), want (%v, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestAlertNoiseKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"digits stripped so percentages group", "disk 91% full", "disk % full"},
		{"same alert different number groups equal", "disk 97% full", "disk % full"},
		{"case and whitespace normalized", "  CPU   High\tLoad ", "cpu high load"},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := alertNoiseKey(tt.in); got != tt.want {
				t.Errorf("alertNoiseKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
