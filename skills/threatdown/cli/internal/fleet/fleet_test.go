// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func ago(h int) string { return now.Add(-time.Duration(h) * time.Hour).Format(time.RFC3339Nano) }

func TestParseSites(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"envelope", `{"sites":[{"id":"s1","account_id":"a1","company_name":"Acme"},{"id":"s2","company_name":"Trial"}]}`, 2},
		{"bare array", `[{"id":"s1","account_id":"a1","company_name":"Acme"}]`, 1},
		{"empty envelope", `{"sites":[]}`, 0},
	}
	for _, tc := range cases {
		got, err := ParseSites(json.RawMessage(tc.raw))
		if err != nil || len(got) != tc.want {
			t.Fatalf("%s: got %d sites, err %v; want %d", tc.name, len(got), err, tc.want)
		}
	}
	// Malformed lists must error, never read as "no sites" (reconciliation
	// would delete every cached site).
	for _, raw := range []string{`"nope"`, `null`, `{}`, `{"sites":null}`, `[null]`, `{"sites":[{"company_name":"No id"}]}`} {
		if _, err := ParseSites(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected error for %s", raw)
		}
	}
	if got, err := ParseSites(json.RawMessage(`[]`)); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("an empty bare array is a real empty inventory: %v %v", got, err)
	}
}

func TestAccountIDsSkipsUnsubscribedAndFilters(t *testing.T) {
	sites := []Site{{AccountID: "a1", Name: "Acme Corp"}, {AccountID: "", Name: "No Sub"}, {AccountID: "a2", Name: "Globex"}}
	if got := AccountIDs(sites, ""); strings.Join(got, ",") != "a1,a2" {
		t.Fatalf("all = %v", got)
	}
	if got := AccountIDs(sites, "acme"); strings.Join(got, ",") != "a1" {
		t.Fatalf("filtered = %v", got)
	}
	if got := AccountIDs(sites, "nomatch"); len(got) != 0 {
		t.Fatalf("mismatch filter should return none, got %v", got)
	}
}

