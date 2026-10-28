// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/config"
)

// zoom-phone hand-fix: zoom-phone-mcp-write-confirm.

func TestZoomMCPWriteGateRefusesUnconfirmedWrites(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	gate := zoomMCPWriteGate{base: http.DefaultTransport, c: &client.Client{Config: &config.Config{TokenURL: srv.URL + "/oauth/token"}}}
	hc := &http.Client{Transport: gate}
	do := func(method, path string) int {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(`{}`))
		resp, err := hc.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusPreconditionRequired && !strings.Contains(string(body), "confirm: true") {
			t.Fatalf("refusal body = %s", body)
		}
		return resp.StatusCode
	}
	zoomMCPWriteConfirmed.Store(false)
	t.Cleanup(func() { zoomMCPWriteConfirmed.Store(false) })

	t.Setenv(cliutil.MCPChildEnvVar, "1")
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, "delete", "PROPFIND"} {
		hits.Store(0)
		if code := do(m, "/v2/phone/users/u1"); code != http.StatusPreconditionRequired || hits.Load() != 0 {
			t.Fatalf("%s from an MCP child: status %d, %d request(s) reached the server", m, code, hits.Load())
		}
	}
	// The token mint is the one POST that is not a Zoom change; only its exact URL.
	hits.Store(0)
	if code := do(http.MethodPost, "/oauth/token"); code != http.StatusNoContent || hits.Load() != 1 {
		t.Fatalf("token mint: status %d hits %d", code, hits.Load())
	}
	for _, p := range []string{"/oauth/token/x", "/v2/oauth/token", "/OAUTH/TOKEN"} {
		if code := do(http.MethodPost, p); code != http.StatusPreconditionRequired {
			t.Fatalf("POST %s treated as the token mint: %d", p, code)
		}
	}
	hits.Store(0)
	if code := do(http.MethodGet, "/v2/phone/users"); code != http.StatusNoContent || hits.Load() != 1 {
		t.Fatalf("GET from an MCP child: status %d hits %d", code, hits.Load())
	}
	zoomMCPWriteConfirmed.Store(true)
	if code := do(http.MethodDelete, "/v2/phone/users/u1"); code != http.StatusNoContent {
		t.Fatalf("confirmed DELETE: %d", code)
	}
	zoomMCPWriteConfirmed.Store(false)
	t.Setenv(cliutil.MCPChildEnvVar, "")
	if code := do(http.MethodDelete, "/v2/phone/users/u1"); code != http.StatusNoContent {
		t.Fatalf("terminal DELETE: %d", code)
	}
}

// offboard through the real command tree: an MCP call without confirm gets
// the plan and sends nothing; with confirm it applies.
func TestZoomMCPOffboardApplyNeedsConfirm(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	db := localTestStore(t, localTestFootprintRows())
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	t.Cleanup(func() { zoomMCPWriteConfirmed.Store(false) })
	run := func(extra ...string) (localOffboardResult, int) {
		t.Helper()
		_, before := ts.snapshot()
		root := newRootCmd(&rootFlags{})
		var out, errb bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errb)
		root.SetArgs(append([]string{"offboard", "jane@example.com", "--db", db, "--apply", "--json"}, extra...))
		if err := root.Execute(); err != nil {
			t.Fatalf("offboard %v: %v\nstderr: %s", extra, err, errb.String())
		}
		var res localOffboardResult
		localDecode(t, out.String(), &res)
		_, after := ts.snapshot()
		return res, len(after) - len(before)
	}
	res, sent := run()
	if sent != 0 || res.Mode != "plan" || len(res.Receipts) != 0 || res.Summary.Planned == 0 || !strings.Contains(res.Note, "confirm: true") {
		t.Fatalf("unconfirmed MCP offboard: sent=%d mode=%s receipts=%d planned=%d note=%q", sent, res.Mode, len(res.Receipts), res.Summary.Planned, res.Note)
	}
	res, sent = run("--confirm")
	if sent != 6 || res.Mode != "apply" || res.Summary.Removed != 6 {
		t.Fatalf("confirmed MCP offboard: sent=%d mode=%s summary=%+v", sent, res.Mode, res.Summary)
	}
}

// fleet from an MCP call: a write-capable target returns the plan without
// confirm, and a read-only target is not held back.
func TestZoomMCPFleetWritesNeedConfirm(t *testing.T) {
	localTestEnv(t)
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	run := func(args ...string) string {
		t.Helper()
		root := newRootCmd(&rootFlags{})
		var out, errb bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errb)
		root.SetArgs(append([]string{"fleet", "--json"}, args...))
		err := root.Execute()
		if err != nil && !strings.Contains(err.Error(), "account") {
			t.Fatalf("fleet %v: %v\nstderr: %s", args, err, errb.String())
		}
		return out.String()
	}
	if out := run("--allow-writes", "--", "users", "delete-call-log", "u1", "cl1"); !strings.Contains(out, `"dry_run": true`) && !strings.Contains(out, `"dry_run":true`) || !strings.Contains(out, "confirm: true") {
		t.Fatalf("unconfirmed MCP fleet write: %s", out)
	}
	if out := run("--", "doctor"); strings.Contains(out, "confirm: true") {
		t.Fatalf("read-only fleet held back: %s", out)
	}
}

// Codex review r1 (class mcp-write-gate: confirmation coerced through argv).
// The MCP server turns a JSON boolean true into a bare --confirm and every
// other value (the string "1" or "true", a number, an array element) into
// --confirm=<value>. Only the bare flag may confirm, as zoom-phone_execute
// accepts only boolean true; any other form leaves the call a preview.
func TestZoomMCPConfirmAcceptsOnlyBooleanTrue(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	db := localTestStore(t, localTestFootprintRows())
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	t.Cleanup(func() { zoomMCPWriteConfirmed.Store(false) })
	for _, coerced := range []string{"--confirm=1", "--confirm=true", "--confirm=TRUE", "--confirm=t", "--confirm=T", "--confirm=yes"} {
		_, before := ts.snapshot()
		root := newRootCmd(&rootFlags{})
		var out, errb bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errb)
		root.SetArgs([]string{"offboard", "jane@example.com", "--db", db, "--apply", "--json", coerced})
		err := root.Execute()
		_, after := ts.snapshot()
		if sent := len(after) - len(before); sent != 0 {
			t.Fatalf("offboard %s from an MCP call sent %d request(s); only a JSON boolean true may confirm (err=%v)", coerced, sent, err)
		}
		if err == nil {
			var res localOffboardResult
			localDecode(t, out.String(), &res)
			if res.Mode != "plan" || len(res.Receipts) != 0 {
				t.Fatalf("offboard %s: mode=%s receipts=%d; want the plan", coerced, res.Mode, len(res.Receipts))
			}
		}
		if zoomMCPWriteConfirmed.Load() {
			t.Fatalf("offboard %s opened the transport write gate", coerced)
		}

		root = newRootCmd(&rootFlags{})
		out.Reset()
		errb.Reset()
		root.SetOut(&out)
		root.SetErr(&errb)
		root.SetArgs([]string{"fleet", "--json", "--allow-writes", coerced, "--", "users", "delete-call-log", "u1", "cl1"})
		_ = root.Execute()
		if s := out.String(); !strings.Contains(s, "confirm: true") {
			t.Fatalf("fleet --allow-writes %s from an MCP call did not return its plan: %s (stderr %s)", coerced, s, errb.String())
		}
	}
}
