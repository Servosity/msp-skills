// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token) wrote the base URL as
// configured, so a credential it carries (a userinfo password, a
// credential-named query value, a held value under any query name) landed in
// config.toml, and an environment credential (RUNZERO_API_KEY, the MCP HTTP
// token) must never be persisted at all. Under the shared credential kit the
// configuration file is, with the credential store, a declared 0600
// exemption (credwire.Declarations): it may hold the base URL as configured,
// so it must be 0600, and every other file the program writes is scanned.
// runZero reads its one key from RUNZERO_API_KEY; the key, the MCP HTTP
// token, and both together are a case each.

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigFileNeverHoldsACredential(t *testing.T) {
	const (
		urlPw     = "UrlPassw0rd-73c1"
		urlQuery  = "UrlSecret739182x"
		envKey    = "CTEnvRunzeroApiKey-4b8e2d19"
		httpToken = "EnvMcpHttpToken-0c9d1e7a6f0a3b2e5c8a4d7b"
		storedKey = "OTStandaloneKey-123abc" // gitleaks:allow synthetic test fixture
	)
	for _, c := range []struct {
		name  string
		creds []string
		held  string // an environment credential the base URL repeats under a query name that is not credential-shaped
	}{
		{"RUNZERO_API_KEY", []string{"RUNZERO_API_KEY=" + envKey}, envKey},
		{"RUNZERO_API_KEY with its scheme", []string{"RUNZERO_API_KEY=Bearer " + envKey}, envKey},
		{"MCP HTTP token", []string{"RUNZERO_MCP_HTTP_TOKEN=" + httpToken}, httpToken},
		{"all", []string{"RUNZERO_API_KEY=" + envKey, "RUNZERO_MCP_HTTP_TOKEN=" + httpToken}, httpToken},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := cleanEnv(home, append(c.creds,
				"RUNZERO_BASE_URL=https://ro:"+urlPw+"@console.runzero.com/api/v1.0?api_key="+urlQuery+"&tenant="+c.held,
			)...)
			runWithIn(t, env, storedKey+"\n", "auth", "set-token")
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
					if strings.Contains(string(data), storedKey) {
						sawStore++
					}
					return nil
				}
				if d.Name() == "config.toml" {
					sawConfig++
					if info, ierr := d.Info(); ierr == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
						t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
					}
					return nil
				}
				for _, s := range []string{urlPw, urlQuery, envKey, httpToken, storedKey} {
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
