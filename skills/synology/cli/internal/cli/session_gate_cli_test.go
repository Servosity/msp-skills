// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-probe-before-session) end to end, and the
// type-preserving, any-length whole-field masking of secret values.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/dsm/dsmtest"
)

// stubNAS is a DSM stand-in answering the probe, any login (sid sid-1,
// SynoToken tok-1), logout and the reads in body; it records every request.
type stubNAS struct {
	*httptest.Server
	mu  sync.Mutex
	log []string
}

func newStubNAS(t *testing.T, body map[string]string) *stubNAS {
	t.Helper()
	n := &stubNAS{}
	n.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		n.mu.Lock()
		n.log = append(n.log, r.Method+" "+r.URL.String()+" cookie="+r.Header.Get("Cookie")+" body="+string(b))
		n.mu.Unlock()
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		form, _ := neturl.ParseQuery(string(b))
		q := r.URL.Query()
		api, method := firstOf(q.Get("api"), form["api"]), firstOf(q.Get("method"), form["method"])
		switch {
		case api == "SYNO.API.Auth" && method == "login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`))
		case method == "logout":
			_, _ = w.Write([]byte(`{"success":true}`))
		case body[api+"/"+method] != "":
			_, _ = w.Write([]byte(`{"success":true,"data":` + body[api+"/"+method] + `}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
		}
	}))
	t.Cleanup(n.Close)
	return n
}

func (n *stubNAS) requests() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return strings.Join(n.log, "\n")
}

func firstOf(v string, vs []string) string {
	if v != "" || len(vs) == 0 {
		return v
	}
	return vs[0]
}

// TestRawSessionFromConfigIsNotSentToANonDSMHost is the review's repro: a
// private config holding only base_url and a raw auth_header sid (no account,
// no password, no cached session) made `system dsm --no-cache` send
// _sid=<sentinel> straight to the host, even one serving a router page. The
// host now gets the unauthenticated probe only, the command fails saying the
// session was not sent, and a DSM at that address still gets the sid.
func TestRawSessionFromConfigIsNotSentToANonDSMHost(t *testing.T) {
	const sid = "SID-sentinel-9917"
	writeConfig := func(t *testing.T, base string) string {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(`{"base_url":"`+base+`","auth_header":"`+sid+`"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Run("router page", func(t *testing.T) {
		sweepIsolate(t)
		dsm.ForgetVerifiedAddresses()
		url, seen := notDSMHost(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<html><body>router login</body></html>")
		})
		cfg := writeConfig(t, url)
		for _, args := range [][]string{
			{"system", "dsm", "--no-cache", "--config", cfg, "--json"},
			{"abb", "tasks", "--no-cache", "--config", cfg, "--json", "--data-source", "live"},
		} {
			out, errText, code := runCLIErr(t, args...)
			if code == 0 || !strings.Contains(out+errText, "the session was not sent") || !strings.Contains(out+errText, "does not answer as a DSM WebAPI") {
				t.Fatalf("%v: exit %d, want a refusal naming the host as not DSM:\nout=%s\nerr=%s", args, code, out, errText)
			}
		}
		if strings.Contains(seen(), sid) {
			t.Fatalf("the raw session went to a host that is not DSM:\n%s", seen())
		}
		for _, line := range strings.Split(strings.TrimSpace(seen()), "\n") {
			if !strings.Contains(line, "SYNO.API.Info") {
				t.Fatalf("a host that is not DSM received more than the probe: %s", line)
			}
		}
	})
	t.Run("dsm", func(t *testing.T) {
		sweepIsolate(t)
		dsm.ForgetVerifiedAddresses()
		nas := newStubNAS(t, map[string]string{"SYNO.DSM.Info/getinfo": `{"model":"DS1621+","version_string":"DSM 7.2.2"}`})
		out, errText, code := runCLIErr(t, "system", "dsm", "--no-cache", "--config", writeConfig(t, nas.URL), "--json")
		if code != 0 || !strings.Contains(out, "DS1621+") {
			t.Fatalf("system dsm against a DSM: exit %d\nout=%s\nerr=%s", code, out, errText)
		}
		if !strings.Contains(nas.requests(), "_sid="+sid) || strings.Count(nas.requests(), "SYNO.API.Info") != 1 {
			t.Fatalf("want one probe, then the sid:\n%s", nas.requests())
		}
	})
}

// TestShortSecretsAndOneTimeCodesAreMaskedWherever: a password shorter than
// 4 characters (SYNOLOGY_PASSWORD=abc) that a NAS answer carries as an entire
// field stayed in the output, the response cache, the store and the export.
// Every non-empty secret is masked as an entire string value; a one-time
// code echoed as a string is masked, while the same digits as a JSON number
// (an id) stay a number.
func TestShortSecretsAndOneTimeCodesAreMaskedWherever(t *testing.T) {
	const pw, otp = "abc", "482913"
	home := sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	nas := newStubNAS(t, map[string]string{
		"SYNO.Core.User/list": `{"total":3,"offset":0,"users":[` +
			`{"name":"alice","uid":1026,"description":"abc"},` +
			`{"name":"bob","uid":482913,"description":"482913"},` +
			`{"name":"carol","uid":1028,"description":"xabcx and 482913x"}]}`,
	})
	t.Setenv("SYNOLOGY_BASE_URL", nas.URL)
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", pw)
	t.Setenv("SYNOLOGY_OTP_CODE", otp)
	whole := []string{`"abc"`, `"482913"`, `"description":"abc"`, `"description": "abc"`}
	leaks := func(where, text string) {
		t.Helper()
		for _, s := range whole {
			if strings.Contains(text, s) {
				t.Errorf("%s carries the secret as a whole field (%s)", where, s)
			}
		}
	}
	out, errOut, code := sweepExecute(t, "user", "--json", "--data-source", "live")
	if code != 0 {
		t.Fatalf("user: exit %d out=%s err=%s", code, out, errOut)
	}
	leaks("user stdout", out)
	if !strings.Contains(out, dsm.ResponseMask) || !strings.Contains(out, "482913") || !strings.Contains(out, "xabcx and 482913x") {
		t.Fatalf("user: want the whole fields masked, the uid number and the longer text kept: %s", out)
	}
	if out, errOut, code = sweepExecute(t, "sync", "--resources", "user", "--json"); code != 0 {
		t.Fatalf("sync: exit %d %s %s", code, out, errOut)
	}
	out, errOut, code = sweepExecute(t, "export", "user")
	if code != 0 || !strings.Contains(out, "alice") {
		t.Fatalf("export: exit %d out=%s err=%s", code, out, errOut)
	}
	leaks("export stdout", out)
	files := 0
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, "stdout") || strings.HasSuffix(p, "stderr") || strings.Contains(filepath.Base(p), "session") {
			return nil
		}
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "alice") {
			files++
			leaks(p, string(b))
		}
		return nil
	})
	if files == 0 {
		t.Fatal("nothing was stored or cached; the check proved nothing")
	}
}

