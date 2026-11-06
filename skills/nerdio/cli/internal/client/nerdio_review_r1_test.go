// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"nerdio-pp-cli/internal/config"
)

// Codex r1 (nerdio reprint on 4.34.2) class cached authentication ignores
// credential override: the credential store holds client A's pair and the
// token minted for it; the environment names client B. A request for B must
// carry a token minted for B at the Entra token URL (A's token would return
// A's NMM accounts). Without the environment pair, A's stored token is reused
// and nothing mints.
func TestStoredTokenOfAnotherClientIsNeverSent(t *testing.T) {
	isolateCredentialStore(t)
	nmm, entra := newNMMFake(t), newEntraFake(t, "")
	t.Setenv("NERDIO_BASE_URL", nmm.srv.URL)
	t.Setenv("NERDIO_TOKEN_URL", entra.tokenURL)
	// The files hold client A's pair and the token minted for it, as
	// `auth login` with A's pair writes them (config.toml records the NMM
	// instance and the token URL, the 0600 credential store the pair and the
	// token).
	files, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := files.SaveTokens("client-a-4f1c", "secret-for-client-a-4f1c", "token-for-client-a-4f1c", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	header := func() string {
		t.Helper()
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		h, err := New(cfg, 5*time.Second, 0).authHeader(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if h := header(); h != "Bearer token-for-client-a-4f1c" {
		t.Fatalf("the stored client's own token: got %q, want it reused", h)
	}
	if _, requests, _, _ := entra.state(); requests != 0 {
		t.Fatalf("the stored client's own token was re-minted (%d token request(s))", requests)
	}
	t.Setenv("NERDIO_CLIENT_ID", "client-b-7e3a")
	t.Setenv("NERDIO_CLIENT_SECRET", "secret-for-client-b-7e3a")
	if h := header(); h != "Bearer minted-client-b-7e3a-1" {
		t.Fatalf("the environment names client B; the request carried %q (client A's stored token authenticates as A)", h)
	}
	if mints, _, _, refused := entra.state(); mints["client-b-7e3a"] != 1 || len(refused) != 0 {
		t.Fatalf("want one accepted mint for B, got %v (refused %v)", mints, refused)
	}
}

// isolateCredentialStore points every config, data, state, cache and
// credential path at a temp dir and clears every Nerdio credential and
// endpoint variable, so a Load, a mint or a token save never reaches the
// operator's own files or hosts. NERDIO_OAUTH_SCOPE is the test scope (a
// setting every operator has), so a test that wants a mint sets only the
// hosts.
func isolateCredentialStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for k, v := range map[string]string{"NERDIO_HOME": dir, "HOME": dir, "USERPROFILE": dir,
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_STATE_HOME": filepath.Join(dir, "state"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"),
		"NERDIO_CONFIG": "", "NERDIO_CLIENT_ID": "", "NERDIO_CLIENT_SECRET": "", "NERDIO_BASE_URL": "",
		"NERDIO_TOKEN_URL": "", "NERDIO_OAUTH_SCOPE": nerdioTestScope, "NERDIO_NO_CONFIG_WRITE": ""} {
		t.Setenv(k, v)
	}
	return dir
}
