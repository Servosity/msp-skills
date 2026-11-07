// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
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

	"kaseya-bms-pp-cli/internal/client"
	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/cliutil/testenv"
	"kaseya-bms-pp-cli/internal/config"
)

// kbmsCredentialVars are every Kaseya BMS credential and routing variable a
// test must clear before it sets its own.
var kbmsCredentialVars = []string{config.KbmsEnvUsername, config.KbmsEnvLoginVar, config.KbmsEnvTenant, "KASEYA_BMS_TOKEN",
	"KASEYA_BMS_BEARER_AUTH", "KASEYA_BMS_BASE_URL", "KASEYA_BMS_CONFIG", "KASEYA_BMS_CA_CERT", "KASEYA_BMS_TLS_SERVER_NAME"}

// credRulesHome points every config, data, state, cache and credential path
// at a temp dir (KASEYA_BMS_HOME, HOME and the XDG dirs) and clears every
// Kaseya BMS credential and routing variable, so a Load, a mint or a token
// save in these tests never reaches the operator's own files or a Kaseya BMS
// region.
func credRulesHome(t *testing.T) {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("KASEYA_BMS_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.Unsetenv("PRINTING_PRESS_VERIFY")
	for _, k := range kbmsCredentialVars {
		t.Setenv(k, "")
	}
}

// setKbmsLogin configures the BMS login in the environment, as the CLI and
// the MCP server read it.
func setKbmsLogin(t *testing.T, l config.KbmsLogin) {
	t.Helper()
	t.Setenv(config.KbmsEnvUsername, l.Username)
	t.Setenv(config.KbmsEnvLoginVar, l.Password)
	t.Setenv(config.KbmsEnvTenant, l.Tenant)
}

// kbmsAuthPath is the BMS login endpoint, on the API host.
const kbmsAuthPath = "/v2/security/authenticate"

// kbmsMint is one token request as a fake read it.
type kbmsMint struct {
	method, path, contentType, authorization string
	fields                                   map[string]string
	files                                    int
}

func readKbmsMint(r *http.Request) kbmsMint {
	m := kbmsMint{method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"),
		authorization: r.Header.Get("Authorization"), fields: map[string]string{}}
	if err := r.ParseMultipartForm(1 << 20); err == nil && r.MultipartForm != nil {
		for k, v := range r.MultipartForm.Value {
			m.fields[k] = strings.Join(v, ",")
		}
		m.files = len(r.MultipartForm.File)
	}
	return m
}

// valid reports the documented BMS login for l: POST /v2/security/authenticate
// as multipart/form-data with exactly UserName, Password, Tenant and
// GrantType=password, no file part and no Authorization header.
func (m kbmsMint) valid(l config.KbmsLogin) bool {
	mt, _, _ := mime.ParseMediaType(m.contentType)
	return m.method == http.MethodPost && m.path == kbmsAuthPath && mt == "multipart/form-data" && m.authorization == "" &&
		m.files == 0 && len(m.fields) == 4 && m.fields["UserName"] == l.Username && m.fields["Password"] == l.Password &&
		m.fields["Tenant"] == l.Tenant && m.fields["GrantType"] == "password"
}

// kbmsTokenAnswer is BMS's successful login answer (TokenOutputDto in the
// envelope).
func kbmsTokenAnswer(access, refresh string, expires time.Time) string {
	return fmt.Sprintf(`{"Success":true,"Error":null,"Result":{"AccessToken":%q,"RefreshToken":%q,"AccessTokenExpireOn":%q}}`,
		access, refresh, expires.UTC().Format(time.RFC3339))
}

// kbmsLoginRefused is BMS's answer to a login it does not know.
const kbmsLoginRefused = `{"Success":false,"Error":{"Code":978001,"Message":"Security Error","Details":"Invalid username or password"},"Result":null}`

// kbmsEnvelope wraps a record the way BMS answers a read.
func kbmsEnvelope(result string) string {
	return `{"Success":true,"Error":null,"Result":` + result + `}`
}

// fakeBMS is a Kaseya BMS host: the API under /v2 and the login endpoint at
// POST /v2/security/authenticate on the same origin (multipart/form-data
// UserName, Password, Tenant, GrantType=password, no Authorization header),
// answering TokenOutputDto in the BMS envelope for the one login it knows and
// {"Success":false,"Error":{"Code":978001,...}} with HTTP 401 for any other.
// api answers every other request.
type fakeBMS struct {
	srv             *httptest.Server
	login           config.KbmsLogin
	access, refresh string
	mints, bad      atomic.Int32
	lastMint        atomic.Value // kbmsMint
}

func newFakeBMS(t *testing.T, login config.KbmsLogin, access string, api http.HandlerFunc) *fakeBMS {
	t.Helper()
	m := &fakeBMS{login: login, access: access, refresh: access + "-refresh"}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != kbmsAuthPath {
			api(w, r)
			return
		}
		got := readKbmsMint(r)
		m.lastMint.Store(got)
		w.Header().Set("Content-Type", "application/json")
		if !got.valid(m.login) {
			m.bad.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, kbmsLoginRefused)
			return
		}
		m.mints.Add(1)
		_, _ = io.WriteString(w, kbmsTokenAnswer(m.access, m.refresh, time.Now().Add(time.Hour)))
	}))
	t.Cleanup(m.srv.Close)
	return m
}

