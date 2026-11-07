// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review r2 classes, each test taken from the reviewer's input and seen
// failing before its fix (hand-fixes credential-identity-one-resolver,
// refreshed-collection-replaces-inventory, inband-error-field-is-a-failure).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"datto-rmm-pp-cli/internal/config"
	"datto-rmm-pp-cli/internal/store"
)

// r2-1 credential identity mismatch: a config [headers] Authorization is sent
// beside a configured API key B.
func TestR2HeaderAuthorizationNeverStandsInForTheKey(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"id":1,"uid":"acct","name":"X"}`))
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	cfg := filepath.Join(home, "datto.toml")
	if err := os.WriteFile(cfg, []byte("[headers]\nAuthorization = \"Bearer accountAHeaderToken-77aa\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, _ = runDattoAt(t, home, map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_API_KEY": "keyB-r2-1", "DATTO_RMM_NO_CONFIG_WRITE": "1"},
		"--config", cfg, "account", "get-user", "--json")
	mu.Lock()
	defer mu.Unlock()
	for _, a := range seen {
		if a == "Bearer accountAHeaderToken-77aa" {
			t.Fatalf("a config header token was sent while API key B is configured")
		}
	}
}

// One credential is in effect, and it is the only Authorization on the wire,
// whatever the spelling of the header key. A config header with nothing more
// explicit configured still works (the other direction).
func TestR2OneBearerOnTheWire(t *testing.T) {
	const minted = "mintedForKeyB-r2-5c1d"
	cases := []struct {
		name, file string
		env        map[string]string
		want       string
	}{
		{"header A, DATTO_RMM_TOKEN B", "[headers]\nAuthorization = \"Bearer headerA-r2-1\"\n",
			map[string]string{"DATTO_RMM_TOKEN": "envTokenB-r2-1"}, "Bearer envTokenB-r2-1"},
		{"lower-case header A, DATTO_RMM_TOKEN B", "[headers]\nauthorization = \"Bearer headerA-r2-2\"\n",
			map[string]string{"DATTO_RMM_TOKEN": "envTokenB-r2-2"}, "Bearer envTokenB-r2-2"},
		{"header A, key B and its secret", "[headers]\nAuthorization = \"Bearer headerA-r2-3\"\n",
			map[string]string{"DATTO_RMM_API_KEY": "keyB-r2-3", "DATTO_RMM_API_SECRET_KEY": "secretB-r2-3"}, "Bearer " + minted},
		{"empty header, DATTO_RMM_TOKEN B", "[headers]\nAuthorization = \"\"\n",
			map[string]string{"DATTO_RMM_TOKEN": "envTokenB-r2-4"}, "Bearer envTokenB-r2-4"},
		{"header A alone", "[headers]\nAuthorization = \"Bearer headerA-r2-5\"\n",
			nil, "Bearer headerA-r2-5"},
		{"header A, stored auth_header", "auth_header = \"Bearer storedA-r2-6\"\n[headers]\nAuthorization = \"Bearer headerA-r2-6\"\n",
			nil, "Bearer headerA-r2-6"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var seen []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/auth/oauth/token") {
					_, _ = w.Write([]byte(`{"access_token":"` + minted + `","token_type":"bearer","expires_in":360000}`))
					return
				}
				mu.Lock()
				seen = append(seen, r.Header.Get("Authorization"))
				mu.Unlock()
				_, _ = w.Write([]byte(`{"id":1,"uid":"acct","name":"X"}`))
			}))
			t.Cleanup(srv.Close)
			home := t.TempDir()
			cfg := filepath.Join(home, "datto.toml")
			if err := os.WriteFile(cfg, []byte(tc.file), 0o600); err != nil {
				t.Fatal(err)
			}
			env := map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_NO_CONFIG_WRITE": "1"}
			for k, v := range tc.env {
				env[k] = v
			}
			if _, stderr, err := runDattoAt(t, home, env, "--config", cfg, "account", "get-user", "--json"); err != nil {
				t.Fatalf("%v\n%s", err, stderr)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(seen) == 0 {
				t.Fatal("no API request reached the server")
			}
			for _, a := range seen {
				if a != tc.want {
					t.Fatalf("Authorization %q reached the API, want %q", a, tc.want)
				}
			}
		})
	}
}

// r2-2 tenant store identity mismatch: DATTO_RMM_TOKEN B with an ambient key A
// keeps key A's store.
func TestR2StoreFollowsTheBearerInEffect(t *testing.T) {
	credRulesHome(t)
	t.Setenv("DATTO_RMM_API_KEY", "keyA-r2-2")
	t.Setenv("DATTO_RMM_API_SECRET_KEY", "secretA-r2-2")
	a, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATTO_RMM_TOKEN", "accountBToken-r2-2")
	b, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if a.StoreScopeCredential() == b.StoreScopeCredential() {
		t.Fatalf("the session sending DATTO_RMM_TOKEN B uses key A's store")
	}
}

// The store scope and the Authorization come from one choice, so two sessions
// share a store exactly when they act as one identity.
func TestR2StoreScopeNamesTheCredentialInEffect(t *testing.T) {
	load := func(t *testing.T, file string, env map[string]string) *config.Config {
		t.Helper()
		credRulesHome(t)
		for k, v := range env {
			t.Setenv(k, v)
		}
		p := filepath.Join(t.TempDir(), "datto.toml")
		if err := os.WriteFile(p, []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := config.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	scope := func(t *testing.T, file string, env map[string]string) string {
		t.Helper()
		return load(t, file, env).StoreScopeCredential()
	}
	const hdrA, hdrB = "[headers]\nAuthorization = \"Bearer headerA-r2\"\n", "[headers]\nAuthorization = \"Bearer headerB-r2\"\n"
	keyA := map[string]string{"DATTO_RMM_API_KEY": "scopeKeyA-r2", "DATTO_RMM_API_SECRET_KEY": "scopeSecretA-r2"}
	tokB := map[string]string{"DATTO_RMM_TOKEN": "scopeTokenB-r2"}
	tokBKeyA := map[string]string{"DATTO_RMM_TOKEN": "scopeTokenB-r2", "DATTO_RMM_API_KEY": "scopeKeyA-r2", "DATTO_RMM_API_SECRET_KEY": "scopeSecretA-r2"}

	if a, b := scope(t, hdrA, nil), scope(t, hdrB, nil); a == "" || a == b {
		t.Errorf("two config header identities share a store: %q %q", a, b)
	}
	if a, b := scope(t, hdrA, nil), scope(t, "", nil); a == b {
		t.Errorf("a config header identity shares the store of no credential: %q", a)
	}
	if a, b := scope(t, hdrA, keyA), scope(t, "", keyA); a != b {
		t.Errorf("a config header moved key A's store: %q %q", a, b)
	}
	if a, b := scope(t, hdrA, tokB), scope(t, "", tokB); a != b {
		t.Errorf("a config header moved token B's store: %q %q", a, b)
	}
	if a, b := scope(t, "", tokBKeyA), scope(t, "", tokB); a != b {
		t.Errorf("an ambient key moved token B's store: %q %q", a, b)
	}
	if a, b := scope(t, "", keyA), scope(t, "", tokB); a == b {
		t.Errorf("key A and token B share a store: %q", a)
	}
	for _, c := range []*config.Config{load(t, hdrA, keyA), load(t, hdrA, tokB), load(t, hdrA, nil)} {
		for k := range c.Headers {
			if strings.EqualFold(k, "Authorization") {
				t.Errorf("Authorization is still in the header map (AuthHeader set: %v)", c.AuthHeader() != "")
			}
		}
	}
	if got := load(t, hdrA, nil).AuthHeader(); got != "Bearer headerA-r2" {
		t.Errorf("header identity: AuthHeader = %q", got)
	}
}

// Naming the header as the credential in effect must not copy it into the
// credential store on a later save.
func TestR2HeaderIdentityStaysOutOfTheCredentialFile(t *testing.T) {
	credRulesHome(t)
	p := filepath.Join(t.TempDir(), "datto.toml")
	if err := os.WriteFile(p, []byte("[headers]\nAuthorization = \"Bearer headerPersist-r2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SaveTokens("", "", "savedAccess-r2", "", c.TokenExpiry); err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("DATTO_RMM_HOME")
	err = filepath.Walk(home, func(path string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() || path == p {
			return werr
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(data), "headerPersist-r2") {
			t.Errorf("%s holds the config header's credential", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// r2-3 refreshed collection keeps stale inventory: an empty documented
// SoftwarePage leaves the old software in place.
func TestR2SprawlEmptyRefreshReplacesTheOldList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"pageDetails":{"count":0,"totalCount":0,"prevPageUrl":null,"nextPageUrl":null},"software":[]}`))
	}))
	t.Cleanup(srv.Close)
	db := filepath.Join(t.TempDir(), "s.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = s.UpsertBatch(fleetDevicesResource, []json.RawMessage{json.RawMessage(`{"id":1,"uid":"d1","hostname":"PC1"}`)})
	_ = s.Upsert(fleetSoftwareResource, "d1", json.RawMessage(`{"software":[{"name":"OldApp","version":"1"}]}`))
	s.Close()
	stdout, stderr, err := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "sprawlR2-1"},
		"fleet", "sprawl", "--refresh", "--json", "--db", db)
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	var view []sprawlView
	_ = json.Unmarshal([]byte(stdout), &view)
	if len(view) != 0 {
		t.Fatalf("a refresh that found no software still reports %+v", view)
	}
}

