// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. End-to-end through the Cobra tree: register a fake NAS, sync
// it, and check every fleet/abb novel command's output and exit code.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
)

func fakeABBNAS(t *testing.T) *httptest.Server {
	t.Helper()
	now := time.Now()
	at := func(ago time.Duration) int64 { return now.Add(-ago).Unix() }
	body := map[string]string{
		"SYNO.API.Info/query":   `{"SYNO.ActiveBackup.Task":{"path":"entry.cgi","minVersion":1,"maxVersion":2}}`,
		"SYNO.Core.System/info": `{"model":"DS1621+","firmware_ver":"DSM 7.2.2"}`,
		"SYNO.ActiveBackup.Task/list": fmt.Sprintf(`{"tasks":[
 {"task_id":8,"task_name":"File Server","backup_type":4,"next_trigger_time":%d,
  "last_result":{"result_id":593,"status":4,"job_action":1,"time_start":%d,"time_end":%d},
  "devices":[{"device_id":21,"host_name":"FS01","os_name":"Windows Server 2022","agent_status":"online"}]},
 {"task_id":5,"task_name":"Office PCs","backup_type":2,"next_trigger_time":%d,
  "last_result":{"result_id":590,"status":2,"job_action":1,"time_start":%d,"time_end":%d},
  "devices":[{"device_id":11,"host_name":"BOB-LAPTOP","agent_status":"online"}]}]}`,
			at(-12*time.Hour), at(6*time.Hour), at(5*time.Hour), at(-12*time.Hour), at(3*time.Hour), at(2*time.Hour)),
		"SYNO.ActiveBackup.Log/list_result": fmt.Sprintf(`{"results":[
 {"result_id":593,"task_id":8,"task_name":"File Server","status":4,"job_action":1,"time_start":%d,"time_end":%d,"error_count":2,"task_config":{"device_list":[{"host_name":"FS01"}]}},
 {"result_id":590,"task_id":5,"task_name":"Office PCs","status":2,"job_action":1,"time_start":%d,"time_end":%d,"task_config":{"device_list":[{"host_name":"BOB-LAPTOP"}]}}]}`,
			at(6*time.Hour), at(5*time.Hour), at(3*time.Hour), at(2*time.Hour)),
		"SYNO.ActiveBackup.Log/list_result_detail": fmt.Sprintf(`{"result_detail_list":[
 {"log_level":2,"log_time":%d,"log_type":1104,"other_params":{}},
 {"log_level":0,"log_time":%d,"log_type":6002,"other_params":{"path":"E:/data","fs_error":-65}}]}`, at(5*time.Hour), at(6*time.Hour)),
		"SYNO.ActiveBackup.Log/list_log": fmt.Sprintf(`{"count":1,"logs":[
 {"log_level":1,"log_time":%d,"log_type":1500,"device_name":"FS01","other_params":{"task_id":8}}]}`, at(5*time.Hour+30*time.Minute)),
		"SYNO.ActiveBackup.Overview/list_device_transfer_size": fmt.Sprintf(`{"device_list":[
 {"device":{"device_id":11,"host_name":"BOB-LAPTOP"},"transfer_list":[{"device_result_id":1,"result_id":590,"status":2,"time_start":%d,"time_end":%d}]},
 {"device":{"device_id":21,"host_name":"FS01"},"transfer_list":[{"device_result_id":2,"result_id":593,"status":4,"time_start":%d,"time_end":%d}]}]}`,
			at(3*time.Hour), at(2*time.Hour), at(6*time.Hour), at(5*time.Hour)),
		"SYNO.Storage.CGI.Storage/load_info": `{"volumes":[{"id":"volume_1","vol_path":"/volume1","status":"normal","size":{"total":"1000","used":"950"}}],
 "storagePools":[{"id":"reuse_1","status":"normal"}],
 "disks":[{"id":"sata1","name":"Drive 1","device":"/dev/sata1","temp":38,"status":"normal","smart_status":"normal"}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			if r.PostForm.Get("account") != "msp-admin" || r.PostForm.Get("passwd") != "s3cret" {
				_, _ = w.Write([]byte(`{"success":false,"error":{"code":400}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`))
			return
		}
		q := r.URL.Query()
		if q.Get("method") == "logout" {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		if q.Get("_sid") != "sid-1" || r.Header.Get("X-SYNO-TOKEN") != "tok-1" {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":119}}`))
			return
		}
		b, ok := body[q.Get("api")+"/"+q.Get("method")]
		if !ok {
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	code := 0
	if err != nil {
		code = ExitCode(err)
	}
	return out.String(), code
}

func TestFleetEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeABBNAS(t)
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")

	out, code := runCLI(t, "fleet", "add", "acme", "--url", srv.URL, "--client", "Acme Corp", "--check", "--json")
	if code != 0 || !strings.Contains(out, `"abb_available": true`) {
		t.Fatalf("fleet add: code=%d out=%s", code, out)
	}
	if strings.Contains(out, "s3cret") {
		t.Fatal("fleet add echoed the password")
	}

	out, code = runCLI(t, "fleet", "sync", "--json")
	if code != 0 || !strings.Contains(out, `"ok": true`) || !strings.Contains(out, `"tasks": 2`) {
		t.Fatalf("fleet sync: code=%d out=%s", code, out)
	}

	out, code = runCLI(t, "fleet", "status", "--json")
	var status []map[string]any
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("fleet status JSON: %v\n%s", err, out)
	}
	if code != exitFindings || len(status) != 1 || status[0]["state"] != "critical" || status[0]["tasks_failed"] != float64(1) || status[0]["client"] != "Acme Corp" {
		t.Fatalf("fleet status: code=%d %+v", code, status)
	}
	if _, code = runCLI(t, "fleet", "status", "--json", "--no-fail"); code != 0 {
		t.Fatalf("--no-fail exit = %d", code)
	}

	out, code = runCLI(t, "abb", "failures", "--since", "24h", "--json")
	if code != exitFindings || !strings.Contains(out, `"task_name": "File Server"`) || strings.Contains(out, "Office PCs") ||
		!strings.Contains(out, "fs_error=-65") {
		t.Fatalf("abb failures: code=%d out=%s", code, out)
	}

	out, code = runCLI(t, "abb", "stale", "--older-than", "36h", "--json")
	if code != exitFindings || !strings.Contains(out, `"host_name": "FS01"`) || strings.Contains(out, "BOB-LAPTOP") {
		t.Fatalf("abb stale: code=%d out=%s", code, out)
	}

	out, code = runCLI(t, "fleet", "health", "--full-pct", "90", "--json")
	if code != exitFindings || !strings.Contains(out, `"used_pct": 95`) {
		t.Fatalf("fleet health: code=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "fleet", "health", "--full-pct", "99", "--json"); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("fleet health clean: code=%d out=%q", code, out)
	}

	out, code = runCLI(t, "abb", "device", "bob", "--json", "--select", "host_name,task_name")
	if code != 0 || !strings.Contains(out, "BOB-LAPTOP") || !strings.Contains(out, "Office PCs") || strings.Contains(out, "recent_runs") {
		t.Fatalf("abb device: code=%d out=%s", code, out)
	}

	// An unknown host on fresh mirrors is "not found" (exit 3), never an empty yes.
	if out, code = runCLI(t, "abb", "device", "no-such-host", "--json"); code != 3 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("unknown host: code=%d out=%q", code, out)
	}

	out, code = runCLI(t, "abb", "trend", "--since", "7d", "--json")
	if code != 0 || !strings.Contains(out, `"key": "File Server"`) {
		t.Fatalf("abb trend: code=%d out=%s", code, out)
	}

	out, code = runCLI(t, "abb", "triage", "file", "--nas", "acme", "--json")
	var tri map[string]any
	if err := json.Unmarshal([]byte(out), &tri); err != nil || code != 0 {
		t.Fatalf("abb triage: code=%d err=%v out=%s", code, err, out)
	}
	tl, _ := tri["timeline"].([]any)
	if tri["status"] != "failed" || len(tl) != 3 || tl[0].(map[string]any)["level"] != "error" {
		t.Fatalf("triage report = %+v", tri)
	}

	out, code = runCLI(t, "fleet", "remove", "acme", "--json")
	if code != 0 || !strings.Contains(out, `"mirror_purged": true`) {
		t.Fatalf("fleet remove: code=%d out=%s", code, out)
	}
	// Nothing registered any more: a config error, never an empty "all clear".
	if out, code = runCLI(t, "fleet", "status", "--json"); code != 10 || strings.TrimSpace(out) != "" {
		t.Fatalf("empty fleet status: code=%d out=%q", code, out)
	}
}