func TestParseEndpoint(t *testing.T) {
	raw := `{"display_name":"LAPTOP-042","protection_status":"protected",
	 "agent":{"machine_id":"m1","account_id":"a1","host_name":"laptop-042","serial_number":"SN1",
	  "is_software_update_available":true,"os_info":{"os_type":"Workstation","os_platform":"Windows"},
	  "nics":[{"mac_address":"AA:BB:CC:00:11:22"}]},
	 "machine":{"id":"m1","account_id":"a1","online":true,"isolated":false,"infection_count":2,
	  "suspicious_activity_count":1,"reboot_required":1,"last_day_seen":"2026-10-13T10:00:00Z","last_active":"2026-10-13T11:30:00.123Z"}}`
	e, err := ParseEndpoint(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "m1" || e.AccountID != "a1" || e.HostName != "laptop-042" || !e.RebootRequired || !e.UpdateAvailable {
		t.Fatalf("unexpected flatten: %+v", e)
	}
	if len(e.MACs) != 1 || e.MACs[0] != "aa:bb:cc:00:11:22" {
		t.Fatalf("mac not normalized: %v", e.MACs)
	}
	if h := e.HoursSinceSeen(now); h != 0 {
		t.Fatalf("last_active should win over last_day_seen; hours = %d", h)
	}
	if _, err := ParseEndpoint(json.RawMessage(`{"machine":{"id":"m1"}}`)); err == nil {
		t.Fatal("an endpoint without an account id must be malformed")
	}
	if _, err := ParseEndpoint(json.RawMessage(`{"agent":{},"machine":{}}`)); err == nil {
		t.Fatal("record without machine id must error")
	}
}

func TestHoursSinceSeen(t *testing.T) {
	cases := []struct {
		e    Endpoint
		want int
	}{
		{Endpoint{LastDaySeen: ago(30)}, 30},
		{Endpoint{LastActive: ago(5), LastDaySeen: ago(30)}, 5},
		{Endpoint{LastDaySeen: "2026-10-12"}, 36},
		{Endpoint{}, -1},
		{Endpoint{LastDaySeen: "garbage"}, -1},
	}
	for i, tc := range cases {
		if got := tc.e.HoursSinceSeen(now); got != tc.want {
			t.Errorf("case %d: got %d want %d", i, got, tc.want)
		}
	}
}

func TestStale(t *testing.T) {
	sites := map[string]Site{"a1": {Name: "Acme"}, "a2": {Name: "Globex"}}
	all := []Endpoint{
		{ID: "fresh", AccountID: "a1", HostName: "pc1", LastDaySeen: ago(2)},
		{ID: "dark30", AccountID: "a1", HostName: "pc2", LastDaySeen: ago(30)},
		{ID: "dark300", AccountID: "a1", HostName: "pc3", LastDaySeen: ago(300)},
		{ID: "never", AccountID: "a2", HostName: "pc4"},
		{ID: "deleted", AccountID: "a2", HostName: "pc5", LastDaySeen: ago(500), IsDeleted: true},
	}
	rows := Stale(all, sites, now, StaleOptions{Hours: 24, NewSince: 72 * time.Hour})
	if len(rows) != 3 {
		t.Fatalf("want 3 stale rows (fresh and deleted excluded), got %d: %+v", len(rows), rows)
	}
	if rows[0].EndpointID != "dark30" || rows[0].NewlyDark == nil || !*rows[0].NewlyDark || rows[0].HoursSinceSeen != 30 || rows[0].Site != "Acme" {
		t.Fatalf("first row should be the newly dark Acme endpoint: %+v", rows[0])
	}
	if rows[1].EndpointID != "dark300" || rows[1].NewlyDark == nil || *rows[1].NewlyDark {
		t.Fatalf("chronic endpoint must not be newly dark: %+v", rows[1])
	}
	if rows[2].EndpointID != "never" || rows[2].HoursSinceSeen != -1 {
		t.Fatalf("never-seen endpoint should be stale with -1 hours: %+v", rows[2])
	}
	if none := Stale(all[:1], sites, now, StaleOptions{Hours: 24}); len(none) != 0 {
		t.Fatalf("a fresh fleet must produce no stale rows, got %+v", none)
	}
	// Without NewSince the newly-dark check never ran: the field must be
	// absent, not a false "no".
	for _, r := range Stale(all, sites, now, StaleOptions{Hours: 24}) {
		if r.NewlyDark != nil {
			t.Fatalf("newly_dark must be unset without NewSince: %+v", r)
		}
	}
}

func TestStaleTwinShowsHostName(t *testing.T) {
	sites := map[string]Site{"a1": {Name: "Acme"}}
	all := []Endpoint{
		{ID: "old", AccountID: "a1", HostName: "pc1", SerialNumber: "SN1", LastDaySeen: ago(400)},
		{ID: "new", AccountID: "a1", HostName: "pc1-reimaged", SerialNumber: "SN1", LastDaySeen: ago(1)},
	}
	rows := Stale(all, sites, now, StaleOptions{Hours: 24, Twins: true})
	if len(rows) != 1 || rows[0].TwinOf != "pc1-reimaged" || rows[0].TwinOfID != "new" {
		t.Fatalf("twin should be named by host name with its id alongside: %+v", rows)
	}
	if plain := Stale(all, sites, now, StaleOptions{Hours: 24}); plain[0].TwinOf != "" || plain[0].TwinOfID != "" {
		t.Fatalf("no twin lookup without Twins: %+v", plain[0])
	}
}

func TestLiveTwins(t *testing.T) {
	all := []Endpoint{
		{ID: "old", AccountID: "a1", HostName: "pc1", SerialNumber: "SN1", LastDaySeen: ago(400)},
		{ID: "new", AccountID: "a1", HostName: "pc1-reimaged", SerialNumber: "SN1", LastDaySeen: ago(1)},
		{ID: "othersite", AccountID: "a2", HostName: "pc9", SerialNumber: "SN1", LastDaySeen: ago(400)},
		{ID: "macold", AccountID: "a1", HostName: "x", MACs: []string{"aa"}, LastDaySeen: ago(100)},
		{ID: "macnew", AccountID: "a1", HostName: "y", MACs: []string{"aa"}, LastDaySeen: ago(3)},
		{ID: "lonely", AccountID: "a1", HostName: "z", LastDaySeen: ago(100)},
	}
	twins := LiveTwins(all, now, 24)
	if twins["old"] != "new" || twins["macold"] != "macnew" {
		t.Fatalf("expected serial and mac twins, got %v", twins)
	}
	if _, ok := twins["othersite"]; ok {
		t.Fatal("a twin must be in the same account")
	}
	if _, ok := twins["lonely"]; ok {
		t.Fatal("an endpoint with no live match must not get a twin")
	}
}

func TestHealthFromEndpoints(t *testing.T) {
	sites := []Site{{AccountID: "a1", Name: "Acme"}, {AccountID: "a2", Name: "Empty Co"}}
	all := []Endpoint{
		{ID: "1", AccountID: "a1", Online: true, LastDaySeen: ago(1), LastScannedAt: ago(10), InfectionCount: 1},
		{ID: "2", AccountID: "a1", LastDaySeen: ago(50), Isolated: true, ProtectionStatus: "unprotected", RebootRequired: true},
		{ID: "3", AccountID: "a1", LastDaySeen: ago(1), IsDeleted: true, InfectionCount: 9},
	}
	rows := HealthFromEndpoints(all, sites, now, 24, 7)
	if len(rows) != 2 {
		t.Fatalf("every site needs a row, got %d", len(rows))
	}
	acme := rows[0]
	if acme.Site != "Acme" || acme.Total != 2 || acme.Online != 1 || acme.Stale != 1 || acme.Infected != 1 ||
		acme.Isolated != 1 || acme.Unprotected != 1 || acme.RebootRequired != 1 || acme.ScanOverdue != 1 {
		t.Fatalf("wrong rollup: %+v", acme)
	}
	if acme.Attention != 10+6+4+3+2+1 {
		t.Fatalf("attention score = %d", acme.Attention)
	}
	if rows[1].Site != "Empty Co" || rows[1].Total != 0 || rows[1].Attention != 0 {
		t.Fatalf("empty site row wrong: %+v", rows[1])
	}
}

func TestDecodeJobStatus(t *testing.T) {
	cases := map[int]string{32: "COMPLETED", 16: "TIMED_OUT", 1 | 2: "CREATED|SENT", 0: "UNKNOWN(0)", 1024: "UNKNOWN(1024)"}
	for in, want := range cases {
		if got := DecodeJobStatus(in); got != want {
			t.Errorf("DecodeJobStatus(%d) = %q want %q", in, got, want)
		}
	}
	if JobStatusNotRan != 223 {
		t.Fatalf("not-ran status sum = %d, want 223", JobStatusNotRan)
	}
	if !JobFailed(64) || JobFailed(32) || JobFailed(8) {
		t.Fatal("JobFailed misclassifies")
	}
	if !JobTerminal("completed") || JobTerminal("STARTED") || JobTerminal("") {
		t.Fatal("JobTerminal misclassifies")
	}
}

func TestSpread(t *testing.T) {
	sites := map[string]Site{"a1": {Name: "Acme"}, "a2": {Name: "Globex"}}
	dets := []Detection{
		{AccountID: "a1", MachineID: "m1", ThreatName: "Trojan.X", SHA256: "AB", ActionTaken: "quarantined", ReportedAt: ago(5)},
		{AccountID: "a2", MachineID: "m2", ThreatName: "Trojan.X", SHA256: "ab", ActionTaken: "quarantined", ReportedAt: ago(30)},
		{AccountID: "a2", MachineID: "m3", ThreatName: "Trojan.X", SHA256: "ab", ActionTaken: "blocked", ReportedAt: ago(10)},
		{AccountID: "a1", MachineID: "m1", ThreatName: "PUP.Y", ReportedAt: ago(2)},
		{AccountID: "a1", MachineID: "m1", ThreatName: "Old.Z", SHA256: "cd", ReportedAt: ago(24 * 30)},
	}
	rows := Spread(dets, sites, now.Add(-7*24*time.Hour), 1)
	if len(rows) != 2 {
		t.Fatalf("want 2 groups in window (old one excluded), got %+v", rows)
	}
	top := rows[0]
	if top.Threat != "Trojan.X" || top.Sites != 2 || top.Endpoints != 3 || top.Detections != 3 ||
		top.ActionsTaken["quarantined"] != 2 || strings.Join(top.SiteNames, ",") != "Acme,Globex" {
		t.Fatalf("hash group wrong: %+v", top)
	}
	if multi := Spread(dets, sites, now.Add(-7*24*time.Hour), 2); len(multi) != 1 || multi[0].Threat != "Trojan.X" {
		t.Fatalf("min-sites 2 should keep only the cross-site threat: %+v", multi)
	}
	if none := Spread(nil, sites, now, 1); len(none) != 0 {
		t.Fatal("no detections must give no rows")
	}
}

func TestQueue(t *testing.T) {
	sites := map[string]Site{"a1": {Name: "Acme"}}
	eps := []Endpoint{
		{ID: "quiet", AccountID: "a1", HostName: "clean-pc"},
		{ID: "loose", AccountID: "a1", HostName: "infected pc", Online: true, InfectionCount: 2},
		{ID: "contained", AccountID: "a1", HostName: "boxed", Isolated: true, InfectionCount: 1},
		{ID: "sus", AccountID: "a1", HostName: "sus", SuspiciousCount: 3},
		{ID: "forgot", AccountID: "a1", HostName: "forgot", Isolated: true},
	}
	dets := []Detection{{AccountID: "a1", MachineID: "loose", ThreatName: "Ransom.A", ReportedAt: ago(2)}}
	rows := Queue(eps, dets, sites, now.Add(-7*24*time.Hour), "td")
	if len(rows) != 4 {
		t.Fatalf("clean endpoint must not be queued; got %d rows", len(rows))
	}
	order := []string{rows[0].EndpointID, rows[1].EndpointID, rows[2].EndpointID, rows[3].EndpointID}
	if strings.Join(order, ",") != "loose,contained,sus,forgot" {
		t.Fatalf("rank order = %v", order)
	}
	if rows[0].Suggested != "td respond isolate --host 'infected pc' --site Acme" || rows[0].LastThreat != "Ransom.A" || rows[0].Rank != 1 {
		t.Fatalf("top row = %+v", rows[0])
	}
	if rows[1].Suggested != "td respond scan --host boxed --site Acme --remove" {
		t.Fatalf("isolated infected endpoint should get a scan suggestion: %q", rows[1].Suggested)
	}
	if rows[3].Kind != "forgotten-isolation" || rows[3].Suggested != "td respond restore --host forgot --site Acme" {
		t.Fatalf("forgotten isolation row = %+v", rows[3])
	}
}

func TestJobCommands(t *testing.T) {
	want := map[string]string{
		CmdIsolate: "command.edr.isolation",
		CmdRestore: "command.edr.restore",
		CmdScan:    "command.threat.scan",
		CmdReboot:  "command.asset.reboot",
		CmdUpdate:  "command.protection.update.now",
	}
	for got, w := range want {
		if got != w {
			t.Errorf("job command %q, want %q", got, w)
		}
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"LAPTOP-042":    "LAPTOP-042",
		"pc.acme.local": "pc.acme.local",
		"infected pc":   "'infected pc'",
		"x;rm -rf /":    "'x;rm -rf /'",
		"a&b":           "'a&b'",
		"line\nbreak":   "'line\nbreak'",
		"it's":          `'it'\''s'`,
		"$(id)":         "'$(id)'",
		"":              "''",
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolvedDetectionsAreHistoryNotThreats(t *testing.T) {
	sites := map[string]Site{"a1": {Name: "Acme"}}
	eps := []Endpoint{{ID: "clean", AccountID: "a1", HostName: "pc1", Online: true}}
	since := now.Add(-7 * 24 * time.Hour)
	done := []Detection{
		{AccountID: "a1", MachineID: "clean", ThreatName: "PUP.X", Status: "quarantined", ReportedAt: ago(3)},
		{AccountID: "a1", MachineID: "clean", ThreatName: "PUP.Y", ActionTaken: "blocked", ReportedAt: ago(4)},
		{AccountID: "a1", MachineID: "clean", ThreatName: "PUP.Z", CleanedAt: ago(1), ReportedAt: ago(5)},
	}
	if rows := Queue(eps, done, sites, since, "td"); len(rows) != 0 {
		t.Fatalf("resolved detections on a clean endpoint must not queue it: %+v", rows)
	}
	open := append(done, Detection{AccountID: "a1", MachineID: "clean", ThreatName: "Trojan", Status: "found", ReportedAt: ago(2)})
	rows := Queue(eps, open, sites, since, "td")
	if len(rows) != 1 || rows[0].Kind != "active-threat" || rows[0].Detections != 4 {
		t.Fatalf("an unresolved detection should queue the endpoint with full history: %+v", rows)
	}
}

func TestLabelVersusTargetingName(t *testing.T) {
	e, err := ParseEndpoint(json.RawMessage(`{"display_name":"Front Desk","agent":{"machine_id":"m1","account_id":"a1"},"machine":{"id":"m1","account_id":"a1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if e.HostName != "" || e.Label() != "Front Desk" {
		t.Fatalf("display name must label but never become the host name: %+v label=%q", e, e.Label())
	}
}

func TestQueueContainmentAndTargetingSafety(t *testing.T) {
	sites := map[string]Site{"a1": {Name: "Acme Corp"}}
	since := now.Add(-7 * 24 * time.Hour)
	eps := []Endpoint{
		{ID: "contained", AccountID: "a1", HostName: "ws02", Isolated: true},
		{ID: "alias-only", AccountID: "a1", DisplayName: "ws02", InfectionCount: 1},
		{ID: "mixed", AccountID: "a1", HostName: "pc2", InfectionCount: 0},
	}
	dets := []Detection{
		{AccountID: "a1", MachineID: "contained", ThreatName: "Ransom.Lockbit", Status: "quarantined", ReportedAt: ago(2)},
		{AccountID: "a1", MachineID: "mixed", ThreatName: "Trojan.Agent", Status: "found", ReportedAt: ago(30)},
		{AccountID: "a1", MachineID: "mixed", ThreatName: "PUP.Bundler", Status: "quarantined", ReportedAt: ago(1)},
	}
	rows := Queue(eps, dets, sites, since, "td")
	byID := map[string]QueueRow{}
	for _, r := range rows {
		byID[r.EndpointID] = r
	}
	if r, ok := byID["contained"]; ok {
		t.Fatalf("a just-contained endpoint must not be suggested for release: %+v", r)
	}
	alias := byID["alias-only"]
	if !strings.Contains(alias.Suggested, `--stdin`) || strings.Contains(alias.Suggested, "--host") || !strings.Contains(alias.Suggested, `"endpoint_id":"alias-only"`) {
		t.Fatalf("an endpoint without a host name must be targeted by id, not alias: %q", alias.Suggested)
	}
	if byID["mixed"].LastThreat != "Trojan.Agent" {
		t.Fatalf("last_threat should name the open threat, got %q", byID["mixed"].LastThreat)
	}
	if !strings.Contains(byID["mixed"].Suggested, "--site 'Acme Corp'") {
		t.Fatalf("suggestions are site-scoped: %q", byID["mixed"].Suggested)
	}
}

func TestKnownJobState(t *testing.T) {
	for _, s := range []string{"COMPLETED", "pending", "TIMED_OUT", "cancelled"} {
		if !KnownJobState(s) {
			t.Errorf("%s should be known", s)
		}
	}
	for _, s := range []string{"", "garbage", "DONE"} {
		if KnownJobState(s) {
			t.Errorf("%q should not be known", s)
		}
	}
}