// The class: a refresh states what is on each device now. An uninstalled app
// goes, a shrunk record loses its old fields, a device the fleet no longer has
// goes, and a device whose read failed keeps its last list (reported).
func TestR2SprawlRefreshStatesTheCurrentInventory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/device/d1/"):
			_, _ = w.Write([]byte(`{"pageDetails":{"nextPageUrl":null},"software":[{"name":"Kept","version":"2"}]}`))
		case strings.Contains(r.URL.Path, "/device/d2/"):
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	db := filepath.Join(t.TempDir(), "s.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = s.UpsertBatch(fleetDevicesResource, []json.RawMessage{
		json.RawMessage(`{"id":1,"uid":"d1","hostname":"PC1"}`), json.RawMessage(`{"id":2,"uid":"d2","hostname":"PC2"}`)})
	_ = s.Upsert(fleetSoftwareResource, "d1", json.RawMessage(`{"software":[{"name":"Kept","version":"1"},{"name":"Uninstalled","version":"1"}]}`))
	_ = s.Upsert(fleetSoftwareResource, "d2", json.RawMessage(`{"software":[{"name":"LastKnown","version":"1"}]}`))
	_ = s.Upsert(fleetSoftwareResource, "gone", json.RawMessage(`{"software":[{"name":"RetiredPC","version":"1"}]}`))
	s.Close()
	stdout, stderr, err := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "sprawlR2-2"},
		"fleet", "sprawl", "--refresh", "--json", "--db", db)
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	var view []sprawlView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	got := map[string]bool{}
	for _, v := range view {
		got[v.Name+"/"+v.Version] = true
	}
	want := map[string]bool{"Kept/2": true, "LastKnown/1": true}
	if len(got) != len(want) || !got["Kept/2"] || !got["LastKnown/1"] {
		t.Fatalf("inventory after refresh = %v, want %v", got, want)
	}
	if !strings.Contains(stderr, "d2") {
		t.Fatalf("the failed device is not reported: %q", stderr)
	}
}

