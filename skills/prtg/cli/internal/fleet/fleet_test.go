// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) string { return now.Add(-d).Format(time.RFC3339) }

func TestValidateServer(t *testing.T) {
	good := []string{"prtg.acme.example", "prtg.acme.example:1616", "10.0.0.5:1616", "core-1"}
	bad := []string{"", "https://prtg.acme.example", "prtg.acme.example/api", "user@host", "host:", "host:99999", "-bad.example", "a b"}
	for _, s := range good {
		if err := ValidateServer(s); err != nil {
			t.Errorf("ValidateServer(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range bad {
		if err := ValidateServer(s); err == nil {
			t.Errorf("ValidateServer(%q) = nil, want error", s)
		}
	}
}

func TestCoreValidate(t *testing.T) {
	ok := Core{Name: "acme-west.1", Server: "prtg.acme.example:1616"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []Core{
		{Name: "Acme", Server: "h"},
		{Name: "a..b", Server: "h"},
		{Name: "acme", Server: "https://h"},
		{Name: "acme", Server: "h", Tags: []string{"Bad Tag"}},
	} {
		if err := c.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want error", c)
		}
	}
}

func TestRegistryValidateAndSelect(t *testing.T) {
	r := &Registry{Cores: []Core{
		{Name: "acme", Server: "a.example", Tags: []string{"gold"}},
		{Name: "globex", Server: "g.example"},
	}}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	all, err := r.Select(nil, nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("Select(all) = %v, %v", all, err)
	}
	gold, err := r.Select(nil, []string{"gold"})
	if err != nil || len(gold) != 1 || gold[0].Name != "acme" {
		t.Fatalf("Select(tag gold) = %v, %v", gold, err)
	}
	if _, err := r.Select([]string{"nope"}, nil); err == nil {
		t.Fatal("unknown core must be an error, not a smaller report")
	}
	if _, err := r.Select(nil, []string{"silver"}); err == nil {
		t.Fatal("unknown tag must be an error")
	}
	dup := &Registry{Cores: []Core{
		{Name: "a", Server: "a.example"},
		{Name: "a", Server: "b.example"},
	}}
	if err := dup.Validate(); err == nil {
		t.Fatal("two cores sharing one name (and so one PRTG_FLEET_KEYS entry) must be rejected")
	}
	if !r.Upsert(Core{Name: "acme", Server: "a2.example"}) || len(r.Cores) != 2 {
		t.Fatal("Upsert should replace by name")
	}
	if !r.Remove("globex") || r.Remove("globex") {
		t.Fatal("Remove should delete once")
	}
}

func TestHomeChangesWithIdentity(t *testing.T) {
	a := Core{Name: "acme", Server: "a.example"}
	b := a
	b.Server = "b.example"
	if a.Home("/base") == b.Home("/base") {
		t.Fatal("re-pointing a core at another server must land in a fresh home")
	}
	// Two cores on one server never share a store: the name is part of the
	// identity hash as well as of the path.
	other := Core{Name: "globex", Server: "a.example"}
	if filepath.Base(a.Home("/base")) == filepath.Base(other.Home("/base")) || a.Home("/base") == other.Home("/base") {
		t.Fatal("two cores on one server must not share a home")
	}
	if !strings.HasPrefix(a.Home("/base"), filepath.Join("/base", "cores", "acme")+string(filepath.Separator)) {
		t.Fatalf("Home = %s", a.Home("/base"))
	}
	if a.BaseURL() != "https://a.example/api/v2" {
		t.Fatalf("BaseURL = %s", a.BaseURL())
	}
}

func TestScrub(t *testing.T) {
	env := []string{"PATH=/bin", "PRTG_API_KEY=x", "PRTG_SERVER=evil", "PRTG_TOKEN_ACME=a", "PRTG_SKIP_TLS_VERIFY=1", "PRTG_CONFIG=/tmp/c",
		DirEnv + "=/fleet", "PRTG_FLEET_KEYS=acme=a,globex=g"}
	got := strings.Join(Scrub(env, Core{Name: "acme"}, "a"), " ")
	for _, want := range []string{"PATH=/bin", "PRTG_SKIP_TLS_VERIFY=1", DirEnv + "=/fleet"} {
		if !strings.Contains(got, want) {
			t.Errorf("Scrub dropped %s: %s", want, got)
		}
	}
	for _, gone := range []string{"PRTG_API_KEY", "PRTG_SERVER", "PRTG_TOKEN_ACME", "PRTG_CONFIG"} {
		if strings.Contains(got, gone+"=") {
			t.Errorf("Scrub kept %s: %s", gone, got)
		}
	}
}

func TestRequestEncodeDecode(t *testing.T) {
	cases := []Request{
		{Path: "/sensors/alarms", Params: map[string]string{"filter": `name = "a b"`}},
		{Method: MethodGetAll, Path: "/sensors", Params: map[string]string{"limit": "3000"}},
		{Method: MethodPost, Path: "/sensors/acknowledge"},
	}
	for _, q := range cases {
		got, err := DecodeRequest(q.Encode())
		if err != nil {
			t.Fatal(err)
		}
		want := q.Method
		if want == "" {
			want = MethodGet
		}
		if got.Method != want || got.Path != q.Path || len(got.Params) != len(q.Params) {
			t.Errorf("round trip %+v -> %+v", q, got)
		}
		for k, v := range q.Params {
			if got.Params[k] != v {
				t.Errorf("param %s = %q, want %q", k, got.Params[k], v)
			}
		}
	}
	if _, err := DecodeRequest("DELETE /sensors"); err == nil {
		t.Fatal("unsupported method must be rejected")
	}
}

func TestErrorEnvelope(t *testing.T) {
	e := ErrorBody(403, "forbidden")
	if got, ok := RequestError(e); !ok || got.Status != 403 || got.Message != "forbidden" {
		t.Fatalf("RequestError = %+v %v", got, ok)
	}
	if _, ok := RequestError(json.RawMessage(`{"down":1}`)); ok {
		t.Fatal("a normal body is not an error")
	}
	if _, all := AllFailed([]json.RawMessage{e, json.RawMessage(`[]`)}); all {
		t.Fatal("one success means the core was reached")
	}
	if msg, all := AllFailed([]json.RawMessage{e, ErrorBody(0, "dial")}); !all || msg != "forbidden" {
		t.Fatalf("AllFailed = %q %v", msg, all)
	}
}

func TestRedact(t *testing.T) {
	body := json.RawMessage(`{"key":"LIC-123","edition":"PRTG 500","windowsconnection":{"windowsloginpassword":"secret","windowsloginusername":"bob"},"apikey":"k","password_complexity":true,"users":[{"token":"t"}]}`)
	out, err := RedactJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, leaked := range []string{"LIC-123", `"secret"`, `"k"`, `"t"`} {
		if strings.Contains(s, leaked) {
			t.Errorf("redaction leaked %s: %s", leaked, s)
		}
	}
	for _, kept := range []string{"PRTG 500", "bob", `"password_complexity":true`} {
		if !strings.Contains(s, kept) {
			t.Errorf("redaction removed %s: %s", kept, s)
		}
	}
	if !SecretField("snmpauthpass") || SecretField("name") {
		t.Fatal("SecretField classification")
	}
}

func TestItemsAndStr(t *testing.T) {
	if got := Items(json.RawMessage(`[{"name":"a"},{"name":"b"}]`)); len(got) != 2 || Str(got[1], "name") != "b" {
		t.Fatalf("Items(array) = %v", got)
	}
	if got := Items(json.RawMessage(`{"prtg_version":"26.3"}`)); len(got) != 1 {
		t.Fatalf("Items(object) = %v", got)
	}
	if got := Items(json.RawMessage(`null`)); len(got) != 0 {
		t.Fatalf("Items(null) = %v", got)
	}
	if Str(map[string]any{"n": 1}, "n") != "" {
		t.Fatal("Str on non-string")
	}
}

func TestParseTime(t *testing.T) {
	if _, ok := ParseTime("0001-01-01T00:00:00Z"); ok {
		t.Fatal("year-1 placeholder must read as unset")
	}
	if tm, ok := ParseTime("2026-10-15T12:00:00Z"); !ok || !tm.Equal(now) {
		t.Fatalf("ParseTime = %v %v", tm, ok)
	}
	if _, ok := ParseTime(""); ok {
		t.Fatal("empty must be unset")
	}
}

func alarmNodes() []Node {
	dev := func(name string) []Ref {
		return []Ref{{Name: "Root", Type: "REFERENCED_ROOT"}, {Name: "Probe", Type: "REFERENCED_PROBE"}, {ID: "dev-" + name, Name: name, Type: "REFERENCED_DEVICE"}}
	}
	return []Node{
		{ID: "1", Name: "Ping", Status: "DOWN", StatusSince: ago(3 * time.Hour), Priority: 5, Path: dev("fw01")},
		{ID: "2", Name: "HTTP", Status: "DOWN", StatusSince: ago(5 * time.Minute), Priority: 3, Path: dev("fw01")},
		{ID: "3", Name: "Disk", Status: "WARNING", StatusSince: ago(30 * time.Hour), Priority: 4, Path: dev("db")},
		{ID: "4", Name: "CPU", Status: "DOWN_ACKNOWLEDGED", StatusSince: ago(240 * time.Hour), Priority: 3, Path: dev("app"),
			StatusInfo: &StatusInfo{Info: &NodeStatusInfo{Message: "TKT-1", User: &Ref{Name: "rosa"}}}},
	}
}

func TestParseAlarmStatuses(t *testing.T) {
	set, err := ParseAlarmStatuses([]string{"down"})
	if err != nil || !set["DOWN"] || !set["DOWN_PARTIAL"] || set["WARNING"] {
		t.Fatalf("down set = %v %v", set, err)
	}
	all, _ := ParseAlarmStatuses(nil)
	if len(all) != 5 {
		t.Fatalf("default set = %v", all)
	}
	if _, err := ParseAlarmStatuses([]string{"red"}); err == nil {
		t.Fatal("unknown word must error")
	}
}

func TestAlarmRowsFiltersAndSort(t *testing.T) {
	all, _ := ParseAlarmStatuses(nil)
	rows := AlarmRows("acme", alarmNodes(), AlarmOptions{Statuses: all, Now: now})
	rows = append(rows, AlarmRow{Core: "dead", Error: "dial"})
	SortAlarms(rows)
	order := []string{}
	for _, r := range rows {
		order = append(order, r.ID+r.Error)
	}
	if strings.Join(order, ",") != "1,2,3,4,dial" {
		t.Fatalf("order = %v (want down by priority, warning, acknowledged, errors last)", order)
	}
	if rows[3].AckedBy != "rosa" || rows[3].AckMessage != "TKT-1" || rows[0].Device != "fw01" {
		t.Fatalf("row fields = %+v / %+v", rows[3], rows[0])
	}
	unacked := AlarmRows("acme", alarmNodes(), AlarmOptions{Statuses: all, Unacked: true, MinAge: time.Hour, Now: now})
	for _, r := range unacked {
		if r.Status == "DOWN_ACKNOWLEDGED" || r.ID == "2" {
			t.Fatalf("--unacked/--min-age kept %+v", r)
		}
	}
	none := AlarmRows("acme", []Node{{ID: "9", Status: "UP"}}, AlarmOptions{Statuses: all, Now: now})
	if len(none) != 0 {
		t.Fatalf("an UP sensor is not an alarm: %v", none)
	}
}

func TestRollupByDevice(t *testing.T) {
	all, _ := ParseAlarmStatuses(nil)
	nodes := append(alarmNodes(),
		// Same display name as fw01, different device: must not merge.
		Node{ID: "7", Name: "Ping", Status: "DOWN", StatusSince: ago(time.Hour), Path: []Ref{{ID: "other-fw", Name: "fw01", Type: "REFERENCED_DEVICE"}}},
		// No device in the path: stays its own row.
		Node{ID: "8", Name: "Probe Health", Status: "WARNING", StatusSince: ago(time.Hour)})
	rows := RollupByDevice(AlarmRows("acme", nodes, AlarmOptions{Statuses: all, Now: now}))
	if len(rows) != 5 {
		t.Fatalf("rollup rows = %d (%+v), want 5", len(rows), rows)
	}
	fw := rows[0]
	if fw.Device != "fw01" || fw.DeviceID != "dev-fw01" || fw.Sensors != 2 || fw.Priority != 5 || fw.StatusSince != ago(3*time.Hour) {
		t.Fatalf("fw01 rollup = %+v", fw)
	}
	for _, r := range rows {
		if r.DeviceID == "other-fw" && r.Sensors != 1 {
			t.Fatalf("a second device named fw01 was merged: %+v", r)
		}
	}
}

func TestStatusRowsAndHealth(t *testing.T) {
	c := Core{Name: "acme", Server: "a"}
	row := StatusRowFor(c, json.RawMessage(`null`), json.RawMessage(`{"down":2,"down_partial":1,"warning":3,"paused_by_user":1,"paused_by_schedule":2,"total":20}`))
	if row.Health != "ok" || *row.Down != 3 || *row.Paused != 3 || *row.Total != 20 {
		t.Fatalf("status row = %+v", row)
	}
	bad := StatusRowFor(c, ErrorBody(503, `GET /health returned HTTP 503: {"code":"LICENSE_INACTIVE","message":"x"}`), ErrorBody(401, "auth"))
	if bad.Health != "LICENSE_INACTIVE" || bad.Error != "auth" || bad.Down != nil {
		t.Fatalf("bad row = %+v", bad)
	}
	if HealthState(ErrorBody(502, "no json")) != "HTTP 502" || HealthState(ErrorBody(0, "dial")) != "unreachable" {
		t.Fatal("HealthState fallbacks")
	}
	rows := []StatusRow{{Core: "b", Down: ip(1)}, {Core: "a", Down: ip(5)}, {Core: "z", Error: "x"}}
	SortStatus(rows)
	if rows[0].Core != "z" || rows[1].Core != "a" {
		t.Fatalf("SortStatus = %+v", rows)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"26.3.124.1727", "25.4.114.1032", 1},
		{"25.4.114", "25.4.114.0", 0},
		{"24.3.100", "24.3.102", -1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%s,%s) = %d", c.a, c.b, got)
		}
	}
}

