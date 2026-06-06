// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, ok := parseSuperopsTime(s)
	if !ok {
		t.Fatalf("parseSuperopsTime(%q) failed", s)
	}
	return ts
}

func TestParseSuperopsTime(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2026-05-01T10:15:30", true},
		{"2026-05-01T10:15:30Z", true},
		{"2026-05-01", true},
		{"", false},
		{"not-a-date", false},
	}
	for _, c := range cases {
		if _, ok := parseSuperopsTime(c.in); ok != c.ok {
			t.Errorf("parseSuperopsTime(%q) ok=%v, want %v", c.in, ok, c.ok)
		}
	}
}

func TestComputeSLAWatch(t *testing.T) {
	now := mustTime(t, "2026-05-29T12:00:00Z")
	tickets := []rec{
		// breached (due in the past), open
		{"ticketId": "1", "displayId": "T-1", "subject": "breached", "resolutionDueTime": "2026-05-29T08:00:00Z",
			"technician": map[string]any{"name": "Maya"}, "client": map[string]any{"name": "Acme"}},
		// at-risk (due within 4h)
		{"ticketId": "2", "displayId": "T-2", "subject": "soon", "resolutionDueTime": "2026-05-29T14:00:00Z",
			"technician": map[string]any{"name": "Raj"}, "client": map[string]any{"name": "Beta"}},
		// safe (due far out) -> excluded
		{"ticketId": "3", "displayId": "T-3", "subject": "fine", "resolutionDueTime": "2026-05-30T12:00:00Z",
			"technician": map[string]any{"name": "Maya"}},
		// resolved -> excluded even though overdue
		{"ticketId": "4", "displayId": "T-4", "subject": "done", "resolutionDueTime": "2026-05-01T00:00:00Z",
			"resolutionTime": "2026-05-02T00:00:00Z"},
	}
	rows := computeSLAWatch(tickets, "tech", 4*time.Hour, now)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].State != "breached" || rows[0].DisplayID != "T-1" {
		t.Errorf("most-overdue first; got %+v", rows[0])
	}
	if rows[1].State != "at-risk" {
		t.Errorf("second should be at-risk; got %+v", rows[1])
	}
	if rows[0].GroupBy != "Maya" {
		t.Errorf("group by tech name; got %q", rows[0].GroupBy)
	}
	// group by client
	byClient := computeSLAWatch(tickets, "client", 4*time.Hour, now)
	if byClient[0].GroupBy != "Acme" {
		t.Errorf("group by client; got %q", byClient[0].GroupBy)
	}
}

func TestComputeUnbilled(t *testing.T) {
	tickets := []rec{
		{"ticketId": "100", "client": map[string]any{"name": "Acme"}},
		{"ticketId": "200", "client": map[string]any{"name": "Beta"}},
	}
	worklogs := []rec{
		{"billable": true, "timespent": 60, "startTime": "2026-05-10T00:00:00Z", "ticket": map[string]any{"ticketId": "100"}},
		{"billable": true, "timespent": 30, "startTime": "2026-05-11T00:00:00Z", "ticket": map[string]any{"ticketId": "100"}},
		{"billable": false, "timespent": 90, "ticket": map[string]any{"ticketId": "100"}}, // non-billable excluded
		{"billable": true, "timespent": 45, "startTime": "2026-04-01T00:00:00Z", "ticket": map[string]any{"ticketId": "200"}},
	}
	rows := computeUnbilled(worklogs, tickets, "", time.Time{})
	if len(rows) != 2 {
		t.Fatalf("want 2 client rows, got %d: %+v", len(rows), rows)
	}
	// Acme = 90 min (2 entries), sorted first by minutes
	if rows[0].Client != "Acme" || rows[0].BillableMinutes != 90 || rows[0].BillableEntries != 2 {
		t.Errorf("Acme agg wrong: %+v", rows[0])
	}
	// --since filter drops the April Beta entry
	since := mustTime(t, "2026-05-01")
	filtered := computeUnbilled(worklogs, tickets, "", since)
	if len(filtered) != 1 || filtered[0].Client != "Acme" {
		t.Errorf("since filter should leave only Acme; got %+v", filtered)
	}
	// client filter
	only := computeUnbilled(worklogs, tickets, "Beta", time.Time{})
	if len(only) != 1 || only[0].Client != "Beta" {
		t.Errorf("client filter failed; got %+v", only)
	}
}

