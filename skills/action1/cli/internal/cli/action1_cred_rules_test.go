// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"action1-pp-cli/internal/client"
	"action1-pp-cli/internal/cliutil"
	"action1-pp-cli/internal/cliutil/testenv"
	"action1-pp-cli/internal/credkit"
)

// credRulesHome points every config, data, state, cache and credential path
// at a temp dir (ACTION1_HOME, HOME and the XDG dirs) and clears every
// Action1 credential variable, so a Load, a mint or a token save in these
// tests never reaches the operator's own files.
func credRulesHome(t *testing.T) {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("ACTION1_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	for _, k := range []string{"ACTION1_OAUTH2", "ACTION1_CLIENT_ID", "ACTION1_CLIENT_SECRET", "ACTION1_BASE_URL", "ACTION1_ORG_ID", "ACTION1_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
	}
}

// spyMinted replaces the cliutil.Hold no-op (TestMain) with a spy for the
// rest of t and returns every value held under a minted: source. The values
// the credential kit holds from startup (SecretEnv) are credkit.Start's, which
// no in-process test runs; the built binaries' leak tests prove those.
func spyMinted(t *testing.T) *[]string {
	t.Helper()
	held := &[]string{}
	restore := cliutil.Hold
	cliutil.Hold = func(src credkit.Source, vs ...string) {
		if strings.HasPrefix(string(src), "minted:") {
			*held = append(*held, vs...)
		}
	}
	t.Cleanup(func() { cliutil.Hold = restore })
	return held
}

