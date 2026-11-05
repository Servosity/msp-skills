// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"prtg-pp-cli/internal/fleet"
)

func TestCleanAPIPath(t *testing.T) {
	good := map[string]string{
		"/experimental/probes": "/experimental/probes",
		"version":              "/version",
		"/api/v2/sensors/1234": "/sensors/1234",
	}
	for in, want := range good {
		got, err := cleanAPIPath(in)
		if err != nil || got != want {
			t.Errorf("cleanAPIPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"https://evil.example/x", "/sensors?limit=1", "/../etc", "//evil.example/x", "/a b", "/x#y"} {
		if _, err := cleanAPIPath(bad); err == nil {
			t.Errorf("cleanAPIPath(%q) accepted a non-API path", bad)
		}
	}
}

func TestWritePathsAllowlist(t *testing.T) {
	for _, p := range []string{"/sensors/acknowledge", "/devices/pause", "/groups/resume", "/probes/scan", "/sensors/pause"} {
		if !writePaths.MatchString(p) {
			t.Errorf("%s should be an allowed bulk action", p)
		}
	}
	for _, p := range []string{"/devices/acknowledge", "/experimental/objects", "/users/1/api-keys", "/devices/40/pause", "/session"} {
		if writePaths.MatchString(p) || scanOnePath.MatchString(p) {
			t.Errorf("%s must not be sendable by the raw child", p)
		}
	}
	if !scanOnePath.MatchString("/devices/40/scan") || scanOnePath.MatchString("/devices/40/scan/x") || scanOnePath.MatchString("/devices/a b/scan") {
		t.Error("per-object scan path match")
	}
}

func TestParseKindAndDuration(t *testing.T) {
	for in, want := range map[string]string{"device": "devices", "Groups": "groups", " probe ": "probes", "sensors": "sensors"} {
		if got, err := parseKind(in); err != nil || got != want {
			t.Errorf("parseKind(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := parseKind("channel"); err == nil {
		t.Error("channels cannot be paused")
	}
	for in, want := range map[string]float64{"": 0, "0": 0, "2h": 7200, "90m": 5400, "1d": 86400, "100ms": 0.1} {
		if got, err := durationSeconds(in); err != nil || got != want {
			t.Errorf("durationSeconds(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := durationSeconds("soon"); err == nil {
		t.Error("bad duration must be a usage error")
	}
}

func TestLimitAlarmsKeepsErrors(t *testing.T) {
	rows := []fleet.AlarmRow{{Core: "a", ID: "1"}, {Core: "a", ID: "2"}, {Core: "dead", Error: "dial"}}
	got := limitAlarms(rows, 1)
	if len(got) != 2 || got[0].ID != "1" || got[1].Error != "dial" {
		t.Fatalf("limitAlarms = %+v", got)
	}
	if len(limitAlarms(rows, 0)) != 3 {
		t.Fatal("limit 0 keeps everything")
	}
}

func TestScopedFilter(t *testing.T) {
	ack := fleetAction{verb: "acknowledge", filter: `name = "Ping"`}
	if got := ack.scopedFilter(); got != `(name = "Ping") and (status = down or status = partialdown)` {
		t.Fatalf("ack filter = %s", got)
	}
	pause := fleetAction{verb: "pause", filter: `name = "fw01"`}
	if got := pause.scopedFilter(); got != `(name = "fw01") and not(status = pausedbyuser or status = pausedbyuseruntil)` {
		t.Fatalf("pause filter = %s", got)
	}
	if got := (fleetAction{verb: "resume", filter: "x = 1"}).scopedFilter(); got != "(x = 1) and (status = pausedbyuseruntil)" {
		t.Fatalf("resume filter = %s", got)
	}
	if got := (fleetAction{verb: "resume", filter: "x = 1", includeIndefinite: true}).scopedFilter(); got != "(x = 1) and (status = pausedbyuser or status = pausedbyuseruntil)" {
		t.Fatalf("resume --include-indefinite filter = %s", got)
	}
	for verb, cases := range map[string]map[string]bool{
		"acknowledge": {"DOWN": true, "DOWN_PARTIAL": true, "DOWN_ACKNOWLEDGED": false, "UP": false, "": false},
		"pause":       {"UP": true, "DOWN": true, "PAUSED_BY_USER": false, "PAUSED_BY_USER_UNTIL": false, "": false},
		"resume":      {"PAUSED_BY_USER_UNTIL": true, "PAUSED_BY_USER": false, "UP": false},
	} {
		for st, want := range cases {
			if got := (fleetAction{verb: verb}).inScope(st); got != want {
				t.Errorf("%s inScope(%q) = %v, want %v", verb, st, got, want)
			}
		}
	}
	if !relationshipPrefix.MatchString(`children name = "fw01"`) || relationshipPrefix.MatchString(`name = "children"`) {
		t.Error("relationship prefix detection")
	}
	m := 5
	if (ActionRow{Matched: &m}).actionable() != 5 {
		t.Fatal("actionable counts the scoped matches")
	}
}

func TestValidateFilter(t *testing.T) {
	for _, ok := range []string{`name = "Ping"`, `name = "a (b"`, `name = "Disk Free C:\"`, `name = "x\"`, `(status = down) and tags contains "x"`, `not(status = up)`} {
		if err := validateFilter(ok); err != nil {
			t.Errorf("validateFilter(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{`status = acknowledged) or (status = down`, `(name = "x"`, `name = "x`, `a) or (b`, `name = "x\" ) or (status = up"`} {
		if err := validateFilter(bad); err == nil {
			t.Errorf("validateFilter(%q) accepted an expression that escapes the scope", bad)
		}
	}
}

func TestResolveRejectsPinnedConflicts(t *testing.T) {
	t.Setenv(fleet.DirEnv, t.TempDir())
	if err := fleet.Update(func(r *fleet.Registry) error {
		r.Upsert(fleet.Core{Name: "acme", Server: "a.example"})
		r.Upsert(fleet.Core{Name: "globex", Server: "g.example", Tags: []string{"gold"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	old := pinnedCore
	defer func() { pinnedCore = old }()
	pinnedCore = "acme"
	for _, s := range []fleetScope{{only: []string{"globex"}}, {tags: []string{"gold"}}, {only: []string{"acme", "globex"}}} {
		if _, err := s.resolve(); err == nil {
			t.Fatalf("pinned acme with %+v must be a usage error", s)
		}
	}
	s := fleetScope{only: []string{"acme"}}
	if cores, err := s.resolve(); err != nil || len(cores) != 1 || cores[0].Name != "acme" {
		t.Fatalf("pinned acme with --cores acme = %v %v", cores, err)
	}
}

func TestUnpreviewed(t *testing.T) {
	got := unpreviewed([]string{"1", "2", "3"}, map[string]bool{"1": true, "3": true})
	if len(got) != 1 || got[0] != "2" {
		t.Fatalf("unpreviewed = %v", got)
	}
}

func TestActionResults(t *testing.T) {
	res, err := actionResults(json.RawMessage(`[{"node":{"id":"11","name":"a"}},{"node":{"name":"b"},"failure":{"code":"FORBIDDEN","message":"no right"}},{"node":{"id":"13","name":"c"},"failure":null}]`))
	if err != nil || len(res.okIDs) != 2 || res.okIDs[1] != "13" || res.failed != 1 || len(res.msgs) != 1 || !strings.Contains(res.msgs[0], "b: FORBIDDEN") {
		t.Fatalf("actionResults = %+v %v", res, err)
	}
	if res, err := actionResults(json.RawMessage(`[]`)); err != nil || len(res.okIDs) != 0 {
		t.Fatal("[] is a legitimate zero-match result")
	}
	for _, bad := range []string{`null`, ``, `{"status":"accepted"}`} {
		if _, err := actionResults(json.RawMessage(bad)); err == nil {
			t.Fatalf("%q must be an error, not a success", bad)
		}
	}
	// An entry that names no node is a failed item, never a success.
	if res, err := actionResults(json.RawMessage(`[{}]`)); err != nil || res.failed != 1 || len(res.okIDs) != 0 {
		t.Fatalf("[{}] = %+v %v, want one failed item", res, err)
	}
}

func TestFleetActionRequest(t *testing.T) {
	a := fleetAction{verb: "pause", kind: "devices", filter: `name = "fw01"`, body: map[string]any{"message": "m", "duration": 60.0}}
	q := a.request("/devices/pause")
	var body map[string]any
	if err := json.Unmarshal(q.Body, &body); err != nil {
		t.Fatal(err)
	}
	if q.Method != fleet.MethodPost || body["filter"] != `name = "fw01"` || body["message"] != "m" || body["duration"] != 60.0 {
		t.Fatalf("pause request = %s %s", q.Method, q.Body)
	}

}

func TestSyncRowFor(t *testing.T) {
	c := fleet.Core{Name: "acme"}
	out := []byte(`{"event":"sync_start","resource":"sensors"}
{"event":"sync_error","resource":"groups","error":"GET /groups: 403"}
{"event":"sync_summary","total_records":15,"resources":3,"success":2,"warned":0,"errored":1,"duration_ms":10}
{
  "total_records": 15,
  "resources": 3,
  "success": 2,
  "warned": 0,
  "errored": 1
}
`)
	row := syncRowFor(c, out, nil, time.Second)
	if row.Error != "" || row.Records == nil || *row.Records != 15 || len(row.SyncErrors) != 1 {
		t.Fatalf("partial sync row = %+v", row)
	}
	dead := syncRowFor(c, []byte(`{"event":"sync_error","resource":"sensors","error":"dial refused"}
{"total_records":0,"resources":1,"success":1,"warned":0,"errored":1}`), nil, time.Second)
	if dead.Error != "dial refused" {
		t.Fatalf("a run that stored nothing and errored must fail the core: %+v", dead)
	}
	if failed := syncRowFor(c, nil, errors.New("timed out"), time.Second); failed.Error != "timed out" {
		t.Fatalf("child error row = %+v", failed)
	}
	tls := syncRowFor(c, []byte(`{"event":"sync_error","resource":"sensors","error":"x509: certificate signed by unknown authority"}`), errors.New("1 resource(s) failed to sync: sensors"), time.Second)
	if !strings.Contains(tls.Error, "x509") {
		t.Fatalf("a failed child must report its sync_error cause: %+v", tls)
	}
	if none := syncRowFor(c, []byte("not json"), nil, time.Second); none.Error == "" {
		t.Fatal("missing summary must be an error")
	}
}

// The literal names this package reads must stay the variables package fleet
// sets on its children.
func TestFleetEnvNamesMatchPackageFleet(t *testing.T) {
	if coreSelectEnv != fleet.SelectEnv {
		t.Errorf("coreSelectEnv = %q, fleet.SelectEnv = %q", coreSelectEnv, fleet.SelectEnv)
	}
	if fleetChildEnv != fleet.ChildEnv {
		t.Errorf("fleetChildEnv = %q, fleet.ChildEnv = %q", fleetChildEnv, fleet.ChildEnv)
	}
}
