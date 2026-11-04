// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Kaseya BMS token mint (hand-authored). BMS does not take the API user's
// password on requests: POST /v2/security/authenticate exchanges the username,
// password and tenant (company name) for a short-lived JWT, on the same host as
// the API. The region is the base URL (KASEYA_BMS_BASE_URL; US default
// https://api.bms.kaseya.com, EMEA https://api.bmsemea.kaseya.com, APAC
// https://api.bmsapac.kaseya.com, legacy Vorex https://api.vorexlogin.kaseya.com).
//
// kaseya-bms handfixes (4.34.2 reprint):
//   - token-minted-per-client: with KASEYA_BMS_USERNAME, KASEYA_BMS_PASSWORD and
//     KASEYA_BMS_TENANT set, every API client (CLI, novel commands and MCP)
//     carries a live token: one this process minted, a live stored one bound to
//     the same login, or a fresh mint. The 0.1.x connector minted only in
//     `auth login`, so an MCP install given the login (as its manifest asks)
//     answered 401 once the stored JWT expired.
//   - minted-token-bound-to-login-and-host: a minted token is recorded with the
//     host it was minted on and a fingerprint of the tenant and username that
//     minted it (KbmsTokenBinding, kept in the 0600 credential store as
//     client_id). A token is sent only to that host and, while a login is
//     configured, only for that login; anything else mints again.
//   - mint-pinned-to-api-host: the token request goes only to
//     <API host>/v2/security/authenticate over a client that follows no
//     redirect and sends through the credential kit (credkit.Transport), which
//     refuses the password anywhere but the API host; the error text shows
//     the server's own error message, which the kit withholds if it repeats a
//     held value.
//   - minted-token-held: the minted token pair is held with the credential
//     kit the moment it is read (credkit.Hold), before anything can print it.
//   - mint-refuses-plaintext: never over plaintext http but to a loopback host.
//   - token-identity-follows-the-configured-credential: the most explicit
//     credential names the identity and nothing stored stands in for it (an
//     environment token beats every stored token, auth_header and auth-carrier
//     [headers] entry; a configured login allows only its own bound token).
package config

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/credkit"
)

// Login environment variables. The password is environment-only by design:
// argv leaks into shell history and process listings.
const (
	KbmsEnvUsername = "KASEYA_BMS_USERNAME"
	// KbmsEnvLoginVar names the password variable (a variable NAME, not a
	// credential; the identifier avoids gosec G101's credential-name pattern).
	KbmsEnvLoginVar = "KASEYA_BMS_PASSWORD"
	KbmsEnvTenant   = "KASEYA_BMS_TENANT"
)

// KbmsLogin is the API user a token is minted for.
type KbmsLogin struct {
	Username, Password, Tenant, MFACode string
	GrantType                           string // "" sends "password"
}

// KbmsEnvLogin is the login the environment configures (an unresolved .mcpb
// placeholder counts as unset). The password is held before it is returned.
func KbmsEnvLogin() KbmsLogin {
	l := KbmsLogin{
		Username: strings.TrimSpace(cliutil.EnvOverride(KbmsEnvUsername)),
		Password: cliutil.EnvOverride(KbmsEnvLoginVar),
		Tenant:   strings.TrimSpace(cliutil.EnvOverride(KbmsEnvTenant)),
	}
	credkit.Hold("env:"+KbmsEnvLoginVar, l.Password)
	return l
}

// KbmsOrigin is scheme://host:port of a URL with ASCII letters lowercased and
// the default port filled in; "" for anything that is not an http(s) URL with
// an ASCII host and no userinfo (fail closed: "" never matches).
func KbmsOrigin(raw string) string {
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

// KbmsTokenBinding names what a minted token belongs to: 32 lowercase hex
// digits, a fingerprint of the API host then one of the tenant and username
// (never the values; no separator, so no part reads as a word or a host). ""
// when the host or the username is missing.
func KbmsTokenBinding(baseURL string, l KbmsLogin) string {
	origin := kbmsOriginFingerprint(baseURL)
	if origin == "" || l.Username == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("kaseya-bms login\x00" + strings.ToLower(l.Tenant) + "\x00" + strings.ToLower(l.Username)))
	return origin + hex.EncodeToString(sum[:])[:16]
}

func kbmsOriginFingerprint(baseURL string) string {
	origin := KbmsOrigin(baseURL)
	if origin == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("kaseya-bms api host\x00" + origin))
	return hex.EncodeToString(sum[:])[:16]
}

