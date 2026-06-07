// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Table-driven tests for the hand-built analytics engine and auth login helpers.

package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, ok := parseBlumiraTime(s)
	if !ok {
		t.Fatalf("parseBlumiraTime(%q) failed", s)
	}
	return tm
}

func TestParseBlumiraTime(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2026-05-29T21:00:00Z", true},
		{"2026-05-29T21:00:00.123456Z", true},
		{"2026-05-29 21:00:00", true},
		{"2026-05-29", true},
		{"", false},
		{"not-a-date", false},
	}
	for _, c := range cases {
		if _, ok := parseBlumiraTime(c.in); ok != c.ok {
			t.Errorf("parseBlumiraTime(%q) ok=%v, want %v", c.in, ok, c.ok)
		}
	}
}

func TestParseWindow(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		err  bool
	}{
		{"", 0, false},
		{"24h", 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"bogus", 0, true},
	}
	for _, c := range cases {
		got, err := parseWindow(c.in)
		if (err != nil) != c.err {
			t.Errorf("parseWindow(%q) err=%v, want err=%v", c.in, err, c.err)
			continue
		}
		if !c.err && got != c.want {
			t.Errorf("parseWindow(%q)=%v, want %v", c.in, got, c.want)
		}
	}
}

func TestParsePriorityFilter(t *testing.T) {
	cases := []struct {
		spec    string
		p       int
		match   bool
		wantErr bool
	}{
		{"", 5, true, false},
		{"high", 1, true, false},
		{"high", 2, true, false},
		{"high", 3, false, false},
		{"critical", 1, true, false},
		{"critical", 2, false, false},
		{"p1", 1, true, false},
		{"low", 4, true, false},
		{"low", 3, false, false},
		{"3", 3, true, false},
		{"3", 2, false, false},
		{"nonsense", 1, false, true},
	}
	for _, c := range cases {
		fn, err := parsePriorityFilter(c.spec)
		if (err != nil) != c.wantErr {
			t.Errorf("parsePriorityFilter(%q) err=%v wantErr=%v", c.spec, err, c.wantErr)
			continue
		}
		if c.wantErr {
			continue
		}
		if got := fn(c.p); got != c.match {
			t.Errorf("parsePriorityFilter(%q)(%d)=%v want %v", c.spec, c.p, got, c.match)
		}
	}
}

func TestStatusSemantics(t *testing.T) {
	resolvedByCode := findingRec{Status: resolvedStatusCode}
	resolvedByName := findingRec{Status: 99, StatusName: "Resolved"}
	open := findingRec{Status: 10, StatusName: "Open"}
	if !resolvedByCode.IsResolved() || !resolvedByName.IsResolved() {
		t.Error("resolved findings not detected")
	}
	if open.IsResolved() || !open.IsOpen() {
		t.Error("open finding misclassified")
	}
}

func TestComputeTriage(t *testing.T) {
	now := mustTime(t, "2026-05-29T12:00:00Z")
	findings := []findingRec{
		{ID: "a", Name: "low-old", Priority: 4, Status: 10, StatusName: "Open", Account: "acct1", Created: "2026-05-28T12:00:00Z"},
		{ID: "b", Name: "p1-new", Priority: 1, Status: 10, StatusName: "Open", Account: "acct2", Created: "2026-05-29T11:00:00Z"},
		{ID: "c", Name: "p1-old", Priority: 1, Status: 10, StatusName: "Open", Account: "acct3", Created: "2026-05-27T12:00:00Z"},
		{ID: "d", Name: "resolved", Priority: 1, Status: resolvedStatusCode, StatusName: "Resolved", Account: "acct1", Created: "2026-05-29T11:00:00Z"},
	}
	prio, _ := parsePriorityFilter("")
	status, _ := parseStatusFilter("open")
	rows := computeTriage(findings, now, triageOpts{priorityMatch: prio, statusMatch: status})
	// resolved finding "d" must be excluded
	if len(rows) != 3 {
		t.Fatalf("expected 3 open rows, got %d", len(rows))
	}
	// ranking: P1 before P4; within P1, older (c) before newer (b)
	if rows[0].FindingID != "c" || rows[1].FindingID != "b" || rows[2].FindingID != "a" {
		t.Errorf("triage ranking wrong: got %s,%s,%s", rows[0].FindingID, rows[1].FindingID, rows[2].FindingID)
	}
	// age computed: c was created 2026-05-27T12:00 vs now 2026-05-29T12:00 => 48h
	if rows[0].AgeHours != 48 {
		t.Errorf("age for c = %d, want 48", rows[0].AgeHours)
	}
	// negative test: high filter excludes the P4
	prioHigh, _ := parsePriorityFilter("high")
	high := computeTriage(findings, now, triageOpts{priorityMatch: prioHigh, statusMatch: status})
	for _, r := range high {
		if r.Priority > 2 {
			t.Errorf("high filter leaked priority %d", r.Priority)
		}
	}
}