// A refresh that read nothing proves nothing about removed devices: with no
// synced device table nothing is dropped.
func TestR2SprawlRefreshWithNoDeviceTableDropsNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	db := filepath.Join(t.TempDir(), "s.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Upsert(fleetSoftwareResource, "d1", json.RawMessage(`{"software":[{"name":"Known","version":"1"}]}`))
	s.Close()
	stdout, stderr, err := runDatto(t, map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "sprawlR2-3"},
		"fleet", "sprawl", "--refresh", "--json", "--db", db)
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	var view []sprawlView
	_ = json.Unmarshal([]byte(stdout), &view)
	if len(view) != 1 || view[0].Name != "Known" {
		t.Fatalf("inventory = %+v", view)
	}
}

// A replaced record is still scrubbed of credential fields on its way in.
func TestR2ReplaceKeepsTheStoreSecretRule(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.ReplaceResource(fleetSoftwareResource, "d1", json.RawMessage(`{"software":[],"apiSecretKey":"replaceSecret-r2-9f"}`)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(fleetSoftwareResource, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "replaceSecret-r2-9f") {
		t.Fatalf("a credential field reached the store: %s", got)
	}
}

// r2-4 incomplete sync reported healthy: ActivityLogsPage.error (a documented
// in-band failure) is synced as a complete read.
func TestR2InBandErrorIsAGap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"pageDetails":{"count":0,"totalCount":0,"prevPageUrl":null,"nextPageUrl":null},"activities":[],"error":"database timeout"}`))
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	db := filepath.Join(home, "g.db")
	env := map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "gapsR2-1"}
	_, _, _ = runDattoAt(t, home, env, "sync", "--resources", "activity-logs", "--json", "--db", db)
	stdout, _, err := runDattoAt(t, home, env, "fleet", "sync-gaps", "--json", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	var view syncGapsView
	_ = json.Unmarshal([]byte(stdout), &view)
	for _, r := range view.Resources {
		if r.Resource == "activity-logs" && r.Status == "ok" {
			t.Fatalf("an activity read Datto reported failed (error field) reads as ok: %+v", r)
		}
	}
}

// Both directions on every page: a populated error fails the read whatever
// else the page says, a blank one is a normal page.
func TestR2ErrorFieldWinsOnEveryPage(t *testing.T) {
	cases := []struct {
		name, tail string
		wantStatus string
	}{
		{"populated", `,"error":"database timeout"`, "incomplete"},
		{"populated beside success true", `,"success":true,"error":"database timeout"`, "incomplete"},
		{"populated object", `,"error":{"code":500}`, "incomplete"},
		{"null", `,"error":null`, "ok"},
		{"empty", `,"error":""`, "ok"},
		{"absent", ``, "ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			second := false
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if !second {
					second = true
					next := srv.URL + "/api/v2/activity-logs?page=abc"
					_, _ = w.Write([]byte(`{"pageDetails":{"count":1,"nextPageUrl":"` + next + `"},"activities":[{"id":"a1"}]}`))
					return
				}
				_, _ = w.Write([]byte(`{"pageDetails":{"count":1,"nextPageUrl":null},"activities":[{"id":"a2"}]` + tc.tail + `}`))
			}))
			t.Cleanup(srv.Close)
			home := t.TempDir()
			db := filepath.Join(home, "g.db")
			env := map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "gapsR2-2"}
			_, _, _ = runDattoAt(t, home, env, "sync", "--resources", "activity-logs", "--json", "--db", db)
			stdout, _, err := runDattoAt(t, home, env, "fleet", "sync-gaps", "--json", "--db", db)
			if err != nil {
				t.Fatal(err)
			}
			var view syncGapsView
			if err := json.Unmarshal([]byte(stdout), &view); err != nil {
				t.Fatalf("%v\n%s", err, stdout)
			}
			found := false
			for _, r := range view.Resources {
				if r.Resource == "activity-logs" {
					found = true
					if r.Status != tc.wantStatus {
						t.Fatalf("status = %q, want %q (%+v)", r.Status, tc.wantStatus, r)
					}
				}
			}
			if !found {
				t.Fatalf("activity-logs is missing from %s", stdout)
			}
		})
	}
}

// A page that reports a failure fails every read that would otherwise keep
// only its records: `--all`, export (walked and limited) and the novel walker.
func TestR2ErrorFieldFailsEveryListRead(t *testing.T) {
	const bad = `{"pageDetails":{"count":1,"nextPageUrl":null},"activities":[{"id":"a1"}],"error":"database timeout"}`
	const clean = `{"pageDetails":{"count":1,"nextPageUrl":null},"activities":[{"id":"a1"}]}`
	var failing atomic.Bool
	failing.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			_, _ = w.Write([]byte(bad))
			return
		}
		_, _ = w.Write([]byte(clean))
	}))
	t.Cleanup(srv.Close)
	env := map[string]string{"DATTO_RMM_BASE_URL": srv.URL + "/api", "DATTO_RMM_TOKEN": "gapsR2-3"}
	for _, args := range [][]string{
		{"activity-logs", "--all", "--json"},
		{"export", "activity-logs", "--format", "json"},
		{"export", "activity-logs", "--format", "json", "--limit", "1"},
	} {
		home := t.TempDir()
		stdout, stderr, err := runDattoAt(t, home, env, args...)
		if err == nil {
			t.Errorf("%v: exit 0 with the page's failure dropped\nstdout=%s\nstderr=%s", args, stdout, stderr)
		}
		// The other direction: the same read of a clean page succeeds.
		failing.Store(false)
		if _, stderr, err := runDattoAt(t, t.TempDir(), env, args...); err != nil {
			t.Errorf("%v: a clean page fails: %v\n%s", args, err, stderr)
		}
		failing.Store(true)
	}
}
