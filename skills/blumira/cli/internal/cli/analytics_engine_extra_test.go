// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Tests for the 2026-06 reprint engine extension: overview, reconcile,
// dc-roster, workload, evidence helpers, and the tolerant owner decoder.

package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var extraNow = time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

func ts(t time.Time) string { return t.Format(time.RFC3339) }

func TestDecodeOwners(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want []string
	}{
		{"missing", map[string]any{}, nil},
		{"string array", map[string]any{"owners": []any{"a@x.com", "b@x.com"}}, []string{"a@x.com", "b@x.com"}},
		{"object array email", map[string]any{"owners": []any{map[string]any{"email": "a@x.com"}}}, []string{"a@x.com"}},
		{"object array name fallback", map[string]any{"owners": []any{map[string]any{"name": "Maria"}}}, []string{"Maria"}},
		{"single string", map[string]any{"owner": "solo@x.com"}, []string{"solo@x.com"}},
		{"single object", map[string]any{"owner": map[string]any{"email": "one@x.com"}}, []string{"one@x.com"}},
		{"blank entries dropped", map[string]any{"owners": []any{"", "  ", "real@x.com"}}, []string{"real@x.com"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decodeOwners(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("decodeOwners(%v) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("decodeOwners(%v)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestComputeOverview(t *testing.T) {
	findings := []findingRec{
		{ID: "f1", Account: "acct-a", AccountName: "Alpha", Priority: 1, Status: 10, Created: ts(extraNow.Add(-48 * time.Hour))},
		{ID: "f2", Account: "acct-a", Priority: 2, Status: 10, Created: ts(extraNow.Add(-2 * time.Hour))},
		{ID: "f3", Account: "acct-a", Priority: 3, Status: 40, StatusName: "Resolved"},
		{ID: "f4", Account: "acct-b", AccountName: "Bravo", Priority: 3, Status: 10, Created: ts(extraNow.Add(-1 * time.Hour))},
	}
	devices := []deviceRec{
		{DeviceID: "d1", Account: "acct-a", IsDC: true, Alive: ts(extraNow.Add(-1 * time.Hour))},
		{DeviceID: "d2", Account: "acct-a", Alive: ts(extraNow.Add(-72 * time.Hour))},
	}
	rows := computeOverview(findings, devices, extraNow)
	if len(rows) != 2 {
		t.Fatalf("expected 2 account rows, got %d: %+v", len(rows), rows)
	}
	a := rows[0]
	if a.Account != "acct-a" {
		t.Fatalf("hottest account should sort first (has the only P1), got %q", a.Account)
	}
	if a.OpenFindings != 2 || a.P1Open != 1 || a.P2Open != 1 || a.Resolved != 1 {
		t.Fatalf("acct-a counts wrong: %+v", a)
	}
	if a.OldestOpenHrs != 48 {
		t.Fatalf("acct-a oldest open should be 48h, got %d", a.OldestOpenHrs)
	}
	if a.Devices != 2 || a.DomainCtrls != 1 || a.StaleDevices != 1 {
		t.Fatalf("acct-a device tallies wrong: %+v", a)
	}
	if rows[1].Account != "acct-b" || rows[1].OpenFindings != 1 {
		t.Fatalf("acct-b row wrong: %+v", rows[1])
	}
}

func TestComputeOverviewEmpty(t *testing.T) {
	rows := computeOverview(nil, nil, extraNow)
	if len(rows) != 0 {
		t.Fatalf("empty inputs must produce zero rows, got %+v", rows)
	}
}

func TestComputeReconcile(t *testing.T) {
	findings := []findingRec{
		{ID: "f2", Account: "acct-b", Name: "Later", Status: 10, Created: "2026-06-02T00:00:00Z", Owners: []string{"a@x.com", "b@x.com"}},
		{ID: "f1", Account: "acct-a", Name: "Open one", Status: 10, Created: "2026-06-01T00:00:00Z"},
		{ID: "f3", Account: "acct-a", Name: "Closed", Status: 40, StatusName: "Resolved"},
	}
	openOnly, err := parseStatusFilter("open")
	if err != nil {
		t.Fatal(err)
	}
	rows := computeReconcile(findings, openOnly)
	if len(rows) != 2 {
		t.Fatalf("open filter should keep 2 rows, got %d", len(rows))
	}
	if rows[0].Account != "acct-a" || rows[1].Account != "acct-b" {
		t.Fatalf("rows must sort by account: %+v", rows)
	}
	if rows[1].Assignees != "a@x.com,b@x.com" {
		t.Fatalf("assignees must comma-join: %q", rows[1].Assignees)
	}
	all, _ := parseStatusFilter("all")
	if got := computeReconcile(findings, all); len(got) != 3 {
		t.Fatalf("all filter should keep 3 rows, got %d", len(got))
	}
}

func TestComputeDCRoster(t *testing.T) {
	devices := []deviceRec{
		{DeviceID: "d1", Account: "a", Hostname: "dc01", IsDC: true, Alive: ts(extraNow.Add(-2 * time.Hour))},
		{DeviceID: "d2", Account: "a", Hostname: "dc02", IsDC: true, Alive: ts(extraNow.Add(-50 * time.Hour))},
		{DeviceID: "d3", Account: "a", Hostname: "dc03", IsDC: true}, // never checked in
		{DeviceID: "d4", Account: "a", Hostname: "ws01", IsDC: false, Alive: ts(extraNow)},
	}
	rows := computeDCRoster(devices, extraNow, 24*time.Hour)
	if len(rows) != 3 {
		t.Fatalf("non-DC must be excluded; got %d rows", len(rows))
	}
	states := map[string]string{}
	for _, r := range rows {
		states[r.Hostname] = r.State
	}
	if states["dc01"] != "protected" || states["dc02"] != "stale" || states["dc03"] != "never-checked-in" {
		t.Fatalf("states wrong: %+v", states)
	}
	for _, r := range rows {
		if r.Hostname == "dc03" && r.LastSeenHrs != -1 {
			t.Fatalf("never-checked-in must report -1 last seen, got %d", r.LastSeenHrs)
		}
	}
}

func TestComputeWorkload(t *testing.T) {
	findings := []findingRec{
		{ID: "f1", Account: "a", Priority: 1, Status: 10, Created: ts(extraNow.Add(-2 * time.Hour)), Owners: []string{"maria@x.com"}},
		{ID: "f2", Account: "b", Priority: 3, Status: 10, Created: ts(extraNow.Add(-30 * time.Hour)), Owners: []string{"maria@x.com"}},
		{ID: "f3", Account: "a", Priority: 3, Status: 10, Created: ts(extraNow.Add(-100 * time.Hour))},
		{ID: "f4", Account: "a", Priority: 2, Status: 40, StatusName: "Resolved", Owners: []string{"maria@x.com"}},
	}
	rows := computeWorkload(findings, extraNow)
	if len(rows) != 2 {
		t.Fatalf("expected maria + (unassigned), got %d rows: %+v", len(rows), rows)
	}
	if rows[0].Assignee != "maria@x.com" {
		t.Fatalf("maria has the most open and should lead: %+v", rows[0])
	}
	m := rows[0]
	if m.OpenTotal != 2 || m.P1Open != 1 || m.Under24h != 1 || m.Days1to3 != 1 || m.Over3Days != 0 {
		t.Fatalf("maria buckets wrong: %+v", m)
	}
	if m.Accounts != 2 {
		t.Fatalf("maria touches 2 accounts, got %d", m.Accounts)
	}
	u := rows[1]
	if u.Assignee != "(unassigned)" || !u.Unassigned || u.OpenTotal != 1 || u.Over3Days != 1 {
		t.Fatalf("unassigned bucket wrong: %+v", u)
	}
}

func TestComputeWorkloadResolvedExcluded(t *testing.T) {
	findings := []findingRec{
		{ID: "f1", Status: 40, StatusName: "Resolved", Owners: []string{"x@x.com"}},
	}
	if rows := computeWorkload(findings, extraNow); len(rows) != 0 {
		t.Fatalf("resolved-only input must produce zero rows, got %+v", rows)
	}
}

func TestEvidenceSnippet(t *testing.T) {
	long := "prefix " + string(make([]byte, 0)) + "The quick brown fox connected to 10.0.4.21 over RDP repeatedly" // readable content
	got := evidenceSnippet(long, "10.0.4.21")
	if got == "" || len(got) > len(long)+2 {
		t.Fatalf("snippet shape wrong: %q", got)
	}
	if want := "10.0.4.21"; !strings.Contains(got, want) {
		t.Fatalf("snippet must contain the term: %q", got)
	}
	// no-match content returns a bounded prefix
	if s := evidenceSnippet("short content", "absent"); s != "short content" {
		t.Fatalf("no-match short content should pass through, got %q", s)
	}
}

func TestExtractEvidenceItems(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"array", `[{"a":1},{"b":2}]`, 2},
		{"object data wrap", `{"data":[{"a":1},{"b":2},{"c":3}]}`, 3},
		{"object evidence wrap", `{"evidence":[{"a":1}]}`, 1},
		{"opaque object", `{"weird":"shape"}`, 1},
		{"null", `null`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractEvidenceItems(json.RawMessage(c.in))
			if len(got) != c.want {
				t.Fatalf("extractEvidenceItems(%s) = %d items, want %d", c.in, len(got), c.want)
			}
		})
	}
}

func TestMatchFindingsRaw(t *testing.T) {
	findings := []findingRec{
		{ID: "f1", Name: "RDP brute force", Account: "a"},
		{ID: "f2", Name: "Clean finding", Account: "a"},
	}
	raws := map[string]string{
		"f1": `{"finding_id":"f1","analysis":"repeated RDP attempts from 10.0.4.21"}`,
		"f2": `{"finding_id":"f2","analysis":"nothing of note"}`,
	}
	got := matchFindingsRaw(findings, raws, "10.0.4.21", 10)
	if len(got) != 1 || got[0].FindingID != "f1" || got[0].Source != "finding" {
		t.Fatalf("expected one f1 match, got %+v", got)
	}
	// negative: a mismatching term returns nothing
	if none := matchFindingsRaw(findings, raws, "no-such-indicator", 10); len(none) != 0 {
		t.Fatalf("mismatching term must return no rows, got %+v", none)
	}
}
