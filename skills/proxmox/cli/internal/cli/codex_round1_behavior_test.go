// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// Codex round 2.

// --cluster narrows the report, not the collision check; and a storage
// without a PBS fingerprint still matches one that pins it at the same server.
func TestSharedPBSDetectionSeesUnselectedClustersAndEitherIdentity(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	// beta's PVE pins the PBS fingerprint on its storage; acme's does not.
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
			port, _ := strconv.Atoi(hp[1])
			writeData(w, []map[string]any{{"storage": "pbs1", "type": "pbs", "server": hp[0], "port": port, "datastore": "main", "namespace": "", "fingerprint": f.pbsFingerprint()}})
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	defer other.Close()
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
pve_base_url = "`+other.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(other.Certificate())+`"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "VMIDs collide") {
		t.Errorf("--cluster acme must still see beta writing the same namespace:\n%s", out)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.VMID == 100 && r.Verdict == "fresh" {
			t.Errorf("acme/100 credited with a snapshot beta may own: %s", r.Reason)
		}
	}
}

// A guest whose ACL keeps VM.Audit but drops VM.Backup has its archives
// filtered out of a fleet-wide listing: that storage is unread, not empty.
func TestBackupListingNeedsVMBackupOnEveryGuest(t *testing.T) {
	f := newFixture(t)
	f.pveRoutes["/access/permissions"] = map[string]any{
		"/":        map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "Datastore.Audit": 1, "Datastore.AllocateSpace": 1, "VM.Backup": 1},
		"/vms/105": map[string]any{"VM.Audit": 1},
	}
	t.Setenv("PROXMOX_PBS_HOST", "")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "")
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", "")
	rep, rows := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["storage pbs1"]; e == "" {
		t.Errorf("pbs1's fleet listing is filtered for 105; want a fetch failure: %+v", rep.FetchFailures)
	}
	if r := rows[105]; r.Verdict == "never" {
		t.Errorf("105 = never while its archives were filtered out: %q", r.Reason)
	}
}

// PBS reports usage only for datastores the token may audit.
func TestCapacityPBSDatastoreListCompleteness(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/access/permissions"] = map[string]any{"/": map[string]any{"Sys.Audit": true}, "/datastore/main": map[string]any{"Datastore.Audit": true}}
	out, _, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if e := failureByScope(decodeJSON[capacityReport](t, out).FetchFailures)["pbs datastores"]; e == "" {
		t.Errorf("a datastore list the token may not fully see must be flagged:\n%s", out)
	}
}

// A disabled backup storage (a former target) cannot be listed: unread.
func TestDisabledBackupStorageIsUnread(t *testing.T) {
	f := newFixture(t)
	st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
	f.pveRoutes["/nodes/pve1/storage"] = append(st, map[string]any{"storage": "oldnfs", "type": "nfs", "shared": 1, "active": 0, "enabled": 0, "content": "backup"})
	rep, rows := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["storage oldnfs"]; !strings.Contains(e, "disabled") {
		t.Errorf("want the disabled storage reported: %+v", rep.FetchFailures)
	}
	if r := rows[105]; r.Verdict == "never" {
		t.Errorf("105 = never while a disabled storage may hold its points: %q", r.Reason)
	}
}

