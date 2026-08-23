// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"resourceguru-pp-cli/internal/store"
)

func mustDay(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(utilDateFmt, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBuildUtilization(t *testing.T) {
	s := newTestStore(t)
	// Use the typed upserts sync actually calls — they populate the generic
	// resources/bookings tables that List() reads.
	if err := s.UpsertResources(json.RawMessage(`{"id":1,"name":"Alice","minutes_per_day":480}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertResources(json.RawMessage(`{"id":2,"name":"Boardroom","minutes_per_day":0}`)); err != nil {
		t.Fatal(err)
	}
	// Alice: 4h on day1, full day2, overbooked day3 (10h > 8h capacity).
	if err := s.UpsertBookings(json.RawMessage(`{"id":10,"resource_id":1,"durations":[
		{"date":"2026-06-01","duration":240},
		{"date":"2026-06-02","duration":480},
		{"date":"2026-06-03","duration":600}]}`)); err != nil {
		t.Fatal(err)
	}

	rows, err := buildUtilization(s, mustDay(t, "2026-06-01"), mustDay(t, "2026-06-03"), "")
	if err != nil {
		t.Fatalf("buildUtilization: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 resources, got %d", len(rows))
	}
	var alice resourceUtil
	for _, r := range rows {
		if r.ID == "1" {
			alice = r
		}
	}
	if alice.Name != "Alice" {
		t.Fatalf("resource 1 should be Alice, got %q", alice.Name)
	}
	if got := alice.Days[0].Utilization; got == nil || *got != 0.5 {
		t.Errorf("day1 utilization = %v, want 0.5", got)
	}
	if got := alice.Days[1].Utilization; got == nil || *got != 1.0 {
		t.Errorf("day2 utilization = %v, want 1.0", got)
	}
	if alice.OverbookedDays != 1 {
		t.Errorf("overbooked days = %d, want 1 (day3 is 10h > 8h)", alice.OverbookedDays)
	}
	if alice.BookedMinutes != 1320 {
		t.Errorf("booked minutes = %d, want 1320", alice.BookedMinutes)
	}
	// avg = 1320 booked / 1440 capacity (3 days * 480)
	if got := alice.AvgUtilization; got == nil || (*got < 0.916 || *got > 0.917) {
		t.Errorf("avg utilization = %v, want ~0.9167", got)
	}

	// Zero-capacity resource (Boardroom) must report nil utilization, not divide by zero.
	var room resourceUtil
	for _, r := range rows {
		if r.ID == "2" {
			room = r
		}
	}
	if room.AvgUtilization != nil {
		t.Errorf("zero-capacity resource avg should be nil, got %v", *room.AvgUtilization)
	}
}

func TestBuildUtilizationResourceIDsAttribution(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"1", "2"} {
		if err := s.UpsertResources(json.RawMessage(`{"id":` + id + `,"name":"R` + id + `","minutes_per_day":480}`)); err != nil {
			t.Fatal(err)
		}
	}
	// A group booking with resource_ids attributes its duration to BOTH resources.
	if err := s.UpsertBookings(json.RawMessage(`{"id":20,"resource_ids":[1,2],"durations":[{"date":"2026-06-01","duration":480}]}`)); err != nil {
		t.Fatal(err)
	}
	rows, err := buildUtilization(s, mustDay(t, "2026-06-01"), mustDay(t, "2026-06-01"), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.BookedMinutes != 480 {
			t.Errorf("resource %s booked = %d, want 480 (group booking attributed to each)", r.ID, r.BookedMinutes)
		}
	}
}

func TestResolveWindow(t *testing.T) {
	// explicit range
	start, end, err := resolveWindow("2026-06-01", "2026-06-30")
	if err != nil {
		t.Fatal(err)
	}
	if start.Format(utilDateFmt) != "2026-06-01" || end.Format(utilDateFmt) != "2026-06-30" {
		t.Errorf("got %s..%s", start.Format(utilDateFmt), end.Format(utilDateFmt))
	}
	// default end = start + 27d
	_, end2, err := resolveWindow("2026-06-01", "")
	if err != nil {
		t.Fatal(err)
	}
	if end2.Format(utilDateFmt) != "2026-06-28" {
		t.Errorf("default end = %s, want 2026-06-28", end2.Format(utilDateFmt))
	}
	// errors
	if _, _, err := resolveWindow("nope", ""); err == nil {
		t.Error("invalid start should error")
	}
	if _, _, err := resolveWindow("2026-06-30", "2026-06-01"); err == nil {
		t.Error("end before start should error")
	}
	if _, _, err := resolveWindow("2026-01-01", "2027-12-31"); err == nil {
		t.Error("window > 366 days should error")
	}
}

func TestBuildUtilizationWeekdayCapacity(t *testing.T) {
	s := newTestStore(t)
	// Available Mon–Fri 09:00–17:00 (540→1020 = 480 min); no weekend periods.
	periods := ""
	for wd := 1; wd <= 5; wd++ {
		if periods != "" {
			periods += ","
		}
		periods += `{"week_day":` + strconv.Itoa(wd) + `,"start_time":540,"end_time":1020,"valid_until":null}`
	}
	if err := s.UpsertResources(json.RawMessage(`{"id":1,"name":"Alice","minutes_per_day":480,"available_periods":[` + periods + `]}`)); err != nil {
		t.Fatal(err)
	}
	// A full week.
	rows, err := buildUtilization(s, mustDay(t, "2026-06-01"), mustDay(t, "2026-06-07"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 resource, got %d", len(rows))
	}
	for _, cell := range rows[0].Days {
		d := mustDay(t, cell.Date)
		wd := d.Weekday()
		wantCap := 480
		if wd == time.Saturday || wd == time.Sunday {
			wantCap = 0
		}
		if cell.CapacityMinutes != wantCap {
			t.Errorf("%s (%s) capacity = %d, want %d", cell.Date, wd, cell.CapacityMinutes, wantCap)
		}
		if wantCap == 0 && cell.Utilization != nil {
			t.Errorf("%s should have nil utilization (no capacity)", cell.Date)
		}
	}
}

func TestNormalizeDate(t *testing.T) {
	cases := map[string]string{
		"2026-06-01":             "2026-06-01",
		"2026-06-01T00:00:00Z":   "2026-06-01",
		"2026-06-01T12:30:00-07": "2026-06-01",
		"short":                  "short",
	}
	for in, want := range cases {
		if got := normalizeDate(in); got != want {
			t.Errorf("normalizeDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildUtilizationDedupsRepeatedResourceID(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpsertResources(json.RawMessage(`{"id":1,"name":"Alice","minutes_per_day":480}`)); err != nil {
		t.Fatal(err)
	}
	// resource_ids repeats id 1; minutes must count once, not twice.
	if err := s.UpsertBookings(json.RawMessage(`{"id":30,"resource_id":1,"resource_ids":[1,1],"durations":[{"date":"2026-06-01","duration":240}]}`)); err != nil {
		t.Fatal(err)
	}
	rows, err := buildUtilization(s, mustDay(t, "2026-06-01"), mustDay(t, "2026-06-01"), "")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].BookedMinutes != 240 {
		t.Errorf("booked = %d, want 240 (repeated id must not double-count)", rows[0].BookedMinutes)
	}
}

func TestDayRange(t *testing.T) {
	days := dayRange(mustDay(t, "2026-06-01"), mustDay(t, "2026-06-03"))
	want := []string{"2026-06-01", "2026-06-02", "2026-06-03"}
	if len(days) != len(want) {
		t.Fatalf("got %d days, want %d", len(days), len(want))
	}
	for i := range want {
		if days[i] != want[i] {
			t.Errorf("day[%d] = %s, want %s", i, days[i], want[i])
		}
	}
}
