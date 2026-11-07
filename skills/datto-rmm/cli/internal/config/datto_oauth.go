// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Datto RMM composed-OAuth2 token mint and platform routing (hand-authored).
// Datto RMM does not take API keys on requests: the API key + API secret key
// are exchanged for a bearer token through an OAuth2 password grant against a
// fixed public client, on the same host as the API.
//
// datto-rmm handfixes (4.34.2 reprint):
//   - platform-routes-every-request: DATTO_RMM_PLATFORM (or DATTO_RMM_API_URL)
//     chooses the base URL inside Load, wherever config.go reads
//     DATTO_RMM_BASE_URL, so every request, the credential-origin checks and the
//     mint move together. The 4.24 print applied the platform only in
//     `auth login`, so a token minted on one platform was sent to the default.
//   - minted-token-bound-to-key-and-host: a minted token is recorded with
//     the host it was minted on and a fingerprint of the API key that minted
//     it (DattoTokenBinding, kept in the 0600 credential store as client_id).
//     A token is sent only to that host and, while an API key is configured,
//     only for that key; anything else mints again.
//   - mint-pinned-to-api-host: the token request goes only to
//     <API host>/auth/oauth/token over a client that follows no redirect; the
//     error text never repeats the server's answer.
//   - mint-refuses-plaintext: never over plaintext http but to a loopback
//     host (dattoPlaintextRefused).
package config

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"datto-rmm-pp-cli/internal/cliutil"
	"datto-rmm-pp-cli/internal/credkit"
)

// dattoPlatformURLs maps each Datto RMM regional platform to its API host.
// An MSP account lives on exactly one platform.
var dattoPlatformURLs = map[string]string{
	"pinotage":  "https://pinotage-api.centrastage.net",
	"merlot":    "https://merlot-api.centrastage.net",
	"concord":   "https://concord-api.centrastage.net",
	"vidal":     "https://vidal-api.centrastage.net",
	"zinfandel": "https://zinfandel-api.centrastage.net",
	"syrah":     "https://syrah-api.centrastage.net",
}

// DattoPlatformBaseURL is the API base URL the operator chose with
// DATTO_RMM_API_URL (the full base, host root or .../api) or DATTO_RMM_PLATFORM,
// "" when neither is set. An unresolved .mcpb placeholder counts as unset.
func DattoPlatformBaseURL() (string, error) {
	if v := strings.TrimSpace(cliutil.EnvOverride("DATTO_RMM_API_URL")); v != "" {
		b := strings.TrimRight(v, "/")
		if !strings.HasSuffix(b, "/api") {
			b += "/api"
		}
		return b, nil
	}
	plat := strings.ToLower(strings.TrimSpace(cliutil.EnvOverride("DATTO_RMM_PLATFORM")))
	if plat == "" {
		return "", nil
	}
	base, ok := dattoPlatformURLs[plat]
	if !ok {
		return "", fmt.Errorf("unknown DATTO_RMM_PLATFORM %q (valid: pinotage, merlot, concord, vidal, zinfandel, syrah)", plat)
	}
	return base + "/api", nil
}

// checkDattoPlatform refuses an unknown platform before anything loads.
func checkDattoPlatform() error {
	_, err := DattoPlatformBaseURL()
	return err
}

// dattoBaseURLEnv is the base URL the environment chose explicitly:
// DATTO_RMM_BASE_URL, else the platform choice. config.go reads it wherever it
// read DATTO_RMM_BASE_URL (handfix platform-routes-every-request).
func dattoBaseURLEnv() string {
	if v := cliutil.EnvOverride("DATTO_RMM_BASE_URL"); v != "" {
		return v
	}
	v, _ := DattoPlatformBaseURL()
	return v
}