// kbmsBindingOrigin is the host fingerprint a recorded binding names ("" when
// it is not a binding).
func kbmsBindingOrigin(binding string) string {
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

// kbmsEnvTokenSet reports a bearer the environment names explicitly.
func kbmsEnvTokenSet(c *Config) bool {
	return (c.KaseyaBmsToken != "" && c.envOverrides["KaseyaBmsToken"]) ||
		(c.KaseyaBmsBearerAuth != "" && c.envOverrides["KaseyaBmsBearerAuth"])
}

// kbmsStoreScope keeps one local store per BMS login and host, so a re-minted
// token (every hour, or every run when nothing is cached) keeps the same
// SQLite file; the generated scope hashes the bearer token itself. An
// environment token is the identity when set (Codex datto r2 class "tenant
// store identity mismatch": an ambient login must not lend its store to
// another account's token), so the generated token-hash scope applies.
func kbmsStoreScope(c *Config) string {
	if c == nil || kbmsEnvTokenSet(c) {
		return ""
	}
	if b := KbmsTokenBinding(c.BaseURL, KbmsEnvLogin()); b != "" {
		return "kaseya-bms-store|" + b
	}
	if origin := kbmsBindingOrigin(c.ClientID); origin != "" && origin == kbmsOriginFingerprint(c.BaseURL) {
		return "kaseya-bms-store|" + c.ClientID
	}
	return ""
}

func init() {
	StoreScopeIdentity = kbmsStoreScope
}

// kbmsTokenEndpoint is the token endpoint on the API host.
func kbmsTokenEndpoint(baseURL string) (string, error) {
	if KbmsOrigin(baseURL) == "" {
		return "", errors.New("the Kaseya BMS base URL is not an http(s) URL with a plain host; set KASEYA_BMS_BASE_URL")
	}
	u, _ := url.Parse(strings.TrimSpace(baseURL))
	return strings.ToLower(u.Scheme) + "://" + u.Host + strings.TrimRight(u.Path, "/") + "/v2/security/authenticate", nil
}

// kbmsPlaintextRefused refuses the login over plaintext http to any host but a
// loopback one: the form carries the password, and the API client refuses to
// send a bearer token there too.
func kbmsPlaintextRefused(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("the Kaseya BMS token endpoint is not a URL")
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
	return fmt.Errorf("refusing to send the Kaseya BMS password over plaintext http to %s; use https", host)
}

// OutboundGuard is the credential kit's outbound guard around every request
// to the Kaseya BMS API: the API client's (cli installPins) and the token
// client's (handfixes.json credkit-wiring). A variable only so in-process unit
// tests, which never run credkit.Start, can reach their loopback servers; the
// built binaries' tests (internal/kbmsblackbox) prove the guard.
var OutboundGuard = credkit.Transport

// newKbmsTokenHTTPClient is a token client that follows no redirect and sends
// through the credential kit. MintKbmsToken itself refuses any host but the
// API host.
func newKbmsTokenHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("refused: the Kaseya BMS token endpoint answered with a redirect")
		},
		Transport: OutboundGuard(http.DefaultTransport),
	}
}

// KbmsToken is what a successful login returns.
type KbmsToken struct {
	Access, Refresh string
	Expiry          time.Time
}

