// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestExplicitConfigLogoutNotesAndDoctorProbe (handfixes.json:
// credentials-path-follows-config): logout under an explicit config clears
// only that config's file, so it must say when the default login still (or
// from now on) applies; doctor must probe the credentials file the config
// actually read, not always the global one.
func TestExplicitConfigLogoutNotesAndDoctorProbe(t *testing.T) {
	// Earlier tests' --home leaves a process-global home override; clear it
	// so the default login below lands where this test's commands read.
	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	testenv.Isolate(t)
	respondClearHarness(t)
	for _, k := range []string{"ESET_PROTECT_TOKEN", "ESET_PROTECT_USERNAME", "ESET_PROTECT_PASSWORD", "ESET_PROTECT_CONFIG"} {
		t.Setenv(k, "")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ESET_PROTECT_BASE_URL", srv.URL)
	if err := cliutil.SaveCredentials(&cliutil.Credentials{AccessToken: "token-global"}); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		_ = cmd.Execute()
		return out.String()
	}
	newConfig := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "tenant", "config.toml")
		if err := cliutil.AtomicWritePrivateFile(p, []byte(body), 0o600, 0o700); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// No credentials of its own: it reads the default login, which this
	// logout does not clear.
	noOwn := newConfig(t, "")
	if got := run("--config", noOwn, "auth", "logout", "--json"); !strings.Contains(got, "uses the default login") {
		t.Fatalf("logout under a config using the default login must say so: %s", got)
	}

	// Its own colocated file: after logout the default login applies.
	own := newConfig(t, "")
	colocated, err := cliutil.CredentialsFilePathForConfig(own)
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentialsAt(colocated, &cliutil.Credentials{AccessToken: "token-a"}); err != nil {
		t.Fatal(err)
	}
	if got := run("--config", own, "auth", "logout", "--json"); !strings.Contains(got, "from now on it uses the default login") {
		t.Fatalf("logout that leaves the default login in force must say so: %s", got)
	}

	// Inline credentials with a different global login: doctor must not warn
	// about credentials stored in more than one location.
	inline := newConfig(t, "access_token = 'inline-a'\n")
	if got := run("--config", inline, "doctor", "--json"); strings.Contains(got, "more than one location") {
		t.Fatalf("doctor probed the global file for an inline-credentials config: %s", got)
	}
}
