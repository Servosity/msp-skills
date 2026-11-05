// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package pax8blackbox

import (
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// The four classes Codex review round 2 left open on the hand-written funnel,
// each with its failing input kept exactly and driven through the built binary
// and the shared credential kit (handfixes.json credkit-wiring).
const r2Secret = "ABCDEFGHabcdefgh12345678"

// noteServer answers every read with one record whose note is the given text.
func noteServer(t *testing.T, note string) *server {
	t.Helper()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, pageOf(fmt.Sprintf(`{%s,"note":%q}`, pax8Record, note)))
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return &server{url: srv.URL + "/v1", tokenURL: srv.URL + "/oauth/token", caFile: ca}
}

// Class encoded-output: a secret partly escaped and mixed with plain text.
func TestKitEncodedOutputMixedWithPlainTextIsWithheld(t *testing.T) {
	for _, text := range []string{"=41BCDEFGHabcdefgh12345678", "ABCDEFGHabcdefgh1234567=38", `A\x42CDEFGHabcdefgh12345678`} {
		e := newEnv(t, noteServer(t, text), false).store(t)
		e.env = append(e.env, "PAX8_CLIENT_ID="+bbClientID, "PAX8_CLIENT_SECRET="+r2Secret)
		out := e.run(t, "companies", "find", "--json")
		// The stretch all three forms share in plain text: JSON output may escape
		// the rest of a form again.
		if strings.Contains(out, "CDEFGHabcdefgh1234567") {
			t.Errorf("output keeps the encoded secret %q:\n%.1000s", text, out)
		}
		if !strings.Contains(out, kitNotice("stdout")) {
			t.Errorf("%q: want the kit's stdout notice:\n%.1000s", text, out)
		}
	}
	// The same read prints an answer that carries nothing held.
	e := newEnv(t, noteServer(t, "a plain note"), false).store(t)
	e.env = append(e.env, "PAX8_CLIENT_ID="+bbClientID, "PAX8_CLIENT_SECRET="+r2Secret)
	if out := e.run(t, "companies", "find", "--json"); !strings.Contains(out, "a plain note") {
		t.Fatalf("the control read printed nothing:\n%.1000s", out)
	}
}

// Class outbound-encoded-halves: hex of each half of the secret, in two query
// values of the feedback endpoint. The server must see no request at all.
func TestKitFeedbackSendRefusesHexEncodedHalves(t *testing.T) {
	var hits atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(sink.Close)
	query := "?a=" + hex.EncodeToString([]byte(r2Secret[:12])) + "&b=" + hex.EncodeToString([]byte(r2Secret[12:]))
	home := t.TempDir()
	secret := "PAX8_CLIENT_SECRET=" + r2Secret
	out := runWith(t, cleanEnv(home, secret, "PAX8_FEEDBACK_ENDPOINT="+sink.URL+"/feedback"+query), "feedback", "--send", "a note")
	if n := hits.Load(); n != 0 {
		t.Fatalf("hex-encoded halves reached the server %d time(s):\n%.1000s", n, out)
	}
	// The endpoint without the halves receives the same send: the refusal above
	// is the halves, not a broken sender.
	runWith(t, cleanEnv(home, secret, "PAX8_FEEDBACK_ENDPOINT="+sink.URL+"/feedback"), "feedback", "--send", "a note")
	if hits.Load() != 1 {
		t.Fatalf("the control send did not arrive (%d requests)", hits.Load())
	}
}

// configFile finds the config.toml a run wrote under home.
func configFile(t *testing.T, home string) string {
	t.Helper()
	var found string
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "config.toml" {
			found = path
		}
		return nil
	})
	if found == "" {
		t.Fatal("the run wrote no config.toml; the test did not run")
	}
	return found
}