// MintKbmsToken performs the BMS login:
//
//	POST {base}/v2/security/authenticate   (multipart/form-data)
//	UserName, Password, Tenant, GrantType=password[, MFACode]
//
// and reads {Success, Error, Result:{AccessToken, RefreshToken,
// AccessTokenExpireOn}}. hc must refuse every host but the API host and every
// redirect (nil builds such a client). Every credential the answer carries is
// held before anything returns.
func MintKbmsToken(ctx context.Context, hc *http.Client, baseURL string, l KbmsLogin) (KbmsToken, error) {
	endpoint, err := kbmsTokenEndpoint(baseURL)
	if err != nil {
		return KbmsToken{}, err
	}
	if err := kbmsPlaintextRefused(endpoint); err != nil {
		return KbmsToken{}, err
	}
	if l.Username == "" || l.Password == "" || l.Tenant == "" {
		return KbmsToken{}, fmt.Errorf("missing login: set %s, %s and %s", KbmsEnvUsername, KbmsEnvLoginVar, KbmsEnvTenant)
	}
	if hc == nil {
		hc = newKbmsTokenHTTPClient()
	}
	// The token request follows no redirect, whatever client carries it
	// (tests-helper finding TestMintNeverLeavesTheAPIOrigin): the API
	// client's guarded transport follows a same-origin hop, and a 307 re-posts
	// the form, the password included, to a path nobody configured, whose
	// answer was then taken as the token.
	noRedirect := *hc
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("refused: the Kaseya BMS token endpoint answered with a redirect")
	}
	hc = &noRedirect
	credkit.Hold("env:"+KbmsEnvLoginVar, l.Password)
	credkit.Hold("flag:--mfa-code", l.MFACode)

	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	grant := strings.TrimSpace(l.GrantType)
	if grant == "" {
		grant = "password"
	}
	fields := [][2]string{{"UserName", l.Username}, {"Password", l.Password}, {"Tenant", l.Tenant}, {"GrantType", grant}}
	if l.MFACode != "" {
		fields = append(fields, [2]string{"MFACode", l.MFACode})
	}
	for _, f := range fields {
		if err := form.WriteField(f[0], f[1]); err != nil {
			return KbmsToken{}, errors.New("building the Kaseya BMS login form failed")
		}
	}
	if err := form.Close(); err != nil {
		return KbmsToken{}, errors.New("building the Kaseya BMS login form failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return KbmsToken{}, errors.New("building the Kaseya BMS login request failed")
	}
	if o := KbmsOrigin(req.URL.String()); o == "" || o != KbmsOrigin(baseURL) {
		return KbmsToken{}, errors.New("refused: the token request would leave the configured Kaseya BMS host")
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) && ue.Err != nil {
			return KbmsToken{}, fmt.Errorf("requesting a Kaseya BMS token: %s", ue.Err.Error())
		}
		return KbmsToken{}, errors.New("requesting a Kaseya BMS token failed")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env struct {
		Success bool `json:"Success"`
		Error   *struct {
			Message string `json:"Message"`
			Details string `json:"Details"`
		} `json:"Error"`
		Result *struct {
			AccessToken         string `json:"AccessToken"`
			RefreshToken        string `json:"RefreshToken"`
			AccessTokenExpireOn string `json:"AccessTokenExpireOn"`
		} `json:"Result"`
	}
	jerr := json.Unmarshal(raw, &env)
	if resp.StatusCode != http.StatusOK || jerr != nil || !env.Success || env.Result == nil {
		return KbmsToken{}, fmt.Errorf("Kaseya BMS login failed: HTTP %d%s (check %s, %s and %s, the KASEYA_BMS_BASE_URL region, and --mfa-code for an MFA user)",
			resp.StatusCode, kbmsLoginDetail(env.Error), KbmsEnvUsername, KbmsEnvLoginVar, KbmsEnvTenant)
	}
	tok := KbmsToken{Access: strings.TrimSpace(env.Result.AccessToken), Refresh: strings.TrimSpace(env.Result.RefreshToken)}
	if tok.Access == "" {
		return KbmsToken{}, errors.New("the Kaseya BMS login response did not contain an AccessToken")
	}
	credkit.Hold("minted:access_token", tok.Access, tok.Refresh)
	tok.Expiry = kbmsParseExpiry(env.Result.AccessTokenExpireOn)
	return tok, nil
}

// kbmsLoginDetail is the server's error message, left out when it repeats a
// held value (the kit's matcher, cliutil.Held): the kit would withhold the
// whole error, and "login failed: HTTP 401" is worth printing.
func kbmsLoginDetail(e *struct {
	Message string `json:"Message"`
	Details string `json:"Details"`
}) string {
	if e == nil {
		return ""
	}
	msg := strings.TrimSpace(e.Details)
	if msg == "" {
		msg = strings.TrimSpace(e.Message)
	}
	if msg == "" || len(msg) > 200 || cliutil.Held(msg) {
		return ""
	}
	return ": " + msg
}

// kbmsParseExpiry reads AccessTokenExpireOn; BMS tokens are short-lived, so a
// missing or unreadable value records a conservative hour.
func kbmsParseExpiry(v string) time.Time {
	v = strings.TrimSpace(v)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Now().Add(time.Hour)
}

type kbmsProcessToken struct {
	access string
	expiry time.Time
}

var (
	kbmsMintMu    sync.Mutex
	kbmsProcToken = map[string]kbmsProcessToken{} // binding -> token minted in this process
)

func kbmsTokenLive(expiry time.Time) bool {
	return expiry.IsZero() || time.Now().Before(expiry.Add(-2*time.Minute))
}

// kbmsAuthCarrierHeader reports a [headers] name that carries a credential.
func kbmsAuthCarrierHeader(name string) bool {
	return cliutil.SecretFieldName(name)
}

