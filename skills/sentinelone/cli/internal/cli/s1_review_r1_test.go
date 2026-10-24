// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Regression tests for the round-1 review of the hand layer. Each one failed on
// the code it was written against; the comment names the defect class.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"sentinelone-pp-cli/internal/cliutil/testenv"
	"sentinelone-pp-cli/internal/store"
)

// ---- history capture: only a full, unfiltered, refreshed walk is fleet state ----

// s1StatefulAPI serves one agent and one threat the way SentinelOne's list
// endpoints do: createdAt__gte (spec.yaml /agents and /threats, "created after
// or at this timestamp") returns only records created at or after it. The
// agent and threat were created long ago, so an incremental walk returns none.
func s1StatefulAPI(t *testing.T, network *string, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		windowed := r.URL.Query().Get("createdAt__gte") != ""
		mu.Lock()
		status := *network
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/agents"):
			if windowed {
				_, _ = w.Write([]byte(`{"data":[],"pagination":{"nextCursor":null,"totalItems":0}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"101","computerName":"ws-01","agentVersion":"23.4.1","networkStatus":"` + status + `","createdAt":"2020-01-01T00:00:00Z"}],"pagination":{"nextCursor":null,"totalItems":1}}`))
		case strings.HasSuffix(r.URL.Path, "/threats"):
			if windowed {
				_, _ = w.Write([]byte(`{"data":[],"pagination":{"nextCursor":null,"totalItems":0}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"201","threatInfo":{"threatName":"x","createdAt":"2020-01-01T00:00:00Z"}}],"pagination":{"nextCursor":null,"totalItems":1}}`))
		default:
			_, _ = w.Write([]byte(`{"data":[],"pagination":{"nextCursor":null,"totalItems":0}}`))
		}
	}))
}

func runSyncArgs(t *testing.T, db string, args ...string) {
	t.Helper()
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"sync", "--db", db, "--json"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("sync %v: %v\n%s", args, err, out.String())
	}
}

func captureCount(t *testing.T, db, entity string) int {
	t.Helper()
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	times, err := s.FleetSnapshotTimes(entity)
	if err != nil {
		t.Fatal(err)
	}
	return len(times)
}

