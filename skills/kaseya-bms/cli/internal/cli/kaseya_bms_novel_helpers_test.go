// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the Kaseya BMS novel commands.

package cli

import (
	"testing"
	"time"
)

func TestKbmsStrCasing(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want string
	}{
		{"pascal", map[string]any{"QueueName": "SD"}, "QueueName", "SD"},
		{"camel", map[string]any{"queueName": "SD"}, "QueueName", "SD"},
		{"lower", map[string]any{"queuename": "SD"}, "QueueName", "SD"},
		{"missing", map[string]any{}, "QueueName", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kbmsStr(tt.m, tt.key); got != tt.want {
				t.Errorf("kbmsStr = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKbmsNum(t *testing.T) {
	tests := []struct {
		name   string
		m      map[string]any
		want   float64
		wantOK bool
	}{
		{"number", map[string]any{"Timespent": float64(90)}, 90, true},
		{"string number", map[string]any{"timespent": "90.5"}, 90.5, true},
		{"garbage string", map[string]any{"Timespent": "abc"}, 0, false},
		{"missing", map[string]any{}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := kbmsNum(tt.m, "Timespent")
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("kbmsNum = (%v, %v), want (%v, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestKbmsBool(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want bool
	}{
		{"bool true", map[string]any{"IsBillable": true}, true},
		{"number 1", map[string]any{"isBillable": float64(1)}, true},
		{"string true", map[string]any{"IsBillable": "true"}, true},
		{"bool false", map[string]any{"IsBillable": false}, false},
		{"missing", map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kbmsBool(tt.m, "IsBillable"); got != tt.want {
				t.Errorf("kbmsBool = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKbmsTime(t *testing.T) {
	tests := []struct {
		name   string
		val    string
		wantOK bool
	}{
		{"iso no zone", "2026-06-01T10:00:00", true},
		{"rfc3339", "2026-06-01T10:00:00Z", true},
		{"date only", "2026-06-01", true},
		{"odata literal", "/Date(1717236000000)/", true},
		{"garbage", "not-a-date", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := kbmsTime(map[string]any{"StartDate": tt.val}, "StartDate")
			if ok != tt.wantOK {
				t.Errorf("kbmsTime(%q) ok = %v, want %v", tt.val, ok, tt.wantOK)
			}
		})
	}
}

func TestKbmsTicketOpen(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want bool
	}{
		{"open new", map[string]any{"StatusName": "New"}, true},
		{"completed date set", map[string]any{"StatusName": "New", "CompletedDate": "2026-06-01T00:00:00"}, false},
		{"terminal status", map[string]any{"StatusName": "Closed"}, false},
		{"resolved-ish status", map[string]any{"StatusName": "Auto Resolved"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kbmsTicketOpen(tt.m); got != tt.want {
				t.Errorf("kbmsTicketOpen = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKbmsHoursFromTimespent(t *testing.T) {
	tests := []struct {
		unit string
		raw  float64
		want float64
	}{
		{"minutes", 90, 1.5},
		{"hours", 2.5, 2.5},
		{"seconds", 7200, 2},
		{"", 60, 1},
	}
	for _, tt := range tests {
		if got := kbmsHoursFromTimespent(tt.raw, tt.unit); got != tt.want {
			t.Errorf("kbmsHoursFromTimespent(%v, %q) = %v, want %v", tt.raw, tt.unit, got, tt.want)
		}
	}
}

func TestKbmsTicketActivityFallback(t *testing.T) {
	m := map[string]any{"CreatedOn": "2026-06-01T00:00:00"}
	got, ok := kbmsTicketActivity(m)
	if !ok || got.Year() != 2026 {
		t.Fatalf("expected CreatedOn fallback, got (%v, %v)", got, ok)
	}
	m["ModifiedOn"] = "2026-06-02T00:00:00"
	got, _ = kbmsTicketActivity(m)
	if got.Day() != 2 {
		t.Errorf("ModifiedOn should win over CreatedOn, got %v", got)
	}
	m["LastActivityUpdate"] = "2026-06-03T00:00:00"
	got, _ = kbmsTicketActivity(m)
	if got.Day() != 3 {
		t.Errorf("LastActivityUpdate should win, got %v", got)
	}
	if _, ok := kbmsTicketActivity(map[string]any{}); ok {
		t.Errorf("no timestamps should report ok=false")
	}
	_ = time.Now
}

func TestKbmsRound2Negative(t *testing.T) {
	tests := []struct{ in, want float64 }{
		{1.006, 1.01},
		{-1.006, -1.01}, // symmetric for negative values (the truncate form returned -1.0)
		{2.344, 2.34},
		{-1.5, -1.5},
	}
	for _, tt := range tests {
		if got := kbmsRound2(tt.in); got != tt.want {
			t.Errorf("kbmsRound2(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestKbmsIDString(t *testing.T) {
	if got := kbmsIDString(map[string]any{"UserId": float64(42)}, "UserId"); got != "42" {
		t.Errorf("numeric id = %q, want 42", got)
	}
	if got := kbmsIDString(map[string]any{"userId": "abc-1"}, "UserId"); got != "abc-1" {
		t.Errorf("string id = %q, want abc-1", got)
	}
	if got := kbmsIDString(map[string]any{}, "UserId"); got != "" {
		t.Errorf("missing id = %q, want empty", got)
	}
}

func TestKbmsNormName(t *testing.T) {
	if got := kbmsNormName("  Jane   SMITH "); got != "jane smith" {
		t.Errorf("norm = %q, want 'jane smith'", got)
	}
}
