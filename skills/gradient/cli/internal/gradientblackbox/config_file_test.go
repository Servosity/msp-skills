// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package gradientblackbox

// Codex r1 (4.34.2 reprint) class credential-persisted-outside-the-store:
// saving the configuration (auth set-token) wrote the base URL as configured,
// so a credential it carries (a userinfo password, a credential-named query
// value, a held value under any query name) landed in config.toml, and an
// environment credential (GRADIENT_TOKEN, either key of the pair or the header
// derived from it) must never be persisted at all. Nothing but the 0600
// credential store may hold a credential: every other file the program writes
// is scanned.

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
		urlPw       = "cfupw-mqmqmkkwwm"
		urlQuery    = "cfuqy-qkwkqwqwqk"
		envToken    = "cfetk-kkqmwqzmzmkqkmzwzmq"
		vendorKey   = "cfvnd-kqqwzmkwmmqmmkm"
		partnerKey  = "cfptn-kwqqwqwmmqwqmz"
		storedToken = "cfsto-qmmqmmwmzzqmzwkm"
	)
	derived := base64.StdEncoding.EncodeToString([]byte(vendorKey + ":" + partnerKey))
	for _, c := range []struct {
		name  string
		creds []string
		held  string // an environment credential the base URL repeats under a query name that is not credential-shaped
	}{
		{"GRADIENT_TOKEN", []string{"GRADIENT_TOKEN=" + envToken}, envToken},
		{"key pair", []string{"GRADIENT_VENDOR_API_KEY=" + vendorKey, "GRADIENT_PARTNER_API_KEY=" + partnerKey}, partnerKey},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			env := cleanEnv(home, append(c.creds,
				"GRADIENT_BASE_URL=https://ro:"+urlPw+"@app.usegradient.com/api?api_key="+urlQuery+"&tenant="+c.held,
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
				for _, s := range []string{urlPw, urlQuery, envToken, vendorKey, partnerKey, derived, storedToken} {
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