func TestFleetSyncWrongPasswordIsRecorded(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeABBNAS(t)
	t.Setenv("SYNOLOGY_LAB_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_LAB_PASSWORD", "wrong")
	if _, code := runCLI(t, "fleet", "add", "lab", "--url", srv.URL); code != 0 {
		t.Fatalf("add exit %d", code)
	}
	out, code := runCLI(t, "fleet", "sync", "--json")
	if code != 5 || !strings.Contains(out, "invalid account or password") || strings.Contains(out, "wrong") {
		t.Fatalf("sync with bad password: code=%d out=%s", code, out)
	}
	out, code = runCLI(t, "fleet", "status", "--json")
	if code != exitFindings || !strings.Contains(out, `"state": "unknown"`) {
		t.Fatalf("status after failed sync: code=%d out=%s", code, out)
	}
}

func TestFleetCommandsDryRunAndEmpty(t *testing.T) {
	testenv.Isolate(t)
	for _, args := range [][]string{
		{"fleet", "sync", "--dry-run", "--json"}, {"fleet", "add", "x", "--url", "https://n:5001", "--dry-run", "--json"},
		{"abb", "triage", "x", "--dry-run", "--json"}, {"abb", "failures", "--dry-run", "--json"},
	} {
		out, code := runCLI(t, args...)
		if code != 0 || !strings.Contains(out, `"dry_run":true`) {
			t.Fatalf("%v: code=%d out=%s", args, code, out)
		}
	}
	// With no NAS configured, every rollup is a config error (exit 10), never
	// an empty result an RMM check would read as "all clear".
	for _, args := range [][]string{{"fleet", "status", "--json"}, {"abb", "failures", "--json"}, {"abb", "stale", "--json"},
		{"fleet", "health", "--json"}, {"abb", "device", "pc", "--json"}, {"abb", "trend", "--json"}} {
		out, code := runCLI(t, args...)
		if code != 10 || strings.TrimSpace(out) != "" {
			t.Fatalf("%v on an empty fleet: code=%d out=%q", args, code, out)
		}
	}
	if out, code := runCLI(t, "fleet", "list", "--json"); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("fleet list on an empty fleet: code=%d out=%q", code, out)
	}
	if _, code := runCLI(t, "fleet", "sync", "--json"); code != 10 {
		t.Fatalf("fleet sync with nothing configured exit = %d, want 10", code)
	}
	if _, code := runCLI(t, "fleet", "add", "Bad Name", "--url", "https://n:5001"); code != 2 {
		t.Fatalf("invalid name exit = %d, want 2", code)
	}
}

func TestFleetAddGuards(t *testing.T) {
	testenv.Isolate(t)
	if _, code := runCLI(t, "fleet", "add", "acme-hq", "--url", "https://a:5001"); code != 0 {
		t.Fatalf("first add exit %d", code)
	}
	// acme_hq folds to the same SYNOLOGY_ACME_HQ prefix: refuse silently sharing credentials.
	if _, code := runCLI(t, "fleet", "add", "acme_hq", "--url", "https://b:5001"); code != 2 {
		t.Fatalf("prefix collision exit %d, want 2", code)
	}
	if _, code := runCLI(t, "fleet", "add", "acme_hq", "--url", "https://b:5001", "--env-prefix", "SYNOLOGY_ACME_HQ"); code != 0 {
		t.Fatalf("deliberate shared prefix exit %d, want 0", code)
	}
	if _, code := runCLI(t, "fleet", "add", "lab", "--url", "https://c:5001", "--pin-cert", "--tls-fingerprint", "ab"); code != 2 {
		t.Fatalf("pin + fingerprint exit %d, want 2", code)
	}
	if out, code := runCLI(t, "fleet", "remove", "ACME-HQ", "--json"); code != 0 || !strings.Contains(out, `"removed": "acme-hq"`) {
		t.Fatalf("case-insensitive remove: code=%d out=%s", code, out)
	}
}