// restore-points: a dumpdir job target is never read, so evidence is partial;
// unreadable jobs leave the points unmarked rather than storage-retention marked.
func TestRestorePointsJobTargetGapsAndUnknownRetention(t *testing.T) {
	f := newFixture(t)
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	delete(jobs[0], "storage")
	jobs[0]["dumpdir"] = "/mnt/backup"
	out, _, err := runCLI(t, "restore-points", "105", "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 5 {
		t.Errorf("105 has no readable point and an unread dumpdir target: want exit 5, got %v", err)
	}
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; g.Evidence != "partial" || !strings.Contains(strings.Join(g.NotRead, ";"), "dumpdir") {
		t.Errorf("want the dumpdir target in not_read: %+v", g)
	}

	f = newFixture(t)
	delete(f.pveRoutes, "/cluster/backup")
	out, _, err = runCLI(t, "restore-points", "100", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rep := decodeJSON[restorePointsReport](t, out)
	for _, rp := range rep.Guests[0].RestorePoints {
		if rp.PruneMark != "" {
			t.Errorf("with the jobs unreadable the retention is unknown; got prune_mark %q", rp.PruneMark)
		}
	}
	if rep.Guests[0].Evidence != "partial" {
		t.Errorf("unknown jobs mean unknown targets: %+v", rep.Guests[0])
	}
}

// Codex round 3.

// Unknown ownership counts as shared: a peer naming the same PBS whose client
// cannot be built, or whose storage list the token may not fully see, keeps
// acme from being credited (posture, and rpo's met schedule).
func TestUnknownPBSOwnershipIsNotCredited(t *testing.T) {
	for name, beta := range map[string]func(f *fixture) string{
		"client cannot be built": func(f *fixture) string {
			return `
[clusters.beta]
pve_base_url = "https://127.0.0.1:9/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "zz"
pbs_host = "` + f.pbs.URL + `"
`
		},
		"storage list hidden": func(f *fixture) string {
			other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/api2/json/storage"):
					writeData(w, []any{})
				case strings.HasSuffix(r.URL.Path, "/access/permissions"):
					writeData(w, map[string]any{"/": map[string]any{"Sys.Audit": 1, "VM.Audit": 1}, "/storage": map[string]any{"Datastore.Audit": 0}})
				default:
					f.pve.Config.Handler.ServeHTTP(w, r)
				}
			}))
			t.Cleanup(other.Close)
			return `
[clusters.beta]
pve_base_url = "` + other.URL + `/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "` + client.CertFingerprint(other.Certificate()) + `"
pbs_host = "` + f.pbs.URL + `"
`
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			setAcmeTokens(t)
			writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+beta(f))
			out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "could not be read") {
				t.Errorf("acme must report that beta's use of the PBS is unknown:\n%s", out)
			}
			for _, r := range decodeJSON[postureReport](t, out).Rows {
				if r.VMID == 100 && r.Verdict == "fresh" {
					t.Errorf("acme/100 credited while beta's ownership is unknown: %s", r.Reason)
				}
			}
			out, _, err = runCLI(t, "--cluster", "acme", "rpo", "--days", "1", "--by-client", "--json")
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range decodeJSON[[]rpoClientRow](t, out) {
				if c.GuestsMet != 0 {
					t.Errorf("met schedule proven from snapshots of unknown ownership: %+v", c)
				}
			}
		})
	}
}