// DattoOrigin is scheme://host:port of a URL with ASCII letters lowercased and
// the default port filled in; "" for anything that is not an http(s) URL with
// an ASCII host and no userinfo (fail closed: "" never matches).
func DattoOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User != nil || u.Host == "" {
		return ""
	}
	for i := 0; i < len(u.Host); i++ {
		if u.Host[i] >= 0x80 {
			return ""
		}
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	switch scheme {
	case "https":
		if port == "" {
			port = "443"
		}
	case "http":
		if port == "" {
			port = "80"
		}
	default:
		return ""
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

// DattoTokenBinding names what a minted token belongs to: 32 lowercase hex
// digits, a fingerprint of the API host then one of the API key (never the
// key; no separator, so no part of it reads as a word or a host). "" when
// either is missing.
func DattoTokenBinding(baseURL, apiKey string) string {
	origin := dattoOriginFingerprint(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if origin == "" || apiKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("datto-rmm api key\x00" + apiKey))
	return origin + hex.EncodeToString(sum[:])[:16]
}

func dattoOriginFingerprint(baseURL string) string {
	origin := DattoOrigin(baseURL)
	if origin == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("datto-rmm api host\x00" + origin))
	return hex.EncodeToString(sum[:])[:16]
}

// dattoBindingOrigin is the host fingerprint a recorded binding names ("" when
// it is not a binding).
func dattoBindingOrigin(binding string) string {
	if len(binding) != 32 {
		return ""
	}
	for i := 0; i < len(binding); i++ {
		c := binding[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return ""
		}
	}
	return binding[:16]
}

// dattoStoreScope keeps one local store per identity (handfix
// credential-identity-one-resolver): the scope comes from the credential
// dattoCredentialInEffect picks, the same choice the Authorization comes from.
// A key, and a stored token bound to a key, keep one store across re-mints
// (every ~100h, or every run when nothing is cached); the generated scope hashes
// the bearer itself, which is right for every other credential: "" hands it
// that bearer, so DATTO_RMM_TOKEN B beside an ambient key A never opens A's store.
func dattoStoreScope(c *Config) string {
	if c == nil {
		return ""
	}
	switch dattoCredentialInEffect(c) {
	case dattoCredAPIKey:
		if b := DattoTokenBinding(c.BaseURL, c.DattoRmmApiKey); b != "" {
			return "datto-rmm-store|" + b
		}
	case dattoCredStoredAccess:
		if origin := dattoBindingOrigin(c.ClientID); origin != "" && origin == dattoOriginFingerprint(c.BaseURL) {
			return "datto-rmm-store|" + c.ClientID
		}
	}
	return ""
}

func init() {
	StoreScopeIdentity = dattoStoreScope
}

// dattoTokenEndpoint is the OAuth token endpoint on the API host:
// https://<platform>-api.centrastage.net/auth/oauth/token.
func dattoTokenEndpoint(baseURL string) (string, error) {
	origin := DattoOrigin(baseURL)
	if origin == "" {
		return "", errors.New("the Datto RMM base URL is not an http(s) URL with a plain host; set DATTO_RMM_PLATFORM or DATTO_RMM_BASE_URL")
	}
	u, _ := url.Parse(strings.TrimSpace(baseURL))
	return strings.ToLower(u.Scheme) + "://" + u.Host + "/auth/oauth/token", nil
}

// dattoPlaintextRefused refuses the password grant over plaintext http to any
// host but a loopback one (handfix mint-refuses-plaintext): the form carries
// the API key and secret, and the API client refuses to send a bearer token
// there too ("refusing to send credentials over plaintext http"). The mint
// runs before any API request and through clients that do not apply that
// check, so it applies it itself.
func dattoPlaintextRefused(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("the Datto RMM token endpoint is not a URL")
	}
	if !strings.EqualFold(u.Scheme, "http") {
		return nil
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if addr, err := netip.ParseAddr(host); err == nil && addr.Unmap().IsLoopback() {
		return nil
	}
	return fmt.Errorf("refusing to send the Datto RMM API key and secret over plaintext http to %s; use https", host)
}

// OutboundGuard is the credential kit's outbound guard around every API and token
// transport. A variable so the unit tests that run no credkit.Start, against
// loopback fakes, can run without it (handfixes.json credkit-wiring); the guard is
// proven on the built binaries in internal/credwire.
var OutboundGuard = credkit.Transport

// NewDattoTokenClient is the one token client (handfix mint-pinned-to-api-host,
// credkit-wiring): it follows no redirect, reaches only the API host, and sends
// through the credential kit on base (nil: the default transport). hc is what
// MintDattoToken takes, so every production mint goes through this.
func NewDattoTokenClient(baseURL string, base http.RoundTripper) *http.Client {
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("refused: the Datto RMM token endpoint answered with a redirect")
		},
		Transport: OutboundGuard(dattoTokenTripper{origin: DattoOrigin(baseURL), next: base}),
	}
}