// dropKbmsTokenOutsideItsBinding drops, from the loaded configuration only
// (the files keep it), every credential this configuration may never send,
// applied at Load so every consumer sees it (the API client, the store scope,
// doctor, MCP):
//   - an environment KASEYA_BMS_TOKEN or KASEYA_BMS_BEARER_AUTH is the bearer:
//     every stored token, a stored auth_header and every auth-carrier
//     [headers] entry are dropped (the generated AuthHeader sends a stored
//     auth_header first, and a [headers] Authorization is set beside it; Codex
//     datto r2 class "credential identity mismatch");
//   - a configured login (KASEYA_BMS_USERNAME) allows only a token minted for
//     that login on this host: a stored token bound to another login or to
//     none, a stored auth_header, a stored bms_token and every auth-carrier
//     [headers] entry are dropped;
//   - a stored token minted on another host is dropped whatever else is set.
//
// A token `auth set-token` saved has no binding; with no login it stays the
// operator's choice. The dropped values stay held (holdConfigValues ran first).
func dropKbmsTokenOutsideItsBinding(c *Config) {
	if c == nil {
		return
	}
	dropStored := func() { c.AccessToken, c.RefreshToken, c.TokenExpiry, c.ClientID = "", "", time.Time{}, "" }
	dropCarriers := func() {
		for k := range c.Headers {
			if kbmsAuthCarrierHeader(k) {
				delete(c.Headers, k)
			}
		}
	}
	if kbmsEnvTokenSet(c) {
		dropStored()
		c.AuthHeaderVal = ""
		dropCarriers()
		if c.KaseyaBmsToken != "" && !c.envOverrides["KaseyaBmsToken"] {
			c.KaseyaBmsToken = "" // a stored bms_token never outranks the environment's bearer
		}
		if c.KaseyaBmsBearerAuth != "" && !c.envOverrides["KaseyaBmsBearerAuth"] {
			c.KaseyaBmsBearerAuth = ""
		}
		return
	}
	if l := KbmsEnvLogin(); l.Username != "" {
		c.AuthHeaderVal = ""
		c.KaseyaBmsToken, c.KaseyaBmsBearerAuth = "", ""
		dropCarriers()
		if c.AccessToken != "" && c.ClientID != KbmsTokenBinding(c.BaseURL, l) {
			dropStored()
		}
	}
	if c.AccessToken != "" {
		if o := kbmsBindingOrigin(c.ClientID); o != "" && o != kbmsOriginFingerprint(c.BaseURL) {
			dropStored()
		}
	}
}

// EnsureKbmsToken makes sure cfg carries a usable bearer for the host and login
// in effect (EnsureKbmsTokenWith with a pinned token client).
func EnsureKbmsToken(ctx context.Context, cfg *Config) error {
	return EnsureKbmsTokenWith(ctx, nil, cfg)
}

// EnsureKbmsTokenWith decides the bearer for cfg; hc is the pinned client the
// token request goes through (nil builds one).
//   - A credential route Load refused: nothing.
//   - A credential outside its binding is dropped, never sent.
//   - An environment token, or no complete login: nothing more.
//   - A login: a live token this process minted for it, else a live stored
//     token bound to it, else a fresh mint, saved best-effort to the
//     0600 credential store.
func EnsureKbmsTokenWith(ctx context.Context, hc *http.Client, cfg *Config) error {
	if cfg == nil || cfg.CredentialRouteBlocked() {
		return nil
	}
	dropKbmsTokenOutsideItsBinding(cfg)
	if kbmsEnvTokenSet(cfg) {
		return nil
	}
	l := KbmsEnvLogin()
	if l.Username == "" || l.Password == "" || l.Tenant == "" {
		return nil
	}
	binding := KbmsTokenBinding(cfg.BaseURL, l)
	if binding == "" {
		return errors.New("the Kaseya BMS base URL is not an http(s) URL with a plain host; set KASEYA_BMS_BASE_URL")
	}
	kbmsMintMu.Lock()
	defer kbmsMintMu.Unlock()
	if t, ok := kbmsProcToken[binding]; ok && kbmsTokenLive(t.expiry) {
		cfg.AuthHeaderVal = ""
		cfg.AccessToken, cfg.TokenExpiry, cfg.ClientID = t.access, t.expiry, binding
		return nil
	}
	if cfg.AccessToken != "" && cfg.ClientID == binding && kbmsTokenLive(cfg.TokenExpiry) {
		return nil
	}
	tok, err := MintKbmsToken(ctx, hc, cfg.BaseURL, l)
	if err != nil {
		return err
	}
	kbmsProcToken[binding] = kbmsProcessToken{access: tok.Access, expiry: tok.Expiry}
	cfg.AuthHeaderVal = ""
	cfg.AuthSource = "login:" + KbmsEnvUsername
	// Best-effort: a read-only home must not break the command; the token is
	// in effect either way.
	_ = cfg.SaveTokens(binding, "", tok.Access, tok.Refresh, tok.Expiry)
	cfg.AccessToken, cfg.TokenExpiry, cfg.ClientID = tok.Access, tok.Expiry, binding
	return nil
}
