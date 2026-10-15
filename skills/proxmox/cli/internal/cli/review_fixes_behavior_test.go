// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Regression tests for the Phase 4.95 review findings (Codex + Claude reviewers).

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/config"
)

// A privilege-separated token without an ACL sees empty lists, not 403: that
// must read as "no data", never as a healthy empty cluster.
func TestNoPrivilegeTokenIsAFailure(t *testing.T) {
	newFixture(t)
	empty := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/api2/json") {
		case "/access/permissions":
			writeData(w, map[string]any{})
		default:
			writeData(w, []any{})
		}
	}))
	defer empty.Close()
	// No harness env here: the empty server answers 200, so nothing retries, and
	// the harness-only PBS loopback fallback stays off (as for a real user).
	t.Setenv("PROXMOX_TLS_FINGERPRINT", "")
	t.Setenv("PROXMOX_BASE_URL", empty.URL+"/api2/json")
	t.Setenv("PROXMOX_PBS_HOST", "")
	t.Setenv("PROXMOX_PBS_API_TOKEN", "")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(empty.Certificate()))
	for _, args := range [][]string{{"posture", "--json"}, {"backup", "coverage", "--json"}, {"rpo", "--json"}, {"failures", "--json"}, {"capacity", "--json"}} {
		if _, stderr, err := runCLI(t, args...); ExitCode(err) != 5 || !strings.Contains(stderr, "holds no privileges") {
			t.Errorf("%v: exit %d, stderr %q", args, ExitCode(err), stderr)
		}
	}
}

func TestWarningsStatusIsNotAFailure(t *testing.T) {
	if !taskOK("WARNINGS: 1") || taskOK("job errors") || !taskOK("OK") {
		t.Fatal("taskOK misclassifies statuses")
	}
}

// On a shared PBS without an explicit namespace, snapshots are attributed only
// through this cluster's own PVE storage config (server + datastore).
func TestSharedPBSAttribution(t *testing.T) {
	f := newFixture(t)
	t.Setenv("ACME_PVE", "auditor@pve!cli=00000000-0000-0000-0000-000000000000")
	t.Setenv("ACME_PBS", "auditor@pbs!cli:11111111-1111-1111-1111-111111111111")
	// The cluster's PVE storage points at 127.0.0.1; reach the same PBS as "localhost",
	// so the storage reference does not match this PBS host.
	pbsAlias := strings.Replace(f.pbs.URL, "127.0.0.1", "localhost", 1)
	writeClustersFile(t, `
[clusters.acme]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
pbs_host = "`+pbsAlias+`"
pbs_token_env = "ACME_PBS"
pbs_fingerprint = "`+f.pbsFingerprint()+`"
`)
	out, stderr, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rep := decodeJSON[postureReport](t, out)
	for _, r := range rep.Rows {
		if r.VMID == 100 && r.RestorePoints != 2 {
			t.Errorf("unattributable PBS snapshots were credited to the guest: %d points", r.RestorePoints)
		}
	}
	if !strings.Contains(stderr, "not attributed") {
		t.Errorf("missing attribution warning: %q", stderr)
	}
}

func TestCapacityKeepsPBSStorageWithoutPBSAPI(t *testing.T) {
	newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	out, _, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, r := range decodeJSON[capacityReport](t, out).Rows {
		names[r.Name] = true
	}
	if !names["pbs1"] || !names["local"] {
		t.Errorf("PBS-backed storage dropped without an authoritative PBS row: %v", names)
	}
}

func TestOneBadProfileDoesNotAbortTheFleet(t *testing.T) {
	f := newFixture(t)
	t.Setenv("GOOD_PVE", "auditor@pve!cli=00000000-0000-0000-0000-000000000000")
	writeClustersFile(t, `
[clusters.good]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "GOOD_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"

[clusters.typo]
pve_host = "pve1"
pve_fingerprint = "AB:CD"
`)
	out, stderr, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatalf("one malformed entry aborted the fleet: %v (%s)", err, stderr)
	}
	rep := decodeJSON[postureReport](t, out)
	if len(rep.Rows) == 0 || !strings.Contains(stderr, "typo") || !strings.Contains(stderr, "PARTIAL: 1 of 2") {
		t.Errorf("want rows from good and a failure for typo; rows=%d stderr=%q", len(rep.Rows), stderr)
	}
	if _, _, err := runCLI(t, "--cluster", "good", "posture", "--json"); err != nil {
		t.Errorf("--cluster good blocked by another entry: %v", err)
	}
	if _, _, err := runCLI(t, "--cluster", "typo", "posture", "--json"); err == nil {
		t.Error("--cluster typo accepted a malformed fingerprint")
	}
}

