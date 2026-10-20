// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/config"
)

// resetStoreScope puts the process-wide store scope back after a test.
func resetStoreScope(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		setDefaultDBScopeIdentity("", "", "")
		setLegacyDBClaimSuppressed(false)
	})
	setLegacyDBClaimSuppressed(false)
}

// storePathFor resolves the default store path in dir for cfg, the way the
// root command does before every run.
func storePathFor(cfg *config.Config, dir string) string {
	setDefaultDBScopeIdentity(cfg.StoreScopeCredential(), cfg.StoreScopeLegacyCredential(), cfg.BaseURL)
	return defaultDBPathInDir(dir)
}

func scopeConfig(baseURL, clientID, token string) *config.Config {
	return &config.Config{BaseURL: baseURL, ClientID: clientID, ClientSecret: "csecret", AccessToken: token}
}

const scopeAPI = "https://api.cloudally.com"

// scopeBinding is the binding scopeConfig(scopeAPI, ...) resolves to.
var scopeBinding = authBinding{origin: cloudallyStoreOrigin(scopeAPI)}

// testPartner is the partner user the command-level tests sign in as.
var testPartner = partnerPrincipal("tech@example.com")

// bindSidecar writes the auth record a login or use-account leaves behind
// for a token with no refresh token, under scopeBinding.
func bindSidecar(t *testing.T, scope, token, principal string) {
	t.Helper()
	actx := &authContext{Scope: scope}
	if scope == authScopePartner {
		actx.stashPartner(scopeBinding, token, "", time.Time{}, principal)
	}
	actx.bindTo(scopeBinding, token, "", principal)
	if err := saveAuthContext(actx); err != nil {
		t.Fatalf("save sidecar: %v", err)
	}
}

func TestStoreScopeSurvivesTokenRotation(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	dir := t.TempDir()

	bindSidecar(t, authScopePartner, "token-A", testPartner)
	a := storePathFor(scopeConfig(scopeAPI, "cid", "token-A"), dir)

	// What the refresh hook does on rotation: rebind to the new token.
	bindSidecar(t, authScopePartner, "token-B", testPartner)
	b := storePathFor(scopeConfig(scopeAPI, "cid", "token-B"), dir)
	if a != b {
		t.Fatalf("token rotation moved the store: %s -> %s", filepath.Base(a), filepath.Base(b))
	}
	if got := storePathFor(scopeConfig("HTTPS://API.cloudally.com/", "cid", "token-B"), dir); got != a {
		t.Fatalf("a spelling variant of the base URL moved the store: %s -> %s", filepath.Base(a), filepath.Base(got))
	}
	// Same user, rotated API key pair: same store (see TestRotatedKeyPairKeepsTheStore).
	if got := storePathFor(scopeConfig(scopeAPI, "other-client", "token-B"), dir); got != a {
		t.Fatalf("a rotated key pair moved the store: %s -> %s", filepath.Base(a), filepath.Base(got))
	}
	if got := storePathFor(scopeConfig("https://stub.example.test", "cid", "token-B"), dir); got == a {
		t.Fatalf("a different API origin shares the store %s", filepath.Base(a))
	}
	id := cloudallyStoreScopeIdentity(scopeConfig(scopeAPI, "cid", "token-B"))
	if id == "" {
		t.Fatal("a bound partner session should have a stable identity")
	}
	for _, secret := range []string{"token-B", "csecret"} {
		if strings.Contains(id, secret) {
			t.Fatalf("store identity %q carries a secret", id)
		}
	}
}

