package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// VM 105 (created 84.5h ago, never backed up since) has one verified backup
// dated 85h ago: it may be an earlier guest's. rpo names it instead of "no
// restore points retained", keeps the misses certain, and the client rollup
// does not count the guest as having no restore points.
func TestRPONamesFlaggedBackups(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 105, 84.5)
	addPointAt(f, 105, -85*time.Hour, "ok")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	r := decodeJSON[rpoReport](t, out).Rows[0]
	if r.MayBeEarlierGuest != 1 || strings.Contains(r.Note, rpoNoPointsNote) || r.MissedRuns == 0 || r.PointsUnread {
		t.Errorf("rpo: flagged %d misses %d unread %v note %q", r.MayBeEarlierGuest, r.MissedRuns, r.PointsUnread, r.Note)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--agent")
	if !strings.Contains(out, `"may_be_earlier_guest"`) {
		t.Errorf("rpo --agent drops may_be_earlier_guest:\n%s", out)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--by-client", "--json")
	for _, c := range decodeJSON[[]rpoClientRow](t, out) {
		if c.GuestsWithNoData != 0 {
			t.Errorf("rollup counts %d guest(s) without restore points", c.GuestsWithNoData)
		}
	}
}

// The guest's may_be_earlier_guest counts only LISTED points: not one with no
// readable manifest, not one --verified-only hides.
func TestFlaggedCountIsListedPointsOnly(t *testing.T) {
	f := newFixture(t)
	setCreated(f, 100, 1.5) // 2h point: either guest's
	out, _, _ := runCLI(t, "restore-points", "100", "--verified-only", "--json")
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; g.MayBeEarlierGuest != 1 {
		t.Fatalf("baseline: flagged %d", g.MayBeEarlierGuest)
	}
	for _, s := range f.pbsRoutes["/admin/datastore/main/snapshots"].([]any) {
		m := s.(map[string]any)
		if m["backup-id"] == "100" && m["backup-time"] == ago(f.now, 2) {
			delete(m, "verification")
		}
	}
	for _, s := range f.pveRoutes["/nodes/pve1/storage/pbs1/content"].([]any) {
		m := s.(map[string]any)
		if m["vmid"] == 100 && m["ctime"] == ago(f.now, 2) {
			delete(m, "verification")
		}
	}
	out, _, _ = runCLI(t, "restore-points", "100", "--verified-only", "--json")
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; g.MayBeEarlierGuest != 0 || strings.Contains(strings.Join(g.NotRead, ";"), "listed with may_be_earlier_guest") {
		t.Errorf("--verified-only hid the flagged point but still counts it: %d %v", g.MayBeEarlierGuest, g.NotRead)
	}
}

// An empty 200 to the config read is no record of anything: unread, not "no ctime".
func TestEmptyConfigBodyIsUnread(t *testing.T) {
	f := newFixture(t)
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if isVMConfig(r) && strings.Contains(r.URL.Path, "/qemu/100/") {
			w.WriteHeader(http.StatusOK)
			return true
		}
		return false
	})
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 100); r.Severity == "green" {
		t.Errorf("an empty config body credited VM 100: %s", r.Verdict)
	}
}

// posture, rpo and restore-points never store what they read (backup notes,
// PBS snapshot comments) in the response cache.
func TestAssuranceReadsStoreNothing(t *testing.T) {
	f := newFixture(t)
	const sentinel = "pw-in-backup-notes-hunter2"
	addPointAt(f, 100, -time.Hour, "ok")
	snaps := f.pbsRoutes["/admin/datastore/main/snapshots"].([]any)
	snaps[len(snaps)-1].(map[string]any)["comment"] = sentinel
	vols := f.pveRoutes["/nodes/pve1/storage/pbs1/content"].([]any)
	vols[len(vols)-1].(map[string]any)["notes"] = sentinel
	dir := t.TempDir()
	t.Setenv("PROXMOX_HOME", dir)
	t.Setenv("PROXMOX_CACHE_DIR", dir)
	saw := false
	for _, args := range [][]string{{"posture", "--json"}, {"rpo", "--json"}, {"restore-points", "100", "--json"}} {
		out, _, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		saw = saw || strings.Contains(out, sentinel)
	}
	if !saw {
		t.Fatal("restore-points did not read the notes")
	}
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), sentinel) {
				t.Errorf("notes stored in %s", p)
			}
		}
		return nil
	})
}
