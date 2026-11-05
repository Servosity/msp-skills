// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json under
// the "held-values-redacted-before-url-rewrite" entry (class "secret-echo").

package main

import (
	"bytes"
	"context"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// doctorRedactKey is the Codex round 1 API key.
const doctorRedactKey = "df_live_" + "0123456789abcdef"

// doctorURLOnlySecret is a credential that only the base URL carries (not the
// API key), so no mask of the configured API key can stand in for the URL
// redaction under test.
const doctorURLOnlySecret = "UrlOnlySecret0123456789"

// TestDoctorNeverPrintsAKeyTheBaseURLCarries drives the built CLI's doctor,
// in a sandboxed home with an unreachable base URL, the way Codex round 1 did
// (`doctor --json --deliver file:doctor.json`, plus the text report): the
// API key inside the base URL, after a space in a credential-named value, in
// the userinfo username (with and without a password), percent-encoded in
// the query, or in a URL url.Parse rejects, appears in no output, not in the
// deliver file and not in any file the run leaves. The same holds for a
// credential that only the base URL carries.
func TestDoctorNeverPrintsAKeyTheBaseURLCarries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives the built binary with a Unix sandbox home; the in-package redaction tests cover Windows")
	}
	cliBin, _ := baseURLBinaries(t)
	encoded := strings.ReplaceAll(url.QueryEscape(doctorRedactKey), "_", "%5F")
	cases := []struct{ name, base, apiKey string }{
		{"query-value-after-space", "http://127.0.0.1:1/?api_key=prefix " + doctorRedactKey, doctorRedactKey},
		{"userinfo-username-with-password", "https://" + doctorRedactKey + ":pw12345x@127.0.0.1:1", doctorRedactKey},
		{"userinfo-username-only", "https://" + doctorRedactKey + "@127.0.0.1:1", doctorRedactKey},
		{"query-percent-encoded", "http://127.0.0.1:1/?api_key=" + encoded, doctorRedactKey},
		{"malformed-url", "ht!tp://127.0.0.1:1/?api_key=prefix " + doctorRedactKey, doctorRedactKey},
		{"url-only-query-value-after-quote", "http://127.0.0.1:1/?api_key=prefix'" + doctorURLOnlySecret, doctorRedactKey},
		{"url-only-userinfo-username", "https://" + doctorURLOnlySecret + ":pw12345x@127.0.0.1:1", doctorRedactKey},
		{"url-only-malformed", "ht!tp://127.0.0.1:1/?token=a\"b" + doctorURLOnlySecret, doctorRedactKey},
	}
	forms := []string{doctorRedactKey, encoded, "live_0123456789abcdef", doctorURLOnlySecret, "Secret0123456789"}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := baseURLEnv(home, "DNSFILTER_API_KEY="+c.apiKey, "DNSFILTER_BASE_URL="+c.base)
			runs := [][]string{
				{"doctor", "--json", "--deliver", "file:doctor.json"},
				{"doctor"},
			}
			for _, args := range runs {
				out := runDoctorInHome(t, cliBin, home, env, args...)
				for _, f := range forms {
					if strings.Contains(out, f) {
						t.Errorf("dnsfilter-cli %s printed %q:\n%s", strings.Join(args, " "), f, out)
					}
				}
			}
			// The credential kit may withhold the whole report (a held value in
			// any form), so no file is a pass; a written one must still read
			// as a report, or the walk below proves nothing.
			if delivered, err := os.ReadFile(filepath.Join(home, "doctor.json")); err == nil && !bytes.Contains(delivered, []byte(`"base_url"`)) {
				t.Fatalf("the deliver file has no base_url line, so it proves nothing:\n%s", delivered)
			}
			_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				data, rerr := os.ReadFile(path)
				if rerr != nil {
					return nil
				}
				for _, f := range forms {
					if bytes.Contains(data, []byte(f)) {
						t.Errorf("%s holds %q:\n%s", path, f, data)
					}
				}
				return nil
			})
		})
	}
}

// runDoctorInHome runs the CLI with home as its working directory, so a
// relative --deliver file lands in the sandbox.
func runDoctorInHome(t *testing.T, bin, home string, env []string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Dir = home
	cmd.Stdin = strings.NewReader("")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run() // doctor exits non-zero for an unreachable API; only the output is judged
	return out.String()
}
