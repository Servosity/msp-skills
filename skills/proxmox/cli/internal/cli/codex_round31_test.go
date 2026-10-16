package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

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
// earlier guest's. Neither is credited, and neither is proof of absence.
func TestBackupJustBeforeCreationIsUncertain(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 100, 1.5)
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Verdict != "unproven" || !strings.Contains(r.Reason, "within an hour before") {
		t.Errorf("posture: want unproven, got %s %q", r.Verdict, r.Reason)
	}
	out, _, _ = runCLI(t, "restore-points", "100", "--json")
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; g.Count != 0 || g.PredatingGuest != 2 || g.Evidence != "partial" {
		t.Errorf("restore-points: count %d predating %d evidence %s", g.Count, g.PredatingGuest, g.Evidence)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.RestorePoints != 0 || r.MetSchedule || r.Evidence != "partial" {
		t.Errorf("rpo: points %d met %v evidence %s", r.RestorePoints, r.MetSchedule, r.Evidence)
	}
}

// A VM config that cannot be read hides when the VM was created: nothing
// under its VMID is credited to it or named after it.
func TestUnreadVMConfigCreditsNothing(t *testing.T) {
	f := newFixture(t)
	tasks := f.pveRoutes["/nodes/pve1/tasks"].([]map[string]any)
	tasks[0]["starttime"], tasks[0]["endtime"] = ago(f.now, 5), ago(f.now, 4.5)
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if isVMConfig(r) {
			http.Error(w, `{"data":null}`, http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Verdict != "unproven" || !strings.Contains(r.Reason, "could not be read") {
		t.Errorf("posture: want unproven, got %s %q", r.Verdict, r.Reason)
	}
	out, _, _ = runCLI(t, "restore-points", "100", "--json")
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; g.Count != 0 || g.Evidence != "partial" {
		t.Errorf("restore-points: count %d evidence %s", g.Count, g.Evidence)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--guest", "100", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.RestorePoints != 0 || r.MetSchedule || r.Evidence != "partial" {
		t.Errorf("rpo: points %d met %v evidence %s", r.RestorePoints, r.MetSchedule, r.Evidence)
	}
	out, _, _ = runCLI(t, "failures", "--json")
	for _, r := range decodeJSON[failuresReport](t, out).Rows {
		if r.VMID == 108 && r.GuestName != "" {
			t.Errorf("failures named a task %q with VM 108's config unread", r.GuestName)
		}
	}
}

// A VM created an hour ago missed no run of a daily job that fired before it existed.
func TestRPOJudgesNoRunBeforeCreation(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 105, 1) // 105: covered, never backed up
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	if r := decodeJSON[rpoReport](t, out).Rows[0]; r.MissedRuns != 0 || r.ScheduledRuns != 0 {
		t.Errorf("rpo judged runs from before VM 105 existed: %d scheduled, %d missed", r.ScheduledRuns, r.MissedRuns)
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

// A VM config carries its notes: it never reaches the response cache.
func TestVMConfigNotCached(t *testing.T) {
	f := newFixture(t)
	const sentinel = "root-password-is-hunter2"
	f.pveRoutes["/nodes/pve1/qemu/100/config"] = map[string]any{"description": sentinel, "meta": "ctime=1"}
	dir := t.TempDir()
	t.Setenv("PROXMOX_CACHE_DIR", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	if _, _, err := runCLI(t, "posture", "--json"); err != nil {
		t.Fatal(err)
	}
	cached := 0
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			cached++
			if b, _ := os.ReadFile(p); strings.Contains(string(b), sentinel) {
				t.Errorf("VM notes cached in %s", p)
			}
		}
		return nil
	})
	t.Logf("%d cache files checked", cached)
}