// TestFleetSyncWithAOneTimeCodeEqualToAResultID: with <prefix>_OTP_CODE set
// (the documented first two-factor fleet login), a NAS answer whose
// result_id (a JSON number) equals the code had the number turned into the
// string "[redacted]", so the typed fleet parser failed the whole sync with
// "unexpected payload: ... no valid result_id". Masking never rewrites a
// number: the sync and abb triage succeed and the id is mirrored, while a
// string echo of the code (a task name) is still masked.
func TestFleetSyncWithAOneTimeCodeEqualToAResultID(t *testing.T) {
	const otp = "482913"
	sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	now := time.Now()
	at := func(h float64) int64 { return now.Add(-time.Duration(h * float64(time.Hour))).Unix() }
	nas := newStubNAS(t, map[string]string{
		"SYNO.API.Info/query":   `{"SYNO.ActiveBackup.Task":{"path":"entry.cgi","minVersion":1,"maxVersion":2}}`,
		"SYNO.Core.System/info": `{"model":"DS1621+","firmware_ver":"DSM 7.2.2"}`,
		"SYNO.ActiveBackup.Task/list": fmt.Sprintf(`{"tasks":[{"task_id":8,"task_name":"482913","backup_type":4,"next_trigger_time":%d,
 "last_result":{"result_id":482913,"status":4,"job_action":1,"time_start":%d,"time_end":%d},
 "devices":[{"device_id":21,"host_name":"SRV1","agent_status":"online"}]}]}`, at(-12), at(3), at(2)),
		"SYNO.ActiveBackup.Log/list_result": fmt.Sprintf(`{"results":[{"result_id":482913,"task_id":8,"task_name":"482913","status":4,"job_action":1,
 "time_start":%d,"time_end":%d,"error_count":1,"task_config":{"device_list":[{"host_name":"SRV1"}]}}]}`, at(3), at(2)),
		"SYNO.ActiveBackup.Log/list_result_detail": fmt.Sprintf(`{"result_detail_list":[{"log_level":0,"log_time":%d,"log_type":6002,"other_params":{}}]}`, at(2.5)),
		"SYNO.ActiveBackup.Log/list_log":           `{"count":0,"logs":[]}`,
		"SYNO.ActiveBackup.Overview/list_device_transfer_size": fmt.Sprintf(`{"device_list":[{"device":{"device_id":21,"host_name":"SRV1"},
 "transfer_list":[{"device_result_id":2,"result_id":482913,"status":4,"time_start":%d,"time_end":%d,"transfered_bytes":12345}]}]}`, at(3), at(2)),
		"SYNO.Storage.CGI.Storage/load_info": `{"volumes":[{"id":"volume_1","vol_path":"/volume1","status":"normal","size":{"total":"1000","used":"10"}}],"storagePools":[],"disks":[]}`,
	})
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "PW-OTP-11111111")
	t.Setenv("SYNOLOGY_ACME_OTP_CODE", otp)
	if _, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", nas.URL, "--json"); code != 0 {
		t.Fatalf("fleet add: %d %s", code, errText)
	}
	out, errText, code := runCLIErr(t, "fleet", "sync", "--nas", "acme", "--json")
	if code != 0 || strings.Contains(out, "unexpected payload") {
		t.Fatalf("fleet sync with the one-time code set: exit %d\nout=%s\nerr=%s", code, out, errText)
	}
	out, errText, code = runCLIErr(t, "abb", "failures", "--json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("abb failures: exit %d out=%s err=%s", code, out, errText)
	}
	if rows[0]["result_id"] != float64(482913) || rows[0]["task_name"] != dsm.ResponseMask {
		t.Fatalf("want result_id 482913 mirrored as a number and the task name echoing the code masked: %v", rows[0])
	}
	out, errText, code = runCLIErr(t, "abb", "triage", "8", "--nas", "acme", "--json")
	if code != 0 || strings.Contains(out+errText, "no valid result_id") {
		t.Fatalf("abb triage with the one-time code set: exit %d\nout=%s\nerr=%s", code, out, errText)
	}
}