func TestDrift(t *testing.T) {
	n := func(v int) *int { return &v }
	facts := []CoreFacts{
		{Core: "acme", Health: "ok", Version: &Versions{PRTGVersion: "26.3.124"}, SensorsInUse: n(900),
			License: &License{MaintenanceEndDate: now.Add(10 * 24 * time.Hour).Format(time.RFC3339), SensorCount: n(1000), LicenseSensorCount: n(1000)},
			Probes:  []Node{{Name: "Local", ProbeStatus: "CONNECTED"}, {Name: "Branch", ProbeStatus: "DISCONNECTED"}},
			Errors:  map[string]string{}},
		{Core: "globex", Health: "LICENSE_INACTIVE", Version: &Versions{PRTGVersion: "25.4.114"}, SensorsInUse: n(520),
			License: &License{EndDate: now.Add(-48 * time.Hour).Format(time.RFC3339), SensorCount: n(500), LicenseSensorCount: n(500)},
			Errors:  map[string]string{"probes": "403 forbidden"}},
		{Core: "dead", Unread: true, ReadErr: "dial tcp: refused"},
		{Core: "calm", Health: "ok", Version: &Versions{PRTGVersion: "26.3.124"},
			License: &License{EndDate: now.Add(400 * 24 * time.Hour).Format(time.RFC3339), UnlimitedSensors: true, SensorCount: n(9999), LicenseSensorCount: n(1)}},
		// Overusage enabled but below the licensed count: not overuse.
		{Core: "roomy", Health: "ok", Version: &Versions{PRTGVersion: "26.3.124"},
			SensorsInUse: n(100), License: &License{OverusageEnabled: true, SensorCount: n(500), LicenseSensorCount: n(500)}},
		// Over the base count but inside the absolute overusage limit: not overuse.
		{Core: "stretch", Health: "ok", Version: &Versions{PRTGVersion: "26.3.124"},
			SensorsInUse: n(600), License: &License{OverusageEnabled: true, OverusageLimit: n(1000), SensorCount: n(500), LicenseSensorCount: n(500)}},
		{Core: "useunl", Health: "ok", Version: &Versions{PRTGVersion: "26.3.124"},
			SensorsInUse: n(9000), License: &License{UseUnlimited: true, SensorCount: n(500), LicenseSensorCount: n(500)}},
		{Core: "badkey", Health: "ok", AuthFailed: true, Errors: map[string]string{}},
		// An unreadable /version leaves Version nil: no version finding.
		{Core: "noversion", Health: "ok", Errors: map[string]string{"version": "unexpected /version body"}},
	}
	got := map[string]bool{}
	for _, f := range Drift(facts, DriftOptions{Within: 30 * 24 * time.Hour, Now: now}) {
		got[f.Core+"/"+f.Check+"/"+f.Severity] = true
		if f.Core == "calm" || f.Core == "roomy" || f.Core == "stretch" || f.Core == "useunl" {
			t.Errorf("healthy core produced a finding: %+v", f)
		}
		if f.Core == "noversion" && f.Check == "version" && f.Severity != "info" {
			t.Errorf("missing version reported as behind: %+v", f)
		}
		if f.Check == "maintenance_end" && !strings.Contains(f.Detail, "in 10 days") {
			t.Errorf("days left = %q", f.Detail)
		}
	}
	for _, want := range []string{"acme/probe/critical", "acme/maintenance_end/warning", "globex/health/critical",
		"globex/version/warning", "globex/license_end/critical", "globex/sensor_overuse/warning", "globex/probes/info", "dead/unreachable/critical",
		"badkey/auth/critical", "noversion/version/info"} {
		if !got[want] {
			t.Errorf("missing finding %s (got %v)", want, got)
		}
	}
	if got["acme/sensor_overuse/warning"] || got["acme/version/warning"] {
		t.Errorf("false findings for acme: %v", got)
	}
}