// api-origin-pinned: the minted Authorization (Bearer) reaches only the host
// the base URL names, a redirect to another host is refused before it is
// followed, and the configured host still answers.
func TestOriginPinKeepsTheTokenOnTheAPIHost(t *testing.T) {
	inHoldingChild(t)
	login := config.KbmsLogin{Username: "pinUnitUser-2d6a", Password: "pinUnitPassword-8b1f4c", Tenant: "pinUnitTenant"}
	const tok = "pinUnitMintedToken-7e3a9c1f5b"
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		_, _ = w.Write([]byte(kbmsEnvelope(`[{"Id":1,"TicketNumber":"T-OTHER"}]`)))
	}))
	t.Cleanup(other.Close)
	m := newFakeBMS(t, login, tok, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tok {
			t.Errorf("the API host got Authorization %q", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/redirect") {
			http.Redirect(w, r, other.URL+"/v2/servicedesk/tickets", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(kbmsEnvelope(`[{"Id":1,"TicketNumber":"T20261029.0001","Title":"Pin MSP"}]`)))
	})
	credRulesHome(t)
	t.Setenv("KASEYA_BMS_BASE_URL", m.srv.URL)
	setKbmsLogin(t, login)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/v2/servicedesk/tickets", nil); err != nil {
		t.Fatalf("configured host: %v", err)
	}
	_, err = c.Get(context.Background(), "/v2/redirect", nil)
	if err == nil || !errors.Is(err, ErrOriginChanged) && !strings.Contains(err.Error(), "leave the configured Kaseya BMS host") {
		t.Fatalf("redirect: want the pin refusal, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("the other host was contacted %d time(s)", otherHits.Load())
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 {
		t.Fatalf("want one accepted mint, got %d (refused %d)", m.mints.Load(), m.bad.Load())
	}
	if !cliutil.Held("x " + tok + " y") {
		t.Errorf("the minted token is not held")
	}
}

// api-origin-pinned and mint-pinned-to-api-host, login mode: the mint posts
// the login form to <API host>/v2/security/authenticate through the API
// client's own guarded transport and follows no redirect at all, so a token
// endpoint that redirects (a 307 resends the request and its form, the
// password included) is refused before the form follows, to another host or
// to another path on the same host, and no API request goes out without a
// minted token.
func TestMintNeverLeavesTheAPIOrigin(t *testing.T) {
	inHoldingChild(t)
	login := config.KbmsLogin{Username: "pinMintUser-3a7c", Password: "pinMintPassword-9e2b4d", Tenant: "pinMintTenant"}
	var otherHits atomic.Int32
	var otherSaw atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		b, _ := io.ReadAll(r.Body)
		otherSaw.Store(r.Header.Get("Authorization") + " " + string(b))
		_, _ = w.Write([]byte(kbmsTokenAnswer("stolenMint-1", "stolenRefresh-1", time.Now().Add(time.Hour))))
	}))
	t.Cleanup(other.Close)
	for _, where := range []string{"another host", "another path on the API host"} {
		t.Run(where, func(t *testing.T) {
			otherHits.Store(0)
			var tokenHits, landingHits, readHits atomic.Int32
			var api *httptest.Server
			api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case kbmsAuthPath:
					tokenHits.Add(1)
					target := other.URL + kbmsAuthPath
					if where != "another host" {
						target = api.URL + "/v2/security/landing"
					}
					http.Redirect(w, r, target, http.StatusTemporaryRedirect)
				case "/v2/security/landing":
					landingHits.Add(1)
					_, _ = io.ReadAll(r.Body)
					_, _ = w.Write([]byte(kbmsTokenAnswer("landingMint-2", "landingRefresh-2", time.Now().Add(time.Hour))))
				default:
					readHits.Add(1)
					_, _ = w.Write([]byte(kbmsEnvelope(`[]`)))
				}
			}))
			t.Cleanup(api.Close)
			credRulesHome(t)
			t.Setenv("KASEYA_BMS_BASE_URL", api.URL)
			setKbmsLogin(t, login)
			flags := rootFlags{timeout: 10 * time.Second}
			c, err := flags.newClient()
			if err == nil {
				_, err = c.Get(context.Background(), "/v2/servicedesk/tickets", nil)
			}
			if err == nil || !(strings.Contains(err.Error(), "leave the configured Kaseya BMS host") || strings.Contains(err.Error(), "redirect")) {
				t.Fatalf("redirected mint: want the redirect refused, got %v", err)
			}
			if strings.Contains(err.Error(), login.Password) {
				t.Fatalf("the refusal repeats the password: %v", err)
			}
			if tokenHits.Load() == 0 {
				t.Fatal("the mint never reached the host's token endpoint; the test did not run")
			}
			if otherHits.Load() != 0 || landingHits.Load() != 0 {
				t.Fatalf("the redirect was followed: %d request(s) to the other host (saw %v), %d to the redirect path", otherHits.Load(), otherSaw.Load(), landingHits.Load())
			}
			if readHits.Load() != 0 {
				t.Fatalf("the read was sent %d time(s) without a minted token", readHits.Load())
			}
		})
	}
}

