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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"crowdstrike-pp-cli/internal/client"
	"crowdstrike-pp-cli/internal/cliutil"
	"crowdstrike-pp-cli/internal/cliutil/testenv"
	"crowdstrike-pp-cli/internal/config"
)

// credRulesHome points every config, data, state, cache and credential path
// at a temp dir (CROWDSTRIKE_HOME, HOME and the XDG dirs) and clears every
// CrowdStrike credential and routing variable, so a Load, a mint or a token save
// in these tests never reaches the operator's own files or a real Falcon cloud.
func credRulesHome(t *testing.T) {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("CROWDSTRIKE_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	for _, k := range []string{"FALCON_CLIENT_ID", "FALCON_CLIENT_SECRET", "CROWDSTRIKE_BASE_URL", "CROWDSTRIKE_TOKEN_URL",
		"CROWDSTRIKE_OAUTH_SCOPE", "CROWDSTRIKE_CONFIG", "CROWDSTRIKE_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
	}
}

// fakeFalcon is a CrowdStrike Falcon cloud: the API at the root of its origin
// and the token endpoint at /oauth2/token on the same origin (POST, a form
// body grant_type=client_credentials&client_id=..&client_secret=..
// [&member_cid=..][&scope=..], no HTTP Basic header: CrowdStrike reads the
// pair from the form), answering {"access_token","token_type":"bearer",
// "expires_in":1799} for the one pair it knows. A request with member_cid is
// answered with a token minted for that child CID (access + "-" + the CID).
// api answers every other request.
type fakeFalcon struct {
	srv          *httptest.Server
	id, secret   string
	access       string
	mints, bad   atomic.Int32
	lastMintForm atomic.Value
	lastMintAuth atomic.Value
}

func newFakeFalcon(t *testing.T, id, secret, access string, api http.HandlerFunc) *fakeFalcon {
	t.Helper()
	m := &fakeFalcon{id: id, secret: secret, access: access}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			api(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		m.lastMintForm.Store(r.Header.Get("Content-Type") + " " + string(raw))
		m.lastMintAuth.Store(r.Header.Get("Authorization"))
		form, _ := url.ParseQuery(string(raw))
		if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") ||
			r.Header.Get("Authorization") != "" || form.Get("client_id") != id || form.Get("client_secret") != secret ||
			form.Get("grant_type") != "client_credentials" {
			m.bad.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"errors":[{"code":401,"message":"access denied, invalid client"}]}`)
			return
		}
		m.mints.Add(1)
		tok := access
		if cid := form.Get("member_cid"); cid != "" {
			tok = access + "-" + cid
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+tok+`","token_type":"bearer","expires_in":1799}`)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// falconList is a Falcon list response: the resources and the paging meta.
func falconList(resources ...string) string {
	return fmt.Sprintf(`{"meta":{"query_time":0.01,"pagination":{"offset":0,"limit":100,"total":%d},"trace_id":"trace-1"},"resources":[%s],"errors":[]}`,
		len(resources), strings.Join(resources, ","))
}

// api-origin-pinned: the minted Authorization (Bearer) reaches only the
// Falcon cloud the base URL names, a redirect to another host is refused
// before it is followed, and the configured cloud still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	const id, secret, tok = "pinUnitClientId-2d6a", "pinUnitClientSecret-8b1f4c", "pinUnitMintedToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	m := newFakeFalcon(t, id, secret, tok, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tok {
			t.Errorf("the API host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/devices/queries/devices/v1", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(falconList(`"dev-1"`)))
	})
	credRulesHome(t)
	t.Setenv("CROWDSTRIKE_BASE_URL", m.srv.URL)
	t.Setenv("FALCON_CLIENT_ID", id)
	t.Setenv("FALCON_CLIENT_SECRET", secret)
	t.Setenv("CROWDSTRIKE_NO_CONFIG_WRITE", "1")
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/devices/queries/devices/v1", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/redirect", nil)
	if err == nil || !strings.Contains(err.Error(), "leave the configured CrowdStrike host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 {
		t.Fatalf("want one accepted mint, got %d (refused %d)", m.mints.Load(), m.bad.Load())
	}
}

