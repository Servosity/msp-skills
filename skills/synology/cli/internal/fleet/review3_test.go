// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Regressions for the round-3 review findings.

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"synology-pp-cli/internal/dsm"
)

func vol() []StorageItem {
	return []StorageItem{{Kind: "volume", ID: "v1", Status: "normal", SizeTotal: 10, SizeUsed: 1}}
}

func TestNameRepointedAtAnotherNASDropsOldHistory(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	a := NAS{Name: "default", URL: "https://a.example:5001"}
	// Box A: device 7 backed up an hour ago.
	if err := CommitSync(ctx, db, a, now, &Snapshot{TakenAt: now, ABBInstalled: true, StorageOK: true, Storage: vol(),
		Tasks:      []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day), Devices: []Device{{DeviceID: 7, HostName: "A-PC"}}}},
		Results:    []Result{{ResultID: 1, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour)}},
		DeviceRuns: []DeviceRun{{DeviceResultID: 1, ResultID: 1, DeviceID: 7, Status: StatusSuccess, Start: ts(2 * time.Hour), End: ts(time.Hour)}},
	}, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	b := NAS{Name: "default", URL: "https://b.example:5001"}
	if c, err := ResultCursor(ctx, db, b); err != nil || c != 0 {
		t.Fatalf("cursor for a repointed name = %d (%v), want 0", c, err)
	}
	// Box B reuses task 1 / device 7 with no runs at all.
	later := now.Add(time.Minute)
	if err := CommitSync(ctx, db, b, later, &Snapshot{TakenAt: later, ABBInstalled: true, StorageOK: true, Storage: vol(),
		Tasks: []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day), Devices: []Device{{DeviceID: 7, HostName: "B-PC"}}}},
	}, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	s, _ := Stale(ctx, db, []NAS{b}, 36*time.Hour, later, 26*time.Hour)
	if len(s) != 1 || s[0].Kind != "never_succeeded" {
		t.Fatalf("box B inherited box A's success: %+v", s)
	}
	f, _ := Failures(ctx, db, []NAS{b}, later.Add(-day), later, 26*time.Hour)
	if len(f) != 1 || f[0].Kind != "no_run_in_window" {
		t.Fatalf("box B failures = %+v", f)
	}
}

func TestOlderSyncCannotOverwriteNewer(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	if err := CommitSync(ctx, db, n, now, nil, SystemInfo{}, errors.New("DSM login: HTTP 502")); err != nil {
		t.Fatal(err)
	}
	err := CommitSync(ctx, db, n, now.Add(-time.Minute), &Snapshot{TakenAt: now.Add(-time.Minute), Storage: vol(), StorageOK: true}, SystemInfo{}, nil)
	if !errors.Is(err, ErrSuperseded) {
		t.Fatalf("older commit err = %v, want ErrSuperseded", err)
	}
	st, _ := LoadSyncStates(ctx, db)
	if st["acme"].Error == "" || st["acme"].FreshFor(n, now, 26*time.Hour) {
		t.Fatalf("older healthy fetch replaced the newer failure: %+v", st["acme"])
	}
}

