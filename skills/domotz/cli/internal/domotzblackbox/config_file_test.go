// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package domotzblackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token) wrote the base URL as configured,
// so a credential it carries (a userinfo password, a credential-named query
// value, a held value under any query name) landed in config.toml, and an
// environment credential (DOMOTZ_API_KEY, the MCP HTTP token) must
// never be persisted at all. Nothing but the 0600 credential store may hold a
// credential: every other file the program writes is scanned. Domotz
// reads its API key from one environment name (DOMOTZ_API_KEY);
// it and the MCP HTTP token are each a case, and both together a third.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFileNeverHoldsACredential(t *testing.T) {
	t.Parallel()
	const (
		urlPw       = "UrlPassw0rd-73c1"
		urlQuery    = "UrlSecret739182x"
		envHeld     = "EnvDomotzApiKey-4b8e2d19"
		httpToken   = "EnvMcpHttpToken-0c9d1e7a6f0a3b2e5c8a4d7b"
		storedToken = "StandaloneToken-123abc"
	)
	for _, c := range []struct {
		name  string
		creds []string
		held  string // an environment credential the base URL repeats under a query name that is not credential-shaped
	}{
		{"DOMOTZ_API_KEY", []string{"DOMOTZ_API_KEY=" + envHeld}, envHeld},
		{"MCP HTTP token", []string{"DOMOTZ_MCP_HTTP_TOKEN=" + httpToken}, httpToken},
		{"both", []string{"DOMOTZ_API_KEY=" + envHeld, "DOMOTZ_MCP_HTTP_TOKEN=" + httpToken}, httpToken},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := cleanEnv(home, append(c.creds,
				"DOMOTZ_BASE_URL=https://ro:"+urlPw+"@api-us-east-1-cell-1.domotz.com/public-api/v1?api_key="+urlQuery+"&tenant="+c.held,
			)...)
			runWithIn(t, env, storedToken+"\n", "auth", "set-token")
			var scanned, sawStore int
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
					return nil
				}
				scanned++
				for _, s := range []string{urlPw, urlQuery, envHeld, httpToken, storedToken} {
					if strings.Contains(string(data), s) {
						t.Errorf("%s holds the credential %q:\n%s", rel, s, data)
					}
				}
				return nil
			})
			if sawStore == 0 {
				t.Fatalf("auth set-token wrote no credential store; the case did not run")
			}
			if scanned == 0 {
				t.Fatalf("no config file was written; the case did not run")
			}
		})
	}
}