func TestPatchStatusRisky(t *testing.T) {
	risky := []string{"Missing", "patches pending", "CRITICAL", "Updates available", "non-compliant"}
	for _, s := range risky {
		if !patchStatusRisky(s) {
			t.Errorf("patchStatusRisky(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "Up to date", "compliant", "installed"} {
		if patchStatusRisky(s) {
			t.Errorf("patchStatusRisky(%q) = true, want false", s)
		}
	}
}

func TestComputeAtRiskAssets(t *testing.T) {
	assets := []rec{
		{"assetId": "a1", "name": "WS-1", "patchStatus": "missing critical", "client": map[string]any{"name": "Acme"}},
		{"assetId": "a2", "name": "WS-2", "patchStatus": "up to date", "client": map[string]any{"name": "Acme"}}, // not risky
		{"assetId": "a3", "name": "WS-3", "patchStatus": "missing", "client": map[string]any{"name": "Beta"}},    // risky but no alert
	}
	alerts := []rec{
		{"asset": map[string]any{"assetId": "a1"}, "status": "open"},
		{"asset": map[string]any{"assetId": "a1"}, "status": "open"},
		{"asset": map[string]any{"assetId": "a2"}, "status": "open"},     // a2 not risky
		{"asset": map[string]any{"assetId": "a3"}, "status": "resolved"}, // resolved -> not counted
		{"asset": map[string]any{"assetId": "a3"}, "resolvedTime": "2026-05-01T00:00:00Z"},
	}
	rows := computeAtRiskAssets(assets, alerts, "")
	if len(rows) != 1 {
		t.Fatalf("want 1 at-risk asset (a1), got %d: %+v", len(rows), rows)
	}
	if rows[0].AssetID != "a1" || rows[0].OpenAlerts != 2 {
		t.Errorf("a1 should have 2 open alerts; got %+v", rows[0])
	}
}

func TestComputeAlertCoverage(t *testing.T) {
	assets := []rec{{"assetId": "a1", "client": map[string]any{"name": "Acme"}}}
	alerts := []rec{
		{"asset": map[string]any{"assetId": "a1"}, "status": "open"},
		{"asset": map[string]any{"assetId": "a1"}, "status": "resolved"},
		{"asset": map[string]any{"assetId": "a1"}, "resolvedTime": "2026-05-01T00:00:00Z"},
	}
	rows := computeAlertCoverage(alerts, assets, "")
	if len(rows) != 1 {
		t.Fatalf("want 1 client row, got %d", len(rows))
	}
	if rows[0].Client != "Acme" || rows[0].Open != 1 || rows[0].Resolved != 2 || rows[0].Total != 3 {
		t.Errorf("coverage agg wrong: %+v", rows[0])
	}
}

func TestComputeStaleTickets(t *testing.T) {
	now := mustTime(t, "2026-05-29T12:00:00Z")
	tickets := []rec{
		// idle 20 days, open
		{"ticketId": "1", "displayId": "T-1", "subject": "old", "updatedTime": "2026-05-09T12:00:00Z"},
		// recently updated -> not stale
		{"ticketId": "2", "displayId": "T-2", "updatedTime": "2026-05-28T12:00:00Z"},
		// resolved -> excluded
		{"ticketId": "3", "displayId": "T-3", "updatedTime": "2026-01-01T00:00:00Z", "resolutionTime": "2026-01-02T00:00:00Z"},
		// stale by updatedTime BUT a recent worklog refreshes it -> not stale
		{"ticketId": "4", "displayId": "T-4", "updatedTime": "2026-04-01T00:00:00Z"},
	}
	worklogs := []rec{
		{"ticket": map[string]any{"ticketId": "4"}, "startTime": "2026-05-27T00:00:00Z"},
	}
	rows := computeStaleTickets(tickets, worklogs, 7, now)
	if len(rows) != 1 || rows[0].DisplayID != "T-1" {
		t.Fatalf("want only T-1 stale, got %+v", rows)
	}
	if rows[0].IdleDays < 19 || rows[0].IdleDays > 21 {
		t.Errorf("idle days ~20 expected; got %d", rows[0].IdleDays)
	}
}

func TestBuildClient360(t *testing.T) {
	clients := []rec{{"accountId": "c1", "name": "Acme"}}
	sites := []rec{{"id": "s1", "client": map[string]any{"accountId": "c1", "name": "Acme"}}}
	tickets := []rec{
		{"ticketId": "t1", "client": map[string]any{"accountId": "c1"}},                                           // open
		{"ticketId": "t2", "client": map[string]any{"accountId": "c1"}, "resolutionTime": "2026-05-01T00:00:00Z"}, // closed -> excluded
		{"ticketId": "t3", "client": map[string]any{"accountId": "other"}},                                        // other client
	}
	invoices := []rec{
		{"invoiceId": "i1", "client": map[string]any{"accountId": "c1"}},                              // open
		{"invoiceId": "i2", "client": map[string]any{"accountId": "c1"}, "paymentDate": "2026-05-01"}, // paid -> excluded
	}
	bundle, ok := buildClient360("Acme", clients, sites, nil, nil, tickets, nil, invoices)
	if !ok {
		t.Fatal("client not found")
	}
	if len(bundle.Sites) != 1 {
		t.Errorf("want 1 site, got %d", len(bundle.Sites))
	}
	if len(bundle.OpenTickets) != 1 {
		t.Errorf("want 1 open ticket, got %d", len(bundle.OpenTickets))
	}
	if len(bundle.OpenInvoices) != 1 {
		t.Errorf("want 1 open invoice, got %d", len(bundle.OpenInvoices))
	}
	if _, ok := buildClient360("Nope", clients, sites, nil, nil, tickets, nil, invoices); ok {
		t.Error("unknown client should not match")
	}
}

func TestBuildTicketContext(t *testing.T) {
	tickets := []rec{
		{"ticketId": "t1", "displayId": "T-1", "subject": "hi",
			"client": map[string]any{"name": "Acme"}, "sla": map[string]any{"name": "Gold"}},
	}
	worklogs := []rec{
		{"worklogEntryId": "w1", "ticket": map[string]any{"ticketId": "t1"}},
		{"worklogEntryId": "w2", "ticket": map[string]any{"ticketId": "other"}},
	}
	ctx, ok := buildTicketContext("t1", tickets, worklogs)
	if !ok {
		t.Fatal("ticket not found")
	}
	if recStr(ctx.Client, "name") != "Acme" || recStr(ctx.SLA, "name") != "Gold" {
		t.Errorf("embedded client/sla wrong: %+v / %+v", ctx.Client, ctx.SLA)
	}
	if len(ctx.Worklogs) != 1 {
		t.Errorf("want 1 worklog for t1, got %d", len(ctx.Worklogs))
	}
	if _, ok := buildTicketContext("missing", tickets, worklogs); ok {
		t.Error("unknown ticket should not match")
	}
}