// pbs_namespace covers the namespaces below it: points left in acme/old after
// the storage moved to acme still count.
func TestPBSNamespaceDescendantsAreAttributed(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	auditOnly(f) // only the PBS API can show them
	f.pbsNS = "acme"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "acme"}, {"ns": "acme/old"}}
	f.pbsRoutes["/admin/datastore/main/snapshots#acme"] = []any{}
	f.pbsRoutes["/admin/datastore/main/snapshots#acme/old"] = []any{
		map[string]any{"backup-type": "vm", "backup-id": "105", "backup-time": ago(f.now, 3), "size": 1 << 30, "owner": "root@pam", "verification": map[string]any{"state": "ok", "upid": "UPID:pbs:verify"}},
	}
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`))
	_, rows := postureByVMID(t)
	if r := rows[105]; r.RestorePoints == 0 {
		t.Errorf("105's point in acme/old was not counted: %s %q", r.Verdict, r.Reason)
	}
}

// A snapshot listed without a readable manifest is uncertain: restore-points
// says so, and verify-health reports it.
func TestUnreadableManifestIsReported(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/datastore/main/snapshots"] = append(f.pbsRoutes["/admin/datastore/main/snapshots"].([]any),
		map[string]any{"backup-type": "vm", "backup-id": "105", "backup-time": ago(f.now, 1), "owner": "root@pam"})
	out, _, _ := runCLI(t, "restore-points", "105", "--json")
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; g.Evidence != "partial" || !strings.Contains(strings.Join(g.NotRead, ";"), "readable manifest") {
		t.Errorf("restore-points must flag the manifest-less snapshot: %+v", g)
	}
	out, _, _ = runCLI(t, "verify-health", "--json")
	if !strings.Contains(out, "readable manifest") {
		t.Errorf("verify-health must report the manifest-less snapshot:\n%s", out)
	}
}

// keep-all, or keep counts that are all zero, prune nothing.
func TestRetentionThatKeepsEverythingIsUnpruned(t *testing.T) {
	f := newFixture(t)
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["prune-backups"] = "keep-all=1"
	f.pbsRoutes["/admin/prune"] = []map[string]any{{"id": "p1", "store": "main", "keep-last": 0}}
	out, _, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[capacityReport](t, out).Rows {
		if r.Kind == "pbs-datastore" && !strings.Contains(strings.Join(r.Flags, ","), "unpruned") {
			t.Errorf("keep-all and keep-last=0 prune nothing; want unpruned: %+v", r)
		}
	}
	for opts, want := range map[string]bool{"keep-last=3": true, "keep-all=1,keep-last=3": false, "keep-daily=0": false, "": false} {
		if got := retentionPrunes(parseRetention(opts)); got != want {
			t.Errorf("retentionPrunes(%q) = %v, want %v", opts, got, want)
		}
	}
}

// A guest that met its schedule is met even while another storage is unread.
func TestRPOMetGuestIsNotUnproven(t *testing.T) {
	f := newFixture(t)
	auditOnly(f) // local@pve1 unread
	out, _, err := runCLI(t, "rpo", "--days", "1", "--by-client", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range decodeJSON[[]rpoClientRow](t, out) {
		if c.GuestsMet == 0 || c.GuestsMet+c.GuestsUnproven > c.Guests {
			t.Errorf("met guests must not also be unproven: %+v", c)
		}
	}
}

// Codex round 4.

// pbsOtherPVE serves the fixture PVE as another cluster whose pbs1 storage
// writes to ns (and pins fp when set).
func pbsOtherPVE(t *testing.T, f *fixture, ns, fp string) *httptest.Server {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
			port, _ := strconv.Atoi(hp[1])
			writeData(w, []map[string]any{{"storage": "pbs1", "type": "pbs", "server": hp[0], "port": port, "datastore": "main", "namespace": ns, "fingerprint": fp}})
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A namespace below pbs_namespace that another cluster writes to is not acme's.
func TestDescendantWrittenByAnotherClusterIsNotAttributed(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	auditOnly(f)
	f.pbsNS = "clients/acme"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "clients"}, {"ns": "clients/acme"}, {"ns": "clients/acme/beta"}}
	f.pbsRoutes["/admin/datastore/main/snapshots#clients/acme"] = []any{}
	f.pbsRoutes["/admin/datastore/main/snapshots#clients/acme/beta"] = []any{
		map[string]any{"backup-type": "vm", "backup-id": "100", "backup-time": ago(f.now, 1), "size": 1 << 30, "owner": "root@pam", "verification": map[string]any{"state": "ok", "upid": "UPID:pbs:verify"}},
	}
	beta := pbsOtherPVE(t, f, "clients/acme/beta", "")
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "clients/acme"`)+`
[clusters.beta]
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.VMID == 100 && r.RestorePoints > 0 {
			t.Errorf("acme/100 credited with beta's snapshot in clients/acme/beta: %s %q", r.Verdict, r.Reason)
		}
	}
}

// An unreadable peer pinning the same PBS certificate may write there, even
// when its PBS host is spelled differently.
func TestUnreadablePeerMatchedByFingerprint(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsFP = f.pbsFingerprint()
	port := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")[1]
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
pve_base_url = "https://127.0.0.1:9/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "zz"
pbs_host = "https://localhost:`+port+`"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "beta (could not be read)") {
		t.Errorf("beta pins the same PBS certificate; acme's ownership is unknown:\n%s", out)
	}
}

// With several clusters configured, a PBS storage whose config could not be
// read cannot be tied to a namespace: its listing is not counted.
func TestPBSStorageWithoutConfigIsNotCounted(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	acmePVE := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			http.Error(w, `{"data":null}`, http.StatusInternalServerError)
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	defer acmePVE.Close()
	beta := pbsOtherPVE(t, f, "beta", "")
	writeClustersFile(t, `
[clusters.acme]
pve_base_url = "`+acmePVE.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(acmePVE.Certificate())+`"

[clusters.beta]
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if e := failureByScope(decodeJSON[postureReport](t, out).FetchFailures)["storage pbs1"]; !strings.Contains(e, "storage config of pbs1 could not be read") {
		t.Errorf("want pbs1 refused without its config:\n%s", out)
	}
}

// verify-health marks rows in a namespace another configured cluster writes to.
func TestVerifyHealthMarksSharedNamespace(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	beta := pbsOtherPVE(t, f, "", "")
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "verify-health", "--all", "--json")
	if err != nil {
		t.Fatal(err)
	}
	groups := decodeJSON[verifyReport](t, out).Groups
	if len(groups) == 0 {
		t.Fatalf("no groups:\n%s", out)
	}
	for _, g := range groups {
		if !strings.HasPrefix(g.Attribution, "shared") || g.Tenant != "" {
			t.Errorf("group %s in a shared namespace must say so and credit no tenant: %+v", g.Group, g)
		}
	}
}