// api-origin-pinned, client-credentials mode: the token mint posts the client
// pair (in the form body) to <cloud>/oauth2/token on the pinned origin through
// the same guarded transport, so a token endpoint that redirects to another
// host (a 307 would resend the request and the form carrying the secret) is
// refused before the secret follows.
func TestMintNeverLeavesTheAPIOrigin(t *testing.T) {
	const id, secret = "pinMintClientId-3a7c", "pinMintClientSecret-9e2b4d"
	var otherHits atomic.Int32
	var otherSaw atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		b, _ := io.ReadAll(r.Body)
		otherSaw.Store(r.Header.Get("Authorization") + " " + string(b))
		_, _ = w.Write([]byte(`{"access_token":"stolenMint-1","token_type":"bearer","expires_in":1799}`))
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
		_, _ = w.Write([]byte(falconList()))
	}))
	t.Cleanup(api.Close)
	credRulesHome(t)
	t.Setenv("CROWDSTRIKE_BASE_URL", api.URL)
	t.Setenv("FALCON_CLIENT_ID", id)
	t.Setenv("FALCON_CLIENT_SECRET", secret)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), "/devices/queries/devices/v1", nil)
	// The token request follows no redirect at all (Codex r1 P2
	// token-redirect-deadlock): the redirect is the refused answer, and the
	// pin would refuse the other host anyway.
	if err == nil || !(strings.Contains(err.Error(), "leave the configured CrowdStrike host") || strings.Contains(err.Error(), "HTTP 307") || strings.Contains(err.Error(), "redirect")) {
		t.Fatalf("redirected mint: want the redirect refused, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("the refusal repeats the client secret: %v", err)
	}
	if tokenHits.Load() == 0 {
		t.Fatal("the mint never reached the cloud's token endpoint; the test did not run")
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s) (saw %v)", otherHits.Load(), otherSaw.Load())
	}
	if readHits.Load() != 0 {
		t.Fatalf("the read was sent %d time(s) without a minted token", readHits.Load())
	}
}

// With the token URL derived from the base URL (one Falcon cloud origin), the
// pin refuses every origin but that one: another host, another Falcon cloud
// (each is its own tenant), a scheme downgrade, another port and a non-ASCII
// spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	base := "https://api.eu-1.crowdstrike.com"
	pins := newCrowdStrikePins(base, client.ResolveCrowdStrikeTokenURL(&config.Config{BaseURL: base}))
	for raw, ok := range map[string]bool{
		"https://api.eu-1.crowdstrike.com/devices/queries/devices/v1":  true,
		"https://API.EU-1.CrowdStrike.com:443/oauth2/token":            true,
		"http://api.eu-1.crowdstrike.com/devices/queries/devices/v1":   false,
		"https://api.eu-1.crowdstrike.com:8443/oauth2/token":           false,
		"https://api.crowdstrike.com/oauth2/token":                     false, // US-1: another cloud
		"https://api.us-2.crowdstrike.com/devices/combined/devices/v1": false,
		"https://evil.example/oauth2/token":                            false,
		"https://\u0430pi.eu-1.crowdstrike.com/oauth2/token":           false, // Cyrillic a
	} {
		req, _ := http.NewRequest(http.MethodPost, raw, nil)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("id:secret")))
		if err := pins.check(req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.crowdstrike.com", nil)
	if (crowdstrikePins{}).check(req) == nil {
		t.Error("an empty pin (unparseable base and token URLs) must refuse everything")
	}
	if newCrowdStrikePins("::not a url", "::not a url").check(req) == nil {
		t.Error("pins from URLs that do not parse must refuse everything")
	}
}

