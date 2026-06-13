// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestVeeamJobHealth(t *testing.T) {
	cases := map[string]string{
		"Success":       "success",
		"success":       "success",
		"Warning":       "warning",
		"Failed":        "failed",
		"Error":         "failed",
		"Running":       "running",
		"Working":       "running",
		"Queued":        "running",
		"":              "unknown",
		"SomethingElse": "other",
	}
	for in, want := range cases {
		if got := veeamJobHealth(in); got != want {
			t.Errorf("veeamJobHealth(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVeeamAgentOnline(t *testing.T) {
	cases := map[string]bool{
		"Online":       true,
		"online":       true,
		"Healthy":      true,
		"Offline":      false,
		"Inaccessible": false,
		"":             false,
	}
	for in, want := range cases {
		if got := veeamAgentOnline(in); got != want {
			t.Errorf("veeamAgentOnline(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestVeeamSeverityRank(t *testing.T) {
	if veeamSeverityRank("Error") >= veeamSeverityRank("Warning") {
		t.Error("Error must rank more urgent (lower) than Warning")
	}
	if veeamSeverityRank("Warning") >= veeamSeverityRank("Info") {
		t.Error("Warning must rank more urgent than Info")
	}
	if veeamSeverityRank("nonsense") != 4 {
		t.Error("unknown severity must rank last")
	}
}

func TestParseVeeamTime(t *testing.T) {
	ok := []string{
		"2026-06-12T10:30:00Z",
		"2026-06-12T10:30:00.123Z",
		"2026-06-12T10:30:00",
		"2026-06-12",
	}
	for _, s := range ok {
		if _, parsed := parseVeeamTime(s); !parsed {
			t.Errorf("parseVeeamTime(%q) should parse", s)
		}
	}
	if _, parsed := parseVeeamTime(""); parsed {
		t.Error("empty string must not parse")
	}
	if _, parsed := parseVeeamTime("not-a-time"); parsed {
		t.Error("garbage must not parse")
	}
}

func TestVeeamParseDays(t *testing.T) {
	d, err := veeamParseDays("", 3)
	if err != nil || d != 3*24*time.Hour {
		t.Errorf("default days = %v, %v; want 72h", d, err)
	}
	d, err = veeamParseDays("2", 3)
	if err != nil || d != 48*time.Hour {
		t.Errorf("days 2 = %v, %v; want 48h", d, err)
	}
	if d, err := veeamParseDays("36h", 3); err != nil || d != 36*time.Hour {
		t.Errorf("days 36h = %v, %v; want 36h", d, err)
	}
	if _, err := veeamParseDays("-1", 3); err == nil {
		t.Error("negative days must error")
	}
}

func TestVeeamParseWindow(t *testing.T) {
	if d, err := veeamParseWindow("", 24*time.Hour); err != nil || d != 24*time.Hour {
		t.Errorf("default window = %v, %v; want 24h", d, err)
	}
	if d, err := veeamParseWindow("3d", time.Hour); err != nil || d != 72*time.Hour {
		t.Errorf("window 3d = %v, %v; want 72h", d, err)
	}
	if _, err := veeamParseWindow("nonsense", time.Hour); err == nil {
		t.Error("garbage window must error")
	}
}

func TestVeeamFieldAccessors(t *testing.T) {
	row := veeamRow{
		"name":   "Job A",
		"status": "Success",
		"count":  float64(5),
		"on":     true,
		"object": map[string]any{
			"organizationUid": "ORG-1",
			"nested":          map[string]any{"deep": "value"},
		},
	}
	if vstr(row, "name") != "Job A" {
		t.Error("vstr top-level failed")
	}
	if vstr(row, "object.organizationUid") != "ORG-1" {
		t.Error("vstr nested failed")
	}
	if vstr(row, "object.nested.deep") != "value" {
		t.Error("vstr deep-nested failed")
	}
	if vstr(row, "missing.path") != "" {
		t.Error("vstr missing path must be empty")
	}
	if vnum(row, "count") != 5 {
		t.Error("vnum failed")
	}
	if v, ok := vbool(row, "on"); !ok || !v {
		t.Error("vbool failed")
	}
	if vstr(row, "count") != "5" {
		t.Error("vstr should coerce number")
	}
}

func TestVeeamCompanyLabel(t *testing.T) {
	names := map[string]string{"org-1": "Alpha Corp"}
	if veeamCompanyLabel(names, "ORG-1") != "Alpha Corp" {
		t.Error("label must resolve case-insensitively")
	}
	if veeamCompanyLabel(names, "org-x") != "org-x" {
		t.Error("unknown uid must fall back to uid")
	}
	if veeamCompanyLabel(names, "") != "(unassigned)" {
		t.Error("empty uid must be (unassigned)")
	}
}

func TestRoundTo(t *testing.T) {
	if roundTo(3.14159, 1) != 3.1 {
		t.Errorf("roundTo(3.14159,1) = %v, want 3.1", roundTo(3.14159, 1))
	}
	if roundTo(-1, 1) != -1 {
		t.Error("negative sentinel must be preserved")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "  ", "x", "y") != "x" {
		t.Error("firstNonEmpty must skip blank/whitespace")
	}
	if firstNonEmpty("", "") != "" {
		t.Error("all-blank must be empty")
	}
}

func TestVeeamHoursSince(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	past := now.Add(-90 * time.Minute)
	if veeamHoursSince(now, past) != 1 {
		t.Errorf("hours since = %d, want 1", veeamHoursSince(now, past))
	}
	future := now.Add(time.Hour)
	if veeamHoursSince(now, future) != 0 {
		t.Error("future time must clamp to 0 hours")
	}
}
