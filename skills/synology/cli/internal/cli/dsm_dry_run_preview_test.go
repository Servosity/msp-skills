// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestDSMWriteDryRunNamesTheRequest: abb backup, abb cancel and session
// logout answered --dry-run with {"would":"run abb backup; no changes
// made"}: no NAS, no DSM call, no task ids, so the person approving the
// preview saw nothing. The preview names the exact request and still sends
// nothing; the password, the cached sid and the SynoToken are never shown.
func TestDSMWriteDryRunNamesTheRequest(t *testing.T) {
	sweepIsolate(t)
	const pw, sid, tok = "PWSENTINEL-3307", "SIDSENTINEL-3307", "TOKSENTINEL-3307"
	var live int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		atomic.AddInt32(&live, 1)
		_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"` + sid + `","synotoken":"` + tok + `"}}`))
	}))
	defer srv.Close()
	t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", pw)
	// Cache a session, so a preview that leaked the sid or SynoToken would show.
	if _, errOut, code := sweepExecute(t, "auth", "login", "--json"); code != 0 {
		t.Fatalf("auth login: %d %s", code, errOut)
	}
	atomic.StoreInt32(&live, 0)

	cases := []struct {
		args      []string
		apiMethod string
		version   string
		params    map[string]string
	}{
		{[]string{"abb", "backup", "--task-ids", "[5,8]"}, "backup", "1", map[string]string{"task_ids": "[5,8]", "trigger_type": "1"}},
		{[]string{"abb", "cancel", "--task-ids", "[5]"}, "cancel", "1", map[string]string{"task_ids": "[5]"}},
		{[]string{"session", "logout"}, "logout", "6", map[string]string{}},
	}
	for _, c := range cases {
		out, errOut, code := sweepExecute(t, append(c.args, "--dry-run", "--json")...)
		if code != 0 {
			t.Fatalf("%v --dry-run: exit %d %s", c.args, code, errOut)
		}
		var got struct {
			DryRun  bool   `json:"dry_run"`
			Action  string `json:"action"`
			Would   string `json:"would"`
			Request struct {
				HTTPMethod string            `json:"http_method"`
				NAS        string            `json:"nas"`
				Path       string            `json:"path"`
				API        string            `json:"api"`
				APIMethod  string            `json:"api_method"`
				Version    string            `json:"version"`
				Params     map[string]string `json:"params"`
			} `json:"request"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("%v: preview is not JSON: %v: %s", c.args, err, out)
		}
		r := got.Request
		if !got.DryRun || got.Action != strings.Join(c.args[:2], " ") || r.HTTPMethod != "GET" || r.NAS != srv.URL ||
			r.Path != "/webapi/entry.cgi" || r.APIMethod != c.apiMethod || r.Version != c.version ||
			!strings.HasPrefix(got.Would, "GET "+srv.URL+"/webapi/entry.cgi?") || !strings.Contains(got.Would, "method="+c.apiMethod) {
			t.Fatalf("%v: the preview does not name the request: %s", c.args, out)
		}
		for k, v := range c.params {
			if r.Params[k] != v {
				t.Fatalf("%v: params[%s] = %q, want %q: %s", c.args, k, r.Params[k], v, out)
			}
		}
		human, _, _ := sweepExecute(t, append(c.args, "--dry-run")...)
		for _, leak := range []string{pw, sid, tok} {
			if strings.Contains(out+errOut+human, leak) {
				t.Fatalf("%v: the preview printed a credential or session value", c.args)
			}
		}
		if c.params["task_ids"] != "" && !strings.Contains(human, "task_ids="+c.params["task_ids"]) {
			t.Fatalf("%v: the human preview does not name the task ids: %s", c.args, human)
		}
	}
	if n := atomic.LoadInt32(&live); n != 0 {
		t.Fatalf("a dry run sent %d request(s) to the NAS", n)
	}
}
