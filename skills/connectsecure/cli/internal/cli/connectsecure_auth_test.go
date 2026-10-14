// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectsecure-pp-cli/internal/client"
	"connectsecure-pp-cli/internal/cliutil"
	"connectsecure-pp-cli/internal/config"
)

func fakeJWT(exp time.Time) string {
	enc := base64.RawURLEncoding
	payload, _ := json.Marshal(map[string]int64{"exp": exp.Unix()})
	return enc.EncodeToString([]byte(`{"alg":"none"}`)) + "." + enc.EncodeToString(payload) + ".sig"
}

func TestConnectsecureMintInjectsBearerAndUserID(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()

	jwt := fakeJWT(time.Now().Add(2 * time.Hour))
	wantClientAuth := base64.StdEncoding.EncodeToString([]byte("acme+cid:sec"))
	var mints int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/w/authorize":
			atomic.AddInt32(&mints, 1)
			if r.Method != http.MethodPost || r.Header.Get("Client-Auth-Token") != wantClientAuth {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"status":true,"data":{"access_token":"` + jwt + `","user_id":"42"}}`))
		case "/r/company/companies":
			if r.Header.Get("Authorization") != "Bearer "+jwt || r.Header.Get("X-USER-ID") != "42" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"status":true,"data":[{"id":1,"name":"Acme Dental"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	newCfg := func() *config.Config {
		return &config.Config{BaseURL: srv.URL, ConnectsecureTenant: "acme", ClientID: "cid", ClientSecret: "sec"}
	}
	for i := 0; i < 2; i++ {
		c := client.New(newCfg(), 5*time.Second, 0)
		c.NoCache = true
		if err := connectsecureClientHook(c); err != nil {
			t.Fatalf("hook: %v", err)
		}
		raw, err := c.Get(context.Background(), "/r/company/companies", nil)
		if err != nil {
			t.Fatalf("companies call %d: %v", i, err)
		}
		if !strings.Contains(string(raw), "Acme Dental") {
			t.Fatalf("unexpected body %s", raw)
		}
	}
	if got := atomic.LoadInt32(&mints); got != 1 {
		t.Fatalf("expected 1 mint (second run reuses cache), got %d", got)
	}
}

func TestConnectsecureMintRejected(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":false,"message":"Invalid client credentials"}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, ConnectsecureTenant: "acme", ClientID: "cid", ClientSecret: "bad"}, 5*time.Second, 0)
	err = connectsecureClientHook(c)
	if err == nil || !strings.Contains(err.Error(), "Invalid client credentials") {
		t.Fatalf("want rejection error, got %v", err)
	}
	if c.Config.AccessToken != "" {
		t.Fatal("rejected mint must not set a token")
	}
}

func TestConnectsecurePreMintedTokenSkipsMint(t *testing.T) {
	c := client.New(&config.Config{BaseURL: "https://pod106.myconnectsecure.com", AccessToken: "tok", ConnectsecureUserId: "7", ConnectsecureTenant: "acme", ClientID: "cid", ClientSecret: "sec"}, time.Second, 0)
	if err := connectsecureClientHook(c); err != nil {
		t.Fatal(err)
	}
	if c.Config.AccessToken != "tok" || c.Config.ConnectsecureUserId != "7" {
		t.Fatal("pre-minted credentials must be left untouched")
	}
}

func TestConnectsecureBaseURLProblem(t *testing.T) {
	cases := map[string]bool{
		"https://your-pod.myconnectsecure.com": true,
		"https://portal.myconnectsecure.com":   true,
		"not a url":                            true,
		"https://pod106.myconnectsecure.com":   false,
		"https://pod1.connectsecure.com":       false,
		"http://127.0.0.1:8080":                false,
		"http://localhost:9000":                false,
		"http://pod106.myconnectsecure.com":    true,
	}
	for base, wantProblem := range cases {
		if got := connectsecureBaseURLProblem(base) != ""; got != wantProblem {
			t.Errorf("%q: problem=%v want %v", base, got, wantProblem)
		}
	}
	c := client.New(&config.Config{BaseURL: "https://your-pod.myconnectsecure.com", AccessToken: "tok"}, time.Second, 0)
	if err := connectsecureClientHook(c); err == nil || !strings.Contains(err.Error(), "CONNECTSECURE_BASE_URL") {
		t.Fatalf("placeholder base must fail with guidance, got %v", err)
	}
	c.DryRun = true
	if err := connectsecureClientHook(c); err != nil {
		t.Fatalf("dry-run must not be blocked by the base URL guard: %v", err)
	}
}

func TestConnectsecureTokenExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	if got := connectsecureTokenExpiry(fakeJWT(now.Add(10*time.Minute)), now); !got.Equal(now.Add(9 * time.Minute)) {
		t.Fatalf("short exp: got %v", got)
	}
	if got := connectsecureTokenExpiry(fakeJWT(now.Add(5*time.Hour)), now); !got.Equal(now.Add(connectsecureTokenCacheMax)) {
		t.Fatalf("long exp should cap: got %v", got)
	}
	if got := connectsecureTokenExpiry("opaque-token", now); !got.Equal(now.Add(connectsecureTokenCacheMax)) {
		t.Fatalf("opaque token should cap: got %v", got)
	}
}

func TestConnectsecureStoreIdentityIgnoresToken(t *testing.T) {
	a := &config.Config{BaseURL: "https://pod1.myconnectsecure.com/", ConnectsecureTenant: "acme", ClientID: "cid", AccessToken: "one"}
	b := &config.Config{BaseURL: "https://pod1.myconnectsecure.com", ConnectsecureTenant: "acme", ClientID: "cid", AccessToken: "two"}
	if connectsecureStoreIdentity(a) == "" || connectsecureStoreIdentity(a) != connectsecureStoreIdentity(b) {
		t.Fatal("store identity must be stable across re-minted tokens")
	}
}

func TestConnectsecureMintRefusesRedirectAndNeverForwardsSecret(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	var leaked int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Client-Auth-Token") != "" {
			atomic.AddInt32(&leaked, 1)
		}
		_, _ = w.Write([]byte(`{"status":true,"data":{"access_token":"x","user_id":"1"}}`))
	}))
	defer foreign.Close()
	for _, code := range []int{http.StatusFound, http.StatusTemporaryRedirect} {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, foreign.URL+"/w/authorize", code)
		}))
		c := client.New(&config.Config{BaseURL: origin.URL, ConnectsecureTenant: "acme", ClientID: "cid", ClientSecret: "s3cret"}, 5*time.Second, 0)
		// A plain client with Go's default redirect policy, so only the mint's
		// own guard stands between the secret and the redirect target.
		c.HTTPClient = &http.Client{Timeout: 5 * time.Second}
		err := connectsecureClientHook(c)
		origin.Close()
		if err == nil || !strings.Contains(err.Error(), "redirect") {
			t.Fatalf("HTTP %d: want a redirect config error, got %v", code, err)
		}
		if strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), connectsecureClientAuthToken("acme", "cid", "s3cret")) {
			t.Fatal("error message must not carry the secret")
		}
	}
	if atomic.LoadInt32(&leaked) != 0 {
		t.Fatal("Client-Auth-Token reached the redirect target")
	}
}

func TestConnectsecureMintResultExitCodes(t *testing.T) {
	cases := []struct {
		status int
		body   string
		code   int
	}{
		{http.StatusUnauthorized, ``, 4},
		{http.StatusForbidden, ``, 4},
		{http.StatusNotFound, ``, 10},
		{http.StatusFound, ``, 10},
		{http.StatusInternalServerError, `boom`, 5},
		{http.StatusOK, `{"status":false,"message":"bad"}`, 4},
	}
	for _, tc := range cases {
		_, err := connectsecureMintResult(tc.status, []byte(tc.body), "https://pod1.myconnectsecure.com")
		var ce *cliError
		if !errors.As(err, &ce) || ce.code != tc.code {
			t.Errorf("HTTP %d %q: want exit %d, got %v", tc.status, tc.body, tc.code, err)
		}
	}
}

func TestConnectsecureExpiredStoredTokenReMints(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	fresh := fakeJWT(time.Now().Add(time.Hour))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":true,"data":{"access_token":"` + fresh + `","user_id":"9"}}`))
	}))
	defer srv.Close()
	expired := fakeJWT(time.Now().Add(-time.Hour))
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: expired, ConnectsecureUserId: "7", ConnectsecureTenant: "acme", ClientID: "cid", ClientSecret: "sec"}, 5*time.Second, 0)
	if err := connectsecureClientHook(c); err != nil {
		t.Fatal(err)
	}
	if c.Config.AccessToken != fresh {
		t.Fatal("an expired stored JWT must be replaced by a fresh mint when client credentials are set")
	}
	if c.Config.ConnectsecureUserId != "9" {
		t.Fatalf("X-USER-ID must come from the fresh mint, got %q", c.Config.ConnectsecureUserId)
	}
	live := fakeJWT(time.Now().Add(time.Hour))
	c = client.New(&config.Config{BaseURL: srv.URL, AccessToken: live, ConnectsecureTenant: "acme", ClientID: "cid", ClientSecret: "sec"}, 5*time.Second, 0)
	if err := connectsecureClientHook(c); err != nil || c.Config.AccessToken != live {
		t.Fatalf("an unexpired pre-minted JWT must be kept: %v", err)
	}
}

