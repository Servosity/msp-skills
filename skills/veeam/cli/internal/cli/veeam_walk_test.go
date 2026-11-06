// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// The seven local-mirror reports count only the records the last finished,
// UNRESTRICTED sync walk of each partition returned (review class "incomplete
// read coverage"). last_attempt_complete=1 proves only that the requested walk
// ended, not that it covered the collection. These tests drive the real sync
// command against a loopback fake VSPC that answers with the documented paging
// envelope (spec.json PagingInformation), then run the real reports.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"veeam-pp-cli/internal/cliutil"
	"veeam-pp-cli/internal/cliutil/testenv"
	"veeam-pp-cli/internal/store"
)

// walkJobs is the sync partition of /infrastructure/backupServers/jobs
// (spec.json BackupServerJob), the partition Codex's failing input narrows.
const walkJobs = "infrastructure-backup-servers-jobs"

// walkFake serves documented VSPC list paths, paged by the offset and limit
// query parameters, inside the documented envelope
// {"meta":{"pagingInfo":{"total":N,"count":n,"offset":o}},"data":[...]}.
// A path with no list answers with an empty page; a path in failFrom answers
// 409 for any page at or past that offset (an interrupted walk).
type walkFake struct {
	mu       sync.Mutex
	lists    map[string][]map[string]any
	failFrom map[string]int
}

func (f *walkFake) set(path string, items ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists[path] = items
}

