// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Unit tests for the pure transcendence helpers: duration parsing, formatting,
// tokenization, TF-IDF ranking, JSON:API name navigation, and open-state logic.

package cli

import (
	"testing"
	"time"
)

func TestParseWindowDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"30d", 30 * 24 * time.Hour, true},
		{"12h", 12 * time.Hour, true},
		{"90m", 90 * time.Minute, true},
		{"", 0, false},
		{"banana", 0, false},
	}
	for _, c := range cases {
		got, ok := parseWindowDuration(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseWindowDuration(%q) = (%v,%v), want (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{90 * time.Minute, "1h 30m"},
		{50 * time.Hour, "2d 2h"},
		{30 * time.Second, "0m"},
		{0, "0m"},
	}
	for _, c := range cases {
		if got := humanDuration(c.d); got != c.want {
			t.Errorf("humanDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestNovelTokenizeDropsStopwords(t *testing.T) {
	toks := novelTokenize("The Checkout API is on FIRE incident")
	got := map[string]bool{}
	for _, tk := range toks {
		got[tk] = true
	}
	if got["the"] || got["is"] || got["on"] || got["incident"] {
		t.Errorf("expected stopwords dropped, got %v", toks)
	}
	if !got["checkout"] || !got["api"] || !got["fire"] {
		t.Errorf("expected content tokens kept, got %v", toks)
	}
}

func TestTFIDFRanksSharedTermsHigher(t *testing.T) {
	corpus := newTFIDF(map[string]string{
		"a": "database connection pool exhausted",
		"b": "database connection timeout error",
		"c": "slack notification delivery failure",
	})
	av := corpus.vec("a")
	simB := cosine(av, corpus.vec("b")) // shares database/connection
	simC := cosine(av, corpus.vec("c")) // shares nothing
	if !(simB > simC) {
		t.Errorf("expected b more similar to a than c: simB=%v simC=%v", simB, simC)
	}
}

func TestRecNameNavigatesJSONAPI(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"plain", "plain"},
		{map[string]any{"name": "direct"}, "direct"},
		{map[string]any{"data": map[string]any{"attributes": map[string]any{"name": "nested"}}}, "nested"},
		{map[string]any{"full_name": "Ada Lovelace"}, "Ada Lovelace"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := recName(c.in); got != c.want {
			t.Errorf("recName(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIncidentOpen(t *testing.T) {
	open := record{Attrs: map[string]any{"status": "started"}}
	if !incidentOpen(open) {
		t.Errorf("started incident should be open")
	}
	resolved := record{Attrs: map[string]any{"status": "resolved", "resolved_at": "2026-01-01T00:00:00Z"}}
	if incidentOpen(resolved) {
		t.Errorf("resolved incident should not be open")
	}
	cancelled := record{Attrs: map[string]any{"status": "cancelled"}}
	if incidentOpen(cancelled) {
		t.Errorf("cancelled incident should not be open")
	}
}
