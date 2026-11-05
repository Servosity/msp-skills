// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token) writes the base URL as
// configured, so a credential it carries (a userinfo password, a
// credential-named query value, a held value under any query name) lands in
// config.toml. The 0600 credential store and the config file are the
// credential kit's credential SOURCES (publish-common item 7: their writes
// are a declared exemption and credwire.Declarations.Files names them), so
// both may hold a credential and the config file must be 0600; every other
// file the program writes is scanned. Rewst reads its token from
// REWST_API_TOKEN (also as an already-prefixed "Bearer <token>"); each
// environment credential, and the MCP HTTP token, is a case, and all
// together another.

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigFileNeverHoldsACredential(t *testing.T) {
	budget(t)
	const (
		urlPw       = "UrlPassw0rd-73c1"
		urlQuery    = "UrlSecret739182x"
		envToken    = "EnvRewstApiToken-4b8e2d19"
		httpToken   = "EnvMcpHttpToken-0c9d1e7a6f0a3b2e5c8a4d7b"
		storedToken = "StandaloneToken-123abc"
	)
	for _, c := range []struct {
		name  string
		creds []string
		held  string // an environment credential the base URL repeats under a query name that is not credential-shaped
	}{
		{"REWST_API_TOKEN", []string{"REWST_API_TOKEN=" + envToken}, envToken},
		{"MCP HTTP token", []string{"REWST_MCP_HTTP_TOKEN=" + httpToken}, httpToken},
		{"both", []string{"REWST_API_TOKEN=" + envToken, "REWST_MCP_HTTP_TOKEN=" + httpToken}, httpToken},
		{"prefixed REWST_API_TOKEN", []string{"REWST_API_TOKEN=Bearer " + envToken}, envToken},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := cleanEnv(home, append(c.creds,
				"REWST_BASE_URL=https://ro:"+urlPw+"@api.rewst.io/graphql?api_key="+urlQuery+"&tenant="+c.held,
			)...)
			runWithIn(t, env, storedToken+"\n", "auth", "set-token")
			var sawStore, sawConfig int
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
				if d.Name() == "config.toml" {
					sawConfig++
					if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
						t.Errorf("%s holds credentials with mode %v, want 0600", rel, info.Mode().Perm())
					}
					return nil
				}
				for _, s := range []string{urlPw, urlQuery, envToken, httpToken, storedToken} {
					if strings.Contains(string(data), s) {
						t.Errorf("%s holds the credential %q:\n%s", rel, s, data)
					}
				}
				return nil
			})
			if sawStore == 0 {
				t.Fatalf("auth set-token wrote no credential store; the case did not run")
			}
			if sawConfig == 0 {
				t.Fatalf("no config file was written; the case did not run")
			}
		})
	}
}