func TestHandoff(t *testing.T) {
	nodes := []Node{
		{ID: "1", Name: "Ping", Status: "DOWN", StatusSince: ago(2 * time.Hour)},
		{ID: "2", Name: "Old", Status: "DOWN", StatusSince: ago(48 * time.Hour)},
		// Went down before the window, came back inside it: a recovery at status_since.
		{ID: "3", Name: "Mem", Status: "UP", StatusSince: ago(3 * time.Hour), LastDown: ago(4 * time.Hour)},
		// Resumed after long maintenance: a recovery after a gap. A brand-new
		// sensor (no recorded down) is not a recovery.
		{ID: "10", Name: "Resumed", Status: "UP", StatusSince: ago(time.Hour), LastDown: ago(400 * 24 * time.Hour)},
		{ID: "11", Name: "New", Status: "UP", StatusSince: ago(time.Hour)},
		{ID: "4", Name: "Steady", Status: "UP", StatusSince: ago(100 * time.Hour), LastDown: ago(101 * time.Hour)},
		{ID: "5", Name: "CPU", Status: "DOWN_ACKNOWLEDGED", StatusInfo: &StatusInfo{Info: &NodeStatusInfo{TimeStamp: ago(time.Hour), User: &Ref{Name: "rosa"}, Message: "TKT"}}},
		{ID: "9", Name: "Disk", Status: "DOWN_ACKNOWLEDGED", StatusInfo: &StatusInfo{Info: &NodeStatusInfo{TimeStamp: ago(5 * time.Hour), AutoAcknowledged: true}}},
		{ID: "6", Name: "Bak", Status: "PAUSED_BY_USER", StatusSince: ago(4 * time.Hour)},
	}
	rows := Handoff("acme", nodes, now.Add(-12*time.Hour), now)
	SortHandoff(rows)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.ID+":"+r.Event)
	}
	if strings.Join(got, ",") != "5:acknowledged,10:recovered,1:new_alarm,3:recovered,6:paused,9:acknowledged" {
		t.Fatalf("handoff = %v", got)
	}
	if rows[0].By != "rosa" || rows[0].Message != "TKT" || rows[0].Auto {
		t.Fatalf("ack row = %+v", rows[0])
	}
	if rows[3].At != ago(3*time.Hour) || rows[3].LastDown != ago(4*time.Hour) {
		t.Fatalf("recovery must be timed by status_since: %+v", rows[3])
	}
	if rows[1].Via != "after_gap" {
		t.Fatalf("a resumed sensor is a recovery after a gap: %+v", rows[1])
	}
	if !rows[5].Auto {
		t.Fatalf("trigger acknowledgement must be marked auto: %+v", rows[5])
	}
	if len(Handoff("acme", []Node{{ID: "4", Status: "UP", StatusSince: ago(100 * time.Hour)}}, now.Add(-12*time.Hour), now)) != 0 {
		t.Fatal("nothing in the window must give no rows")
	}
}

