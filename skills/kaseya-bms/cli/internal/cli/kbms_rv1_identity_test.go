// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/config"
)

// Codex review r1, class "credential identity mismatch" (kaseya-bms handfix
// token-identity-follows-the-configured-credential): a stored credential is
// sent only when nothing more explicit names another identity. An environment
// KASEYA_BMS_TOKEN (or KASEYA_BMS_BEARER_AUTH) is the bearer whatever the
// store holds; a configured BMS login (KASEYA_BMS_USERNAME, with or without
// its password) allows only a token minted for that login on this host. Each
// case stores login A's credential and configures login B.
func TestStoredCredentialNeverStandsInForTheConfiguredOne(t *testing.T) {
	inHoldingChild(t)
	const (
		tokA    = "storedTokenLoginA-6c1e9a"
		tokB    = "envTokenLoginB-3f7d2b"
		mintedB = "mintedTokenLoginB-9a4e1c"
		boundLB = "boundTokenLoginB-2e8f5a"
	)
	// The passwords share no 8-character piece with a word: the credential kit
	// holds every piece, and a held "identity" would withhold any later write in
	// this process that names one (a migration receipt's expected_identity).
	loginA := config.KbmsLogin{Username: "identityUserA-4b7d", Tenant: "identityTenant"}
	loginB := config.KbmsLogin{Username: "identityUserB-8e2a", Password: "zq4PwB-" + "1c5f9d7e", Tenant: "identityTenant"}
	sameUserOtherTenant := config.KbmsLogin{Username: loginA.Username, Password: "zq4PwA2" + "-7d3e8b", Tenant: "identityTenantOther"}
	type tc struct {
		name   string
		stored func(base string) string
		login  config.KbmsLogin
		token  string
		bearer string
		want   string // the only Authorization the API may see
	}
	minted := func(base string, l config.KbmsLogin, tok string) string {
		return "base_url = \"" + base + "\"\nclient_id = \"" + config.KbmsTokenBinding(base, l) + "\"\naccess_token = \"" + tok + "\"\n"
	}
	cases := []tc{
		{name: "login B without its password, token minted for login A",
			stored: func(base string) string { return minted(base, loginA, tokA) },
			login:  config.KbmsLogin{Username: loginB.Username, Tenant: loginB.Tenant}, want: ""},
		{name: "login B, token minted for login A",
			stored: func(base string) string { return minted(base, loginA, tokA) },
			login:  loginB, want: "Bearer " + mintedB},
		{name: "login A's username on another tenant, token minted for login A",
			stored: func(base string) string { return minted(base, loginA, tokA) },
			login:  sameUserOtherTenant, want: "Bearer " + mintedB},
		{name: "KASEYA_BMS_TOKEN B, token minted for login A",
			stored: func(base string) string { return minted(base, loginA, tokA) },
			token:  tokB, want: "Bearer " + tokB},
		{name: "KASEYA_BMS_BEARER_AUTH B, token minted for login A",
			stored: func(base string) string { return minted(base, loginA, tokA) },
			bearer: tokB, want: "Bearer " + tokB},
		{name: "KASEYA_BMS_TOKEN B, stored auth_header A",
			stored: func(base string) string {
				return "base_url = \"" + base + "\"\nauth_header = \"Bearer " + tokA + "\"\n"
			},
			token: tokB, want: "Bearer " + tokB},
		{name: "KASEYA_BMS_TOKEN B, stored bms_token A",
			stored: func(base string) string { return "base_url = \"" + base + "\"\nbms_token = \"" + tokA + "\"\n" },
			token:  tokB, want: "Bearer " + tokB},
		{name: "KASEYA_BMS_TOKEN B beside login B and a token bound to login B",
			stored: func(base string) string { return minted(base, loginB, boundLB) },
			login:  loginB, token: tokB, want: "Bearer " + tokB},
		{name: "login B, a token bound to login B, stored auth_header A",
			stored: func(base string) string {
				return "base_url = \"" + base + "\"\nauth_header = \"Bearer " + tokA + "\"\nclient_id = \"" + config.KbmsTokenBinding(base, loginB) + "\"\naccess_token = \"" + boundLB + "\"\n"
			},
			login: loginB, want: "Bearer " + boundLB},
		{name: "login B, a token bound to login B, stored bms_token A",
			stored: func(base string) string { return minted(base, loginB, boundLB) + "bms_token = \"" + tokA + "\"\n" },
			login:  loginB, want: "Bearer " + boundLB},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			var seen []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == kbmsAuthPath {
					_, _ = io.ReadAll(r.Body)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, kbmsTokenAnswer(mintedB, mintedB+"-refresh", time.Now().Add(time.Hour)))
					return
				}
				mu.Lock()
				seen = append(seen, r.Header.Get("Authorization"))
				mu.Unlock()
				_, _ = w.Write([]byte(kbmsEnvelope(`[{"Id":1,"TicketNumber":"T-LOGIN-B","AccountName":"Account B"}]`)))
			}))
			t.Cleanup(srv.Close)
			credRulesHome(t)
			base := srv.URL
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
			t.Setenv("KASEYA_BMS_BASE_URL", base)
			setKbmsLogin(t, c.login)
			t.Setenv("KASEYA_BMS_TOKEN", c.token)
			t.Setenv("KASEYA_BMS_BEARER_AUTH", c.bearer)
			flags := rootFlags{timeout: 10 * time.Second}
			cl, err := flags.newClient()
			if err == nil {
				_, _ = cl.Get(context.Background(), "/v2/servicedesk/tickets", nil)
			}
			mu.Lock()
			defer mu.Unlock()
			if c.want != "" && len(seen) == 0 {
				t.Fatalf("no API request was sent (client err %v); the case did not run", err)
			}
			for _, got := range seen {
				if got == "Bearer "+tokA {
					t.Fatalf("the API received login A's stored credential while login B is configured")
				}
				if c.want != "" && got != c.want {
					t.Fatalf("the API received Authorization of another identity (want the configured one)")
				}
				if c.want == "" && got != "" {
					t.Fatalf("the API received a credential although the configured login has no token of its own")
				}
			}
		})
	}
}
