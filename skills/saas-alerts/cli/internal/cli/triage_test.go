// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
	"saas-alerts-pp-cli/internal/store"
)

// TestNovelTriageHelpWires smoke-tests that the triage command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelTriageHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"triage", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("triage --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "triage"} {
		if !strings.Contains(help, want) {
			t.Fatalf("triage --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestTriageGroupsOrderingCountsAndDedupe(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	var got saasTriageResult
	stdout, stderr := saasRunJSON(t, &got, "triage", "--db", db, "--json")
	saasRequireKeys(t, stdout, "window_start", "window_end", "min_severity", "synced", "scanned_events", "total_groups", "groups", "window_totals_by_severity")
	if strings.Contains(stderr, "not been synced") || !got.Synced || got.Note != "" || got.MinSeverity != "low" {
		t.Errorf("synced fixture: synced=%v note=%q min_severity=%q stderr=%q", got.Synced, got.Note, got.MinSeverity, stderr)
	}
	if got.WindowStart != "2026-10-13T12:00:00Z" || got.WindowEnd != "2026-10-14T12:00:00Z" {
		t.Errorf("window = %s..%s", got.WindowStart, got.WindowEnd)
	}
	// 8 distinct in-window events: EV-A4 (3d), EV-A5 (34d), EV-C3 (2d) are
	// outside 24h and the two empty-eventId copies count once.
	if got.ScannedEvents != 8 {
		t.Errorf("scanned_events = %d, want 8", got.ScannedEvents)
	}
	if got.WindowTotalsBySeverity != (saasTriageTotals{Critical: 3, Medium: 3, Low: 2}) {
		t.Errorf("totals = %+v, want critical 3 medium 3 low 2", got.WindowTotalsBySeverity)
	}
	type row struct {
		cust, user, sev, desc string
		count                 int
	}
	want := []row{
		{"CUST_A", "jane.doe@example.com", "critical", saasBrute, 2},
		{"CUST_B", "bob.smith@example.com", "critical", saasBrute, 1},
		{"CUST_A", "jane.doe@example.com", "medium", saasOutside, 1},
		{"CUST_B", "bob.smith@example.com", "medium", saasOutside, 1},
		{"CUST_C", "carl.jones@example.com", "medium", saasOutside, 1},
		{"CUST_B", "bob.smith@example.com", "low", saasShare, 1},
		{"CUST_C", "carl.jones@example.com", "low", saasLogin, 1},
	}
	if len(got.Groups) != len(want) || got.TotalGroups != len(want) {
		t.Fatalf("groups = %d (total %d), want %d", len(got.Groups), got.TotalGroups, len(want))
	}
	for i, w := range want {
		g := got.Groups[i]
		if g.CustomerID != w.cust || g.User != w.user || g.Severity != w.sev || g.JointDesc != w.desc || g.Count != w.count {
			t.Errorf("group[%d] = %s/%s/%s/%q x%d, want %+v", i, g.CustomerID, g.User, g.Severity, g.JointDesc, g.Count, w)
		}
	}
	top := got.Groups[0]
	if top.FirstSeen != "2026-10-14T11:00:00Z" || top.LastSeen != "2026-10-14T11:30:00Z" {
		t.Errorf("top seen = %s..%s", top.FirstSeen, top.LastSeen)
	}
	if strings.Join(top.DistinctIPs, ",") != "198.51.100.7" || strings.Join(top.Countries, ",") != "US" {
		t.Errorf("top ips/countries = %v/%v", top.DistinctIPs, top.Countries)
	}
	if strings.Join(top.Tickets, ",") != "T-1001" || top.NoTicket {
		t.Errorf("top tickets = %v no_ticket=%v, want [T-1001] false", top.Tickets, top.NoTicket)
	}
	if top.MaxPullLagSeconds == nil || *top.MaxPullLagSeconds != 300 {
		t.Errorf("top max lag = %v, want 300", top.MaxPullLagSeconds)
	}
	if got.Groups[1].MaxPullLagSeconds != nil || !got.Groups[1].NoTicket || len(got.Groups[1].Tickets) != 0 {
		t.Errorf("B brute lag/ticket = %v/%v/%v, want null/true/[]", got.Groups[1].MaxPullLagSeconds, got.Groups[1].NoTicket, got.Groups[1].Tickets)
	}
	if lag := got.Groups[4].MaxPullLagSeconds; lag == nil || *lag != 600 {
		t.Errorf("C outside lag = %v, want 600", lag)
	}
	// The empty-eventId event was stored twice; it must be one group of one.
	if share := got.Groups[5]; share.Count != 1 || share.FirstSeen != "2026-10-14T09:15:00Z" {
		t.Errorf("deduped share group = x%d first %s", share.Count, share.FirstSeen)
	}
}

func TestTriageFiltersAndLimit(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)

	var med saasTriageResult
	saasRunJSON(t, &med, "triage", "--db", db, "--min-severity", "MEDIUM", "--json")
	if len(med.Groups) != 5 || med.MinSeverity != "medium" {
		t.Fatalf("medium+ groups = %d min_severity=%q, want 5 and the normalized echo \"medium\"", len(med.Groups), med.MinSeverity)
	}
	for _, g := range med.Groups {
		if g.Severity == "low" {
			t.Errorf("low group leaked through --min-severity medium: %+v", g)
		}
	}
	if med.WindowTotalsBySeverity.Low != 2 {
		t.Errorf("totals must still count low events, got %+v", med.WindowTotalsBySeverity)
	}

	var crit saasTriageResult
	saasRunJSON(t, &crit, "triage", "--db", db, "--min-severity", "critical", "--json")
	if len(crit.Groups) != 2 || crit.Groups[0].Count != 2 {
		t.Fatalf("critical groups = %+v", crit.Groups)
	}

	var byName saasTriageResult
	saasRunJSON(t, &byName, "triage", "--db", db, "--customer", "birch legal", "--json")
	if len(byName.Groups) != 3 {
		t.Fatalf("--customer by name groups = %d, want 3", len(byName.Groups))
	}
	for _, g := range byName.Groups {
		if g.CustomerID != "CUST_B" {
			t.Errorf("--customer leaked %s", g.CustomerID)
		}
	}
	if byName.WindowTotalsBySeverity != (saasTriageTotals{Critical: 1, Medium: 1, Low: 1}) {
		t.Errorf("--customer totals = %+v", byName.WindowTotalsBySeverity)
	}

	var byID saasTriageResult
	saasRunJSON(t, &byID, "triage", "--db", db, "--customer", "CUST_C", "--json")
	if len(byID.Groups) != 2 || byID.Groups[0].CustomerName != "Cedar Clinic" || byID.Groups[0].Severity != "medium" {
		t.Fatalf("--customer by id = %+v", byID.Groups)
	}

	var none saasTriageResult
	saasRunJSON(t, &none, "triage", "--db", db, "--customer", "CUST_NOPE", "--json")
	if len(none.Groups) != 0 || none.WindowTotalsBySeverity != (saasTriageTotals{}) || none.ScannedEvents != 8 {
		t.Fatalf("unmatched customer = %+v", none)
	}

	var limited saasTriageResult
	saasRunJSON(t, &limited, "triage", "--db", db, "--limit", "2", "--json")
	if len(limited.Groups) != 2 || limited.TotalGroups != 7 || limited.Groups[0].Count != 2 {
		t.Fatalf("--limit 2 = %d groups of %d", len(limited.Groups), limited.TotalGroups)
	}

	var week saasTriageResult
	saasRunJSON(t, &week, "triage", "--db", db, "--since", "7d", "--json")
	if week.ScannedEvents != 10 {
		t.Fatalf("7d scanned = %d, want 10 (adds EV-A4 and EV-C3)", week.ScannedEvents)
	}
}