func TestABBNeverInstalledIsNotAFinding(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "files"}
	off := false
	if err := CommitSync(ctx, db, n, now, &Snapshot{TakenAt: now, ABBAbsent: true, Storage: vol(), StorageOK: true}, SystemInfo{ABBInstalled: &off}, nil); err != nil {
		t.Fatal(err)
	}
	f, _ := Failures(ctx, db, []NAS{n}, now.Add(-day), now, 26*time.Hour)
	st, _ := Status(ctx, db, []NAS{n}, now.Add(-day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	if len(f) != 1 || f[0].Kind != "abb_not_installed" || st[0].State != SevOK || st[0].StaleDevices != 0 {
		t.Fatalf("never-ABB NAS: failures=%+v status=%+v", f, st[0])
	}
	m, _ := FindDevice(ctx, db, []NAS{n}, "PC", 5, now, 26*time.Hour)
	tr, _ := Trend(ctx, db, []NAS{n}, now.Add(-30*24*time.Hour), false, 2, now, 26*time.Hour)
	if len(m) != 1 || m[0].Kind != "abb_not_installed" || len(tr) != 1 || tr[0].Kind != "abb_not_installed" {
		t.Fatalf("device=%+v trend=%+v", m, tr)
	}
}

func TestABBMissingIsCriticalEverywhere(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	on, off := true, false
	_ = CommitSync(ctx, db, n, now.Add(-time.Hour), &Snapshot{TakenAt: now.Add(-time.Hour), ABBInstalled: true, Storage: vol(), StorageOK: true}, SystemInfo{ABBInstalled: &on}, nil)
	_ = CommitSync(ctx, db, n, now, &Snapshot{TakenAt: now, ABBAbsent: true, Storage: vol(), StorageOK: true}, SystemInfo{ABBInstalled: &off}, nil)
	st, _ := Status(ctx, db, []NAS{n}, now.Add(-day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	s, _ := Stale(ctx, db, []NAS{n}, 36*time.Hour, now, 26*time.Hour)
	tr, _ := Trend(ctx, db, []NAS{n}, now.Add(-day), false, 2, now, 26*time.Hour)
	m, _ := FindDevice(ctx, db, []NAS{n}, "PC", 5, now, 26*time.Hour)
	if st[0].State != SevCritical || st[0].StaleDevices != 0 || st[0].Note == "" {
		t.Fatalf("status = %+v", st[0])
	}
	if len(s) != 1 || s[0].Kind != "abb_missing" || len(tr) != 1 || tr[0].Kind != "abb_missing" || len(m) != 1 || m[0].Kind != "abb_missing" {
		t.Fatalf("stale=%+v trend=%+v device=%+v", s, tr, m)
	}
}

func TestSameSecondEmptyStorageReplacesSnapshot(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	_ = CommitSync(ctx, db, n, now, &Snapshot{TakenAt: now, Storage: vol(), StorageOK: true}, SystemInfo{}, nil)
	_ = CommitSync(ctx, db, n, now, &Snapshot{TakenAt: now, StorageOK: true}, SystemInfo{}, nil)
	h, _ := Health(ctx, db, []NAS{n}, 85, 55, now, 26*time.Hour)
	if len(h) != 1 || h[0].Severity != SevUnknown {
		t.Fatalf("empty second snapshot kept the old one: %+v", h)
	}
}

func TestRunningStart(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{
			{TaskID: 1, Name: "Unknown", Running: true, Scheduled: true, NextTrigger: ts(-day), LastStart: ts(3 * day), LastEnd: ts(3*day - time.Hour)},
			{TaskID: 2, Name: "FromResults", Running: true, Scheduled: true, NextTrigger: ts(-day)},
		},
		Results: []Result{{ResultID: 9, TaskID: 2, Status: StatusRunning, JobAction: JobBackup, Start: ts(4 * day)}},
	})
	f, _ := Failures(ctx, db, []NAS{n}, now.Add(-day), now, 26*time.Hour)
	kinds := map[string]string{}
	for _, x := range f {
		kinds[x.TaskName] = x.Kind
	}
	if kinds["Unknown"] != "running_start_unknown" || kinds["FromResults"] != "running_too_long" {
		t.Fatalf("running kinds = %v", kinds)
	}
}

func TestParserValidity(t *testing.T) {
	tasks := func(body string) ([]Task, error) {
		return parseTasks(json.RawMessage(`{"tasks":[`+body+`]}`), now.Unix())
	}
	if _, err := tasks(`{"task_id":1,"devices":[{"device_id":3},{"device_id":3}]}`); err == nil {
		t.Fatal("duplicate device ids accepted")
	}
	if _, err := tasks(`{"task_id":1,"devices":[{"host_name":"x"}]}`); err == nil {
		t.Fatal("device without device_id accepted")
	}
	got, err := tasks(`{"task_id":1,"next_trigger_time":"garbage","devices":[],
	  "progress":{"running_task_status":1},"last_result":{"result_id":7,"job_action":1,"status":2,"time_start":100,"time_end":200}}`)
	if err != nil || !got[0].Scheduled || !got[0].Running || got[0].RunStart != 0 {
		t.Fatalf("task = %+v (%v): a malformed trigger stays scheduled; a finished last_result is not the run's start", got, err)
	}
	storage := func(body string) ([]StorageItem, error) { return parseStorage(json.RawMessage(body)) }
	if _, err := storage(`{"volumes":[{"id":"v"},{"id":"v"}],"disks":[],"storagePools":[]}`); err == nil {
		t.Fatal("duplicate volume ids accepted")
	}
	if _, err := storage(`{"volumes":[{"status":"degraded"}],"disks":[],"storagePools":[]}`); err == nil {
		t.Fatal("volume without id accepted")
	}
	items, err := storage(`{"volumes":[{"id":"v","status":"normal","size":{"total":"100"}}],"disks":[],"storagePools":[]}`)
	if err != nil || items[0].SizeTotal != -1 {
		t.Fatalf("missing size.used read as known capacity: %+v (%v)", items, err)
	}
}

type constCaller string

func (c constCaller) Get(context.Context, string, url.Values) (json.RawMessage, error) {
	return json.RawMessage(c), nil
}

func TestFetchResultsValidity(t *testing.T) {
	cases := map[string]string{
		"no job_action":  `{"results":[{"result_id":1,"status":2,"time_start":5}]}`,
		"short of total": `{"results":[{"result_id":1,"status":2,"job_action":1,"time_start":5}],"total":3}`,
	}
	for name, body := range cases {
		if _, _, err := fetchResults(context.Background(), constCaller(body), 0, now.Unix(), 2); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestHistoryCoverage(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commit := func(at time.Time, from int64) {
		t.Helper()
		if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, HistoryFrom: from, Storage: vol(), StorageOK: true,
			Results: []Result{{ResultID: at.Unix(), TaskID: 1, TaskName: "T", Status: StatusSuccess, JobAction: JobBackup,
				Start: at.Add(-time.Hour).Unix(), End: at.Add(-30 * time.Minute).Unix()}},
		}, SystemInfo{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	commit(now, ts(day))
	tr, _ := Trend(ctx, db, []NAS{n}, now.Add(-30*24*time.Hour), false, 2, now, 26*time.Hour)
	if len(tr) != 1 || tr[0].HistoryFrom != ts(day) || !strings.Contains(tr[0].Note, "--full") {
		t.Fatalf("30-day trend over 1 day of history not marked: %+v", tr)
	}
	// A later sync that overlaps keeps the start; one after a gap restarts it.
	commit(now.Add(time.Hour), ts(2*time.Hour))
	if st, _ := LoadSyncStates(ctx, db); st["acme"].HistoryFrom != ts(day) {
		t.Fatalf("overlapping sync moved history_from to %d", st["acme"].HistoryFrom)
	}
	gapStart := now.Add(5 * 24 * time.Hour)
	commit(gapStart.Add(time.Hour), gapStart.Unix())
	if st, _ := LoadSyncStates(ctx, db); st["acme"].HistoryFrom != gapStart.Unix() {
		t.Fatalf("sync after a gap kept history_from %d", st["acme"].HistoryFrom)
	}
}

// --- Codex round-3 regressions ---

func TestCodexRound3ParserChecks(t *testing.T) {
	bad := map[string]string{
		"result without task_id": `{"results":[{"result_id":1,"status":4,"job_action":1,"time_start":5}]}`,
	}
	for name, body := range bad {
		if _, _, err := fetchResults(context.Background(), constCaller(body), 0, now.Unix(), 2); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	runs := func(body string) error {
		_, err := parseDeviceRuns(json.RawMessage(`{"device_list":[`+body+`]}`), now.Unix())
		return err
	}
	if runs(`{"device":{},"transfer_list":[{"device_result_id":1,"status":2,"time_start":5}]}`) == nil {
		t.Error("transfer without a device id accepted")
	}
	if runs(`{"device":{"device_id":1},"transfer_list":[{"device_result_id":1,"status":2,"time_start":5},{"device_result_id":1,"status":4,"time_start":6}]}`) == nil {
		t.Error("duplicate device_result_id accepted")
	}
	if runs(`{"device":{"device_id":1},"transfer_list":[{"device_result_id":1,"time_start":5}]}`) == nil {
		t.Error("transfer without status accepted")
	}
	got, err := parseTasks(json.RawMessage(`{"tasks":[{"task_id":1,"next_trigger_time":0.5,"devices":[]}]}`), now.Unix())
	if err != nil || !got[0].Scheduled {
		t.Fatalf("fractional next_trigger_time made the task unscheduled: %+v (%v)", got, err)
	}
	items, err := parseStorage(json.RawMessage(`{"volumes":[{"id":"v","status":"normal","size":{"total":"10","used":"1"}}],"storagePools":[],
	  "disks":[{"id":"d","status":"normal","smart_status":"normal"}]}`))
	if err != nil || items[1].Temp != -1 {
		t.Fatalf("missing disk temperature read as a value: %+v (%v)", items, err)
	}
}

func TestHealthFlagsMissingTemperatureAndSkipsInvalidCapacitySamples(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	yesterday := now.Add(-2 * day)
	_ = CommitSync(ctx, db, n, yesterday, &Snapshot{TakenAt: yesterday, StorageOK: true,
		Storage: []StorageItem{{Kind: "volume", ID: "v1", Status: "normal", SizeTotal: -1, SizeUsed: -1}}}, SystemInfo{}, nil)
	_ = CommitSync(ctx, db, n, now, &Snapshot{TakenAt: now, StorageOK: true, Storage: []StorageItem{
		{Kind: "volume", ID: "v1", Status: "normal", SizeTotal: 100, SizeUsed: 90},
		{Kind: "disk", ID: "d1", Status: "normal", SmartStatus: "normal", Temp: -1}}}, SystemInfo{}, nil)
	h, _ := Health(ctx, db, []NAS{n}, 85, 55, now, 26*time.Hour)
	var temp, full bool
	for _, i := range h {
		temp = temp || strings.HasPrefix(i.Problem, "temperature not reported")
		if i.Kind == "volume" && i.UsedPct > 0 {
			full = true
			if i.DaysToFull != nil {
				t.Fatalf("forecast from an unknown-capacity sample: %v days", *i.DaysToFull)
			}
		}
	}
	if !temp || !full {
		t.Fatalf("health = %+v", h)
	}
	if h2, _ := Health(ctx, db, []NAS{n}, 85, 0, now, 26*time.Hour); strings.Contains(fmt.Sprint(h2), "temperature") {
		t.Fatalf("--disk-temp 0 still checks temperature: %+v", h2)
	}
}

func TestOldSyncOfReplacedBoxIsSuperseded(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	b := NAS{Name: "acme", URL: "https://b.example:5001"}
	if err := CommitSync(ctx, db, b, now, &Snapshot{TakenAt: now, Storage: vol(), StorageOK: true}, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	a := NAS{Name: "acme", URL: "https://a.example:5001"}
	if err := CommitSync(ctx, db, a, now.Add(-time.Minute), &Snapshot{TakenAt: now.Add(-time.Minute), Storage: vol(), StorageOK: true}, SystemInfo{}, nil); !errors.Is(err, ErrSuperseded) {
		t.Fatalf("old sync of the replaced box: err = %v, want ErrSuperseded", err)
	}
	if st, _ := LoadSyncStates(ctx, db); st["acme"].URL != b.URL {
		t.Fatalf("replacement's record overwritten: %+v", st["acme"])
	}
}

func TestEnsureSchemaMigratesOldMirror(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	for _, q := range []string{`DROP TABLE fleet_sync`, `DROP TABLE abb_tasks`,
		`CREATE TABLE fleet_sync (nas TEXT PRIMARY KEY, url TEXT, client TEXT, attempted_at INTEGER, succeeded_at INTEGER, error TEXT, model TEXT, dsm_version TEXT, abb_installed INTEGER)`,
		`CREATE TABLE abb_tasks (nas TEXT, task_id INTEGER, task_name TEXT, backup_type INTEGER, running INTEGER, scheduled INTEGER, next_trigger_time INTEGER,
		  last_result_id INTEGER, last_status INTEGER, last_job_action INTEGER, last_time_start INTEGER, last_time_end INTEGER, synced_at INTEGER, PRIMARY KEY (nas, task_id))`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if stale, err := NeedsMigration(ctx, db); err != nil || !stale {
		t.Fatalf("old mirror not detected: %v %v", stale, err)
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if stale, _ := NeedsMigration(ctx, db); stale {
		t.Fatal("still stale after EnsureSchema")
	}
	commitTasks(t, db, NAS{Name: "acme"}, &Snapshot{TakenAt: now, ABBInstalled: true, Tasks: []Task{{TaskID: 1, Name: "T", RunStart: 5}}})
	if _, err := LoadSyncStates(ctx, db); err != nil {
		t.Fatal(err)
	}
}

// errCaller answers every call with a transport-style (non-DSM) error.
type errCaller struct{}

func (errCaller) Get(context.Context, string, url.Values) (json.RawMessage, error) {
	return nil, errors.New("context deadline exceeded")
}

func TestTransientTaskErrorSaysNothingAboutABB(t *testing.T) {
	_, info, err := Fetch(context.Background(), partialCaller{}, 0, now, DefaultOptions)
	if err == nil {
		t.Fatal("want an error")
	}
	if info.ABBInstalled != nil {
		t.Fatalf("a timeout recorded ABB installed=%v", *info.ABBInstalled)
	}
}

// partialCaller answers system info and fails everything else with a
// transport-style error, as a NAS that times out mid-sync would.
type partialCaller struct{}

func (partialCaller) Get(_ context.Context, path string, _ url.Values) (json.RawMessage, error) {
	if path == PathSystemInfo {
		return json.RawMessage(`{"model":"DS920+"}`), nil
	}
	return errCaller{}.Get(context.Background(), path, nil)
}

// --- Codex round-4 regressions ---

func TestCodexRound4RunValidation(t *testing.T) {
	n := now.Unix()
	row := func(fields string) string { return `{"results":[` + fields + `]}` }
	bad := map[string]string{
		"time_start 0":        row(`{"result_id":1,"task_id":1,"status":4,"job_action":1,"time_start":0,"time_end":0}`),
		"future time_start":   row(fmt.Sprintf(`{"result_id":1,"task_id":1,"status":2,"job_action":1,"time_start":%d}`, n+3*86400)),
		"end before start":    row(`{"result_id":1,"task_id":1,"status":2,"job_action":1,"time_start":500,"time_end":100}`),
		"fractional id":       row(`{"result_id":1.5,"task_id":1,"status":2,"job_action":1,"time_start":5}`),
		"conflicting dup ids": row(`{"result_id":1,"task_id":1,"status":4,"job_action":1,"time_start":5},{"result_id":1,"task_id":1,"status":2,"job_action":1,"time_start":5}`),
	}
	for name, body := range bad {
		if _, _, err := fetchResults(context.Background(), constCaller(body), 0, n, 2); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	same := `{"result_id":1,"task_id":1,"status":2,"job_action":1,"time_start":5}`
	if out, _, err := fetchResults(context.Background(), constCaller(row(same+","+same)), 0, n, 2); err != nil || len(out) != 1 {
		t.Errorf("identical duplicate: out=%d err=%v", len(out), err)
	}
	// A full last page that reaches the reported total is complete, not truncated.
	var rows []string
	for i := 1; i <= 200; i++ {
		rows = append(rows, fmt.Sprintf(`{"result_id":%d,"task_id":1,"status":2,"job_action":1,"time_start":5}`, i))
	}
	full := `{"total":200,"results":[` + strings.Join(rows, ",") + `]}`
	if out, truncated, err := fetchResults(context.Background(), constCaller(full), 0, n, 1); err != nil || truncated || len(out) != 200 {
		t.Errorf("full page at total: out=%d truncated=%v err=%v", len(out), truncated, err)
	}
	runs := func(body string) error {
		_, err := parseDeviceRuns(json.RawMessage(`{"device_list":[`+body+`]}`), n)
		return err
	}
	if runs(`{"device":{"device_id":7.5},"transfer_list":[{"device_result_id":1,"status":2,"time_start":5}]}`) == nil {
		t.Error("fractional device id accepted")
	}
	if runs(`{"device":{"device_id":7},"transfer_list":[{"device_result_id":1,"config_device_id":8,"status":2,"time_start":5}]}`) == nil {
		t.Error("conflicting device ids accepted")
	}
	if runs(fmt.Sprintf(`{"device":{"device_id":7},"transfer_list":[{"device_result_id":1,"status":2,"time_start":5,"time_end":%d}]}`, n+5*86400)) == nil {
		t.Error("future time_end accepted")
	}
}

func TestMigrationBackfillsABBSeenAndZeroTempIsUnknown(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	for _, q := range []string{`DROP TABLE fleet_sync`,
		`CREATE TABLE fleet_sync (nas TEXT PRIMARY KEY, url TEXT, client TEXT, attempted_at INTEGER, succeeded_at INTEGER, error TEXT, model TEXT, dsm_version TEXT, abb_installed INTEGER)`,
		`INSERT INTO fleet_sync (nas, url, abb_installed) VALUES ('acme', '', 1)`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if st, _ := LoadSyncStates(ctx, db); !st["acme"].ABBSeen {
		t.Fatal("migration dropped the evidence that ABB answered")
	}
	n := NAS{Name: "acme"}
	_ = CommitSync(ctx, db, n, now, &Snapshot{TakenAt: now, StorageOK: true, Storage: []StorageItem{
		{Kind: "volume", ID: "v1", Status: "normal", SizeTotal: 10, SizeUsed: 1},
		{Kind: "disk", ID: "d1", Status: "normal", SmartStatus: "normal", Temp: 0}}}, SystemInfo{}, nil)
	h, _ := Health(ctx, db, []NAS{n}, 85, 55, now, 26*time.Hour)
	if !strings.Contains(fmt.Sprint(h), "temperature not reported") {
		t.Fatalf("a stored 0 temperature read as cool: %+v", h)
	}
}

// --- Codex round-5 regressions ---

// pagedCaller returns the same page for every offset.
type pagedCaller string

func (c pagedCaller) Get(context.Context, string, url.Values) (json.RawMessage, error) {
	return json.RawMessage(c), nil
}

func TestCodexRound5(t *testing.T) {
	ctx := context.Background()
	n := now.Unix()
	// A repeated full page must not count toward the total.
	var rows []string
	for i := 1; i <= 200; i++ {
		rows = append(rows, fmt.Sprintf(`{"result_id":%d,"task_id":1,"status":2,"job_action":1,"time_start":5}`, i))
	}
	page := `{"total":400,"results":[` + strings.Join(rows, ",") + `]}`
	if _, _, err := fetchResults(ctx, pagedCaller(page), 0, n, 5); err == nil {
		t.Error("a repeated page was counted toward the total")
	}
	// last_result times are bounded like any result.
	future := fmt.Sprintf(`{"tasks":[{"task_id":1,"devices":[],"last_result":{"status":1,"job_action":1,"time_start":%d,"time_end":0}}]}`, n+10*86400)
	if _, err := parseTasks(json.RawMessage(future), n); err == nil {
		t.Error("a last_result starting ten days ahead was accepted")
	}
	// abb_seen survives a mirror that recorded abb_installed=1 but no abb_seen.
	db := openDB(t)
	if _, err := db.Exec(`INSERT INTO fleet_sync (nas, url, abb_installed) VALUES ('acme', '', 1)`); err != nil {
		t.Fatal(err)
	}
	off := false
	_ = CommitSync(ctx, db, NAS{Name: "acme"}, now, &Snapshot{TakenAt: now, ABBAbsent: true, Storage: vol(), StorageOK: true}, SystemInfo{ABBInstalled: &off}, nil)
	if f, _ := Failures(ctx, db, []NAS{{Name: "acme"}}, now.Add(-day), now, 26*time.Hour); len(f) != 1 || f[0].Kind != "abb_missing" {
		t.Fatalf("installed-then-absent read as never installed: %+v", f)
	}
}

func TestConcurrentRegistryWritesKeepEveryNAS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fleet.json")
	done := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func(i int) {
			done <- UpdateRegistry(path, func(r *Registry) error {
				r.Put(NAS{Name: fmt.Sprintf("nas%02d", i), URL: "https://n:5001", EnvPrefix: fmt.Sprintf("P%02d", i)})
				return nil
			})
		}(i)
	}
	for i := 0; i < 20; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	r, err := LoadRegistry(path)
	if err != nil || len(r.NAS) != 20 {
		t.Fatalf("registry kept %d of 20 concurrent registrations (%v)", len(r.NAS), err)
	}
}

// --- Codex round-6 regressions ---

// offsetCaller serves page i (by offset/200) from pages; past the end it
// returns the last entry.
type offsetCaller []string

func (c offsetCaller) Get(_ context.Context, _ string, q url.Values) (json.RawMessage, error) {
	off, _ := strconv.Atoi(q.Get("offset"))
	i := off / 200
	if i >= len(c) {
		i = len(c) - 1
	}
	return json.RawMessage(c[i]), nil
}

func TestCodexRound6Pagination(t *testing.T) {
	n := now.Unix()
	var rows []string
	for i := 1; i <= 200; i++ {
		rows = append(rows, fmt.Sprintf(`{"result_id":%d,"task_id":1,"status":2,"job_action":1,"time_start":5}`, i))
	}
	first := `{"total":400,"results":[` + strings.Join(rows, ",") + `]}`
	if _, _, err := fetchResults(context.Background(), offsetCaller{first, `{"results":[]}`}, 0, n, 5); err == nil {
		t.Error("an earlier page's total was forgotten")
	}
	if _, _, err := fetchResults(context.Background(), constCaller(`{"total":-1,"results":[]}`), 0, n, 5); err == nil {
		t.Error("a negative total was accepted")
	}
	exact := `{"results":[{"result_id":1,"task_id":1,"status":2.0000000000000001,"job_action":1,"time_start":5}]}`
	if _, _, err := fetchResults(context.Background(), constCaller(exact), 0, n, 5); err == nil {
		t.Error("a non-integer status that rounds to 2 was accepted")
	}
}

// --- Codex round-7 regressions ---

func TestLastResultFailureBeatsOlderMirroredSuccess(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	tasks := []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day),
		LastResultID: 9, LastStatus: StatusFailed, LastJobAction: JobBackup, LastStart: ts(2 * time.Hour), LastEnd: ts(time.Hour)}}
	// The listing omitted result 9; only an older success was fetched. Fetch
	// merges the task's last_result into history before committing.
	listed := []Result{{ResultID: 8, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(10 * time.Hour), End: ts(9 * time.Hour)}}
	merged, err := mergeLastResults(listed, tasks)
	if err != nil {
		t.Fatal(err)
	}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, Tasks: tasks, Results: merged})
	f, _ := Failures(ctx, db, []NAS{n}, now.Add(-day), now, 26*time.Hour)
	if len(f) != 1 || f[0].Kind != "run_failed" || f[0].ResultID != 9 {
		t.Fatalf("last_result failure hidden by an older success: %+v", f)
	}
}

// --- Codex round-8 regressions ---

func TestLastResultMergedIntoHistory(t *testing.T) {
	tasks := []Task{{TaskID: 1, Name: "PCs", LastResultID: 9, LastStatus: StatusFailed, LastJobAction: JobBackup, LastStart: 500, LastEnd: 600}}
	// Omitted from the listing: added.
	got, err := mergeLastResults([]Result{{ResultID: 8, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: 100, End: 200}}, tasks)
	if err != nil || len(got) != 2 || got[1].ResultID != 9 || got[1].Status != StatusFailed || !got[1].FromTask {
		t.Fatalf("omitted last_result not merged: %+v (%v)", got, err)
	}
	// Listed as still running: the task's final status is taken.
	got, err = mergeLastResults([]Result{{ResultID: 9, TaskID: 1, Status: StatusRunning, JobAction: JobBackup, Start: 500}}, tasks)
	if err != nil || got[0].Status != StatusFailed {
		t.Fatalf("finished run kept as running: %+v (%v)", got, err)
	}
	// Any other disagreement about one run is inconsistent evidence.
	for name, listed := range map[string]Result{
		"other outcome": {ResultID: 9, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: 500, End: 600},
		"other action":  {ResultID: 9, TaskID: 1, Status: StatusFailed, JobAction: 131072, Start: 500, End: 600},
		"other start":   {ResultID: 9, TaskID: 1, Status: StatusFailed, JobAction: JobBackup, Start: 400, End: 600},
	} {
		if _, err := mergeLastResults([]Result{listed}, tasks); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A non-object last_result is malformed, not "never ran".
	if _, err := parseTasks(json.RawMessage(`{"tasks":[{"task_id":1,"devices":[],"last_result":[{"result_id":9,"status":4}]}]}`), now.Unix()); err == nil {
		t.Fatal("array last_result accepted")
	}
	// A populated last_result without time_start is malformed.
	if _, err := parseTasks(json.RawMessage(`{"tasks":[{"task_id":1,"devices":[],"last_result":{"result_id":9,"status":4,"job_action":1,"time_end":600}}]}`), now.Unix()); err == nil {
		t.Fatal("last_result without time_start accepted")
	}
	// An empty last_result (never ran) is fine.
	if _, err := parseTasks(json.RawMessage(`{"tasks":[{"task_id":1,"devices":[],"last_result":{}}]}`), now.Unix()); err != nil {
		t.Fatalf("empty last_result rejected: %v", err)
	}
}

func TestTrendNewestRunWithoutEndIsUnknown(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	var res []Result
	for i, d := range []int64{60, 60, 60} {
		start := ts(time.Duration(10-i) * time.Hour)
		res = append(res, Result{ResultID: int64(i + 1), TaskID: 1, TaskName: "T", Status: StatusSuccess, JobAction: JobBackup, Start: start, End: start + d})
	}
	res = append(res, Result{ResultID: 4, TaskID: 1, TaskName: "T", Status: StatusFailed, JobAction: JobBackup, Start: ts(time.Hour)})
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, HistoryFrom: ts(30 * day), Results: res})
	tr, _ := Trend(ctx, db, []NAS{n}, now.Add(-day), false, 2, now, 26*time.Hour)
	if len(tr) != 1 || tr[0].LastSec != nil || tr[0].Overrun != nil || !strings.Contains(tr[0].Note, "no end time") {
		t.Fatalf("newest run without an end borrowed an older duration: %+v", tr)
	}
}

// --- Codex round-9 regressions ---

func TestTaskResultKeepsMirroredMeasurementsAndOldMirrorsResync(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, Results: []Result{
		{ResultID: 9, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour), Hosts: "PC-1", Bytes: 42}}})
	later := now.Add(time.Minute)
	if err := CommitSync(ctx, db, n, later, &Snapshot{TakenAt: later, ABBInstalled: true, Storage: vol(), StorageOK: true, Results: []Result{
		{ResultID: 9, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour), FromTask: true}}}, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	var hosts string
	var bytes int64
	_ = db.QueryRow(`SELECT hosts, transfered_bytes FROM abb_results WHERE result_id = 9`).Scan(&hosts, &bytes)
	if hosts != "PC-1" || bytes != 42 {
		t.Fatalf("task-only row erased measurements: hosts=%q bytes=%d", hosts, bytes)
	}
	// The task now says the same run failed: inconsistent evidence fails the commit.
	again := later.Add(time.Minute)
	if err := CommitSync(ctx, db, n, again, &Snapshot{TakenAt: again, ABBInstalled: true, Results: []Result{
		{ResultID: 9, TaskID: 1, Status: StatusFailed, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour), FromTask: true}}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a task outcome contradicting the mirror was committed")
	}
	// A mirror written by an older version is not fresh.
	if _, err := db.Exec(`UPDATE fleet_sync SET mirror_format = 1`); err != nil {
		t.Fatal(err)
	}
	f, _ := Failures(ctx, db, []NAS{n}, now.Add(-day), later, 26*time.Hour)
	if len(f) != 1 || f[0].Kind != "nas_not_synced" || !strings.Contains(f[0].Note, "older version") {
		t.Fatalf("old-format mirror read as current: %+v", f)
	}
}