// Generic [headers] are applied after the per-service token; an Authorization
// entry there would override it for every tenant and service.
func TestConfiguredAuthorizationHeaderDropped(t *testing.T) {
	c := &client.Client{Config: &config.Config{Headers: map[string]string{" authorization": "PVEAPIToken=tenant-a", "X-Trace": "1"}}}
	dropConfiguredAuthorization(c)
	if len(c.Config.Headers) != 1 || c.Config.Headers["X-Trace"] != "1" {
		t.Fatalf("headers after drop: %v", c.Config.Headers)
	}
}

// A PVE base that names the PBS host would send the PVE token to PBS.
func TestPVEBaseAtPBSHostRefused(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_BASE_URL", f.pbs.URL+"/api2/json")
	before := f.pbsHits.Load()
	if _, stderr, err := runCLI(t, "cluster", "resources", "--json"); err == nil || !strings.Contains(stderr+err.Error(), "points at the configured Proxmox Backup Server") {
		t.Errorf("PVE base at the PBS host accepted: %v", err)
	}
	if f.pbsHits.Load() != before {
		t.Error("a request reached PBS")
	}
}

func TestHostFragmentsRejected(t *testing.T) {
	for _, cl := range []clusterProfile{{PBSHost: "pbs1#x"}, {PVEHost: "pve1#x"}, {PVEHost: "pve1?"}, {PVEBaseURL: "https://pve1/api2/json#"}} {
		if err := cl.validate("x"); err == nil {
			t.Errorf("accepted %+v", cl)
		}
	}
}

func TestClustersFileInWritableDirRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o770); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "clusters.toml")
	if err := os.WriteFile(path, []byte("[clusters.a]\npve_host = \"pve1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROXMOX_CLUSTERS_FILE", path)
	if _, _, err := runCLI(t, "clusters", "list", "--json"); err == nil || !strings.Contains(err.Error(), "writable by other users") {
		t.Errorf("group-writable directory accepted: %v", err)
	}
}

// pve_host (the documented form) must never inherit PROXMOX_BASE_URL.
func TestProfileNeverInheritsShellBaseURL(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	t.Setenv("OTHER_PVE", "other@pve!cli=SECRET-OF-OTHER-TENANT")
	// PROXMOX_BASE_URL points at the fixture; the profile names a different host.
	writeClustersFile(t, `
[clusters.other]
pve_host = "pve-other.invalid"
pve_token_env = "OTHER_PVE"
`)
	before := f.pveHits.Load()
	if _, _, err := runCLI(t, "posture", "--json"); ExitCode(err) != 5 {
		t.Errorf("unreachable profile host must be 'no data', got %v", err)
	}
	if f.pveHits.Load() != before {
		t.Fatal("a profile with pve_host sent requests (and its token) to PROXMOX_BASE_URL")
	}
	if err := (clusterProfile{PVEBaseURL: "https://user:pw@pve1/api2/json"}).validate("x"); err == nil {
		t.Error("credentials in pve_base_url accepted")
	}
	if err := (clusterProfile{PBSHost: "http://pbs1:8007"}).validate("x"); err == nil {
		t.Error("plain http pbs_host accepted")
	}
}

// Two PBS services on one host are different services: PVE storage pointing at
// :8007 must not claim the snapshots of the configured PBS on another port.
func TestPBSAttributionComparesPort(t *testing.T) {
	if pbsEndpoint("PBS1", 0) != "pbs1:8007" || pbsEndpoint("pbs1", 9007) == pbsEndpoint("pbs1", 8007) {
		t.Fatal("pbsEndpoint ignores the port")
	}
	if pbsEndpoint("[fd00::1]", 0) != "[fd00::1]:8007" {
		t.Fatalf("IPv6 endpoint: %s", pbsEndpoint("[fd00::1]", 0))
	}
}