func TestConnectsecureDefaultSyncIsKnownAndSafe(t *testing.T) {
	for _, name := range connectsecureDefaultSyncResources() {
		if _, err := syncResourcePath(name); err != nil {
			t.Errorf("default sync resource %q is not a sync resource: %v", name, err)
		}
		if cliutil.ConnectsecureSecretResource(name) {
			t.Errorf("default sync must not include secret resource %q", name)
		}
	}
	for cmdPath, resources := range readCommandResources {
		for _, r := range resources {
			if cliutil.ConnectsecureSecretResource(r) {
				t.Errorf("auto-refresh would pre-sync secret resource %q for %q", r, cmdPath)
			}
		}
	}
}

func TestConnectsecureMintOfflinePaths(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	// Unreachable pod: the hook defers to the request so --data-source auto can fall back.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	c := client.New(&config.Config{BaseURL: deadURL, ConnectsecureTenant: "acme", ClientID: "cid", ClientSecret: "sec"}, time.Second, 0)
	if err := connectsecureClientHook(c); err != nil || c.Config.AccessToken != "" {
		t.Fatalf("unreachable mint must defer, got err=%v token=%q", err, c.Config.AccessToken)
	}
	// --data-source local never mints.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) }))
	defer srv.Close()
	prev := connectsecureFlags
	connectsecureFlags = &rootFlags{dataSource: "local"}
	defer func() { connectsecureFlags = prev }()
	c = client.New(&config.Config{BaseURL: srv.URL, ConnectsecureTenant: "acme", ClientID: "cid", ClientSecret: "sec"}, time.Second, 0)
	if err := connectsecureClientHook(c); err != nil || atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("local reads must not mint: err=%v hits=%d", err, hits)
	}
}

// TestMintWithholdsAnEchoedCredential guards handfixes.json
// error-body-redacts-secrets on the token mint, which has its own HTTP path.
func TestMintWithholdsAnEchoedCredential(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusOK} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"status":false,"message":"bad header ` + r.Header.Get("Client-Auth-Token") + `"}`))
		}))
		cfg := &config.Config{BaseURL: srv.URL, ConnectsecureTenant: "acme", ClientID: "cid", ClientSecret: "CLIENTSECRETSENTINEL"}
		c := client.New(cfg, 5*time.Second, 0)
		_, err := connectsecureMint(context.Background(), c)
		cat := connectsecureClientAuthToken("acme", "cid", "CLIENTSECRETSENTINEL")
		if err == nil || strings.Contains(err.Error(), cat) || strings.Contains(err.Error(), "CLIENTSECRETSENTINEL") {
			t.Errorf("HTTP %d: mint error leaks the credential: %v", status, err)
		}
		srv.Close()
	}
}