func TestComputeSLA(t *testing.T) {
	now := mustTime(t, "2026-05-29T12:00:00Z")
	findings := []findingRec{
		// P1 SLA=4h; created 6h ago => breached
		{ID: "breach", Priority: 1, Status: 10, StatusName: "Open", Created: "2026-05-29T06:00:00Z"},
		// P1 created 1h ago => 3h remaining (not breached)
		{ID: "ok", Priority: 1, Status: 10, StatusName: "Open", Created: "2026-05-29T11:00:00Z"},
		// resolved finding excluded
		{ID: "done", Priority: 1, Status: resolvedStatusCode, StatusName: "Resolved", Created: "2026-05-29T00:00:00Z"},
	}
	rows := computeSLA(findings, now, slaOpts{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 open rows, got %d", len(rows))
	}
	// breached sorts first with negative breach_in
	if rows[0].FindingID != "breach" || !rows[0].Breached || rows[0].BreachInHrs >= 0 {
		t.Errorf("breach row wrong: %+v", rows[0])
	}
	if rows[1].Breached {
		t.Errorf("ok row should not be breached: %+v", rows[1])
	}
	// breach-in window filters out the not-imminent one
	imminent := computeSLA(findings, now, slaOpts{breachIn: 1 * time.Hour})
	for _, r := range imminent {
		if !r.Breached && r.BreachInHrs > 1 {
			t.Errorf("breach-in window leaked non-imminent row: %+v", r)
		}
	}
}

func TestComputeCoverage(t *testing.T) {
	basis := []ruleRec{
		{ID: "1", Name: "RDP Brute Force"},
		{ID: "2", Name: "Suspicious PowerShell"},
		{ID: "3", Name: "Impossible Travel"},
	}
	org := []ruleRec{
		{ID: "10", Name: "RDP Brute Force", Status: ruleStatusEnabled},
		{ID: "11", Name: "Suspicious PowerShell", Status: 5, StatusLabel: "disabled"},
		// Impossible Travel missing entirely
	}
	rows := computeCoverage(basis, org)
	if len(rows) != 2 {
		t.Fatalf("expected 2 gaps, got %d: %+v", len(rows), rows)
	}
	gaps := map[string]string{}
	for _, r := range rows {
		gaps[r.RuleName] = r.Gap
	}
	if gaps["Suspicious PowerShell"] != "disabled" {
		t.Errorf("PowerShell gap = %q, want disabled", gaps["Suspicious PowerShell"])
	}
	if gaps["Impossible Travel"] != "missing" {
		t.Errorf("Impossible Travel gap = %q, want missing", gaps["Impossible Travel"])
	}
	// fully-covered basis => no gaps
	full := computeCoverage(basis, []ruleRec{
		{Name: "RDP Brute Force", Status: 1}, {Name: "Suspicious PowerShell", Status: 1}, {Name: "Impossible Travel", Status: 1},
	})
	if len(full) != 0 {
		t.Errorf("expected 0 gaps for full coverage, got %d", len(full))
	}
}

func TestComputeExposure(t *testing.T) {
	now := mustTime(t, "2026-05-29T12:00:00Z")
	devices := []deviceRec{
		{DeviceID: "dc1", Hostname: "DC01", IsDC: true, Alive: "2026-05-27T12:00:00Z"},  // 48h stale DC
		{DeviceID: "ws1", Hostname: "WS01", IsDC: false, Alive: "2026-05-29T11:30:00Z"}, // healthy
		{DeviceID: "dc2", Hostname: "DC02", IsDC: true, Alive: "2026-05-29T11:30:00Z"},  // healthy DC
		{DeviceID: "dc3", Hostname: "DC03", IsDC: true, Alive: ""},                      // never checked in
	}
	// onlyDCs: dc1 (stale) and dc3 (never) qualify; dc2 healthy excluded
	rows := computeExposure(devices, now, exposureOpts{staleAfter: 24 * time.Hour, onlyDCs: true})
	if len(rows) != 2 {
		t.Fatalf("expected 2 exposed DCs, got %d: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if !r.IsDC {
			t.Errorf("onlyDCs leaked non-DC: %+v", r)
		}
	}
	// without onlyDCs, ws1 is still healthy so not included; only the 2 bad DCs
	all := computeExposure(devices, now, exposureOpts{staleAfter: 24 * time.Hour})
	if len(all) != 2 {
		t.Errorf("expected 2 exposed devices overall, got %d", len(all))
	}
}

func snap(key string, fs ...findingRec) historySnapshot {
	t, _ := parseBlumiraTime(key)
	return historySnapshot{Key: key, SyncedAt: t, Findings: fs}
}

func TestComputeDrift(t *testing.T) {
	prev := []findingRec{
		{ID: "a", Name: "alpha", Status: 10, StatusName: "Open"},
		{ID: "b", Name: "bravo", Status: 10, StatusName: "Open"},
	}
	cur := []findingRec{
		{ID: "a", Name: "alpha", Status: resolvedStatusCode, StatusName: "Resolved"}, // resolved
		{ID: "b", Name: "bravo", Status: 20, StatusName: "Analysis"},                 // status change
		{ID: "c", Name: "charlie", Status: 10, StatusName: "Open"},                   // new
	}
	rows := computeDrift(prev, cur)
	changes := map[string]string{}
	for _, r := range rows {
		changes[r.FindingID] = r.Change
	}
	if changes["a"] != "resolved" || changes["b"] != "status-change" || changes["c"] != "new" {
		t.Errorf("drift changes wrong: %+v", changes)
	}
	// no-change case
	same := computeDrift(prev, prev)
	if len(same) != 0 {
		t.Errorf("expected no drift for identical snapshots, got %d", len(same))
	}
}

func TestComputeVelocity(t *testing.T) {
	snaps := []historySnapshot{
		snap("2026-05-01T00:00:00Z",
			findingRec{ID: "x", Account: "acct1", Status: 10, StatusName: "Open"},
			findingRec{ID: "y", Account: "acct1", Status: 10, StatusName: "Open"}),
		snap("2026-05-03T00:00:00Z", // x resolved 48h after first seen
			findingRec{ID: "x", Account: "acct1", Status: resolvedStatusCode, StatusName: "Resolved"},
			findingRec{ID: "y", Account: "acct1", Status: 10, StatusName: "Open"}),
	}
	rows := computeVelocity(snaps, time.Time{}, true)
	if len(rows) != 1 || rows[0].Key != "acct1" {
		t.Fatalf("expected 1 acct1 row, got %+v", rows)
	}
	r := rows[0]
	if r.Resolved != 1 || r.Open != 1 {
		t.Errorf("velocity counts wrong: resolved=%d open=%d", r.Resolved, r.Open)
	}
	if r.MTTRHours != 48 {
		t.Errorf("MTTR=%v, want 48", r.MTTRHours)
	}
	if r.OpenRatePct != 50 {
		t.Errorf("open rate=%v, want 50", r.OpenRatePct)
	}
}

func TestComputeAudit(t *testing.T) {
	snaps := []historySnapshot{
		snap("2026-05-01T00:00:00Z", findingRec{ID: "z", Name: "flapper", Status: resolvedStatusCode, StatusName: "Resolved"}),
		snap("2026-05-02T00:00:00Z", findingRec{ID: "z", Name: "flapper", Status: 10, StatusName: "Open"}), // re-fire 1
		snap("2026-05-03T00:00:00Z", findingRec{ID: "z", Name: "flapper", Status: resolvedStatusCode, StatusName: "Resolved"}),
		snap("2026-05-04T00:00:00Z", findingRec{ID: "z", Name: "flapper", Status: 10, StatusName: "Open"}), // re-fire 2
		snap("2026-05-01T00:00:00Z", findingRec{ID: "stable", Name: "ok", Status: resolvedStatusCode, StatusName: "Resolved"}),
	}
	rows := computeAudit(snaps)
	if len(rows) != 1 || rows[0].FindingID != "z" {
		t.Fatalf("expected 1 re-fired finding z, got %+v", rows)
	}
	if rows[0].Reopens != 2 {
		t.Errorf("reopens=%d, want 2", rows[0].Reopens)
	}
}

func TestComputeRecurring(t *testing.T) {
	snaps := []historySnapshot{
		snap("2026-05-01T00:00:00Z",
			findingRec{ID: "1", Name: "RDP Brute Force", Account: "a"},
			findingRec{ID: "2", Name: "RDP Brute Force", Account: "b"}),
		snap("2026-05-02T00:00:00Z",
			findingRec{ID: "3", Name: "RDP Brute Force", Account: "a"},
			findingRec{ID: "4", Name: "One Off", Account: "a"}),
	}
	rows := computeRecurring(snaps, time.Time{}, 3)
	if len(rows) != 1 || rows[0].Name != "RDP Brute Force" {
		t.Fatalf("expected RDP Brute Force recurring, got %+v", rows)
	}
	if rows[0].Count != 3 {
		t.Errorf("count=%d, want 3 distinct findings", rows[0].Count)
	}
	if rows[0].Accounts != 2 {
		t.Errorf("accounts=%d, want 2", rows[0].Accounts)
	}
	// raise threshold beyond what exists
	none := computeRecurring(snaps, time.Time{}, 10)
	if len(none) != 0 {
		t.Errorf("expected no recurring at threshold 10, got %d", len(none))
	}
}

func TestMaskCredential(t *testing.T) {
	if got := maskCredential(""); got != "" {
		t.Errorf("mask empty = %q", got)
	}
	if got := maskCredential("ab"); got != "****" {
		t.Errorf("mask short = %q", got)
	}
	if got := maskCredential("abcdef123456"); got != "abcd…(redacted)" {
		t.Errorf("mask long = %q", got)
	}
}

func TestMintBlumiraToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("audience") != "public-api" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid_request"}`))
			return
		}
		if r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "sec" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"access_denied","error_description":"bad creds"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"jwt-token","token_type":"Bearer","expires_in":2592000}`))
	}))
	defer srv.Close()

	tok, err := mintBlumiraToken(t.Context(), srv.URL, "cid", "sec", "public-api", 5*time.Second)
	if err != nil {
		t.Fatalf("mint failed: %v", err)
	}
	if tok.AccessToken != "jwt-token" || tok.ExpiresIn != 2592000 {
		t.Errorf("unexpected token: %+v", tok)
	}
	// bad creds -> error surfaced from error_description
	if _, err := mintBlumiraToken(t.Context(), srv.URL, "cid", "wrong", "public-api", 5*time.Second); err == nil {
		t.Error("expected error for bad creds")
	}
}