// /access/permissions values are propagate bits: a grant on "/" reaches
// /nodes/pve1 only when it propagates; a grant on the path itself always counts.
func TestTokenPrivsPropagation(t *testing.T) {
	p := tokenPrivs{"/": {"Sys.Audit": false}, "/nodes": {"VM.Audit": true}, "/nodes/pve2": {"Sys.Audit": false}}
	switch {
	case p.has("Sys.Audit", "/nodes/pve1"):
		t.Error("non-propagating grant on / reached /nodes/pve1")
	case !p.has("Sys.Audit", "/"):
		t.Error("grant on the path itself ignored")
	case !p.has("Sys.Audit", "/nodes/pve2"), !p.has("VM.Audit", "/nodes/pve1"):
		t.Error("exact or propagated grant missed")
	}
	if err := taskListVisibility(p, false, "pve1"); err == nil {
		t.Error("task list trusted without Sys.Audit on the node")
	}
}

// Merged views of one archive: the newer verify task wins; an unresolved
// contradiction is never reported as verified.
func TestDedupeKeepsFailedVerification(t *testing.T) {
	pve := restorePoint{Source: "pve-storage:pbs1", canonical: "pbs:x", VerifyState: "ok"}
	pbs := restorePoint{Source: "main", canonical: "pbs:x", VerifyState: "failed"}
	if got := dedupeRestorePoints([]restorePoint{pve, pbs}); len(got) != 1 || got[0].VerifyState != "failed" {
		t.Errorf("contradiction resolved to %+v", got)
	}
	pve.VerifyUPID = "UPID:pve1:0001E240:00A1B2C3:6720F0E1:verify::root@pam:"
	pbs.VerifyUPID = "UPID:pbs:00000ABC:0000F00D:00000001:6720F0E0:verify:main:root@pam:"
	if got := dedupeRestorePoints([]restorePoint{pve, pbs}); got[0].VerifyState != "ok" {
		t.Errorf("newer verify task (ok) lost: %+v", got[0])
	}
}

// encrypted-only / verified-only jobs skip snapshots that do not qualify, and a
// run that ended before the snapshot existed did not copy it.
func TestOffsiteRequiresAdmittedSnapshot(t *testing.T) {
	rp := restorePoint{VMID: 100, GuestType: "qemu", Time: 1000, canonical: "pbs:h:8007/main//qemu/100/1000", pbsStore: "main", VerifyState: "ok"}
	job := pbsSyncJob{Store: "main", Direction: "push", LastRunState: "OK", LastRunEnd: 2000, evidenced: map[string]bool{syncEvidenceKey("main", "", "vm/100", 1000): true}}
	if offsiteFor([]restorePoint{rp}, "h:8007", []pbsSyncJob{job}) == "" {
		t.Fatal("qualifying push run not credited")
	}
	enc := job
	enc.EncryptedOnly = true
	early := job
	early.LastRunEnd = 900
	if offsiteFor([]restorePoint{rp}, "h:8007", []pbsSyncJob{enc, early}) != "" {
		t.Error("encrypted-only or pre-snapshot run credited for an unencrypted snapshot")
	}
}

