// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for counts an MSP acts on:
//   - a distinct-computer total is the union of computers across its parts, never
//     the sum of per-part counts (DetectedComputerSoftwareResponse rows are one
//     title on one computer, keyed by computerId, so one computer behind on two
//     titles is still one computer);
//   - a count labeled failed holds only failures (MaintenanceActionResult read by
//     the shared outcome parser), never actions a user filter selected.
// The new JSON fields are decoded into test-local shapes so these tests compile
// against, and fail on, code that does not emit them yet.

package cli

import (
	"strings"
	"testing"
	"time"
)

func swRow(id int, computerID any, computer, tenant string, tenantID int, title, version string) map[string]any {
	return map[string]any{"id": id, "computerId": computerID, "computerName": computer, "tenantId": tenantID,
		"tenantName": tenant, "softwareName": title, "version": version, "detectedAt": "2026-10-26T00:00:00Z"}
}

type countDriftTitle struct {
	Title           string `json:"title"`
	ComputerCount   int    `json:"computer_count"`
	ComputersBehind int    `json:"computers_behind"`
}

type countDriftTenant struct {
	Tenant             string            `json:"tenant"`
	TitlesBehind       int               `json:"titles_behind"`
	ComputersBehind    int               `json:"computers_behind"`
	UnidentifiedBehind int               `json:"computers_behind_unidentified"`
	Behind             []countDriftTitle `json:"behind"`
}

type countDriftView struct {
	Tenants []countDriftTenant `json:"tenants"`
	Note    string             `json:"note"`
}

func driftByTenant(t *testing.T, db string) (map[string]countDriftTenant, countDriftView, string) {
	t.Helper()
	out, err := runCLI(t, "drift", "--json", "--db", db)
	if err != nil {
		t.Fatalf("drift: %v\n%s", err, out)
	}
	v := decode[countDriftView](t, out)
	by := map[string]countDriftTenant{}
	for _, tn := range v.Tenants {
		by[tn.Tenant] = tn
	}
	return by, v, out
}

func TestDriftComputersBehindIsDistinctComputersAcrossTitles(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			swRow(1, 101, "WS-101", "Tenant One", 1, "Google Chrome", "1"),
			swRow(2, 101, "WS-101", "Tenant One", 1, "Microsoft Edge", "1"),
			swRow(3, 201, "WS-201", "Tenant Two", 2, "Google Chrome", "2"),
			swRow(4, 201, "WS-201", "Tenant Two", 2, "Microsoft Edge", "2"),
		},
	})
	by, _, out := driftByTenant(t, db)
	one := by["Tenant One"]
	if one.TitlesBehind != 2 || len(one.Behind) != 2 {
		t.Fatalf("Tenant One = %+v, want 2 titles behind:\n%s", one, out)
	}
	if one.ComputersBehind != 1 {
		t.Fatalf("Tenant One computers_behind = %d, want 1 (computer 101 is behind on Chrome AND Edge; it is one computer):\n%s",
			one.ComputersBehind, out)
	}
	for _, b := range one.Behind {
		if b.ComputersBehind != 1 || b.ComputerCount != 1 {
			t.Fatalf("Tenant One %s = %+v, want computers_behind 1 of computer_count 1 (per-title counts stay per title)", b.Title, b)
		}
	}
	if one.UnidentifiedBehind != 0 {
		t.Fatalf("Tenant One computers_behind_unidentified = %d, want 0 (every row carries a computerId)", one.UnidentifiedBehind)
	}
	if two := by["Tenant Two"]; two.TitlesBehind != 0 || two.ComputersBehind != 0 {
		t.Fatalf("Tenant Two = %+v, want nothing behind (it holds the fleet latest of both titles)", two)
	}
}

