// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/config"
)

// userAuthStub answers partner and company logins with tokens named after
// the login email, refreshes, and account mints.
func userAuthStub(t *testing.T) *httptest.Server {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case r.URL.Path == cloudallyPartnerLoginPath:
			_, _ = w.Write([]byte(`{"accessToken":"partner-access-` + body["email"] + `","expiresIn":1893456000000,"refreshToken":"partner-refresh-` + body["email"] + `"}`))
		case r.URL.Path == cloudallyAccountLoginPath:
			_, _ = w.Write([]byte(`{"accessToken":"company-access-` + body["email"] + `","expiresIn":1893456000000,"refreshToken":"company-refresh"}`))
		case r.URL.Path == cloudallyRefreshPath:
			_, _ = w.Write([]byte(`{"accessToken":"rotated-` + strings.Repeat("x", int(n.Add(1))) + `","expiresIn":1893456000000,"refreshToken":"` + body["refreshToken"] + `-r"}`))
		case strings.HasSuffix(r.URL.Path, "/partner-access-token"):
			_, _ = w.Write([]byte(`{"accessToken":"account-token-` + strings.Repeat("y", int(n.Add(1))) + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)
	return srv
}

func mustRun(t *testing.T, args ...string) {
	t.Helper()
	if _, _, err := runAuthCmd(t, "", args...); err != nil {
		t.Fatalf("%s: %v", strings.Join(args, " "), err)
	}
}

func storeHas(path, want string) bool {
	data, err := os.ReadFile(path)
	return err == nil && string(data) == want
}

// Round two, item 1: two partner users of one partner (one API key pair)
// have different rights, so neither may open the other's cached customer
// rows, directly or through use-account.
func TestPartnerUsersOfOneKeyPairDoNotShareAStore(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	userAuthStub(t)

	mustRun(t, "auth", "login", "--partner", "--email", "admin@msp.example", "--password", "pw")
	admin := currentStorePath()
	writeStoreMarker(t, admin, "every customer's rows")
	mustRun(t, "auth", "use-account", "5f1c2a")
	adminAccount := currentStorePath()

	mustRun(t, "auth", "login", "--partner", "--email", "restricted@msp.example", "--password", "pw")
	restricted := currentStorePath()
	if restricted == admin {
		t.Fatalf("the restricted partner user opened the administrator's store %s", filepath.Base(admin))
	}
	if storeHas(restricted, "every customer's rows") {
		t.Fatal("the restricted user's store holds the administrator's rows")
	}
	mustRun(t, "auth", "use-account", "5f1c2a")
	if got := currentStorePath(); got == adminAccount || got == admin {
		t.Fatalf("use-account under the restricted user opened the administrator's store %s", filepath.Base(got))
	}

	// The administrator signing in again finds the administrator's rows.
	mustRun(t, "auth", "login", "--partner", "--email", "Admin@MSP.example ", "--password", "pw")
	if got := currentStorePath(); got != admin || !storeHas(got, "every customer's rows") {
		t.Fatalf("the administrator's re-login opened %s", filepath.Base(got))
	}
}

// Round two, item 2: the API key pair is not part of the key, so rotating
// it and signing in again keeps the same user's ledger. Isolation holds
// because the principal is the CloudAlly user: a different user under the
// new key pair still gets a store of its own.
func TestRotatedKeyPairKeepsTheStore(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	userAuthStub(t)

	mustRun(t, "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw")
	before := currentStorePath()
	writeStoreMarker(t, before, "restore ledger with 3 batches")

	t.Setenv("CLOUDALLY_CLIENT_ID", "rotated-cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "rotated-secret")
	mustRun(t, "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw")
	after := currentStorePath()
	if after != before || !storeHas(after, "restore ledger with 3 batches") {
		t.Fatalf("rotating the key pair moved the store: %s -> %s", filepath.Base(before), filepath.Base(after))
	}

	mustRun(t, "auth", "login", "--partner", "--email", "other@example.com", "--password", "pw")
	if got := currentStorePath(); got == before {
		t.Fatalf("another partner user under the rotated key pair opened %s", filepath.Base(before))
	}
	mustRun(t, "auth", "login", "--email", "tech@example.com", "--password", "pw")
	if got := currentStorePath(); got == before {
		t.Fatal("a company login with the same email opened the partner user's store")
	}
}

// legacyPartnerRecord writes the record the original binary left behind:
// no fingerprints, no email, the active partner token kept aside.
func legacyPartnerRecord(t *testing.T, access, refresh string) {
	t.Helper()
	if err := saveAuthContext(&authContext{Scope: authScopePartner, PartnerAccessToken: access, PartnerRefreshToken: refresh}); err != nil {
		t.Fatal(err)
	}
}

// Round two, item 3a: a record from the original binary is bound when its
// token is the active one, its token-named store is carried over, and it
// stays bound through the automatic refresh instead of every run moving to
// a fresh per-token store. Its store is never merged into the store of the
// user who signs in next.
func TestLegacyRecordIsBoundAndFollowsRefresh(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
	default:
		t.Skip("legacy store adoption is a no-op on " + runtime.GOOS)
	}
	isolateAuthState(t)
	resetStoreScope(t)
	userAuthStub(t)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens("cid", "csecret", "old-binary-access", "old-binary-refresh", time.Now().Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	legacyPartnerRecord(t, "old-binary-access", "old-binary-refresh")
	cfg, _ = config.Load("")
	tokenStore := filepath.Join(filepath.Dir(currentStorePathUnscoped(t)), "data-"+hashDefaultDBScope(cfg.StoreScopeLegacyCredential())+".db")
	writeStoreMarker(t, tokenStore, "ledger from the original binary")

	bound := currentStorePath()
	if bound == tokenStore {
		t.Fatal("the legacy record was not bound")
	}
	if !storeHas(bound, "ledger from the original binary") {
		t.Fatal("the original binary's store was not carried over")
	}
	if err := requireBoundStoreScope(); err != nil {
		t.Fatalf("a bound legacy record should pass: %v", err)
	}

	if err := cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0)); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	reloaded, _ := config.Load("")
	if reloaded.AccessToken == "old-binary-access" {
		t.Fatal("the token was not refreshed")
	}
	if got := currentStorePath(); got != bound {
		t.Fatalf("the refresh moved the legacy session's store: %s -> %s", filepath.Base(bound), filepath.Base(got))
	}
	if actx, ok := loadAuthContext(); !ok || actx.TokenFingerprint == "" || !strings.HasPrefix(actx.Principal, principalLegacyPrefix) {
		t.Fatalf("the refresh did not record the binding: %+v", actx)
	}

	// The next explicit login names its user and gets that user's store;
	// the unidentified session's rows are not merged into it.
	mustRun(t, "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw")
	if got := currentStorePath(); got == bound || storeHas(got, "ledger from the original binary") {
		t.Fatalf("the legacy session's store was merged into the identified user's store %s", filepath.Base(got))
	}
}