func TestTiedStartPrefersNonSuccess(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day)}},
		Results: []Result{
			{ResultID: 9, TaskID: 1, Status: StatusFailed, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour)},
			{ResultID: 8, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour)}}})
	f, _ := Failures(ctx, db, []NAS{n}, now.Add(-day), now, 26*time.Hour)
	if len(f) != 1 || f[0].Kind != "run_failed" {
		t.Fatalf("tie resolved to the success: %+v", f)
	}
}

// --- Codex round-10 regressions ---

func TestCodexRound10(t *testing.T) {
	// Nested values are run fields, not "never ran".
	if _, err := CheckLastResult(map[string]any{"result_id": []any{9}, "status": map[string]any{"code": 4}}, now.Unix()); err == nil {
		t.Error("nested last_result fields read as empty")
	}
	if lr, err := CheckLastResult(map[string]any{"result_id": json.Number("0"), "status": ""}, now.Unix()); err != nil || lr != nil {
		t.Errorf("zero/empty last_result = %v, %v", lr, err)
	}
	// Same outcome: the task's known end time is kept.
	tasks := []Task{{TaskID: 1, LastResultID: 9, LastStatus: StatusFailed, LastJobAction: JobBackup, LastStart: ts(48 * time.Hour), LastEnd: ts(time.Hour)}}
	got, err := mergeLastResults([]Result{{ResultID: 9, TaskID: 1, Status: StatusFailed, JobAction: JobBackup, Start: ts(48 * time.Hour)}}, tasks)
	if err != nil || got[0].End != ts(time.Hour) {
		t.Fatalf("end time dropped: %+v (%v)", got, err)
	}
	if _, err := mergeLastResults([]Result{{ResultID: 9, TaskID: 1, Status: StatusFailed, JobAction: JobBackup, Start: ts(48 * time.Hour), End: ts(2 * time.Hour)}}, tasks); err == nil {
		t.Error("conflicting end times accepted")
	}
	// A failed run that started 48h ago but ended an hour ago is in a 24h window.
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day)}}, Results: got})
	if f, _ := Failures(ctx, db, []NAS{n}, now.Add(-day), now, 26*time.Hour); len(f) != 1 || f[0].Kind != "run_failed" {
		t.Fatalf("failure that ended inside the window not reported: %+v", f)
	}
	// A run finishing in the second it started has a known duration of 0.
	db2 := openDB(t)
	commitTasks(t, db2, n, &Snapshot{TakenAt: now, ABBInstalled: true, HistoryFrom: ts(30 * day), Results: []Result{
		{ResultID: 1, TaskID: 1, TaskName: "T", Status: StatusSuccess, JobAction: JobBackup, Start: ts(time.Hour), End: ts(time.Hour)}}})
	tr, _ := Trend(ctx, db2, []NAS{n}, now.Add(-day), false, 2, now, 26*time.Hour)
	if len(tr) != 1 || tr[0].LastSec == nil || *tr[0].LastSec != 0 {
		t.Fatalf("zero-second run: %+v", tr)
	}
}