// api-origin-pinned: the Authorization header (Bearer ACTION1_OAUTH2) reaches
// only the API host, a redirect to another host is refused before it is
// followed, and the configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const tok = "pinUnitAction1Oauth2-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`{"items":[],"total_items":0,"next_page":""}`))
	}))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tok {
			t.Errorf("the API host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/api/3.0/organizations", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"org-1","name":"Acme"}],"total_items":1,"next_page":""}`))
	}))
	t.Cleanup(api.Close)
	credRulesHome(t)
	t.Setenv("ACTION1_BASE_URL", api.URL+"/api/3.0")
	t.Setenv("ACTION1_OAUTH2", tok)
	held := spyMinted(t)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/organizations", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Action1 host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
	if !slices.Contains(*held, "Bearer "+tok) {
		t.Errorf("the built Authorization header is not held before it is sent")
	}
}

// api-origin-pinned, client-credentials mode: the token mint posts the client
// secret to <base URL>/oauth2/token on the pinned origin through the same
// guarded transport, so a token endpoint that redirects to another host
// (a 307 would resend the JSON body) is refused before the secret follows.
func TestMintNeverLeavesTheAPIOrigin(t *testing.T) {
	const id, secret = "pinMintClientId-3a7c", "pinMintClientSecret-9e2b4d"
	var otherHits atomic.Int32
	var otherSaw atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		b, _ := io.ReadAll(r.Body)
		otherSaw.Store(string(b))
		_, _ = w.Write([]byte(`{"access_token":"stolenMint-1","expires_in":3600}`))
	}))
	t.Cleanup(other.Close)
	var tokenHits, readHits atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/3.0/oauth2/token" {
			tokenHits.Add(1)
			http.Redirect(w, r, other.URL+"/oauth2/token", http.StatusTemporaryRedirect)
			return
		}
		readHits.Add(1)
		_, _ = w.Write([]byte(`{"items":[],"total_items":0,"next_page":""}`))
	}))
	t.Cleanup(api.Close)
	credRulesHome(t)
	t.Setenv("ACTION1_BASE_URL", api.URL+"/api/3.0")
	t.Setenv("ACTION1_CLIENT_ID", id)
	t.Setenv("ACTION1_CLIENT_SECRET", secret)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/organizations", nil)
	// The token request follows no redirect at all (Codex r1 P2
	// token-redirect-deadlock): the redirect is the refused answer, and the
	// pin would refuse the other host anyway.
	if err == nil || !(strings.Contains(err.Error(), "leave the configured Action1 host") || strings.Contains(err.Error(), "returned HTTP 307")) {
		t.Fatalf("redirected mint: want the redirect refused, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("the refusal repeats the client secret: %v", err)
	}
	if tokenHits.Load() == 0 {
		t.Fatal("the mint never reached the API origin's token endpoint; the test did not run")
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s) (body %v)", otherHits.Load(), otherSaw.Load())
	}
	if readHits.Load() != 0 {
		t.Fatalf("the read was sent %d time(s) without a minted token", readHits.Load())
	}
}

// The pin refuses every origin but the API base URL's: another host, another
// Action1 region (EU), a scheme downgrade, another port and a non-ASCII
// spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pin := normalizedOrigin(&url.URL{Scheme: "https", Host: "app.action1.com"})
	for raw, ok := range map[string]bool{
		"https://app.action1.com/api/3.0/organizations":      true,
		"https://APP.Action1.com:443/api/3.0/oauth2/token":   true,
		"http://app.action1.com/api/3.0/organizations":       false,
		"https://app.action1.com:8443/api/3.0":               false,
		"https://app.eu.action1.com/api/3.0/organizations":   false,
		"https://evil.example/api/3.0/oauth2/token":          false,
		"https://app.\u0430ction1.com/api/3.0/organizations": false, // Cyrillic a
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := checkPin(pin, req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://app.action1.com/api/3.0", nil)
	if checkPin("", req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
}

// The pin is the outermost transport and sends through the client's own (the
// credential kit's, in client.New), the response cache is off, a second hook
// run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	inner := &http.Transport{}
	c := &client.Client{BaseURL: "https://app.action1.com/api/3.0", HTTPClient: &http.Client{Transport: inner}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if pin.next != http.RoundTripper(inner) {
		t.Fatalf("the pin sends through %T, not the client's transport", pin.next)
	}
	if !c.NoCache {
		t.Fatal("the response cache is on")
	}
	guardClient(c)
	if again, ok := c.HTTPClient.Transport.(originPinTripper); !ok || again.next != pin.next {
		t.Fatal("a second guard run re-wrapped the transport")
	}
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	dry := &client.Client{BaseURL: srv.URL, HTTPClient: &http.Client{}, DryRun: true}
	guardClient(dry)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/organizations", nil)
	if _, err := dry.HTTPClient.Do(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a --dry-run client sent a request")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://APP.Action1.com/api/3.0":            "https://app.action1.com:443",
		"https://app.action1.com:8443/x":             "https://app.action1.com:8443",
		"https://app.au.action1.com/api/3.0":         "https://app.au.action1.com:443",
		"http://127.0.0.1/x":                         "http://127.0.0.1:80",
		"https://app.\u0430ction1.com/api/3.0/x":     "",
		"https://[::1]:9443/api/3.0/organizations/x": "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// client-credentials-mint + credkit-wiring: the access and refresh tokens the
// mint returns, and the Bearer header built from them, are held (minted:*)
// before any command reads them (the client pair itself is SecretEnv, held by
// credkit.Start); the minted Bearer is what goes on the wire; the mint posted the JSON pair to <base URL>/oauth2/token on the API
// origin; and the token minted for an environment pair is written to no file
// at all (minted-token-bound-to-its-client: stored beside the files' pair it
// would read as that client's token), nor is the pair.
func TestMintedTokenIsHeldAndSent(t *testing.T) {
	const id, secret = "mintUnitClientId-2c4e6a8b", "mintUnitClientSecret-9f7d5b3a"
	const access, refresh = "mintUnitAccess-6b1d9e3f7a", "mintUnitRefresh-4c8a2e6d0b"
	var got atomic.Value
	var mintBody atomic.Value
	var mints atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/3.0/oauth2/token" {
			mints.Add(1)
			b, _ := io.ReadAll(r.Body)
			mintBody.Store(r.Header.Get("Content-Type") + " " + string(b))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"` + access + `","refresh_token":"` + refresh + `","expires_in":3600,"token_type":"Bearer"}`))
			return
		}
		got.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"items":[{"id":"org-1","name":"Acme"}],"total_items":1,"next_page":""}`))
	}))
	t.Cleanup(api.Close)
	credRulesHome(t)
	t.Setenv("ACTION1_BASE_URL", api.URL+"/api/3.0")
	t.Setenv("ACTION1_CLIENT_ID", id)
	t.Setenv("ACTION1_CLIENT_SECRET", secret)
	held := spyMinted(t)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/organizations", nil); err != nil {
		t.Fatal(err)
	}
	if mints.Load() != 1 {
		t.Fatalf("want one mint, got %d", mints.Load())
	}
	var body map[string]any
	ct, raw, _ := strings.Cut(fmt.Sprint(mintBody.Load()), " ")
	if ct != "application/json" || json.Unmarshal([]byte(raw), &body) != nil || body["client_id"] != id || body["client_secret"] != secret {
		t.Fatalf("the mint did not post the JSON client pair: %v", mintBody.Load())
	}
	if got.Load() != "Bearer "+access {
		t.Fatalf("Authorization on the wire = %v, want the minted Bearer", got.Load())
	}
	for _, v := range []string{access, refresh, "Bearer " + access} {
		if !slices.Contains(*held, v) {
			t.Errorf("%q (minted) is not held", v)
		}
	}
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(credPath); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("credential store mode %v, want 0600", info.Mode().Perm())
	}
	home := os.Getenv("ACTION1_HOME")
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		for _, v := range []string{id, secret} {
			if bytes.Contains(b, []byte(v)) {
				t.Errorf("%s holds the environment client credential %q", path, v)
			}
		}
		if bytes.Contains(b, []byte(access)) || bytes.Contains(b, []byte(refresh)) {
			t.Errorf("%s holds the token minted for the environment pair", path)
		}
		return nil
	})
}
