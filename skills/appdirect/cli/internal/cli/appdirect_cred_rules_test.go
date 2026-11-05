// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"compress/zlib"
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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"appdirect-pp-cli/internal/client"
	"appdirect-pp-cli/internal/cliutil"
	"appdirect-pp-cli/internal/cliutil/testenv"
	"appdirect-pp-cli/internal/config"
)

// credRulesHome points every config, data, state, cache and credential path
// at a temp dir (APPDIRECT_HOME, HOME and the XDG dirs) and clears every
// AppDirect credential and routing variable, so a Load, a mint or a token save
// in these tests never reaches the operator's own files or AppDirect's own
// marketplace.
func credRulesHome(t *testing.T) {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("APPDIRECT_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	for _, k := range []string{"APPDIRECT_CLIENT_ID", "APPDIRECT_CLIENT_SECRET", "APPDIRECT_BASE_URL", "APPDIRECT_TOKEN_URL",
		"APPDIRECT_OAUTH_SCOPE", "APPDIRECT_CONFIG", "APPDIRECT_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
	}
}

// fakeMarketplace is an AppDirect marketplace: the API under /api and the
// token endpoint at /oauth2/token on the same origin (POST, a form body
// grant_type=client_credentials&scope=..., the client authenticated with HTTP
// Basic), answering {"access_token","expires_in"} for the one pair it knows.
// api answers every other request.
type fakeMarketplace struct {
	srv          *httptest.Server
	id, secret   string
	access       string
	mints, bad   atomic.Int32
	lastMintForm atomic.Value
}

