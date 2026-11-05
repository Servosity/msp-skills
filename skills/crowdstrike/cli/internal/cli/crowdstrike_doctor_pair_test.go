// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// client-credentials-mint: with only FALCON_CLIENT_ID and
// FALCON_CLIENT_SECRET set (CrowdStrike's only credential: the pair mints a
// token at <cloud>/oauth2/token on first use), doctor reports auth
// configured, mints and probes the credential, and auth status says
// authenticated. 4.34.2 counts only a ready Authorization header, so a
// pair-only install reads "auth: not configured" and auth status exits with
// "no credentials configured" on a working install.
func TestDoctorCountsTheClientPair(t *testing.T) {
	var mints int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			raw, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(raw))
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || form.Get("client_id") != "doctor-client-id" || form.Get("client_secret") != "doctor-client-secret-91" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"errors":[{"code":401,"message":"access denied, invalid client"}]}`)
				return
			}
			atomic.AddInt32(&mints, 1)
			_, _ = io.WriteString(w, `{"access_token":"doctor-minted-7c1d","token_type":"bearer","expires_in":1799}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer doctor-minted-7c1d" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"errors":[{"code":401,"message":"access denied, authorization failed"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"meta":{"pagination":{"offset":0,"limit":100,"total":0}},"resources":[],"errors":[]}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	for k, v := range map[string]string{"CROWDSTRIKE_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "CROWDSTRIKE_BASE_URL": srv.URL, "CROWDSTRIKE_TOKEN_URL": "", "CROWDSTRIKE_NO_CONFIG_WRITE": "1",
		"FALCON_CLIENT_ID": "doctor-client-id", "FALCON_CLIENT_SECRET": "doctor-client-secret-91"} {
		t.Setenv(k, v)
	}
	run := func(args ...string) (string, error) {
		cmd := newRootCmd(&rootFlags{})
		var out, errb bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String() + errb.String(), err
	}
	out, _ := run("doctor", "--json")
	var report map[string]any
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &report); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	if report["auth"] != "configured" {
		t.Errorf("doctor auth = %v, want configured (client pair set)\n%s", report["auth"], out)
	}
	if ev, _ := report["env_vars"].(string); strings.HasPrefix(ev, "ERROR") {
		t.Errorf("doctor env_vars = %q with the client pair set", ev)
	}
	if atomic.LoadInt32(&mints) == 0 {
		t.Errorf("doctor never minted: the client pair was not probed\n%s", out)
	}
	// Codex r1 P2: the credential probe counts the pair as configured too.
	if cr, _ := report["credentials"].(string); strings.Contains(cr, "no credential is configured") {
		t.Errorf("doctor credentials = %q with the client pair set\n%s", cr, out)
	}
	st, err := run("auth", "status", "--json")
	if err != nil || !strings.Contains(st, `"authenticated": true`) {
		t.Errorf("auth status with the client pair: %v\n%s", err, st)
	}
}