func TestDriftRowWithNoComputerIDIsNeverMergedNorDropped(t *testing.T) {
	noID := func(id int, computer string) map[string]any {
		r := swRow(id, nil, computer, "Contoso", 7, "Google Chrome", "139.0")
		delete(r, "computerId")
		return r
	}
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			swRow(1, 1, "WS-01", "Contoso", 7, "Google Chrome", "141.0"),
			// Four lagging rows with no computerId at one version: two carry no
			// name, two share the name WS-02. Neither a name nor a version proves
			// two rows are one machine, so none of them may merge.
			noID(2, ""), noID(3, ""), noID(4, "WS-02"), noID(5, "WS-02"),
		},
	})
	by, v, out := driftByTenant(t, db)
	c := by["Contoso"]
	if c.ComputersBehind != 4 || len(c.Behind) != 1 || c.Behind[0].ComputersBehind != 4 {
		t.Fatalf("Contoso = %+v, want 4 computers behind on Chrome (no row without a computerId merged into another):\n%s", c, out)
	}
	if c.Behind[0].ComputerCount != 5 {
		t.Fatalf("Contoso Chrome computer_count = %d, want 5 (the same key rule as computers_behind)", c.Behind[0].ComputerCount)
	}
	if c.UnidentifiedBehind != 4 {
		t.Fatalf("Contoso computers_behind_unidentified = %d, want 4 (the rows counted without a computerId are reported):\n%s",
			c.UnidentifiedBehind, out)
	}
	if !strings.Contains(v.Note, "computerId") {
		t.Fatalf("note = %q, want it to say some lagging rows carry no computerId", v.Note)
	}
}

type countSpreadBucket struct {
	Version       string `json:"version"`
	ComputerCount int    `json:"computer_count"`
}

type countSpreadView struct {
	BelowFloor             int                 `json:"computers_below_floor"`
	UnidentifiedBelowFloor int                 `json:"computers_below_floor_unidentified"`
	Versions               []countSpreadBucket `json:"versions"`
	Note                   string              `json:"note"`
}

func TestVersionSpreadBelowFloorCountsEachComputerOnce(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			// Computer 1 has two Edge-line titles below the floor.
			swRow(1, 1, "WS-01", "Contoso", 7, "Microsoft Edge", "139.0"),
			swRow(2, 1, "WS-01", "Contoso", 7, "Microsoft Edge WebView2 Runtime", "138.0"),
			swRow(3, 2, "WS-02", "Fabrikam", 8, "Microsoft Edge", "141.0"),
			// Computer 3 carries the same exact title twice, at two old builds.
			swRow(4, 3, "WS-03", "Contoso", 7, "Google Chrome", "138.0"),
			swRow(5, 3, "WS-03", "Contoso", 7, "Google Chrome", "139.0"),
			swRow(6, 4, "WS-04", "Contoso", 7, "Google Chrome", "141.0"),
		},
	})
	out, err := runCLI(t, "version-spread", "Microsoft Edge", "--contains", "--min-version", "140", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread --contains: %v\n%s", err, out)
	}
	if v := decode[countSpreadView](t, out); v.BelowFloor != 1 {
		t.Fatalf("computers_below_floor = %d, want 1 (computer 1 is below the floor on two titles; it is one computer):\n%s", v.BelowFloor, out)
	}
	out, err = runCLI(t, "version-spread", "Google Chrome", "--min-version", "140", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	v := decode[countSpreadView](t, out)
	if v.BelowFloor != 1 {
		t.Fatalf("computers_below_floor = %d, want 1 (computer 3 has two old builds; it is one computer):\n%s", v.BelowFloor, out)
	}
	for _, b := range v.Versions {
		if b.ComputerCount != 1 {
			t.Fatalf("version %s computer_count = %d, want 1 (per-version counts stay per version):\n%s", b.Version, b.ComputerCount, out)
		}
	}
}

func TestVersionSpreadRowWithNoComputerIDStillCountsBelowFloor(t *testing.T) {
	noID := swRow(2, nil, "", "Contoso", 7, "Google Chrome", "137.0")
	delete(noID, "computerId")
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			swRow(1, 1, "WS-01", "Contoso", 7, "Google Chrome", "141.0"),
			noID,
		},
	})
	out, err := runCLI(t, "version-spread", "Google Chrome", "--min-version", "140", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	v := decode[countSpreadView](t, out)
	if v.BelowFloor != 1 || v.UnidentifiedBelowFloor != 1 {
		t.Fatalf("computers_below_floor = %d (unidentified %d), want 1 (1): a lagging row with no computerId is counted, and said so:\n%s",
			v.BelowFloor, v.UnidentifiedBelowFloor, out)
	}
	if !strings.Contains(v.Note, "computerId") {
		t.Fatalf("note = %q, want it to say a row below the floor carries no computerId", v.Note)
	}
}

type countTriageCluster struct {
	Action         string `json:"action"`
	Status         string `json:"status"`
	ActionCount    int    `json:"action_count"`
	MatchedActions int    `json:"matched_actions"`
}

