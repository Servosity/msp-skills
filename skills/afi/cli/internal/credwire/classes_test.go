// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the three
// credential classes the 4.34.2 print's review left open, closed through the
// shared credential kit and proved against the built binaries; the startup
// ordering of a malformed config; and the MCP HTTP refusal without a token.

package credwire

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// classKey is the AFI_API_KEY these runs hold (the review's own fixture):
// letters, digits and a dash, no vendor token format.
const classKey = "appkey-" + "0123456789" + "abcdefghijklmnopqrstuv"

// The base32 form of classKey, cut in two: each half alone is no held form,
// only the two read together are.
const (
	splitHead = "MFYHA23FPEWTAMJSGM2DKNRXHA4WCYTD"
	splitTail = "MRSWMZ3INFVGW3DNNZXXA4LSON2HK5Q"
)

// inviteToken is the credential an invitation URL carries.
const inviteToken = "InviteTok" + "Qx3Lm9Rv2Wd"

// tenantBody answers GET /api/v1/tenants/T with body; every other request
// with an empty page.
func tenantBody(body string) func(http.ResponseWriter, *http.Request, seen) {
	return func(w http.ResponseWriter, r *http.Request, _ seen) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/tenants/T" {
			_, _ = w.Write([]byte(body))
			return
		}
		_, _ = w.Write([]byte(`{"items":[],"next_page_token":""}`))
	}
}

// TestOrgsCreateNeverPrintsTheInvitationURL: class credential-bearing-url.
// `orgs create` returns invitation_url; whoever follows it becomes the new
// organization's administrator. It is a declared response credential, so the
// run that receives it never prints it, on the CLI or through MCP. The request
// does reach the fake, so the answer was there to print.
func TestOrgsCreateNeverPrintsTheInvitationURL(t *testing.T) {
	invite := "https://portal.example/invite?token=" + inviteToken
	answer := func(w http.ResponseWriter, r *http.Request, _ seen) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs" {
			_, _ = fmt.Fprintf(w, `{"org":{"id":"org-new-1042","external_id":"ext-1042","kind":"partner","name":"HQ-Org"},"invitation_url":%q}`, invite)
			return
		}
		_, _ = w.Write([]byte(`{"items":[],"next_page_token":""}`))
	}
	e := newEnv(t, answer, "AFI_API_KEY="+classKey)
	runs := map[string]string{}
	runs["cli"] = e.run(t, "", "orgs", "create", "--name", "HQ-Org", "--admin-email", "admin@corp.example", "--json").out
	runs["cli --stdin"] = e.run(t, `{"name":"HQ-Org","admin_email":"admin@corp.example"}`, "orgs", "create", "--stdin", "--json").out
	_, logs := e.mcp(t, map[string]any{"name": "orgs_create", "arguments": map[string]any{"name": "HQ-Org", "admin_email": "admin@corp.example", "confirm": true}})
	runs["mcp"] = logs
	if got := e.api.all(); len(got) != 3 {
		t.Fatalf("want the three POST /orgs at the fake, got %d\n%v", len(got), runs)
	}
	for where, out := range runs {
		if strings.Contains(out, inviteToken) || strings.Contains(out, "portal.example/invite") {
			t.Errorf("%s printed the invitation URL:\n%s", where, out)
		}
		if !strings.Contains(out, "withheld") {
			t.Errorf("%s printed no notice, so the invitation was not there to withhold:\n%s", where, out)
		}
	}
}

// TestSplitKeyNeverPrintedOrStored: class secret-split-across-fields. The
// fake answers with the key's base32 form cut across two values of one record.
// That record is withheld on stdout (`tenants T`) and never reaches the local
// SQLite store (`sync --resources applications`).
func TestSplitKeyNeverPrintedOrStored(t *testing.T) {
	record := `{"id":"T","parts":["` + splitHead + `","separator","` + splitTail + `"]}`
	answer := func(w http.ResponseWriter, r *http.Request, _ seen) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/tenants/T":
			_, _ = w.Write([]byte(record))
		case "/api/v1/applications/me/installations":
			_, _ = fmt.Fprintf(w, `{"items":[%s],"next_page_token":""}`, record)
		default:
			_, _ = w.Write([]byte(`{"items":[],"next_page_token":""}`))
		}
	}
	e := newEnv(t, answer, "AFI_API_KEY="+classKey)
	r := e.run(t, "", "tenants", "T", "--json", "--data-source", "live")
	if strings.Contains(r.out, splitHead) || strings.Contains(r.out, splitTail) {
		t.Errorf("stdout carries the split key:\n%s", r.out)
	}
	if !strings.Contains(r.out, "withheld") {
		t.Errorf("tenants T printed no notice, so the record was not there to withhold:\n%s", r.out)
	}
	r = e.run(t, "", "sync", "--resources", "applications", "--full")
	if !e.reached("/api/v1/applications/me/installations") {
		t.Fatalf("sync never fetched the installations, so this test proves nothing\nprinted:\n%s", r.out)
	}
	stores := 0
	for path, body := range e.files(t) {
		if strings.Contains(path, ".db") {
			stores++
		}
		if strings.Contains(body, splitHead) || strings.Contains(body, splitTail) {
			t.Errorf("%s holds the split key", path)
		}
	}
	if stores == 0 {
		t.Fatalf("sync left no SQLite store, so this test proves nothing\nprinted:\n%s", r.out)
	}
	if strings.Contains(r.out, splitHead) || strings.Contains(r.out, splitTail) {
		t.Errorf("sync printed the split key:\n%s", r.out)
	}
}

