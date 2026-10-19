// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/config"
)

func runAuthCmd(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var flags rootFlags
	root := newRootCmd(&flags)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	if stdin != "" {
		root.SetIn(strings.NewReader(stdin))
	}
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func cliErrCode(t *testing.T, err error) int {
	t.Helper()
	var ce *cliError
	if !errors.As(err, &ce) {
		t.Fatalf("expected cliError, got %T: %v", err, err)
	}
	return ce.code
}

func TestAuthLoginMissingInputsAreUsageErrors(t *testing.T) {
	isolateAuthState(t)
	_, _, err := runAuthCmd(t, "", "auth", "login")
	if cliErrCode(t, err) != 2 || !strings.Contains(err.Error(), "CLOUDALLY_EMAIL") || strings.Contains(err.Error(), "PARTNER") {
		t.Fatalf("expected usage error naming CLOUDALLY_EMAIL, got %v", err)
	}
	_, _, err = runAuthCmd(t, "", "auth", "login", "--email", "a@b.c")
	if cliErrCode(t, err) != 2 || !strings.Contains(err.Error(), "CLOUDALLY_PASSWORD") || strings.Contains(err.Error(), "PARTNER") {
		t.Fatalf("expected usage error naming CLOUDALLY_PASSWORD, got %v", err)
	}
	_, _, err = runAuthCmd(t, "", "auth", "login", "--partner")
	if cliErrCode(t, err) != 2 || !strings.Contains(err.Error(), "CLOUDALLY_PARTNER_EMAIL") {
		t.Fatalf("expected usage error naming CLOUDALLY_PARTNER_EMAIL, got %v", err)
	}
	_, _, err = runAuthCmd(t, "", "auth", "login", "--partner", "--email", "a@b.c")
	if cliErrCode(t, err) != 2 || !strings.Contains(err.Error(), "CLOUDALLY_PARTNER_PASSWORD") {
		t.Fatalf("expected usage error naming CLOUDALLY_PARTNER_PASSWORD, got %v", err)
	}
	_, _, err = runAuthCmd(t, "", "auth", "login", "--email", "a@b.c", "--password", "x", "--password-stdin")
	if cliErrCode(t, err) != 2 {
		t.Fatalf("expected usage error for both password flags, got %v", err)
	}
	_, _, err = runAuthCmd(t, "", "auth", "login", "--partner", "--as-account", "--email", "a@b.c", "--password", "x")
	if cliErrCode(t, err) != 2 {
		t.Fatalf("expected usage error for --partner with --as-account, got %v", err)
	}
}

// Partner env vars must never feed the default company login, so a partner
// password set in the shell cannot silently widen a single-company session.
func TestAuthLoginDefaultIgnoresPartnerEnv(t *testing.T) {
	isolateAuthState(t)
	t.Setenv("CLOUDALLY_PARTNER_EMAIL", "partner@example.com")
	t.Setenv("CLOUDALLY_PARTNER_PASSWORD", "partner-pw")
	_, _, err := runAuthCmd(t, "", "auth", "login")
	if cliErrCode(t, err) != 2 || !strings.Contains(err.Error(), "CLOUDALLY_EMAIL") {
		t.Fatalf("expected default login to ignore partner env, got %v", err)
	}
}

func TestAuthLoginDefaultUsesAccountEndpointAndEnv(t *testing.T) {
	isolateAuthState(t)
	t.Setenv("CLOUDALLY_EMAIL", "backups@contoso.com")
	t.Setenv("CLOUDALLY_PASSWORD", "pw")
	stdout, _, err := runAuthCmd(t, "", "auth", "login", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(stdout, "POST /v2/auth as b***@contoso.com (account login)") {
		t.Fatalf("default login dry-run output = %q", stdout)
	}
}

func TestAuthLoginDryRunMasksEmailAndSkipsNetwork(t *testing.T) {
	isolateAuthState(t)
	stdout, _, err := runAuthCmd(t, "", "auth", "login", "--dry-run", "--partner", "--email", "tech@example.com", "--password", "x")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(stdout, "/v2/auth/partner") || !strings.Contains(stdout, "t***@example.com") {
		t.Fatalf("dry-run output = %q", stdout)
	}
	if strings.Contains(stdout, "tech@") {
		t.Fatalf("dry-run leaked the email: %q", stdout)
	}
	stdout, _, err = runAuthCmd(t, "", "auth", "login", "--dry-run", "--email", "c@d.e", "--password", "x")
	if err != nil {
		t.Fatalf("dry run as account: %v", err)
	}
	if !strings.Contains(stdout, "POST /v2/auth as") {
		t.Fatalf("as-account dry-run output = %q", stdout)
	}
}

func TestAuthLoginSavesTokensAndUseAccountSwitches(t *testing.T) {
	isolateAuthState(t)
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")

	var loginBody map[string]string
	var mintAuth string
	var mintPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("client-id") != "cid" || r.Header.Get("client-secret") != "csecret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v2/auth/partner":
			_ = json.NewDecoder(r.Body).Decode(&loginBody)
			if loginBody["password"] != "secret-pw" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errorMessage":"Bad credentials"}`))
				return
			}
			_, _ = w.Write([]byte(`{"accessToken":"partner-token","tokenType":"Bearer","expiresIn":1893456000000,"refreshToken":"partner-refresh","refreshTokenExpiresIn":1894000000000}`))
		case strings.HasSuffix(r.URL.Path, "/partner-access-token"):
			mintAuth = r.Header.Get("Authorization")
			mintPath = r.URL.Path
			_, _ = w.Write([]byte(`{"accessToken":"account-token"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Point the config at the test server before any command runs.
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.BaseURL = srv.URL
	if err := cfg.SaveTokens("", "", "", "", time.Time{}); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	_, _, err = runAuthCmd(t, "", "auth", "login", "--partner", "--email", "tech@example.com", "--password", "wrong")
	if cliErrCode(t, err) != 4 || !strings.Contains(err.Error(), "Bad credentials") {
		t.Fatalf("expected authErr with server message, got %v", err)
	}

	stdout, stderr, err := runAuthCmd(t, "secret-pw\n", "auth", "login", "--partner", "--email", "tech@example.com", "--password-stdin")
	if err != nil {
		t.Fatalf("login: %v (stdout=%q stderr=%q)", err, stdout, stderr)
	}
	if loginBody["email"] != "tech@example.com" || loginBody["password"] != "secret-pw" {
		t.Fatalf("login body = %v", loginBody)
	}
	if !strings.Contains(stderr, "Logged in as partner; token expires 2030-01-01T") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stdout+stderr, "partner-token") {
		t.Fatalf("token value leaked: %q %q", stdout, stderr)
	}
	cfg, err = config.Load("")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.AccessToken != "partner-token" || cfg.RefreshToken != "partner-refresh" {
		t.Fatalf("saved tokens = (%q, %q)", cfg.AccessToken, cfg.RefreshToken)
	}
	actx, ok := loadAuthContext()
	if !ok || actx.Scope != authScopePartner || actx.PartnerAccessToken != "partner-token" {
		t.Fatalf("sidecar after login = %+v (ok=%v)", actx, ok)
	}

	stdout, _, err = runAuthCmd(t, "", "auth", "use-account", "5f1c2a", "--json")
	if err != nil {
		t.Fatalf("use-account: %v", err)
	}
	if mintAuth != "Bearer partner-token" || mintPath != "/v2/distributors/accounts/5f1c2a/partner-access-token" {
		t.Fatalf("mint request = %q %q", mintAuth, mintPath)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("use-account json: %v (%q)", err, stdout)
	}
	if result["scope"] != "account" || result["account_id"] != "5f1c2a" {
		t.Fatalf("use-account result = %v", result)
	}
	cfg, _ = config.Load("")
	if cfg.AccessToken != "account-token" || cfg.RefreshToken != "" {
		t.Fatalf("active token after use-account = (%q, %q)", cfg.AccessToken, cfg.RefreshToken)
	}
	actx, _ = loadAuthContext()
	if actx.Scope != authScopeAccount || actx.PartnerAccessToken != "partner-token" || actx.PartnerRefreshToken != "partner-refresh" {
		t.Fatalf("sidecar after use-account = %+v", actx)
	}

	// Switching to a second account mints with the stashed partner token.
	mintAuth = ""
	_, _, err = runAuthCmd(t, "", "auth", "use-account", "abc123", "--reseller")
	if err != nil {
		t.Fatalf("second use-account: %v", err)
	}
	if mintAuth != "Bearer partner-token" || mintPath != "/v2/resellers/accounts/abc123/partner-access-token" {
		t.Fatalf("second mint request = %q %q", mintAuth, mintPath)
	}

	_, stderr, err = runAuthCmd(t, "", "auth", "use-account", "--partner")
	if err != nil {
		t.Fatalf("use-account --partner: %v", err)
	}
	if !strings.Contains(stderr, "Restored partner token") {
		t.Fatalf("stderr = %q", stderr)
	}
	cfg, _ = config.Load("")
	if cfg.AccessToken != "partner-token" || cfg.RefreshToken != "partner-refresh" {
		t.Fatalf("tokens after --partner = (%q, %q)", cfg.AccessToken, cfg.RefreshToken)
	}
	actx, _ = loadAuthContext()
	if actx.Scope != authScopePartner || actx.AccountID != "" {
		t.Fatalf("sidecar after --partner = %+v", actx)
	}
}

func TestAuthUseAccountValidation(t *testing.T) {
	isolateAuthState(t)
	_, _, err := runAuthCmd(t, "", "auth", "use-account")
	if cliErrCode(t, err) != 2 || !strings.Contains(err.Error(), "--partner") {
		t.Fatalf("expected usage error, got %v", err)
	}
	_, _, err = runAuthCmd(t, "", "auth", "use-account", "a", "b")
	if cliErrCode(t, err) != 2 {
		t.Fatalf("expected usage error for two ids, got %v", err)
	}
	_, _, err = runAuthCmd(t, "", "auth", "use-account", "a", "--partner")
	if cliErrCode(t, err) != 2 {
		t.Fatalf("expected usage error for id with --partner, got %v", err)
	}
	stdout, _, err := runAuthCmd(t, "", "auth", "use-account", "5f1c2a", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(stdout, "/v2/distributors/accounts/5f1c2a/partner-access-token") {
		t.Fatalf("dry-run output = %q", stdout)
	}
	_, _, err = runAuthCmd(t, "", "auth", "use-account", "--partner")
	if cliErrCode(t, err) != 4 {
		t.Fatalf("expected authErr with nothing stashed, got %v", err)
	}
}