func newFakeMarketplace(t *testing.T, id, secret, access string, api http.HandlerFunc) *fakeMarketplace {
	t.Helper()
	m := &fakeMarketplace{id: id, secret: secret, access: access}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			api(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		m.lastMintForm.Store(r.Header.Get("Content-Type") + " " + string(raw))
		form, _ := url.ParseQuery(string(raw))
		gotID, gotSecret, basic := r.BasicAuth()
		if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") ||
			!basic || gotID != id || gotSecret != secret || form.Get("grant_type") != "client_credentials" || form.Get("client_secret") != "" {
			m.bad.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
			return
		}
		m.mints.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+access+`","expires_in":3600}`)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// api-origin-pinned: the minted Authorization (Bearer) reaches only the
// marketplace the base URL names, a redirect to another host is refused before
// it is followed, and the configured marketplace still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const id, secret, tok = "pinUnitClientId-2d6a", "pinUnitClientSecret-8b1f4c", "pinUnitMintedToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	m := newFakeMarketplace(t, id, secret, tok, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tok {
			t.Errorf("the API host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/api/billing/v1/subscriptions", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"sub-1","status":"ACTIVE"}]`))
	})
	credRulesHome(t)
	t.Setenv("APPDIRECT_BASE_URL", m.srv.URL+"/api")
	t.Setenv("APPDIRECT_CLIENT_ID", id)
	t.Setenv("APPDIRECT_CLIENT_SECRET", secret)
	t.Setenv("APPDIRECT_NO_CONFIG_WRITE", "1")
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/billing/v1/subscriptions", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured AppDirect host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 {
		t.Fatalf("want one accepted mint, got %d (refused %d)", m.mints.Load(), m.bad.Load())
	}
	if !cliutil.HeldEchoes("x " + tok + " y") {
		t.Errorf("the minted token is not held")
	}
}

// api-origin-pinned, client-credentials mode: the token mint posts the client
// pair (HTTP Basic) to <marketplace>/oauth2/token on the pinned origin through
// the same guarded transport, so a token endpoint that redirects to another
// host (a 307 would resend the request and its Basic credentials) is refused
// before the secret follows.
func TestMintNeverLeavesTheAPIOrigin(t *testing.T) {
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
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			tokenHits.Add(1)
			http.Redirect(w, r, other.URL+"/oauth2/token", http.StatusTemporaryRedirect)
			return
		}
		readHits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(api.Close)
	credRulesHome(t)
	t.Setenv("APPDIRECT_BASE_URL", api.URL+"/api")
	t.Setenv("APPDIRECT_CLIENT_ID", id)
	t.Setenv("APPDIRECT_CLIENT_SECRET", secret)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/billing/v1/subscriptions", nil)
	// The token request follows no redirect at all (Codex r1 P2
	// token-redirect-deadlock): the redirect is the refused answer, and the
	// pin would refuse the other host anyway.
	if err == nil || !(strings.Contains(err.Error(), "leave the configured AppDirect host") || strings.Contains(err.Error(), "HTTP 307") || strings.Contains(err.Error(), "redirect")) {
		t.Fatalf("redirected mint: want the redirect refused, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("the refusal repeats the client secret: %v", err)
	}
	if tokenHits.Load() == 0 {
		t.Fatal("the mint never reached the marketplace's token endpoint; the test did not run")
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s) (saw %v)", otherHits.Load(), otherSaw.Load())
	}
	if readHits.Load() != 0 {
		t.Fatalf("the read was sent %d time(s) without a minted token", readHits.Load())
	}
}

// With the token URL derived from the base URL (one marketplace origin), the
// pin refuses every origin but that one: another host, another marketplace, a
// scheme downgrade, another port and a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	base := "https://marketplace.appdirect.com/api"
	pins := newAppDirectPins(base, client.ResolveAppDirectTokenURL(&config.Config{BaseURL: base}))
	for raw, ok := range map[string]bool{
		"https://marketplace.appdirect.com/api/account/v2/companies":  true,
		"https://MARKETPLACE.AppDirect.com:443/oauth2/token":          true,
		"http://marketplace.appdirect.com/api/account/v2/companies":   false,
		"https://marketplace.appdirect.com:8443/api":                  false,
		"https://reseller.appdirect.com/api/account/v2/companies":     false,
		"https://evil.example/oauth2/token":                           false,
		"https://m\u0430rketplace.appdirect.com/api/account/v2/users": false, // Cyrillic a
	} {
		req, _ := http.NewRequest(http.MethodPost, raw, nil)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("id:secret")))
		if err := pins.check(req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://marketplace.appdirect.com/api", nil)
	if (appdirectPins{}).check(req) == nil {
		t.Error("an empty pin (unparseable base and token URLs) must refuse everything")
	}
	if newAppDirectPins("::not a url", "::not a url").check(req) == nil {
		t.Error("pins from URLs that do not parse must refuse everything")
	}
}

// The pin is the outermost transport, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	c := &client.Client{BaseURL: "https://marketplace.appdirect.com/api", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if _, ok := pin.next.(responseHoldTripper); !ok {
		t.Fatalf("the pin sends through %T, not the response hold", pin.next)
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
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/account/v2/companies", nil)
	if _, err := dry.HTTPClient.Do(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a --dry-run client sent a request")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://MARKETPLACE.AppDirect.com/api":         "https://marketplace.appdirect.com:443",
		"https://marketplace.appdirect.com:8443/x":      "https://marketplace.appdirect.com:8443",
		"https://reseller.example.com/api":              "https://reseller.example.com:443",
		"http://127.0.0.1/x":                            "http://127.0.0.1:80",
		"https://m\u0430rketplace.appdirect.com/api/x":  "",
		"https://[::1]:9443/api/account/v2/companies/x": "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// response-credentials-held: a credential a response carries is held before
// any command reads the body, deflate-encoded or not, at any depth; masks and
// names that are not credentials are not. The POST answers the way a token
// endpoint does (access_token, and a refresh_token AppDirect may add), so a
// minted token is held by the transport as well as by the mint.
func TestResponseCredentialsHeld(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			_, _ = zw.Write([]byte(`{"access_token":"respDeflateKeyUnit77","refresh_token":"respDeflatePartnerUnit88","expires_in":3600,"token_type":"Bearer"}`))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(z.Bytes())
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"ds-1","client_secret":"respKeyUnitF6g7h8i9j0","script":{"parameters":{"access_token":"nestedTokenUnitA1b2c3"}},"password":"**********","name":"getNameNotSecretK2m4"}],"total_items":1,"next_page":""}`))
	}))
	t.Cleanup(srv.Close)
	rt := responseHoldTripper{next: http.DefaultTransport}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path := "/api/channel/v1/applications"
		if method == http.MethodPost {
			path = "/oauth2/token"
		}
		req, _ := http.NewRequest(method, srv.URL+path, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		if body, _ := io.ReadAll(resp.Body); len(body) == 0 {
			t.Fatalf("%s: body not handed on", method)
		}
	}
	for _, v := range []string{"respKeyUnitF6g7h8i9j0", "nestedTokenUnitA1b2c3", "respDeflateKeyUnit77", "respDeflatePartnerUnit88"} {
		if !cliutil.HeldEchoes("x " + v + " y") {
			t.Errorf("%s not held", v)
		}
	}
	for _, v := range []string{"getNameNotSecretK2m4", "**********"} {
		if cliutil.HeldEchoes(v) {
			t.Errorf("%s held, but it is not a credential", v)
		}
	}
}