func TestTriageUsageErrors(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	for _, args := range [][]string{
		{"triage", "--db", db, "--min-severity", "high"},
		{"triage", "--db", db, "--since", "yesterday"},
		{"triage", "--db", db, "--since", "0h"},
		{"triage", "--db", db, "--limit", "-1"},
	} {
		stdout, _, err := saasRunCLI(t, args...)
		if err == nil || ExitCode(err) != 2 {
			t.Errorf("%v: err=%v code=%d, want usage error (2)", args, err, ExitCode(err))
		}
		if stdout != "" {
			t.Errorf("%v: usage error must not print results: %q", args, stdout)
		}
	}
}

func TestTriageMissingStoreReturnsEmptyEnvelope(t *testing.T) {
	saasIsolate(t)
	missing := saasMissingDBPath(t)
	var got saasTriageResult
	stdout, stderr := saasRunJSON(t, &got, "triage", "--db", missing, "--json")
	if !strings.HasPrefix(stderr, "hint: no local mirror at "+missing) || !strings.Contains(stderr, "sync --resources customers,events --since 7d --db "+missing) {
		t.Errorf("stderr = %q", stderr)
	}
	if !strings.Contains(stdout, `"groups": []`) || got.ScannedEvents != 0 || got.WindowEnd != "2026-10-14T12:00:00Z" {
		t.Errorf("empty envelope = %s", stdout)
	}
}

