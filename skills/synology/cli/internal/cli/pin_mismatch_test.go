// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
	"synology-pp-cli/internal/store"
)

const pinMismatchText = "does not match the pinned SHA-256 fingerprint"

// TestPinMismatchIsNamedEverywhere: a NAS that renewed its certificate (DSM's
// self-signed one, or Let's Encrypt every ~90 days) read as "the request
// failed (connection error or malformed response)" and silently broke every
// sync. fleet sync rows, live commands and doctor now name the pin mismatch
// and the re-pin remedy.
func TestPinMismatchIsNamedEverywhere(t *testing.T) {
	sweepIsolate(t)
	srv := httptest.NewTLSServer(fakeABBNAS(t).Config.Handler)
	defer srv.Close()
	stale := strings.Repeat("ab", 32)
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	if out, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", srv.URL, "--tls-fingerprint", stale, "--json"); code != 0 {
		t.Fatalf("fleet add: %d %s %s", code, out, errText)
	}
	out, _, code := runCLIErr(t, "fleet", "sync", "--json")
	if code == 0 || !strings.Contains(out, pinMismatchText) || !strings.Contains(out, "re-pin") {
		t.Fatalf("fleet sync row does not name the pin mismatch: exit %d %s", code, out)
	}

	t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "s3cret")
	t.Setenv("SYNOLOGY_TLS_FINGERPRINT", stale)
	if out, errText, code := runCLIErr(t, "system", "dsm", "--json"); code == 0 || !strings.Contains(out+errText, pinMismatchText) {
		t.Fatalf("live command does not name the pin mismatch: exit %d %s %s", code, out, errText)
	}
	out, errText, _ := runCLIErr(t, "doctor", "--json")
	if !strings.Contains(out, "certificate pin mismatch") || !strings.Contains(out, "re-pin") {
		t.Fatalf("doctor does not name the pin mismatch: %s %s", out, errText)
	}
	// The remedy pins the value the operator checked in DSM; it never says
	// --pin-cert (which trusts whatever certificate is presented right now).
	var report map[string]any
	_ = json.Unmarshal([]byte(out), &report)
	api, _ := report["api"].(string)
	if !strings.Contains(api, "--tls-fingerprint <the value you checked>") || !strings.Contains(api, "Control Panel > Security > Certificate") || strings.Contains(out, "--pin-cert") {
		t.Fatalf("doctor's pin-mismatch remedy: %s", out)
	}
}

