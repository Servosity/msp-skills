// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

// pax8 handfix: oauth-audience-on-token-request, token-request-body-credentials.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"pax8-pp-cli/internal/cliutil"
	"pax8-pp-cli/internal/cliutil/testenv"
	"pax8-pp-cli/internal/config"
)

// TestMintSendsPax8TokenForm: the automatic mint sends the client credentials
// and the audience in the form body and no HTTP Basic header (Pax8's token
// manager documents client_secret_post with audience=api://p8p.client).
func TestMintSendsPax8TokenForm(t *testing.T) {
	for _, tc := range []struct{ env, want string }{{"", "api://p8p.client"}, {"api://custom", "api://custom"}} {
		t.Run("audience="+tc.want, func(t *testing.T) {
			t.Setenv("PAX8_AUDIENCE", tc.env)
			var got http.Header
			form := map[string]string{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					got = r.Header.Clone()
					_ = r.ParseForm()
					for k := range r.PostForm {
						form[k] = r.PostForm.Get(k)
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"access_token":"minted-token","expires_in":3600}`)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"ok":true}`)
			}))
			defer srv.Close()
			restore, err := cliutil.SetHomeOverride("")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(restore)
			home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
			t.Setenv("LOCALAPPDATA", filepath.Join(home, "localapp"))
			t.Setenv("APPDATA", filepath.Join(home, "appdata"))
			cfg := &config.Config{BaseURL: srv.URL, TokenURL: srv.URL + "/token", ClientID: "cid-1234567", ClientSecret: "csecret-1234567", Path: filepath.Join(home, "config.toml")}
			c := New(cfg, 5*time.Second, 0)
			c.HTTPClient = srv.Client()
			c.NoCache = true
			if _, err := c.Get(context.Background(), "/companies", nil); err != nil {
				t.Fatalf("Get: %v", err)
			}
			if a := got.Get("Authorization"); a != "" {
				t.Errorf("token request carried Authorization %q; Pax8 takes the credentials in the body", a)
			}
			want := map[string]string{"grant_type": "client_credentials", "client_id": "cid-1234567", "client_secret": "csecret-1234567", "audience": tc.want}
			for k, v := range want {
				if form[k] != v {
					t.Errorf("form %s = %q, want %q", k, form[k], v)
				}
			}
		})
	}
}

func TestClientCredentialsTokenFormIsTheOneBody(t *testing.T) {
	t.Setenv("PAX8_AUDIENCE", "")
	t.Setenv("PAX8_OAUTH_SCOPE", "")
	f := ClientCredentialsTokenForm("id", "secret")
	if f.Encode() != "audience=api%3A%2F%2Fp8p.client&client_id=id&client_secret=secret&grant_type=client_credentials" {
		t.Fatalf("form = %s", f.Encode())
	}
}