func TestTriageSelectAndAgentOutput(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)

	stdout, _, err := saasRunCLI(t, "triage", "--db", db, "--select", "groups.customer_name,groups.count", "--json")
	if err != nil {
		t.Fatal(err)
	}
	saasRequireKeys(t, stdout, "groups")
	var sel struct {
		Groups []map[string]any `json:"groups"`
	}
	if err := json.Unmarshal([]byte(stdout), &sel); err != nil {
		t.Fatal(err)
	}
	if len(sel.Groups) != 7 {
		t.Fatalf("selected groups = %d", len(sel.Groups))
	}
	for _, g := range sel.Groups {
		if len(g) != 2 || g["customer_name"] == nil || g["count"] == nil {
			t.Fatalf("--select row = %v, want only customer_name and count", g)
		}
	}

	var agent struct {
		Meta    map[string]any   `json:"meta"`
		Results saasTriageResult `json:"results"`
	}
	saasRunJSON(t, &agent, "triage", "--db", db, "--agent")
	if agent.Meta["source"] != "local" {
		t.Errorf("agent meta = %v, want source local", agent.Meta)
	}
	if len(agent.Results.Groups) != 7 || len(agent.Results.Groups[0].DistinctIPs) != 1 || agent.Results.Groups[0].MaxPullLagSeconds == nil {
		t.Fatalf("--agent dropped group fields: %+v", agent.Results.Groups)
	}
}