// --- Codex round-11 regressions ---

func TestListedRewritesCannotEraseOutcomesOrEnds(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	failed := Result{ResultID: 9, TaskID: 1, Status: StatusFailed, JobAction: JobBackup, Start: ts(48 * time.Hour), End: ts(time.Hour)}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, Results: []Result{failed}})
	at := now
	commit := func(r Result) error {
		at = at.Add(time.Minute)
		return CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, Results: []Result{r}}, SystemInfo{}, nil)
	}
	noEnd := failed
	noEnd.End = 0
	if err := commit(noEnd); err != nil {
		t.Fatal(err)
	}
	var end, status int64
	_ = db.QueryRow(`SELECT time_end, status FROM abb_results WHERE result_id = 9`).Scan(&end, &status)
	if end != failed.End || status != StatusFailed {
		t.Fatalf("listing without an end erased it: end=%d status=%d", end, status)
	}
	flipped := failed
	flipped.Status = StatusSuccess
	if err := commit(flipped); err == nil {
		t.Fatal("a mirrored failure was rewritten as a success")
	}
	running := failed
	running.Status, running.End = StatusRunning, 0
	if err := commit(running); err != nil {
		t.Fatalf("a stale running copy failed the commit: %v", err)
	}
	_ = db.QueryRow(`SELECT status FROM abb_results WHERE result_id = 9`).Scan(&status)
	if status != StatusFailed {
		t.Fatalf("final status regressed to %d", status)
	}
	// Device transfers follow the same rule.
	dr := DeviceRun{DeviceResultID: 1, ResultID: 9, DeviceID: 7, Status: StatusFailed, Start: ts(48 * time.Hour), End: ts(time.Hour)}
	at = at.Add(time.Minute)
	_ = CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, DeviceRuns: []DeviceRun{dr}}, SystemInfo{}, nil)
	dr.Status = StatusSuccess
	at = at.Add(time.Minute)
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, DeviceRuns: []DeviceRun{dr}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a failed device transfer was rewritten as a success")
	}
}