// A namespace list the PBS token may not fully see is incomplete.
func TestPBSNamespaceListCompleteness(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsNS = "acme"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "acme"}}
	f.pbsRoutes["/access/permissions"] = map[string]any{
		"/":                    map[string]any{"Sys.Audit": true, "Datastore.Audit": true},
		"/datastore/main/acme": map[string]any{"Datastore.Audit": false},
	}
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`))
	rep, _ := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["pbs"]; !strings.Contains(e, "propagate") {
		t.Errorf("posture must flag the namespace list below acme as incomplete: %+v", rep.FetchFailures)
	}
	out, _, _ := runCLI(t, "verify-health", "--json")
	if !strings.Contains(out, "propagate") {
		t.Errorf("verify-health must flag it too:\n%s", out)
	}
}

// A job's own retention overrides the storage's: keep-all on every job means
// nothing prunes, whatever the storage says.
func TestJobRetentionOverridesStorage(t *testing.T) {
	f := newFixture(t)
	f.pbsPrune = "keep-last=3"
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	jobs[0]["prune-backups"] = "keep-all=1"
	out, _, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[capacityReport](t, out).Rows {
		if r.Kind == "pbs-datastore" && !strings.Contains(strings.Join(r.Flags, ","), "unpruned") {
			t.Errorf("every job keeps all: want unpruned despite the storage's keep-last: %+v", r)
		}
	}
}

// restore-points exits 5 when no match has a readable point and any is partial.
func TestNothingReadableAcrossMatches(t *testing.T) {
	full := guestRestorePoints{Evidence: "full"}
	partial := guestRestorePoints{Evidence: "partial"}
	withPoint := guestRestorePoints{Evidence: "partial", Count: 1}
	for _, c := range []struct {
		in   []guestRestorePoints
		want bool
	}{
		{[]guestRestorePoints{full, partial}, true},
		{[]guestRestorePoints{full}, false},
		{[]guestRestorePoints{withPoint, partial}, false},
	} {
		if got := nothingReadable(c.in); got != c.want {
			t.Errorf("nothingReadable(%+v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// Human rpo says when misses are unproven.
func TestRPOHumanShowsPartialEvidence(t *testing.T) {
	f := newFixture(t)
	auditOnly(f)
	out, _, err := runCLI(t, "rpo", "--days", "7", "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "partial evidence") || !strings.Contains(out, "EVIDENCE") {
		t.Errorf("human rpo must flag partial evidence:\n%s", out)
	}
}

// Adversarial round 6.

func prodSnapshots(f *fixture) []any { return f.pbsRoutes["/admin/datastore/main/snapshots"].([]any) }

// An unreadable peer that declares its own pbs_namespace may write only there:
// one client offline must not blank every other client's PBS evidence. One
// that declares none may write anywhere on that PBS.
func TestUnreadablePeerIsScopedByItsDeclaredNamespace(t *testing.T) {
	for name, c := range map[string]struct {
		betaNS    string
		wantFresh bool
	}{"beta declares beta": {"beta", true}, "beta declares nothing": {"", false}} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			setAcmeTokens(t)
			auditOnly(f)
			f.pbsNS = "acme"
			f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "acme"}, {"ns": "beta"}}
			f.pbsRoutes["/admin/datastore/main/snapshots#acme"] = prodSnapshots(f)
			decl := ""
			if c.betaNS != "" {
				decl = `pbs_namespace = "` + c.betaNS + `"`
			}
			writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`)+`
[clusters.beta]
pve_base_url = "https://127.0.0.1:9/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "zz"
pbs_host = "`+f.pbs.URL+`"
`+decl+"\n")
			out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
			if err != nil {
				t.Fatal(err)
			}
			fresh := false
			for _, r := range decodeJSON[postureReport](t, out).Rows {
				fresh = fresh || (r.VMID == 100 && r.Verdict == "fresh")
			}
			if fresh != c.wantFresh {
				t.Errorf("acme/100 fresh = %v, want %v:\n%s", fresh, c.wantFresh, out)
			}
		})
	}
}

// A declared pbs_namespace owns its subtree: points beta left in msp/beta/old
// are not acme's, even under acme's broader pbs_namespace "msp".
func TestDeclaredNamespaceOwnsItsSubtree(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	auditOnly(f)
	f.pbsNS = "msp"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "msp"}, {"ns": "msp/beta"}, {"ns": "msp/beta/old"}, {"ns": "msp/beta/x"}}
	for _, ns := range []string{"msp", "msp/beta", "msp/beta/x"} {
		f.pbsRoutes["/admin/datastore/main/snapshots#"+ns] = []any{}
	}
	f.pbsRoutes["/admin/datastore/main/snapshots#msp/beta/old"] = []any{
		map[string]any{"backup-type": "vm", "backup-id": "105", "backup-time": ago(f.now, 3), "size": 1 << 30, "owner": "root@pam", "verification": map[string]any{"state": "ok", "upid": "UPID:pbs:verify"}},
	}
	beta := pbsOtherPVE(t, f, "msp/beta/x", "")
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "msp"`)+`
[clusters.beta]
pve_base_url = "`+beta.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+client.CertFingerprint(beta.Certificate())+`"
pbs_host = "`+f.pbs.URL+`"
pbs_namespace = "msp/beta"
`)
	out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.VMID == 105 && r.RestorePoints > 0 {
			t.Errorf("acme/105 credited with beta's point in msp/beta/old: %s %q", r.Verdict, r.Reason)
		}
	}
}