// TestFleetAddRepinsARegisteredNAS: 'fleet add <name> --pin-cert' on a
// registered NAS re-pins it in place: the address, client, env-var prefix and
// mirrored history stay; a different --url is refused (a new address is a new
// NAS) and the registry is left as it was.
func TestFleetAddRepinsARegisteredNAS(t *testing.T) {
	sweepIsolate(t)
	srv := httptest.NewTLSServer(fakeABBNAS(t).Config.Handler)
	defer srv.Close()
	t.Setenv("ACME_NAS_ACCOUNT", "msp-admin")
	t.Setenv("ACME_NAS_PASSWORD", "s3cret")
	if _, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", srv.URL, "--client", "Acme Corp", "--env-prefix", "ACME_NAS", "--pin-cert", "--json"); code != 0 {
		t.Fatalf("fleet add --pin-cert: %d %s", code, errText)
	}
	if out, _, code := runCLIErr(t, "fleet", "sync", "--json"); code != 0 {
		t.Fatalf("first sync: %d %s", code, out)
	}
	live, err := dsm.FetchFingerprint(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	// The NAS "renews" its certificate: the registered pin goes stale.
	stale := strings.Repeat("cd", 32)
	if _, errText, code := runCLIErr(t, "fleet", "add", "acme", "--tls-fingerprint", stale, "--json"); code != 0 {
		t.Fatalf("re-pin to an explicit fingerprint: %d %s", code, errText)
	}
	if out, _, code := runCLIErr(t, "fleet", "sync", "--json"); code == 0 || !strings.Contains(out, pinMismatchText) {
		t.Fatalf("a stale pin did not fail the sync with the mismatch: %d %s", code, out)
	}

	// A re-pin that also moves the NAS is refused and changes nothing.
	if _, errText, code := runCLIErr(t, "fleet", "add", "acme", "--pin-cert", "--url", "https://elsewhere.invalid:5001"); code != 2 || !strings.Contains(errText, "re-pinning keeps that address") {
		t.Fatalf("re-pin with another --url: exit %d %s", code, errText)
	}
	if _, errText, code := runCLIErr(t, "fleet", "add", "unknown", "--pin-cert"); code != 2 || !strings.Contains(errText, "not registered") {
		t.Fatalf("--pin-cert on an unregistered name: exit %d %s", code, errText)
	}

	out, errText, code := runCLIErr(t, "fleet", "add", "acme", "--pin-cert", "--json")
	if code != 0 {
		t.Fatalf("re-pin: exit %d %s", code, errText)
	}
	var res struct {
		NAS         fleet.NAS `json:"nas"`
		Repinned    bool      `json:"repinned"`
		PreviousPin string    `json:"previous_tls_fingerprint"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("re-pin output: %v %s", err, out)
	}
	n := res.NAS
	if !res.Repinned || res.PreviousPin != stale || n.TLSFingerprint != live || n.URL != srv.URL || n.Client != "Acme Corp" || n.EnvPrefix != "ACME_NAS" {
		t.Fatalf("re-pin did not keep the registration: %s", out)
	}
	// The mirror kept the NAS's history (same address, so no purge).
	path, err := fleet.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.OpenReadOnlyContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var tasks int
	err = s.DB().QueryRow(`SELECT COUNT(*) FROM abb_tasks WHERE nas = ?`, "acme").Scan(&tasks)
	_ = s.Close()
	if err != nil || tasks != 2 {
		t.Fatalf("re-pin dropped the mirrored history: %d task rows (%v)", tasks, err)
	}
	if out, _, code := runCLIErr(t, "fleet", "sync", "--json"); code != 0 {
		t.Fatalf("sync after re-pin: %d %s", code, out)
	}
	if out, _, _ := runCLIErr(t, "fleet", "status", "--json"); !strings.Contains(out, `"tasks": 2`) || strings.Contains(out, "sync_error") {
		t.Fatalf("status after the re-pinned sync: %s", out)
	}
}

// TestPinMismatchIsNotRetried: a live call over a cached session that meets
// a certificate other than the pinned one fails at once (one connection);
// retrying a pin mismatch only delays the re-pin hint (three back-off
// rounds, seven seconds).
func TestPinMismatchIsNotRetried(t *testing.T) {
	sweepIsolate(t)
	srv := httptest.NewUnstartedServer(fakeABBNAS(t).Config.Handler)
	var conns atomic.Int32
	srv.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew {
			conns.Add(1)
		}
	}
	srv.StartTLS()
	defer srv.Close()
	live, err := dsm.FetchFingerprint(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "s3cret")
	t.Setenv("SYNOLOGY_TLS_FINGERPRINT", live)
	if _, errText, code := runCLIErr(t, "auth", "login", "--json"); code != 0 {
		t.Fatalf("auth login with the right pin: %d %s", code, errText)
	}
	t.Setenv("SYNOLOGY_TLS_FINGERPRINT", strings.Repeat("ef", 32))
	conns.Store(0)
	start := time.Now()
	_, errText, code := runCLIErr(t, "abb", "tasks", "--json")
	if code == 0 || !strings.Contains(errText, pinMismatchText) {
		t.Fatalf("live call over a stale pin: exit %d %s", code, errText)
	}
	if n := conns.Load(); n != 1 || time.Since(start) > 5*time.Second {
		t.Fatalf("the pin mismatch was retried: %d connection(s) in %s", n, time.Since(start))
	}
}