// --- Codex round-13 regressions ---

func TestUnfinishedStatusWithEndIsMalformed(t *testing.T) {
	if _, err := CheckLastResult(map[string]any{"result_id": json.Number("9"), "job_action": json.Number("1"), "status": json.Number("0"),
		"time_start": json.Number("100"), "time_end": json.Number("200")}, now.Unix()); err == nil {
		t.Error("last_result status 0 with an end time accepted")
	}
	body := `{"results":[{"result_id":1,"task_id":1,"status":1,"job_action":1,"time_start":100,"time_end":200}]}`
	if _, _, err := fetchResults(context.Background(), constCaller(body), 0, now.Unix(), 2); err == nil {
		t.Error("a listed running result with an end time accepted")
	}
}

// --- Codex round-14 regressions ---

func TestOlderFormatMirrorStartsOverButKeepsABBEvidence(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	// A format-1 mirror holding a far-future success and ABB evidence.
	if _, err := db.Exec(`INSERT INTO fleet_sync (nas, url, abb_installed, abb_seen, mirror_format) VALUES ('acme', '', 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO abb_results (nas, result_id, task_id, status, job_action, time_start, time_end) VALUES ('acme', 1, 1, 2, 1, ?, ?)`,
		ts(-100*day), ts(-100*day)+60); err != nil {
		t.Fatal(err)
	}
	if c, _ := ResultCursor(ctx, db, n); c != 0 {
		t.Fatalf("cursor taken from an older-format mirror: %d", c)
	}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks:   []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day)}},
		Results: []Result{{ResultID: 2, TaskID: 1, Status: StatusFailed, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour)}}})
	f, _ := Failures(ctx, db, []NAS{n}, now.Add(-day), now, 26*time.Hour)
	if len(f) != 1 || f[0].Kind != "run_failed" {
		t.Fatalf("legacy far-future success hid a current failure: %+v", f)
	}
	st, _ := LoadSyncStates(ctx, db)
	if !st["acme"].ABBSeen {
		t.Fatal("restart dropped the ABB evidence")
	}
	if st["acme"].HistoryFrom != 0 {
		t.Fatalf("restart kept the old coverage start %d", st["acme"].HistoryFrom)
	}
}