// The pin refuses every origin but the API base URL's (the token request is
// on the same host): another host, another BMS region, a scheme downgrade,
// another port and a non-ASCII spelling of the host.
func TestPinAllowsOnlyTheAPIOrigin(t *testing.T) {
	pins := newKbmsPins("https://api.bms.kaseya.com")
	for raw, ok := range map[string]bool{
		"https://api.bms.kaseya.com/v2/servicedesk/tickets":              true,
		"https://API.BMS.kaseya.com:443/v2/security/authenticate":        true,
		"http://api.bms.kaseya.com/v2/servicedesk/tickets":               false,
		"https://api.bms.kaseya.com:8443/v2/servicedesk/tickets":         false,
		"https://api.bmsemea.kaseya.com/v2/servicedesk/tickets":          false,
		"https://api.vorexlogin.kaseya.com/v2/security/authenticate":     false,
		"https://evil.example/v2/security/authenticate":                  false,
		"https://api.bms.kasеya.com/v2/servicedesk/tickets":              false, // Cyrillic e
		"https://api.bms.kaseya.com.evil.example/v2/servicedesk/tickets": false,
	} {
		req, _ := http.NewRequest(http.MethodPost, raw, nil)
		req.Header.Set("Authorization", "Bearer pinTableToken-1")
		if err := pins.check(req); (err == nil) != ok {
			t.Errorf("%s: allowed=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.bms.kaseya.com", nil)
	if (kbmsPins{}).check(req) == nil {
		t.Error("an empty pin (unparseable base URL) must refuse everything")
	}
	if newKbmsPins("::not a url").check(req) == nil {
		t.Error("a pin from a URL that does not parse must refuse everything")
	}
	if newKbmsPins("https://u:p@api.bms.kaseya.com").check(req) == nil {
		t.Error("a base URL with userinfo pins nothing")
	}
}

// The pin is the outermost transport, the kit is inside it, the response cache is off, a second
// hook run does not wrap the chain again, and under --dry-run nothing is sent.
func TestGuardWrapsTheTransport(t *testing.T) {
	c := &client.Client{BaseURL: "https://api.bms.kaseya.com", HTTPClient: &http.Client{}}
	guardClient(c)
	pin, ok := c.HTTPClient.Transport.(originPinTripper)
	if !ok {
		t.Fatalf("the origin pin is not outermost: %T", c.HTTPClient.Transport)
	}
	if _, ok := pin.next.(credentialResultTripper); !ok {
		t.Fatalf("the pin sends through %T, not the credential-result hold over the kit", pin.next)
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
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v2/servicedesk/tickets/1042", nil)
	if _, err := dry.HTTPClient.Do(req); !errors.Is(err, ErrDryRunSend) {
		t.Fatalf("dry run: want ErrDryRunSend, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("a --dry-run client sent a request")
	}
}

func TestNormalizedOriginRefusesNonASCIIAndFillsPorts(t *testing.T) {
	for raw, want := range map[string]string{
		"https://API.BMS.Kaseya.com":                                  "https://api.bms.kaseya.com:443",
		"https://api.bms.kaseya.com:8443/x":                           "https://api.bms.kaseya.com:8443",
		"https://api.bmsemea.kaseya.com/v2/servicedesk/tickets":       "https://api.bmsemea.kaseya.com:443",
		"http://127.0.0.1/x":                                          "http://127.0.0.1:80",
		"https://api.bms.kasеya.com/v2/x":                             "",
		"https://[::1]:9443/v2/servicedesk/tickets?PageNumber=1":      "https://[::1]:9443",
		"https://api.vorexlogin.kaseya.com/v2/security/authenticate":  "https://api.vorexlogin.kaseya.com:443",
		"https://api.bmsapac.kaseya.com:443/v2/security/refreshtoken": "https://api.bmsapac.kaseya.com:443",
	} {
		u, _ := url.Parse(raw)
		if got := normalizedOrigin(u); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// response-credentials-held is the kit's now (ResponseFields): proven on the
// built binaries by kbmsblackbox TestResponseCredentialFieldsWithheld.

// Argv credentials are the kit's now (CredentialFlags; auth set-token holds a
// positional token itself): proven on the built binaries by kbmsblackbox
// TestArgvCredentialsNeverPrint.

// token-minted-per-client + output-chokepoint: the login password is held
// the moment config loads; the access and refresh tokens the mint returns are
// held before any command reads them; the minted Bearer is what goes on the
// wire; the mint posted the documented multipart login (UserName, Password,
// Tenant, GrantType=password, no Authorization header) to
// <API host>/v2/security/authenticate on the API origin; a second client in
// the same process does not mint again (kbmsProcToken); and the minted token
// is kept only in the 0600 credential store, the password in no file at all.
func TestMintedTokenIsHeldAndSent(t *testing.T) {
	inHoldingChild(t)
	login := config.KbmsLogin{Username: "mintUnitUser-2c4e6a8b", Password: "mintUnitPassword-9f7d5b3a", Tenant: "mintUnitTenant"}
	const access = "mintUnitAccess-6b1d9e3f7a"
	var got atomic.Value
	m := newFakeBMS(t, login, access, func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(kbmsEnvelope(`[{"Id":1,"TicketNumber":"T20261029.0002","Title":"Mint MSP"}]`)))
	})
	credRulesHome(t)
	t.Setenv("KASEYA_BMS_BASE_URL", m.srv.URL)
	setKbmsLogin(t, login)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if !cliutil.Held("x " + login.Password + " y") {
		t.Errorf("the password is not held after config load")
	}
	if _, err := c.Get(context.Background(), "/v2/servicedesk/tickets", nil); err != nil {
		t.Fatal(err)
	}
	if m.mints.Load() != 1 || m.bad.Load() != 0 {
		t.Fatalf("want one accepted mint, got %d (refused %d; last request %+v)", m.mints.Load(), m.bad.Load(), m.lastMint.Load())
	}
	if mint, _ := m.lastMint.Load().(kbmsMint); !mint.valid(login) {
		t.Fatalf("the mint did not post the documented multipart login: %+v", mint)
	}
	if got.Load() != "Bearer "+access {
		t.Fatalf("Authorization on the wire = %v, want the minted Bearer", got.Load())
	}
	for _, v := range []string{access, "Bearer " + access, m.refresh} {
		if !cliutil.Held("x " + v + " y") {
			t.Errorf("%q (minted) is not held", v)
		}
	}
	// A second client in this process reuses the token it minted.
	c2, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c2.Get(context.Background(), "/v2/servicedesk/tickets", nil); err != nil {
		t.Fatal(err)
	}
	if m.mints.Load() != 1 {
		t.Fatalf("a second client in the same process minted again (%d mints)", m.mints.Load())
	}
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatalf("the minted token was not cached in the credential store: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("credential store mode %v, want 0600", info.Mode().Perm())
	}
	home := os.Getenv("KASEYA_BMS_HOME")
	var cached bool
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if bytes.Contains(b, []byte(login.Password)) {
			t.Errorf("%s holds the login password", path)
		}
		for _, v := range []string{access, m.refresh} {
			if bytes.Contains(b, []byte(v)) {
				if path != credPath {
					t.Errorf("%s holds a minted token; only the credential store may", path)
				}
				cached = true
			}
		}
		return nil
	})
	if !cached {
		t.Errorf("no file holds the minted token; the credential store did not cache it")
	}
}

// minted-token-bound-to-login-and-host: a token minted on one host is sent to
// no other, whatever else is configured. The credential store holds a token
// an earlier run minted on the EMEA host (its binding names that host and the
// login); the operator now points the CLI at host B with a pre-minted
// KASEYA_BMS_TOKEN for B. Every request to B carries B's token, never the
// EMEA one, and nothing mints.
func TestStoredTokenNeverReachesAnotherHost(t *testing.T) {
	inHoldingChild(t)
	const tokA, tokB = "boundTokenHostA-5e7a9c1d3f", "envTokenHostB-2b4d6f8a0c"
	var sawA, sawB atomic.Int32
	var mints atomic.Int32
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == kbmsAuthPath {
			mints.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, kbmsLoginRefused)
			return
		}
		switch r.Header.Get("Authorization") {
		case "Bearer " + tokA:
			sawA.Add(1)
		case "Bearer " + tokB:
			sawB.Add(1)
		}
		_, _ = w.Write([]byte(kbmsEnvelope(`[{"Id":1,"TicketNumber":"T-HOSTB"}]`)))
	}))
	t.Cleanup(b.Close)
	credRulesHome(t)
	hostA := "https://api.bmsemea.kaseya.com"
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(credPath), 0o700); err != nil {
		t.Fatal(err)
	}
	bound := config.KbmsTokenBinding(hostA, config.KbmsLogin{Username: "boundUserHostA-1111", Tenant: "boundTenant"})
	stored := "base_url = \"" + hostA + "\"\nclient_id = \"" + bound + "\"\naccess_token = \"" + tokA + "\"\n"
	if err := os.WriteFile(credPath, []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KASEYA_BMS_BASE_URL", b.URL)
	t.Setenv("KASEYA_BMS_TOKEN", tokB)
	flags := rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/v2/servicedesk/tickets", nil); err != nil {
		t.Fatal(err)
	}
	if sawA.Load() != 0 {
		t.Fatalf("host B received the token minted on host A (%d request(s))", sawA.Load())
	}
	if sawB.Load() != 1 || mints.Load() != 0 {
		t.Fatalf("want one request carrying KASEYA_BMS_TOKEN and no mint, got %d and %d mint(s)", sawB.Load(), mints.Load())
	}
}
