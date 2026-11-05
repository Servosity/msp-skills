// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package appdirectblackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token) wrote the base URL as configured,
// so a credential it carries (a userinfo password, a credential-named query
// value, a held value under any query name) landed in config.toml, and an
// environment credential (either half of the
// APPDIRECT_CLIENT_ID + APPDIRECT_CLIENT_SECRET pair) must never be persisted at
// all. Nothing but the 0600 credential store may hold a credential: every
// other file the program writes is scanned, and the store holds only the token
// it was given.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestConfigFileNeverHoldsACredential(t *testing.T) {
	const (
		urlPw        = "UrlPassw0rd-73c1"
		urlQuery     = "UrlSecret739182x"
		tokenURLPw   = "TokenUrlPassw0rd-2e8b"
		tokenURLKey  = "TokenUrlSecret-5f1a9c"
		clientID     = "EnvClientId-0c9d1e7a"
		clientSecret = "EnvClientSecret-6f0a3b"
		storedToken  = "StandaloneToken-123abc"
	)
	for _, c := range []struct {
		name  string
		creds []string
		held  string // an environment credential the base URL repeats under a query name that is not credential-shaped
		keep  string // a non-credential value config.toml must still keep
	}{
		{"client pair", []string{"APPDIRECT_CLIENT_ID=" + clientID, "APPDIRECT_CLIENT_SECRET=" + clientSecret}, clientSecret, "marketplace.appdirect.com/api"},
		// The token URL is persisted to config.toml (token_url); one that
		// carries a userinfo password and a credential-named query value must
		// lose both there too.
		{"client pair and token URL", []string{"APPDIRECT_CLIENT_ID=" + clientID, "APPDIRECT_CLIENT_SECRET=" + clientSecret,
			"APPDIRECT_TOKEN_URL=https://ro:" + tokenURLPw + "@login.example.com/oauth2/token?api_key=" + tokenURLKey}, clientID, "https://login.example.com/oauth2/token"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := cleanEnv(home, append(c.creds,
				"APPDIRECT_BASE_URL=https://ro:"+urlPw+"@marketplace.appdirect.com/api?api_key="+urlQuery+"&tenant="+c.held,
			)...)
			out := runWithIn(t, env, storedToken+"\n", "auth", "set-token")
			var scanned, sawStore int
			var kept bool
			_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				data, rerr := os.ReadFile(path)
				if rerr != nil {
					return nil
				}
				rel, _ := filepath.Rel(home, path)
				if strings.HasPrefix(d.Name(), "credentials") {
					if strings.Contains(string(data), storedToken) {
						sawStore++
					}
					// No credential field of the store is an environment
					// credential. (Its base_url is the server the token was
					// issued for, kept as configured; this case's URL repeats
					// one under a query name on purpose.)
					fields := string(data) // a lock file: no field at all
					if strings.HasSuffix(d.Name(), ".toml") {
						var doc map[string]any
						if err := toml.Unmarshal(data, &doc); err != nil {
							t.Fatalf("%s: %v", rel, err)
						}
						delete(doc, "base_url")
						fields = fmt.Sprint(doc)
					}
					for _, s := range []string{clientID, clientSecret, tokenURLPw, tokenURLKey} {
						if strings.Contains(fields, s) {
							t.Errorf("the credential store %s keeps the environment credential %q:\n%s", rel, s, data)
						}
					}
					return nil
				}
				scanned++
				kept = kept || strings.Contains(string(data), c.keep)
				for _, s := range []string{urlPw, urlQuery, tokenURLPw, tokenURLKey, clientID, clientSecret, storedToken} {
					if strings.Contains(string(data), s) {
						t.Errorf("%s holds the credential %q:\n%s", rel, s, data)
					}
				}
				return nil
			})
			if sawStore == 0 {
				t.Fatalf("auth set-token wrote no credential store; the case did not run:\n%s", out)
			}
			if scanned == 0 {
				t.Fatalf("no config file was written; the case did not run")
			}
			if !kept {
				t.Errorf("no config file keeps %q: the credential was removed with the setting around it", c.keep)
			}
		})
	}
}
