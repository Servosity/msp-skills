// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func TestValidateFQDN(t *testing.T) {
	good := []string{"acme.3cx.us", "pbx.example.com:5001", "10.0.0.5", "10.0.0.5:5001", "PBX.Example.com"}
	for _, f := range good {
		if err := ValidateFQDN(f); err != nil {
			t.Errorf("ValidateFQDN(%q) = %v, want ok", f, err)
		}
	}
	bad := []string{"", "https://acme.3cx.us", "acme.3cx.us/xapi", "a.com@b.com", "a.com:0", "a.com:99999", "a.com:",
		"a..com", "-a.com", "a.com?x", "a com", "user:pw@a.com", "[::1]"}
	for _, f := range bad {
		if err := ValidateFQDN(f); err == nil {
			t.Errorf("ValidateFQDN(%q) = nil, want error", f)
		}
	}
}

func TestPBXValidate(t *testing.T) {
	ok := PBX{Name: "acme", FQDN: "acme.3cx.us", ClientID: "900", EnvPrefix: "THREECX_ACME", Tags: []string{"gold"}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]PBX{
		"name":   {Name: "../etc", FQDN: "a.com", ClientID: "1", EnvPrefix: "X"},
		"upper":  {Name: "Acme", FQDN: "a.com", ClientID: "1", EnvPrefix: "X"},
		"client": {Name: "a", FQDN: "a.com", ClientID: " ", EnvPrefix: "X"},
		"env":    {Name: "a", FQDN: "a.com", ClientID: "1", EnvPrefix: "lower-case"},
		"tag":    {Name: "a", FQDN: "a.com", ClientID: "1", EnvPrefix: "X", Tags: []string{"Bad Tag"}},
	}
	for name, p := range cases {
		if err := p.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want error", name)
		}
	}
}

func TestRegistrySelect(t *testing.T) {
	r := &Registry{PBXes: []PBX{{Name: "a", Tags: []string{"gold"}}, {Name: "b"}, {Name: "c", Tags: []string{"gold"}}}}
	all, _ := r.Select(nil, nil)
	if len(all) != 3 {
		t.Fatalf("all = %d", len(all))
	}
	got, err := r.Select([]string{"b"}, []string{"gold"})
	if err != nil || len(got) != 3 || got[0].Name != "b" {
		t.Fatalf("select = %+v %v", got, err)
	}
	if _, err := r.Select([]string{"zzz"}, nil); err == nil || !strings.Contains(err.Error(), "registered: a, b, c") {
		t.Fatalf("unknown name error = %v", err)
	}
	if _, err := r.Select(nil, []string{"golf"}); err == nil || !strings.Contains(err.Error(), "tags in use: gold") {
		t.Fatalf("unknown tag error = %v", err)
	}
	if _, err := (&Registry{PBXes: []PBX{{Name: "a"}}}).Select(nil, []string{"gold"}); err == nil || !strings.Contains(err.Error(), "no registered PBX has a tag") {
		t.Fatalf("tag on an untagged fleet error = %v", err)
	}
	if r.Upsert(PBX{Name: "b", FQDN: "x"}) != true || r.PBXes[1].FQDN != "x" {
		t.Fatal("upsert should replace b")
	}
	if !r.Remove("a") || r.Remove("a") {
		t.Fatal("remove should succeed once")
	}
}

func TestItemsAndRedact(t *testing.T) {
	body := json.RawMessage(`{"@odata.context":"x","value":[{"Number":"100","AuthPassword":"pw","VMPIN":"1234","EnablePin":true,"Nested":{"ClientSecret":"s","Ok":"y"}}]}`)
	rows := Items(body)
	if len(rows) != 1 || rows[0]["Number"] != "100" {
		t.Fatalf("rows = %v", rows)
	}
	Redact(rows[0])
	if rows[0]["AuthPassword"] != Redacted || rows[0]["VMPIN"] != Redacted {
		t.Fatalf("secrets not redacted: %v", rows[0])
	}
	if rows[0]["EnablePin"] != true {
		t.Fatalf("boolean flag must survive: %v", rows[0])
	}
	nested := rows[0]["Nested"].(map[string]any)
	if nested["ClientSecret"] != Redacted || nested["Ok"] != "y" {
		t.Fatalf("nested = %v", nested)
	}
	single := Items(json.RawMessage(`{"@odata.context":"x","Version":"20"}`))
	if len(single) != 1 || single[0]["Version"] != "20" || single[0]["@odata.context"] != nil {
		t.Fatalf("single = %v", single)
	}
	if got := Items(json.RawMessage(`[1,2]`)); len(got) != 2 || got[0]["value"] != float64(1) {
		t.Fatalf("scalar array = %v", got)
	}
	red, _ := RedactJSON(json.RawMessage(`{"Location":{"GbJson":{"Concealed":false,"Contents":"{gcs-json-key}","Name":"key.json"}},"LicenseKey":"AAAA","ProductCode":"P","PinNumber":"4321","TranscribeKeyGrok":"gk",
		"ProvLink":"https://pbx/provisioning/abc","ProvisioningLinkExt":"https://x/prov/def","AuthPassword":{"ConcealedValue":"zz"},"Number":"100"}`))
	for _, leak := range []string{"gcs-json-key", "AAAA", "4321", "gk", "abc", "def", "zz"} {
		if strings.Contains(string(red), leak) {
			t.Fatalf("RedactJSON leaked %q: %s", leak, red)
		}
	}
	if !strings.Contains(string(red), `"P"`) || !strings.Contains(string(red), `"100"`) {
		t.Fatalf("RedactJSON over-redacted: %s", red)
	}
}

