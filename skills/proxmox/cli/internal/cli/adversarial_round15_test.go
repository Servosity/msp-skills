package cli

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// addPointAt adds one PBS snapshot (and its PVE-storage twin) of vmid dated
// offset from now with verify state st.
func addPointAt(f *fixture, vmid int, offset time.Duration, st string) {
	ts := f.now.Add(offset).Unix()
	snap := map[string]any{"backup-type": "vm", "backup-id": strconv.Itoa(vmid), "backup-time": ts, "size": 1 << 30, "files": []string{"index.json.blob"}, "owner": "root@pam"}
	pve := map[string]any{"volid": "pbs1:backup/vm/" + strconv.Itoa(vmid) + "/" + strconv.FormatInt(ts, 10), "vmid": vmid, "ctime": ts, "size": 1 << 30, "subtype": "qemu", "format": "pbs-vm"}
	if st != "" {
		snap["verification"] = map[string]any{"state": st, "upid": "UPID:pbs:verify"}
		pve["verification"] = map[string]any{"state": st, "upid": "UPID:pbs:verify"}
	}
	f.pbsRoutes["/admin/datastore/main/snapshots"] = append(f.pbsRoutes["/admin/datastore/main/snapshots"].([]any), snap)
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = append(f.pveRoutes["/nodes/pve1/storage/pbs1/content"].([]any), pve)
}

func postureRowFor(t *testing.T, out string, vmid int64) postureRow {
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.VMID == vmid {
			return r
		}
	}
	t.Fatalf("no row for %d:\n%s", vmid, out)
	return postureRow{}
}

// A future-dated point does not count toward age, but its failed verify still
// does: the newest backup is bad, so the guest is red.
func TestFutureDatedFailedVerifyStaysRed(t *testing.T) {
	f := newFixture(t)
	guests := f.pveRoutes["/cluster/resources"].([]map[string]any)
	f.pveRoutes["/cluster/resources"] = guests[:1] // guest 100 alone decides --fail-on-red
	addPointAt(f, 100, 3*time.Hour, "failed")
	out, _, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if r := postureRowFor(t, out, 100); r.Verdict != "verify-failed" || r.Severity != "red" || r.VerifyState != "failed" {
		t.Errorf("want verify-failed/red/failed, got %s/%s/%s: %q", r.Verdict, r.Severity, r.VerifyState, r.Reason)
	}
	if _, _, err := runCLI(t, "posture", "--fail-on-red", "--json"); ExitCode(err) != 1 {
		t.Errorf("--fail-on-red exit %d, want 1", ExitCode(err))
	}
}

// Inside the clock-skew tolerance a point counts, but its age is never negative.
func TestNoNegativeAgeInsideTolerance(t *testing.T) {
	f := newFixture(t)
	addPointAt(f, 101, 50*time.Minute, "ok")
	out, _, _ := runCLI(t, "posture", "--json")
	if r := postureRowFor(t, out, 101); r.AgeHours < 0 {
		t.Errorf("negative age %v", r.AgeHours)
	}
}

// verify-health judges recency on real points: a future-dated ok point neither
// hides a failed newest real point nor dates the newest good one in the future.
func TestVerifyHealthIgnoresFutureRecency(t *testing.T) {
	f := newFixture(t)
	addPointAt(f, 108, 200*24*time.Hour, "ok")
	addPointAt(f, 108, -1*time.Hour, "failed")
	out, _, _ := runCLI(t, "verify-health", "--all", "--json")
	found := false
	for _, g := range decodeJSON[verifyReport](t, out).Groups {
		if g.Group != "vm/108" {
			continue
		}
		found = true
		if !g.NewestIsBad {
			t.Errorf("newest real snapshot failed, newest_is_bad=false")
		}
		if g.NewestVerifiedOKAt > f.now.Add(time.Hour).UTC().Format(time.RFC3339) {
			t.Errorf("newest_verified_ok_at in the future: %s", g.NewestVerifiedOKAt)
		}
	}
	if !found {
		t.Fatalf("no vm/108 group:\n%s", out)
	}
	// The other way round: a future-dated FAILED point is still the newest
	// backup, and it is bad.
	f = newFixture(t)
	addPointAt(f, 108, -1*time.Hour, "ok")
	addPointAt(f, 108, 200*24*time.Hour, "failed")
	out, _, _ = runCLI(t, "verify-health", "--all", "--json")
	for _, g := range decodeJSON[verifyReport](t, out).Groups {
		if g.Group == "vm/108" && !g.NewestIsBad {
			t.Errorf("a future-dated failed snapshot left newest_is_bad=false")
		}
	}
}

