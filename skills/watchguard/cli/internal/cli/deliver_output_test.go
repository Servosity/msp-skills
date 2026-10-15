// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// hand-fix (handfixes.json export-tail-honor-deliver, ported from
// connectsecure): export (without --output), tail, agent-context and version
// wrote to os.Stdout directly, so --deliver file:<path> captured nothing and
// replaced the destination with an empty file while the command exited 0.
func TestCommandsDeliverTheirOutputToFile(t *testing.T) {
	isolateAuthEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"id":"a1","serial":"FB-DELIVER-1"}],"pagination":{"totalItems":1}}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_CLIENT_ID", "rw")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "rwsec")
	t.Setenv("WATCHGUARD_API_KEY", "KEY")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", "ACC-1")
	t.Setenv("WATCHGUARD_NO_LEARN", "1")
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"export", []string{"export", "activations"}, "FB-DELIVER-1"},
		{"tail", []string{"tail", "activations", "--follow=false"}, "FB-DELIVER-1"},
		{"version", []string{"version"}, "watchguard-cli"},
		{"agent-context", []string{"agent-context"}, `"commands"`},
	} {
		out := filepath.Join(home, tc.name+".out")
		prevArgs := os.Args
		// Delivery runs in Execute (the real entrypoint), which reads os.Args.
		os.Args = append(append([]string{"watchguard-cli"}, tc.args...), "--deliver", "file:"+out)
		err := Execute()
		os.Args = prevArgs
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if b, _ := os.ReadFile(out); !bytes.Contains(b, []byte(tc.want)) {
			t.Errorf("%s --deliver file: delivered %d bytes without %q", tc.name, len(b), tc.want)
		}
	}
}

// hand-fix (handfixes.json deliver-webhook-no-redirect, ported from
// connectsecure): a webhook that answers 307/308 would get the delivered body
// replayed to whatever origin it names; any 3xx is an error instead.
func TestWebhookDeliveryRefusesRedirect(t *testing.T) {
	for _, code := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect, http.StatusFound} {
		hit := false
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL, code)
		}))
		err := deliverWebhook(srv.URL, []byte(`{"psk":"SENTINEL"}`), false)
		srv.Close()
		other.Close()
		if err == nil || hit {
			t.Fatalf("HTTP %d: webhook followed a redirect: hit=%v err=%v", code, hit, err)
		}
	}
}
