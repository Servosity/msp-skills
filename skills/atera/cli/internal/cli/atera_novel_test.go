// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"atera-pp-cli/internal/store"
)

// fixedNow anchors all time-window math so stale/SLA/since results are
// deterministic regardless of when the test runs.
var fixedNow = time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

func withFixedNow(t *testing.T) {
	t.Helper()
	prev := nvNow
	nvNow = func() time.Time { return fixedNow }
	t.Cleanup(func() { nvNow = prev })
}

// --- pure-helper unit tests -------------------------------------------------

func TestNvParseWindow(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"24h", 24 * time.Hour, true},
		{"90m", 90 * time.Minute, true},
		{"7d", 7 * 24 * time.Hour, true},
		{"2d12h", 2*24*time.Hour + 12*time.Hour, true},
		{"", 0, false},
		{"banana", 0, false},
		{"0h", 0, false},
	}
	for _, c := range cases {
		got, ok := nvParseWindow(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("nvParseWindow(%q) = %v,%v; want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestIsEOL(t *testing.T) {
	eol := []string{"Windows 7 Professional", "Windows Server 2012 R2", "macOS 10.15 Catalina"}
	notEOL := []string{"Windows 11 Pro", "Windows Server 2022", "macOS 14 Sonoma", ""}
	for _, s := range eol {
		if !isEOL(s) {
			t.Errorf("isEOL(%q) = false; want true", s)
		}
	}
	for _, s := range notEOL {
		if isEOL(s) {
			t.Errorf("isEOL(%q) = true; want false", s)
		}
	}
}

func TestSeverityRankAndOpenTicket(t *testing.T) {
	if severityRank("Critical") <= severityRank("Warning") || severityRank("Warning") <= severityRank("Information") {
		t.Error("severityRank ordering wrong")
	}
	if nvIsOpenTicket("Closed") || nvIsOpenTicket("Resolved") {
		t.Error("Closed/Resolved should not be open")
	}
	if !nvIsOpenTicket("Open") || !nvIsOpenTicket("Pending") {
		t.Error("Open/Pending should be open")
	}
}

func TestNvIntToleratesStringEncodedNumber(t *testing.T) {
	o := nvObj{
		"A": json.RawMessage(`42`),
		"B": json.RawMessage(`"99"`), // Atera occasionally quotes numbers
	}
	if v, ok := nvInt(o, "A"); !ok || v != 42 {
		t.Errorf("nvInt A = %d,%v", v, ok)
	}
	if v, ok := nvInt(o, "B"); !ok || v != 99 {
		t.Errorf("nvInt B (string-encoded) = %d,%v", v, ok)
	}
}

func TestIsRecurringContractType(t *testing.T) {
	recurring := []string{"RemoteMonitoring", "Remote Monitoring", "Retainer Flat Fee", "RetainerFlatFee"}
	oneTime := []string{"Hourly", "Block Hours", "Block Money", "Project One Time Fee", "OnlineBackup", ""}
	for _, s := range recurring {
		if !isRecurringContractType(s) {
			t.Errorf("isRecurringContractType(%q) = false; want true", s)
		}
	}
	for _, s := range oneTime {
		if isRecurringContractType(s) {
			t.Errorf("isRecurringContractType(%q) = true; want false", s)
		}
	}
}

func TestPatchRollupExcludesFailuresFromTotals(t *testing.T) {
	results := []patchFetchResult{
		{guid: "g-1", machine: "WS-A", customer: "Acme", missing: 5, critical: 2},
		{guid: "g-2", machine: "WS-B", customer: "Acme", missing: 0},
		{guid: "g-3", machine: "WS-C", customer: "Beta", err: errors.New("boom")},
		{guid: "g-4", machine: "WS-D", customer: "Beta", missing: 3, critical: 1},
	}
	rep := patchRollup(results, 10, 50)
	if rep.ScannedAgents != 4 {
		t.Errorf("ScannedAgents = %d; want 4", rep.ScannedAgents)
	}
	if rep.TotalMissing != 8 {
		t.Errorf("TotalMissing = %d; want 8 (failed fetch must not count)", rep.TotalMissing)
	}
	if len(rep.FetchFailures) != 1 || rep.FetchFailures[0].DeviceGuid != "g-3" {
		t.Errorf("FetchFailures wrong: %+v", rep.FetchFailures)
	}
	if rep.ByCustomer["Beta"] != 3 {
		t.Errorf("ByCustomer[Beta] = %d; want 3 (failure excluded, not zeroed)", rep.ByCustomer["Beta"])
	}
	// Ranked list: WS-A (5) first, WS-D (3) second; fully-patched WS-B omitted.
	if len(rep.Agents) != 2 || rep.Agents[0].MachineName != "WS-A" || rep.Agents[1].MachineName != "WS-D" {
		t.Errorf("ranked agents wrong: %+v", rep.Agents)
	}
}

func TestPatchRollupLimit(t *testing.T) {
	results := []patchFetchResult{
		{guid: "g-1", machine: "A", missing: 3},
		{guid: "g-2", machine: "B", missing: 2},
		{guid: "g-3", machine: "C", missing: 1},
	}
	rep := patchRollup(results, 2, 50)
	if len(rep.Agents) != 2 || rep.Agents[0].MachineName != "A" {
		t.Errorf("limit not applied or order wrong: %+v", rep.Agents)
	}
	if rep.TotalMissing != 6 {
		t.Errorf("TotalMissing should count beyond the display limit, got %d", rep.TotalMissing)
	}
}

// --- seeded-store behavioral tests ------------------------------------------

func seedStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	put := func(rt, id, jsonStr string) {
		if err := s.Upsert(rt, id, json.RawMessage(jsonStr)); err != nil {
			t.Fatalf("upsert %s/%s: %v", rt, id, err)
		}
	}

	// Agents: one EOL+stale, one fresh (created in window), one mid.
	put("agents", "1", `{"AgentID":1,"AgentName":"WS-OLD","MachineName":"WS-OLD","DeviceGuid":"g-1","CustomerID":100,"CustomerName":"Acme","OS":"Windows 7 Professional","Online":false,"LastSeen":"2026-01-01T08:00:00","Created":"2025-06-01T08:00:00"}`)
	put("agents", "2", `{"AgentID":2,"AgentName":"WS-NEW","MachineName":"WS-NEW","DeviceGuid":"g-2","CustomerID":100,"CustomerName":"Acme","OS":"Windows 11 Pro","Online":true,"LastSeen":"2026-05-30T09:00:00","Created":"2026-05-30T06:00:00"}`)
	put("agents", "3", `{"AgentID":3,"AgentName":"WS-BETA","MachineName":"WS-BETA","DeviceGuid":"g-3","CustomerID":200,"CustomerName":"Beta","OS":"Windows 10 Pro","Online":true,"LastSeen":"2026-05-29T12:00:00","Created":"2025-09-01T08:00:00"}`)

	// Tickets: breached-open, future-open, closed.
	put("tickets", "10", `{"TicketID":10,"TicketTitle":"Server down","CustomerName":"Acme","TicketStatus":"Open","TicketPriority":"Critical","TechnicianFullName":"Pat Tech","FirstResponseDueDate":"2026-05-30T10:00:00","ClosedTicketDueDate":"2026-05-31T12:00:00","TotalDurationMinutes":45,"TicketCreatedDate":"2026-05-30T06:00:00"}`)
	put("tickets", "11", `{"TicketID":11,"TicketTitle":"Slow PC","CustomerName":"Beta","TicketStatus":"Open","TicketPriority":"Low","TechnicianFullName":"Pat Tech","FirstResponseDueDate":"2026-05-30T18:00:00","TotalDurationMinutes":30,"TicketCreatedDate":"2026-05-20T06:00:00"}`)
	put("tickets", "12", `{"TicketID":12,"TicketTitle":"Done","CustomerName":"Acme","TicketStatus":"Closed","TechnicianFullName":"Sam Tech","FirstResponseDueDate":"2026-05-30T09:00:00","TotalDurationMinutes":100,"TicketCreatedDate":"2026-05-01T06:00:00"}`)

	// Customers + contracts. Acme has active RemoteMonitoring expiring inside
	// 60d (June 15) and an inactive backup contract already lapsed; Beta has an
	// active Hourly contract far in the future (active but NOT recurring
	// coverage, and outside the 60d expiry window).
	put("customers", "100", `{"CustomerID":100,"CustomerName":"Acme"}`)
	put("customers", "200", `{"CustomerID":200,"CustomerName":"Beta"}`)
	put("contracts", "1", `{"ContractID":1,"CustomerID":100,"CustomerName":"Acme","ContractName":"Acme MSP","ContractType":"RemoteMonitoring","Active":true,"EndDate":"2026-06-15T00:00:00"}`)
	put("contracts", "2", `{"ContractID":2,"CustomerID":100,"CustomerName":"Acme","ContractName":"Acme Backup","ContractType":"OnlineBackup","Active":false,"EndDate":"2026-03-01T00:00:00"}`)
	put("contracts", "3", `{"ContractID":3,"CustomerID":200,"CustomerName":"Beta","ContractName":"Beta Hours","ContractType":"Hourly","Active":true,"EndDate":"2027-01-01T00:00:00"}`)

	// Alerts: critical-open on WS-OLD, warning-open on WS-BETA (in 24h window),
	// second warning on WS-OLD (in 7d window but outside 24h), archived one.
	put("alerts", "1", `{"AlertID":1,"AgentId":1,"DeviceGuid":"g-1","Severity":"Critical","Title":"Disk full","CustomerName":"Acme","DeviceName":"WS-OLD","Created":"2026-05-28T12:00:00","Archived":false}`)
	put("alerts", "2", `{"AlertID":2,"AgentId":3,"DeviceGuid":"g-3","Severity":"Warning","Title":"High CPU","CustomerName":"Beta","DeviceName":"WS-BETA","Created":"2026-05-30T11:00:00","Archived":false}`)
	put("alerts", "3", `{"AlertID":3,"Severity":"Critical","Title":"Old archived","CustomerName":"Acme","Created":"2026-04-01T12:00:00","Archived":true}`)
	put("alerts", "4", `{"AlertID":4,"AgentId":1,"DeviceGuid":"g-1","Severity":"Warning","Title":"Disk warning","CustomerName":"Acme","DeviceName":"WS-OLD","Created":"2026-05-29T10:00:00","Archived":false}`)

	return dbPath
}

func runNovel(t *testing.T, cmdFn func(*rootFlags) *cobra.Command, dbPath string, extraArgs ...string) []byte {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := cmdFn(flags)
	// Keep stderr separate: the sync-hint helpers write advisory lines to
	// stderr, and mixing them into stdout would corrupt the JSON under test.
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(append([]string{"--db", dbPath}, extraArgs...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errBuf.String())
	}
	return out.Bytes()
}

func TestAgentsStaleSeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got []staleAgent
	if err := json.Unmarshal(runNovel(t, newNovelAgentsStaleCmd, db, "--days", "30"), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].MachineName != "WS-OLD" {
		t.Fatalf("want only WS-OLD stale, got %+v", got)
	}
	if got[0].DaysSinceSeen < 100 {
		t.Errorf("WS-OLD should be ~149d dark, got %d", got[0].DaysSinceSeen)
	}
}

func TestAgentsInventoryEOLSeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got struct {
		EOLCount  int        `json:"EOLCount"`
		EOLAgents []eolAgent `json:"EOLAgents"`
	}
	if err := json.Unmarshal(runNovel(t, newNovelAgentsInventoryCmd, db, "--eol"), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.EOLCount != 1 || got.EOLAgents[0].MachineName != "WS-OLD" {
		t.Fatalf("want WS-OLD as the one EOL machine, got %+v", got)
	}
}

func TestTicketsSlaSeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got []slaTicket
	if err := json.Unmarshal(runNovel(t, newNovelTicketsSlaCmd, db), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 open SLA tickets (closed excluded), got %d: %+v", len(got), got)
	}
	if got[0].TicketID != 10 || !got[0].Breached {
		t.Errorf("most-overdue ticket 10 should rank first and be breached, got %+v", got[0])
	}
	for _, tk := range got {
		if tk.TicketID == 12 {
			t.Error("closed ticket 12 must be excluded")
		}
	}
}

func TestTicketsWorkloadSeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got []techLoad
	if err := json.Unmarshal(runNovel(t, newNovelTicketsWorkloadCmd, db), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Technician != "Pat Tech" {
		t.Fatalf("want only Pat Tech with open tickets, got %+v", got)
	}
	if got[0].OpenTickets != 2 || got[0].TotalDurationMinutes != 75 {
		t.Errorf("Pat Tech should have 2 open / 75 min, got %+v", got[0])
	}
}

func TestCustomersBookSeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got []bookEntry
	if err := json.Unmarshal(runNovel(t, newNovelCustomersBookCmd, db), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0].CustomerName != "Acme" {
		t.Fatalf("want Acme first (most agents), got %+v", got)
	}
	if got[0].AgentCount != 2 || got[0].ContractCount != 2 || got[0].ActiveContractCount != 1 {
		t.Errorf("Acme join wrong: %+v", got[0])
	}
}

func TestAlertsTriageSeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got []triageAlert
	if err := json.Unmarshal(runNovel(t, newNovelAlertsTriageCmd, db), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 open alerts (archived excluded), got %d: %+v", len(got), got)
	}
	if got[0].AlertID != 1 || got[0].Severity != "Critical" {
		t.Errorf("critical alert should rank first, got %+v", got[0])
	}
	for _, a := range got {
		if a.AlertID == 3 {
			t.Error("archived alert 3 must be excluded")
		}
	}
}

func TestSinceSeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got sinceReport
	if err := json.Unmarshal(runNovel(t, newNovelSinceCmd, db, "24h"), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Within 24h of fixedNow: agent 2 (06:00), ticket 10 (06:00), alert 2 (11:00).
	// Alert 4 (05-29T10:00) is 26h old — outside the 24h window.
	if got.NewAgents != 1 || got.NewTickets != 1 || got.NewAlerts != 1 {
		t.Fatalf("since 24h counts wrong: %+v", got)
	}
	if len(got.Items) != 3 {
		t.Errorf("want 3 items in window, got %d", len(got.Items))
	}
}

func TestCustomersCoverageSeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got []coverageEntry
	if err := json.Unmarshal(runNovel(t, newNovelCustomersCoverageCmd, db), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Acme has active RemoteMonitoring → covered. Beta has 1 agent and only an
	// active Hourly contract → uncovered gap row.
	if len(got) != 1 || got[0].CustomerName != "Beta" {
		t.Fatalf("want only Beta uncovered, got %+v", got)
	}
	if got[0].Covered || got[0].AgentCount != 1 || got[0].ActiveContracts != 1 || got[0].RecurringContracts != 0 {
		t.Errorf("Beta coverage fields wrong: %+v", got[0])
	}

	// --any-active flips Beta to covered → empty gap view.
	var anyActive []coverageEntry
	if err := json.Unmarshal(runNovel(t, newNovelCustomersCoverageCmd, db, "--any-active"), &anyActive); err != nil {
		t.Fatalf("unmarshal --any-active: %v", err)
	}
	if len(anyActive) != 0 {
		t.Errorf("--any-active should leave no uncovered customers, got %+v", anyActive)
	}

	// --all shows both customers, uncovered first.
	var all []coverageEntry
	if err := json.Unmarshal(runNovel(t, newNovelCustomersCoverageCmd, db, "--all"), &all); err != nil {
		t.Fatalf("unmarshal --all: %v", err)
	}
	if len(all) != 2 || all[0].CustomerName != "Beta" || !all[1].Covered {
		t.Errorf("--all ordering/coverage wrong: %+v", all)
	}
}

func TestContractsExpiringSeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got []expiringContract
	if err := json.Unmarshal(runNovel(t, newNovelContractsExpiringCmd, db, "--days", "60"), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Only contract 1 (Acme MSP, ends 2026-06-15, 16 days out) is active and
	// inside 60d. Contract 3 ends in 2027 (outside); contract 2 is inactive.
	if len(got) != 1 || got[0].ContractID != 1 {
		t.Fatalf("want only Acme MSP expiring, got %+v", got)
	}
	if got[0].DaysToExpiry < 15 || got[0].DaysToExpiry > 17 {
		t.Errorf("Acme MSP should be ~16 days out, got %d", got[0].DaysToExpiry)
	}

	// --include-inactive --include-expired pulls in the lapsed backup contract.
	var withExpired []expiringContract
	if err := json.Unmarshal(runNovel(t, newNovelContractsExpiringCmd, db, "--days", "60", "--include-inactive", "--include-expired"), &withExpired); err != nil {
		t.Fatalf("unmarshal expired: %v", err)
	}
	if len(withExpired) != 2 || withExpired[0].ContractID != 2 || withExpired[0].DaysToExpiry >= 0 {
		t.Errorf("expired contract should rank first with negative days, got %+v", withExpired)
	}
}

