// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/config"
	"zoom-phone-pp-cli/internal/store"
)

// zoom-phone hand-fix: zoom-phone-store-scope-account-id (class
// store-scope-identity, subclasses mint-transition and header-override, Codex
// review r2 of the onboard). The local store must be scoped to the credential
// that is actually sent; these drive the real `sync` against a server whose
// /phone/users answer depends on the bearer token.

// zoomExecute runs Execute exactly as main.go does, with os.Args set and the
// process's stdout and stderr captured in files.
func zoomExecute(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	dir := t.TempDir()
	outF, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	errF, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr, oldArgs := os.Stdout, os.Stderr, os.Args
	os.Stdout, os.Stderr = outF, errF
	os.Args = append([]string{"zoom-phone-cli"}, args...)
	runErr := Execute()
	os.Stdout, os.Stderr, os.Args = oldOut, oldErr, oldArgs
	_ = outF.Close()
	_ = errF.Close()
	stdout, _ := os.ReadFile(outF.Name())
	stderr, _ := os.ReadFile(errF.Name())
	return string(stdout), string(stderr), runErr
}

type zoomR2Tenant struct {
	srv  *httptest.Server
	mu   sync.Mutex
	seen []string // Authorization headers sent to /phone/users
}

func newZoomR2Tenant(t *testing.T) *zoomR2Tenant {
	t.Helper()
	tn := &zoomR2Tenant{}
	tn.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			_ = r.ParseForm()
			if r.PostForm.Get("account_id") != "acct-A" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"token-A-minted","expires_in":3600}`)
			return
		}
		if strings.TrimPrefix(r.URL.Path, "/v2") == "/phone/users" {
			auth := r.Header.Get("Authorization")
			tn.mu.Lock()
			tn.seen = append(tn.seen, auth)
			tn.mu.Unlock()
			user := map[string]string{"Bearer token-A-minted": "userA1", "Bearer token-B": "userB1"}[auth]
			if user == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"code":124,"message":"Invalid access token."}`)
				return
			}
			_, _ = fmt.Fprintf(w, `{"users":[{"id":%q,"email":"%s@example.com","name":%q}],"next_page_token":"","total_records":1}`, user, user, user)
			return
		}
		_, _ = io.WriteString(w, `{"next_page_token":""}`)
	}))
	t.Cleanup(tn.srv.Close)
	t.Setenv("ZOOM_PHONE_BASE_URL", tn.srv.URL+"/v2")
	t.Setenv("ZOOM_PHONE_TOKEN_URL", tn.srv.URL+"/oauth/token")
	return tn
}

func (tn *zoomR2Tenant) sent() []string {
	tn.mu.Lock()
	defer tn.mu.Unlock()
	return append([]string(nil), tn.seen...)
}

func zoomR2WriteConfig(t *testing.T, body string) string {
	t.Helper()
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// zoomR2StoreFor is the default store file a config with these values scopes to.
func zoomR2StoreFor(t *testing.T, cfg *config.Config) string {
	t.Helper()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "data-"+hashDefaultDBScope(cfg.StoreScopeCredential())+".db")
}

// zoomR2UsersByStore maps every store file in the data dir to the users rows it holds.
func zoomR2UsersByStore(t *testing.T) map[string][]string {
	t.Helper()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "data*.db"))
	out := map[string][]string{}
	for _, p := range paths {
		db, err := store.OpenReadOnly(p)
		if err != nil {
			continue
		}
		rows, err := db.DB().Query(`SELECT id FROM resources WHERE resource_type = 'users'`)
		if err == nil {
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					out[p] = append(out[p], id)
				}
			}
			_ = rows.Close()
		}
		_ = db.Close()
		sort.Strings(out[p])
	}
	return out
}

func zoomR2Base(tn *zoomR2Tenant) string { return tn.srv.URL + "/v2" }

func zoomR2MintedA(tn *zoomR2Tenant) *config.Config {
	a := &config.Config{BaseURL: zoomR2Base(tn), ClientID: "app-A", ZoomPhoneAccountId: "acct-A", AccessToken: "token-A-minted"}
	a.ZoomRecordMint("acct-A", "app-A", "token-A-minted")
	return a
}

