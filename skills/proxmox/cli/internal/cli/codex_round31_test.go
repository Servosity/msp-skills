package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"proxmox-pp-cli/internal/client"
)

// pveVia serves the fixture's PVE through h, which sees every request first.
func pveVia(t *testing.T, f *fixture, h func(w http.ResponseWriter, r *http.Request) bool) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h(w, r) {
			f.pve.Config.Handler.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
}

func isVMConfig(r *http.Request) bool {
	return strings.Contains(r.URL.Path, "/qemu/") && strings.HasSuffix(r.URL.Path, "/config")
}

// VM 100 was created 90 minutes ago: its 2h-old backup is inside the clock-skew
// tolerance before creation (either guest's), its 26h- and 50h-old ones an
// earlier guest's. The either-guest one is listed and flagged, never credited
// and never withheld.
func TestBackupJustBeforeCreationIsUncertain(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 100, 1.5)
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Verdict != "unproven" || !strings.Contains(r.Reason, "within an hour before") {
		t.Errorf("posture: want unproven, got %s %q", r.Verdict, r.Reason)
	}
	out, _, err := runCLI(t, "restore-points", "100", "--json")
	g := decodeJSON[restorePointsReport](t, out).Guests[0]
	if err != nil || g.Count != 1 || g.MayBeEarlierGuest != 1 || !g.RestorePoints[0].MayBeEarlierGuest || g.NewestAt != "" || g.PredatingGuest != 2 || g.Evidence != "partial" {
		t.Errorf("restore-points: err %v count %d flagged %d newest %q predating %d evidence %s", err, g.Count, g.MayBeEarlierGuest, g.NewestAt, g.PredatingGuest, g.Evidence)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.RestorePoints != 0 || r.MetSchedule {
		t.Errorf("rpo: points %d met %v", r.RestorePoints, r.MetSchedule)
	}
}

// A VM config that cannot be read hides when the VM was created: its backups
// are listed but flagged, none is credited, the failed read is a fetch
// failure, and the dead node is asked once, in one attempt.
func TestUnreadVMConfigCreditsNothing(t *testing.T) {
	f := newFixture(t)
	tasks := f.pveRoutes["/nodes/pve1/tasks"].([]map[string]any)
	tasks[0]["starttime"], tasks[0]["endtime"] = ago(f.now, 5), ago(f.now, 4.5)
	var reads atomic.Int64
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if isVMConfig(r) {
			reads.Add(1)
			http.Error(w, `{"data":null}`, http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	out, _, _ := runCLI(t, "posture", "--json")
	rep := decodeJSON[postureReport](t, out)
	if r := postureRowFor(t, out, 100); r.Verdict != "unproven" || !strings.Contains(r.Reason, "are listed, but its config") {
		t.Errorf("posture: want unproven, got %s %q", r.Verdict, r.Reason)
	}
	if !strings.Contains(fmt.Sprint(rep.FetchFailures), "config") {
		t.Errorf("posture: the failed config read is no fetch failure: %v", rep.FetchFailures)
	}
	if n := reads.Swap(0); n != 1 {
		t.Errorf("posture: %d config reads against a node that did not answer, want 1", n)
	}
	out, _, err := runCLI(t, "restore-points", "100", "--json")
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; err != nil || g.Count == 0 || g.MayBeEarlierGuest != g.Count || g.NewestAt != "" || g.Evidence != "partial" {
		t.Errorf("restore-points: err %v count %d flagged %d newest %q evidence %s", err, g.Count, g.MayBeEarlierGuest, g.NewestAt, g.Evidence)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.RestorePoints != 0 || r.MetSchedule || r.Evidence != "partial" || strings.Contains(r.Note, "no restore points retained") || !strings.Contains(r.Note, "; the VM's creation time could not be read") {
		t.Errorf("rpo: points %d met %v evidence %s note %q", r.RestorePoints, r.MetSchedule, r.Evidence, r.Note)
	}
	out, _, _ = runCLI(t, "failures", "--json")
	for _, r := range decodeJSON[failuresReport](t, out).Rows {
		if r.VMID == 108 && r.GuestName != "" {
			t.Errorf("failures named a task %q with VM 108's config unread", r.GuestName)
		}
	}
}

// One VM's config error (PVE answers 500 for a guest deleted mid-run) blinds
// that VM only: the node's other configs are still read.
func TestOneConfigErrorKeepsTheNode(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 104, 24*30)
	var reads atomic.Int64
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if isVMConfig(r) {
			reads.Add(1)
			if strings.Contains(r.URL.Path, "/qemu/100/") {
				http.Error(w, `{"data":null}`, http.StatusInternalServerError)
				return true
			}
		}
		return false
	})
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Severity == "green" {
		t.Errorf("100 credited with its config unread: %s", r.Verdict)
	}
	if n := reads.Load(); n < 4 {
		t.Errorf("only %d config reads: one guest's error stopped the node", n)
	}
}

