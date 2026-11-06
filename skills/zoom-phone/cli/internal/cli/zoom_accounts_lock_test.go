// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/config"
)

// acctMintServer is a token endpoint that holds every token request until the
// test releases it, so a profile replacement can land deterministically while
// a mint is in flight.
type acctMintServer struct {
	srv     *httptest.Server
	minting chan string   // account_id of each token request, sent on arrival
	proceed chan struct{} // closed (or sent on) to let the response go
}

func newAcctMintServer(t *testing.T) *acctMintServer {
	t.Helper()
	m := &acctMintServer{minting: make(chan string, 4), proceed: make(chan struct{})}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth/token"):
			_ = r.ParseForm()
			acct := r.PostForm.Get("account_id")
			m.minting <- acct
			<-m.proceed
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"access_token":"tok-%s","expires_in":3600}`, acct)
		default:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"users":[],"next_page_token":""}`)
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// acctStartMint loads the profile like a command would and starts an API call
// whose automatic token mint blocks in the token endpoint.
func acctStartMint(t *testing.T, m *acctMintServer, profile string) (*config.Config, chan error) {
	t.Helper()
	cfg, err := config.Load(profile)
	if err != nil {
		t.Fatal(err)
	}
	cfg.BaseURL = m.srv.URL + "/v2"
	cfg.TokenURL = m.srv.URL + "/oauth/token"
	c := client.New(cfg, 10*time.Second, 0)
	done := make(chan error, 1)
	go func() {
		_, err := c.Get(context.Background(), "/phone/users", nil)
		done <- err
	}()
	select {
	case acct := <-m.minting:
		if acct != "ACCT_A" {
			t.Fatalf("mint started for %q, want ACCT_A", acct)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mint never reached the token endpoint")
	}
	return cfg, done
}

func acctSelect(t *testing.T, profile string) {
	t.Helper()
	t.Setenv("ZOOM_PHONE_HOME", filepath.Dir(profile))
	t.Setenv("ZOOM_PHONE_CONFIG", profile)
}

// Codex round 8: a mint in flight when `accounts connect --force` replaces the
// profile must not write tenant A back over tenant B.
func TestAcctMintRaceWithConnectForceKeepsReplacement(t *testing.T) {
	acctTestHome(t)
	profile := acctMustAdd(t, "acme", "ACCT_A", "CID_A", "secret-A")
	acctSelect(t, profile)
	m := newAcctMintServer(t)
	_, done := acctStartMint(t, m, profile)

	if _, errOut, code := acctRun(t, "secret-B\n", "accounts", "connect", "acme", "--account-id", "ACCT_B", "--client-id", "CID_B", "--client-secret-stdin", "--force"); code != 0 {
		t.Fatalf("connect --force: exit=%d stderr=%s", code, errOut)
	}
	close(m.proceed) // tenant A's token response arrives now

	mintErr := <-done
	cfg, err := config.Load(profile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "CID_B" || cfg.ClientSecret != "secret-B" || cfg.ZoomPhoneAccountId != "ACCT_B" || cfg.AccessToken != "" {
		t.Fatalf("profile reverted: client %q secret %q account %q token %q", cfg.ClientID, cfg.ClientSecret, cfg.ZoomPhoneAccountId, cfg.AccessToken)
	}
	if mintErr == nil || !strings.Contains(mintErr.Error(), "saved profile changed") {
		t.Fatalf("stale mint save: err=%v, want the profile-changed refusal", mintErr)
	}
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(profile), "data", "credentials.toml")); err == nil && strings.Contains(string(data), "tok-ACCT_A") {
		t.Fatalf("tenant A's token was cached for the replaced profile:\n%s", data)
	}
}

// The same race with `accounts remove`: the save must not recreate the profile.
func TestAcctMintRaceWithRemoveDoesNotRecreate(t *testing.T) {
	acctTestHome(t)
	profile := acctMustAdd(t, "acme", "ACCT_A", "CID_A", "secret-A")
	m := newAcctMintServer(t)
	_, done := acctStartMint(t, m, profile)
	if _, errOut, code := acctRun(t, "", "accounts", "remove", "acme"); code != 0 {
		t.Fatalf("remove: exit=%d stderr=%s", code, errOut)
	}
	close(m.proceed)
	if err := <-done; err == nil || !strings.Contains(err.Error(), "saved profile changed") {
		t.Fatalf("mint after remove: err=%v", err)
	}
	if _, err := os.Lstat(filepath.Dir(profile)); !os.IsNotExist(err) {
		t.Fatalf("removed profile was recreated: %v", err)
	}
}

