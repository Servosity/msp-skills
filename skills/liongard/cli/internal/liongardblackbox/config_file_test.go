// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package liongardblackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token) wrote the base URL as configured,
// so a credential it carries (a userinfo password, a credential-named query
// value, a held value under any query name) landed in config.toml, and an
// environment credential (LIONGARD_API_KEY, either half of the access key
// pair or the X-ROAR-API-KEY value derived from it, the session token) must
// never be persisted at all. Nothing but the 0600 credential store may hold
// a credential: every other file the program writes is scanned.

import (
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFileNeverHoldsACredential(t *testing.T) {
	const (
		urlPw       = "UrlPassw0rd-73c1"
		urlQuery    = "UrlSecret739182x"
		envKey      = "EnvLio" + "ngardKey-4b8e2d19"
		keyID       = "EnvAccessKeyId-0c9d1e7a"
		keySecret   = "EnvAccessKeySecret-6f0a3b"
		session     = "EnvSessionToken-2e5c8a"
		storedToken = "StandaloneToken-123abc"
	)
	derived := base64.StdEncoding.EncodeToString([]byte(keyID + ":" + keySecret))
	for _, c := range []struct {
		name  string
		creds []string
		held  string // an environment credential the base URL repeats under a query name that is not credential-shaped
	}{
		{"LIONGARD_API_KEY", []string{"LIONGARD_API_KEY=" + envKey}, envKey},
		{"key pair", []string{"LIONGARD_ACCESS_KEY_ID=" + keyID, "LIONGARD_ACCESS_KEY_SECRET=" + keySecret}, keySecret},
		{"session token", []string{"LIONGARD_API_KEY=" + envKey, "LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH=" + session}, session},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := cleanEnv(home, append(c.creds,
				"LIONGARD_BASE_URL=https://ro:"+urlPw+"@us1.app.liongard.com/api/v1?api_key="+urlQuery+"&tenant="+c.held,
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
				for _, s := range []string{urlPw, urlQuery, envKey, keyID, keySecret, derived, session, storedToken} {
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
