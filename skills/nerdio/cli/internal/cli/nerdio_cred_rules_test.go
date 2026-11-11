// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/base64"
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

	"nerdio-pp-cli/internal/client"
	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/cliutil/testenv"
	"nerdio-pp-cli/internal/config"
	"nerdio-pp-cli/internal/credkit"
)

// credRulesHome points every config, data, state, cache and credential path
// at a temp dir (NERDIO_HOME, HOME and the XDG dirs) and clears every
// Nerdio credential and routing variable, so a Load, a mint or a token save
// in these tests never reaches the operator's own files, NMM instance or
// Entra tenant. NERDIO_OAUTH_SCOPE is the test scope (a setting every
// operator has).
func credRulesHome(t *testing.T) {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("NERDIO_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	for _, k := range []string{"NERDIO_CLIENT_ID", "NERDIO_CLIENT_SECRET", "NERDIO_BASE_URL", "NERDIO_TOKEN_URL",
		"NERDIO_CONFIG", "NERDIO_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
	}
	t.Setenv("NERDIO_OAUTH_SCOPE", nerdioTestScope)
}

// api-origin-pinned: the minted Authorization (Bearer) reaches only the
// NMM instance the base URL names, a redirect to another host is refused before
// it is followed, and the configured NMM instance still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	held := recordHolds(t)
	const id, secret, tok = "pinUnitClientId-2d6a", "pinUnitClientSecret-8b1f4c", "pinUnitMintedToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	m := newFakeInstance(t, id, secret, tok, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tok {
			t.Errorf("the API host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/rest-api/v1/accounts", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`[{"id":101,"name":"Contoso"}]`))
	})
	credRulesHome(t)
	m.useAs(t)
	t.Setenv("NERDIO_NO_CONFIG_WRITE", "1")
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/rest-api/v1/accounts", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/rest-api/v1/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured Nerdio host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 || m.other.Load() != 0 {
		t.Fatalf("want one accepted mint, got %d (refused %d, other requests on the token host %d)", m.mints.Load(), m.bad.Load(), m.other.Load())
	}
	if !slices.Contains(*held, tok) {
		t.Errorf("the minted token is not held")
	}
}

// api-origin-pinned, client-credentials mode: the token mint posts the client
// pair (in the form body) to NERDIO_TOKEN_URL (the Entra tenant, another origin than
// the NMM instance) through the same guarded transport, so a token endpoint
// that redirects to another host (a 307 would resend the request and its
// form body) is refused before the secret follows, and the read is
// never sent without a token.
func TestMintNeverLeavesTheTokenOrigin(t *testing.T) {
	const id, secret = "pinMintClientId-3a7c", "pinMintClientSecret-9e2b4d"
	var otherHits atomic.Int32
	var otherSaw atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		b, _ := io.ReadAll(r.Body)
		otherSaw.Store(r.Header.Get("Authorization") + " " + string(b))
		_, _ = w.Write([]byte(`{"access_token":"stolenMint-1","expires_in":3600}`))
	}))
	t.Cleanup(other.Close)
	var tokenHits, readHits atomic.Int32
	entra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenHits.Add(1)
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(entra.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		readHits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(api.Close)
	credRulesHome(t)
	t.Setenv("NERDIO_BASE_URL", api.URL)
	t.Setenv("NERDIO_TOKEN_URL", entra.URL+nerdioTestTokenPath)
	t.Setenv("NERDIO_CLIENT_ID", id)
	t.Setenv("NERDIO_CLIENT_SECRET", secret)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/rest-api/v1/accounts", nil)
	// The token request follows no redirect at all (Codex r1 P2
	// token-redirect-deadlock): the redirect is the refused answer, and the
	// pin would refuse the other host anyway.
	if err == nil || !(strings.Contains(err.Error(), "leave the configured Nerdio host") || strings.Contains(err.Error(), "HTTP 307") || strings.Contains(err.Error(), "redirect")) {
		t.Fatalf("redirected mint: want the redirect refused, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("the refusal repeats the client secret: %v", err)
	}
	if tokenHits.Load() == 0 {
		t.Fatal("the mint never reached the Entra token endpoint; the test did not run")
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s) (saw %v)", otherHits.Load(), otherSaw.Load())
	}
	if readHits.Load() != 0 {
		t.Fatalf("the read was sent %d time(s) without a minted token", readHits.Load())
	}
}