// One API key pair serves partner and company logins, so the principal has
// to be part of the key: a company login must never open the store a
// partner login filled with every customer's rows, and two companies, or two
// customers reached through use-account, must not share one either.
func TestStoreScopeSeparatesPrincipals(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	dir := t.TempDir()
	paths := map[string]string{}
	for _, p := range []struct{ name, scope, principal string }{
		{"partner", authScopePartner, testPartner},
		{"company A", authScopeAccount, companyPrincipal("backups@contoso.com")},
		{"company B", authScopeAccount, companyPrincipal("it@fabrikam.com")},
		{"customer 5f1c2a", authScopeAccount, accountPrincipal(testPartner, "5f1c2a")},
		{"customer abc123", authScopeAccount, accountPrincipal(testPartner, "abc123")},
		{"second partner user", authScopePartner, partnerPrincipal("restricted@example.com")},
		{"customer 5f1c2a for the second user", authScopeAccount, accountPrincipal(partnerPrincipal("restricted@example.com"), "5f1c2a")},
	} {
		token := "token-" + p.name
		bindSidecar(t, p.scope, token, p.principal)
		got := storePathFor(scopeConfig(scopeAPI, "cid", token), dir)
		for other, path := range paths {
			if path == got {
				t.Fatalf("%s and %s share the store %s", p.name, other, filepath.Base(got))
			}
		}
		paths[p.name] = got
	}
	if companyPrincipal("Backups@Contoso.com ") != companyPrincipal("backups@contoso.com") {
		t.Fatal("the company principal should not depend on email case or spacing")
	}
}

// A record that does not describe the active token never names the store.
// Where that is by design (an environment token, a raw auth header, a known
// token with no known user) the per-token store is used; where the record
// is simply stale the run fails closed for ledger commands.
func TestStoreScopeIgnoresARecordForAnotherToken(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "")
	bindSidecar(t, authScopePartner, "token-A", testPartner)

	stale := scopeConfig(scopeAPI, "cid", "other-token")
	if id := cloudallyStoreScopeIdentity(stale); id != "" {
		t.Fatalf("identity %q came from a record for another token", id)
	}
	if err := requireBoundStoreScope(); err == nil {
		t.Fatal("a stale record should fail ledger commands closed")
	}

	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "env-token")
	env := scopeConfig(scopeAPI, "cid", "env-token")
	if id := cloudallyStoreScopeIdentity(env); id != "" {
		t.Fatalf("identity %q for an environment token", id)
	}
	if env.StoreScopeCredential() != env.StoreScopeLegacyCredential() {
		t.Fatal("an environment token should keep the generated per-token key")
	}
	if err := requireBoundStoreScope(); err != nil {
		t.Fatalf("an environment token is per-token by design: %v", err)
	}
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "")

	withHeader := scopeConfig(scopeAPI, "cid", "token-A")
	withHeader.AuthHeaderVal = "Bearer other"
	if id := cloudallyStoreScopeIdentity(withHeader); id != "" {
		t.Fatalf("identity under a raw auth header = %q", id)
	}
	if err := requireBoundStoreScope(); err != nil {
		t.Fatalf("a raw auth header is per-token by design: %v", err)
	}

	// A known token whose user is not known.
	bindSidecar(t, authScopeAccount, "token-C", "")
	if id := cloudallyStoreScopeIdentity(scopeConfig(scopeAPI, "cid", "token-C")); id != "" {
		t.Fatalf("identity %q for a token with no known user", id)
	}
	if err := requireBoundStoreScope(); err != nil {
		t.Fatalf("a known token with no user is per-token by design: %v", err)
	}
}

// A store kept under the per-token key while no record vouched for the
// token (for example when the record could not be written) is adopted once
// the principal is vouched for, so its rows and restore ledger carry over.
func TestStoreScopeAdoptsTokenNamedStore(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
	default:
		t.Skip("legacy store adoption is a no-op on " + runtime.GOOS)
	}
	isolateAuthState(t)
	resetStoreScope(t)
	dir := t.TempDir()
	cfg := scopeConfig(scopeAPI, "cid", "token-A")

	legacy := filepath.Join(dir, "data-"+hashDefaultDBScope(cfg.StoreScopeLegacyCredential())+".db")
	if got := storePathFor(cfg, dir); got != legacy {
		t.Fatalf("without a vouching record the store should be the per-token one: %s", filepath.Base(got))
	}
	bindSidecar(t, authScopePartner, "token-A", testPartner)
	if err := os.WriteFile(legacy, []byte("existing ledger"), 0o600); err != nil {
		t.Fatalf("seed legacy store: %v", err)
	}

	stable := storePathFor(cfg, dir)
	if stable == legacy {
		t.Fatal("the store is still named after the token")
	}
	data, err := os.ReadFile(stable)
	if err != nil || string(data) != "existing ledger" {
		t.Fatalf("stable store content = %q, %v; want the adopted legacy store", data, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("the token-named store is still on disk: %v", err)
	}
	bindSidecar(t, authScopePartner, "token-B", testPartner)
	if got := storePathFor(scopeConfig(scopeAPI, "cid", "token-B"), dir); got != stable {
		t.Fatalf("after adoption a rotated token moved the store: %s -> %s", filepath.Base(stable), filepath.Base(got))
	}
}