func TestFleetRepointPurgesAndValidates(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeABBNAS(t)
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	if _, code := runCLI(t, "fleet", "add", "acme", "--url", srv.URL); code != 0 {
		t.Fatal("add")
	}
	if _, code := runCLI(t, "fleet", "sync"); code != 0 {
		t.Fatal("sync")
	}
	// Same name, another NAS: the old mirror must not answer for it.
	if _, code := runCLI(t, "fleet", "add", "acme", "--url", "https://replacement.invalid:5001"); code != 0 {
		t.Fatal("re-add")
	}
	out, code := runCLI(t, "fleet", "status", "--json")
	if code != exitFindings || !strings.Contains(out, `"state": "unknown"`) {
		t.Fatalf("status after re-point: code=%d out=%s", code, out)
	}
	if _, code := runCLI(t, "fleet", "add", "x", "--url", "https://admin:pw@nas:5001"); code != 2 {
		t.Fatalf("userinfo URL exit %d, want 2", code)
	}
	if _, code := runCLI(t, "fleet", "sync", "--nas-timeout", "0s"); code != 2 {
		t.Fatalf("--nas-timeout 0 exit %d, want 2", code)
	}
}

func TestFleetURLAndPinGuards(t *testing.T) {
	testenv.Isolate(t)
	if _, code := runCLI(t, "fleet", "add", "a", "--url", "https://a:5001", "--tls-fingerprint", "sha256/"); code != 2 {
		t.Fatalf("empty-after-normalization pin exit %d, want 2", code)
	}
	if _, code := runCLI(t, "fleet", "add", "b", "--url", "http://b:5000", "--tls-fingerprint", strings.Repeat("ab", 32)); code != 2 {
		t.Fatalf("pin on http:// exit %d, want 2", code)
	}
	// The implicit NAS must not store or print a password in SYNOLOGY_BASE_URL.
	t.Setenv("SYNOLOGY_BASE_URL", "https://admin:testpass0@nas:5001")
	out, code := runCLI(t, "fleet", "list", "--json")
	if code != 10 || strings.Contains(out, "testpass0") {
		t.Fatalf("userinfo base URL: code=%d out=%s", code, out)
	}
}

func TestTriageValidatesRowsAndNotesNewerSuccess(t *testing.T) {
	results := map[string]string{
		"recovered": `{"results":[
 {"result_id":3,"task_id":8,"status":2,"job_action":1,"time_start":300},
 {"result_id":2,"task_id":9,"status":4,"job_action":1,"time_start":250},
 {"result_id":1,"task_id":8,"status":4,"job_action":1,"time_start":100}]}`,
		"malformed": `{"results":[{"result_id":3,"task_id":8,"status":4,"time_start":300}]}`,
	}
	for name, res := range results {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
				return
			}
			b := map[string]string{
				"SYNO.ActiveBackup.Task/list":              `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
				"SYNO.ActiveBackup.Log/list_result":        res,
				"SYNO.ActiveBackup.Log/list_result_detail": `{"result_detail_list":[]}`,
				"SYNO.ActiveBackup.Log/list_log":           `{"logs":[]}`,
			}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
			_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
		}))
		c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
		rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20)
		srv.Close()
		switch name {
		case "recovered":
			if err != nil || numOf(rep.Result["result_id"]) != 1 || !strings.Contains(rep.Note, "newer backup run succeeded") {
				t.Fatalf("recovered: rep=%+v err=%v", rep, err)
			}
		case "malformed":
			if err == nil {
				t.Fatalf("malformed row read as a report: %+v", rep)
			}
		}
	}
}

func TestTriageRejectsRowsWithoutTaskOrStart(t *testing.T) {
	for name, res := range map[string]string{
		"no task_id":    `{"results":[{"result_id":3,"status":4,"job_action":1,"time_start":300}]}`,
		"no time_start": `{"results":[{"result_id":3,"task_id":8,"status":4,"job_action":1},{"result_id":1,"task_id":8,"status":4,"job_action":1,"time_start":100}]}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
				return
			}
			b := map[string]string{
				"SYNO.ActiveBackup.Task/list":       `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
				"SYNO.ActiveBackup.Log/list_result": res,
			}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
			_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
		}))
		c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
		rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20)
		srv.Close()
		if err == nil {
			t.Fatalf("%s: accepted, report %+v", name, rep)
		}
	}
}

func TestTriageTrimmedNumbersAndZeroStart(t *testing.T) {
	cases := map[string]struct {
		res  string
		want int64 // selected result_id; 0 = expect an error
	}{
		"trimmed job_action": {`{"results":[{"result_id":3,"task_id":8,"status":4,"job_action":" 1 ","time_start":300}]}`, 3},
		"zero time_start":    {`{"results":[{"result_id":3,"task_id":8,"status":4,"job_action":1,"time_start":0}]}`, 0},
	}
	for name, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
				return
			}
			b := map[string]string{
				"SYNO.ActiveBackup.Task/list":              `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
				"SYNO.ActiveBackup.Log/list_result":        tc.res,
				"SYNO.ActiveBackup.Log/list_result_detail": `{"result_detail_list":[]}`,
				"SYNO.ActiveBackup.Log/list_log":           `{"logs":[]}`,
			}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
			_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
		}))
		c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
		rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20)
		srv.Close()
		if tc.want == 0 {
			if err == nil {
				t.Errorf("%s: accepted", name)
			}
			continue
		}
		if err != nil || numOf(rep.Result["result_id"]) != tc.want {
			t.Errorf("%s: rep=%+v err=%v", name, rep, err)
		}
	}
}

