// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token) wrote the base URL as configured,
// so a credential it carries (a userinfo password, a credential-named query
// value, a held value under any query name) landed in config.toml, and an
// environment credential (the integration code, the Secret, the MCP HTTP
// token) must never be persisted at all. The API user name
// (AUTOTASK_PSA_USER_NAME, AUTOTASK_USERNAME) is an identifier, not a held
// credential, so it is not a case.
// Nothing but the 0600 credential store may hold a credential: every other
// file the program writes is scanned. Each environment credential is a case.

import (
	"io/fs"
	"net/url"
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
		envCode     = "EnvAutotaskCode-4b8e2d19"
		envUser     = "env-autotask-user-7c1f3a92@psa-api.example"
		envSecret   = "EnvAutotaskSecret-2d6b9e01"
		httpToken   = "EnvMcpHttpToken-0c9d1e7a6f0a3b2e5c8a4d7b"
		storedToken = "StandaloneToken-123abc"
	)
	all := []string{urlPw, urlQuery, envCode, envSecret, httpToken, storedToken}
	for _, c := range []struct {
		name  string
		creds []string
		held  string // an environment credential the base URL repeats under a query name that is not credential-shaped
	}{
		{"AUTOTASK_API_INTEGRATION_CODE", []string{"AUTOTASK_API_INTEGRATION_CODE=" + envCode}, envCode},
		{"AUTOTASK_PSA_SECRET", []string{"AUTOTASK_PSA_SECRET=" + envSecret}, envSecret},
		{"all three env names", []string{"AUTOTASK_API_INTEGRATION_CODE=" + envCode, "AUTOTASK_PSA_USER_NAME=" + envUser,
			"AUTOTASK_PSA_SECRET=" + envSecret}, envSecret},
		{"MCP HTTP token", []string{"AUTOTASK_MCP_HTTP_TOKEN=" + httpToken}, httpToken},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := cleanEnv(home, append(c.creds,
				"AUTOTASK_BASE_URL=https://ro:"+urlPw+"@webservices5.autotask.net/atservicesrest/V1.0?api_key="+urlQuery+
					"&tenant="+url.QueryEscape(c.held),
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
				for _, s := range all {
					for _, f := range []string{s, url.QueryEscape(s)} {
						if strings.Contains(string(data), f) {
							t.Errorf("%s holds the credential %q:\n%s", rel, f, data)
						}
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