// currentStorePath is the store the next command would open.
func currentStorePath() string {
	configureDefaultDBScope("")
	return defaultDBPath("cloudally-pp-cli")
}

func writeStoreMarker(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// End to end through the real commands and the real rotation path: login,
// automatic refresh, and the reviewer's partner-then-company sequence.
func TestStoreFollowsTheLoginNotTheToken(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	var refreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case cloudallyPartnerLoginPath:
			_, _ = w.Write([]byte(`{"accessToken":"partner-access","expiresIn":1893456000000,"refreshToken":"partner-refresh"}`))
		case cloudallyAccountLoginPath:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, _ = w.Write([]byte(`{"accessToken":"company-access-` + body["email"] + `","expiresIn":1893456000000,"refreshToken":"company-refresh"}`))
		case cloudallyRefreshPath:
			refreshes++
			_, _ = w.Write([]byte(`{"accessToken":"rotated-` + time.Now().Format("150405.000000000") + `","expiresIn":1893456000000,"refreshToken":"rotated-refresh"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)

	refresh := func() {
		t.Helper()
		cfg, err := config.Load("")
		if err != nil {
			t.Fatalf("config.Load: %v", err)
		}
		before := cfg.AccessToken
		cfg.TokenExpiry = time.Now().Add(10 * time.Second)
		if err := cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0)); err != nil {
			t.Fatalf("refresh: %v", err)
		}
		after, _ := config.Load("")
		if after.AccessToken == before {
			t.Fatal("the refresh did not rotate the saved token")
		}
	}

	// Partner login, sync something, then an automatic refresh.
	if _, _, err := runAuthCmd(t, "", "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw"); err != nil {
		t.Fatalf("partner login: %v", err)
	}
	partnerStore := currentStorePath()
	writeStoreMarker(t, partnerStore, "partner rows and restore ledger")
	refresh()
	if got := currentStorePath(); got != partnerStore {
		t.Fatalf("the partner refresh moved the store: %s -> %s", filepath.Base(partnerStore), filepath.Base(got))
	}

	// A company login with the same API key pair gets its own store.
	if _, _, err := runAuthCmd(t, "", "auth", "login", "--email", "backups@contoso.com", "--password", "pw"); err != nil {
		t.Fatalf("company login: %v", err)
	}
	companyStore := currentStorePath()
	if companyStore == partnerStore {
		t.Fatalf("the company login opened the partner's store %s", filepath.Base(partnerStore))
	}
	if _, err := os.Stat(companyStore); !os.IsNotExist(err) {
		t.Fatalf("the company store already exists: %v", err)
	}
	writeStoreMarker(t, companyStore, "company rows")
	refresh()
	if got := currentStorePath(); got != companyStore {
		t.Fatalf("the company refresh moved the store: %s -> %s", filepath.Base(companyStore), filepath.Base(got))
	}

	// Signing in as the partner again finds the partner history.
	if _, _, err := runAuthCmd(t, "", "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw"); err != nil {
		t.Fatalf("second partner login: %v", err)
	}
	if got := currentStorePath(); got != partnerStore {
		t.Fatalf("re-login moved the partner store: %s -> %s", filepath.Base(partnerStore), filepath.Base(got))
	}
	if data, err := os.ReadFile(partnerStore); err != nil || string(data) != "partner rows and restore ledger" {
		t.Fatalf("partner store content = %q, %v", data, err)
	}
	if refreshes != 2 {
		t.Fatalf("refreshes = %d, want 2", refreshes)
	}
}

// Codex round two: a company login whose token is refreshed by use-account
// (before a failed mint) must keep the company principal, never become the
// partner and open the partner's store.
func TestUseAccountRefreshNeverFilesACompanyTokenAsPartner(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == cloudallyPartnerLoginPath:
			_, _ = w.Write([]byte(`{"accessToken":"partner-access","expiresIn":1893456000000,"refreshToken":"partner-refresh"}`))
		case r.URL.Path == cloudallyAccountLoginPath:
			// Expires inside the refresh window, so use-account refreshes it.
			_, _ = w.Write([]byte(`{"accessToken":"company-access","expiresIn":30,"refreshToken":"company-refresh"}`))
		case r.URL.Path == cloudallyRefreshPath:
			_, _ = w.Write([]byte(`{"accessToken":"company-rotated","expiresIn":1893456000000,"refreshToken":"company-refresh-2"}`))
		case strings.HasSuffix(r.URL.Path, "/partner-access-token"):
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)

	if _, _, err := runAuthCmd(t, "", "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw"); err != nil {
		t.Fatalf("partner login: %v", err)
	}
	partnerStore := currentStorePath()
	if _, _, err := runAuthCmd(t, "", "auth", "login", "--email", "backups@contoso.com", "--password", "pw"); err != nil {
		t.Fatalf("company login: %v", err)
	}
	companyStore := currentStorePath()

	if _, _, err := runAuthCmd(t, "", "auth", "use-account", "5f1c2a"); err == nil {
		t.Fatal("use-account with a company token should fail to mint")
	}
	cfg, err := config.Load("")
	if err != nil || cfg.AccessToken != "company-rotated" {
		t.Fatalf("use-account did not refresh the company token: %q, %v", cfg.AccessToken, err)
	}
	got := currentStorePath()
	if got == partnerStore {
		t.Fatalf("the refreshed company token opens the partner store %s", filepath.Base(partnerStore))
	}
	if got != companyStore {
		t.Fatalf("the refreshed company token moved off the company store: %s -> %s", filepath.Base(companyStore), filepath.Base(got))
	}
	actx, _ := loadAuthContext()
	if actx.stashPrincipal(bindingFor(cfg)) != "" {
		t.Fatalf("a company token was kept as a verified partner session: %+v", actx)
	}
}

// Codex round two: a refresh of a token the auth record does not describe
// (another config, an environment token) must not overwrite the kept partner
// session, so use-account --partner can never restore that token as the
// partner.
func TestRefreshOfAnotherTokenLeavesThePartnerSessionAlone(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case cloudallyPartnerLoginPath:
			_, _ = w.Write([]byte(`{"accessToken":"partner-access","expiresIn":1893456000000,"refreshToken":"partner-refresh"}`))
		case cloudallyRefreshPath:
			_, _ = w.Write([]byte(`{"accessToken":"other-rotated","expiresIn":1893456000000,"refreshToken":"other-refresh-2"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)

	if _, _, err := runAuthCmd(t, "", "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw"); err != nil {
		t.Fatalf("partner login: %v", err)
	}
	partnerStore := currentStorePath()

	// Another credential set, expiring, refreshed by the client hook.
	other, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	other.AccessToken, other.RefreshToken, other.TokenExpiry = "other-access", "other-refresh", time.Now().Add(10*time.Second)
	if err := cloudallyRefreshHook(client.New(other, 5*time.Second, 0)); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	actx, ok := loadAuthContext()
	if !ok || actx.PartnerAccessToken != "partner-access" || actx.stashPrincipal(bindingFor(other)) != testPartner {
		t.Fatalf("the refresh of another token rewrote the partner session: %+v", actx)
	}
	if id := cloudallyStoreScopeIdentity(other); id != "" {
		t.Fatalf("the other token resolved to %q", id)
	}

	if _, _, err := runAuthCmd(t, "", "auth", "use-account", "--partner"); err != nil {
		t.Fatalf("use-account --partner: %v", err)
	}
	cfg, _ := config.Load("")
	if cfg.AccessToken != "partner-access" {
		t.Fatalf("restored token = %q, want the partner login's token", cfg.AccessToken)
	}
	if got := currentStorePath(); got != partnerStore {
		t.Fatalf("the restored partner session lost its store: %s -> %s", filepath.Base(partnerStore), filepath.Base(got))
	}
}

// Codex round three: the access token alone does not vouch for a refresh.
// An access token the record describes, paired with another session's
// refresh token, must not get the new token filed under the principal.
func TestRefreshWithAnotherSessionsRefreshTokenIsNotBound(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == cloudallyPartnerLoginPath:
			_, _ = w.Write([]byte(`{"accessToken":"partner-access","expiresIn":1893456000000,"refreshToken":"partner-refresh"}`))
		case r.URL.Path == cloudallyRefreshPath:
			_, _ = w.Write([]byte(`{"accessToken":"company-rotated","expiresIn":1893456000000,"refreshToken":"company-refresh-2"}`))
		case strings.HasSuffix(r.URL.Path, "/partner-access-token"):
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)

	login := func() string {
		t.Helper()
		if _, _, err := runAuthCmd(t, "", "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw"); err != nil {
			t.Fatalf("partner login: %v", err)
		}
		return currentStorePath()
	}
	// The partner access token, paired with a company refresh token.
	mixed := func() *config.Config {
		t.Helper()
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "partner-access", "company-refresh", time.Now().Add(10*time.Second)); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	assertNotPartner := func(where string) {
		t.Helper()
		cfg, _ := config.Load("")
		if cfg.AccessToken != "company-rotated" {
			t.Fatalf("%s: the token was not refreshed (%q)", where, cfg.AccessToken)
		}
		if id := cloudallyStoreScopeIdentity(cfg); id != "" {
			t.Fatalf("%s: the token minted from another session's refresh token resolved to %q", where, id)
		}
		if actx, ok := loadAuthContext(); ok && actx.PartnerAccessToken == "company-rotated" && actx.stashPrincipal(bindingFor(cfg)) != "" {
			t.Fatalf("%s: that token was kept as a verified partner session", where)
		}
	}

	partnerStore := login()
	cfg := mixed()
	// The client hook refuses outright (round five): see
	// TestMixedCredentialPairIsNeverRefreshed.
	if err := cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0)); err == nil {
		t.Fatal("the refresh hook refreshed a mixed credential pair")
	}
	if got := currentStorePath(); got != partnerStore {
		t.Fatalf("a refused refresh moved the store: %s", filepath.Base(got))
	}

	// use-account does not hold the store open, so it may refresh, but the
	// result must not be filed under the partner.
	login()
	mixed()
	if _, _, err := runAuthCmd(t, "", "auth", "use-account", "5f1c2a"); err == nil {
		t.Fatal("the mint should fail")
	}
	assertNotPartner("use-account")
	if got := currentStorePath(); got == partnerStore {
		t.Fatal("use-account: the token opens the partner store")
	}
}