// TestOrdinarySyncRefreshesStateAndOnlyFullWalksAreCaptured: an ordinary
// second sync must refresh existing agents (not just fetch newly created ones),
// and a windowed, filtered, capped or dry-run sync must never be recorded as
// the fleet's state.
func TestOrdinarySyncRefreshesStateAndOnlyFullWalksAreCaptured(t *testing.T) {
	testenv.Isolate(t)
	var mu sync.Mutex
	network := "connected"
	srv := s1StatefulAPI(t, &network, &mu)
	defer srv.Close()
	t.Setenv("SENTINELONE_BASE_URL", srv.URL)
	t.Setenv("SENTINELONE_API_TOKEN", "test-token-1234")
	db := filepath.Join(t.TempDir(), "s1.db")

	runSyncArgs(t, db, "--resources", "agents,threats")
	mu.Lock()
	network = "disconnected"
	mu.Unlock()
	time.Sleep(1100 * time.Millisecond)
	runSyncArgs(t, db, "--resources", "agents,threats")

	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.Get("agents", "101")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"disconnected"`) {
		t.Fatalf("an ordinary sync left agent 101 stale: %s", raw)
	}
	times, _ := s.FleetSnapshotTimes("agents")
	if len(times) != 2 {
		t.Fatalf("agents captures = %d, want 2", len(times))
	}
	rows, _ := s.FleetSnapshotRowsByID("agents", times[1])
	if !strings.Contains(string(rows["101"]), `"disconnected"`) {
		t.Fatalf("latest capture is not the refreshed state: %s", rows["101"])
	}
	s.Close()

	for _, args := range [][]string{
		{"--resources", "agents", "--since", "7d"},
		{"--resources", "agents", "--param", "siteIds=1"},
		{"--resources", "agents", "--resource-param", "agents:siteIds=1"},
		{"--resources", "agents", "--max-pages", "1"},
		{"--resources", "agents", "--latest-only"},
		{"--resources", "agents", "--dry-run"},
	} {
		time.Sleep(1100 * time.Millisecond)
		runSyncArgs(t, db, args...)
		if n := captureCount(t, db, "agents"); n != 2 {
			t.Fatalf("sync %v recorded a fleet capture (agents captures = %d, want 2)", args, n)
		}
	}
}

// ---- reclaim: a hostname twin is not proof of replacement ----

func TestReclaimNeverListsAHostnameTwinAsReplaced(t *testing.T) {
	agents := []map[string]any{
		agentFix("1", "WS01", "Main", "24.1", map[string]any{"siteId": "9", "isActive": false, "lastActiveDate": daysAgo(1)}),
		agentFix("2", "WS01", "Main", "24.1", map[string]any{"siteId": "9", "lastActiveDate": daysAgo(0)}),
	}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelAgentsReclaimCmd(jsonFlags()), "--db", db)
	if got := out["reclaimable_total"].(float64); got != 0 {
		t.Fatalf("reclaimable_total = %v, want 0: a machine offline one day must not be listed for decommission because another shares its name; output %v", got, out)
	}
}

// ---- site identity: two customers' "Default site" are two sites ----

func twoDefaultSites() ([]map[string]any, []map[string]any) {
	off := func(id, name, site, acct, version string) map[string]any {
		return agentFix(id, name, "Default site", version, map[string]any{
			"siteId": site, "accountName": acct, "osType": "windows",
			"isActive": false, "lastActiveDate": daysAgo(60), "mitigationMode": "detect",
		})
	}
	agents := []map[string]any{
		off("10", "ACME-PC", "100", "Acme", "23.4.1.1"),
		off("20", "BOLT-PC", "200", "Bolt", "23.4.1.1"),
		agentFix("30", "ACME-NEW", "Default site", "24.1.2.6", map[string]any{"siteId": "100", "accountName": "Acme", "osType": "windows"}),
	}
	th := func(id, ep, agentID, site, acct string) map[string]any {
		t := threatFix(id, "Trojan", "h-"+id, ep, "Default site", map[string]any{
			"mitigationStatus": "mitigated", "incidentStatus": "resolved",
			"createdAt":   time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339),
			"mitigatedAt": time.Now().Add(-4 * time.Hour).UTC().Format(time.RFC3339),
		})
		t["agentRealtimeInfo"] = map[string]any{"agentComputerName": ep, "agentId": agentID, "siteName": "Default site", "siteId": site, "accountName": acct}
		return t
	}
	threats := []map[string]any{th("t1", "ACME-PC", "10", "100", "Acme"), th("t2", "BOLT-PC", "20", "200", "Bolt")}
	return agents, threats
}

func TestSameNamedSitesInDifferentAccountsStayApart(t *testing.T) {
	agents, threats := twoDefaultSites()
	db := newSeededStore(t, agents, threats)
	for _, tc := range []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		key  string
		want int
	}{
		{"agents reclaim", func() *cobra.Command { return newNovelAgentsReclaimCmd(jsonFlags()) }, nil, "sites", 2},
		{"versions plan", func() *cobra.Command { return newNovelVersionsPlanCmd(jsonFlags()) }, nil, "groups", 2},
		{"coverage", func() *cobra.Command { return newNovelCoverageCmd(jsonFlags()) }, nil, "sites", 2},
		{"posture", func() *cobra.Command { return newNovelPostureCmd(jsonFlags()) }, nil, "sites", 2},
		{"sites risk", func() *cobra.Command { return newNovelSitesRiskCmd(jsonFlags()) }, nil, "sites", 2},
		{"fleet-health", func() *cobra.Command { return newNovelFleetHealthCmd(jsonFlags()) }, nil, "sites", 2},
		{"fleet-health summary", func() *cobra.Command { return newNovelFleetHealthSummaryCmd(jsonFlags()) }, []string{"--by-site"}, "sites", 2},
		{"threats mttr", func() *cobra.Command { return newNovelThreatsMttrCmd(jsonFlags()) }, nil, "sites", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := runJSON(t, tc.cmd(), append([]string{"--db", db}, tc.args...)...)
			rows, _ := out[tc.key].([]any)
			if len(rows) != tc.want {
				t.Fatalf("%s: %d %s rows, want %d (two accounts' \"Default site\" merged): %v", tc.name, len(rows), tc.key, tc.want, out)
			}
			ids := map[string]bool{}
			for _, r := range rows {
				id, _ := r.(map[string]any)["site_id"].(string)
				ids[id] = true
			}
			if !ids["100"] || !ids["200"] {
				t.Fatalf("%s: rows must carry site_id 100 and 200, got %v", tc.name, ids)
			}
		})
	}
}

// ---- whatchanged: an entity is compared only across its own captures ----

func TestWhatchangedNeverReadsAMissingThreatCaptureAsResolution(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	t0 := time.Now().Add(-3 * time.Hour)
	t1 := time.Now().Add(-1 * time.Minute)
	ag := []map[string]any{agentFix("a1", "WS-01", "SiteA", "24.1", nil)}
	seedSnapshot(t, db, "agents", t0, ag)
	seedSnapshot(t, db, "threats", t0, []map[string]any{threatFix("t1", "Trojan", "h1", "WS-01", "SiteA", nil)})
	seedSnapshot(t, db, "agents", t1, ag) // an agents-only sync: no threats capture at t1
	out := runJSON(t, newNovelWhatchangedCmd(jsonFlags()), "--db", db, "--since", "2h")
	if rs, _ := out["resolved_threats"].([]any); len(rs) != 0 {
		t.Fatalf("an agents-only capture made active threat t1 read as resolved: %v", out)
	}
	if cmp, ok := out["threats_compared"].(bool); !ok || cmp {
		t.Fatalf("threats_compared = %v, want false (only one threats capture exists): %v", out["threats_compared"], out)
	}
}

// ---- versions: targets, denominators and comparisons are per OS ----

func mixedOSAgents() []map[string]any {
	return []map[string]any{
		agentFix("w1", "WIN-1", "SiteA", "24.1.2.6", map[string]any{"osType": "windows"}),
		agentFix("w2", "WIN-2", "SiteA", "24.1.2.6", map[string]any{"osType": "windows"}),
		agentFix("l1", "LNX-1", "SiteA", "23.4.1.1", map[string]any{"osType": "linux"}),
	}
}

func TestVersionCurrencyIsJudgedPerOS(t *testing.T) {
	agents := mixedOSAgents()
	db := newSeededStore(t, agents, nil)
	seedSnapshot(t, db, "agents", time.Now().Add(-time.Hour), agents)

	out := runJSON(t, newNovelVersionsCmd(jsonFlags()), "--db", db)
	if b := out["behind_current"].(float64); b != 0 {
		t.Fatalf("versions: behind_current = %v, want 0 (each OS is on its own newest build): %v", b, out)
	}
	ro := runJSON(t, newNovelVersionsRolloutCmd(jsonFlags()), "--db", db)
	tl := ro["timeline"].([]any)
	if pct := tl[len(tl)-1].(map[string]any)["on_target_pct"].(float64); pct != 100 {
		t.Fatalf("versions rollout: on_target_pct = %v, want 100 (Linux is not behind a Windows build): %v", pct, ro)
	}
	st := runJSON(t, newNovelFleetHealthStaleCmd(jsonFlags()), "--db", db, "--min-score", "0")
	for _, a := range st["agents"].([]any) {
		m := a.(map[string]any)
		rs, _ := m["reasons"].([]any)
		for _, r := range rs {
			if r.(string) == "behind fleet version" {
				t.Fatalf("fleet-health stale: %v flagged behind a different OS's version", m["computer_name"])
			}
		}
	}
	po := runJSON(t, newNovelPostureCmd(jsonFlags()), "--db", db)
	if v := po["sites"].([]any)[0].(map[string]any)["version_compliance_pct"].(float64); v != 100 {
		t.Fatalf("posture: version_compliance_pct = %v, want 100", v)
	}
}

func TestRetiredAgentsNeverPickTheTargetVersion(t *testing.T) {
	var agents []map[string]any
	for i := 0; i < 10; i++ {
		agents = append(agents, agentFix("d"+itoa(i), "OLD-"+itoa(i), "SiteA", "23.4.1.1", map[string]any{"osType": "windows", "isDecommissioned": true}))
	}
	agents = append(agents,
		agentFix("n1", "NEW-1", "SiteA", "24.1.2.6", map[string]any{"osType": "windows"}),
		agentFix("n2", "NEW-2", "SiteA", "24.1.2.6", map[string]any{"osType": "windows"}))
	db := newSeededStore(t, agents, nil)
	seedSnapshot(t, db, "agents", time.Now().Add(-time.Hour), agents)
	ro := runJSON(t, newNovelVersionsRolloutCmd(jsonFlags()), "--db", db)
	tl := ro["timeline"].([]any)
	if pct := tl[len(tl)-1].(map[string]any)["on_target_pct"].(float64); pct != 100 {
		t.Fatalf("rollout on_target_pct = %v, want 100 (decommissioned agents chose the target): %v", pct, ro)
	}
	out := runJSON(t, newNovelVersionsCmd(jsonFlags()), "--db", db)
	if b := out["behind_current"].(float64); b != 0 {
		t.Fatalf("versions behind_current = %v, want 0: %v", b, out)
	}
}

func TestVersionsPlanNeverClaimsCompletionForUnmeasuredAgents(t *testing.T) {
	db := newSeededStore(t, []map[string]any{agentFix("x", "NOVER", "SiteA", "", map[string]any{"osType": "windows"})}, nil)
	cmd := newNovelVersionsPlanCmd(&rootFlags{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--db", db, "--os", "windows", "--target", "24.1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "at or above the target") {
		t.Fatalf("plan asserted compliance for an agent with no version:\n%s", out.String())
	}
}

// ---- endpoint identity: a hostname is not an agent ----

func twoDC01() ([]map[string]any, []map[string]any) {
	agents := []map[string]any{
		agentFix("1", "DC01", "SiteA", "24.1", map[string]any{"uuid": "u1", "siteId": "100"}),
		agentFix("2", "DC01", "SiteB", "24.1", map[string]any{"uuid": "u2", "siteId": "200"}),
	}
	th := func(id, agentID, site, siteID, created string, info map[string]any) map[string]any {
		if info == nil {
			info = map[string]any{}
		}
		info["createdAt"] = created
		t := threatFix(id, "Trojan", "samehash", "DC01", site, info)
		t["agentRealtimeInfo"] = map[string]any{"agentComputerName": "DC01", "agentId": agentID, "siteName": site, "siteId": siteID}
		return t
	}
	now := time.Now().UTC()
	threats := []map[string]any{
		th("ta1", "1", "SiteA", "100", now.Add(-48*time.Hour).Format(time.RFC3339), nil),
		th("ta2", "1", "SiteA", "100", now.Add(-24*time.Hour).Format(time.RFC3339), nil),
		th("tb1", "2", "SiteB", "200", now.Add(-48*time.Hour).Format(time.RFC3339), nil),
		th("tb2", "2", "SiteB", "200", now.Add(-24*time.Hour).Format(time.RFC3339), nil),
	}
	return agents, threats
}

func TestDossierJoinsThreatsAndActivitiesByAgentIdentity(t *testing.T) {
	agents, threats := twoDC01()
	db := newSeededStore(t, agents, threats)
	seedResource(t, db, "activities", []map[string]any{
		{"id": "act-b", "agentId": "2", "primaryDescription": "B's activity", "createdAt": daysAgo(0), "data": map[string]any{"computerName": "DC01"}},
		{"id": "act-a", "agentId": "1", "primaryDescription": "A's activity", "createdAt": daysAgo(0)},
	})
	out := runJSON(t, newNovelAgentsDossierCmd(jsonFlags()), "--db", db, "1")
	if th, _ := out["threats"].([]any); len(th) != 2 {
		t.Fatalf("dossier for agent 1 shows %d threats, want its own 2 (not the other DC01's): %v", len(th), out["threats"])
	}
	acts, _ := out["recent_activities"].([]any)
	if len(acts) != 1 || acts[0].(map[string]any)["description"] != "A's activity" {
		t.Fatalf("dossier for agent 1 activities = %v, want only A's", acts)
	}
}

func TestRecurrenceKeepsSameNamedEndpointsApart(t *testing.T) {
	agents, threats := twoDC01()
	db := newSeededStore(t, agents, threats)
	out := runJSON(t, newNovelThreatsRecurrenceCmd(jsonFlags()), "--db", db)
	g := out["threats"].([]any)[0].(map[string]any)
	if d := g["distinct_endpoints"].(float64); d != 2 {
		t.Fatalf("distinct_endpoints = %v, want 2 (two agents named DC01)", d)
	}
	by := runJSON(t, newNovelThreatsRecurrenceCmd(jsonFlags()), "--db", db, "--by-agent")
	if rows := by["endpoints"].([]any); len(rows) != 2 {
		t.Fatalf("--by-agent rows = %d, want 2 (DC01 in SiteA and DC01 in SiteB): %v", len(rows), rows)
	}
}

func TestRecurrenceRequiresMitigationBeforeRedetection(t *testing.T) {
	ep := func(t map[string]any) map[string]any {
		t["agentRealtimeInfo"] = map[string]any{"agentComputerName": "WS-1", "agentId": "1", "siteName": "S", "siteId": "9"}
		return t
	}
	oct := func(d, h int) string { return time.Date(2026, 10, d, h, 0, 0, 0, time.UTC).Format(time.RFC3339) }
	// Active detection on Oct 1, a separate detection mitigated on Oct 2: the
	// active one predates the mitigation, so nothing came back after cleanup.
	neg := []map[string]any{
		ep(threatFix("a", "X", "hh", "WS-1", "S", map[string]any{"createdAt": oct(1, 0)})),
		ep(threatFix("b", "X", "hh", "WS-1", "S", map[string]any{"createdAt": oct(2, 0), "mitigationStatus": "mitigated", "mitigatedAt": oct(2, 1)})),
	}
	out := runJSON(t, newNovelThreatsRecurrenceCmd(jsonFlags()), "--db", newSeededStore(t, nil, neg))
	if g := out["threats"].([]any)[0].(map[string]any); g["recurred_after_mitigation"].(bool) {
		t.Fatalf("recurred_after_mitigation = true for a detection that predates the mitigation: %v", g)
	}
	// Mitigated Oct 1, detected again Oct 3 on the same agent: a real recurrence.
	pos := []map[string]any{
		ep(threatFix("a", "X", "hh", "WS-1", "S", map[string]any{"createdAt": oct(1, 0), "mitigationStatus": "mitigated", "mitigatedAt": oct(1, 2)})),
		ep(threatFix("b", "X", "hh", "WS-1", "S", map[string]any{"createdAt": oct(3, 0)})),
	}
	out = runJSON(t, newNovelThreatsRecurrenceCmd(jsonFlags()), "--db", newSeededStore(t, nil, pos))
	if g := out["threats"].([]any)[0].(map[string]any); !g["recurred_after_mitigation"].(bool) {
		t.Fatalf("recurred_after_mitigation = false for a redetection after mitigation: %v", g)
	}
}

func TestBlastRadiusCountsEachEndpointOnce(t *testing.T) {
	ep := func(t map[string]any) map[string]any {
		t["agentRealtimeInfo"] = map[string]any{"agentComputerName": "WS-1", "agentId": "1", "siteName": "S", "siteId": "9"}
		return t
	}
	threats := []map[string]any{
		ep(threatFix("a", "X", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "WS-1", "S", nil)),
		ep(threatFix("b", "X", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "WS-1", "S", map[string]any{"mitigationStatus": "mitigated"})),
	}
	out := runJSON(t, newNovelThreatsBlastRadiusCmd(jsonFlags()), "--db", newSeededStore(t, nil, threats), "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if n := out["endpoints_total"].(float64); n != 1 {
		t.Fatalf("endpoints_total = %v, want 1 (two detections on one agent)", n)
	}
	if a, m := out["active"].(float64), out["mitigated"].(float64); a != 1 || m != 0 {
		t.Fatalf("active=%v mitigated=%v, want 1 and 0 (the endpoint still has an active detection)", a, m)
	}
}

// ---- mttr: time to mitigate ends at mitigation, not at the last edit ----

func TestMTTRUsesMitigationCompletionNotLastUpdate(t *testing.T) {
	now := time.Now().UTC()
	det := now.Add(-72 * time.Hour)
	threats := []map[string]any{
		threatFix("m", "X", "h", "WS", "S", map[string]any{
			"mitigationStatus": "mitigated", "incidentStatus": "resolved",
			"createdAt":   det.Format(time.RFC3339),
			"mitigatedAt": det.Add(time.Hour).Format(time.RFC3339),
			"updatedAt":   det.Add(48 * time.Hour).Format(time.RFC3339),
		}),
	}
	out := runJSON(t, newNovelThreatsMttrCmd(jsonFlags()), "--db", newSeededStore(t, nil, threats), "--sla", "24")
	if h := out["overall_mttr_hours"].(float64); h != 1 {
		t.Fatalf("overall_mttr_hours = %v, want 1 (mitigated an hour after detection; the later update is not mitigation)", h)
	}
	if b := out["sites"].([]any)[0].(map[string]any)["sla_breaches"].(float64); b != 0 {
		t.Fatalf("sla_breaches = %v, want 0", b)
	}
}

// TestMTTRReadsMitigationActionEndTimes: SentinelOne lists a threat's
// mitigation actions under a top-level mitigationStatus array; the latest
// successful mitigationEndedAt is when mitigation completed. Without any
// mitigation time the threat is mitigated_unmeasured, never timed by updatedAt.
func TestMTTRReadsMitigationActionEndTimes(t *testing.T) {
	det := time.Now().UTC().Add(-72 * time.Hour)
	timed := threatFix("a", "X", "h1", "WS", "S", map[string]any{
		"mitigationStatus": "mitigated", "incidentStatus": "resolved",
		"createdAt": det.Format(time.RFC3339), "updatedAt": det.Add(40 * time.Hour).Format(time.RFC3339),
	})
	timed["mitigationStatus"] = []any{
		map[string]any{"action": "kill", "status": "success", "mitigationEndedAt": det.Add(time.Hour).Format(time.RFC3339)},
		map[string]any{"action": "quarantine", "status": "success", "mitigationEndedAt": det.Add(2 * time.Hour).Format(time.RFC3339)},
		map[string]any{"action": "remediate", "status": "failed", "mitigationEndedAt": det.Add(30 * time.Hour).Format(time.RFC3339)},
	}
	untimed := threatFix("b", "Y", "h2", "WS", "S", map[string]any{
		"mitigationStatus": "mitigated", "incidentStatus": "resolved",
		"createdAt": det.Format(time.RFC3339), "updatedAt": det.Add(40 * time.Hour).Format(time.RFC3339),
	})
	out := runJSON(t, newNovelThreatsMttrCmd(jsonFlags()), "--db", newSeededStore(t, nil, []map[string]any{timed, untimed}), "--sla", "24")
	if h := out["overall_mttr_hours"].(float64); h != 2 {
		t.Fatalf("overall_mttr_hours = %v, want 2 (last successful action ended 2h after detection)", h)
	}
	site := out["sites"].([]any)[0].(map[string]any)
	if u := site["mitigated_unmeasured"].(float64); u != 1 {
		t.Fatalf("mitigated_unmeasured = %v, want 1", u)
	}
	if b := site["sla_breaches"].(float64); b != 0 {
		t.Fatalf("sla_breaches = %v, want 0 (an untimed mitigation is not a breach)", b)
	}
}