// TestShortPasswordEmbeddedNeverPrints: class short-held-value-embedded. A
// two-character password is a declared response credential; the record that
// carries it, and repeats it inside another value, is withheld whole. The
// control shows the same command prints the record without the password.
func TestShortPasswordEmbeddedNeverPrints(t *testing.T) {
	control := newEnv(t, tenantBody(`{"id":"T","echo":"xqJy","prefix":"x","suffix":"y"}`), "AFI_API_KEY="+classKey)
	if r := control.run(t, "", "tenants", "T", "--json", "--data-source", "live"); !strings.Contains(r.out, "xqJy") {
		t.Fatalf("control: tenants T never printed the record, so this test proves nothing:\n%s", r.out)
	}
	e := newEnv(t, tenantBody(`{"id":"T","password":"qJ","echo":"xqJy","prefix":"x","suffix":"y"}`), "AFI_API_KEY="+classKey)
	r := e.run(t, "", "tenants", "T", "--json", "--data-source", "live")
	if strings.Contains(r.out, "xqJy") || strings.Contains(r.out, `"qJ"`) {
		t.Errorf("the record carrying the short password printed:\n%s", r.out)
	}
	if !strings.Contains(r.out, "withheld") {
		t.Errorf("no notice:\n%s", r.out)
	}
}

// TestMalformedConfigCredentialNeverPrints: startup ordering. Cobra quotes an
// unknown command back before any config is read, and the run then fails on a
// config that does not parse; the key in that config is held from Start, so
// the echo is withheld, from the default config (two malformed shapes) and
// from an explicit --config. The control shows the echo prints a value the
// config does not carry.
func TestMalformedConfigCredentialNeverPrints(t *testing.T) {
	const cfgKey = "cfgkey-mmmm-hhhh-mmmm-hhhh"
	if r := newEnv(t, nil).run(t, "", "ctlword-pppp-qqqq"); !strings.Contains(r.out, "ctlword-pppp-qqqq") {
		t.Fatalf("control: an unknown command is not quoted back, so this test proves nothing:\n%s", r.out)
	}
	shapes := map[string]string{
		"bare value":       "base_url = \"https://papi.afi.ai\"\napi_key = " + cfgKey + "\n",
		"bad line after":   "api_key = \"" + cfgKey + "\"\nnot toml\n",
		"unterminated str": "api_key = \"" + cfgKey + "\n",
	}
	for name, body := range shapes {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, nil)
			writeFile(t, filepath.Join(e.home, "afi", "config", "config.toml"), body)
			explicit := writeFile(t, filepath.Join(e.home, "elsewhere", "afi.toml"), body)
			for _, args := range [][]string{
				{cfgKey},
				{"profile", "show", cfgKey},
				{"--config", explicit, cfgKey},
				{"doctor", "--json"},
			} {
				if args[0] == "--config" {
					// Only the explicit file carries the key on this run.
					_ = os.Remove(filepath.Join(e.home, "afi", "config", "config.toml"))
				}
				r := e.run(t, "", args...)
				if strings.Contains(r.out, cfgKey) {
					t.Errorf("%v printed the key from a malformed config:\n%s", args, r.out)
				}
				if args[0] != "doctor" && !strings.Contains(r.out, "withheld") {
					t.Errorf("%v printed no notice, so the echo was not there to withhold:\n%s", args, r.out)
				}
			}
			for _, got := range e.api.all() {
				if strings.Contains(fmt.Sprint(got.Header), cfgKey) || strings.Contains(got.Path+got.Query+got.Body, cfgKey) {
					t.Errorf("a run on a malformed config sent its key: %+v", got)
				}
			}
		})
	}
}

// TestMCPHTTPWithoutTokenRefuses: `afi-mcp --transport http` without
// AFI_MCP_HTTP_TOKEN exits non-zero and says why on stderr.
func TestMCPHTTPWithoutTokenRefuses(t *testing.T) {
	_, mcpBin := binaries(t)
	e := newEnv(t, nil, "AFI_API_KEY="+classKey)
	cmd := exec.Command(mcpBin, "--transport", "http", "--addr", "127.0.0.1:0")
	cmd.Env = e.vars
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("a token-less http launch kept running; stderr:\n%s", errb.String())
	}
	if code := cmd.ProcessState.ExitCode(); code == 0 {
		t.Fatalf("a token-less http launch exited 0; stderr:\n%s", errb.String())
	}
	if !strings.Contains(errb.String(), "AFI_MCP_HTTP_TOKEN") {
		t.Fatalf("stderr does not name AFI_MCP_HTTP_TOKEN: %q (stdout %q)", errb.String(), out.String())
	}
}

// reached reports whether the fake received a request for path.
func (e *env) reached(path string) bool {
	for _, s := range e.api.all() {
		if s.Path == path {
			return true
		}
	}
	return false
}