// Capacity with every node's storage list failing has no evidence: no data, not "no storage at risk".
func TestCapacityAllStorageReadsFailedIsNoData(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	broken := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api2/json")
		if strings.HasSuffix(path, "/storage") && strings.HasPrefix(path, "/nodes/") {
			http.Error(w, `{"data":null}`, http.StatusForbidden)
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	defer broken.Close()
	t.Setenv("PROXMOX_BASE_URL", broken.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(broken.Certificate()))
	if _, _, err := runCLI(t, "capacity", "--json"); ExitCode(err) != 5 {
		t.Errorf("capacity with no readable storage: exit %d, want 5", ExitCode(err))
	}
}

// Unread vzdump logs (here: --max-logs 0) leave the last attempt unknown: never
// green, and the cron gate fails on the missing evidence.
func TestPostureLogCapMarksAttemptUnknown(t *testing.T) {
	newFixture(t)
	out, _, err := runCLI(t, "posture", "--json", "--max-logs", "0")
	if err != nil {
		t.Fatal(err)
	}
	unknown := 0
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.Verdict == "fresh" {
			t.Errorf("vmid %d green without reading its last attempt", r.VMID)
		}
		if r.Verdict == "attempt-unknown" {
			unknown++
		}
	}
	if unknown == 0 {
		t.Error("no guest reported attempt-unknown")
	}
	if _, _, err := runCLI(t, "posture", "--json", "--max-logs", "0", "--fail-on-red"); ExitCode(err) != 1 {
		t.Errorf("--fail-on-red passed with unread attempt evidence: %v", err)
	}
}

// A PBS-only failure query on a cluster without PBS has no evidence at all.
func TestFailuresVerifyWithoutPBSIsNoData(t *testing.T) {
	newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	if _, _, err := runCLI(t, "failures", "--type", "verify", "--json"); ExitCode(err) != 5 {
		t.Errorf("failures --type verify without PBS: exit %d, want 5", ExitCode(err))
	}
}

// Merging is order-independent and the newest verify task wins.
func TestMergeVerificationOrderIndependent(t *testing.T) {
	upid := func(start string) string { return "UPID:pve1:0001E240:00A1B2C3:" + start + ":verify::root@pam:" }
	views := []restorePoint{
		{Source: "a", canonical: "pbs:x", VerifyState: "failed", VerifyUPID: upid("00000064")},
		{Source: "b", canonical: "pbs:x", VerifyState: "failed", VerifyUPID: upid("0000012C")},
		{Source: "c", canonical: "pbs:x", VerifyState: "ok", VerifyUPID: upid("000000C8")},
	}
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}, {2, 0, 1}, {1, 2, 0}} {
		in := []restorePoint{views[order[0]], views[order[1]], views[order[2]]}
		if got := dedupeRestorePoints(in); len(got) != 1 || got[0].VerifyState != "failed" {
			t.Errorf("order %v merged to %+v", order, got)
		}
	}
}

// verified-only credits only verification that started before the run did,
// and an archive newer than the run start was not part of it.
func TestVerifiedOnlyNeedsVerificationBeforeRun(t *testing.T) {
	runUPID := "UPID:pbs:00000ABC:0000F00D:00000001:000007D0:syncjob:j1:root@pam:" // run start 2000
	job := pbsSyncJob{Store: "main", Direction: "push", LastRunState: "OK", LastRunEnd: 2500, LastRunUPID: runUPID, VerifiedOnly: true, evidenced: map[string]bool{syncEvidenceKey("main", "", "vm/100", 1000): true}}
	rp := restorePoint{VMID: 100, GuestType: "qemu", Time: 1000, canonical: "pbs:h:8007/main//qemu/100/1000", pbsStore: "main", VerifyState: "ok",
		VerifyUPID: "UPID:pbs:00000ABC:0000F00D:00000001:00000960:verify:main:root@pam:"} // verified at 2400, after the run started
	if offsiteFor([]restorePoint{rp}, "h:8007", []pbsSyncJob{job}) != "" {
		t.Error("verification after the run start qualified the archive retroactively")
	}
	rp.VerifyUPID = "UPID:pbs:00000ABC:0000F00D:00000001:000005DC:verify:main:root@pam:" // 1500
	if offsiteFor([]restorePoint{rp}, "h:8007", []pbsSyncJob{job}) == "" {
		t.Error("archive verified before the run not credited")
	}
	late := rp
	late.Time, late.canonical = 2100, "pbs:h:8007/main//qemu/100/2100"
	if offsiteFor([]restorePoint{late}, "h:8007", []pbsSyncJob{job}) != "" {
		t.Error("archive created after the run started was credited")
	}
}