type countTriageView struct {
	ScannedActions int                  `json:"scanned_actions"`
	FailedActions  int                  `json:"failed_actions"`
	MatchedActions int                  `json:"matched_actions"`
	Clusters       []countTriageCluster `json:"clusters"`
	Note           string               `json:"note"`
}

func triageOutcomeRows(now string) []map[string]any {
	return []map[string]any{
		// GetMaintenanceActionResponse integer forms: result 1 = Success, status 4 = Complete.
		{"id": 1, "maintenanceSessionId": 601, "computerId": 42, "computerName": "WS-42", "tenantId": 7, "tenantName": "Contoso",
			"maintenanceDisplayName": "Install Chrome", "result": 1, "status": 4, "startTime": now},
		// result 2 = Failed, status 4 = Complete.
		{"id": 2, "maintenanceSessionId": 602, "computerId": 43, "computerName": "WS-43", "tenantId": 7, "tenantName": "Contoso",
			"maintenanceDisplayName": "Patch Windows", "result": 2, "status": 4, "resultReasonMessage": "Reboot pending", "startTime": now},
	}
}

func TestSessionTriageStatusSelectsNonFailuresAsMatchedNotFailed(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{"maintenance-actions": triageOutcomeRows(now)})

	out, err := runCLI(t, "session-triage", "--status", "Success", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage --status Success: %v\n%s", err, out)
	}
	v := decode[countTriageView](t, out)
	if v.FailedActions != 0 {
		t.Fatalf("failed_actions = %d, want 0 (a successful action selected by --status Success is not a failure):\n%s", v.FailedActions, out)
	}
	if v.MatchedActions != 1 {
		t.Fatalf("matched_actions = %d, want 1 (the action --status Success selected):\n%s", v.MatchedActions, out)
	}
	if len(v.Clusters) != 1 || v.Clusters[0].MatchedActions != 1 || v.Clusters[0].ActionCount != 1 {
		t.Fatalf("clusters = %+v, want one cluster with matched_actions 1 and action_count 1:\n%s", v.Clusters, out)
	}

	// The human summary says matched, and never calls the selection failed.
	human, err := runCLI(t, "session-triage", "--status", "Success", "--human-friendly", "--db", db)
	if err != nil {
		t.Fatalf("session-triage --status Success (human): %v\n%s", err, human)
	}
	if !strings.Contains(human, "matched") || strings.Contains(strings.ToLower(human), "fail") {
		t.Fatalf("human summary for --status Success must say matched and never failed:\n%s", human)
	}

	// --status selecting a real failure still counts it as failed.
	out, err = runCLI(t, "session-triage", "--status", "Failed", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage --status Failed: %v\n%s", err, out)
	}
	if v := decode[countTriageView](t, out); v.MatchedActions != 1 || v.FailedActions != 1 {
		t.Fatalf("--status Failed matched %d / failed %d, want 1 / 1:\n%s", v.MatchedActions, v.FailedActions, out)
	}
	human, err = runCLI(t, "session-triage", "--status", "Failed", "--human-friendly", "--db", db)
	if err != nil {
		t.Fatalf("session-triage --status Failed (human): %v\n%s", err, human)
	}
	if !strings.Contains(human, "1 of them failed") {
		t.Fatalf("human summary for --status Failed must count the failure among the matched actions:\n%s", human)
	}
}

func TestSessionTriageDefaultKeepsFailedMeaning(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{"maintenance-actions": triageOutcomeRows(now)})

	out, err := runCLI(t, "session-triage", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	v := decode[countTriageView](t, out)
	if v.ScannedActions != 2 || v.FailedActions != 1 || v.MatchedActions != 1 {
		t.Fatalf("scanned %d / failed %d / matched %d, want 2 / 1 / 1 (the default selection is the failures):\n%s",
			v.ScannedActions, v.FailedActions, v.MatchedActions, out)
	}
	if len(v.Clusters) != 1 || v.Clusters[0].Action != "Patch Windows" || v.Clusters[0].MatchedActions != 1 {
		t.Fatalf("clusters = %+v, want only the Patch Windows failure:\n%s", v.Clusters, out)
	}
	human, err := runCLI(t, "session-triage", "--since", "24h", "--human-friendly", "--db", db)
	if err != nil {
		t.Fatalf("session-triage (human): %v\n%s", err, human)
	}
	if !strings.Contains(human, "1 failed action(s)") {
		t.Fatalf("default human summary must keep counting failed actions:\n%s", human)
	}
}
