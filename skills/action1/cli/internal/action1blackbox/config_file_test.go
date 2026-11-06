// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package action1blackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token) wrote the base URL as configured,
// so a credential it carries (a userinfo password, a credential-named query
// value, a held value under any query name) landed in config.toml, and an
// environment credential (ACTION1_OAUTH2, or either half of the
// ACTION1_CLIENT_ID + ACTION1_CLIENT_SECRET pair) must never be persisted at
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
	t.Parallel()
	const (
		urlPw        = "cfupw-mqmqmkkwwm"
		urlQuery     = "cfuqy-qkwkqwqwqk"
		envToken     = "cfetk-kkqmwqzmzmkqkmzwzmq"
		clientID     = "cfcid-kqqwzmkwmmqmmkm"
		clientSecret = "cfsec-kwqqwqwmmqwqmz"
		storedToken  = "cfsto-qmmqmmwmzzqmzwkm"
	)
	for _, c := range []struct {
		name  string
		creds []string
		held  string // an environment credential the base URL repeats under a query name that is not credential-shaped
	}{
		{"ACTION1_OAUTH2", []string{"ACTION1_OAUTH2=" + envToken}, envToken},
		{"client pair", []string{"ACTION1_CLIENT_ID=" + clientID, "ACTION1_CLIENT_SECRET=" + clientSecret}, clientSecret},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := cleanEnv(home, append(c.creds,
				"ACTION1_BASE_URL=https://ro:"+urlPw+"@app.action1.com/api?api_key="+urlQuery+"&tenant="+c.held,
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
					for _, s := range []string{envToken, clientID, clientSecret} {
						if strings.Contains(fields, s) {
							t.Errorf("the credential store %s keeps the environment credential %q:\n%s", rel, s, data)
						}
					}
					return nil
				}
				scanned++
				for _, s := range []string{urlPw, urlQuery, envToken, clientID, clientSecret, storedToken} {
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
