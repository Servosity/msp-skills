// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// client-credentials-mint: with only the client pair (NERDIO_CLIENT_ID and
// NERDIO_CLIENT_SECRET: Nerdio's only credential; it mints a token at
// NERDIO_TOKEN_URL, the operator's Entra tenant, on first use), doctor
// reports auth configured, mints and probes the credential, and auth status
// says authenticated. 4.34.2 counts only a ready Authorization header, so a
// pair-only install reads "auth: not configured" and auth status exits with
// "no credentials configured" on a working install.
func TestDoctorCountsTheClientPair(t *testing.T) {
	m := newFakeInstance(t, "doctor-client-id", "doctor-client-secret-91", "doctor-minted-7c1d", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer doctor-minted-7c1d" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	})
	dir := t.TempDir()
	for k, v := range map[string]string{"NERDIO_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "NERDIO_NO_CONFIG_WRITE": "1"} {
		t.Setenv(k, v)
	}
	m.useAs(t)
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
	if m.mints.Load() == 0 {
		t.Errorf("doctor never minted at the Entra tenant: the client pair was not probed (refused %d)\n%s", m.bad.Load(), out)
	}
	// Codex r1 P2: the credential probe counts the pair as configured too.
	if cr, _ := report["credentials"].(string); strings.Contains(cr, "no credential is configured") || !strings.HasPrefix(cr, "valid") {
		t.Errorf("doctor credentials = %q with the client pair set\n%s", cr, out)
	}
	st, err := run("auth", "status", "--json")
	if err != nil || !strings.Contains(st, `"authenticated": true`) {
		t.Errorf("auth status with the client pair: %v\n%s", err, st)
	}
}