func TestSilenced(t *testing.T) {
	info := func(ts, until string) *StatusInfo {
		return &StatusInfo{Info: &NodeStatusInfo{TimeStamp: ts, ValidUntil: until, User: &Ref{Name: "marcus"}, Message: "maint"}}
	}
	nodes := []Node{
		{ID: "1", Name: "fw01", Type: "device", Status: "PAUSED_BY_USER", StatusInfo: info(ago(20*24*time.Hour), "")},
		{ID: "2", Name: "sw01", Type: "device", Status: "PAUSED_BY_USER_UNTIL", StatusInfo: info(ago(time.Hour), now.Add(time.Hour).Format(time.RFC3339))},
		{ID: "3", Name: "CPU", Type: "sensor", Status: "DOWN_ACKNOWLEDGED", StatusInfo: info(ago(2*24*time.Hour), "")},
		{ID: "4", Name: "child", Type: "sensor", Status: "PAUSED_BY_PARENT"},
		{ID: "6", Name: "auto", Type: "sensor", Status: "DOWN_ACKNOWLEDGED", StatusInfo: &StatusInfo{Info: &NodeStatusInfo{AutoAcknowledged: true, TimeStamp: ago(30 * 24 * time.Hour)}}},
		{ID: "5", Name: "ok", Type: "sensor", Status: "UP"},
	}
	all := Silenced("acme", nodes, SilencedOptions{Now: now})
	if len(all) != 3 {
		t.Fatalf("silenced = %+v (paused-by-parent, trigger acknowledgements and up must be left out)", all)
	}
	SortSilenced(all)
	if all[0].ID != "1" || all[0].By != "marcus" || !all[0].Indefinite {
		t.Fatalf("first row = %+v", all[0])
	}
	stale := Silenced("acme", nodes, SilencedOptions{Now: now, OlderThan: 7 * 24 * time.Hour, Indefinite: true})
	if len(stale) != 1 || stale[0].ID != "1" || !stale[0].Stale {
		t.Fatalf("stale = %+v", stale)
	}
	for _, r := range all {
		if r.ID == "2" && (r.Indefinite || r.Until == "") {
			t.Fatalf("timed pause = %+v", r)
		}
	}
}

