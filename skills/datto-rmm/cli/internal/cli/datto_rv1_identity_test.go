// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"datto-rmm-pp-cli/internal/cliutil/testenv"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"datto-rmm-pp-cli/internal/cliutil"
	"datto-rmm-pp-cli/internal/config"
)

// Codex review r1, class "credential identity mismatch" (datto-rmm handfix
// token-identity-follows-the-configured-credential): a stored credential is
// sent only when nothing more explicit names another identity. An environment
// DATTO_RMM_TOKEN is the bearer whatever the store holds; a configured API key
// (with or without its secret) allows only a token minted for that key on this
// host. Each case stores account A's credential and configures account B.
func TestStoredCredentialNeverStandsInForTheConfiguredOne(t *testing.T) {
	const (
		tokA     = "storedTokenAccountA-6c1e9a"
		tokB     = "envTokenAccountB-3f7d2b"
		mintedB  = "mintedTokenAccountB-9a4e1c"
		boundKB  = "boundTokenKeyB-2e8f5a"
		keyA     = "identityKeyA-4b7d"
		keyB     = "identityKeyB-8e2a"
		secretB  = "identitySecretB-1c5f9d"
		noSecret = ""
	)
	type tc struct {
		name               string
		stored             func(base string) string
		key, secret, token string
		want               string // the only Authorization the API may see
	}
	cases := []tc{
		{name: "key B without its secret, token minted for key A",
			stored: func(base string) string {
				return "base_url = \"" + base + "\"\nclient_id = \"" + config.DattoTokenBinding(base, keyA) + "\"\naccess_token = \"" + tokA + "\"\n"
			},
			key: keyB, secret: noSecret, want: ""},
		{name: "DATTO_RMM_TOKEN B, token minted for key A",
			stored: func(base string) string {
				return "base_url = \"" + base + "\"\nclient_id = \"" + config.DattoTokenBinding(base, keyA) + "\"\naccess_token = \"" + tokA + "\"\n"
			},
			token: tokB, want: "Bearer " + tokB},
		{name: "DATTO_RMM_TOKEN B, stored auth_header A",
			stored: func(base string) string {
				return "base_url = \"" + base + "\"\nauth_header = \"Bearer " + tokA + "\"\n"
			},
			token: tokB, want: "Bearer " + tokB},
		{name: "DATTO_RMM_TOKEN B beside key B and a token bound to key B",
			stored: func(base string) string {
				return "base_url = \"" + base + "\"\nclient_id = \"" + config.DattoTokenBinding(base, keyB+"-tok") + "\"\naccess_token = \"" + boundKB + "\"\n"
			},
			key: keyB + "-tok", secret: secretB, token: tokB, want: "Bearer " + tokB},
		{name: "key B and secret, a token bound to key B, stored auth_header A",
			stored: func(base string) string {
				return "base_url = \"" + base + "\"\nauth_header = \"Bearer " + tokA + "\"\nclient_id = \"" + config.DattoTokenBinding(base, keyB+"-hdr") + "\"\naccess_token = \"" + boundKB + "\"\n"
			},
			key: keyB + "-hdr", secret: secretB, want: "Bearer " + boundKB},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var seen []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/auth/oauth/token" {
					_, _ = io.ReadAll(r.Body)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"access_token":"`+mintedB+`","token_type":"bearer","expires_in":359999}`)
					return
				}
				mu.Lock()
				seen = append(seen, r.Header.Get("Authorization"))
				mu.Unlock()
				_, _ = w.Write([]byte(`{"id":1,"uid":"acct-b","name":"Account B"}`))
			}))
			t.Cleanup(srv.Close)
			credRulesHome(t)
			base := srv.URL + "/api"
			credPath, err := cliutil.CredentialsFilePath()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(credPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(credPath, []byte(c.stored(base)), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("DATTO_RMM_BASE_URL", base)
			t.Setenv("DATTO_RMM_API_KEY", c.key)
			t.Setenv("DATTO_RMM_API_SECRET_KEY", c.secret)
			t.Setenv("DATTO_RMM_TOKEN", c.token)
			t.Setenv("DATTO_RMM_NO_CONFIG_WRITE", "1")
			flags := rootFlags{timeout: 10 * time.Second}
			cl, err := flags.newClient()
			if err != nil {
				t.Fatal(err)
			}
			_, _ = cl.Get(context.Background(), "/v2/account", nil)
			mu.Lock()
			defer mu.Unlock()
			for _, got := range seen {
				if got == "Bearer "+tokA {
					t.Fatalf("the API received account A's stored credential while account B is configured")
				}
				if c.want != "" && got != c.want {
					t.Fatalf("the API received Authorization of another identity (want the configured one)")
				}
				if c.want == "" && got != "" {
					t.Fatalf("the API received a credential although the configured key has no token of its own")
				}
			}
		})
	}
}

func credRulesHome(t *testing.T) {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("DATTO_RMM_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.Unsetenv("PRINTING_PRESS_VERIFY")
	for _, k := range []string{"DATTO_RMM_API_KEY", "DATTO_RMM_API_SECRET_KEY", "DATTO_RMM_TOKEN", "DATTO_RMM_BASE_URL",
		"DATTO_RMM_PLATFORM", "DATTO_RMM_API_URL", "DATTO_RMM_CONFIG", "DATTO_RMM_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
	}
}
