// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kbmsblackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token) wrote the base URL as configured,
// so a credential it carries (a userinfo password, a credential-named query
// value, a held value under any query name) landed in config.toml, and an
// environment credential (the login's KASEYA_BMS_PASSWORD, a pre-minted
// KASEYA_BMS_TOKEN) must never be persisted at all. Nothing
// but the 0600 credential store may hold a credential: every other file the
// program writes is scanned, and the store holds only the token it was given.

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
		urlPw       = "UrlPassw0rd-73c1"
		urlQuery    = "UrlSecret739182x"
		user        = "env.user@example.com"
		password    = "EnvLoginPassw0rd-6f0a3b"
		envToken    = "EnvBearerToken-4e8a2c"
		storedToken = "StandaloneToken-123abc"
	)
	for _, c := range []struct {
		name  string
		creds []string // the environment: credentials, and a base URL whose userinfo, credential-named query and other query repeat credentials
		keep  string   // a non-credential value config.toml must still keep
	}{
		{"login", []string{"KASEYA_BMS_USERNAME=" + user, "KASEYA_BMS_PASSWORD=" + password, "KASEYA_BMS_TENANT=envtenant",
			"KASEYA_BMS_BASE_URL=https://ro:" + urlPw + "@api.bms.kaseya.com/?api_key=" + urlQuery + "&tenant=" + password},
			"api.bms.kaseya.com"},
		// A regional host with a userinfo password: the config file keeps its
		// host, never its userinfo password; the pre-minted token is never
		// persisted.
		{"regional host and token", []string{"KASEYA_BMS_USERNAME=" + user, "KASEYA_BMS_PASSWORD=" + password, "KASEYA_BMS_TENANT=envtenant", "KASEYA_BMS_TOKEN=" + envToken,
			"KASEYA_BMS_BASE_URL=https://ro:" + urlPw + "@api.bmsemea.kaseya.com"},
			"api.bmsemea.kaseya.com"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := cleanEnv(home, c.creds...)
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
					for _, s := range []string{password, envToken} {
						if strings.Contains(fields, s) {
							t.Errorf("the credential store %s keeps the environment credential %q:\n%s", rel, s, data)
						}
					}
					return nil
				}
				scanned++
				kept = kept || strings.Contains(string(data), c.keep)
				for _, s := range []string{urlPw, urlQuery, password, envToken, storedToken} {
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