// An empty storage list from a token without Datastore.Audit is not "no storage at risk".
func TestCapacityFilteredStorageIsNoData(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api2/json")
		switch {
		case path == "/access/permissions":
			writeData(w, map[string]any{"/": map[string]any{"Sys.Audit": 1, "VM.Audit": 1}})
		case strings.HasPrefix(path, "/nodes/") && strings.HasSuffix(path, "/storage"):
			writeData(w, []any{})
		default:
			f.pve.Config.Handler.ServeHTTP(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
	if _, stderr, err := runCLI(t, "capacity", "--json"); ExitCode(err) != 5 || !strings.Contains(stderr, "Datastore.Audit") {
		t.Errorf("filtered storage list: exit %d, stderr %q", ExitCode(err), stderr)
	}
}

// A shell whose own routing is invalid must not block a valid profile.
func TestProfileClientIgnoresShellRouting(t *testing.T) {
	f := newFixture(t)
	t.Setenv("GOOD_PVE", "auditor@pve!cli=00000000-0000-0000-0000-000000000000")
	t.Setenv("PROXMOX_BASE_URL", f.pbs.URL+"/api2/json") // shell PVE base at the shell PBS host
	writeClustersFile(t, `
[clusters.good]
pve_base_url = "`+f.pve.URL+`/api2/json"
pve_token_env = "GOOD_PVE"
pve_fingerprint = "`+f.pveFingerprint()+`"
`)
	if _, stderr, err := runCLI(t, "posture", "--json"); err != nil {
		t.Errorf("valid profile blocked by shell routing: %v (%s)", err, stderr)
	}
}

// verify-health with no readable snapshot list has no verify evidence: no data, not healthy.
func TestVerifyHealthNoSnapshotsReadIsNoData(t *testing.T) {
	f := newFixture(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/snapshots") {
			http.Error(w, `{"data":null}`, http.StatusForbidden)
			return
		}
		f.pbs.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	t.Setenv("PROXMOX_PBS_HOST", strings.TrimPrefix(srv.URL, "https://"))
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
	if _, _, err := runCLI(t, "verify-health", "--json"); ExitCode(err) != 5 {
		t.Errorf("verify-health without snapshot evidence: exit %d, want 5", ExitCode(err))
	}
}

// An undated disagreeing view keeps the archive failed in every order.
func TestReduceVerificationAllPermutations(t *testing.T) {
	upid := func(start string) string { return "UPID:pve1:0001E240:00A1B2C3:" + start + ":verify::root@pam:" }
	views := []restorePoint{
		{Source: "a", canonical: "pbs:x", VerifyState: "failed"},
		{Source: "b", canonical: "pbs:x", VerifyState: "ok", VerifyUPID: upid("00000064")},
		{Source: "c", canonical: "pbs:x", VerifyState: "ok", VerifyUPID: upid("000000C8")},
	}
	for _, o := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		if got := dedupeRestorePoints([]restorePoint{views[o[0]], views[o[1]], views[o[2]]}); got[0].VerifyState != "failed" {
			t.Errorf("order %v -> %s", o, got[0].VerifyState)
		}
	}
	older := []restorePoint{{Source: "a", canonical: "pbs:y", VerifyState: "failed", VerifyUPID: upid("00000064")}, {Source: "b", canonical: "pbs:y", VerifyState: "ok", VerifyUPID: upid("000000C8")}}
	if got := dedupeRestorePoints(older); got[0].VerifyState != "ok" {
		t.Errorf("newer ok verification should win over an older failure, got %s", got[0].VerifyState)
	}
}

// A storage whose content could not be read leaves "fresh" unprovable.
func TestPostureUnreadStorageIsNotGreen(t *testing.T) {
	f := newFixture(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			http.Error(w, `{"data":null}`, http.StatusForbidden)
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
	out, _, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.Verdict == "fresh" {
			t.Errorf("vmid %d green with an unread storage", r.VMID)
		}
	}
}

// One node refused, one filtered empty: zero capacity evidence is no data.
func TestCapacityMixedFailureAndFilteredIsNoData(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	n := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api2/json")
		switch {
		case path == "/nodes":
			writeData(w, []map[string]any{{"node": "pve1", "status": "online"}, {"node": "pve2", "status": "online"}})
		case strings.HasPrefix(path, "/nodes/pve2/") && strings.HasSuffix(path, "/storage"):
			http.Error(w, `{"data":null}`, http.StatusForbidden)
		case strings.HasSuffix(path, "/storage") && strings.HasPrefix(path, "/nodes/"):
			n++
			writeData(w, []any{})
		default:
			f.pve.Config.Handler.ServeHTTP(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
	if _, _, err := runCLI(t, "capacity", "--json"); ExitCode(err) != 5 {
		t.Errorf("mixed refused/filtered storage: exit %d, want 5", ExitCode(err))
	}
}

// An enabled backup storage that is not active is missing evidence, not an empty storage.
func TestInactiveStorageIsMissingEvidence(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api2/json")
		if strings.HasPrefix(path, "/nodes/") && strings.HasSuffix(path, "/storage") {
			writeData(w, []map[string]any{{"storage": "nfs-backup", "type": "nfs", "content": "backup", "active": 0, "shared": 1}})
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
	if _, _, err := runCLI(t, "capacity", "--json"); ExitCode(err) != 5 {
		t.Errorf("capacity with only an inactive storage: exit %d, want 5", ExitCode(err))
	}
	out, _, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.Severity == "green" && r.CoveredByJob {
			t.Errorf("vmid %d green while its backup storage is unavailable", r.VMID)
		}
	}
}

// An offline node's tasks and local storage are missing evidence.
func TestOfflineNodeIsMissingEvidence(t *testing.T) {
	f := newFixture(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/api2/json") == "/nodes" {
			writeData(w, []map[string]any{{"node": "pve1", "status": "online"}, {"node": "pve2", "status": "offline"}})
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
	out, stderr, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.Verdict == "fresh" {
			t.Errorf("vmid %d green with node pve2 offline", r.VMID)
		}
	}
	if !strings.Contains(stderr, "node pve2") {
		t.Errorf("offline node not reported: %q", stderr)
	}
}

// A covering job whose target storage this token cannot see leaves the guest unproven.
func TestHiddenJobStorageIsNotGreen(t *testing.T) {
	cov := &coverage{Jobs: []backupJob{{ID: "job1", Storage: "hidden"}, {ID: "job2", Storage: "local"}}, EnabledFor: map[int64][]string{100: {"job1"}, 101: {"job2"}}}
	targets := newJobTargets(context.Background(), nil, cov)
	if gaps := jobStorageGaps(cov, 100, "pve1", map[string]bool{"pbs1": true}, targets); len(gaps) != 1 {
		t.Fatalf("gaps = %v", gaps)
	}
	if gaps := jobStorageGaps(cov, 100, "pve1", map[string]bool{"hidden": true}, targets); len(gaps) != 0 {
		t.Fatalf("read shared storage reported as a gap: %v", gaps)
	}
	// Node-local storage: reading local on pve1 does not vouch for a guest on pve2.
	if gaps := jobStorageGaps(cov, 101, "pve2", map[string]bool{"local@pve1": true}, targets); len(gaps) != 1 {
		t.Fatalf("local@pve1 satisfied a pve2 guest: %v", gaps)
	}
	if gaps := jobStorageGaps(cov, 101, "pve2", map[string]bool{"local@pve2": true}, targets); len(gaps) != 0 {
		t.Fatalf("local@pve2 read but reported as a gap: %v", gaps)
	}
}

// A vzdump task that failed before processing any guest (lock, job-start hook)
// supersedes older attempts: no guest stays green, and the gate trips.
func TestUnattributedFailedVzdumpTask(t *testing.T) {
	f := newFixture(t)
	upid := "UPID:pve1:00001234:00ABCDEF:" + strconv.FormatInt(f.now.Unix(), 16) + ":vzdump::root@pam:"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api2/json")
		switch {
		case strings.HasSuffix(path, "/tasks") && strings.HasPrefix(path, "/nodes/"):
			writeData(w, []map[string]any{{"upid": upid, "node": "pve1", "type": "vzdump", "id": "", "status": "job errors",
				"starttime": f.now.Add(-time.Hour).Unix(), "endtime": f.now.Add(-time.Hour + time.Minute).Unix()}})
		case strings.HasSuffix(path, "/log") && strings.Contains(path, "/tasks/"):
			writeData(w, []map[string]any{{"n": 1, "t": "ERROR: can't acquire lock '/var/run/vzdump.lock' - got timeout"}})
		default:
			f.pve.Config.Handler.ServeHTTP(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
	out, _, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.Verdict == "fresh" {
			t.Errorf("vmid %d green after a failed vzdump run with unknown per-guest outcome", r.VMID)
		}
	}
	if _, _, err := runCLI(t, "posture", "--json", "--fail-on-red"); ExitCode(err) != 1 {
		t.Errorf("--fail-on-red passed after an unattributed failed vzdump task: %v", err)
	}
}

// A job without a storage inherits the node default (vzdump.conf, else local);
// when that target is not read, posture is amber and --fail-on-red trips.
func TestInheritedJobStorageGapTripsGate(t *testing.T) {
	f := newFixture(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api2/json")
		switch {
		case path == "/cluster/backup":
			writeData(w, []map[string]any{{"id": "job1", "enabled": 1, "schedule": "21:00", "all": 1, "exclude": "103"}})
		case strings.HasSuffix(path, "/vzdump/defaults"):
			writeData(w, map[string]any{"storage": "hidden-nfs"})
		default:
			f.pve.Config.Handler.ServeHTTP(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
	out, stderr, err := runCLI(t, "posture", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[postureReport](t, out).Rows {
		if r.CoveredByJob && r.Severity == "green" {
			t.Errorf("vmid %d green though job1 writes to an unread inherited target", r.VMID)
		}
	}
	if !strings.Contains(stderr, "hidden-nfs") {
		t.Errorf("gap not reported: %q", stderr)
	}
	if _, _, err := runCLI(t, "posture", "--json", "--fail-on-red"); ExitCode(err) != 1 {
		t.Errorf("--fail-on-red passed with an unread backup target: %v", err)
	}
}

// Assurance commands read live state on every run, never the response cache.
func TestAssuranceReadsBypassCache(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	if _, _, err := runCLI(t, "capacity", "--json"); err != nil {
		t.Fatal(err)
	}
	first := f.pveHits.Load()
	if _, _, err := runCLI(t, "capacity", "--json"); err != nil {
		t.Fatal(err)
	}
	if second := f.pveHits.Load() - first; second < first {
		t.Errorf("second run made %d requests vs %d: answered from cache", second, first)
	}
}

// A job interrupted after some guests leaves the guests it never reached
// without an outcome: with --all they become unknown, with an explicit list
// they are incomplete; never green, and the gate trips.
func TestAbortedVzdumpJobUnreachedGuests(t *testing.T) {
	for _, tc := range []struct {
		cmdline string
		log     []string
		want    string
	}{
		{"vzdump --all 1 --storage pbs1", nil, "attempt-unknown"},
		{"vzdump 101 100 --storage pbs1", nil, "last-attempt-failed"},
		// 100 lives on another node: this node's aborted task never owned it.
		{"vzdump 101 100 --storage pbs1", []string{"INFO: skip external VMs: 100"}, "fresh"},
		// Died in the job-start hook: only the listed guest is affected, others are not blinded.
		{"vzdump 101 --storage pbs1", []string{"ERROR: job-start hook failed"}, "fresh"},
	} {
		f := newFixture(t)
		upid := "UPID:pve1:00001234:00ABCDEF:" + strconv.FormatInt(f.now.Unix(), 16) + ":vzdump::root@pam:"
		log := append([]string{"INFO: starting new backup job: " + tc.cmdline}, tc.log...)
		if tc.log == nil || strings.Contains(tc.log[0], "skip external") {
			log = append(log, "INFO: Starting Backup of VM 101 (qemu)", "ERROR: Backup of VM 101 failed - interrupted by signal",
				"ERROR: Backup job failed - interrupted by signal")
		}
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/api2/json")
			switch {
			case strings.HasSuffix(path, "/tasks") && strings.HasPrefix(path, "/nodes/"):
				writeData(w, []map[string]any{{"upid": upid, "node": "pve1", "type": "vzdump", "id": "", "status": "interrupted by signal",
					"starttime": f.now.Add(-time.Hour).Unix(), "endtime": f.now.Add(-time.Hour + time.Minute).Unix()}})
			case strings.HasSuffix(path, "/log") && strings.Contains(path, "/tasks/"):
				lines := []map[string]any{}
				for i, l := range log {
					lines = append(lines, map[string]any{"n": i + 1, "t": l})
				}
				writeData(w, lines)
			default:
				f.pve.Config.Handler.ServeHTTP(w, r)
			}
		}))
		t.Setenv("PROXMOX_BASE_URL", srv.URL+"/api2/json")
		t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(srv.Certificate()))
		out, _, err := runCLI(t, "posture", "--json")
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range decodeJSON[postureReport](t, out).Rows {
			if r.VMID == 100 && r.Verdict != tc.want {
				t.Errorf("%s: vmid 100 verdict %s, want %s", tc.cmdline, r.Verdict, tc.want)
			}
		}
		if _, _, err := runCLI(t, "posture", "--json", "--fail-on-red"); ExitCode(err) != 1 {
			t.Errorf("%s: --fail-on-red passed after an aborted job: %v", tc.cmdline, err)
		}
		srv.Close()
	}
}

// Only a snapshot the last run's own log shows it pushed counts: not a group
// added to the filter later, not another namespace, not a skipped snapshot.
func TestOffsiteNeedsRunLogEvidence(t *testing.T) {
	rp := restorePoint{VMID: 100, GuestType: "qemu", Time: 1000, canonical: "pbs:h:8007/main//qemu/100/1000", pbsStore: "main", VerifyState: "ok"}
	job := pbsSyncJob{Store: "main", Direction: "push", LastRunState: "OK", LastRunEnd: 2000, GroupFilter: []string{"group:vm/100", "group:vm/101"}}
	job.evidenced = map[string]bool{syncEvidenceKey("main", "", "vm/101", 1000): true}
	if offsiteFor([]restorePoint{rp}, "h:8007", []pbsSyncJob{job}) != "" {
		t.Error("offsite credited for a group the last run never pushed")
	}
	snap := time.Unix(1000, 0).UTC().Format(time.RFC3339)
	lines := []string{
		"2026-10-13T07:00:00+02:00: ----",
		"2026-10-13T07:00:00+02:00: Syncing datastore 'main', root namespace into datastore 'dr', root namespace",
		"2026-10-13T07:01:02+02:00: [vm/100]: " + snap + ": sync done",
		"2026-10-13T07:01:03+02:00: Syncing datastore 'main', namespace 'tenant-a' into datastore 'dr', namespace 'tenant-a'",
		"2026-10-13T07:01:04+02:00: [ct/200]: skipped: 2 snapshot(s) (a .. b) - older than the newest snapshot present on sync target",
		"2026-10-13T07:01:05+02:00: [vm/300]: Failed to push group vm/300 to remote!",
	}
	got := parseSyncEvidence(lines)
	if len(got) != 1 || !got[syncEvidenceKey("main", "", "vm/100", 1000)] {
		t.Errorf("evidence parse = %v (skips and failures must not count)", got)
	}
	job.evidenced = got
	if offsiteFor([]restorePoint{rp}, "h:8007", []pbsSyncJob{job}) == "" {
		t.Error("pushed snapshot not credited")
	}
	older := rp
	older.Time, older.canonical = 900, "pbs:h:8007/main//qemu/100/900"
	if offsiteFor([]restorePoint{older}, "h:8007", []pbsSyncJob{job}) != "" {
		t.Error("a snapshot the run did not push was credited")
	}
	other := rp
	other.pbsNS, other.canonical = "tenant-b", "pbs:h:8007/main/tenant-b/qemu/100/1000"
	job.evidenced = map[string]bool{syncEvidenceKey("main", "tenant-a", "vm/100", 1000): true}
	if offsiteFor([]restorePoint{other}, "h:8007", []pbsSyncJob{job}) != "" {
		t.Error("tenant-a evidence credited tenant-b")
	}
}