func TestSessionErrorIsNotABBEvidence(t *testing.T) {
	_, info, _ := Fetch(context.Background(), sessionErrCaller{}, 0, now, DefaultOptions)
	if info.ABBInstalled != nil {
		t.Fatalf("a session error recorded ABB installed=%v", *info.ABBInstalled)
	}
}

// sessionErrCaller answers system info and returns DSM 119 for ABB.
type sessionErrCaller struct{}

func (sessionErrCaller) Get(_ context.Context, path string, _ url.Values) (json.RawMessage, error) {
	if path == PathSystemInfo {
		return json.RawMessage(`{"model":"DS920+"}`), nil
	}
	return nil, &dsm.Error{Code: 119}
}

// --- Codex round-18 regressions ---

func TestTransfersShortOfTotalAndSharedHostNames(t *testing.T) {
	if _, err := parseDeviceRuns(json.RawMessage(`{"total":1,"device_list":[]}`), now.Unix()); err == nil {
		t.Error("a device listing short of its total was accepted")
	}
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day),
			Devices: []Device{{DeviceID: 1, HostName: "PC01"}, {DeviceID: 2, HostName: "PC01"}}}},
		Results: []Result{{ResultID: 5, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour), Hosts: "PC01"}}})
	s, _ := Stale(ctx, db, []NAS{n}, 36*time.Hour, now, 26*time.Hour)
	if len(s) != 2 {
		t.Fatalf("a task result naming a shared host name credited both devices: %+v", s)
	}
}

// --- Codex round-19 regressions ---