// A future-dated point does not cap the rpo window: the runs it would hide
// are still missed.
func TestRPOFuturePointDoesNotCapWindow(t *testing.T) {
	f := newFixture(t)
	addPointAt(f, 105, 200*24*time.Hour, "ok")
	out, _, _ := runCLI(t, "rpo", "--days", "7", "--guest", "105", "--json")
	rows := decodeJSON[rpoReport](t, out).Rows
	if len(rows) != 1 {
		t.Fatalf("want one row:\n%s", out)
	}
	if r := rows[0]; r.WindowCapped || r.MissedRuns == 0 {
		t.Errorf("future point capped the window: capped=%v at=%s missed=%d", r.WindowCapped, r.CappedAt, r.MissedRuns)
	}
}

// --verified-only with partial evidence: JSON says read points were hidden,
// and the human view keeps the not-read line.
func TestVerifiedOnlyPartialTwins(t *testing.T) {
	f := newFixture(t)
	delete(f.pveRoutes, "/nodes/pve1/storage/local/content")
	out, _, _ := runCLI(t, "restore-points", "107", "--verified-only", "--json")
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; g.HiddenUnverified == 0 {
		t.Errorf("JSON does not say read points were hidden: %s", out)
	}
	out, _, _ = runCLI(t, "restore-points", "107", "--verified-only", "--human-friendly")
	if !strings.Contains(out, "none verified") || !strings.Contains(out, "partial: not read") {
		t.Errorf("human view lost a line:\n%s", out)
	}
}

// A PBS-only cluster whose PBS answered but has nothing measurable is not an
// unreachable cluster; one that did not answer (or refused the token) is.
func TestCapacityPBSOnlyAnsweredIsNotUnreachable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, f *fixture)
		want  int
	}{
		{"error-entry", func(t *testing.T, f *fixture) {
			f.pbsRoutes["/status/datastore-usage"] = []map[string]any{{"store": "main", "error": "datastore is not mounted"}}
		}, 0},
		{"unmeasured", func(t *testing.T, f *fixture) {
			f.pbsRoutes["/status/datastore-usage"] = []map[string]any{{"store": "main"}}
		}, 0},
		{"usage-read-refused", func(t *testing.T, f *fixture) {
			delete(f.pbsRoutes, "/status/datastore-usage")
		}, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			t.Setenv("PROXMOX_BASE_URL", "")
			t.Setenv("PROXMOX_HOST", "")
			tc.setup(t, f)
			_, errOut, err := runCLI(t, "capacity", "--json", "--timeout", "2s")
			code := 0
			if err != nil {
				code = ExitCode(err)
			}
			if code != tc.want || (tc.want == 0 && strings.Contains(errOut, "unreachable")) {
				t.Errorf("exit %d, want %d; stderr %q", code, tc.want, errOut)
			}
		})
	}
}

// A PBS-only entry of another client, on a PBS a PVE cluster writes to, is
// not credited with that cluster's datastore or groups.
func TestPBSOnlyOtherClientIsShared(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `
[clusters.beta]
client = "Beta"
pbs_host = "`+f.pbs.URL+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
`))
	out, _, _ := runCLI(t, "--cluster", "beta", "capacity", "--json")
	for _, r := range decodeJSON[capacityReport](t, out).Rows {
		if r.Kind == "pbs-datastore" && r.Name == "main" && (r.Tenant == "Beta" || r.Attribution == "") {
			t.Errorf("beta credited with acme's datastore: tenant %q attribution %q", r.Tenant, r.Attribution)
		}
	}
	out, _, _ = runCLI(t, "--cluster", "beta", "verify-health", "--all", "--json")
	for _, g := range decodeJSON[verifyReport](t, out).Groups {
		if g.Tenant == "Beta" {
			t.Errorf("beta credited with group %s", g.Group)
		}
	}
}

// The coverage all-clear names what it did not check: an uncovered template
// is not "included by an enabled backup job".
func TestCoverageAllClearNamesSkipped(t *testing.T) {
	f := newFixture(t)
	guests := f.pveRoutes["/cluster/resources"].([]map[string]any)
	f.pveRoutes["/cluster/resources"] = []map[string]any{guests[0], {"vmid": 900, "name": "tpl", "type": "qemu", "node": "pve1", "template": 1}}
	out, _, _ := runCLI(t, "backup", "coverage", "--human-friendly")
	if strings.Contains(out, "Every one of") || !strings.Contains(out, "not checked: templates") {
		t.Errorf("all-clear counts a skipped template as covered:\n%s", out)
	}
}

// A snapshot without a readable manifest leaves the store row incomplete.
func TestVerifyHealthManifestlessIsIncomplete(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = append(f.pbsRoutes["/admin/datastore/main/snapshots"].([]any),
		map[string]any{"backup-type": "vm", "backup-id": "100", "backup-time": f.now.Unix() - 60, "owner": "root@pam"})
	out, _, _ := runCLI(t, "verify-health", "--json")
	for _, s := range decodeJSON[verifyReport](t, out).Stores {
		if s.Store == "main" && !s.SnapshotsIncomplete {
			t.Errorf("manifestless snapshot left snapshots_incomplete unset: %+v", s)
		}
	}
}