func TestRunnerDoesNotShareArgsBetweenCores(t *testing.T) {
	// Run must copy args before appending per-core flags: the caller passes
	// one slice (with spare capacity) to every core's child concurrently.
	args := make([]string, 1, 8)
	args[0] = "sync"
	backing := args[:cap(args)]
	before := append([]string(nil), backing...)
	t.Setenv(DirEnv, t.TempDir())
	lax, secure := testChild(t, "args", true), testChild(t, "args", false)
	for i, r := range []Runner{lax, secure} {
		c := Core{Name: []string{"lax", "secure"}[i]}
		if _, err := r.Run(t.Context(), c, nil, args...); err != nil {
			t.Fatal(err)
		}
		for i := range backing {
			if backing[i] != before[i] {
				t.Fatalf("Run(%s) wrote %q into the caller's backing array at %d", c.Name, backing[i], i)
			}
		}
	}
	out, err := secure.Run(t.Context(), Core{Name: "secure"}, nil, args...)
	if err != nil || strings.Contains(string(out), "--insecure") {
		t.Fatalf("a secure core's child got --insecure: %q %v", out, err)
	}
}

func TestSelectIntersectsNamesAndTags(t *testing.T) {
	r := &Registry{Cores: []Core{
		{Name: "acme", Server: "a", Tags: []string{"gold"}},
		{Name: "globex", Server: "g"},
	}}
	if _, err := r.Select([]string{"globex"}, []string{"gold"}); err == nil {
		t.Fatal("naming a core that lacks the requested tag must be an error, not an empty report")
	}
	got, err := r.Select([]string{"acme"}, []string{"gold"})
	if err != nil || len(got) != 1 || got[0].Name != "acme" {
		t.Fatalf("intersection = %v %v", got, err)
	}
}

