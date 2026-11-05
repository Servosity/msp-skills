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

	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
)

// countingNAS is the fake DSM behind TLS, counting connections and HTTP
// requests: a request is application data (a credential POST included).
func countingNAS(t *testing.T) (srv *httptest.Server, conns, reqs *atomic.Int32) {
	t.Helper()
	conns, reqs = new(atomic.Int32), new(atomic.Int32)
	inner := fakeABBNAS(t).Config.Handler
	srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)
		inner.ServeHTTP(w, r)
	}))
	srv.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew {
			conns.Add(1)
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, conns, reqs
}

func registeredNAS(t *testing.T, name string) (fleet.NAS, bool) {
	t.Helper()
	path, err := fleet.RegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := fleet.LoadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	return reg.Get(name)
}

// TestPinCertOnlyDisplays: --pin-cert is trust on first use, so it never
// pins: it prints the fingerprint the address presented, where to compare it
// in DSM and the --tls-fingerprint command that pins it, and saves nothing.
func TestPinCertOnlyDisplays(t *testing.T) {
	sweepIsolate(t)
	srv, _, reqs := countingNAS(t)
	live, err := dsm.FetchFingerprint(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")

	out, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", srv.URL, "--pin-cert", "--human-friendly")
	if code != 0 {
		t.Fatalf("--pin-cert: exit %d %s", code, errText)
	}
	want := "synology-cli fleet add acme --url " + srv.URL + " --tls-fingerprint " + live
	for _, s := range []string{live, "Control Panel > Security > Certificate", want, "Nothing was saved"} {
		if !strings.Contains(out, s) {
			t.Fatalf("--pin-cert output lacks %q:\n%s", s, out)
		}
	}
	if _, ok := registeredNAS(t, "acme"); ok {
		t.Fatal("--pin-cert registered the NAS")
	}

	out, errText, code = runCLIErr(t, "fleet", "add", "acme", "--url", srv.URL, "--pin-cert", "--json")
	if code != 0 {
		t.Fatalf("--pin-cert --json: exit %d %s", code, errText)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("--pin-cert --json: %v %s", err, out)
	}
	if res["presented_tls_fingerprint"] != live || res["saved"] != false || res["pin_command"] != want || res["compare_in"] != "Control Panel > Security > Certificate" {
		t.Fatalf("--pin-cert --json: %s", out)
	}
	if _, ok := registeredNAS(t, "acme"); ok {
		t.Fatal("--pin-cert --json registered the NAS")
	}
	if n := reqs.Load(); n != 0 {
		t.Fatalf("--pin-cert sent %d HTTP request(s); it may only read the certificate", n)
	}
}

// TestPinCertWithCheckIsRefused: --pin-cert --check would send the login to
// whoever holds the certificate it just read. Refused before any connection.
func TestPinCertWithCheckIsRefused(t *testing.T) {
	sweepIsolate(t)
	srv, conns, _ := countingNAS(t)
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	_, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", srv.URL, "--pin-cert", "--check")
	if code != 2 || !strings.Contains(errText, "--tls-fingerprint") {
		t.Fatalf("--pin-cert --check: exit %d %s", code, errText)
	}
	if n := conns.Load(); n != 0 {
		t.Fatalf("--pin-cert --check opened %d connection(s)", n)
	}
	if _, ok := registeredNAS(t, "acme"); ok {
		t.Fatal("--pin-cert --check registered the NAS")
	}
}

// TestPinCertOnRegisteredNASOnlyDisplays: a re-pin through --pin-cert keeps
// the registered pin; the output names the command that re-pins.
func TestPinCertOnRegisteredNASOnlyDisplays(t *testing.T) {
	sweepIsolate(t)
	srv, _, reqs := countingNAS(t)
	live, err := dsm.FetchFingerprint(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	stale := strings.Repeat("ab", 32)
	if _, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", srv.URL, "--tls-fingerprint", stale); code != 0 {
		t.Fatalf("register: %d %s", code, errText)
	}
	out, errText, code := runCLIErr(t, "fleet", "add", "acme", "--pin-cert", "--human-friendly")
	if code != 0 {
		t.Fatalf("re-pin display: exit %d %s", code, errText)
	}
	if want := "synology-cli fleet add acme --tls-fingerprint " + live; !strings.Contains(out, want) {
		t.Fatalf("re-pin display lacks %q:\n%s", want, out)
	}
	if n, _ := registeredNAS(t, "acme"); n.TLSFingerprint != stale {
		t.Fatalf("--pin-cert changed the registered pin to %q", n.TLSFingerprint)
	}
	if n := reqs.Load(); n != 0 {
		t.Fatalf("--pin-cert sent %d HTTP request(s)", n)
	}
}

// TestTLSFingerprintPinsAndChecks: the operator-checked value pins, and
// --check logs in over that pin.
func TestTLSFingerprintPinsAndChecks(t *testing.T) {
	sweepIsolate(t)
	srv, _, reqs := countingNAS(t)
	live, err := dsm.FetchFingerprint(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	out, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", srv.URL, "--tls-fingerprint", live, "--check", "--json")
	if code != 0 || !strings.Contains(out, `"checked_login": true`) {
		t.Fatalf("--tls-fingerprint --check: exit %d %s %s", code, out, errText)
	}
	if n, _ := registeredNAS(t, "acme"); n.TLSFingerprint != live {
		t.Fatalf("registered pin %q, want %q", n.TLSFingerprint, live)
	}
	if reqs.Load() == 0 {
		t.Fatal("--check sent no request")
	}
}