// The pin is the outermost transport and sends through the credential kit's
// guard (cliutil.OutboundGuard), the response cache is off, a second hook run
// does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	saved := cliutil.OutboundGuard
	t.Cleanup(func() { cliutil.OutboundGuard = saved })
	cliutil.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return kitStandIn{rt} }
	c := &client.Client{BaseURL: "https://api.crowdstrike.com", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if _, ok := pin.next.(kitStandIn); !ok {
		t.Fatalf("the pin sends through %T, not the credential kit's guard", pin.next)
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
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/devices/entities/devices-actions/v2?action_name=contain", nil)
	if _, err := dry.HTTPClient.Do(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a --dry-run client sent a request")
	}
}

// kitStandIn marks where cliutil.OutboundGuard was applied.
type kitStandIn struct{ http.RoundTripper }

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://API.CrowdStrike.com/devices/queries/devices/v1": "https://api.crowdstrike.com:443",
		"https://api.crowdstrike.com:8443/x":                     "https://api.crowdstrike.com:8443",
		"https://api.laggar.gcw.crowdstrike.com/oauth2/token":    "https://api.laggar.gcw.crowdstrike.com:443",
		"http://127.0.0.1/x":                                     "http://127.0.0.1:80",
		"https://\u0430pi.crowdstrike.com/devices/x":             "",
		"https://[::1]:9443/devices/queries/devices/v1":          "https://[::1]:9443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// client-credentials-mint + output-chokepoint + token-request-body-credentials:
// the client pair is held the moment config loads; the access token the mint
// returns is held before any command reads it; the minted Bearer is what goes
// on the wire; the mint posted grant_type=client_credentials, client_id and
// client_secret as a form, with no HTTP Basic header and no scope unless
// CROWDSTRIKE_OAUTH_SCOPE names one, to <cloud>/oauth2/token on the API
// origin; and the token minted for an environment pair is written to no file
// at all (minted-token-bound-to-its-client: stored beside the files' pair it
// would read as that client's token), nor is the pair.
func TestMintedTokenIsHeldAndSent(t *testing.T) {
	const id, secret, access = "mintUnitClientId-2c4e6a8b", "mintUnitClientSecret-9f7d5b3a", "mintUnitAccess-6b1d9e3f7a"
	var got atomic.Value
	m := newFakeFalcon(t, id, secret, access, func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(falconList(`"dev-1"`)))
	})
	credRulesHome(t)
	t.Setenv("CROWDSTRIKE_BASE_URL", m.srv.URL)
	t.Setenv("FALCON_CLIENT_ID", id)
	t.Setenv("FALCON_CLIENT_SECRET", secret)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/devices/queries/devices/v1", nil); err != nil {
		t.Fatal(err)
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 {
		t.Fatalf("want one accepted mint, got %d (refused %d; last form %v, Authorization %q)", m.mints.Load(), m.bad.Load(), m.lastMintForm.Load(), m.lastMintAuth.Load())
	}
	ct, raw, _ := strings.Cut(fmt.Sprint(m.lastMintForm.Load()), " ")
	form, _ := url.ParseQuery(raw)
	if ct != "application/x-www-form-urlencoded" || form.Get("grant_type") != "client_credentials" || form.Get("client_id") != id ||
		form.Get("client_secret") != secret || form.Has("scope") || form.Has("member_cid") || m.lastMintAuth.Load() != "" {
		t.Fatalf("the mint did not post the documented form (the pair in the body, no Basic header, no scope): %v (Authorization %q)", m.lastMintForm.Load(), m.lastMintAuth.Load())
	}
	if got.Load() != "Bearer "+access {
		t.Fatalf("Authorization on the wire = %v, want the minted Bearer", got.Load())
	}
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(credPath); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("credential store mode %v, want 0600", info.Mode().Perm())
	}
	home := os.Getenv("CROWDSTRIKE_HOME")
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
	// CROWDSTRIKE_OAUTH_SCOPE is a setting, not a credential: when set it is
	// the form's scope, and the pair still rides in the body.
	t.Setenv("CROWDSTRIKE_OAUTH_SCOPE", "devices:read alerts:read")
	scoped := newFakeFalcon(t, id, secret, access+"-scoped", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(falconList()))
	})
	t.Setenv("CROWDSTRIKE_BASE_URL", scoped.srv.URL)
	c, err = flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/devices/queries/devices/v1", nil); err != nil {
		t.Fatal(err)
	}
	_, raw, _ = strings.Cut(fmt.Sprint(scoped.lastMintForm.Load()), " ")
	form, _ = url.ParseQuery(raw)
	if scoped.mints.Load() != 1 || form.Get("scope") != "devices:read alerts:read" || form.Get("client_secret") != secret {
		t.Fatalf("with CROWDSTRIKE_OAUTH_SCOPE set: want one mint whose form carries the scope and the pair, got %d mints, form %q", scoped.mints.Load(), raw)
	}
}