func TestFallbackMatchesDeviceIDsNotNames(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	// Device 2 replaced device 1 under the same name; the success names id 1.
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day), Devices: []Device{{DeviceID: 2, HostName: "PC01"}}}},
		Results: []Result{{ResultID: 5, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour),
			Hosts: "PC01", DeviceIDs: ",1,"}}})
	if s, _ := Stale(ctx, db, []NAS{n}, 36*time.Hour, now, 26*time.Hour); len(s) != 1 || s[0].Kind != "never_succeeded" {
		t.Fatalf("the replacement inherited its predecessor's backup: %+v", s)
	}
	// Transfer entries resolve one identity each; a duplicate via config_device_id is caught.
	dup := `{"total":2,"device_list":[{"device":{},"transfer_list":[{"device_result_id":1,"config_device_id":1,"status":2,"time_start":5}]},
	  {"device":{},"transfer_list":[{"device_result_id":2,"config_device_id":1,"status":2,"time_start":6}]}]}`
	if _, err := parseDeviceRuns(json.RawMessage(dup), now.Unix()); err == nil {
		t.Error("two entries for one device accepted")
	}
	// fetchResults records the device ids a result's task_config names.
	body := `{"results":[{"result_id":1,"task_id":1,"status":2,"job_action":1,"time_start":5,"task_config":{"device_list":[{"device_id":7,"host_name":"PC07"}]}}]}`
	got, _, err := fetchResults(ctx, constCaller(body), 0, now.Unix(), 2)
	if err != nil || len(got) != 1 || got[0].DeviceIDs != ",7," {
		t.Fatalf("device ids not recorded: %+v (%v)", got, err)
	}
}

// --- Codex round-20 regressions ---

func TestDeviceIDsAndTaskInventoryCompleteness(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	r := Result{ResultID: 5, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour), DeviceIDs: ",1,"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, Results: []Result{r}})
	at := now.Add(time.Minute)
	r2 := r
	r2.DeviceIDs = ",2,"
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, Results: []Result{r2}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a finished run's protected devices were rewritten")
	}
	r3 := r
	r3.DeviceIDs = ""
	at = at.Add(time.Minute)
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, Results: []Result{r3}}, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	var ids string
	_ = db.QueryRow(`SELECT device_ids FROM abb_results WHERE result_id = 5`).Scan(&ids)
	if ids != ",1," {
		t.Fatalf("a copy without device ids erased them: %q", ids)
	}
	if _, err := parseTasks(json.RawMessage(`{"total":1,"tasks":[]}`), now.Unix()); err == nil {
		t.Error("a task list short of its total was accepted")
	}
}

// --- Codex round-21 regressions ---

func TestDeviceSetsAndTransferAttribution(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	r := Result{ResultID: 5, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour), DeviceIDs: ",2,1,"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, Results: []Result{r}})
	at := now.Add(time.Minute)
	r.DeviceIDs = ",1,2,"
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, Results: []Result{r}}, SystemInfo{}, nil); err != nil {
		t.Fatalf("the same device set in another order failed: %v", err)
	}
	at = at.Add(time.Minute)
	dr := DeviceRun{DeviceResultID: 1, ResultID: 5, DeviceID: 3, Status: StatusSuccess, Start: ts(2 * time.Hour), End: ts(time.Hour)}
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, DeviceRuns: []DeviceRun{dr}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a transfer crediting a device its result did not protect was committed")
	}
	if err := CheckTaskInventory(json.RawMessage(`{"tasks":[{"task_id":8},{"task_id":8}]}`)); err == nil {
		t.Error("duplicate task ids accepted")
	}
}

// --- Codex round-22 regressions ---

func TestRetainedParentAndLateDeviceSets(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	// Sync A: result 5 with an unknown device set, and a transfer for device 2.
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Results:    []Result{{ResultID: 5, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour)}},
		DeviceRuns: []DeviceRun{{DeviceResultID: 1, ResultID: 5, DeviceID: 2, Status: StatusSuccess, Start: ts(2 * time.Hour), End: ts(time.Hour)}}})
	// Sync B: result 5 turns out to have protected only device 1.
	at := now.Add(time.Minute)
	err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true,
		Results: []Result{{ResultID: 5, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour), DeviceIDs: ",1,"}}}, SystemInfo{}, nil)
	if err == nil {
		t.Fatal("a known device set contradicting a mirrored transfer was committed")
	}
}

// --- Codex round-23 regression ---

func TestFormat3MirrorIsNotTrusted(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day), Devices: []Device{{DeviceID: 2, HostName: "PC02"}}}}})
	// A mirror written before transfers were checked against their result's
	// device set holds a transfer crediting device 2 to a run that protected 1.
	for _, q := range []string{
		`INSERT INTO abb_results (nas, result_id, task_id, status, job_action, time_start, time_end, device_ids) VALUES ('acme', 5, 1, 2, 1, ?, ?, ',1,')`,
		`INSERT INTO abb_device_runs (nas, device_result_id, result_id, device_id, status, time_start, time_end) VALUES ('acme', 1, 5, 2, 2, ?, ?)`,
		`UPDATE fleet_sync SET mirror_format = 3`,
	} {
		args := []any{}
		if strings.Contains(q, "?") {
			args = []any{ts(2 * time.Hour), ts(time.Hour)}
		}
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := Stale(ctx, db, []NAS{n}, 36*time.Hour, now, 26*time.Hour)
	if len(s) != 1 || s[0].Kind != "nas_not_synced" {
		t.Fatalf("a format-3 mirror was trusted: %+v", s)
	}
}

// --- Codex round-24 regressions ---

func TestOverflowIDsAndNonBackupParents(t *testing.T) {
	if _, err := parseTasks(json.RawMessage(`{"tasks":[{"task_id":1,"devices":[{"device_id":9223372036854775808}]}]}`), now.Unix()); err == nil {
		t.Error("an overflowing device_id was accepted")
	}
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	deletion := Result{ResultID: 6, TaskID: 1, Status: StatusSuccess, JobAction: 131072, Start: ts(2 * time.Hour), End: ts(time.Hour), DeviceIDs: ",2,"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, Results: []Result{deletion}})
	at := now.Add(time.Minute)
	dr := DeviceRun{DeviceResultID: 1, ResultID: 6, DeviceID: 2, Status: StatusSuccess, Start: ts(2 * time.Hour), End: ts(time.Hour)}
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, DeviceRuns: []DeviceRun{dr}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a transfer under a version deletion was committed")
	}
	// The parent becomes known as a non-backup after its transfer was mirrored.
	db2 := openDB(t)
	commitTasks(t, db2, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		DeviceRuns: []DeviceRun{{DeviceResultID: 1, ResultID: 7, DeviceID: 2, Status: StatusSuccess, Start: ts(2 * time.Hour), End: ts(time.Hour)}}})
	if err := CommitSync(ctx, db2, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true,
		Results: []Result{{ResultID: 7, TaskID: 1, Status: StatusSuccess, JobAction: 131072, Start: ts(2 * time.Hour), End: ts(time.Hour)}}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a mirrored transfer's parent turned out to be a deletion and was accepted")
	}
}

// --- Codex round-25 regressions ---