// Round-2 input 1 (mint-transition), verbatim: a private config with valid
// account-A app credentials, access_token=B belonging to account B, and no
// expiry or mint record. `sync --full --resources users` opened B's
// credential-scoped store, then minted A's token and wrote A's rows into it.
func TestZoomR2MintTransitionNeverWritesIntoAnotherCredentialsStore(t *testing.T) {
	localTestEnv(t)
	tn := newZoomR2Tenant(t)
	zoomR2WriteConfig(t, "client_id = \"app-A\"\nclient_secret = \"secret-A-0123456789\"\nphone_account_id = \"acct-A\"\naccess_token = \"token-B\"\n")
	_, stderr, _ := zoomExecute(t, "sync", "--full", "--resources", "users", "--json")

	byStore := zoomR2UsersByStore(t)
	storeB := zoomR2StoreFor(t, &config.Config{BaseURL: zoomR2Base(tn), AccessToken: "token-B"})
	storeA := zoomR2StoreFor(t, zoomR2MintedA(tn))
	for _, id := range byStore[storeB] {
		if id == "userA1" {
			t.Fatalf("account A's rows were written into token B's store %s (stores: %v)\nstderr=%s", storeB, byStore, stderr)
		}
	}
	for p, ids := range byStore {
		if p != storeA && strings.Contains(strings.Join(ids, ","), "userA1") {
			t.Fatalf("account A's rows landed in %s, not A's minted store %s (stores: %v)", p, storeA, byStore)
		}
	}
}