// Without a replacement the mint saves as before.
func TestAcctMintSavesWhenProfileUnchanged(t *testing.T) {
	acctTestHome(t)
	profile := acctMustAdd(t, "acme", "ACCT_A", "CID_A", "secret-A")
	acctSelect(t, profile)
	m := newAcctMintServer(t)
	_, done := acctStartMint(t, m, profile)
	close(m.proceed)
	if err := <-done; err != nil {
		t.Fatalf("normal mint: %v", err)
	}
	cfg, err := config.Load(profile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "CID_A" || cfg.ZoomPhoneAccountId != "ACCT_A" || cfg.AccessToken != "tok-ACCT_A" {
		t.Fatalf("normal mint not saved: client %q account %q token %q", cfg.ClientID, cfg.ZoomPhoneAccountId, cfg.AccessToken)
	}
}

// `accounts connect --force`, `accounts remove` and `auth login` wait for a
// holder of the same per-profile lock.
func TestAcctProfileWritersTakeTheLock(t *testing.T) {
	acctTestHome(t)
	profile := acctMustAdd(t, "acme", "ACCT_A", "CID_A", "secret-A")
	m := newAcctMintServer(t)
	close(m.proceed) // token requests answer immediately here
	// token_url / base_url in the profile so `auth login` mints against the mock.
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	extra := fmt.Sprintf("base_url = '%s/v2'\ntoken_url = '%s/oauth/token'\n", m.srv.URL, m.srv.URL)
	if err := os.WriteFile(profile, append(data, []byte(extra)...), 0o600); err != nil {
		t.Fatal(err)
	}
	acctSelect(t, profile)

	runs := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"auth login", "", []string{"auth", "login", "--client-id", "CID_A", "--client-secret", "secret-A"}},
		{"connect --force", "secret-B\n", []string{"accounts", "connect", "acme", "--account-id", "ACCT_A", "--client-id", "CID_A", "--client-secret-stdin", "--force"}},
		{"set-token", "tok-SET\n", []string{"auth", "set-token"}},
		{"logout", "", []string{"auth", "logout"}},
		{"remove", "", []string{"accounts", "remove", "acme"}},
	}
	for _, r := range runs {
		release, err := config.ZoomAcquireProfileLock(profile)
		if err != nil {
			t.Fatal(err)
		}
		finished := make(chan string, 1)
		go func() {
			_, errOut, code := acctRun(t, r.stdin, r.args...)
			finished <- fmt.Sprintf("exit=%d stderr=%s", code, errOut)
		}()
		select {
		case res := <-finished:
			release()
			t.Fatalf("%s finished while the profile lock was held: %s", r.name, res)
		case <-time.After(300 * time.Millisecond):
		}
		release()
		select {
		case res := <-finished:
			if !strings.HasPrefix(res, "exit=0") {
				t.Fatalf("%s after release: %s", r.name, res)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s never finished after the lock was released", r.name)
		}
	}
}

// Codex round 9: a secret-only rotation by `accounts connect --force` during an
// in-flight mint is not undone by that mint's save.
func TestAcctMintRaceWithSecretRotationKeepsNewSecret(t *testing.T) {
	acctTestHome(t)
	profile := acctMustAdd(t, "acme", "ACCT_A", "CID_A", "secret-A1")
	acctSelect(t, profile)
	m := newAcctMintServer(t)
	_, done := acctStartMint(t, m, profile)
	if _, errOut, code := acctRun(t, "secret-A2\n", "accounts", "connect", "acme", "--account-id", "ACCT_A", "--client-id", "CID_A", "--client-secret-stdin", "--force"); code != 0 {
		t.Fatalf("rotate: exit=%d stderr=%s", code, errOut)
	}
	close(m.proceed)
	mintErr := <-done
	cfg, err := config.Load(profile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientSecret != "secret-A2" || cfg.AccessToken != "" {
		t.Fatalf("rotation undone: secret rotated=%v token %q", cfg.ClientSecret == "secret-A2", cfg.AccessToken)
	}
	if mintErr == nil || !strings.Contains(mintErr.Error(), "saved profile changed") {
		t.Fatalf("mint after rotation: err=%v", mintErr)
	}
	if strings.Contains(mintErr.Error(), "secret-A") {
		t.Fatalf("error leaked a secret: %v", mintErr)
	}
}

// acctEnvOnly sets up an environment-only run (no stored profile) for tenant A.
func acctEnvOnly(t *testing.T) {
	t.Helper()
	acctTestHome(t)
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "ACCT_A")
	t.Setenv("ZOOM_PHONE_CLIENT_ID", "CID_A")
	t.Setenv("ZOOM_PHONE_CLIENT_SECRET", "secret-A")
}

