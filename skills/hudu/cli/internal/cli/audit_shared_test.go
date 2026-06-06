// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Table-driven tests for the audit feature's pure-logic helpers.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseAgeDays(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"180d", 180, false},
		{"26w", 182, false},
		{"6m", 180, false},
		{"2y", 730, false},
		{"90", 90, false},
		{" 30d ", 30, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-5d", 0, true},
	}
	for _, c := range cases {
		got, err := parseAgeDays(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseAgeDays(%q): expected error, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAgeDays(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseAgeDays(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestAgeAndDaysUntil(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	if d, ok := ageDays("2026-04-29T12:00:00Z", now); !ok || d != 30 {
		t.Errorf("ageDays 30 days ago = %d,%v want 30,true", d, ok)
	}
	if d, ok := daysUntil("2026-06-28T12:00:00Z", now); !ok || d != 30 {
		t.Errorf("daysUntil 30 days hence = %d,%v want 30,true", d, ok)
	}
	if d, ok := daysUntil("2026-05-19T12:00:00Z", now); !ok || d != -10 {
		t.Errorf("daysUntil expired = %d,%v want -10,true", d, ok)
	}
	if _, ok := ageDays("not-a-date", now); ok {
		t.Error("ageDays should fail on garbage")
	}
	// Plain date (no time) must parse.
	if _, ok := daysUntil("2026-12-31", now); !ok {
		t.Error("daysUntil should parse a plain YYYY-MM-DD date")
	}
}

func TestFilledLabels(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want map[string]bool
	}{
		{
			name: "array single label->value map",
			raw:  `[{"Hostname":"srv01","IP Address":"","Role":"dc"}]`,
			want: map[string]bool{"hostname": true, "ip address": false, "role": true},
		},
		{
			name: "array of label/value objects",
			raw:  `[{"label":"Hostname","value":"srv01"},{"label":"Role","value":""}]`,
			want: map[string]bool{"hostname": true, "role": false},
		},
		{
			name: "bare object",
			raw:  `{"CPU":"i7","Notes":null}`,
			want: map[string]bool{"cpu": true, "notes": false},
		},
		{
			name: "empty",
			raw:  ``,
			want: map[string]bool{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := filledLabels(json.RawMessage(c.raw))
			if len(got) != len(c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("label %q: got %v want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestLayoutFields(t *testing.T) {
	raw := `[{"label":"Hostname","field_type":"Text","required":true},
	         {"label":"Role","field_type":"Text"},
	         {"field_type":"Text"}]`
	fields := layoutFields(json.RawMessage(raw))
	if len(fields) != 2 {
		t.Fatalf("expected 2 named fields, got %d (%v)", len(fields), fields)
	}
	if fields[0].Label != "Hostname" || !fields[0].Required {
		t.Errorf("field 0 = %+v, want Hostname required", fields[0])
	}
	if fields[1].Required {
		t.Errorf("field 1 should not be required: %+v", fields[1])
	}
}

func TestNormalizeExpType(t *testing.T) {
	cases := map[string]string{
		"SSL Certificate": "ssl",
		"domain_whois":    "domain",
		"Warranty":        "warranty",
		"asset_password":  "password",
		"":                "other",
		"custom":          "custom",
	}
	for in, want := range cases {
		if got := normalizeExpType(in); got != want {
			t.Errorf("normalizeExpType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIntField(t *testing.T) {
	m := map[string]any{"a": float64(42), "b": "7", "c": "x"}
	if intField(m, "a") != 42 || intField(m, "b") != 7 || intField(m, "c") != 0 || intField(m, "missing") != 0 {
		t.Errorf("intField mismatch: %d %d %d %d", intField(m, "a"), intField(m, "b"), intField(m, "c"), intField(m, "missing"))
	}
}