// Round-2 input 2 (header-override), verbatim: an account-A config with valid
// app credentials and [headers] Authorization="Bearer B". The store was A's
// while every request carried B's token. A credential header override is
// refused with a clear error, and nothing is sent or written.
func TestZoomR2AuthorizationHeaderOverrideRefused(t *testing.T) {
	localTestEnv(t)
	tn := newZoomR2Tenant(t)
	zoomR2WriteConfig(t, "client_id = \"app-A\"\nclient_secret = \"secret-A-0123456789\"\nphone_account_id = \"acct-A\"\n\n[headers]\nAuthorization = \"Bearer token-B\"\n")
	stdout, stderr, err := zoomExecute(t, "sync", "--full", "--resources", "users", "--json")
	if err == nil {
		t.Fatalf("a config [headers] Authorization override was accepted\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if shown := stdout + stderr; !strings.Contains(strings.ToLower(shown), "authorization") {
		t.Fatalf("the refusal does not name the Authorization header:\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	for _, auth := range tn.sent() {
		if auth == "Bearer token-B" {
			t.Fatal("token B was sent")
		}
	}
	if byStore := zoomR2UsersByStore(t); len(byStore) != 0 {
		for p, ids := range byStore {
			if len(ids) > 0 {
				t.Fatalf("rows were written under a refused header override: %s %v", p, ids)
			}
		}
	}
}

// Round-1 input, still holding: a config naming app A and account A with a raw
// auth_header for tenant B (no secret, so nothing mints) keys B's rows by the
// credential, never into A's minted store.
func TestZoomR2RawHeaderStillKeysItsOwnStore(t *testing.T) {
	localTestEnv(t)
	tn := newZoomR2Tenant(t)
	zoomR2WriteConfig(t, "client_id = \"app-A\"\nphone_account_id = \"acct-A\"\nauth_header = \"Bearer token-B\"\n")
	_, stderr, _ := zoomExecute(t, "sync", "--full", "--resources", "users", "--json")
	byStore := zoomR2UsersByStore(t)
	storeA := zoomR2StoreFor(t, zoomR2MintedA(tn))
	if ids := byStore[storeA]; len(ids) > 0 {
		t.Fatalf("token B's rows entered A's minted store: %v\nstderr=%s", ids, stderr)
	}
	storeB := zoomR2StoreFor(t, &config.Config{BaseURL: zoomR2Base(tn), AuthHeaderVal: "Bearer token-B"})
	if got := strings.Join(byStore[storeB], ","); got != "userB1" {
		t.Fatalf("token B's store %s holds %q, want userB1 (stores: %v)\nstderr=%s", storeB, got, byStore, stderr)
	}
}

// The same class one branch over: a raw auth_header beside credentials that can
// mint. The client mints and clears the header, so token A is what gets sent;
// the store has to be A's, never one keyed by the header it just dropped.
func TestZoomR2RawHeaderBesideMintCredentialsUsesMintedStore(t *testing.T) {
	localTestEnv(t)
	tn := newZoomR2Tenant(t)
	zoomR2WriteConfig(t, "client_id = \"app-A\"\nclient_secret = \"secret-A-0123456789\"\nphone_account_id = \"acct-A\"\nauth_header = \"Bearer token-B\"\n")
	_, stderr, _ := zoomExecute(t, "sync", "--full", "--resources", "users", "--json")
	byStore := zoomR2UsersByStore(t)
	storeA := zoomR2StoreFor(t, zoomR2MintedA(tn))
	if got := strings.Join(byStore[storeA], ","); got != "userA1" {
		t.Fatalf("A's minted store %s holds %q, want userA1 (stores: %v)\nstderr=%s", storeA, got, byStore, stderr)
	}
	for _, auth := range tn.sent() {
		if auth == "Bearer token-B" {
			t.Fatal("token B was sent beside credentials that mint")
		}
	}
}

// And with a recorded expiry: a foreign token that is still inside its lifetime
// used to be sent until it expired, then the mint swapped the identity under a
// store already open on the foreign token's scope. A token this profile did not
// mint is replaced before the first request, so that switch cannot happen.
func TestZoomR2UnexpiredForeignTokenNeverSwitchesIdentityMidRun(t *testing.T) {
	localTestEnv(t)
	tn := newZoomR2Tenant(t)
	expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	zoomR2WriteConfig(t, "client_id = \"app-A\"\nclient_secret = \"secret-A-0123456789\"\nphone_account_id = \"acct-A\"\naccess_token = \"token-B\"\ntoken_expiry = "+expiry+"\n")
	_, stderr, _ := zoomExecute(t, "sync", "--full", "--resources", "users", "--json")
	byStore := zoomR2UsersByStore(t)
	storeA := zoomR2StoreFor(t, zoomR2MintedA(tn))
	if got := strings.Join(byStore[storeA], ","); got != "userA1" {
		t.Fatalf("A's minted store %s holds %q, want userA1 (stores: %v)\nstderr=%s", storeA, got, byStore, stderr)
	}
	for _, auth := range tn.sent() {
		if auth == "Bearer token-B" {
			t.Fatal("token B was sent beside credentials that mint")
		}
	}
}

// doctor proves a Server-to-Server profile that has minted nothing yet: the
// probe mints, so the verdict is about the app credentials themselves, and a
// rejected mint says so instead of leaving the credentials row blank.
func TestZoomDoctorProvesMintCredentials(t *testing.T) {
	for _, tc := range []struct {
		account, credentials, api string
	}{
		{"acct-A", "valid (verified with GET", "reachable"},
		// The reachability request is what mints, so a rejected mint stops there:
		// the api row names the token endpoint and credentials stays negative.
		{"acct-rejected", "skipped", "unreachable: Zoom S2S OAuth token mint"},
	} {
		t.Run(tc.account, func(t *testing.T) {
			localTestEnv(t)
			newZoomR2Tenant(t)
			t.Setenv("ZOOM_PHONE_ACCOUNT_ID", tc.account)
			t.Setenv("ZOOM_PHONE_CLIENT_ID", "app-A")
			t.Setenv("ZOOM_PHONE_CLIENT_SECRET", "secret-A-0123456789")
			stdout, stderr, _ := zoomExecute(t, "doctor", "--json")
			var report map[string]any
			if err := json.Unmarshal([]byte(stdout), &report); err != nil {
				t.Fatalf("doctor --json: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
			}
			if got, _ := report["credentials"].(string); !strings.HasPrefix(got, tc.credentials) {
				t.Fatalf("credentials = %q, want prefix %q", report["credentials"], tc.credentials)
			}
			if got, _ := report["api"].(string); !strings.HasPrefix(got, tc.api) {
				t.Fatalf("api = %q, want prefix %q", report["api"], tc.api)
			}
		})
	}
}