// dattoPublicClientBasic is Datto's fixed, documented public OAuth client (not
// a secret); the API key and secret in the form are the credentials.
var dattoPublicClientBasic = "Basic " + base64.StdEncoding.EncodeToString([]byte("public-client:public"))

// dattoTokenTripper sits beneath the kit. It refuses every host but the API host
// and adds the public client there: the kit holds every Authorization header it
// reads and both Basic parts, so seen by the kit the word "public" would be
// held and withhold ordinary output for the rest of the run. Nothing here adds a
// credential the kit has not already judged.
type dattoTokenTripper struct {
	origin string
	next   http.RoundTripper
}

func (t dattoTokenTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.origin == "" || DattoOrigin(req.URL.String()) != t.origin {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, errors.New("refused: the token request would leave the configured Datto RMM host")
	}
	out := req.Clone(req.Context())
	out.Header.Set("Authorization", dattoPublicClientBasic)
	return t.next.RoundTrip(out)
}

type dattoTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// MintDattoToken performs the Datto RMM OAuth2 password-grant exchange:
//
//	POST {host}/auth/oauth/token
//	Authorization: Basic base64("public-client:public")
//	Content-Type: application/x-www-form-urlencoded
//	grant_type=password&username=<apiKey>&password=<apiSecretKey>
//
// hc must refuse every host but the API host and every redirect (nil builds
// such a client). The token is held for output before it is returned.
func MintDattoToken(ctx context.Context, hc *http.Client, baseURL, apiKey, apiSecretKey string) (token string, expiresIn int, err error) {
	endpoint, err := dattoTokenEndpoint(baseURL)
	if err != nil {
		return "", 0, err
	}
	if err := dattoPlaintextRefused(endpoint); err != nil {
		return "", 0, err
	}
	if hc == nil {
		hc = NewDattoTokenClient(baseURL, nil)
	}
	cliutil.Hold("minted:api_key", apiKey)
	cliutil.Hold("minted:api_secret_key", apiSecretKey)

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", apiKey)
	form.Set("password", apiSecretKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, errors.New("building the Datto RMM token request failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) && ue.Err != nil {
			return "", 0, fmt.Errorf("requesting a Datto RMM token: %s", ue.Err.Error())
		}
		return "", 0, errors.New("requesting a Datto RMM token failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("Datto RMM token request failed: HTTP %d (check DATTO_RMM_API_KEY / DATTO_RMM_API_SECRET_KEY and that DATTO_RMM_PLATFORM matches your account region)", resp.StatusCode)
	}
	var tr dattoTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, errors.New("the Datto RMM token response was not JSON")
	}
	tr.AccessToken = strings.TrimSpace(tr.AccessToken)
	if tr.AccessToken == "" {
		return "", 0, errors.New("the Datto RMM token response did not contain an access_token")
	}
	cliutil.Hold("minted:access_token", tr.AccessToken)
	return tr.AccessToken, tr.ExpiresIn, nil
}

type dattoProcessToken struct {
	access string
	expiry time.Time
}

var (
	dattoMintMu    sync.Mutex
	dattoProcToken = map[string]dattoProcessToken{} // binding -> token minted in this process
)

func dattoTokenLive(expiry time.Time) bool {
	return expiry.IsZero() || time.Now().Before(expiry.Add(-5*time.Minute))
}

// EnsureDattoToken makes sure cfg carries a usable bearer token for the host
// and API key in effect (EnsureDattoTokenWith with a pinned token client).
func EnsureDattoToken(ctx context.Context, cfg *Config) error {
	return EnsureDattoTokenWith(ctx, nil, cfg)
}

