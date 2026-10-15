// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "env-credential-wins-over-saved" entry.

package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestEnvCredentialWinsOverSavedToken: a credential in the environment that
// differs from the saved one must never send the saved credential's token,
// and must not overwrite what is saved.
func TestEnvCredentialWinsOverSavedToken(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	mints := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			id := r.PostForm.Get("client_id")
			mints[id]++
			_, _ = io.WriteString(w, `{"access_token":"tok-for-`+id+`","expires_in":3600}`)
			return
		}
		sent = append(sent, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[],"pages":{}}`)
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("SOPHOS_CENTRAL_HOME", home)
	t.Setenv("SOPHOS_CENTRAL_BASE_URL", srv.URL)
	t.Setenv("SOPHOS_CENTRAL_TOKEN_URL", srv.URL+"/token")
	run := func(id, secret string) string {
		t.Helper()
		t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", id)
		t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", secret)
		cmd := RootCmd()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"endpoints", "list", "--no-cache", "--json"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("endpoints list as %s: %v", id, err)
		}
		mu.Lock()
		defer mu.Unlock()
		return sent[len(sent)-1]
	}
	creds := func() string {
		b, _ := os.ReadFile(filepath.Join(home, "data", "credentials.toml"))
		return string(b)
	}

	if got := run("client-a", "secret-a"); got != "Bearer tok-for-client-a" {
		t.Fatalf("run A sent %q", got)
	}
	if got := run("client-b", "secret-b"); got != "Bearer tok-for-client-b" {
		t.Fatalf("run B sent %q, want B's own token", got)
	}
	if c := creds(); !strings.Contains(c, "client-a") || strings.Contains(c, "client-b") || strings.Contains(c, "secret-b") {
		t.Fatalf("run B overwrote the saved credential: %s", c)
	}
	if got := run("client-a", "secret-a"); got != "Bearer tok-for-client-a" {
		t.Fatalf("run A again sent %q", got)
	}
	if mints["client-a"] != 1 {
		t.Fatalf("client-a minted %d times, want 1 (its saved token is reused)", mints["client-a"])
	}
}