// A point just before creation is older than every point of the VM, so it
// cannot change a stale verdict or excuse a missed run.
func TestEitherPointLeavesProvenOutcomes(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 101, 50.5) // own point 50h old: stale
	addPointAt(f, 101, -51*time.Hour, "ok")
	setCreated(f, 105, 84.5) // never backed up since
	addPointAt(f, 105, -85*time.Hour, "ok")
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 101); r.Verdict != "stale" {
		t.Errorf("101: want stale, got %s %q", r.Verdict, r.Reason)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns == 0 || r.PointsUnread {
		t.Errorf("105: misses %d unread %v evidence %s", r.MissedRuns, r.PointsUnread, r.Evidence)
	}
}

// A VM created an hour ago missed no run of a daily job that fired before it existed.
func TestRPOJudgesNoRunBeforeCreation(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 105, 1) // 105: covered, never backed up
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MissedRuns != 0 || r.ScheduledRuns != 0 || r.MetSchedule {
		t.Errorf("rpo judged runs from before VM 105 existed: %d scheduled, %d missed, met %v", r.ScheduledRuns, r.MissedRuns, r.MetSchedule)
	}
	if strings.Contains(r.Note, "every scheduled run") || strings.Contains(r.Note, "widen --days") || !strings.Contains(r.Note, "inside the window") {
		t.Errorf("rpo note: %q", r.Note)
	}
}

// A ctime dated past now came from a node clock that ran ahead: it bounds
// nothing, and the VM's own backups stay its own.
func TestFutureCreationTimeIsNoRecord(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 100, -72)
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Verdict != "fresh" {
		t.Errorf("a future ctime dropped VM 100's backups: %s %q", r.Verdict, r.Reason)
	}
}

// No VM config read goes to an offline node: PVE proxies it there and times out.
func TestNoConfigReadOnOfflineNode(t *testing.T) {
	f := newFixture(t)
	f.pveRoutes["/nodes"] = []map[string]any{{"node": "pve1", "status": "offline"}}
	var reads atomic.Int64
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if isVMConfig(r) {
			reads.Add(1)
		}
		return false
	})
	for _, args := range [][]string{{"posture", "--json"}, {"rpo", "--json"}, {"restore-points", "100", "--json"}, {"failures", "--json"}} {
		out, _, _ := runCLI(t, args...)
		if args[0] == "posture" {
			if r := postureRowFor(t, out, 100); r.Severity == "green" {
				t.Errorf("posture: VM 100 on an offline node is %s", r.Verdict)
			}
		}
	}
	if n := reads.Load(); n != 0 {
		t.Errorf("%d VM config reads went to an offline node", n)
	}
}

// Proxmox answers carry free-text notes: no response is cached at all, from
// the backup commands or the generated PVE and PBS reads.
func TestVMConfigNotCached(t *testing.T) {
	f := newFixture(t)
	const sentinel = "root-password-is-hunter2"
	f.pveRoutes["/nodes/pve1/qemu/100/config"] = map[string]any{"description": sentinel, "meta": "ctime=1"}
	f.pveRoutes["/nodes/pve1/qemu/100/pending"] = []map[string]any{{"key": "description", "value": sentinel}}
	f.pveRoutes["/nodes/pve1/lxc/102/config"] = map[string]any{"description": sentinel}
	f.pveRoutes["/nodes/pve1/config"] = map[string]any{"description": sentinel}
	snaps := f.pbsRoutes["/admin/datastore/main/snapshots"].([]any)
	snaps[0].(map[string]any)["comment"] = sentinel
	dir := t.TempDir()
	t.Setenv("PROXMOX_CACHE_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("PROXMOX_HOME", dir)
	for _, args := range [][]string{{"posture", "--json"}, {"vm", "config", "pve1", "100", "--json"}, {"vm", "pending", "pve1", "100", "--json"}, {"ct", "config", "pve1", "102", "--json"}, {"nodes", "config", "pve1", "--json"}, {"pbs-datastore", "snapshots", "main", "--json"}} {
		out, _, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if args[0] != "posture" && !strings.Contains(out, sentinel) {
			t.Fatalf("%v did not read the config: %s", args, out)
		}
	}
	cached := 0
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if strings.Contains(p, string(filepath.Separator)+"http"+string(filepath.Separator)) {
				cached++
			}
			if b, _ := os.ReadFile(p); strings.Contains(string(b), sentinel) {
				t.Errorf("VM notes cached in %s", p)
			}
		}
		return nil
	})
	if cached != 0 {
		t.Errorf("%d response cache file(s) written", cached)
	}
}