// dropDattoTokenOutsideItsBinding drops, from the loaded configuration only
// (the files keep it), a stored token this configuration may never send
// (handfix minted-token-bound-to-key-and-host, applied at Load so every
// consumer sees it: the API client, the store scope, doctor, MCP):
//   - a token minted on another host (its binding names that host), whatever
//     else is configured. EnsureDattoTokenWith dropped it only with no API
//     key, after a DATTO_RMM_TOKEN had already returned, so the generated
//     AuthHeader (a stored access token first) sent host A's token to host B
//     (tests-helper finding TestStoredTokenNeverReachesAnotherHost);
//   - with an API key and secret in effect, any stored token not minted for
//     that key on this host (a token `auth set-token` saved included): the
//     client mints over it anyway, and the default store scope adopted the
//     store named by it, so host B's key read host A's synced records
//     (TestStoreOfAnotherHostIsNeverAdopted).
//
// A token `auth set-token` saved has no binding; with no key it stays the
// operator's choice. The dropped values stay held (credkit.Start read the files
// they came from).
//
// Codex review r1 class "credential identity mismatch" (handfix
// token-identity-follows-the-configured-credential): the most explicit
// credential names the identity, and nothing stored may stand in for it:
//   - an environment DATTO_RMM_TOKEN is the bearer: every stored token and a
//     stored auth_header are dropped (the generated AuthHeader sent a stored
//     access token, or a file auth_header, first, so account A's token went out
//     while DATTO_RMM_TOKEN named account B);
//   - a configured API key, with or without its secret, allows only a token
//     minted for that key on this host: a stored token bound to another key or
//     to none, a stored auth_header and a stored rmm_token are dropped (the
//     binding was checked only with the secret also set, so key B alone sent
//     key A's token).
//
// Codex review r2: a config [headers] Authorization is one of those carriers
// too: the client applied it over the bearer, so it went out beside key B and
// over DATTO_RMM_TOKEN B (handfix credential-identity-one-resolver).
func dropDattoTokenOutsideItsBinding(c *Config) {
	if c == nil {
		return
	}
	dropStored := func() { c.AccessToken, c.RefreshToken, c.TokenExpiry, c.ClientID = "", "", time.Time{}, "" }
	header := takeDattoHeaderAuthorization(c)
	if c.DattoRmmToken != "" && c.envOverrides["DattoRmmToken"] {
		dropStored()
		c.AuthHeaderVal = ""
		return
	}
	if key := strings.TrimSpace(c.DattoRmmApiKey); key != "" {
		c.AuthHeaderVal = ""
		c.DattoRmmToken = ""
		if c.AccessToken != "" && c.ClientID != DattoTokenBinding(c.BaseURL, key) {
			dropStored()
		}
		return
	}
	if header != "" {
		// A config header is the identity only when nothing more explicit is
		// configured. It moves to the one carrier AuthHeader reads; the save path
		// keeps it out of the credential store (configForSave).
		cliutil.Hold("config:headers", header) // Hand-fix (handfixes.json credkit-wiring): a config loaded after startup too
		c.AuthHeaderVal = header
		c.markEnvOverride("AuthHeaderVal")
		if c.AuthSource == "" {
			c.AuthSource = "config"
		}
		if c.CredentialSource == "" {
			c.CredentialSource = c.configOwner
		}
	}
	if c.AccessToken != "" {
		if o := dattoBindingOrigin(c.ClientID); o != "" && o != dattoOriginFingerprint(c.BaseURL) {
			dropStored()
		}
	}
}

// dattoCredentialKind names the one credential a configuration acts on.
type dattoCredentialKind int

const (
	dattoCredNone dattoCredentialKind = iota
	dattoCredEnvToken
	dattoCredAPIKey
	dattoCredAuthHeader // a stored auth_header, or a config [headers] Authorization once Load moved it there
	dattoCredStoredAccess
	dattoCredStoredToken
)