func TestAgentsNoisySeeded(t *testing.T) {
	withFixedNow(t)
	db := seedStore(t)
	var got []noisyDevice
	if err := json.Unmarshal(runNovel(t, newNovelAgentsNoisyCmd, db, "--days", "7"), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// In the 7d window: alerts 1+4 on g-1 (WS-OLD), alert 2 on g-3 (WS-BETA).
	// Archived alert 3 excluded.
	if len(got) != 2 {
		t.Fatalf("want 2 noisy devices, got %d: %+v", len(got), got)
	}
	if got[0].MachineName != "WS-OLD" || got[0].AlertCount != 2 || got[0].CriticalCount != 1 {
		t.Errorf("WS-OLD should lead with 2 alerts (1 critical), got %+v", got[0])
	}
	if got[1].MachineName != "WS-BETA" || got[1].AlertCount != 1 {
		t.Errorf("WS-BETA should follow with 1 alert, got %+v", got[1])
	}
}

func TestSinceAndNoisyMixedTimestampFormatsOrdering(t *testing.T) {
	// Regression: since/noisy must order by PARSED time, not raw strings.
	// Mixed formats (/Date(ms)/, ISO+Z, fractional-no-tz) sort wrong
	// lexicographically.
	withFixedNow(t)
	dbPath := filepath.Join(t.TempDir(), "mixed.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	put := func(rt, id, jsonStr string) {
		if err := s.Upsert(rt, id, json.RawMessage(jsonStr)); err != nil {
			t.Fatalf("upsert %s/%s: %v", rt, id, err)
		}
	}
	// 1780130400000 ms = 2026-05-30T08:40:00Z. All three within 24h of fixedNow.
	put("alerts", "1", `{"AlertID":1,"DeviceGuid":"g-x","Severity":"Warning","Title":"OData ts","CustomerName":"Acme","DeviceName":"WS-X","Created":"/Date(1780130400000)/","Archived":false}`)
	put("alerts", "2", `{"AlertID":2,"DeviceGuid":"g-x","Severity":"Warning","Title":"ISO Z ts","CustomerName":"Acme","DeviceName":"WS-X","Created":"2026-05-30T11:00:00Z","Archived":false}`)
	put("alerts", "3", `{"AlertID":3,"DeviceGuid":"g-y","Severity":"Warning","Title":"fractional ts","CustomerName":"Beta","DeviceName":"WS-Y","Created":"2026-05-30T09:30:00.500","Archived":false}`)
	s.Close()

	// since: newest first must be 11:00Z (ISO), 09:30.500 (fractional),
	// 08:40Z (/Date/). Lexicographic raw-string sort would instead put the
	// /Date(...)/ item out of place ('/' sorts before '2').
	var rep sinceReport
	if err := json.Unmarshal(runNovel(t, newNovelSinceCmd, dbPath, "24h"), &rep); err != nil {
		t.Fatalf("unmarshal since: %v", err)
	}
	if len(rep.Items) != 3 {
		t.Fatalf("want 3 items, got %d: %+v", len(rep.Items), rep.Items)
	}
	wantOrder := []int64{2, 3, 1}
	for i, want := range wantOrder {
		if rep.Items[i].ID != want {
			t.Fatalf("since order wrong at %d: want ID %d, got %+v", i, want, rep.Items)
		}
	}

	// noisy: g-x has 2 alerts; LastAlert must be the chronologically-latest
	// (the ISO 11:00Z one), not the lexicographic max.
	var noisy []noisyDevice
	if err := json.Unmarshal(runNovel(t, newNovelAgentsNoisyCmd, dbPath, "--days", "7"), &noisy); err != nil {
		t.Fatalf("unmarshal noisy: %v", err)
	}
	if len(noisy) != 2 || noisy[0].MachineName != "WS-X" || noisy[0].AlertCount != 2 {
		t.Fatalf("noisy grouping wrong: %+v", noisy)
	}
	if noisy[0].LastAlert != "2026-05-30T11:00:00Z" {
		t.Errorf("LastAlert should be the chronologically-latest raw string, got %q", noisy[0].LastAlert)
	}
}

func TestNovelCommandsTolerateEmptyStore(t *testing.T) {
	withFixedNow(t)
	db := filepath.Join(t.TempDir(), "empty.db")
	// Each transcendence command must exit 0 and emit an empty (non-null) result.
	// patch-status short-circuits before any network IO when no agents are synced.
	for _, tc := range []struct {
		name string
		fn   func(*rootFlags) *cobra.Command
		args []string
	}{
		{"stale", newNovelAgentsStaleCmd, []string{"--days", "30"}},
		{"sla", newNovelTicketsSlaCmd, nil},
		{"workload", newNovelTicketsWorkloadCmd, nil},
		{"book", newNovelCustomersBookCmd, nil},
		{"triage", newNovelAlertsTriageCmd, nil},
		{"since", newNovelSinceCmd, []string{"24h"}},
		{"coverage", newNovelCustomersCoverageCmd, nil},
		{"expiring", newNovelContractsExpiringCmd, nil},
		{"noisy", newNovelAgentsNoisyCmd, nil},
		{"patch-status", newNovelAgentsPatchStatusCmd, nil},
	} {
		out := runNovel(t, tc.fn, db, tc.args...)
		var v any
		if err := json.Unmarshal(out, &v); err != nil {
			t.Errorf("%s: empty-store output is not valid JSON: %s", tc.name, out)
		}
	}
}