func TestParseTimeZeroDate(t *testing.T) {
	if _, ok := ParseTime("1899-12-30T00:00:00Z"); ok {
		t.Fatal("PRTG zero date must read as unset")
	}
	rows := Silenced("acme", []Node{{ID: "1", Status: "PAUSED_BY_USER",
		StatusInfo: &StatusInfo{Info: &NodeStatusInfo{TimeStamp: ago(30 * 24 * time.Hour), ValidUntil: "1899-12-30T00:00:00Z"}}}},
		SilencedOptions{Now: now, Indefinite: true, OlderThan: 7 * 24 * time.Hour})
	if len(rows) != 1 || !rows[0].Indefinite || !rows[0].Stale {
		t.Fatalf("zero-date pause = %+v", rows)
	}
}

func TestLicenseDecodesBothSpellings(t *testing.T) {
	var a, b License
	if err := json.Unmarshal([]byte(`{"end_date":"2026-10-20T00:00:00Z","sensor_count":500,"key":"x"}`), &a); err != nil || a.EndDate == "" || a.SensorCount == nil {
		t.Fatalf("snake_case = %+v %v", a, err)
	}
	if err := json.Unmarshal([]byte(`{"EndDate":"2026-10-20T00:00:00Z","SensorCount":9999999,"LicenseSensorCount":9999999}`), &b); err != nil || b.EndDate == "" || b.LicenseSensorCount == nil {
		t.Fatalf("PascalCase = %+v %v", b, err)
	}
	var c License
	if err := json.Unmarshal([]byte(`{"unrelated":1}`), &c); err == nil {
		t.Fatal("a body with no license fields must be an error")
	}
}

