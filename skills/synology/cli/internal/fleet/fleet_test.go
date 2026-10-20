// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Behavioral tests: a fake DSM serves ABB + storage payloads in
// the shapes cited in spec.yaml, Fetch parses them, CommitSync mirrors them,
// and the reports must flag exactly the broken things.

package fleet

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/store"
)

var now = time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)

func ts(ago time.Duration) int64 { return now.Add(-ago).Unix() }

// fakeDSM serves one NAS. healthy=true makes every task, device and disk fine.
func fakeDSM(t *testing.T, healthy bool) *httptest.Server {
	t.Helper()
	failStatus, volStatus, smart, temp, used := 4, "degraded", "warning", 60, "900"
	if healthy {
		failStatus, volStatus, smart, temp, used = 2, "normal", "normal", 35, "100"
	}
	data := map[string]string{
		"SYNO.Core.System/info": `{"model":"DS920+","firmware_ver":"DSM 7.2.2-72806"}`,
		"SYNO.ActiveBackup.Task/list": fmt.Sprintf(`{"tasks":[
 {"task_id":5,"task_name":"Office PCs","backup_type":2,"next_trigger_time":%d,
  "last_result":{"result_id":600,"status":2,"job_action":131072,"time_start":%d,"time_end":%d},
  "devices":[{"device_id":11,"host_name":"BOB-LAPTOP","os_name":"Windows 11","agent_status":"online","backup_type":2},
             {"device_id":12,"host_name":"NEW-PC","os_name":"Windows 11","agent_status":"offline","backup_type":2}]},
 {"task_id":8,"task_name":"File Server","backup_type":4,"next_trigger_time":%d,
  "last_result":{"result_id":593,"status":%d,"job_action":1,"time_start":%d,"time_end":%d},
  "devices":[{"device_id":21,"host_name":"FS01","backup_type":4}]},
 {"task_id":9,"task_name":"Weekly Archive","backup_type":4,"next_trigger_time":%d,
  "last_result":{"result_id":400,"status":2,"job_action":1,"time_start":%d,"time_end":%d},"devices":[]},
 {"task_id":10,"task_name":"Running VM","backup_type":1,"next_trigger_time":%d,"progress":{"running_task_status":1},
  "last_result":{"result_id":601,"status":1,"job_action":1,"time_start":%d,"time_end":0},"devices":[]}]}`,
			ts(-12*time.Hour), ts(2*time.Hour), ts(time.Hour),
			ts(-12*time.Hour), failStatus, ts(10*time.Hour), ts(9*time.Hour),
			ts(-48*time.Hour), ts(10*24*time.Hour), ts(10*24*time.Hour-time.Hour),
			ts(-12*time.Hour), ts(time.Hour)),
		"SYNO.ActiveBackup.Log/list_result": fmt.Sprintf(`{"count":4,"results":[
 {"result_id":600,"task_id":5,"task_name":"Office PCs","status":2,"job_action":131072,"time_start":%d,"time_end":%d},
 {"result_id":590,"task_id":5,"task_name":"Office PCs","status":2,"job_action":1,"time_start":%d,"time_end":%d,"task_config":{"device_list":[{"host_name":"BOB-LAPTOP"}]}},
 {"result_id":593,"task_id":8,"task_name":"File Server","status":%d,"job_action":1,"time_start":%d,"time_end":%d,"error_count":3,"task_config":{"device_list":[{"host_name":"FS01"}]}},
 {"result_id":400,"task_id":9,"task_name":"Weekly Archive","status":2,"job_action":1,"time_start":%d,"time_end":%d}]}`,
			ts(2*time.Hour), ts(time.Hour),
			ts(72*time.Hour), ts(71*time.Hour),
			failStatus, ts(10*time.Hour), ts(9*time.Hour),
			ts(10*24*time.Hour), ts(10*24*time.Hour-time.Hour)),
		"SYNO.ActiveBackup.Log/list_result_detail": `{"count":2,"result_detail_list":[
 {"log_level":2,"log_type":1104,"other_params":{"path":""}},
 {"log_level":0,"log_type":6002,"error_code":0,"other_params":{"path":"D:/share","fs_error":-65,"os_name":"smb"}}]}`,
		"SYNO.ActiveBackup.Overview/list_device_transfer_size": fmt.Sprintf(`{"total":2,"device_list":[
 {"device":{"device_id":11,"host_name":"BOB-LAPTOP"},"transfer_list":[
   {"config_device_id":11,"device_name":"BOB-LAPTOP","device_result_id":1,"result_id":588,"status":2,"time_start":%d,"time_end":%d,"transfered_bytes":100},
   {"config_device_id":11,"device_name":"BOB-LAPTOP","device_result_id":2,"result_id":591,"status":2,"time_start":%d,"time_end":%d,"transfered_bytes":100},
   {"config_device_id":11,"device_name":"BOB-LAPTOP","device_result_id":3,"result_id":590,"status":2,"time_start":%d,"time_end":%d,"transfered_bytes":100}]},
 {"device":{"device_id":12,"host_name":"NEW-PC"},"transfer_list":[
   {"config_device_id":12,"device_name":"NEW-PC","device_result_id":4,"result_id":593,"status":5,"time_start":%d,"time_end":%d}]}]}`,
			ts(120*time.Hour), ts(120*time.Hour-60*time.Second),
			ts(96*time.Hour), ts(96*time.Hour-60*time.Second),
			ts(72*time.Hour), ts(72*time.Hour-600*time.Second),
			ts(10*time.Hour), ts(9*time.Hour)),
		"SYNO.Storage.CGI.Storage/load_info": fmt.Sprintf(`{
 "volumes":[{"id":"volume_1","vol_path":"/volume1","status":%q,"size":{"total":"1000","used":%q},"space_status":{"detail":"fs_almost_full"}},
            {"id":"volume_2","vol_path":"/volume2","status":"attention","size":{"total":"1000","used":"10"}}],
 "storagePools":[{"id":"reuse_1","status":"normal","device_type":"shr","size":{"total":"2000","used":"1000"}}],
 "disks":[{"id":"sata1","name":"Drive 1","device":"/dev/sata1","model":"WD40EFRX","temp":%d,"status":"normal","smart_status":%q,"size_total":"4000","remain_life":{"value":97,"trustable":true}}]}`,
			volStatus, used, temp, smart),
	}
	if healthy {
		data["SYNO.Storage.CGI.Storage/load_info"] = strings.Replace(data["SYNO.Storage.CGI.Storage/load_info"], `"attention"`, `"normal"`, 1)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		q := r.URL.Query()
		body, ok := data[q.Get("api")+"/"+q.Get("method")]
		if !ok {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":` + body + `}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	s, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := EnsureSchema(context.Background(), s.DB()); err != nil {
		t.Fatal(err)
	}
	return s.DB()
}

func syncFake(t *testing.T, db *sql.DB, n NAS, healthy bool) {
	t.Helper()
	srv := fakeDSM(t, healthy)
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	snap, info, err := Fetch(context.Background(), c, 0, now, DefaultOptions)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if err := CommitSync(context.Background(), db, n, now, snap, info, nil); err != nil {
		t.Fatal(err)
	}
}

const day = 24 * time.Hour

func TestFailuresFlagsOnlyBrokenTasks(t *testing.T) {
	db := openDB(t)
	acme := NAS{Name: "acme", Client: "Acme"}
	syncFake(t, db, acme, false)
	got, err := Failures(context.Background(), db, []NAS{acme}, now.Add(-day), now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]Failure{}
	for _, f := range got {
		kinds[f.TaskName] = f
	}
	fs, ok := kinds["File Server"]
	if !ok || fs.Kind != "run_failed" || fs.Status != "failed" || fs.Errors != 3 {
		t.Fatalf("File Server failure missing or wrong: %+v (all %+v)", fs, got)
	}
	if !strings.Contains(fs.FirstError, "log_type=6002") || !strings.Contains(fs.FirstError, "fs_error=-65") {
		t.Fatalf("first_error = %q, want the error-level detail row", fs.FirstError)
	}
	// Office PCs' newest job is a version deletion; its newest backup (72h ago)
	// is outside the 24h window, so it is a no-run, not a success.
	if op := kinds["Office PCs"]; op.Kind != "no_run_in_window" {
		t.Fatalf("Office PCs = %+v, want no_run_in_window", op)
	}
	if _, ok := kinds["Running VM"]; ok {
		t.Fatal("running task must not be reported")
	}
	if wa := kinds["Weekly Archive"]; wa.Kind != "no_run_in_window" {
		t.Fatalf("Weekly Archive = %+v", wa)
	}
	// Widening the window clears the weekly task and the 72h-old success.
	wide, _ := Failures(context.Background(), db, []NAS{acme}, now.Add(-14*day), now, 26*time.Hour)
	for _, f := range wide {
		if f.TaskName != "File Server" {
			t.Fatalf("unexpected failure with a 14d window: %+v", f)
		}
	}
}

func TestFailuresHealthyNASIsEmpty(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "good"}
	syncFake(t, db, n, true)
	got, err := Failures(context.Background(), db, []NAS{n}, now.Add(-14*day), now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("healthy NAS reported failures: %+v", got)
	}
}

