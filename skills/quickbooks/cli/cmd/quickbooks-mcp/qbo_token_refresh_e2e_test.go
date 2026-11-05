// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/cliutil/testenv"
	"quickbooks-pp-cli/internal/config"
)

// Every token, id and secret below is invented for these tests.
const (
	lkClientID     = "lk-client-id-0001"
	lkClientSecret = "lk-client-secret-4XcV"
	lkOldAccess    = "lk-access-old-7QmT"
	lkNewAccess    = "lk-access-new-9WpR"
	lkRefresh1     = "lk-refresh-r1-3ZkD"
	lkRefresh2     = "lk-refresh-r2-8MvN"
	lkEnvRefresh   = "lk-refresh-env-5TqB"
)

const lkReport = `{"Header":{"ReportName":"ProfitAndLoss","ReportBasis":"Accrual","Currency":"USD"},"Rows":{"Row":[{"type":"Section","group":"NetIncome","Summary":{"ColData":[{"value":"Net Income"},{"value":"12.00"}]}}]}}`

// lkTokenServer is a fake Intuit token endpoint. answer decides the reply to
// each grant from the refresh token it carries.
type lkTokenServer struct {
	mu     sync.Mutex
	grants []url.Values
	basic  []string
	handle http.HandlerFunc
}

func newLKTokenServer(t *testing.T, answer func(refresh string) (int, string)) *lkTokenServer {
	t.Helper()
	ts := &lkTokenServer{}
	ts.handle = func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		ts.mu.Lock()
		ts.grants = append(ts.grants, r.PostForm)
		ts.basic = append(ts.basic, r.Header.Get("Authorization"))
		ts.mu.Unlock()
		status, body := answer(r.PostForm.Get("refresh_token"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
	return ts
}

// lkTokenPath serves the token endpoint on the API stub's origin: the kit lets
// a client secret and a refresh token reach Intuit or the base URL's origin,
// and no other, and refuses a credential over plain http.
const lkTokenPath = "/lk-token"

// lkServe is a TLS origin serving ts at lkTokenPath and h everywhere else.
func lkServe(t *testing.T, ts *lkTokenServer, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == lkTokenPath {
			ts.handle(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// lkCAEnv trusts srv's certificate (all httptest TLS servers share one).
func lkCAEnv(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	ca := filepath.Join(t.TempDir(), "stub-ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return "QUICKBOOKS_CA_CERT=" + ca
}

func (ts *lkTokenServer) calls() []url.Values {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]url.Values(nil), ts.grants...)
}

// lkAPI is a fake QuickBooks API. good decides whether a bearer token is
// accepted; every Authorization header is recorded.
type lkAPI struct {
	mu   sync.Mutex
	auth []string
	srv  *httptest.Server
}

func newLKAPI(t *testing.T, ts *lkTokenServer, good func(token string) bool) *lkAPI {
	t.Helper()
	api := &lkAPI{}
	api.srv = lkServe(t, ts, func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		api.mu.Lock()
		api.auth = append(api.auth, h)
		api.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !good(strings.TrimPrefix(h, "Bearer ")) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"fault":{"error":[{"message":"message=AuthenticationFailed; errorCode=003200; statusCode=401","code":"3200"}],"type":"AUTHENTICATION"}}`)
			return
		}
		if strings.Contains(r.URL.Path, "/query") {
			_, _ = io.WriteString(w, `{"QueryResponse":{"Customer":[{"Id":"1","DisplayName":"Larkspur Widget Co"}]}}`)
			return
		}
		_, _ = io.WriteString(w, lkReport)
	})
	return api
}

func (api *lkAPI) headers() []string {
	api.mu.Lock()
	defer api.mu.Unlock()
	return append([]string(nil), api.auth...)
}

func lkGrantAnswer(access, refresh string) string {
	return `{"access_token":"` + access + `","refresh_token":"` + refresh + `","token_type":"bearer","expires_in":3600,"x_refresh_token_expires_in":8726400}`
}

// lkIsolate sandboxes the home directory and blanks every QuickBooks
// credential the environment might carry, so only what a test sets counts.
func lkIsolate(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t)
	for _, name := range []string{"QUICKBOOKS_ACCESS_TOKEN", "QUICKBOOKS_CLIENT_ID", "QUICKBOOKS_CLIENT_SECRET", "QUICKBOOKS_REFRESH_TOKEN", "QUICKBOOKS_TOKEN_URL", "QUICKBOOKS_REALM_ID", "QUICKBOOKS_BASE_URL", "QUICKBOOKS_DB_PATH"} {
		t.Setenv(name, "")
	}
	return home
}

// lkSeed stores an OAuth token set in the isolated home's credential store,
// as `auth refresh` or `auth login` would have left it.
func lkSeed(t *testing.T, access, refresh string, expiry time.Time) {
	t.Helper()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens(lkClientID, lkClientSecret, access, refresh, expiry); err != nil {
		t.Fatal(err)
	}
}

func lkStored(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// lkRun runs the CLI process for real and returns stdout and stderr
// together and the exit code.
func lkRun(t *testing.T, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(append(os.Environ(), "QBO_E2E_MAIN=cli"), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.String(), exit.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.String(), 0
}

func lkEnv(t *testing.T, api *lkAPI) []string {
	t.Helper()
	return []string{
		"QUICKBOOKS_BASE_URL=" + api.srv.URL + "/v3/company/4620816365",
		"QUICKBOOKS_TOKEN_URL=" + api.srv.URL + lkTokenPath,
		"QUICKBOOKS_NO_LEARN=true",
		lkCAEnv(t, api.srv),
	}
}

var lkReportArgs = []string{"report", "pnl", "--month", "2025-01", "--json", "--no-cache"}

// An access token that expires within 60 seconds is refreshed before the
// request: the request carries the new token, and the rotated refresh token
// is saved with the new access token and expiry.
func TestQBORefreshesBeforeAnExpiringToken(t *testing.T) {
	lkIsolate(t)
	lkSeed(t, lkOldAccess, lkRefresh1, time.Now().Add(30*time.Second))
	ts := newLKTokenServer(t, func(string) (int, string) { return 200, lkGrantAnswer(lkNewAccess, lkRefresh2) })
	api := newLKAPI(t, ts, func(tok string) bool { return tok == lkNewAccess })

	out, code := lkRun(t, lkEnv(t, api), lkReportArgs...)
	if code != 0 || !strings.Contains(out, `"ProfitAndLoss"`) {
		t.Fatalf("report pnl exit %d: %s", code, out)
	}
	grants := ts.calls()
	if len(grants) != 1 || grants[0].Get("grant_type") != "refresh_token" || grants[0].Get("refresh_token") != lkRefresh1 {
		t.Fatalf("token grants = %v, want one refresh_token grant with the stored refresh token", grants)
	}
	if want := "Basic " + base64.StdEncoding.EncodeToString([]byte(lkClientID+":"+lkClientSecret)); ts.basic[0] != want {
		t.Fatalf("the grant did not authenticate the OAuth client with HTTP Basic")
	}
	if got := api.headers(); len(got) != 1 || got[0] != "Bearer "+lkNewAccess {
		t.Fatalf("API saw %d requests (first new-token=%v), want one with the new token", len(got), len(got) > 0 && got[0] == "Bearer "+lkNewAccess)
	}
	cfg := lkStored(t)
	if cfg.AccessToken != lkNewAccess || cfg.RefreshToken != lkRefresh2 {
		t.Fatalf("store holds new access=%v rotated refresh=%v, want both", cfg.AccessToken == lkNewAccess, cfg.RefreshToken == lkRefresh2)
	}
	if left := time.Until(cfg.TokenExpiry); left < 50*time.Minute || left > 61*time.Minute {
		t.Fatalf("stored expiry is %s away, want about an hour", left)
	}
	for _, leaked := range []string{lkNewAccess, lkRefresh2, lkRefresh1, lkClientSecret} {
		if qboLeaks(out, leaked) {
			t.Fatalf("output carries a credential")
		}
	}
}

// A 401 on a token the client believed fresh refreshes once and retries once.
func TestQBORefreshesOnceAfterA401(t *testing.T) {
	lkIsolate(t)
	lkSeed(t, lkOldAccess, lkRefresh1, time.Now().Add(time.Hour))
	ts := newLKTokenServer(t, func(string) (int, string) { return 200, lkGrantAnswer(lkNewAccess, lkRefresh2) })
	api := newLKAPI(t, ts, func(tok string) bool { return tok == lkNewAccess })

	out, code := lkRun(t, lkEnv(t, api), lkReportArgs...)
	if code != 0 {
		t.Fatalf("report pnl exit %d: %s", code, out)
	}
	got := api.headers()
	if len(got) != 2 || got[0] != "Bearer "+lkOldAccess || got[1] != "Bearer "+lkNewAccess {
		t.Fatalf("API saw %d requests, want the old token then the new one", len(got))
	}
	if n := len(ts.calls()); n != 1 {
		t.Fatalf("%d token grants, want 1", n)
	}
	if cfg := lkStored(t); cfg.RefreshToken != lkRefresh2 || cfg.AccessToken != lkNewAccess {
		t.Fatal("the rotated refresh token was not saved")
	}
}

// A second 401 is returned as the auth error it is; the client does not
// loop.
func TestQBOSecond401IsReturnedWithoutLooping(t *testing.T) {
	lkIsolate(t)
	lkSeed(t, lkOldAccess, lkRefresh1, time.Now().Add(time.Hour))
	ts := newLKTokenServer(t, func(string) (int, string) { return 200, lkGrantAnswer(lkNewAccess, lkRefresh2) })
	api := newLKAPI(t, ts, func(string) bool { return false })

	out, code := lkRun(t, lkEnv(t, api), lkReportArgs...)
	if code != 4 || !strings.Contains(out, "HTTP 401") {
		t.Fatalf("exit %d, want the auth exit code 4 and the 401: %s", code, out)
	}
	if n := len(api.headers()); n != 2 {
		t.Fatalf("API saw %d requests, want 2 (the request and one retry)", n)
	}
	if n := len(ts.calls()); n != 1 {
		t.Fatalf("%d token grants, want 1", n)
	}
}

// A refresh token the store holds but Intuit rejects (invalid_grant) falls
// back to QUICKBOOKS_REFRESH_TOKEN, and the token that chain rotated to is
// saved.
func TestQBORejectedStoredRefreshTokenFallsBackToTheEnvironment(t *testing.T) {
	lkIsolate(t)
	lkSeed(t, lkOldAccess, lkRefresh1, time.Now().Add(-time.Minute))
	ts := newLKTokenServer(t, func(rt string) (int, string) {
		if rt == lkEnvRefresh {
			return 200, lkGrantAnswer(lkNewAccess, lkRefresh2)
		}
		return 400, `{"error":"invalid_grant"}`
	})
	api := newLKAPI(t, ts, func(tok string) bool { return tok == lkNewAccess })

	out, code := lkRun(t, append(lkEnv(t, api), "QUICKBOOKS_REFRESH_TOKEN="+lkEnvRefresh), lkReportArgs...)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	grants := ts.calls()
	if len(grants) != 2 || grants[0].Get("refresh_token") != lkRefresh1 || grants[1].Get("refresh_token") != lkEnvRefresh {
		t.Fatalf("grants = %d, want the stored refresh token first, then the environment's", len(grants))
	}
	if cfg := lkStored(t); cfg.RefreshToken != lkRefresh2 {
		t.Fatal("the refresh token the fallback rotated to was not saved")
	}
}

// --dry-run previews the request and never calls the token endpoint.
func TestQBODryRunNeverRefreshes(t *testing.T) {
	lkIsolate(t)
	lkSeed(t, lkOldAccess, lkRefresh1, time.Now().Add(-time.Hour))
	ts := newLKTokenServer(t, func(string) (int, string) { return 200, lkGrantAnswer(lkNewAccess, lkRefresh2) })
	api := newLKAPI(t, ts, func(string) bool { return true })
	_, _ = lkRun(t, lkEnv(t, api), "customers", "create", "--display-name", "Larkspur", "--dry-run")
	if n := len(ts.calls()); n != 0 {
		t.Fatalf("--dry-run made %d token grants", n)
	}
}

// When the new tokens cannot be saved, the request fails and says so
// instead of going ahead: QuickBooks may already have retired the refresh
// token the grant sent.
func TestQBORefreshThatCannotBeSavedFailsTheRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory mode does not stop a write on Windows")
	}
	lkIsolate(t)
	lkSeed(t, lkOldAccess, lkRefresh1, time.Now().Add(-time.Minute))
	dataDir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	// The lock file exists already, so only the save itself meets the
	// read-only directory.
	credsPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	qboWriteFile(t, credsPath+".refresh.lock", "\x00")
	if err := os.Chmod(dataDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dataDir, 0o700) })
	if f, err := os.CreateTemp(dataDir, "probe"); err == nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		t.Skip("this user can write a mode 0500 directory (root?), so the save cannot be made to fail")
	}
	ts := newLKTokenServer(t, func(string) (int, string) { return 200, lkGrantAnswer(lkNewAccess, lkRefresh2) })
	api := newLKAPI(t, ts, func(tok string) bool { return tok == lkNewAccess })

	out, code := lkRun(t, lkEnv(t, api), lkReportArgs...)
	if code == 0 || !strings.Contains(out, "saving refreshed token") {
		t.Fatalf("exit %d, want a failure naming the save: %s", code, out)
	}
	if n := len(api.headers()); n != 0 {
		t.Fatalf("the request went ahead (%d API calls) after the refresh could not be saved", n)
	}
	if qboLeaks(out, lkRefresh2) || qboLeaks(out, lkNewAccess) {
		t.Fatal("the save error carries a new token")
	}
}

// A token endpoint that answers an error and repeats the refresh token (and
// a fresh-looking token pair) in its body reaches no output: not the CLI's
// stdout or stderr, not an MCP result, not a file the runs wrote.
func TestQBOTokenEndpointErrorBodyReachesNoOutput(t *testing.T) {
	home := lkIsolate(t)
	lkSeed(t, lkOldAccess, lkRefresh1, time.Now().Add(-time.Minute))
	echo := `{"error":"invalid_client","error_description":"client ` + lkClientSecret + ` with ` + lkRefresh1 + `","refresh_token":"` + lkRefresh2 + `","access_token":"` + lkNewAccess + `","hint":"` + base64.StdEncoding.EncodeToString([]byte(lkRefresh1)) + `"}`
	ts := newLKTokenServer(t, func(string) (int, string) { return 400, echo })
	api := newLKAPI(t, ts, func(string) bool { return false })
	env := lkEnv(t, api)

	outputs := map[string]string{}
	outputs["cli report"], _ = lkRun(t, env, lkReportArgs...)
	outputs["cli report --agent"], _ = lkRun(t, env, "report", "pnl", "--month", "2025-01", "--agent", "--no-cache")
	outputs["cli auth refresh"], _ = lkRun(t, env, "auth", "refresh", "--json")
	outputs["mcp quickbooks_read"] = qboMCP(t, env, "quickbooks_read", map[string]any{"endpoint_id": "query.run", "params": map[string]any{"query": "select * from Customer"}})
	for name, out := range outputs {
		for _, secret := range []string{lkRefresh1, lkRefresh2, lkNewAccess, lkClientSecret} {
			if qboLeaks(out, secret) {
				t.Errorf("%s carried a credential from the token endpoint's error body: %s", name, out)
			}
		}
	}
	if !strings.Contains(outputs["cli auth refresh"], "invalid_client") {
		t.Errorf("auth refresh lost the OAuth error code: %s", outputs["cli auth refresh"])
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if base := filepath.Base(p); base == "credentials.toml" || base == "config.toml" {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, secret := range []string{lkRefresh2, lkNewAccess} {
			if qboLeaks(string(b), secret) {
				t.Errorf("file %s carries a credential from the token endpoint's error body", p)
			}
		}
		return nil
	})
	if cfg := lkStored(t); cfg.RefreshToken != lkRefresh1 {
		t.Error("a token from an error answer was saved")
	}
}

// The MCP server's own client (quickbooks_read) refreshes too.
func TestQBOMCPReadRefreshes(t *testing.T) {
	lkIsolate(t)
	lkSeed(t, lkOldAccess, lkRefresh1, time.Now().Add(-time.Minute))
	ts := newLKTokenServer(t, func(string) (int, string) { return 200, lkGrantAnswer(lkNewAccess, lkRefresh2) })
	api := newLKAPI(t, ts, func(tok string) bool { return tok == lkNewAccess })
	out := qboMCP(t, lkEnv(t, api), "quickbooks_read", map[string]any{"endpoint_id": "query.run", "params": map[string]any{"query": "select * from Customer"}})
	if !strings.Contains(out, "Larkspur Widget Co") {
		t.Fatalf("quickbooks_read did not return the read after a refresh: %s", out)
	}
	if n := len(ts.calls()); n != 1 {
		t.Fatalf("%d token grants, want 1", n)
	}
	if cfg := lkStored(t); cfg.RefreshToken != lkRefresh2 {
		t.Fatal("the MCP refresh did not save the rotated refresh token")
	}
}

// The Claude Desktop bundle passes every credential in the environment and
// has no store. A stale QUICKBOOKS_ACCESS_TOKEN gets a 401, the refresh uses
// QUICKBOOKS_REFRESH_TOKEN and saves what QuickBooks rotated to; the next
// process, still handed the stale token, uses the saved one after its 401
// without another token call.
func TestQBOEnvironmentOnlyCredentialsRefreshOnceAcrossRuns(t *testing.T) {
	lkIsolate(t)
	ts := newLKTokenServer(t, func(rt string) (int, string) {
		if rt == lkEnvRefresh {
			return 200, lkGrantAnswer(lkNewAccess, lkRefresh2)
		}
		return 400, `{"error":"invalid_grant"}`
	})
	api := newLKAPI(t, ts, func(tok string) bool { return tok == lkNewAccess })
	env := append(lkEnv(t, api),
		"QUICKBOOKS_ACCESS_TOKEN="+lkOldAccess,
		"QUICKBOOKS_CLIENT_ID="+lkClientID,
		"QUICKBOOKS_CLIENT_SECRET="+lkClientSecret,
		"QUICKBOOKS_REFRESH_TOKEN="+lkEnvRefresh)
	for run := 1; run <= 2; run++ {
		if out, code := lkRun(t, env, lkReportArgs...); code != 0 {
			t.Fatalf("run %d exit %d: %s", run, code, out)
		}
	}
	if n := len(ts.calls()); n != 1 {
		t.Fatalf("%d token grants over two runs, want 1", n)
	}
	if cfg := lkStored(t); cfg.RefreshToken != lkRefresh2 {
		t.Fatal("the rotated refresh token was not saved")
	}
	// Only the tokens are saved: the client secret the environment holds is
	// not copied into the credential store.
	credsPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(credsPath); err != nil || strings.Contains(string(b), lkClientSecret) {
		t.Fatalf("the credential store holds the environment's client secret (read err %v)", err)
	}
}

// Codex round 1, class "a token answer's values are held whatever its
// status": a refused grant whose body still carries a token pair, then an
// API answer repeating that access token. Neither the CLI (--raw included)
// nor the MCP server may print it.
func TestQBOTokensInARefusedGrantAreHeld(t *testing.T) {
	lkIsolate(t)
	const minted = "lk-access-in-refusal-0001"
	var grant atomic.Int64
	ts := newLKTokenServer(t, func(string) (int, string) {
		// Codex round 2: a token can ride in an "error" member too (a
		// duplicate key, a nested object), not only beside it.
		switch grant.Add(1) % 3 {
		case 1:
			return 400, `{"error":"invalid_client","access_token":"` + minted + `","refresh_token":"` + lkRefresh2 + `"}`
		case 2:
			return 400, `{"error":"` + minted + `","error":"invalid_client"}`
		default:
			return 400, `{"error":{"error":"` + minted + `"}}`
		}
	})
	api := lkServe(t, ts, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/query") {
			_, _ = io.WriteString(w, `{"QueryResponse":{"Customer":[{"Id":"1","DisplayName":"`+minted+`"}]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"Header":{"ReportName":"CashFlow"},"Rows":{"Row":[{"type":"Data","ColData":[{"value":"`+minted+`"},{"value":"1"}]}]}}`)
	})
	env := []string{
		"QUICKBOOKS_BASE_URL=" + api.URL + "/v3/company/42",
		"QUICKBOOKS_TOKEN_URL=" + api.URL + lkTokenPath,
		lkCAEnv(t, api),
		"QUICKBOOKS_NO_LEARN=true",
		"QUICKBOOKS_CLIENT_ID=" + lkClientID,
		"QUICKBOOKS_CLIENT_SECRET=" + lkClientSecret,
		"QUICKBOOKS_REFRESH_TOKEN=" + lkEnvRefresh,
	}
	outputs := map[string]string{}
	for i := 1; i <= 3; i++ {
		outputs[fmt.Sprintf("cli raw %d", i)], _ = lkRun(t, env, "report", "cash-flow", "--month", "2025-01", "--raw", "--json", "--no-cache")
		outputs[fmt.Sprintf("cli json %d", i)], _ = lkRun(t, env, "report", "cash-flow", "--month", "2025-01", "--json", "--no-cache")
		outputs[fmt.Sprintf("mcp quickbooks_read %d", i)] = qboMCP(t, env, "quickbooks_read", map[string]any{"endpoint_id": "query.run", "params": map[string]any{"query": "select * from Customer"}})
	}
	if len(ts.calls()) == 0 {
		t.Fatal("the case never reached the token endpoint")
	}
	for name, out := range outputs {
		if qboLeaks(out, minted) {
			t.Errorf("%s printed a token from a refused grant: %s", name, out)
		}
	}
}

// Codex round 1, class "the store a refresh writes and locks is the store
// Load reads": a --config profile with its own data/credentials.toml. The
// rotated pair must land there (the next run reads it), and the global store
// must stay untouched.
func TestQBOProfileRefreshSavesToTheProfileStore(t *testing.T) {
	home := lkIsolate(t)
	var mu sync.Mutex
	valid := lkRefresh1
	ts := newLKTokenServer(t, func(rt string) (int, string) {
		mu.Lock()
		defer mu.Unlock()
		if rt != valid {
			return 400, `{"error":"invalid_grant"}`
		}
		if rt == lkRefresh1 {
			valid = lkRefresh2
			return 200, lkGrantAnswer(lkNewAccess, lkRefresh2)
		}
		return 200, lkGrantAnswer(lkNewAccess+"-2", "lk-refresh-r3-1AaB")
	})
	api := newLKAPI(t, ts, func(tok string) bool { return strings.HasPrefix(tok, lkNewAccess) })
	profile := filepath.Join(home, "profile")
	if err := os.MkdirAll(filepath.Join(profile, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(profile, "config.toml")
	qboWriteFile(t, cfgPath, "base_url = \""+api.srv.URL+"/v3/company/42\"\n")
	qboWriteFile(t, filepath.Join(profile, "data", "credentials.toml"),
		"access_token = \""+lkOldAccess+"\"\nrefresh_token = \""+lkRefresh1+"\"\ntoken_expiry = 2020-01-01T00:00:00Z\nclient_id = \""+lkClientID+"\"\nclient_secret = \""+lkClientSecret+"\"\n")
	env := []string{"QUICKBOOKS_TOKEN_URL=" + api.srv.URL + lkTokenPath, "QUICKBOOKS_NO_LEARN=true", lkCAEnv(t, api.srv)}
	args := append([]string{"--config", cfgPath}, lkReportArgs...)
	for run := 1; run <= 2; run++ {
		if out, code := lkRun(t, env, args...); code != 0 {
			t.Fatalf("run %d exit %d: %s", run, code, out)
		}
	}
	if n := len(ts.calls()); n != 1 {
		t.Fatalf("%d token grants over two runs, want 1 (the second run should read the rotated pair)", n)
	}
	b, err := os.ReadFile(filepath.Join(profile, "data", "credentials.toml"))
	if err != nil || !strings.Contains(string(b), lkRefresh2) {
		t.Fatalf("the profile store does not hold the rotated refresh token (err %v)", err)
	}
	globalPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(globalPath); err == nil {
		t.Fatal("the refresh wrote the global credential store for a --config profile")
	}
}