// Class config-split-query: the secret split across two base_url query values.
func TestKitConfigFileRefusesSplitQueryValues(t *testing.T) {
	home := t.TempDir()
	base := "https://api.pax8.com/v1?a=" + r2Secret[:12] + "&b=" + r2Secret[12:]
	runWithIn(t, cleanEnv(home, "PAX8_CLIENT_SECRET="+r2Secret, "PAX8_BASE_URL="+base), "StandaloneBearer123\n", "auth", "set-token")
	data, err := os.ReadFile(configFile(t, home))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), r2Secret[:12]) || strings.Contains(string(data), r2Secret[12:]) {
		t.Fatalf("config.toml holds a half of the client secret:\n%s", data)
	}
	if !strings.Contains(string(data), "https://api.pax8.com/v1") {
		t.Fatalf("config.toml lost the base URL itself:\n%s", data)
	}
}

// Class config-encoded-header: the secret in ROT13 in a config [headers] value.
func TestKitConfigFileRefusesEncodedHeaderCredential(t *testing.T) {
	const rot = "NOPQRSTUnopqrstu12345678"
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfg, []byte("base_url = \"https://api.pax8.com/v1\"\n[headers]\nX-Trace = \""+rot+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The MCP bearer may go nowhere, so its ROT13 form in a header is left out. The
	// client pair may reach the API origin (declarations.go), so that same form of
	// the pair stays in this 0600 credential source (.review/followups.md, P2).
	runWithIn(t, cleanEnv(home, "PAX8_MCP_HTTP_TOKEN="+r2Secret, "PAX8_CONFIG="+cfg), "StandaloneBearer123\n", "auth", "set-token")
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), rot) {
		t.Fatalf("config.toml keeps the ROT13 MCP bearer:\n%s", data)
	}
	if !strings.Contains(string(data), "https://api.pax8.com/v1") {
		t.Fatalf("config.toml lost the base URL itself:\n%s", data)
	}
	// A header that carries nothing held stays: the refusal above is the secret.
	if err := os.WriteFile(cfg, []byte("base_url = \"https://api.pax8.com/v1\"\n[headers]\nX-Trace = \"plain-trace-value\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runWithIn(t, cleanEnv(home, "PAX8_CLIENT_SECRET="+r2Secret, "PAX8_CONFIG="+cfg), "StandaloneBearer123\n", "auth", "set-token")
	if data, _ := os.ReadFile(cfg); !strings.Contains(string(data), "plain-trace-value") {
		t.Fatalf("config.toml dropped a header without a credential:\n%s", data)
	}
}

// Class holder-env-cookie: one value of PAX8_COOKIE, printed on its own.
func TestKitEnvironmentCookieValuesHeld(t *testing.T) {
	const x = "CookieOnlySecret739182"
	e := newEnv(t, noteServer(t, x), false).store(t)
	e.env = append(e.env, "PAX8_COOKIE=sid="+x+"; theme=dark")
	out := e.run(t, "companies", "find", "--json")
	if strings.Contains(out, x) {
		t.Fatalf("the cookie value printed:\n%.1000s", out)
	}
	if !strings.Contains(out, kitNotice("stdout")) {
		t.Fatalf("want the kit's stdout notice:\n%.1000s", out)
	}
}

// The other direction of TestClientPairNeverLeavesForAnotherTokenHost: the real
// token manager is not refused, on the automatic mint and on auth login. A local
// proxy sees where the request is going and answers 502, so nothing leaves the
// machine.
func TestClientPairIsSentToPax8TokenManager(t *testing.T) {
	for name, args := range map[string][]string{
		"automatic mint": {"companies", "find", "--json"},
		"auth login":     {"auth", "login", "--client-id", bbClientID, "--client-secret", bbToken},
	} {
		t.Run(name, func(t *testing.T) {
			var connects atomic.Int32
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodConnect && r.Host == "token-manager.pax8.com:443" {
					connects.Add(1)
				}
				w.WriteHeader(http.StatusBadGateway)
			}))
			t.Cleanup(proxy.Close)
			e := newEnv(t, quietServer(t), true)
			e.env = append(withoutEnv(e.env, "PAX8_TOKEN_URL"), "HTTPS_PROXY="+proxy.URL, "HTTP_PROXY="+proxy.URL)
			out := e.run(t, args...)
			if connects.Load() == 0 {
				t.Fatalf("no request set out for the token manager:\n%.1500s", out)
			}
			if strings.Contains(out, kitNotice("outbound request")) {
				t.Fatalf("the kit refused the token manager:\n%.1500s", out)
			}
		})
	}
}