func TestMalformedConfigIsNotAnEmptyFleet(t *testing.T) {
	testenv.Isolate(t)
	cfg := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfg, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SYNOLOGY_CONFIG", cfg)
	if out, code := runCLI(t, "fleet", "list", "--json"); code != 10 {
		t.Fatalf("malformed config: code=%d out=%s", code, out)
	}
}

func TestTriageRejectsFutureSuccess(t *testing.T) {
	future := time.Now().Add(10 * 24 * time.Hour).Unix()
	res := fmt.Sprintf(`{"results":[{"result_id":5,"task_id":8,"status":2,"job_action":1,"time_start":%d},
 {"result_id":3,"task_id":8,"status":4,"job_action":1,"time_start":300}]}`, future)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list":       `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
			"SYNO.ActiveBackup.Log/list_result": res,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20); err == nil {
		t.Fatalf("a success dated ten days ahead was accepted: %+v", rep)
	}
}

func TestFleetAddFailedCheckRollsBack(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeABBNAS(t)
	t.Setenv("SYNOLOGY_LAB_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_LAB_PASSWORD", "wrong")
	if _, code := runCLI(t, "fleet", "add", "lab", "--url", srv.URL, "--check"); code == 0 {
		t.Fatal("a failed --check registered the NAS")
	}
	if out, _ := runCLI(t, "fleet", "list", "--json"); strings.Contains(out, `"lab"`) {
		t.Fatalf("failed --check left the NAS registered: %s", out)
	}
}

func TestTriageWithoutEndTimeReadsLogToNextRun(t *testing.T) {
	res := `{"results":[{"result_id":3,"task_id":8,"status":4,"job_action":1,"time_start":300}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list":              `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
			"SYNO.ActiveBackup.Log/list_result":        res,
			"SYNO.ActiveBackup.Log/list_result_detail": `{"result_detail_list":[]}`,
			"SYNO.ActiveBackup.Log/list_log":           `{"logs":[]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20)
	if err != nil || !strings.Contains(rep.Note, "no end time") {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
}

func TestTriageRejectsMalformedLastResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list":       `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[],"last_result":{"result_id":9,"status":4,"job_action":1}}]}`,
			"SYNO.ActiveBackup.Log/list_result": `{"results":[]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20); err == nil {
		t.Fatalf("a failed last_result without time_start read as no failure: %+v", rep)
	}
}

func TestWriteDryRunRefusesPasswordInAddress(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SYNOLOGY_BASE_URL", "http://user:testpass0@127.0.0.1:9")
	out, code := runCLI(t, "abb", "backup", "--task-ids", "[5]", "--dry-run", "--json")
	if code == 0 || strings.Contains(out, "testpass0") {
		t.Fatalf("dry run with a password in the address: code=%d out=%s", code, out)
	}
	t.Setenv("SYNOLOGY_BASE_URL", "https://nas.example.com:5001")
	if out, code := runCLI(t, "abb", "backup", "--task-ids", "[5]", "--dry-run", "--json"); code != 0 || !strings.Contains(out, `"dry_run":true`) {
		t.Fatalf("placeholder dry run: code=%d out=%s", code, out)
	}
}