func TestRedactContainersUnderSecretKeys(t *testing.T) {
	out, err := RedactJSON(json.RawMessage(`{"password":["p@ss"],"snmpcommunity":["public"],"token":{"value":"abc"},"name":"keep"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"p@ss", "public", "abc"} {
		if strings.Contains(string(out), leaked) {
			t.Fatalf("leaked %s: %s", leaked, out)
		}
	}
	if !strings.Contains(string(out), "keep") {
		t.Fatalf("over-redacted: %s", out)
	}
}

func TestProbeSeverity(t *testing.T) {
	got := map[string]string{}
	for _, f := range Drift([]CoreFacts{{Core: "c", Health: "ok", Probes: []Node{
		{Name: "a", ProbeStatus: "DISCONNECTED"}, {Name: "b", ProbeStatus: "UPDATING"}, {Name: "c", ProbeStatus: "CONNECTED"}}}},
		DriftOptions{Now: now}) {
		got[f.Object] = f.Severity
	}
	if got["a"] != "critical" || got["b"] != "warning" || got["c"] != "" {
		t.Fatalf("probe severities = %v", got)
	}
}

func TestRollupKeepsSingleSensorRow(t *testing.T) {
	all, _ := ParseAlarmStatuses(nil)
	rows := RollupByDevice(AlarmRows("acme", alarmNodes(), AlarmOptions{Statuses: all, Now: now}))
	for _, r := range rows {
		if r.Device == "app" && (r.ID != "4" || r.AckedBy != "rosa" || r.Sensors != 1) {
			t.Fatalf("single-sensor device lost its row: %+v", r)
		}
	}
}
