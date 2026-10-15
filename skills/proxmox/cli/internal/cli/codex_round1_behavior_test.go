// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"proxmox-pp-cli/internal/client"
)

// VM.Audit that does not propagate, or a deeper ACL that drops it, hides
// guests from every list: the report must say so.
func TestGuestVisibilityNeedsPropagatingVMAudit(t *testing.T) {
	for name, perms := range map[string]map[string]any{
		"non-propagating": {
			"/":    map[string]any{"Sys.Audit": 1, "Datastore.Audit": 1, "Datastore.AllocateSpace": 1, "VM.Backup": 1},
			"/vms": map[string]any{"VM.Audit": 0},
		},
		"deeper ACL drops it": {
			"/":        map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "Datastore.Audit": 1, "Datastore.AllocateSpace": 1, "VM.Backup": 1},
			"/vms/105": map[string]any{"VM.Console": 1},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.pveRoutes["/access/permissions"] = perms
			rep, _ := postureByVMID(t)
			if e := failureByScope(rep.FetchFailures)["guests"]; e == "" {
				t.Errorf("posture must flag the incomplete guest list: %+v", rep.FetchFailures)
			}
		})
	}
}

// A storage list the token cannot fully see is incomplete for capacity too.
func TestStorageVisibilityNeedsPropagatingAudit(t *testing.T) {
	f := newFixture(t)
	f.pveRoutes["/access/permissions"] = map[string]any{
		"/":        map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "VM.Backup": 1},
		"/storage": map[string]any{"Datastore.Audit": 0, "Datastore.AllocateSpace": 0},
	}
	out, _, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if e := failureByScope(decodeJSON[capacityReport](t, out).FetchFailures)["pve storage"]; !strings.Contains(e, "propagate") {
		t.Errorf("capacity must flag the incomplete storage list:\n%s", out)
	}
	rep, rows := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["backup storage"]; e == "" {
		t.Errorf("posture must flag the incomplete storage list: %+v", rep.FetchFailures)
	}
	for id, r := range rows {
		if r.Severity == "green" && r.Verdict == "fresh" {
			t.Errorf("%d is fresh while a storage may be hidden", id)
		}
	}
}

// Two different PVE clusters writing the same PBS namespace: VMIDs collide,
// so neither cluster is credited with those snapshots.
func TestSharedPBSNamespaceIsNotAttributed(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	other := httptest.NewTLSServer(f.pve.Config.Handler) // a second PVE cluster with the same storage config
	defer other.Close()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
pve_base_url = "`+other.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(other.Certificate())+`"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
`)
	out, _, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rep := decodeJSON[postureReport](t, out)
	if !strings.Contains(out, "VMIDs collide") {
		t.Errorf("the shared namespace must be reported:\n%+v", rep.FetchFailures)
	}
	for _, r := range rep.Rows {
		if r.VMID == 100 && r.Verdict == "fresh" {
			t.Errorf("%s/100 credited with a snapshot either cluster may own: %s %q", r.Cluster, r.Verdict, r.Reason)
		}
	}
}

// Every archive read failed: a count of 0 is "could not be read", flagged in
// the JSON and in the exit code, never a clean "no restore points".
func TestRestorePointsUnreadIsNotNone(t *testing.T) {
	f := newFixture(t)
	auditOnly(f)
	t.Setenv("PROXMOX_PBS_HOST", "")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "")
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", "")
	out, _, err := runCLI(t, "restore-points", "100", "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 5 {
		t.Fatalf("want exit 5 when nothing could be read, got %v", err)
	}
	g := decodeJSON[restorePointsReport](t, out).Guests[0]
	if g.Count != 0 || g.Evidence != "partial" || len(g.NotRead) == 0 {
		t.Errorf("want partial evidence naming the unread sources: %+v", g)
	}
	_, _, err = runCLI(t, "restore-points", "100")
	if !errors.As(err, &ce) || ce.code != 5 {
		t.Errorf("human mode must exit 5 too, got %v", err)
	}
}

// Only the newest scheduled run may still be in progress: older misses count
// whatever their age.
func TestRPOOnlyNewestRunIsInGrace(t *testing.T) {
	now := time.Now()
	var runs []int64
	var points []restorePoint
	for h := 10; h >= 1; h-- {
		at := now.Add(-time.Duration(h) * time.Hour).Unix()
		runs = append(runs, at)
		if h > 5 {
			points = append(points, restorePoint{Time: at + 60})
		}
	}
	row := evaluateRPO(points, runs, now.Add(-24*time.Hour), now)
	if row.MissedRuns != 4 || row.MetSchedule {
		t.Errorf("hourly runs at -5h..-2h had no point (the -1h run is in grace): missed %d, met %v", row.MissedRuns, row.MetSchedule)
	}
}

// The per-client rollup carries how much was not read.
func TestByClientCountsFetchFailures(t *testing.T) {
	f := newFixture(t)
	auditOnly(f) // local@pve1 is unread
	out, _, err := runCLI(t, "posture", "--by-client", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range decodeJSON[[]postureClientRow](t, out) {
		if c.FetchFailures == 0 {
			t.Errorf("posture by-client must count fetch failures: %+v", c)
		}
	}
	out, _, err = runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range decodeJSON[[]rpoClientRow](t, out) {
		if c.FetchFailures == 0 {
			t.Errorf("rpo by-client must count fetch failures: %+v", c)
		}
	}
}

// The prune preview uses the covering job's own retention, which overrides
// the storage's; jobs that disagree leave the points unmarked.
func TestPruneMarksUseJobRetention(t *testing.T) {
	f := newFixture(t)
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["prune-backups"] = "keep-last=1"
	got := ""
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/prunebackups") {
			got = r.URL.Query().Get("prune-backups")
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
	if _, stderr, err := runCLI(t, "restore-points", "100", "--json"); err != nil {
		t.Fatalf("restore-points: %v\n%s", err, stderr)
	}
	if got != "keep-last=1" {
		t.Errorf("prune preview sent prune-backups=%q, want the job's keep-last=1", got)
	}

	cov := &coverage{EnabledFor: map[int64][]string{100: {"a", "b"}}}
	jt := &jobTargets{jobs: map[string]backupJob{
		"a": {ID: "a", Storage: "s", PruneBackups: json.RawMessage(`"keep-last=2"`)},
		"b": {ID: "b", Storage: "s", PruneBackups: json.RawMessage(`{"keep-last":3}`)},
	}, defaults: map[string]string{}}
	if r := jobRetention(cov, jt, 100, "n", "s"); r.ok {
		t.Errorf("disagreeing jobs must leave the points unmarked: %+v", r)
	}
	if s := pruneSpec(json.RawMessage(`{"keep-daily":7,"keep-last":3}`)); s != "keep-daily=7,keep-last=3" {
		t.Errorf("pruneSpec = %q", s)
	}
}