func TestUnsyncedNASNeverReadsOK(t *testing.T) {
	db := openDB(t)
	ghost := NAS{Name: "ghost"}
	f, _ := Failures(context.Background(), db, []NAS{ghost}, now.Add(-day), now, 26*time.Hour)
	if len(f) != 1 || f[0].Kind != "nas_not_synced" {
		t.Fatalf("failures = %+v", f)
	}
	st, _ := Status(context.Background(), db, []NAS{ghost}, now.Add(-day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	if len(st) != 1 || st[0].State != SevUnknown {
		t.Fatalf("status = %+v, want unknown", st)
	}
	// A failed last attempt after an older success is also not fresh.
	_ = RecordAttempt(context.Background(), db, ghost, now.Add(-2*time.Hour), SystemInfo{}, nil)
	_ = RecordAttempt(context.Background(), db, ghost, now.Add(-time.Hour), SystemInfo{}, fmt.Errorf("DSM login: HTTP 502"))
	st, _ = Status(context.Background(), db, []NAS{ghost}, now.Add(-day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	if st[0].State != SevUnknown || !strings.Contains(st[0].SyncError, "502") {
		t.Fatalf("status after failed sync = %+v", st[0])
	}
}

func TestStaleDevices(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	syncFake(t, db, n, false)
	got, err := Stale(context.Background(), db, []NAS{n}, 36*time.Hour, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	byHost := map[string]StaleDevice{}
	for _, d := range got {
		byHost[d.HostName] = d
	}
	if d := byHost["BOB-LAPTOP"]; d.Kind != "stale" || d.AgeHours < 71 || d.AgeHours > 73 {
		t.Fatalf("BOB-LAPTOP = %+v, want stale ~72h", d)
	}
	if d := byHost["NEW-PC"]; d.Kind != "never_succeeded" {
		t.Fatalf("NEW-PC = %+v, want never_succeeded (only a canceled run)", d)
	}
	if d, ok := byHost["FS01"]; !ok || d.Kind != "never_succeeded" {
		t.Fatalf("FS01 = %+v, want never_succeeded (only a failed run)", d)
	}
	// A 4-day threshold keeps BOB (72h) out.
	got, _ = Stale(context.Background(), db, []NAS{n}, 96*time.Hour, now, 26*time.Hour)
	for _, d := range got {
		if d.HostName == "BOB-LAPTOP" {
			t.Fatalf("BOB-LAPTOP flagged at 96h threshold: %+v", d)
		}
	}
}

func TestHealthThresholds(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	syncFake(t, db, n, false)
	got, err := Health(context.Background(), db, []NAS{n}, 85, 55, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	for _, h := range got {
		problems = append(problems, h.ID+":"+h.Severity.String()+":"+h.Problem)
	}
	joined := strings.Join(problems, "|")
	for _, want := range []string{"volume_1:critical:status degraded", "volume_2:warning:status attention",
		"sata1:warning:SMART warning", "sata1:warning:temperature 60C >= 55C", "volume_1:warning:90.0% used >= 85%"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	if strings.Contains(joined, "reuse_1") {
		t.Errorf("normal pool flagged: %s", joined)
	}
	if got[0].Severity != SevCritical {
		t.Errorf("issues not sorted worst-first: %s", joined)
	}

	healthyDB := openDB(t)
	syncFake(t, healthyDB, n, true)
	if clean, _ := Health(context.Background(), healthyDB, []NAS{n}, 85, 55, now, 26*time.Hour); len(clean) != 0 {
		t.Fatalf("healthy NAS reported storage issues: %+v", clean)
	}
}

func TestStatusRollup(t *testing.T) {
	db := openDB(t)
	bad, good := NAS{Name: "acme"}, NAS{Name: "good"}
	syncFake(t, db, bad, false)
	syncFake(t, db, good, true)
	rows, err := Status(context.Background(), db, []NAS{bad, good}, now.Add(-14*day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].State != SevCritical || rows[0].TasksFailed != 1 || rows[0].TasksRunning != 1 || rows[0].Model != "DS920+" {
		t.Fatalf("acme row = %+v", rows[0])
	}
	if rows[1].State != SevWarning || rows[1].Storage != SevOK {
		// The healthy fake still has NEW-PC (canceled only) and FS01 never-succeeded
		// devices, which are stale protection, not storage problems.
		t.Fatalf("good row = %+v", rows[1])
	}
}

func TestStorageSeverity(t *testing.T) {
	for in, want := range map[string]Severity{"normal": SevOK, "": SevUnknown, "crashed": SevCritical, "degraded": SevCritical,
		"attention": SevWarning, "healthy": SevWarning, "something_new": SevWarning} {
		if got := StorageSeverity(in); got != want {
			t.Errorf("StorageSeverity(%q) = %v want %v", in, got, want)
		}
	}
}

func TestFindDevice(t *testing.T) {
	db := openDB(t)
	acme := NAS{Name: "acme", Client: "Acme"}
	syncFake(t, db, acme, false)
	got, err := FindDevice(context.Background(), db, []NAS{acme}, "bob", 2, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != "device" || got[0].HostName != "BOB-LAPTOP" || got[0].Client != "Acme" || len(got[0].RecentRuns) != 2 || got[0].LastSuccess == 0 {
		t.Fatalf("FindDevice = %+v", got)
	}
	none, _ := FindDevice(context.Background(), db, []NAS{acme}, "zzz-nothing", 2, now, 26*time.Hour)
	if none == nil || len(none) != 0 {
		t.Fatalf("no-match result = %#v, want empty non-nil slice", none)
	}
	// A registered NAS that never synced is reported, not silently skipped.
	withGhost, _ := FindDevice(context.Background(), db, []NAS{acme, {Name: "ghost"}}, "bob", 2, now, 26*time.Hour)
	if len(withGhost) != 2 || withGhost[1].Kind != "nas_not_synced" || withGhost[1].NAS != "ghost" {
		t.Fatalf("FindDevice with unsynced NAS = %+v", withGhost)
	}
	// An unregistered NAS's leftover rows are not returned.
	if other, _ := FindDevice(context.Background(), db, []NAS{{Name: "ghost"}}, "bob", 2, now, 26*time.Hour); len(other) != 1 || other[0].Kind != "nas_not_synced" {
		t.Fatalf("FindDevice for another NAS = %+v", other)
	}
}

func TestTrendOverrun(t *testing.T) {
	db := openDB(t)
	acme := NAS{Name: "acme"}
	syncFake(t, db, acme, false)
	rows, err := Trend(context.Background(), db, []NAS{acme}, now.Add(-30*day), true, 2, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var bob TrendRow
	for _, r := range rows {
		if r.Key == "BOB-LAPTOP" {
			bob = r
		}
	}
	// Three runs of 60s, 60s, 600s: median 60s, last 600s > 2x median.
	if bob.Runs != 3 || bob.MedianSec != 60 || bob.LastSec == nil || *bob.LastSec != 600 || !bob.Overrun || bob.SuccessRatePct != 100 {
		t.Fatalf("BOB trend = %+v", bob)
	}
	tasks, _ := Trend(context.Background(), db, []NAS{acme, {Name: "ghost"}}, now.Add(-30*day), false, 2, now, 26*time.Hour)
	for _, r := range tasks {
		if r.Key == "File Server" && (r.Kind != "task" || r.Failures != 1 || r.SuccessRatePct != 0) {
			t.Fatalf("File Server trend = %+v", r)
		}
	}
	if last := tasks[len(tasks)-1]; last.Kind != "nas_not_synced" || last.NAS != "ghost" {
		t.Fatalf("unsynced NAS missing from trend: %+v", last)
	}
}

func TestFetchWithoutABB(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		if r.URL.Query().Get("api") == "SYNO.Storage.CGI.Storage" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"volumes":[],"disks":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	snap, info, err := Fetch(context.Background(), c, 0, now, DefaultOptions)
	// System info 102 is reported, ABB 102 means "not installed", not an error.
	if snap == nil || snap.ABBInstalled || info.ABBInstalled == nil || *info.ABBInstalled {
		t.Fatalf("snap=%+v info=%+v", snap, info)
	}
	if err == nil || strings.Contains(err.Error(), "ABB") {
		t.Fatalf("err = %v, want only the system-info error", err)
	}
}

func TestFetchLoginFailureAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"error":{"code":400}}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "bad"}, RatePerSec: 1000}
	snap, _, err := Fetch(context.Background(), c, 0, now, DefaultOptions)
	if snap != nil || err == nil || !strings.Contains(err.Error(), "invalid account or password") {
		t.Fatalf("snap=%v err=%v", snap, err)
	}
}

func TestRegistryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg", "fleet.json")
	r, err := LoadRegistry(path)
	if err != nil || len(r.NAS) != 0 {
		t.Fatalf("missing registry: %v %+v", err, r)
	}
	r.Put(NAS{Name: "zeta", URL: "https://z:5001", EnvPrefix: DefaultEnvPrefix("zeta")})
	r.Put(NAS{Name: "acme-hq", URL: "https://a:5001", EnvPrefix: DefaultEnvPrefix("acme-hq")})
	r.Put(NAS{Name: "zeta", URL: "https://z2:5001", EnvPrefix: "X"})
	if err := r.Save(path); err != nil {
		t.Fatal(err)
	}
	// Windows reports 0666 for every file (ACLs, not mode bits).
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("registry mode = %v", fi.Mode().Perm())
	}
	back, _ := LoadRegistry(path)
	if len(back.NAS) != 2 || back.NAS[0].Name != "acme-hq" || back.NAS[0].EnvPrefix != "SYNOLOGY_ACME_HQ" {
		t.Fatalf("registry = %+v", back.NAS)
	}
	if z, _ := back.Get("zeta"); z.URL != "https://z2:5001" {
		t.Fatalf("Put did not replace: %+v", z)
	}
	if !back.Remove("zeta") || back.Remove("zeta") {
		t.Fatal("Remove semantics")
	}
	raw, _ := os.ReadFile(path)
	var anyMap map[string]any
	_ = json.Unmarshal(raw, &anyMap)
	if strings.Contains(string(raw), "password") {
		t.Fatal("registry must never hold a password")
	}
	if !ValidName("acme-hq") || ValidName("Bad Name") || ValidName("") {
		t.Fatal("ValidName")
	}
}

func TestPurgeNAS(t *testing.T) {
	db := openDB(t)
	syncFake(t, db, NAS{Name: "acme"}, false)
	if err := PurgeNAS(context.Background(), db, "acme"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM abb_results WHERE nas = 'acme'`).Scan(&n)
	if n != 0 {
		t.Fatalf("rows left after purge: %d", n)
	}
}

// TestPathsTraceToSpec: every DSM call the fleet layer makes is a path spec.yaml
// declares verbatim (the spec cites a public source for each operation).
func TestPathsTraceToSpec(t *testing.T) {
	var spec []byte
	for _, p := range []string{"../../spec.yaml", "../../../spec.yaml"} {
		if b, err := os.ReadFile(p); err == nil {
			spec = b
			break
		}
	}
	if spec == nil {
		t.Skip("spec.yaml not shipped next to this module")
	}
	// A Windows checkout with core.autocrlf=true ends every line in CRLF.
	text := strings.ReplaceAll(string(spec), "\r\n", "\n")
	for _, p := range []string{PathAPIInfo, PathSystemInfo, PathABBTasks, PathABBResults, PathABBResultDetail,
		PathABBLogs, PathABBTransfers, PathStorage, PathHyperBackupTasks} {
		if !strings.Contains(text, "path: "+p+"\n") {
			t.Errorf("path %s is not declared in spec.yaml", p)
		}
	}
}

func TestCredentialsFallBackToCachedDeviceID(t *testing.T) {
	testenv.Isolate(t)
	n := NAS{Name: "acme", URL: "https://nas.acme.example:5001", EnvPrefix: "SYNOLOGY_ACME"}
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "a")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "p")
	if n.Credentials().DeviceID != "" {
		t.Fatal("unexpected device token")
	}
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := dsm.SaveDeviceID(dir, n.URL, "a", "did-1"); err != nil {
		t.Fatal(err)
	}
	if got := n.Credentials().DeviceID; got != "did-1" {
		t.Fatalf("cached device token not used: %q", got)
	}
	t.Setenv("SYNOLOGY_ACME_DEVICE_ID", "did-env")
	if got := n.Credentials().DeviceID; got != "did-env" {
		t.Fatalf("env token must win: %q", got)
	}
}

// --- review regressions (false-GREEN paths) ---

func TestHealthNeedsCurrentSnapshot(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	syncFake(t, db, n, true)
	// A later successful sync that wrote no storage snapshot: the older one is
	// not today's answer.
	if err := RecordAttempt(context.Background(), db, n, now.Add(time.Hour), SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := Health(context.Background(), db, []NAS{n}, 85, 55, now.Add(time.Hour), 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Severity != SevUnknown || got[0].Kind != "storage_snapshot" {
		t.Fatalf("Health without a current snapshot = %+v", got)
	}
	st, _ := Status(context.Background(), db, []NAS{n}, now.Add(-14*day), 36*time.Hour, 85, 55, now.Add(time.Hour), 26*time.Hour)
	if st[0].Storage != SevUnknown || st[0].State == SevOK {
		t.Fatalf("status with no current snapshot = %+v", st[0])
	}
}

func TestEmptyStorageStatusIsNotOK(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	snap := &Snapshot{TakenAt: now, Storage: []StorageItem{
		{Kind: "volume", ID: "volume_1", Status: "normal", SizeTotal: 100, SizeUsed: 10},
		{Kind: "disk", ID: "sata1", Status: "", SmartStatus: ""},
	}}
	if err := CommitSync(context.Background(), db, n, now, snap, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := Health(context.Background(), db, []NAS{n}, 85, 55, now, 26*time.Hour)
	joined := fmt.Sprint(got)
	if !strings.Contains(joined, "status not reported") || !strings.Contains(joined, "SMART status not reported") {
		t.Fatalf("empty statuses read as OK: %+v", got)
	}
}

func TestRunningDoesNotHideStaleOrStuckJobs(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	snap := &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "Hung", Running: true, Scheduled: true, NextTrigger: ts(-day), LastStart: ts(5 * day), RunStart: ts(5 * day),
			Devices: []Device{{DeviceID: 7, HostName: "PC-7"}}}},
		DeviceRuns: []DeviceRun{{DeviceResultID: 1, DeviceID: 7, DeviceName: "PC-7", Status: StatusSuccess, Start: ts(30 * day), End: ts(30*day - time.Hour)}},
		Storage:    []StorageItem{{Kind: "volume", ID: "v1", Status: "normal", SizeTotal: 10, SizeUsed: 1}},
	}
	if err := CommitSync(context.Background(), db, n, now, snap, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	f, _ := Failures(context.Background(), db, []NAS{n}, now.Add(-day), now, 26*time.Hour)
	if len(f) != 1 || f[0].Kind != "running_too_long" {
		t.Fatalf("hung job not reported: %+v", f)
	}
	s, _ := Stale(context.Background(), db, []NAS{n}, 36*time.Hour, now, 26*time.Hour)
	if len(s) != 1 || s[0].HostName != "PC-7" || !s[0].Running {
		t.Fatalf("running task hid a 30-day-stale device: %+v", s)
	}
	st, _ := Status(context.Background(), db, []NAS{n}, now.Add(-day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	if st[0].State == SevOK || st[0].TasksStuck != 1 {
		t.Fatalf("status with a hung job = %+v", st[0])
	}
}

func TestStaleFallbackOnlyCreditsFullTaskSuccess(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	snap := &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day), Devices: []Device{
			{DeviceID: 1, HostName: "BAD-PC"}, {DeviceID: 2, HostName: "NO-RUNS"}, {DeviceID: 3, HostName: ""}, {DeviceID: 4, HostName: "A_B"}}}},
		// BAD-PC has per-device history: an old success and a recent failure.
		DeviceRuns: []DeviceRun{
			{DeviceResultID: 1, DeviceID: 1, Status: StatusSuccess, Start: ts(10 * day), End: ts(10*day - time.Hour)},
			{DeviceResultID: 2, DeviceID: 1, Status: StatusFailed, Start: ts(day), End: ts(day - time.Hour)}},
		Results: []Result{
			// A task-level partial yesterday naming BAD-PC and NO-RUNS, and one
			// with an empty host list; neither may credit a device.
			{ResultID: 10, TaskID: 1, Status: StatusPartial, JobAction: JobBackup, Start: ts(day), End: ts(day - time.Hour), Hosts: "BAD-PC,NO-RUNS", DeviceIDs: ",1,2,"},
			// A full success that names A_B by host only (no device id): no credit.
			{ResultID: 11, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * day), End: ts(2*day - time.Hour), Hosts: "A_B,AXB"}},
		Storage: []StorageItem{{Kind: "volume", ID: "v1", Status: "normal", SizeTotal: 10, SizeUsed: 1}},
	}
	if err := CommitSync(context.Background(), db, n, now, snap, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := Stale(context.Background(), db, []NAS{n}, 36*time.Hour, now, 26*time.Hour)
	hosts := map[string]string{}
	for _, d := range got {
		hosts[d.HostName] = d.Kind
	}
	if hosts["BAD-PC"] != "stale" || hosts["NO-RUNS"] != "never_succeeded" || hosts[""] != "never_succeeded" || hosts["A_B"] != "never_succeeded" {
		t.Fatalf("stale fallback credited the wrong devices: %+v", got)
	}
}

func TestResultCursorRefetchesRunningJobs(t *testing.T) {
	db := openDB(t)
	snap := &Snapshot{TakenAt: now, ABBInstalled: true, Results: []Result{
		{ResultID: 1, TaskID: 1, Status: StatusRunning, JobAction: JobBackup, Start: ts(3 * day)},
		{ResultID: 2, TaskID: 2, Status: StatusSuccess, JobAction: JobBackup, Start: ts(time.Hour), End: ts(30 * time.Minute)}}}
	n := NAS{Name: "acme"}
	if err := CommitSync(context.Background(), db, n, now, snap, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	c, err := ResultCursor(context.Background(), db, n)
	if err != nil || c != ts(3*day) {
		t.Fatalf("cursor = %d (%v), want the running job's start %d", c, err, ts(3*day))
	}
}

func TestFetchRejectsNonEnvelopeAndShapeDrift(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		switch r.URL.Query().Get("api") {
		case "SYNO.ActiveBackup.Task":
			_, _ = w.Write([]byte(`<html>maintenance</html>`))
		case "SYNO.Storage.CGI.Storage":
			_, _ = w.Write([]byte(`{"success":true,"data":{"something_else":[]}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":{"model":"DS"}}`))
		}
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	snap, _, err := Fetch(context.Background(), c, 0, now, DefaultOptions)
	if err == nil || !strings.Contains(err.Error(), "ABB tasks") || !strings.Contains(err.Error(), "storage") {
		t.Fatalf("err = %v, want ABB tasks and storage errors", err)
	}
	if snap.ABBInstalled || len(snap.Tasks) != 0 || len(snap.Storage) != 0 {
		t.Fatalf("snapshot from bad payloads = %+v", snap)
	}
	// CommitSync must not wipe the previous task rows for a failed parse.
	db := openDB(t)
	prev := &Snapshot{TakenAt: now, ABBInstalled: true, Tasks: []Task{{TaskID: 9, Name: "Keep"}}}
	acme := NAS{Name: "acme"}
	_ = CommitSync(context.Background(), db, acme, now, prev, SystemInfo{}, nil)
	_ = CommitSync(context.Background(), db, acme, now, snap, SystemInfo{}, err)
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM abb_tasks WHERE nas = 'acme'`).Scan(&n)
	if n != 1 {
		t.Fatalf("task rows after a failed fetch = %d, want the previous 1", n)
	}
}

// --- round-2 review regressions ---

func commitTasks(t *testing.T, db *sql.DB, n NAS, snap *Snapshot) {
	t.Helper()
	if snap.Storage == nil {
		snap.Storage = []StorageItem{{Kind: "volume", ID: "v1", Status: "normal", SizeTotal: 10, SizeUsed: 1}}
	}
	if err := CommitSync(context.Background(), db, n, now, snap, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunningTaskDoesNotHidePriorFailure(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks:   []Task{{TaskID: 1, Name: "VMs", Running: true, Scheduled: true, NextTrigger: ts(-day), LastStart: ts(time.Hour), LastJobAction: JobBackup}},
		Results: []Result{{ResultID: 1, TaskID: 1, Status: StatusFailed, JobAction: JobBackup, Start: ts(10 * time.Hour), End: ts(9 * time.Hour)}}})
	f, _ := Failures(context.Background(), db, []NAS{n}, now.Add(-day), now, 26*time.Hour)
	if len(f) != 1 || f[0].Kind != "run_failed" || !f[0].Running {
		t.Fatalf("failed run hidden by the run in progress: %+v", f)
	}
	st, _ := Status(context.Background(), db, []NAS{n}, now.Add(-day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	if st[0].State != SevCritical || st[0].TasksFailed != 1 || st[0].TasksRunning != 0 {
		t.Fatalf("status = %+v", st[0])
	}
}

func TestTerminalJobWithoutEndIsNotRunning(t *testing.T) {
	raw := json.RawMessage(`{"tasks":[{"task_id":1,"task_name":"T","next_trigger_time":9999999999,"devices":[],
	  "last_result":{"result_id":5,"status":4,"job_action":1,"time_start":100,"time_end":0}},
	  {"task_id":2,"task_name":"D","next_trigger_time":9999999999,"devices":[],
	  "last_result":{"result_id":6,"status":1,"job_action":131072,"time_start":100,"time_end":0}}]}`)
	tasks, err := parseTasks(raw, now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if tasks[0].Running || tasks[1].Running {
		t.Fatalf("running = %v/%v, want false (failed job; unfinished version deletion)", tasks[0].Running, tasks[1].Running)
	}
}

func TestParsersRejectShapeDrift(t *testing.T) {
	for name, raw := range map[string]string{
		"task without devices":  `{"tasks":[{"task_id":1}]}`,
		"task without id":       `{"tasks":[{"devices":[]}]}`,
		"device run without id": `{"device_list":[{"device":{"device_id":1},"transfer_list":[{"status":2}]}]}`,
		"device without list":   `{"device_list":[{"device":{"device_id":1}}]}`,
		"storage without pools": `{"volumes":[],"disks":[]}`,
	} {
		var err error
		switch {
		case strings.HasPrefix(name, "task"):
			_, err = parseTasks(json.RawMessage(raw), now.Unix())
		case strings.HasPrefix(name, "device"):
			_, err = parseDeviceRuns(json.RawMessage(raw), now.Unix())
		default:
			_, err = parseStorage(json.RawMessage(raw))
		}
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestABBRemovedIsReportedAndRetired(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	installed, no := true, false
	if err := CommitSync(context.Background(), db, n, now.Add(-time.Hour), &Snapshot{TakenAt: now.Add(-time.Hour), ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "Old", Scheduled: true, NextTrigger: ts(-day)}}}, SystemInfo{ABBInstalled: &installed}, nil); err != nil {
		t.Fatal(err)
	}
	if err := CommitSync(context.Background(), db, n, now, &Snapshot{TakenAt: now, ABBAbsent: true,
		Storage: []StorageItem{{Kind: "volume", ID: "v1", Status: "normal", SizeTotal: 10, SizeUsed: 1}}}, SystemInfo{ABBInstalled: &no}, nil); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = db.QueryRow(`SELECT COUNT(*) FROM abb_tasks WHERE nas = 'acme'`).Scan(&left)
	f, _ := Failures(context.Background(), db, []NAS{n}, now.Add(-day), now, 26*time.Hour)
	if left != 0 || len(f) != 1 || f[0].Kind != "abb_missing" {
		t.Fatalf("tasks left=%d failures=%+v", left, f)
	}
}

func TestStatusBucketsAddUp(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "OK", Scheduled: true, NextTrigger: ts(-day)}, {TaskID: 2, Name: "Off failed", Scheduled: false}},
		Results: []Result{
			{ResultID: 1, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour)},
			{ResultID: 2, TaskID: 2, Status: StatusFailed, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour)}}})
	st, _ := Status(context.Background(), db, []NAS{n}, now.Add(-day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	r := st[0]
	if r.TasksOK != 1 || r.TasksFailed != 1 || r.TasksOff != 0 ||
		r.TasksOK+r.TasksFailed+r.TasksPartial+r.TasksNoRun+r.TasksRunning+r.TasksStuck+r.TasksOff != r.Tasks {
		t.Fatalf("buckets = %+v", r)
	}
}

func TestFindDeviceLiteralAndSharedLastSuccess(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks:   []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day), Devices: []Device{{DeviceID: 1, HostName: "A_B"}, {DeviceID: 2, HostName: "AXB"}}}},
		Results: []Result{{ResultID: 1, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour), Hosts: "A_B", DeviceIDs: ",1,"}}})
	got, _ := FindDevice(context.Background(), db, []NAS{n}, "a_b", 2, now, 26*time.Hour)
	if len(got) != 1 || got[0].HostName != "A_B" || got[0].LastSuccess != ts(time.Hour) {
		t.Fatalf("FindDevice = %+v", got)
	}
	if all, _ := FindDevice(context.Background(), db, []NAS{n}, "%", 2, now, 26*time.Hour); len(all) != 0 {
		t.Fatalf("%% matched as a wildcard: %+v", all)
	}
}

func TestTrendEvenMedian(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	var results []Result
	for i, d := range []int64{60, 60, 600, 700} {
		start := ts(time.Duration(10-i) * day)
		results = append(results, Result{ResultID: int64(i + 1), TaskID: 1, TaskName: "T", Status: StatusSuccess, JobAction: JobBackup, Start: start, End: start + d})
	}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, Results: results})
	rows, _ := Trend(context.Background(), db, []NAS{n}, now.Add(-30*day), false, 2, now, 26*time.Hour)
	if rows[0].MedianSec != 330 || !rows[0].Overrun {
		t.Fatalf("trend = %+v, want median 330 and overrun (700 > 2x330)", rows[0])
	}
}

func TestVolumeWithoutCapacityIsFlagged(t *testing.T) {
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, Storage: []StorageItem{{Kind: "volume", ID: "v1", Status: "normal"}}})
	got, _ := Health(context.Background(), db, []NAS{n}, 85, 55, now, 26*time.Hour)
	if len(got) != 1 || got[0].Problem != "capacity not reported" {
		t.Fatalf("health = %+v", got)
	}
}

func TestURLChangeIsNotFresh(t *testing.T) {
	db := openDB(t)
	old := NAS{Name: "acme", URL: "https://old:5001"}
	commitTasks(t, db, old, &Snapshot{TakenAt: now})
	moved := NAS{Name: "acme", URL: "https://new:5001"}
	st, _ := Status(context.Background(), db, []NAS{moved}, now.Add(-day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	if st[0].State != SevUnknown {
		t.Fatalf("a NAS re-pointed at another URL read as %v", st[0].State)
	}
}

func TestFetchTruncationAndTransferFailureCommitNoHistory(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		q := r.URL.Query()
		switch q.Get("api") + "/" + q.Get("method") {
		case "SYNO.ActiveBackup.Task/list":
			_, _ = w.Write([]byte(`{"success":true,"data":{"tasks":[]}}`))
		case "SYNO.ActiveBackup.Log/list_result":
			page++
			var rows []string
			for i := 0; i < 200; i++ {
				rows = append(rows, fmt.Sprintf(`{"result_id":%d,"task_id":1,"status":2,"job_action":1,"time_start":%d}`, page*1000+i, ts(time.Hour)))
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"results":[` + strings.Join(rows, ",") + `]}}`))
		case "SYNO.ActiveBackup.Overview/list_device_transfer_size":
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":101}}`))
		case "SYNO.Storage.CGI.Storage/load_info":
			_, _ = w.Write([]byte(`{"success":true,"data":{"volumes":[],"disks":[],"storagePools":[]}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":{"model":"DS"}}`))
		}
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	opts := DefaultOptions
	opts.MaxResultPages = 2
	snap, _, err := Fetch(context.Background(), c, 0, now, opts)
	if err == nil || !strings.Contains(err.Error(), "more than 400 results") || len(snap.Results) != 0 || len(snap.DeviceRuns) != 0 {
		t.Fatalf("truncation: err=%v results=%d", err, len(snap.Results))
	}
	opts.MaxResultPages = 50
	page = 1000 // fits on later pages; transfers still fail
	snap, _, err = Fetch(context.Background(), c, 0, now, Options{InitialWindow: day, MaxResultPages: 1, MaxDetails: 0})
	if err == nil || len(snap.Results) != 0 {
		t.Fatalf("results committed without their device runs: err=%v results=%d", err, len(snap.Results))
	}
}