func TestLastResultNamingAnotherTask(t *testing.T) {
	body := `{"tasks":[{"task_id":1,"devices":[],"last_result":{"result_id":9,"task_id":2,"status":2,"job_action":1,"time_start":100,"time_end":200}}]}`
	if _, err := parseTasks(json.RawMessage(body), now.Unix()); err == nil {
		t.Fatal("a last_result naming another task was accepted")
	}
}

// --- Codex round-26 regressions ---

func TestRunningLastResultIdentityIsChecked(t *testing.T) {
	tasks := []Task{{TaskID: 1, LastResultID: 9, LastStatus: StatusRunning, LastJobAction: JobBackup, LastStart: 500, Running: true}}
	listed := []Result{{ResultID: 9, TaskID: 2, Status: StatusFailed, JobAction: JobBackup, Start: 500, End: 600}}
	if _, err := mergeLastResults(listed, tasks); err == nil {
		t.Fatal("a running last_result owned by another task in the listing was accepted")
	}
	// Against the mirror too.
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, Results: []Result{listed[0]}})
	at := now.Add(time.Minute)
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true,
		Results: []Result{{ResultID: 9, TaskID: 1, Status: StatusRunning, JobAction: JobBackup, Start: 500, FromTask: true}}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a task run contradicting the mirror was committed")
	}
}

// --- Codex round-27 regressions ---

func TestTaskReportedRunsAreConsistentAndRemembered(t *testing.T) {
	two := []Task{
		{TaskID: 1, LastResultID: 9, LastStatus: StatusSuccess, LastJobAction: JobBackup, LastStart: 500, LastEnd: 600},
		{TaskID: 2, LastResultID: 9, LastStatus: StatusRunning, LastJobAction: JobBackup, LastStart: 500},
	}
	if _, err := mergeLastResults(nil, two); err == nil {
		t.Fatal("one result claimed as the last result of two tasks was accepted")
	}
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	running := []Task{{TaskID: 1, Name: "PCs", LastResultID: 9, LastStatus: StatusRunning, LastJobAction: JobBackup, LastStart: ts(2 * time.Hour), Running: true}}
	merged, err := mergeLastResults(nil, running)
	if err != nil {
		t.Fatal(err)
	}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, Tasks: running, Results: merged})
	// Next sync: the same result id now claims a different start.
	later := []Task{{TaskID: 1, Name: "PCs", LastResultID: 9, LastStatus: StatusSuccess, LastJobAction: JobBackup, LastStart: ts(time.Hour), LastEnd: ts(30 * time.Minute)}}
	merged, _ = mergeLastResults(nil, later)
	at := now.Add(time.Minute)
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, Tasks: later, Results: merged}, SystemInfo{}, nil); err == nil {
		t.Fatal("a task-reported run changed identity between syncs and was accepted")
	}
}

// --- Codex round-28 regressions ---

func TestUnfinishedTaskRunAndLateDeletionParent(t *testing.T) {
	got, err := mergeLastResults(nil, []Task{{TaskID: 1, LastResultID: 9, LastStatus: 0, LastJobAction: JobBackup, LastStart: 500}})
	if err != nil || got[0].Status != StatusRunning {
		t.Fatalf("an unfinished (status 0) task run was mirrored as %+v (%v)", got, err)
	}
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		DeviceRuns: []DeviceRun{{DeviceResultID: 1, ResultID: 7, DeviceID: 2, Status: StatusSuccess, Start: ts(2 * time.Hour), End: ts(time.Hour)}}})
	at := now.Add(time.Minute)
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true,
		Results: []Result{{ResultID: 7, TaskID: 1, Status: StatusSuccess, JobAction: 131072, Start: ts(2 * time.Hour), End: ts(time.Hour), FromTask: true}}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a task-only deletion parent of a mirrored transfer was committed")
	}
}

// --- Codex round-29 regressions ---

func TestStatusZeroIsUnfinishedInReports(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "PCs", Scheduled: true, NextTrigger: ts(-day), Running: true}}})
	if _, err := db.Exec(`INSERT INTO abb_results (nas, result_id, task_id, status, job_action, time_start, time_end) VALUES ('acme', 3, 1, 0, 1, ?, 0)`, ts(time.Hour)); err != nil {
		t.Fatal(err)
	}
	f, _ := Failures(ctx, db, []NAS{n}, now.Add(-day), now, 26*time.Hour)
	for _, x := range f {
		if x.Kind == "run_failed" {
			t.Fatalf("an unfinished (status 0) run read as a failure: %+v", f)
		}
	}
}

// --- Codex round-34 regression ---

func TestValidationErrorsDoNotQuotePayloadValues(t *testing.T) {
	_, err := parseTasks(json.RawMessage(`{"total":"SIDSECRET34","tasks":[]}`), now.Unix())
	if err == nil || strings.Contains(err.Error(), "SIDSECRET34") {
		t.Fatalf("err = %v", err)
	}
}

// --- Codex round-37 regression ---

func TestTransferMustFallWithinItsRun(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Results: []Result{{ResultID: 5, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(11 * day), End: ts(10 * day)}}})
	at := now.Add(time.Minute)
	dr := DeviceRun{DeviceResultID: 1, ResultID: 5, DeviceID: 2, Status: StatusSuccess, Start: ts(2 * time.Hour), End: ts(time.Hour)}
	if err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true, DeviceRuns: []DeviceRun{dr}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a transfer from an hour ago credited to a run that ended ten days ago was committed")
	}
	// Parent arriving after its transfer.
	db2 := openDB(t)
	commitTasks(t, db2, n, &Snapshot{TakenAt: now, ABBInstalled: true, DeviceRuns: []DeviceRun{dr}})
	if err := CommitSync(ctx, db2, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true,
		Results: []Result{{ResultID: 5, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(11 * day), End: ts(10 * day)}}}, SystemInfo{}, nil); err == nil {
		t.Fatal("a late parent contradicting its mirrored transfer's time was committed")
	}
}

// --- Codex round-38 regression ---

func TestTransferWindowCheckedAfterMerge(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	// A recent transfer with no parent yet.
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		DeviceRuns: []DeviceRun{{DeviceResultID: 1, DeviceID: 2, Status: StatusSuccess, Start: ts(2 * time.Hour), End: ts(time.Hour)}}})
	at := now.Add(time.Minute)
	// The old parent arrives, and the transfer is re-listed under it with no end.
	err := CommitSync(ctx, db, n, at, &Snapshot{TakenAt: at, ABBInstalled: true, Storage: vol(), StorageOK: true,
		Results:    []Result{{ResultID: 5, TaskID: 1, Status: StatusSuccess, JobAction: JobBackup, Start: ts(11 * day), End: ts(10 * day)}},
		DeviceRuns: []DeviceRun{{DeviceResultID: 1, ResultID: 5, DeviceID: 2, Status: StatusSuccess, Start: ts(2 * time.Hour)}}}, SystemInfo{}, nil)
	if err == nil {
		t.Fatal("a merged transfer outside its parent's time was committed")
	}
}