// Codex round 9: an environment-only first run is refused when another process
// saved a profile (tenant B) while its mint was in flight.
func TestAcctEnvOnlyFirstMintVsConcurrentProfileCreation(t *testing.T) {
	acctEnvOnly(t)
	m := newAcctMintServer(t)
	_, done := acctStartMint(t, m, "")
	credsPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	tenantB := "client_id = 'CID_B'\nclient_secret = 'secret-B'\nphone_account_id = 'ACCT_B'\naccess_token = 'tok-B'\n"
	if err := cliutil.AtomicWritePrivateFile(credsPath, []byte(tenantB), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	close(m.proceed)
	mintErr := <-done
	data, err := os.ReadFile(credsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != tenantB {
		t.Fatalf("tenant B's saved profile was overwritten:\n%s", data)
	}
	if mintErr == nil || !strings.Contains(mintErr.Error(), "saved profile changed") {
		t.Fatalf("env-only mint after concurrent creation: err=%v", mintErr)
	}
}

// A true environment-only first save still succeeds.
func TestAcctEnvOnlyFirstMintSaves(t *testing.T) {
	acctEnvOnly(t)
	m := newAcctMintServer(t)
	_, done := acctStartMint(t, m, "")
	close(m.proceed)
	if err := <-done; err != nil {
		t.Fatalf("first env-only mint: %v", err)
	}
	credsPath, _ := cliutil.CredentialsFilePath()
	data, err := os.ReadFile(credsPath)
	if err != nil || !strings.Contains(string(data), "tok-ACCT_A") {
		t.Fatalf("first save missing: %v\n%s", err, data)
	}
}

// Codex round 10 (a): a token-only profile created by `auth set-token` while an
// environment-only mint is in flight is not overwritten by that mint.
func TestAcctEnvOnlyMintVsConcurrentSetToken(t *testing.T) {
	acctEnvOnly(t)
	m := newAcctMintServer(t)
	_, done := acctStartMint(t, m, "")
	if _, errOut, code := acctRun(t, "tok-SET\n", "auth", "set-token"); code != 0 {
		t.Fatalf("set-token: exit=%d stderr=%s", code, errOut)
	}
	credsPath, _ := cliutil.CredentialsFilePath()
	before, err := os.ReadFile(credsPath)
	if err != nil || !strings.Contains(string(before), "tok-SET") {
		t.Fatalf("set-token did not save: %v\n%s", err, before)
	}
	close(m.proceed)
	mintErr := <-done
	after, _ := os.ReadFile(credsPath)
	if string(after) != string(before) {
		t.Fatalf("token-only profile overwritten:\nbefore %s\nafter  %s", before, after)
	}
	if mintErr == nil || !strings.Contains(mintErr.Error(), "saved profile changed") {
		t.Fatalf("env-only mint after set-token: err=%v", mintErr)
	}
}

// Same-account concurrent mints (two commands, two MCP tool calls) still both
// succeed: the profile's app identity is unchanged, only the token is newer.
func TestAcctSameAccountConcurrentMintsBothSave(t *testing.T) {
	acctTestHome(t)
	profile := acctMustAdd(t, "acme", "ACCT_A", "CID_A", "secret-A")
	acctSelect(t, profile)
	m := newAcctMintServer(t)
	_, first := acctStartMint(t, m, profile)
	_, second := acctStartMint(t, m, profile)
	m.proceed <- struct{}{}
	if err := <-first; err != nil {
		t.Fatalf("first mint: %v", err)
	}
	m.proceed <- struct{}{}
	if err := <-second; err != nil {
		t.Fatalf("second same-account mint refused: %v", err)
	}
}

// Codex round 10 (b): a connect that creates the profile while a mint is in
// flight for that path cannot be overwritten, and a connect cannot run inside
// a mint's locked check-and-save window.
func TestAcctConnectCannotSlipIntoMintSave(t *testing.T) {
	acctEnvOnly(t)
	path, err := acctConfigPath("newco")
	if err != nil {
		t.Fatal(err)
	}
	m := newAcctMintServer(t)
	_, done := acctStartMint(t, m, path) // nothing exists at load
	if _, errOut, code := acctRun(t, "secret-B\n", "accounts", "connect", "newco", "--account-id", "ACCT_B", "--client-id", "CID_B", "--client-secret-stdin"); code != 0 {
		t.Fatalf("connect: exit=%d stderr=%s", code, errOut)
	}
	close(m.proceed)
	if err := <-done; err == nil || !strings.Contains(err.Error(), "saved profile changed") {
		t.Fatalf("mint after connect created the profile: err=%v", err)
	}
	info, err := acctLoadInfo("newco", time.Now())
	if err != nil || !info.HasClientID || info.AccountID != "ACCT_B" || info.TokenCached {
		t.Fatalf("connect's profile overwritten: %+v %v", info, err)
	}

	// The mint's check and save run under the lock: hold it and a connect waits.
	other, _ := acctConfigPath("other")
	mustMkdir(t, filepath.Dir(other))
	release, err := config.ZoomAcquireProfileLock(other)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan int, 1)
	go func() {
		_, _, code := acctRun(t, "secret-O\n", "accounts", "connect", "other", "--account-id", "ACCT_O", "--client-id", "CID_O", "--client-secret-stdin")
		finished <- code
	}()
	select {
	case code := <-finished:
		release()
		t.Fatalf("first-time connect finished (exit %d) inside a held lock", code)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	if code := <-finished; code != 0 {
		t.Fatalf("connect after release: exit=%d", code)
	}
}