// The pins are exactly two origins, the NMM instance's (any request) and the
// Entra token URL's (the token POST only): another host, another NMM
// instance, another tenant host, a scheme downgrade, another port and a
// non-ASCII spelling of either host are refused.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	const base, tokenURL = "https://nmm.contoso.com", "https://login.microsoftonline.com/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a/oauth2/v2.0/token"
	pins := newNerdioPins(base, client.ResolveNerdioTokenURL(&config.Config{BaseURL: base, TokenURL: tokenURL}))
	for raw, ok := range map[string]bool{
		"https://nmm.contoso.com/rest-api/v1/accounts":                                             true,
		"https://NMM.Contoso.com:443/rest-api/v1/accounts/101/host-pool":                           true,
		"https://LOGIN.microsoftonline.com/7c1e9a3f-2b4d-4e6a-8c1f-3a5b7d9e1f2a/oauth2/v2.0/token": true,
		"http://nmm.contoso.com/rest-api/v1/accounts":                                              false,
		"https://nmm.contoso.com:8443/rest-api/v1/accounts":                                        false,
		"https://nmm.fabrikam.com/rest-api/v1/accounts":                                            false,
		"https://login.microsoftonline.com/other-tenant/oauth2/v2.0/token":                         false,
		"https://evil.example/oauth2/v2.0/token":                                                   false,
		"https://nmm.cont\u043eso.com/rest-api/v1/accounts":                                        false, // Cyrillic o
	} {
		req, _ := http.NewRequest(http.MethodPost, raw, nil)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("id:secret")))
		if err := pins.check(req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://nmm.contoso.com", nil)
	if (nerdioPins{}).check(req) == nil {
		t.Error("an empty pin (unparseable base and token URLs) must refuse everything")
	}
	if newNerdioPins("::not a url", "::not a url").check(req) == nil {
		t.Error("pins from URLs that do not parse must refuse everything")
	}
	// No token URL configured: the token host is pinned nowhere.
	noToken := newNerdioPins(base, client.ResolveNerdioTokenURL(&config.Config{BaseURL: base}))
	tok, _ := http.NewRequest(http.MethodPost, tokenURL, nil)
	if noToken.check(tok) == nil {
		t.Error("with no NERDIO_TOKEN_URL the Entra token endpoint must be refused")
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	c := &client.Client{BaseURL: "https://nmm.contoso.com", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if _, ok := pin.next.(tenantSecretTripper); !ok {
		t.Fatalf("the pin sends through %T, not the tenant-secret registration", pin.next)
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
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/rest-api/v1/accounts/101/host-pool", nil)
	if _, err := dry.HTTPClient.Do(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a --dry-run client sent a request")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://NMM.Contoso.com/rest-api":                        "https://nmm.contoso.com:443",
		"https://nmm.contoso.com:8443/x":                          "https://nmm.contoso.com:8443",
		"https://login.microsoftonline.com/t/oauth2/v2.0/token":   "https://login.microsoftonline.com:443",
		"http://127.0.0.1/x":                                      "http://127.0.0.1:80",
		"https://nmm.cont\u043eso.com/rest-api/x":                 "",
		"https://[::1]:9443/rest-api/v1/accounts/101/host-pool/x": "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// recordHolds records what the code under test hands the credential kit's
// Hold for the rest of the test (TestMain makes Hold a no-op).
func recordHolds(t *testing.T) *[]string {
	t.Helper()
	held := &[]string{}
	was := cliutil.Hold
	cliutil.Hold = func(_ credkit.Source, v ...string) { *held = append(*held, v...) }
	t.Cleanup(func() { cliutil.Hold = was })
	return held
}

// client-credentials-mint + credkit-wiring: the client pair is held the
// moment config loads; the access token the mint returns is held before any
// command reads it; the minted Bearer is what goes on the wire; the mint
// posted grant_type=client_credentials and NERDIO_OAUTH_SCOPE as a form, the
// pair in that form, to NERDIO_TOKEN_URL (the Entra tenant); and the token
// minted for an environment pair is written to no file at all
// (minted-token-bound-to-its-client: stored beside the files' pair it would
// read as that client's token), nor is the pair.
func TestMintedTokenIsHeldAndSent(t *testing.T) {
	held := recordHolds(t)
	const id, secret, access = "mintUnitClientId-2c4e6a8b", "mintUnitClientSecret-9f7d5b3a", "mintUnitAccess-6b1d9e3f7a"
	var got atomic.Value
	m := newFakeInstance(t, id, secret, access, func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`[{"id":101,"name":"Contoso"}]`))
	})
	credRulesHome(t)
	m.useAs(t)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{id, secret} {
		if !slices.Contains(*held, v) {
			t.Errorf("%q is not held after config load", v)
		}
	}
	if _, err := c.Get(context.Background(), "/rest-api/v1/accounts", nil); err != nil {
		t.Fatal(err)
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 {
		t.Fatalf("want one accepted mint, got %d (refused %d; last form %v)", m.mints.Load(), m.bad.Load(), m.lastMintForm.Load())
	}
	ct, raw, _ := strings.Cut(fmt.Sprint(m.lastMintForm.Load()), " ")
	form, _ := url.ParseQuery(raw)
	if ct != "application/x-www-form-urlencoded" || form.Get("grant_type") != "client_credentials" || form.Get("scope") != nerdioTestScope ||
		form.Get("client_id") != id || form.Get("client_secret") != secret || !strings.HasPrefix(raw, "grant_type=") {
		t.Fatalf("the mint did not post the documented form (the pair in the body, grant_type first): %v", m.lastMintForm.Load())
	}
	if got.Load() != "Bearer "+access {
		t.Fatalf("Authorization on the wire = %v, want the minted Bearer", got.Load())
	}
	for _, v := range []string{access, "Bearer " + access} {
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
	home := os.Getenv("NERDIO_HOME")
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
		if bytes.Contains(b, []byte(access)) {
			t.Errorf("%s holds the token minted for the environment pair", path)
		}
		return nil
	})
}
