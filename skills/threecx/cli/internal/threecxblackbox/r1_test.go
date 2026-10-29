// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package threecxblackbox

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func (e *bbEnv) with(extra ...string) *bbEnv {
	return &bbEnv{home: e.home, env: append(append([]string(nil), e.env...), extra...)}
}

// Class credential-sent-to-another-host, the token exchange: whatever token
// URL the configuration names (THREECX_TOKEN_URL, a config file, a redirect),
// 'auth login' sends the client secret only to the configured phone system's
// own origin (THREECX_FQDN), with the secret from the environment or a flag.
func TestAuthLoginNeverLeavesTheConfiguredPBX(t *testing.T) {
	t.Parallel()
	ps := echoServer(t)
	e := newEnv(t, ps, true)
	for _, c := range []struct {
		env  []string
		args []string
	}{
		{[]string{"THREECX_TOKEN_URL=" + ps.redirectTo + "/connect/token"}, []string{"auth", "login"}},
		{[]string{"THREECX_TOKEN_URL=" + ps.redirectTo + "/connect/token"}, []string{"auth", "login", "--client-id", "903", "--client-secret", bbFlagSecret}},
		{[]string{"THREECX_TOKEN_URL=https://" + ps.host + "/xapi/v1/Parameters"}, []string{"auth", "login"}},
	} {
		out := e.with(c.env...).run(t, c.args...)
		if n := ps.offOrigin.Load(); n != 0 {
			t.Fatalf("%v with %v: another host received %d request(s):\n%.1500s", c.args, c.env, n, out)
		}
		assertNoEcho(t, "auth login output", out)
	}
}

// Class preview-sends-anyway: under --dry-run nothing reaches the phone
// system (token endpoint included) and no credential is saved, for every
// kind of command: auth login, reads, writes, sync, export, a minting
// function, doctor, fleet commands and a --pbx command.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	ps := echoServer(t)
	e := newEnv(t, ps, true)
	before := ps.hits.Load()
	runs := [][]string{
		{"auth", "login", "--dry-run"},
		{"auth", "login", "--client-id", "903", "--client-secret", bbFlagSecret, "--dry-run"},
		{"trunks", "list", "--json", "--dry-run"},
		{"system-status", "get", "--dry-run"},
		{"conference-settings", "generate-api-key", "--dry-run"},
		{"users-id", "update-user", "2", "--auth-password", bbPatchPass, "--json", "--dry-run"},
		{"sync", "--resources", "trunks", "--dry-run"},
		{"export", "trunks", "-o", filepath.Join(e.home, "dry.jsonl"), "--dry-run"},
		{"doctor", "--dry-run"},
		{"fleet", "health", "--dry-run"},
		{"fleet", "exec", "/Users", "--dry-run"},
		{"--pbx", "acme", "trunks", "list", "--dry-run"},
	}
	for _, args := range runs {
		out := e.run(t, args...)
		if n := ps.hits.Load() - before; n != 0 {
			t.Errorf("%v sent %d request(s) under --dry-run:\n%.600s", args, n, out)
		}
		before = ps.hits.Load()
	}
	// A token given to 'auth set-token' under --dry-run is not saved.
	e.runIn(t, bbFlagSecret+"\n", "auth", "set-token", "--dry-run")
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "credentials") {
			return nil
		}
		if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte(bbFlagSecret)) {
			t.Errorf("a --dry-run saved the login secret in %s", path)
		}
		return nil
	})
}

// Class credential-sent-to-another-host, outbound uploads: a feedback upload
// (THREECX_FEEDBACK_ENDPOINT with --send) passes the kit like every other request:
// one carrying a held credential never leaves, and an ordinary one does.
func TestFeedbackUploadCarriesNoCredential(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var got bytes.Buffer
	posts := 0
	fb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got.Write(b)
		posts++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(fb.Close)
	ps := echoServer(t)
	e := newEnv(t, ps, true)
	send := e.with("THREECX_FEEDBACK_ENDPOINT=" + fb.URL + "/fb")
	out := send.run(t, "feedback", "--send", "noted the key "+bbSecret+" in a ticket")
	out2 := send.run(t, "feedback", "--send", "an ordinary note about trunks")
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 || !strings.Contains(got.String(), `"cli":"threecx-cli"`) {
		t.Fatalf("want exactly the ordinary upload, got %d: %q\n%.600s\n%.600s", posts, got.String(), out, out2)
	}
	assertNoEcho(t, "feedback upload body", got.String())
	assertNoEcho(t, "feedback output", out)
}