func TestTriageRejectsConflictingDuplicates(t *testing.T) {
	res := `{"results":[{"result_id":9,"task_id":8,"status":2,"job_action":1,"time_start":500,"time_end":600},
 {"result_id":9,"task_id":8,"status":4,"job_action":1,"time_start":300,"time_end":400}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list":       `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
			"SYNO.ActiveBackup.Log/list_result": res,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20); err == nil {
		t.Fatalf("conflicting copies of one run accepted: %+v", rep)
	}
}

func TestDryRunsRefusePasswordInAddress(t *testing.T) {
	testenv.Isolate(t)
	if _, code := runCLI(t, "fleet", "add", "x", "--url", "http://user:testpass0@127.0.0.1:9", "--dry-run", "--json"); code == 0 {
		t.Fatal("fleet add --dry-run accepted an address with a password")
	}
	t.Setenv("SYNOLOGY_BASE_URL", "http://user:testpass0@127.0.0.1:9")
	if out, code := runCLI(t, "auth", "login", "--dry-run", "--json"); code == 0 || strings.Contains(out, "testpass0") {
		t.Fatalf("auth login --dry-run: code=%d out=%s", code, out)
	}
}

func TestTriageLastResultOwnedByAnotherTask(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list":       `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[],"last_result":{"result_id":77,"status":2,"job_action":1,"time_start":300,"time_end":400}}]}`,
			"SYNO.ActiveBackup.Log/list_result": `{"results":[{"result_id":77,"task_id":9,"status":4,"job_action":1,"time_start":300,"time_end":400}]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20); err == nil {
		t.Fatalf("a last_result owned by another task was accepted: %+v", rep)
	}
}

func TestTriageRejectsListingShortOfTotal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list":       `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
			"SYNO.ActiveBackup.Log/list_result": `{"total":2,"results":[]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20); err == nil {
		t.Fatalf("an empty listing with total 2 read as no failures: %+v", rep)
	}
}

func TestTriageCountsDistinctRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		row := `{"result_id":3,"task_id":8,"status":2,"job_action":1,"time_start":300,"time_end":400}`
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list":       `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
			"SYNO.ActiveBackup.Log/list_result": `{"total":2,"results":[` + row + `,` + row + `]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20); err == nil {
		t.Fatalf("a repeated row counted as the second of two runs: %+v", rep)
	}
}

func TestTriageCountsOnlyThisTasksBackups(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list": `{"tasks":[{"task_id":8,"task_name":"File Server","devices":[]}]}`,
			"SYNO.ActiveBackup.Log/list_result": `{"total":2,"results":[{"result_id":3,"task_id":8,"status":2,"job_action":1,"time_start":300,"time_end":400},
 {"result_id":4,"task_id":8,"status":2,"job_action":131072,"time_start":500,"time_end":600}]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20); err == nil {
		t.Fatalf("a version deletion counted as the second backup run: %+v", rep)
	}
}

func TestTriageRejectsNonPositiveScan(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SYNOLOGY_BASE_URL", "https://127.0.0.1:9")
	if _, code := runCLI(t, "abb", "triage", "8", "--scan", "0"); code != 2 {
		t.Fatalf("--scan 0 exit %d, want 2", code)
	}
}