func TestTimeAndVersion(t *testing.T) {
	if _, ok := ParseTime("0001-01-01T00:00:00Z"); ok {
		t.Fatal("year-1 placeholder must be unset")
	}
	exp, ok := ParseTime("2026-10-30T00:00:00Z")
	if !ok || DaysUntil(exp, now) != 15 {
		t.Fatalf("days = %d", DaysUntil(exp, now))
	}
	past, _ := ParseTime("2026-10-13T00:00:00Z")
	if d := DaysUntil(past, now); d != -2 {
		t.Fatalf("past days = %d, want -2", d)
	}
	cases := []struct {
		a, b string
		want int
	}{{"20.0.10.1479", "20.0.8.1109", 1}, {"20.0.8", "20.0.10", -1}, {"20.0", "20.0.0", 0}, {"18", "20", -1}}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%s,%s) = %d", c.a, c.b, got)
		}
	}
}

func healthyStatus() SystemStatus {
	return SystemStatus{Version: "20.0.10.1479", LicenseActive: ptr(true), ExpirationDate: "2027-06-30T00:00:00Z",
		MaintenanceExpiresAt: "2027-06-30T00:00:00Z", LastBackupDateTime: "2026-10-14T02:00:00Z", BackupScheduled: ptr(true),
		TrunksRegistered: ptr(2), TrunksTotal: ptr(2), ExtensionsRegistered: ptr(8), ExtensionsTotal: ptr(10), TotalDiskSpace: ptr(int64(100)), FreeDiskSpace: ptr(int64(60)), MaxSimCalls: ptr(16), CallsActive: ptr(3),
		HasNotRunningServices: ptr(false), HasUnregisteredSystemExtensions: ptr(false), Firmware3CXOutdated: ptr(false),
		RecordingStopped: ptr(false), RecordingQuotaReached: ptr(false), VoicemailStopped: ptr(false), VoicemailQuotaReached: ptr(false)}
}

