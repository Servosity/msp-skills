// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package pax8blackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token, auth login, a minted token) wrote
// the base URL and token URL as configured, so a credential they carry (a
// userinfo password, a query value) landed in config.toml. config.toml is
// written outside the kit (a credential source, a declared exemption), so its
// URLs are kept without userinfo and query (internal/config/pax8_persist.go).
// Every other file the program writes is scanned.

import (
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
		envSecret   = "EnvClientSecret-4b8e2d19"
		tokQuery    = "TokQuerySecret-6f0a"
		storedToken = "StandaloneBearer-123abc"
	)
	home := t.TempDir()
	env := cleanEnv(home,
		"PAX8_CLIENT_ID=EnvClientId-0c9d1e",
		"PAX8_CLIENT_SECRET="+envSecret,
		"PAX8_BASE_URL=https://ro:"+urlPw+"@api.pax8.com/v1?api_key="+urlQuery+"&tenant="+envSecret,
		"PAX8_TOKEN_URL=https://token-manager.pax8.com/oauth/token?client_secret="+tokQuery,
	)
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
		for _, s := range []string{urlPw, urlQuery, envSecret, tokQuery, storedToken} {
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
}