func TestTriageHumanRender(t *testing.T) {
	saasIsolate(t)
	db := saasSeedFixtureStore(t)
	var got saasTriageResult
	saasRunJSON(t, &got, "triage", "--db", db, "--limit", "1", "--json")
	cmd := newNovelTriageCmd(&rootFlags{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := saasRenderTriageHuman(cmd, got); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"critical 3, medium 3, low 2", "Acme Dental", "T-1001", "showing 1 of 7 groups"} {
		if !strings.Contains(text, want) {
			t.Errorf("human output missing %q:\n%s", want, text)
		}
	}
}

func TestTriageOrderingSeverityThenCount(t *testing.T) {
	at := func(h int) time.Time { return saasFixtureNow.Add(-time.Duration(h) * time.Hour) }
	ev := func(cust, user, sev, desc string, h int) saasLocalEvent {
		return saasLocalEvent{CustomerID: cust, CustomerName: cust, UserName: user, Severity: sev, JointDesc: desc, Time: at(h)}
	}
	events := []saasLocalEvent{
		ev("C1", "old@example.com", "medium", "noisy", 9), // medium x3, oldest
		ev("C1", "old@example.com", "medium", "noisy", 8),
		ev("C1", "old@example.com", "medium", "noisy", 7),
		ev("C2", "new@example.com", "medium", "quiet", 1),   // medium x1, newest
		ev("C3", "crit@example.com", "critical", "rare", 9), // critical x1 beats any medium
		ev("C4", "odd@example.com", "", "unrated", 2),       // no alertStatus: listed last at --min-severity low
	}
	var r saasTriageResult
	saasBuildTriage(&r, events, saasSeverityRank("low"), "", 0)
	var order []string
	for _, g := range r.Groups {
		order = append(order, g.JointDesc+":"+g.Severity)
	}
	if strings.Join(order, ",") != "rare:critical,noisy:medium,quiet:medium,unrated:unknown" {
		t.Fatalf("order = %v", order)
	}
	if r.WindowTotalsBySeverity != (saasTriageTotals{Critical: 1, Medium: 4, Other: 1}) {
		t.Fatalf("totals = %+v", r.WindowTotalsBySeverity)
	}
	var med saasTriageResult
	saasBuildTriage(&med, events, saasSeverityRank("medium"), "", 0)
	if len(med.Groups) != 3 {
		t.Fatalf("medium+ must drop the unrated group, got %d groups", len(med.Groups))
	}
}

// Triage groups by the stable, customer-scoped account id (the storage rule:
// user.id, else UserId, else userEmail/email, else an email-shaped user.name).
// Names and emails are only labels; events with no stable account id land in
// an explicit "unknown account" bucket instead of merging with a named one.
func TestTriageGroupsByStableAccountIdentity(t *testing.T) {
	saasIsolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ev := func(id string, ago time.Duration, mutate func(saasM)) json.RawMessage {
		d := saasEventDoc(id, ago, "CUST_X", "Xylem Partners", "placeholder", "Microsoft", "medium", "outside.own.location", saasOutside, "192.0.2.180", "US")
		mutate(d)
		return saasMustJSON(t, d)
	}
	items := []json.RawMessage{
		ev("EV-U100", time.Hour, func(d saasM) { d["user"] = saasM{"name": "Charles Buck"}; d["UserId"] = "u-100" }),
		ev("EV-U200", 2*time.Hour, func(d saasM) { d["user"] = saasM{"name": "Charles Buck"}; d["UserId"] = "u-200" }),
		ev("EV-AMY", 3*time.Hour, func(d saasM) { delete(d, "user"); d["userEmail"] = "Amy@Example.com" }),
		ev("EV-BEN", 4*time.Hour, func(d saasM) { delete(d, "user"); d["userEmail"] = "ben@example.com" }),
		ev("EV-ANON1", 5*time.Hour, func(d saasM) { d["user"] = saasM{"name": "Charles Buck"} }),
		ev("EV-ANON2", 6*time.Hour, func(d saasM) { d["user"] = saasM{"name": "Someone Else"} }),
	}
	if _, failures, err := db.UpsertBatch("events", items); err != nil || failures != 0 {
		t.Fatalf("UpsertBatch: failures=%d err=%v", failures, err)
	}
	if err := db.SaveSyncState("events", "", 1); err != nil {
		t.Fatal(err)
	}
	db.Close()

	var got struct {
		Groups []struct {
			User           string `json:"user"`
			AccountID      string `json:"account_id"`
			UnknownAccount bool   `json:"unknown_account"`
			Count          int    `json:"count"`
		} `json:"groups"`
	}
	saasRunJSON(t, &got, "triage", "--db", dbPath, "--json")
	var rows []string
	for _, g := range got.Groups {
		rows = append(rows, fmt.Sprintf("%s|%s|%v|%d", g.User, g.AccountID, g.UnknownAccount, g.Count))
	}
	// Count desc, then newest last_seen. The two display-name-only events
	// share one unknown-account bucket and never join a named account.
	want := []string{
		"unknown account||true|2",
		"Charles Buck|u-100|false|1",
		"Charles Buck|u-200|false|1",
		"Amy@Example.com|amy@example.com|false|1",
		"ben@example.com|ben@example.com|false|1",
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("groups:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
}

// A raw account id is only unique per product: Microsoft and Google accounts
// that share an id inside one customer are different accounts and must not
// merge into one triage group.
func TestTriageSeparatesProductsForOneAccountID(t *testing.T) {
	saasIsolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ev := func(id string, ago time.Duration, product saasM) json.RawMessage {
		d := saasEventDoc(id, ago, "CUST_X", "Xylem Partners", "Shared Name", "Microsoft", "medium", "outside.own.location", saasOutside, "192.0.2.200", "US")
		d["user"] = saasM{"id": "acct-1", "name": "Shared Name"}
		d["product"] = product
		return saasMustJSON(t, d)
	}
	items := []json.RawMessage{
		ev("EV-MS1", time.Hour, saasM{"id": "prod-ms", "name": "Microsoft"}),
		ev("EV-MS2", 2*time.Hour, saasM{"id": "prod-ms", "name": "Microsoft"}),
		ev("EV-GWS", 3*time.Hour, saasM{"id": "prod-gws", "name": "Google Workspace"}),
	}
	if _, failures, err := db.UpsertBatch("events", items); err != nil || failures != 0 {
		t.Fatalf("UpsertBatch: failures=%d err=%v", failures, err)
	}
	if err := db.SaveSyncState("events", "", 1); err != nil {
		t.Fatal(err)
	}
	db.Close()

	type group struct {
		AccountID string `json:"account_id"`
		Product   string `json:"product"`
		Count     int    `json:"count"`
	}
	for _, mode := range [][]string{{"--json"}, {"--agent"}} {
		var rows []group
		if mode[0] == "--agent" {
			var got struct {
				Results struct {
					Groups []group `json:"groups"`
				} `json:"results"`
			}
			saasRunJSON(t, &got, append([]string{"triage", "--db", dbPath}, mode...)...)
			rows = got.Results.Groups
		} else {
			var got struct {
				Groups []group `json:"groups"`
			}
			saasRunJSON(t, &got, append([]string{"triage", "--db", dbPath}, mode...)...)
			rows = got.Groups
		}
		var flat []string
		for _, g := range rows {
			flat = append(flat, fmt.Sprintf("%s|%s|%d", g.AccountID, g.Product, g.Count))
		}
		if strings.Join(flat, ",") != "acct-1|Microsoft|2,acct-1|Google Workspace|1" {
			t.Errorf("%v groups = %v, want one Microsoft group of 2 and one Google Workspace group of 1", mode, flat)
		}
	}
}