func TestTriageTaskIDBeatsNumericName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		res := `{"results":[]}`
		if r.URL.Query().Get("task_id") == "8" {
			res = `{"results":[{"result_id":3,"task_id":8,"status":4,"job_action":1,"time_start":300,"time_end":400}]}`
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list":              `{"tasks":[{"task_id":9,"task_name":"8","devices":[]},{"task_id":8,"task_name":"File Server","devices":[]}]}`,
			"SYNO.ActiveBackup.Log/list_result":        res,
			"SYNO.ActiveBackup.Log/list_result_detail": `{"result_detail_list":[]}`,
			"SYNO.ActiveBackup.Log/list_log":           `{"logs":[]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "8", 20, 20)
	if err != nil || rep.TaskID != 8 {
		t.Fatalf("task id 8 lost to a task named \"8\": rep=%+v err=%v", rep, err)
	}
}

func TestAuthLogoutForgetsCachedSession(t *testing.T) {
	testenv.Isolate(t)
	base := "https://127.0.0.1:9"
	t.Setenv("SYNOLOGY_BASE_URL", base)
	t.Setenv("SYNOLOGY_ACCOUNT", "admin")
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, dsm.SessionFileName(base))
	if err := dsm.WriteSessionRecord(path, []byte(`{"token":"sid-1","account":"admin","origin":"`+base+`"}`), "admin", "did-1"); err != nil {
		t.Fatal(err)
	}
	if _, code := runCLI(t, "auth", "logout", "--json"); code != 0 {
		t.Fatalf("auth logout exit %d", code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		raw, _ := os.ReadFile(path)
		t.Fatalf("auth logout left the cached session: %s", raw)
	}
}

func TestAuthLogoutDryRunChangesNothing(t *testing.T) {
	testenv.Isolate(t)
	base := "https://127.0.0.1:9"
	t.Setenv("SYNOLOGY_BASE_URL", base)
	t.Setenv("SYNOLOGY_ACCOUNT", "admin")
	dir, _ := cliutil.DataDir()
	path := filepath.Join(dir, dsm.SessionFileName(base))
	_ = dsm.WriteSessionRecord(path, []byte(`{"token":"sid-1","account":"admin","origin":"`+base+`"}`), "admin", "did-1")
	if _, code := runCLI(t, "auth", "logout", "--dry-run", "--json"); code != 0 {
		t.Fatalf("dry run exit %d", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("auth logout --dry-run removed the cached session")
	}
}

func TestTriageRejectsSharedLastResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list": `{"tasks":[{"task_id":1,"task_name":"A","devices":[],"last_result":{"result_id":9,"status":2,"job_action":1,"time_start":300,"time_end":400}},
 {"task_id":2,"task_name":"B","devices":[],"last_result":{"result_id":9,"status":1,"job_action":1,"time_start":300}}]}`,
			"SYNO.ActiveBackup.Log/list_result": `{"results":[]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "1", 20, 20); err == nil {
		t.Fatalf("one result claimed by two tasks was accepted: %+v", rep)
	}
}

func TestTriageChecksOtherTasksClaimsAgainstListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list": `{"tasks":[{"task_id":1,"task_name":"A","devices":[]},
 {"task_id":2,"task_name":"B","devices":[],"last_result":{"result_id":9,"status":4,"job_action":1,"time_start":300,"time_end":400}}]}`,
			"SYNO.ActiveBackup.Log/list_result": `{"results":[{"result_id":9,"task_id":1,"status":2,"job_action":1,"time_start":300,"time_end":400}]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "1", 20, 20); err == nil {
		t.Fatalf("another task's claim on a listed run was ignored: %+v", rep)
	}
}

func TestTriageChecksUnselectedTaskOwnership(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"s","synotoken":"t"}}`))
			return
		}
		b := map[string]string{
			"SYNO.ActiveBackup.Task/list": `{"tasks":[{"task_id":1,"task_name":"A","devices":[]},
 {"task_id":2,"task_name":"B","devices":[],"last_result":{"result_id":9,"task_id":1,"status":4,"job_action":1,"time_start":300,"time_end":400}}]}`,
			"SYNO.ActiveBackup.Log/list_result": `{"results":[]}`,
		}[r.URL.Query().Get("api")+"/"+r.URL.Query().Get("method")]
		_, _ = w.Write([]byte(`{"success":true,"data":` + b + `}`))
	}))
	defer srv.Close()
	c := &dsm.Client{HTTP: srv.Client(), BaseURL: srv.URL, Creds: dsm.Credentials{Account: "a", Password: "p"}, RatePerSec: 1000}
	if rep, err := triage(context.Background(), c, fleet.NAS{Name: "acme"}, "1", 20, 20); err == nil {
		t.Fatalf("task 2's last_result naming task 1 was accepted: %+v", rep)
	}
}

func TestThresholdFlagsRejectNonFinite(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SYNOLOGY_BASE_URL", "https://127.0.0.1:9")
	for _, args := range [][]string{
		{"fleet", "health", "--full-pct", "NaN"}, {"fleet", "status", "--full-pct", "101"},
		{"fleet", "health", "--disk-temp", "-1"}, {"abb", "trend", "--overrun", "NaN"}, {"abb", "trend", "--overrun", "Inf"},
	} {
		if _, code := runCLI(t, args...); code != 2 {
			t.Errorf("%v exit %d, want 2", args, code)
		}
	}
}