func TestArgvCredentialsHeld(t *testing.T) {
	holdArgvCredentials(RootCmd(), []string{"auth", "login", "--client-id", "argvKeyUnit-5d2e8a", "--client-secret", "-argvSecretUnit-7c1e9a"})
	holdArgvCredentials(RootCmd(), []string{"account", "resource-company-read-company-get", "co-1042", "--token=argvTokenUnit-3b5d7f", "--select", "name"})
	// auth set-token takes no argument (stdin); a token typed positionally is
	// held before Cobra's refusal repeats it.
	holdArgvCredentials(RootCmd(), []string{"auth", "set-token", "--json", "posTokenUnit-6e2a9c4d"})
	holdArgvCredentials(RootCmd(), []string{"auth", "set-token", "--", "-posDashTokenUnit-1f8b3e"})
	for _, v := range []string{"argvKeyUnit-5d2e8a", "-argvSecretUnit-7c1e9a", "argvTokenUnit-3b5d7f", "posTokenUnit-6e2a9c4d", "-posDashTokenUnit-1f8b3e"} {
		if !cliutil.HeldEchoes(v) {
			t.Errorf("%s not held", v)
		}
	}
	for _, v := range []string{"co-1042", "--select"} {
		if cliutil.HeldEchoes(v) {
			t.Errorf("%s held, but it is not a credential", v)
		}
	}
}

// client-credentials-mint + output-chokepoint: the client pair is held the
// moment config loads; the access token the mint returns is held before any
// command reads it; the minted Bearer is what goes on the wire; the mint
// posted grant_type=client_credentials and the scope as a form, the pair as
// HTTP Basic, to <marketplace>/oauth2/token on the API origin; and the token
// minted for an environment pair is written to no file at all
// (minted-token-bound-to-its-client: stored beside the files' pair it would
// read as that client's token), nor is the pair.
func TestMintedTokenIsHeldAndSent(t *testing.T) {
	const id, secret, access = "mintUnitClientId-2c4e6a8b", "mintUnitClientSecret-9f7d5b3a", "mintUnitAccess-6b1d9e3f7a"
	var got atomic.Value
	m := newFakeMarketplace(t, id, secret, access, func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`[{"id":"sub-1","status":"ACTIVE"}]`))
	})
	credRulesHome(t)
	t.Setenv("APPDIRECT_BASE_URL", m.srv.URL+"/api")
	t.Setenv("APPDIRECT_CLIENT_ID", id)
	t.Setenv("APPDIRECT_CLIENT_SECRET", secret)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{id, secret} {
		if !cliutil.HeldEchoes("x " + v + " y") {
			t.Errorf("%q is not held after config load", v)
		}
	}
	if _, err := c.Get(context.Background(), "/billing/v1/subscriptions", nil); err != nil {
		t.Fatal(err)
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 {
		t.Fatalf("want one accepted mint, got %d (refused %d; last form %v)", m.mints.Load(), m.bad.Load(), m.lastMintForm.Load())
	}
	ct, raw, _ := strings.Cut(fmt.Sprint(m.lastMintForm.Load()), " ")
	form, _ := url.ParseQuery(raw)
	if ct != "application/x-www-form-urlencoded" || form.Get("grant_type") != "client_credentials" || form.Get("scope") != "ROLE_PARTNER ROLE_PARTNER_READ" ||
		strings.Contains(raw, secret) || strings.Contains(raw, id) {
		t.Fatalf("the mint did not post the documented form (the pair belongs in Basic, not the body): %v", m.lastMintForm.Load())
	}
	if got.Load() != "Bearer "+access {
		t.Fatalf("Authorization on the wire = %v, want the minted Bearer", got.Load())
	}
	for _, v := range []string{access, "Bearer " + access} {
		if !cliutil.HeldEchoes("x " + v + " y") {
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
	home := os.Getenv("APPDIRECT_HOME")
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
