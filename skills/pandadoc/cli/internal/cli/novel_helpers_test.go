// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDocTime(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		zero  bool
		year  int
		month time.Month
	}{
		{"rfc3339", "2026-05-30T14:05:51Z", false, 2026, time.May},
		{"fractional seconds", "2026-05-30T14:05:51.123456Z", false, 2026, time.May},
		{"numeric offset", "2026-05-30T14:05:51+02:00", false, 2026, time.May},
		{"date only", "2026-01-15", false, 2026, time.January},
		{"empty", "", true, 0, 0},
		{"garbage", "not-a-date", true, 0, 0},
		{"whitespace", "   ", true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDocTime(tt.in)
			if got.IsZero() != tt.zero {
				t.Fatalf("parseDocTime(%q).IsZero() = %v, want %v", tt.in, got.IsZero(), tt.zero)
			}
			if !tt.zero && (got.Year() != tt.year || got.Month() != tt.month) {
				t.Errorf("parseDocTime(%q) = %v, want year=%d month=%v", tt.in, got, tt.year, tt.month)
			}
		})
	}
}

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"sent", "document.sent"},
		{"VIEWED", "document.viewed"},
		{" completed ", "document.completed"},
		{"document.draft", "document.draft"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeStatus(tt.in); got != tt.want {
			t.Errorf("normalizeStatus(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestShortStatus(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"document.sent", "sent"},
		{"sent", "sent"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := shortStatus(tt.in); got != tt.want {
			t.Errorf("shortStatus(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseStatusList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"two shortcuts", "sent,viewed", []string{"document.sent", "document.viewed"}},
		{"spaces and empties", " sent , , completed ", []string{"document.sent", "document.completed"}},
		{"empty input", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStatusList(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parseStatusList(%q) returned %d entries, want %d", tt.in, len(got), len(tt.want))
			}
			for _, w := range tt.want {
				if !got[w] {
					t.Errorf("parseStatusList(%q) missing %q", tt.in, w)
				}
			}
		})
	}
}

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"completed", true},
		{"document.paid", true},
		{"declined", true},
		{"voided", true},
		{"expired", true},
		{"rejected", true},
		{"sent", false},
		{"viewed", false},
		{"draft", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isTerminalStatus(tt.in); got != tt.want {
			t.Errorf("isTerminalStatus(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseNovelDoc(t *testing.T) {
	raw := []byte(`{
		"id": "doc1",
		"name": "MSA — Acme",
		"status": "document.sent",
		"date_created": "2026-05-01T10:00:00Z",
		"date_modified": "2026-05-10T10:00:00Z",
		"date_completed": "",
		"template": {"id": "tpl1", "name": "MSA Template"},
		"grand_total": {"amount": "1234.50", "currency": "USD"}
	}`)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	d := parseNovelDoc(m)
	if d.ID != "doc1" || d.Name != "MSA — Acme" || d.Status != "document.sent" {
		t.Errorf("identity fields wrong: %+v", d)
	}
	if d.TemplateID != "tpl1" || d.TemplateName != "MSA Template" {
		t.Errorf("template fields wrong: %+v", d)
	}
	if d.GrandTotal != 1234.50 || d.Currency != "USD" {
		t.Errorf("value fields wrong: GrandTotal=%v Currency=%q", d.GrandTotal, d.Currency)
	}
	if d.DateCreated.IsZero() || d.DateModified.IsZero() || !d.DateCompleted.IsZero() {
		t.Errorf("date fields wrong: %+v", d)
	}
}

func TestParseNovelDocTolerant(t *testing.T) {
	// Missing keys, non-string values, malformed grand_total must not panic
	// and must degrade to zero values.
	raw := []byte(`{"id": 42, "grand_total": "free", "template": []}`)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	d := parseNovelDoc(m)
	if d.ID != "" || d.GrandTotal != 0 || d.TemplateID != "" {
		t.Errorf("tolerant parse wrong: %+v", d)
	}
}

func TestJSONStr(t *testing.T) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{"a": "x", "b": 7, "c": null}`), &m); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		key  string
		want string
	}{
		{"a", "x"},
		{"b", ""},
		{"c", ""},
		{"missing", ""},
	}
	for _, tt := range tests {
		if got := jsonStr(m, tt.key); got != tt.want {
			t.Errorf("jsonStr(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestLastActivity(t *testing.T) {
	created := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	modified := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		doc  novelDoc
		want time.Time
	}{
		{"modified wins", novelDoc{DateCreated: created, DateModified: modified}, modified},
		{"falls back to created", novelDoc{DateCreated: created}, created},
		{"both zero", novelDoc{}, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.doc.lastActivity(); !got.Equal(tt.want) {
				t.Errorf("lastActivity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgeDays(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want func(int) bool
	}{
		{"zero time is ancient", time.Time{}, func(d int) bool { return d >= 1<<20 }},
		{"ten days ago", time.Now().Add(-10 * 24 * time.Hour), func(d int) bool { return d == 10 }},
		{"now", time.Now(), func(d int) bool { return d == 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ageDays(tt.in); !tt.want(got) {
				t.Errorf("ageDays(%v) = %d, outside expected range", tt.in, got)
			}
		})
	}
}
