// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestNvScalarString(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{float64(101), "101"},
		{float64(101.5), "101.5"},
		{"abc", "abc"},
		{true, "true"},
	}
	for _, c := range cases {
		if got := nvScalarString(c.in); got != c.want {
			t.Errorf("nvScalarString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNvEpoch(t *testing.T) {
	obj := map[string]any{"lastContact": float64(1_700_000_000), "iso": "2026-01-02T03:04:05Z"}
	if ts, ok := nvEpoch(obj, "lastContact"); !ok || ts.Year() != 2023 {
		t.Errorf("epoch seconds parse failed: %v ok=%v", ts, ok)
	}
	if ts, ok := nvEpoch(obj, "iso"); !ok || ts.Year() != 2026 {
		t.Errorf("RFC3339 parse failed: %v ok=%v", ts, ok)
	}
	if _, ok := nvEpoch(obj, "missing"); ok {
		t.Errorf("expected missing key to return ok=false")
	}
}

func TestComputeStaleDevices(t *testing.T) {
	now := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	devices := map[string]nvDevice{
		"1": {ID: "1", Name: "old", OrgID: "a", LastContact: now.AddDate(0, 0, -40), HasContact: true},
		"2": {ID: "2", Name: "fresh", OrgID: "a", LastContact: now.AddDate(0, 0, -2), HasContact: true},
		"3": {ID: "3", Name: "never", OrgID: "b", HasContact: false},
	}
	out := computeStaleDevices(devices, map[string]string{"a": "Acme"}, 14, now)
	// "old" (40d) and "never" qualify; "fresh" (2d) does not.
	if len(out) != 2 {
		t.Fatalf("want 2 stale, got %d: %+v", len(out), out)
	}
	if filterStaleByOrg(out, "a")[0].DeviceName != "old" {
		t.Errorf("org filter wrong: %+v", filterStaleByOrg(out, "a"))
	}
}

func TestAggregateSoftware(t *testing.T) {
	rows := []map[string]any{
		{"deviceId": float64(1), "name": "Chrome", "version": "120"},
		{"deviceId": float64(2), "name": "Chrome", "version": "121"},
		{"deviceId": float64(3), "name": "Chrome", "version": "120"},
		{"deviceId": float64(1), "name": "7-Zip", "version": "23"},
	}
	out := aggregateSoftware(rows, "", 2)
	if len(out) != 1 || out[0].Name != "Chrome" {
		t.Fatalf("want only Chrome with >=2 versions, got %+v", out)
	}
	if out[0].Installs != 3 || out[0].DistinctVersions != 2 {
		t.Errorf("Chrome installs=%d versions=%d, want 3/2", out[0].Installs, out[0].DistinctVersions)
	}
	// name filter
	if got := aggregateSoftware(rows, "zip", 1); len(got) != 1 || got[0].Name != "7-Zip" {
		t.Errorf("name filter failed: %+v", got)
	}
}

func TestLookupEOLLongestMatch(t *testing.T) {
	eol, ok := lookupEOL("Windows Server 2012 R2 Standard")
	if !ok || eol.Format("2006-01-02") != "2023-10-10" {
		t.Errorf("Server 2012 R2 = %v ok=%v", eol, ok)
	}
	if _, ok := lookupEOL("Windows 11 Pro"); ok {
		t.Errorf("Windows 11 should not be in EOL table")
	}
}

func TestComputeOsEOL(t *testing.T) {
	now := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	devices := map[string]nvDevice{
		"1": {ID: "1", Name: "srv", OrgID: "a", OSName: "Windows Server 2012 R2"},
		"2": {ID: "2", Name: "ws", OrgID: "a", OSName: "Windows 11"},
		"3": {ID: "3", Name: "noos", OrgID: "a"},
	}
	out := computeOsEOL(devices, map[string]string{"a": "Acme"}, map[string]string{"3": "Windows 7 Pro"}, now)
	// srv (2012 R2, EOL) + noos (enriched to Win7, EOL). ws (Win11) excluded.
	if len(out) != 2 {
		t.Fatalf("want 2 EOL devices, got %d: %+v", len(out), out)
	}
	if out[0].DaysPastEOL < out[1].DaysPastEOL {
		t.Errorf("expected sort by days-past desc: %+v", out)
	}
}

func TestComputeAVSweep(t *testing.T) {
	now := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	devices := map[string]nvDevice{"1": {ID: "1", Name: "d1", OrgID: "a"}}
	threats := []map[string]any{
		{"deviceId": float64(1), "threatName": "Trojan.Generic"},
		{"deviceId": float64(1), "threatName": "Adware.X"},
	}
	status := []map[string]any{
		{"deviceId": float64(1), "definitionDate": float64(now.AddDate(0, 0, -10).Unix())},
	}
	// threat filter only
	hits := computeAVSweep(threats, status, devices, map[string]string{"a": "Acme"}, "trojan", 0, now)
	if len(hits) != 1 || hits[0].Detail != "Trojan.Generic" {
		t.Fatalf("threat filter: want 1 trojan hit, got %+v", hits)
	}
	// stale defs
	hits2 := computeAVSweep(nil, status, devices, nil, "", 7, now)
	if len(hits2) != 1 || hits2[0].Kind != "stale-definitions" {
		t.Fatalf("stale defs: want 1 hit, got %+v", hits2)
	}
}

func TestBuildDriftRows(t *testing.T) {
	cur := map[string]float64{"a": 50, "b": 100, "c": 80}
	prev := map[string]float64{"a": 0, "b": 100}
	rows := buildDriftRows(cur, prev, map[string]string{}, false)
	dir := map[string]string{}
	for _, r := range rows {
		dir[r.OrgID] = r.Direction
	}
	if dir["a"] != "better" || dir["b"] != "flat" || dir["c"] != "new" {
		t.Errorf("directions wrong: %+v", dir)
	}

	// stale is count-shaped: more stale devices is a regression.
	staleCur := map[string]float64{"a": 9, "b": 2, "c": 5}
	stalePrev := map[string]float64{"a": 4, "b": 6, "c": 5}
	rows = buildDriftRows(staleCur, stalePrev, map[string]string{}, true)
	dir = map[string]string{}
	delta := map[string]float64{}
	for _, r := range rows {
		dir[r.OrgID] = r.Direction
		delta[r.OrgID] = r.Delta
	}
	if dir["a"] != "worse" || dir["b"] != "better" || dir["c"] != "flat" {
		t.Errorf("lowerIsBetter directions wrong: %+v", dir)
	}
	// Negative deltas must round half-away-from-zero, not toward +inf.
	if delta["a"] != 5 || delta["b"] != -4 || delta["c"] != 0 {
		t.Errorf("deltas wrong (negative rounding bias?): %+v", delta)
	}
}

func TestBuildComplianceRowsFilter(t *testing.T) {
	stats := map[string]*orgPatchStat{
		"a": {OrgID: "a", Devices: 4, NonCompliant: 1}, // 75%
		"b": {OrgID: "b", Devices: 2, NonCompliant: 0}, // 100%
	}
	out := buildComplianceRows(stats, map[string]string{"a": "Acme", "b": "Beta"}, 95)
	if len(out) != 1 || out[0].OrgID != "a" {
		t.Fatalf("min-pct 95 should keep only org a (75%%), got %+v", out)
	}
	if out[0].CompliancePct != 75 {
		t.Errorf("compliance pct = %v want 75", out[0].CompliancePct)
	}
}

func TestComputeFleetHealthWeighting(t *testing.T) {
	devices := map[string]nvDevice{
		"1": {ID: "1", OrgID: "a"},
		"2": {ID: "2", OrgID: "a"},
	}
	stats := map[string]*orgPatchStat{"a": {OrgID: "a", Devices: 2, NonCompliant: 2}} // patch 0
	gaps := []backupCoverageRow{{OrgID: "a"}}                                         // 1 of 2 => backup 50
	threats := []map[string]any{}                                                     // av 100
	stale := []staleRow{}                                                             // stale 100
	out := computeFleetHealth(devices, map[string]string{"a": "Acme"}, stats, gaps, threats, stale)
	if len(out) != 1 {
		t.Fatalf("want 1 org, got %d", len(out))
	}
	// 0*.35 + 50*.30 + 100*.20 + 100*.15 = 50
	if out[0].Score != 50 {
		t.Errorf("health score = %v, want 50", out[0].Score)
	}
}