func TestEvaluateHealth(t *testing.T) {
	o := HealthOptions{Now: now, WarnDays: 30, BackupStale: 48 * time.Hour}
	p := PBX{Name: "acme", FQDN: "acme.3cx.us"}
	row := EvaluateHealth(p, healthyStatus(), o)
	if row.Status != Pass || row.Problems != "" {
		t.Fatalf("healthy = %s %q %+v", row.Status, row.Problems, row.Checks)
	}
	s := healthyStatus()
	s.ExpirationDate = "2026-10-30T00:00:00Z" // 16 days -> warn
	s.LastBackupDateTime = "2026-09-20T02:00:00Z"
	s.TrunksRegistered = ptr(1)
	s.FreeDiskSpace = ptr(int64(4))
	s.HasNotRunningServices = ptr(true)
	s.RecordingStopped = ptr(true)
	s.CallsActive = ptr(16)
	row = EvaluateHealth(p, s, o)
	if row.Status != Fail {
		t.Fatalf("degraded status = %s", row.Status)
	}
	for _, want := range []string{"license:warn", "backup:fail", "trunks:fail", "disk:fail", "services:fail", "storage_quota:warn", "capacity:warn"} {
		if !strings.Contains(row.Problems, want) {
			t.Errorf("problems %q missing %s", row.Problems, want)
		}
	}
	s = healthyStatus()
	s.LastBackupDateTime = ""
	if row := EvaluateHealth(p, s, o); !strings.Contains(row.Problems, "backup:fail") {
		t.Fatalf("never-backed-up must fail: %q", row.Problems)
	}
	s = healthyStatus()
	s.LicenseActive = ptr(false)
	if row := EvaluateHealth(p, s, o); !strings.Contains(row.Problems, "license:fail") {
		t.Fatalf("inactive licence must fail: %q", row.Problems)
	}
	rows := []HealthRow{{PBX: "b", Status: Pass}, ErrorHealthRow(PBX{Name: "c"}, errTest), {PBX: "a", Status: Warn}}
	SortHealth(rows)
	if rows[0].PBX != "c" || rows[1].PBX != "a" || rows[2].PBX != "b" {
		t.Fatalf("sort = %v", rows)
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }

var errTest = testErr("HTTP 403")

func TestTrunks(t *testing.T) {
	trunks := []Trunk{
		{Number: "10000", IsOnline: ptr(true), ConfigurationIssue: ptr(""), CertificateExpirationDate: "2026-11-01T00:00:00Z"},
		{Number: "10001", IsOnline: ptr(false), ConfigurationIssue: ptr("Registration failed")},
		{Number: "10002", IsOnline: ptr(true), ConfigurationIssue: ptr("")},
	}
	rows := TrunkRows(PBX{Name: "acme"}, trunks, now)
	rows = append(rows, ErrorTrunkRow(PBX{Name: "globex"}, errTest))
	SortTrunks(rows)
	if rows[0].PBX != "globex" || rows[1].Trunk != "10001" || rows[2].Trunk != "10000" || rows[3].Trunk != "10002" {
		t.Fatalf("order = %+v", rows)
	}
	if rows[0].State != "error" || rows[1].State != "issue" || rows[2].State != "online" {
		t.Fatalf("states wrong: %+v", rows)
	}
	misconfigured := TrunkRow{PBX: "acme", Online: ptr(true), ConfigurationIssue: "Outbound caller ID rejected"}
	if !misconfigured.NeedsAttention() || trunkState(misconfigured) != "issue" {
		t.Fatal("an online trunk with a configuration issue must count as down")
	}
	if rows[2].CertDaysLeft == nil || *rows[2].CertDaysLeft != 17 {
		t.Fatalf("cert days = %v", rows[2].CertDaysLeft)
	}
}

func TestLicencesAndVersions(t *testing.T) {
	l := LicenseStatus{ProductCode: "3CXPSPROF", ExpirationDate: "2026-11-13T12:00:00Z", MaintenanceExpiresAt: "2026-10-01T00:00:00Z", ResellerName: "Someone Else"}
	s := SystemStatus{UserExtensions: ptr(40), MaxUserExtensions: ptr(50)}
	row := LicenceRowFor(PBX{Name: "acme"}, l, s, "Acme MSP", now)
	if *row.LicenseDaysLeft != 30 || *row.MaintenanceDaysLeft != -14 || *row.SeatHeadroom != 10 || !row.ResellerMismatch {
		t.Fatalf("licence row = %+v", row)
	}
	if d, _ := row.SoonestDays(); d != -14 {
		t.Fatalf("soonest = %d", d)
	}
	s.MaxUserExtensions = ptr(0)
	if r := LicenceRowFor(PBX{Name: "x"}, l, s, "", now); r.SeatHeadroom != nil || r.ResellerMismatch {
		t.Fatalf("uncapped/no-reseller row = %+v", r)
	}

	var u UpdateList
	_ = json.Unmarshal([]byte(`{"IsMaintananceExpired":true,"Entries":[{"OutOfDate":true},{"OutOfDate":true,"Ignore":true},{"OutOfDate":false}]}`), &u)
	var current UpdateList
	_ = json.Unmarshal([]byte(`{"IsMaintananceExpired":false,"Entries":[]}`), &current)
	rows := []VersionRow{
		// Every input reported: maintenance current in both sources, an
		// (empty) update list. Fewer inputs make the row unknown.
		VersionRowFor(PBX{Name: "new"}, SystemStatus{Version: "20.0.10.1479", MaintenanceExpiresAt: "2099-01-01T00:00:00Z"}, current, now),
		VersionRowFor(PBX{Name: "old"}, SystemStatus{Version: "20.0.8.1109"}, u, now),
		{PBX: "down", Error: "x"},
	}
	MarkDrift(rows)
	if rows[0].PBX != "down" || rows[1].PBX != "old" || !*rows[1].Behind || rows[1].PendingUpdates != 1 || !*rows[1].MaintenanceExpired {
		t.Fatalf("drift = %+v", rows)
	}
	if *rows[2].Behind || rows[2].NewestInFleet != "20.0.10.1479" || rows[2].NeedsAttention() {
		t.Fatalf("newest row = %+v", rows[2])
	}
}

func TestBackups(t *testing.T) {
	set := BackupSettings{ScheduleEnabled: ptr(false)}
	files := []Backup{
		{FileName: "old.zip", CreationTime: "2026-10-13T02:00:00Z", Size: ptr(int64(200))},
		{FileName: "new.zip", CreationTime: "2026-10-14T02:00:00Z", Size: ptr(int64(50))},
	}
	row := BackupRowFor(PBX{Name: "a"}, SystemStatus{LastBackupDateTime: "2026-10-14T02:00:00Z"}, set, files, 48*time.Hour, now)
	if row.NewestFile != "new.zip" || strings.Join(row.Flags, ",") != "UNSCHEDULED,SHRUNK" || row.Status != "unscheduled" {
		t.Fatalf("row = %+v", row)
	}
	row = BackupRowFor(PBX{Name: "a"}, SystemStatus{}, BackupSettings{ScheduleEnabled: ptr(true)}, nil, 48*time.Hour, now)
	if strings.Join(row.Flags, ",") != "NEVER" {
		t.Fatalf("never row = %+v", row)
	}
	row = BackupRowFor(PBX{Name: "a"}, SystemStatus{LastBackupDateTime: "2026-10-14T02:00:00Z"}, BackupSettings{ScheduleEnabled: ptr(true)}, files[:1], 48*time.Hour, now)
	if row.Status != "ok" || len(row.Flags) != 0 {
		t.Fatalf("healthy backup row = %+v", row)
	}
}

func snap(kind string, full bool, scope []string, health []HealthRow, trunks []TrunkRow) Snapshot {
	return Snapshot{RunAt: now, Kind: kind, Full: full, Scope: scope, Health: health, Trunks: trunks}
}

func TestHistoryDiffs(t *testing.T) {
	o := HealthOptions{Now: now, WarnDays: 30, BackupStale: 48 * time.Hour}
	before := []HealthRow{EvaluateHealth(PBX{Name: "acme"}, healthyStatus(), o), EvaluateHealth(PBX{Name: "gone"}, healthyStatus(), o)}
	s := healthyStatus()
	s.Version = "20.0.11.100"
	s.TrunksRegistered = ptr(1)
	s.ExpirationDate = "2028-06-30T00:00:00Z"
	s.RecordingStopped = ptr(true) // a conditional check moving pass -> warn
	after := []HealthRow{EvaluateHealth(PBX{Name: "acme"}, s, o), ErrorHealthRow(PBX{Name: "new"}, errTest)}
	changes := DiffHealth(snap("health", true, []string{"acme", "gone"}, before, nil), snap("health", true, []string{"acme", "new"}, after, nil))
	var kinds []string
	for _, c := range changes {
		kinds = append(kinds, c.PBX+":"+c.Change)
	}
	got := strings.Join(kinds, " ")
	for _, want := range []string{"acme:version_changed", "acme:license_renewed", "acme:trunks_worsened", "acme:storage_quota_worsened", "new:added", "gone:removed"} {
		if !strings.Contains(got, want) {
			t.Errorf("changes %q missing %s", got, want)
		}
	}
	same := snap("health", true, []string{"acme", "gone"}, before, nil)
	if d := DiffHealth(same, same); len(d) != 0 {
		t.Fatalf("identical runs must not report changes: %+v", d)
	}
	// A narrower --only run must not fabricate removals or additions.
	narrow := snap("health", false, []string{"acme"}, before[:1], nil)
	if d := DiffHealth(same, narrow); len(d) != 0 {
		t.Fatalf("--only run fabricated changes: %+v", d)
	}
	if d := DiffHealth(narrow, same); len(d) != 0 {
		t.Fatalf("wider run after --only fabricated changes: %+v", d)
	}

	// Renewal is judged by the expiry date, not by days left.
	r1 := HealthRow{PBX: "acme", Status: Pass, LicenseExpires: "2026-11-01T00:00:00Z", LicenseDaysLeft: ptr(20)}
	r2 := HealthRow{PBX: "acme", Status: Pass, LicenseExpires: "2026-11-06T00:00:00Z", LicenseDaysLeft: ptr(15)}
	if d := DiffHealth(snap("health", true, []string{"acme"}, []HealthRow{r1}, nil), snap("health", true, []string{"acme"}, []HealthRow{r2}, nil)); len(d) != 1 || d[0].Change != "license_renewed" {
		t.Fatalf("five-day extension over ten days must count as a renewal: %+v", d)
	}

	tFrom := TrunkRows(PBX{Name: "acme"}, []Trunk{{Number: "1", IsOnline: ptr(true)}, {Number: "2", IsOnline: ptr(false)}}, now)
	tTo := TrunkRows(PBX{Name: "acme"}, []Trunk{{Number: "1", IsOnline: ptr(false)}, {Number: "2", IsOnline: ptr(true)}, {Number: "3", IsOnline: ptr(true)}}, now)
	acme := []string{"acme"}
	tc := DiffTrunks(snap("trunks", true, acme, nil, tFrom), snap("trunks", true, acme, nil, tTo))
	if len(tc) != 3 || tc[0].Change != "trunk_offline" || tc[1].Change != "trunk_online" || tc[2].Change != "trunk_added" {
		t.Fatalf("trunk changes = %+v", tc)
	}
	// An unreachable PBX must not report every trunk as removed.
	unreachable := []TrunkRow{ErrorTrunkRow(PBX{Name: "acme"}, errTest)}
	if c := DiffTrunks(snap("trunks", true, acme, nil, tFrom), snap("trunks", true, acme, nil, unreachable)); len(c) != 0 {
		t.Fatalf("error run must not fabricate removals: %+v", c)
	}
	// A PBX outside the later run's scope is not "removed".
	if c := DiffTrunks(snap("trunks", true, []string{"acme", "b"}, nil, tFrom), snap("trunks", false, []string{"b"}, nil, nil)); len(c) != 0 {
		t.Fatalf("out-of-scope PBX fabricated trunk removals: %+v", c)
	}

	hist := []Snapshot{{RunAt: now.Add(-48 * time.Hour), Kind: "health"}, {RunAt: now.Add(-20 * time.Hour), Kind: "health"}, {RunAt: now, Kind: "health"}, {RunAt: now, Kind: "trunks"}}
	from, to, ok := PickRuns(hist, "health", now.Add(-24*time.Hour), "")
	if !ok || !from.RunAt.Equal(now.Add(-48*time.Hour)) || !to.RunAt.Equal(now) {
		t.Fatalf("pick = %v %v %v", from.RunAt, to.RunAt, ok)
	}
	if _, _, ok := PickRuns(hist, "trunks", now, ""); ok {
		t.Fatal("one trunks run is not enough to diff")
	}
}

func TestHistoryFileSafety(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(DirEnv, dir)
	if h, err := LoadHistory(); err != nil || len(h) != 0 {
		t.Fatalf("missing history = %v %v", h, err)
	}
	if err := SaveSnapshot(Snapshot{RunAt: now, Kind: KindHealth, Full: true, Scope: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveSnapshot(Snapshot{RunAt: now.Add(time.Hour), Kind: KindTrunks, Scope: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	h, _ := LoadHistory()
	if len(h) != 2 || h[0].Kind != KindHealth {
		t.Fatalf("history after saves = %+v", h)
	}
	// An unreadable run is an error, never silently dropped.
	hd, _ := HistoryDir()
	entries, _ := os.ReadDir(hd)
	victim := filepath.Join(hd, entries[0].Name())
	before, err := os.Stat(victim)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" { // Windows has no unreadable mode bit
		if err := os.Chmod(victim, 0o000); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chmod(victim, 0o600) }()
		if os.Getuid() != 0 {
			if _, err := LoadHistory(); err == nil {
				t.Fatal("unreadable history must return an error")
			}
		}
	}
	// Saving never rewrites earlier runs.
	if err := SaveSnapshot(Snapshot{RunAt: now.Add(2 * time.Hour), Kind: KindHealth, Scope: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	// Size and modification time hold on every OS; the 0o000 mode is set (and
	// so checked) only where a mode bit exists.
	info, err := os.Stat(victim)
	if err != nil || info.Size() != before.Size() || !info.ModTime().Equal(before.ModTime()) ||
		runtime.GOOS != "windows" && info.Mode().Perm() != 0 {
		t.Fatalf("a save touched an earlier run: %v", err)
	}
}

func TestUnknownTrunkState(t *testing.T) {
	rows := TrunkRows(PBX{Name: "a"}, []Trunk{{Number: "1"}, {Number: "2", IsOnline: ptr(true), ConfigurationIssue: ptr("")}}, now)
	if rows[0].State != "unknown" || !rows[0].NeedsAttention() || rows[1].NeedsAttention() {
		t.Fatalf("states = %+v", rows)
	}
	SortTrunks(rows)
	if rows[0].Trunk != "1" {
		t.Fatalf("unknown should sort before online: %+v", rows)
	}
}

func TestRegistryValidateAndUpdate(t *testing.T) {
	dup := &Registry{PBXes: []PBX{
		{Name: "acme-west", FQDN: "a.com", ClientID: "1", EnvPrefix: "THREECX_ACME_WEST"},
		{Name: "acme_west", FQDN: "b.com", ClientID: "2", EnvPrefix: "THREECX_ACME_WEST"},
	}}
	if err := dup.Validate(); err == nil || !strings.Contains(err.Error(), "both use env prefix") {
		t.Fatalf("a shared env prefix must be rejected, got %v", err)
	}
	dir := t.TempDir()
	t.Setenv(DirEnv, dir)
	add := func(p PBX) error { return Update(func(r *Registry) error { r.Upsert(p); return nil }) }
	if err := add(PBX{Name: "a", FQDN: "a.com", ClientID: "1", EnvPrefix: "S_A"}); err != nil {
		t.Fatal(err)
	}
	if err := add(PBX{Name: "b", FQDN: "b.com", ClientID: "2", EnvPrefix: "S_A"}); err == nil {
		t.Fatal("Update must refuse a registry that shares a secret env")
	}
	reg, _ := Load()
	if len(reg.PBXes) != 1 {
		t.Fatalf("failed update must not be saved: %+v", reg.PBXes)
	}
	info, err := os.Stat(filepath.Join(dir, "fleet.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" { // Windows reports 0666: no group/other bits
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("fleet.json perms = %v", info.Mode())
		}
	}
}

func TestAudit(t *testing.T) {
	var g RoutingGraph
	raw := `{
	 "Peers":[{"Number":"100","Type":"Extension"},{"Number":"101","Type":"Extension"},{"Number":"800","Type":"RingGroup"},{"Number":"801","Type":"Queue"},{"Number":"802","Type":"IVR"}],
	 "Users":[{"Number":"100","Enabled":true},{"Number":"101","Enabled":false}],
	 "RingGroups":[{"Number":"800","Name":"Sales","Members":[{"Number":"100"},{"Number":"199"}],"ForwardNoAnswer":{"To":"Extension","Number":"101"}}],
	 "Queues":[{"Number":"801","Name":"Support","Agents":[]}],
	 "Receptionists":[{"Number":"802","Name":"Main","TimeoutForwardDN":"805","TimeoutForwardType":"Queue",
	   "Forwards":[{"Input":"1","ForwardType":"RingGroup","ForwardDN":"800"},{"Input":"9","ForwardType":"EndCall","ForwardDN":"999"}]}],
	 "InboundRules":[{"Data":"+15550100","RuleName":"Main DID","OfficeHoursDestination":{"To":"IVR","Number":"802"},
	   "OutOfOfficeHoursDestination":{"To":"External","Number":"+15559999"},"HolidaysDestination":{"To":"VoiceMail","Number":"177"}}]
	}`
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		t.Fatal(err)
	}
	findings := Audit("acme", g)
	var got []string
	for _, f := range findings {
		got = append(got, f.ObjectType+"/"+f.Field+"/"+f.Target+"/"+f.Problem)
	}
	want := []string{
		"inbound_rule/HolidaysDestination/177/missing",
		"ivr/TimeoutForwardDN/805/missing",
		"queue/Agents//empty",
		"ring_group/ForwardNoAnswer/101/disabled",
		"ring_group/Members/199/missing",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if f := Audit("x", RoutingGraph{}); len(f) != 0 {
		t.Fatalf("empty graph must have no findings: %+v", f)
	}
}

func TestScrubIsolation(t *testing.T) {
	// Built the way every registry is (Load validates, add upserts), which
	// derives each SecretVar from its prefix.
	reg := &Registry{}
	reg.Upsert(PBX{Name: "acme", EnvPrefix: "THREECX_ACME"})
	reg.Upsert(PBX{Name: "globex", EnvPrefix: "GLOBEX", SecretVar: "THREECX_CLIENT_SECRET"})
	if reg.PBXes[1].SecretVar != "GLOBEX_CLIENT_SECRET" {
		t.Fatalf("SecretVar must be derived from the prefix, got %q", reg.PBXes[1].SecretVar)
	}
	env := []string{"PATH=/bin", "THREECX_CLIENT_SECRET=leak", "THREECX_BASE_URL=http://x", "THREECX_DATA_DIR=/d",
		"THREECX_USER_AGENT=ua-leak", "THREECX_OAUTH_SCOPE=scope-leak", "THREECX_ACME_CLIENT_SECRET=keep", "GLOBEX_CLIENT_SECRET=other-leak",
		"THREECX_PBX=other", "THREECX_NO_LEARN=1"}
	got := strings.Join(Scrub(env, reg, reg.PBXes[0]), " ")
	for _, gone := range []string{"leak", "http://x", "/d", "THREECX_PBX=other"} {
		if strings.Contains(got, gone) {
			t.Errorf("scrubbed env leaked %q: %s", gone, got)
		}
	}
	for _, kept := range []string{"PATH=/bin", "THREECX_ACME_CLIENT_SECRET=keep", "THREECX_NO_LEARN=1"} {
		if !strings.Contains(got, kept) {
			t.Errorf("scrubbed env missing %q: %s", kept, got)
		}
	}
	if childError("warning: x\nError: GET /SystemStatus returned HTTP 403\nhint: y", errTest) != "GET /SystemStatus returned HTTP 403" {
		t.Fatal("childError should keep the first real error line")
	}
	// THREECX would read THREECX_CLIENT_SECRET, the connector's own variable.
	for _, prefix := range []string{"THREECX", "PRINTING_PRESS", "PRINTING_PRESS_VERIFY"} {
		if err := (PBX{Name: "a", FQDN: "a.com", ClientID: "1", EnvPrefix: prefix}).Validate(); err == nil {
			t.Errorf("env_prefix %s must be reserved", prefix)
		}
	}
	for _, prefix := range []string{"ACME_3CX", "THREECX_ACME"} {
		if err := (PBX{Name: "a", FQDN: "a.com", ClientID: "1", EnvPrefix: prefix}).Validate(); err != nil {
			t.Errorf("env_prefix %s must be allowed: %v", prefix, err)
		}
	}
	q := Request{Path: "/Users", Params: map[string]string{"$select": "Number,Name", "$filter": "Enabled eq true"}}
	back, err := DecodeRequest(q.Encode())
	if err != nil || back.Path != "/Users" || back.Params["$select"] != "Number,Name" || back.Params["$filter"] != "Enabled eq true" {
		t.Fatalf("request round trip = %+v %v", back, err)
	}
}

func TestIdentityHomesAndRemoval(t *testing.T) {
	base := t.TempDir()
	a := PBX{Name: "acme", FQDN: "acme.3cx.us", ClientID: "900"}
	b := a
	b.FQDN = "other.3cx.us"
	c := a
	c.ClientID = "901"
	if a.Home(base) == b.Home(base) || a.Home(base) == c.Home(base) || filepath.Dir(a.Home(base)) != PBXRoot(base, "acme") {
		t.Fatal("homes must differ per FQDN and client ID under one root")
	}
	t.Setenv(DirEnv, base)
	if err := Update(func(r *Registry) error {
		r.Upsert(PBX{Name: "acme", FQDN: "acme.3cx.us", ClientID: "900", EnvPrefix: "S_A"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(a.Home(base), 0o700)
	_ = os.MkdirAll(b.Home(base), 0o700)
	if err := PruneHomes(base, a); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.Home(base)); !os.IsNotExist(err) {
		t.Fatal("PruneHomes must remove other identities")
	}
	if _, err := os.Stat(a.Home(base)); err != nil {
		t.Fatal("PruneHomes must keep the current identity")
	}
	if found, err := RemovePBX("acme"); err != nil || !found {
		t.Fatalf("RemovePBX = %v %v", found, err)
	}
	if _, err := os.Stat(PBXRoot(base, "acme")); !os.IsNotExist(err) {
		t.Fatal("RemovePBX must delete the name's homes")
	}
	if found, _ := RemovePBX("acme"); found {
		t.Fatal("second remove must report not found")
	}
}

func TestDiffEdgeCases(t *testing.T) {
	o := HealthOptions{Now: now, WarnDays: 30, BackupStale: 48 * time.Hour}
	// Inactive licence that gets renewed still reports the renewal.
	s := healthyStatus()
	s.LicenseActive = ptr(false)
	s.ExpirationDate = "2026-09-01T00:00:00Z"
	old := EvaluateHealth(PBX{Name: "acme"}, s, o)
	s.LicenseActive = ptr(true)
	s.ExpirationDate = "2027-09-01T00:00:00Z"
	cur := EvaluateHealth(PBX{Name: "acme"}, s, o)
	d := DiffHealth(snap("health", true, []string{"acme"}, []HealthRow{old}, nil), snap("health", true, []string{"acme"}, []HealthRow{cur}, nil))
	got := ""
	for _, c := range d {
		got += c.Change + " "
	}
	if !strings.Contains(got, "license_renewed") {
		t.Fatalf("renewal of an inactive licence missed: %q", got)
	}
	// A check whose telemetry disappeared is not a recovery.
	s = healthyStatus()
	s.TrunksRegistered = ptr(0)
	withTrunks := EvaluateHealth(PBX{Name: "acme"}, s, o)
	s.TrunksRegistered, s.TrunksTotal = nil, nil
	noTrunks := EvaluateHealth(PBX{Name: "acme"}, s, o)
	if d := DiffHealth(snap("health", true, []string{"acme"}, []HealthRow{withTrunks}, nil), snap("health", true, []string{"acme"}, []HealthRow{noTrunks}, nil)); len(d) != 1 || d[0].Change != "trunks_unknown" {
		t.Fatalf("missing telemetry must read as trunks_unknown, never improved: %+v", d)
	}
	// Legacy snapshots without a scope still compare their PBXes.
	legacyFrom := Snapshot{Kind: "health", Health: []HealthRow{withTrunks}}
	legacyTo := Snapshot{Kind: "health", Health: []HealthRow{EvaluateHealth(PBX{Name: "acme"}, healthyStatus(), o)}}
	if d := DiffHealth(legacyFrom, legacyTo); len(d) == 0 {
		t.Fatal("legacy snapshots without scope must still be compared")
	}
	// Registration and configuration issue move independently.
	from := TrunkRows(PBX{Name: "acme"}, []Trunk{{Number: "1", IsOnline: ptr(true), ConfigurationIssue: ptr("bad CID")}}, now)
	to := TrunkRows(PBX{Name: "acme"}, []Trunk{{Number: "1", IsOnline: ptr(false), ConfigurationIssue: ptr("bad CID")}}, now)
	tc := DiffTrunks(snap("trunks", true, []string{"acme"}, nil, from), snap("trunks", true, []string{"acme"}, nil, to))
	if len(tc) != 1 || tc[0].Change != "trunk_offline" {
		t.Fatalf("online -> offline with the same issue = %+v", tc)
	}
	to = TrunkRows(PBX{Name: "acme"}, []Trunk{{Number: "1", IsOnline: ptr(true), ConfigurationIssue: ptr("")}}, now)
	tc = DiffTrunks(snap("trunks", true, []string{"acme"}, nil, from), snap("trunks", true, []string{"acme"}, nil, to))
	if len(tc) != 1 || tc[0].Change != "trunk_issue_cleared" {
		t.Fatalf("issue cleared = %+v", tc)
	}
}

func TestNullTelemetryAndBackupFiles(t *testing.T) {
	o := HealthOptions{Now: now, WarnDays: 30, BackupStale: 48 * time.Hour}
	s := SystemStatus{LastBackupDateTime: "2026-10-14T02:00:00Z"} // services, quotas, licence: all null
	row := EvaluateHealth(PBX{Name: "a"}, s, o)
	seen := map[string]bool{}
	for _, c := range row.Checks {
		switch c.Name {
		case "services", "storage_quota", "system_extensions", "phone_firmware", "license":
			seen[c.Name] = true
			if c.Level != Unknown {
				t.Errorf("check %s must be unknown for null telemetry: %+v", c.Name, c)
			}
		}
	}
	if len(seen) != 5 || row.Status == Pass {
		t.Errorf("null telemetry must show as unknown checks, never a pass: %+v", row)
	}
	// A completed backup with no backup file left to restore is flagged.
	r := BackupRowFor(PBX{Name: "a"}, SystemStatus{LastBackupDateTime: "2026-10-14T02:00:00Z"}, BackupSettings{ScheduleEnabled: ptr(true)}, nil, 48*time.Hour, now)
	if r.Status == "ok" || strings.Join(r.Flags, ",") != "NO_FILES" {
		t.Fatalf("backup with no files = %+v", r)
	}
}

func TestRedactRecordsAndReservedSecretEnv(t *testing.T) {
	red, _ := RedactJSON(json.RawMessage(`{"value":[{"Name":"CLIENT_SECRET","Value":"s1"},{"Name":"Region","Value":"eu"}],
		"Location":{"GbJson":{"Contents":"{json-key}"}},"BackupContents":{"Contents":{"Recordings":true}}}`))
	if strings.Contains(string(red), "s1") || strings.Contains(string(red), "json-key") {
		t.Fatalf("leaked: %s", red)
	}
	if !strings.Contains(string(red), `"eu"`) || !strings.Contains(string(red), `"Recordings":true`) {
		t.Fatalf("over-redacted: %s", red)
	}
	for _, prefix := range []string{"THREECX", "THREECX_", "_ACME", "threecx_acme", "ACME-PBX", "ACME KEY", ""} {
		p := PBX{Name: "a", FQDN: "a.com", ClientID: "1", EnvPrefix: prefix}
		if err := p.Validate(); err == nil {
			t.Errorf("env_prefix %q must be refused", prefix)
		}
	}
}

func TestRoundFourEdgeCases(t *testing.T) {
	// A projection that drops Name cannot be judged, so Value is masked;
	// a non-secret named record keeps its value.
	red, _ := RedactJSON(json.RawMessage(`{"value":[{"Value":"secret-1"},{"Name":"Region","Value":"eu"}]}`))
	if strings.Contains(string(red), "secret-1") || !strings.Contains(string(red), `"eu"`) {
		t.Fatalf("projection redaction = %s", red)
	}
	prop, _ := RedactJSON(json.RawMessage(`{"@odata.context":"x/$metadata#Parameters(1)/Value","value":"secret-2"}`))
	if strings.Contains(string(prop), "secret-2") {
		t.Fatalf("property read leaked: %s", prop)
	}
	coll, _ := RedactJSON(json.RawMessage(`{"value":["PCMU","G722"]}`))
	if !strings.Contains(string(coll), "G722") {
		t.Fatalf("scalar collection must survive: %s", coll)
	}

	o := HealthOptions{Now: now, WarnDays: 30, BackupStale: 48 * time.Hour}
	// Partial quota telemetry is unknown, so no false recovery.
	s := healthyStatus()
	s.RecordingStopped = nil
	for _, c := range EvaluateHealth(PBX{Name: "a"}, s, o).Checks {
		if c.Name == "storage_quota" && c.Level != Unknown {
			t.Fatalf("partial quota telemetry must be unknown: %+v", c)
		}
	}
	// Null free space is unknown, not 0% free.
	s = healthyStatus()
	s.FreeDiskSpace = nil
	for _, c := range EvaluateHealth(PBX{Name: "a"}, s, o).Checks {
		if c.Name == "disk" && c.Level != Unknown {
			t.Fatalf("null free space must be an unknown disk verdict: %+v", c)
		}
	}
	// Recent completion but the newest remaining file is a week old.
	files := []Backup{{FileName: "old.zip", CreationTime: "2026-10-07T02:00:00Z", Size: ptr(int64(100))}}
	r := BackupRowFor(PBX{Name: "a"}, SystemStatus{LastBackupDateTime: "2026-10-14T02:00:00Z"}, BackupSettings{ScheduleEnabled: ptr(true)}, files, 48*time.Hour, now)
	if r.Status == "ok" || !strings.Contains(strings.Join(r.Flags, ","), "FILE_STALE") {
		t.Fatalf("deleted newest backup not flagged: %+v", r)
	}
}

func TestPickRunsForPBX(t *testing.T) {
	hist := []Snapshot{
		{RunAt: now.Add(-3 * time.Hour), Kind: "health", Scope: []string{"acme", "globex"}},
		{RunAt: now.Add(-2 * time.Hour), Kind: "health", Scope: []string{"acme"}},
		{RunAt: now.Add(-1 * time.Hour), Kind: "health", Scope: []string{"globex"}},
	}
	from, to, ok := PickRuns(hist, "health", now, "acme")
	if !ok || !from.RunAt.Equal(now.Add(-3*time.Hour)) || !to.RunAt.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("acme runs = %v %v %v", from.RunAt, to.RunAt, ok)
	}
	if _, _, ok := PickRuns(hist[1:], "health", now, "acme"); ok {
		t.Fatal("one acme run is not enough")
	}
}

func TestRedactionKeepsRoutingFieldsForAudit(t *testing.T) {
	raw := json.RawMessage(`[{"value":[{"Number":"100","Type":"Extension"},{"Number":"802","Type":"IVR"}]},
		{"value":[{"Number":"100","Enabled":true}]},{"value":[]},{"value":[]},
		{"value":[{"Number":"802","Name":"Main","InvalidKeyForwardDN":"100","TimeoutForwardDN":"","Forwards":[]}]},{"value":[]}]`)
	var bodies []json.RawMessage
	if err := json.Unmarshal(raw, &bodies); err != nil {
		t.Fatal(err)
	}
	for i := range bodies {
		red, err := RedactJSON(bodies[i])
		if err != nil {
			t.Fatal(err)
		}
		bodies[i] = red
	}
	var g RoutingGraph
	targets := []any{&g.Peers, &g.Users, &g.RingGroups, &g.Queues, &g.Receptionists, &g.InboundRules}
	for i, tg := range targets {
		var page struct {
			Value json.RawMessage `json:"value"`
		}
		_ = json.Unmarshal(bodies[i], &page)
		if err := json.Unmarshal(page.Value, tg); err != nil {
			t.Fatal(err)
		}
	}
	if f := Audit("acme", g); len(f) != 0 {
		t.Fatalf("redaction broke routing fields; audit reported %+v", f)
	}
	for _, field := range []string{"InvalidKeyForwardDN", "NextPageToken", "TokenEndpoint", "SftpHostKeyFingerprint"} {
		if SecretField(field) {
			t.Errorf("%s must not be redacted", field)
		}
	}
	for _, field := range []string{"AuthPassword", "LicenseKey", "PinNumber", "TranscribeKeyGrok", "ProvLink", "AccessKey"} {
		if !SecretField(field) {
			t.Errorf("%s must be redacted", field)
		}
	}
}

func TestVersionErrorRowOmitsVerdicts(t *testing.T) {
	b, err := json.Marshal(VersionRow{PBX: "down", Error: "HTTP 403"})
	if err != nil || string(b) != `{"pbx":"down","error":"HTTP 403"}` {
		t.Fatalf("error row = %s %v", b, err)
	}
	b, _ = json.Marshal(VersionRow{PBX: "acme", Version: "20.0.10", Behind: ptr(false), MaintenanceExpired: ptr(false)})
	if !strings.Contains(string(b), `"behind":false`) || !strings.Contains(string(b), `"pending_updates":0`) {
		t.Fatalf("readable row must keep its verdicts: %s", b)
	}
}

// A fleet file cannot name the variable a PBX's secret is read from: SecretVar
// is never read from disk, only derived from env_prefix.
func TestSecretVarIsDerivedNotLoaded(t *testing.T) {
	t.Setenv(DirEnv, t.TempDir())
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"pbxes":[{"name":"acme","fqdn":"acme.3cx.us","client_id":"900","env_prefix":"THREECX_ACME","SecretVar":"THREECX_CLIENT_SECRET","secret_env":"THREECX_CLIENT_SECRET"}]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.PBXes[0].SecretVar; got != "THREECX_ACME_CLIENT_SECRET" {
		t.Fatalf("SecretVar = %q, want THREECX_ACME_CLIENT_SECRET", got)
	}
}
