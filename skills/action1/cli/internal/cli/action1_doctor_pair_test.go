// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// action1 handfix client-credentials-mint: with only ACTION1_CLIENT_ID and
// ACTION1_CLIENT_SECRET set (the primary credential), doctor reports auth
// configured, mints and probes the credential, and auth status says
// authenticated. 4.34.2 counted only a ready Authorization header, so doctor
// printed "auth: not configured" and "ERROR missing required: ACTION1_OAUTH2"
// on a working install.
func TestDoctorCountsTheClientPair(t *testing.T) {
	var mints int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			atomic.AddInt32(&mints, 1)
			_, _ = io.WriteString(w, `{"access_token":"doctor-minted-7c1d","expires_in":3600}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer doctor-minted-7c1d" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
			return
		}
		_, _ = io.WriteString(w, `{"type":"ResultPage","items":[],"total_items":0,"next_page":""}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	for k, v := range map[string]string{"ACTION1_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "ACTION1_BASE_URL": srv.URL, "ACTION1_NO_CONFIG_WRITE": "1",
		"ACTION1_CLIENT_ID": "doctor-client-id", "ACTION1_CLIENT_SECRET": "doctor-client-secret-91"} {
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
	st, err := run("auth", "status", "--json")
	if err != nil || !strings.Contains(st, `"authenticated": true`) {
		t.Errorf("auth status with the client pair: %v\n%s", err, st)
	}
}