// currentStorePathUnscoped is where the unscoped store would live, used only
// to find the data directory.
func currentStorePathUnscoped(t *testing.T) string {
	t.Helper()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "data.db")
}

// Round two, item 3b: a record that exists but cannot be used stops the
// ledger commands instead of handing them an empty per-token store.
func TestUnusableRecordFailsClosed(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	userAuthStub(t)
	mustRun(t, "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw")
	path, _ := authContextPath()

	check := func(name string, wantErr bool) {
		t.Helper()
		currentStorePath()
		err := requireBoundStoreScope()
		if wantErr {
			if err == nil {
				t.Fatalf("%s: want a fail-closed error", name)
			}
			if !errors.Is(err, errStoreScopeUnbound) {
				t.Fatalf("%s: unexpected error %v", name, err)
			}
			_ = cliErrCode(t, err) // a typed CLI error, so the exit code is a config error
			return
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	check("bound", false)

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		check("refused for its permissions", true)
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
		check("permissions fixed", false)
	}

	original, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	check("unreadable", true)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.Load("")
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "hand-swapped-token", "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	check("stale (describes another token)", true)

	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "hand-swapped-token")
	check("environment token", false)
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "")

	if err := clearAuthContext(); err != nil {
		t.Fatal(err)
	}
	check("no record", false)
}

// Round two, item 3c: when the record cannot be updated after a refresh,
// the command stops instead of carrying on unbound.
func TestRefreshFailsWhenTheRecordCannotBeSaved(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	userAuthStub(t)
	mustRun(t, "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw")
	cfg, _ := config.Load("")
	cfg.TokenExpiry = time.Now().Add(10 * time.Second)

	orig := authContextVerifyPerms
	var calls int
	authContextVerifyPerms = func(p string) error {
		calls++
		if calls >= 2 { // the read passes; the write after the refresh does not
			return errors.New("owner is not the current user")
		}
		return orig(p)
	}
	t.Cleanup(func() { authContextVerifyPerms = orig })

	err := cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0))
	if err == nil {
		t.Fatal("the refresh carried on although the record could not be saved")
	}
	if !strings.Contains(err.Error(), authContextFileName) || cliErrCode(t, err) == 0 {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Codex review of round two: an old unscoped data.db with no owner (perhaps
// filled by another user) is never handed to the first user who signs in.
func TestUnownedUnscopedStoreIsNotClaimedByAUser(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	userAuthStub(t)
	unscoped := currentStorePathUnscoped(t)
	writeStoreMarker(t, unscoped, "an earlier user's rows")

	mustRun(t, "auth", "login", "--partner", "--email", "restricted@msp.example", "--password", "pw")
	got := currentStorePath()
	if got == unscoped || storeHas(got, "an earlier user's rows") {
		t.Fatalf("the signed-in user was handed the unowned data.db (%s)", filepath.Base(got))
	}
	if data, err := os.ReadFile(unscoped + ".owner"); err != nil || !strings.HasPrefix(string(data), "unclaimed") {
		t.Fatalf("owner marker = %q, %v", data, err)
	}
	if !storeHas(unscoped, "an earlier user's rows") {
		t.Fatal("data.db itself should be left in place")
	}
}

// If the marker cannot be written (a read-only data directory), the ledger
// commands fail closed rather than reading the unowned data.db.
func TestUnwritableUnclaimedMarkerFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory modes")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	isolateAuthState(t)
	resetStoreScope(t)
	userAuthStub(t)
	mustRun(t, "auth", "login", "--partner", "--email", "restricted@msp.example", "--password", "pw")
	// data.db appears after the login (which already marks an existing one).
	unscoped := currentStorePathUnscoped(t)
	writeStoreMarker(t, unscoped, "an earlier user's rows")

	dir := filepath.Dir(unscoped)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	currentStorePath()
	if err := requireBoundStoreScope(); err == nil || !errors.Is(err, errStoreScopeUnbound) {
		t.Fatalf("want a fail-closed error while data.db cannot be marked, got %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	currentStorePath()
	if err := requireBoundStoreScope(); err != nil {
		t.Fatalf("once the marker can be written the scope is bound: %v", err)
	}
}