// A binding names the API origin it was made under, so the same token under
// another API origin does not open that user's store. The API client id is
// not part of it: the same user with a rotated key pair keeps the store.
func TestBindingIsTiedToOriginNotKeyPair(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	bindSidecar(t, authScopePartner, "token-A", testPartner)
	id := cloudallyStoreScopeIdentity(scopeConfig(scopeAPI, "cid", "token-A"))
	if id == "" {
		t.Fatal("the bound token should resolve")
	}
	if got := cloudallyStoreScopeIdentity(scopeConfig(scopeAPI, "client-B", "token-A")); got != id {
		t.Fatalf("the same user under another key pair resolved to %q, want %q", got, id)
	}
	if got := cloudallyStoreScopeIdentity(scopeConfig("https://other-region.example.test", "cid", "token-A")); got != "" {
		t.Fatalf("the token resolved under another API origin: %q", got)
	}
}

// Codex rounds four and five: the root command picks (and a command such
// as restore watch may open) the store before the client refreshes. A
// mixed credential pair, an access token the auth record vouches for over
// another session's refresh token, is never refreshed, so the credential
// cannot change principal under an open store. A refresh nothing vouches
// for moves the rest of the run to the new token's own store.
func TestMixedCredentialPairIsNeverRefreshed(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	var refreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case cloudallyPartnerLoginPath:
			_, _ = w.Write([]byte(`{"accessToken":"partner-access","expiresIn":1893456000000,"refreshToken":"partner-refresh"}`))
		case cloudallyRefreshPath:
			refreshes++
			_, _ = w.Write([]byte(`{"accessToken":"rotated-` + strings.Repeat("x", refreshes) + `","expiresIn":1893456000000,"refreshToken":"rotated-refresh"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)

	if _, _, err := runAuthCmd(t, "", "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw"); err != nil {
		t.Fatalf("partner login: %v", err)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "partner-access", "company-refresh", time.Now().Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	partnerStore := currentStorePath()
	err = cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0))
	if err == nil || cliErrCode(t, err) != 4 {
		t.Fatalf("want an auth error for a mixed credential pair, got %v", err)
	}
	if refreshes != 0 {
		t.Fatalf("the refresh endpoint was called %d time(s)", refreshes)
	}
	reloaded, _ := config.Load("")
	if reloaded.AccessToken != "partner-access" || reloaded.RefreshToken != "company-refresh" {
		t.Fatalf("the credential changed: (%q, %q)", reloaded.AccessToken, reloaded.RefreshToken)
	}
	if got := defaultDBPath("cloudally-pp-cli"); got != partnerStore {
		t.Fatalf("the store moved: %s", filepath.Base(got))
	}

	// Nothing vouches for the token: the refresh goes ahead, and the rest
	// of the run moves to the new token's own store.
	if err := clearAuthContext(); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.Load("")
	before := currentStorePath()
	if err := cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0)); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	after := defaultDBPath("cloudally-pp-cli")
	if after == before {
		t.Fatalf("after an unvouched refresh the run kept the old token's store %s", filepath.Base(before))
	}
	if next := currentStorePath(); next != after {
		t.Fatalf("the run and the next run disagree: %s vs %s", filepath.Base(after), filepath.Base(next))
	}

	// A vouched refresh keeps the run on the principal's store.
	if _, _, err := runAuthCmd(t, "", "auth", "login", "--partner", "--email", "tech@example.com", "--password", "pw"); err != nil {
		t.Fatalf("partner login: %v", err)
	}
	cfg, _ = config.Load("")
	cfg.TokenExpiry = time.Now().Add(10 * time.Second)
	if got := currentStorePath(); got != partnerStore {
		t.Fatalf("re-login moved the partner store: %s", filepath.Base(got))
	}
	if err := cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0)); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got := defaultDBPath("cloudally-pp-cli"); got != partnerStore {
		t.Fatalf("a vouched refresh moved this run off the partner store: %s", filepath.Base(got))
	}
}

// Codex round six: with no auth record at all, an access token from
// CLOUDALLY_ACCESS_TOKEN is not refreshed with the stored session's refresh
// token, which would swap identities under a store already open for it.
func TestEnvironmentTokenIsNotRefreshedWithTheStoredSession(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	var refreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshes++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"stored-session-rotated","expiresIn":1893456000000,"refreshToken":"r2"}`))
	}))
	defer srv.Close()
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)

	stored, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := stored.SaveTokens("cid", "csecret", "stored-access-B", "stored-refresh-B", time.Now().Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "env-access-A")
	cfg, err := config.Load("")
	if err != nil || cfg.AccessToken != "env-access-A" || cfg.RefreshToken != "stored-refresh-B" {
		t.Fatalf("setup: (%q, %q), %v", cfg.AccessToken, cfg.RefreshToken, err)
	}
	before := currentStorePath()
	c := client.New(cfg, 5*time.Second, 0)
	if err := cloudallyRefreshHook(c); err != nil {
		t.Fatalf("refresh hook: %v", err)
	}
	if refreshes != 0 {
		t.Fatalf("the stored refresh token was spent on the environment token (%d call(s))", refreshes)
	}
	if c.Config.AccessToken != "env-access-A" {
		t.Fatalf("the active token changed to %q", c.Config.AccessToken)
	}
	if got := defaultDBPath("cloudally-pp-cli"); got != before {
		t.Fatalf("the store moved: %s -> %s", filepath.Base(before), filepath.Base(got))
	}

	// Without the environment token the stored session refreshes as before.
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "")
	cfg, _ = config.Load("")
	if err := cloudallyRefreshHook(client.New(cfg, 5*time.Second, 0)); err != nil {
		t.Fatalf("refresh hook: %v", err)
	}
	if refreshes != 1 {
		t.Fatalf("the stored session was not refreshed (%d call(s))", refreshes)
	}
}