// dattoCredentialInEffect picks the credential once (handfix
// credential-identity-one-resolver, Codex review r2 classes credential identity
// mismatch and tenant store identity mismatch), most explicit first:
// DATTO_RMM_TOKEN in the environment, then a configured API key, then what is
// stored in the order AuthHeader reads it. The Authorization carriers
// (dropDattoTokenOutsideItsBinding leaves AuthHeader one to read and none in
// Headers) and the store scope (dattoStoreScope) both follow it.
func dattoCredentialInEffect(c *Config) dattoCredentialKind {
	switch {
	case c.DattoRmmToken != "" && c.envOverrides["DattoRmmToken"]:
		return dattoCredEnvToken
	case strings.TrimSpace(c.DattoRmmApiKey) != "":
		return dattoCredAPIKey
	case c.AuthHeaderVal != "":
		return dattoCredAuthHeader
	case c.AccessToken != "":
		return dattoCredStoredAccess
	case c.DattoRmmToken != "":
		return dattoCredStoredToken
	}
	return dattoCredNone
}

// takeDattoHeaderAuthorization removes every spelling of Authorization from
// the loaded header map, so no header can overwrite the chosen bearer on the
// wire (the client applies Headers after the bearer), and returns the value.
// Two spellings in one file: the first in sorted order with a value.
func takeDattoHeaderAuthorization(c *Config) string {
	var names []string
	for k := range c.Headers {
		if strings.EqualFold(strings.TrimSpace(k), "Authorization") {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	value := ""
	for _, k := range names {
		if v := strings.TrimSpace(c.Headers[k]); value == "" {
			value = v
		}
		delete(c.Headers, k)
	}
	return value
}

// EnsureDattoTokenWith decides the bearer for cfg; hc is the pinned client the
// token request goes through (nil builds one).
//   - A credential route Load refused: nothing.
//   - A stored token outside its binding is dropped, never sent
//     (dropDattoTokenOutsideItsBinding).
//   - A pre-minted DATTO_RMM_TOKEN, or no API key and secret: nothing more.
//   - An API key and secret: a live token this process minted for them, else
//     a live stored token bound to them, else a fresh mint, saved best-effort
//     to the credential store (DATTO_RMM_NO_CONFIG_WRITE keeps it in memory).
func EnsureDattoTokenWith(ctx context.Context, hc *http.Client, cfg *Config) error {
	if cfg == nil || cfg.CredentialRouteBlocked() {
		return nil
	}
	dropDattoTokenOutsideItsBinding(cfg)
	if cfg.DattoRmmToken != "" {
		return nil
	}
	key, secret := strings.TrimSpace(cfg.DattoRmmApiKey), cfg.DattoRmmApiSecretKey
	if key == "" || secret == "" {
		return nil
	}
	binding := DattoTokenBinding(cfg.BaseURL, key)
	if binding == "" {
		return errors.New("the Datto RMM base URL is not an http(s) URL with a plain host; set DATTO_RMM_PLATFORM or DATTO_RMM_BASE_URL")
	}
	dattoMintMu.Lock()
	defer dattoMintMu.Unlock()
	if t, ok := dattoProcToken[binding]; ok && dattoTokenLive(t.expiry) {
		cfg.AuthHeaderVal = ""
		cfg.AccessToken, cfg.TokenExpiry = t.access, t.expiry
		return nil
	}
	if cfg.AccessToken != "" && cfg.ClientID == binding && dattoTokenLive(cfg.TokenExpiry) {
		return nil
	}
	token, expiresIn, err := MintDattoToken(ctx, hc, cfg.BaseURL, key, secret)
	if err != nil {
		return err
	}
	expiry := time.Time{}
	if expiresIn > 0 {
		expiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
	dattoProcToken[binding] = dattoProcessToken{access: token, expiry: expiry}
	cfg.AuthHeaderVal = ""
	cfg.AuthSource = "oauth:DATTO_RMM_API_KEY"
	// Best-effort: a read-only home must not break the command; the token is
	// in effect either way.
	_ = cfg.SaveTokens(binding, "", token, "", expiry)
	cfg.AccessToken, cfg.TokenExpiry = token, expiry
	return nil
}