func (f *walkFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api/v3")
	list := f.lists[path]
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if from, ok := f.failFrom[path]; ok && offset >= from {
		http.Error(w, `{"errors":[{"message":"conflict"}]}`, http.StatusConflict)
		return
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 {
		limit = 100
	}
	lo := min(max(offset, 0), len(list))
	hi := min(lo+limit, len(list))
	page := list[lo:hi]
	if page == nil {
		page = []map[string]any{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"meta": map[string]any{"pagingInfo": map[string]any{"total": len(list), "count": len(page), "offset": offset}},
		"data": page,
	})
}

// startWalkFake points the CLI at a fresh fake VSPC from an isolated home and
// returns the fake plus the local mirror path every command in the test uses.
func startWalkFake(t *testing.T) (*walkFake, string) {
	t.Helper()
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, v := range []string{cliutil.VerifyEnvVar, cliutil.DogfoodEnvVar, cliutil.MCPChildEnvVar, noLocalDataEnvVar} {
		t.Setenv(v, "")
	}
	f := &walkFake{lists: map[string][]map[string]any{}, failFrom: map[string]int{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("VEEAM_BASE_URL", srv.URL+"/api/v3")
	t.Setenv("VEEAM_TOKEN", "test-token")
	return f, filepath.Join(home, "walk.db")
}

func walkCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// walkSync runs the real sync command against the fake. --rate-limit 0 only
// stops the client pacing loopback requests; it does not narrow the walk.
func walkSync(t *testing.T, db string, args ...string) {
	t.Helper()
	full := append([]string{"sync", "--db", db, "--json", "--rate-limit", "0"}, args...)
	if out, err := walkCLI(t, full...); err != nil {
		t.Fatalf("%v: %v\n%s", full, err, out)
	}
}

// walkReport runs a report and decodes its JSON view.
func walkReport(t *testing.T, db, report string, args ...string) map[string]any {
	t.Helper()
	full := append([]string{report, "--db", db, "--json"}, args...)
	out, err := walkCLI(t, full...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", full, err, out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%v: decoding %v\n%s", full, err, out)
	}
	return v
}

// walkPartitions is every partition the reports read, optionally without some.
func walkPartitions(t *testing.T, except ...string) string {
	t.Helper()
	var out []string
	for _, p := range veeamNovelPaths {
		for _, name := range veeamSyncNames(p) {
			if !slices.Contains(except, name) && !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	return strings.Join(out, ",")
}

// walkTotals returns fleet-health's totals row.
func walkTotals(t *testing.T, v map[string]any) map[string]float64 {
	t.Helper()
	raw, ok := v["totals"].(map[string]any)
	if !ok {
		t.Fatalf("fleet-health has no totals: %v", v)
	}
	out := map[string]float64{}
	for k, x := range raw {
		if f, ok := x.(float64); ok {
			out[k] = f
		}
	}
	return out
}

func wantTotals(t *testing.T, label string, got map[string]float64, want map[string]float64) {
	t.Helper()
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: fleet-health totals.%s = %v, want %v (totals %v)", label, k, got[k], w, got)
		}
	}
}

// walkRecord reads the membership record sync keeps for a partition.
func walkRecord(t *testing.T, db, partition string) (state string, members []string) {
	t.Helper()
	s, err := store.OpenReadOnly(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var raw string
	if err := s.DB().QueryRow(`SELECT state, members FROM veeam_walk WHERE resource_type = ?`, partition).Scan(&state, &raw); err != nil {
		t.Fatalf("reading the %s walk record: %v", partition, err)
	}
	if err := json.Unmarshal([]byte(raw), &members); err != nil {
		t.Fatalf("decoding the %s walk members %q: %v", partition, raw, err)
	}
	slices.Sort(members)
	return state, members
}

// writeThrough stores items into a partition the way a live `get` write-through
// or an import does: rows land in the mirror without any sync walk.
func writeThrough(t *testing.T, db, partition string, items ...map[string]any) {
	t.Helper()
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	raw := make([]json.RawMessage, 0, len(items))
	for _, it := range items {
		b, _ := json.Marshal(it)
		raw = append(raw, b)
	}
	if stored, _, err := s.UpsertBatch(partition, raw); err != nil || stored != len(items) {
		t.Fatalf("write-through into %s: stored=%d err=%v", partition, stored, err)
	}
}

func walkJob(uid, status string) map[string]any {
	return map[string]any{"instanceUid": uid, "status": status, "isEnabled": true}
}

const (
	walkFailedUID  = "11111111-1111-4111-8111-111111111111"
	walkSuccessUID = "22222222-2222-4222-8222-222222222222"
)

// Codex's failing input, verbatim: with every other partition complete and
// the job partition empty, a `--param offset=1` walk over two jobs ordered
// Failed, Success stores only the Success job and finishes. fleet-health must
// refuse (naming the unrestricted sync that fixes it), never report one success
// and zero failures. The same holds for every other way a user narrows a walk.
func TestNarrowedWalkIsNotTheWholePartition(t *testing.T) {
	for _, narrow := range [][]string{
		{"--param", "offset=1"},
		{"--resource-param", walkJobs + ":offset=1"},
		{"--global-param", "offset=1"},
	} {
		t.Run(strings.Join(narrow, " "), func(t *testing.T) {
			f, db := startWalkFake(t)
			f.set(veeamPathServerJobs, walkJob(walkFailedUID, "Failed"), walkJob(walkSuccessUID, "Success"))
			walkSync(t, db, "--resources", walkPartitions(t, walkJobs))
			walkSync(t, db, append([]string{"--resources", walkJobs}, narrow...)...)

			out, err := walkCLI(t, "fleet-health", "--db", db, "--json")
			if err == nil {
				t.Fatalf("fleet-health over a narrowed job walk returned a report instead of refusing:\n%s", out)
			}
			if want := "veeam-cli sync --resources " + walkJobs; !strings.Contains(err.Error(), want) {
				t.Errorf("refusal %q does not name the fixing command %q", err, want)
			}

			// The unrestricted walk proves both jobs; a later narrowed walk,
			// stamped complete, does not replace that proof.
			walkSync(t, db, "--resources", walkJobs)
			walkSync(t, db, append([]string{"--resources", walkJobs}, narrow...)...)
			wantTotals(t, "after a full then a narrowed walk", walkTotals(t, walkReport(t, db, "fleet-health")),
				map[string]float64{"jobs_total": 2, "failed": 1, "success": 1})
		})
	}
}

// A walk capped by --max-pages and not resumed proves nothing, so the reports
// refuse. Resumed by a plain sync, the walk is one walk across two runs: the
// first page (fetched by the earlier run) still counts, and a job the API
// deleted since the previous full walk does not.
func TestWalkResumedAfterMaxPagesKeepsEveryPage(t *testing.T) {
	f, db := startWalkFake(t)
	limit := determinePaginationDefaults(walkJobs).limit
	page := make([]map[string]any, 0, limit+1)
	for i := 0; i < limit; i++ {
		page = append(page, walkJob(fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "Success"))
	}
	f.set(veeamPathServerJobs, append(slices.Clone(page), walkJob(walkFailedUID, "Failed"))...)
	walkSync(t, db, "--resources", walkPartitions(t))

	// The failed job is deleted; a warning job now sits on the second page.
	f.set(veeamPathServerJobs, append(slices.Clone(page), walkJob(walkSuccessUID, "Warning"))...)
	walkSync(t, db, "--resources", walkJobs, "--max-pages", "1")
	for _, report := range []string{"fleet-health", "stale-backups"} {
		out, err := walkCLI(t, report, "--db", db, "--json")
		if err == nil {
			t.Fatalf("%s over a walk stopped by --max-pages returned a report:\n%s", report, out)
		}
		if want := "veeam-cli sync --resources " + walkJobs; !strings.Contains(err.Error(), want) {
			t.Errorf("%s refusal %q does not name the fixing command %q", report, err, want)
		}
	}

	walkSync(t, db, "--resources", walkJobs)
	wantTotals(t, "after the resumed walk", walkTotals(t, walkReport(t, db, "fleet-health")),
		map[string]float64{"jobs_total": float64(limit + 1), "success": float64(limit), "warning": 1, "failed": 0})
}

// Rows the latest full walk did not return are not current: a job and an alarm
// the API stopped returning (deleted, cleared), and rows a live read or import
// wrote straight into the mirror.
func TestReportsCountOnlyTheLatestFullWalk(t *testing.T) {
	f, db := startWalkFake(t)
	f.set(veeamPathAlarmTemplates, map[string]any{"instanceUid": "tpl", "name": "Job failed"})
	alarm := func(uid, status, object string) map[string]any {
		return map[string]any{"instanceUid": uid, "alarmTemplateUid": "tpl",
			"object":         map[string]any{"objectName": object},
			"lastActivation": map[string]any{"status": status, "message": object}}
	}
	f.set(veeamPathServerJobs, walkJob(walkFailedUID, "Failed"), walkJob(walkSuccessUID, "Success"))
	f.set(veeamPathActiveAlarms, alarm("al-gone", "Error", "cleared-srv"), alarm("al-live", "Warning", "live-srv"))
	walkSync(t, db, "--resources", walkPartitions(t))

	f.set(veeamPathServerJobs, walkJob(walkSuccessUID, "Success"))
	f.set(veeamPathActiveAlarms, alarm("al-live", "Warning", "live-srv"))
	walkSync(t, db, "--resources", walkPartitions(t))
	writeThrough(t, db, walkJobs, walkJob("33333333-3333-4333-8333-333333333333", "Failed"))
	writeThrough(t, db, veeamSyncNames(veeamPathActiveAlarms)[0], alarm("al-read", "Error", "read-srv"))

	wantTotals(t, "stale and written-through rows", walkTotals(t, walkReport(t, db, "fleet-health")),
		map[string]float64{"jobs_total": 1, "failed": 0, "success": 1, "active_alarms": 1, "alarm_errors": 0})
	v := walkReport(t, db, "alarms-triage")
	alarms := arr(v, "alarms")
	if len(alarms) != 1 || alarms[0]["object"] != "live-srv" {
		t.Errorf("alarms-triage = %v, want only the live-srv alarm the latest walk returned", alarms)
	}
}

// A dry-run sync sends no request and must not touch walk membership: the
// record is byte-identical afterwards and the reports still read exactly the
// last finished walk (a written-through row stays out).
func TestDryRunSyncLeavesWalkMembershipUnchanged(t *testing.T) {
	f, db := startWalkFake(t)
	f.set(veeamPathServerJobs, walkJob(walkFailedUID, "Failed"), walkJob(walkSuccessUID, "Success"))
	walkSync(t, db, "--resources", walkPartitions(t))
	beforeState, beforeMembers := walkRecord(t, db, walkJobs)

	f.set(veeamPathServerJobs, walkJob(walkSuccessUID, "Success"))
	writeThrough(t, db, walkJobs, walkJob("33333333-3333-4333-8333-333333333333", "Warning"))
	walkSync(t, db, "--resources", walkJobs, "--dry-run")
	walkSync(t, db, "--resources", walkJobs, "--dry-run", "--full")

	afterState, afterMembers := walkRecord(t, db, walkJobs)
	if beforeState != afterState || !slices.Equal(beforeMembers, afterMembers) {
		t.Errorf("dry-run changed the walk: %s %v -> %s %v", beforeState, beforeMembers, afterState, afterMembers)
	}
	wantTotals(t, "after dry-runs", walkTotals(t, walkReport(t, db, "fleet-health")),
		map[string]float64{"jobs_total": 2, "failed": 1, "success": 1, "warning": 0})
}

// Guard: an unrestricted full walk still reports every record, and it records
// its proof for every partition the reports read.
func TestUnrestrictedFullWalkReportsEveryJob(t *testing.T) {
	f, db := startWalkFake(t)
	f.set(veeamPathServerJobs, walkJob(walkFailedUID, "Failed"), walkJob(walkSuccessUID, "Success"))
	walkSync(t, db, "--resources", walkPartitions(t))

	wantTotals(t, "full walk", walkTotals(t, walkReport(t, db, "fleet-health")),
		map[string]float64{"jobs_total": 2, "failed": 1, "success": 1})
	state, members := walkRecord(t, db, walkJobs)
	if want := []string{walkFailedUID, walkSuccessUID}; state != "finished" || !slices.Equal(members, want) {
		t.Errorf("%s walk = %s %v, want finished %v", walkJobs, state, members, want)
	}
	for _, name := range strings.Split(walkPartitions(t), ",") {
		if state, _ := walkRecord(t, db, name); state != "finished" {
			t.Errorf("%s walk = %q after a full sync, want finished", name, state)
		}
	}
}

// A walk the API interrupted part-way (page 2 failed) proves nothing, and a
// plain sync would resume it from page 2 without the first page's proof. The
// reports refuse and name the --full sync that restarts it; a plain resume is
// still refused; the named --full sync proves the list.
func TestInterruptedWalkNamesTheSyncThatFixesIt(t *testing.T) {
	f, db := startWalkFake(t)
	limit := determinePaginationDefaults(walkJobs).limit
	jobs := make([]map[string]any, 0, limit+1)
	for i := 0; i < limit; i++ {
		jobs = append(jobs, walkJob(fmt.Sprintf("00000000-0000-4000-8000-%012d", i), "Success"))
	}
	f.set(veeamPathServerJobs, append(jobs, walkJob(walkFailedUID, "Failed"))...)
	walkSync(t, db, "--resources", walkPartitions(t, walkJobs))

	f.mu.Lock()
	f.failFrom[veeamPathServerJobs] = limit
	f.mu.Unlock()
	if out, err := walkCLI(t, "sync", "--db", db, "--json", "--rate-limit", "0", "--resources", walkJobs); err == nil {
		t.Fatalf("sync with page 2 failing reported success:\n%s", out)
	}
	f.mu.Lock()
	delete(f.failFrom, veeamPathServerJobs)
	f.mu.Unlock()

	out, err := walkCLI(t, "fleet-health", "--db", db, "--json")
	if err == nil {
		t.Fatalf("fleet-health over an interrupted walk returned a report:\n%s", out)
	}
	if want := "veeam-cli sync --full --resources " + walkJobs; !strings.Contains(err.Error(), want) {
		t.Fatalf("refusal %q does not name the sync that fixes it, %q", err, want)
	}
	walkSync(t, db, "--resources", walkJobs)
	if out, err := walkCLI(t, "fleet-health", "--db", db, "--json"); err == nil {
		t.Fatalf("a plain resume of an interrupted walk was taken as the whole list:\n%s", out)
	}
	walkSync(t, db, "--full", "--resources", walkJobs)
	wantTotals(t, "after the named --full sync", walkTotals(t, walkReport(t, db, "fleet-health")),
		map[string]float64{"jobs_total": float64(limit + 1), "success": float64(limit), "failed": 1})
}
