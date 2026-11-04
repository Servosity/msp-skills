// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// kaseya-bms handfixes applied to every API client (CLI, novel commands and MCP)
// through registerClientHook, so no generated file changes:
//
//   - api-origin-pinned: a credential goes only to the scheme, host and port
//     it is configured for. The bearer token, the token request (POST
//     /v2/security/authenticate on the same host) and every API request go to the
//     origin of the API base URL (KASEYA_BMS_BASE_URL or the config file, default
//     https://api.bms.kaseya.com). Every redirect hop must stay on
//     that origin. Nothing given later (a Location header, a next-page link,
//     an MCP argument, a path) can move a credential to another host.
//     'auth login' builds its own token client; guardTokenClient pins it the
//     same way.
//   - token-minted-per-client: the hook then makes sure the client carries a
//     bearer for the host and login in effect (config.EnsureKbmsTokenWith
//     through this pinned client). The 0.1.x connector minted only in `auth
//     login`, so the login its MCP manifest asks for answered 401 once the
//     short-lived JWT expired, on the CLI and the MCP server alike. Never
//     under --dry-run or the verifier.
//   - no-response-cache: the on-disk response cache is off. Raw bodies must
//     not sit in cache files; the local store keeps credential fields out
//     (store/kbms_secrets.go).
//   - credkit-wiring: every request goes through the shared credential kit
//     (credkit.Transport, inside the origin pin), which refuses a held
//     credential to any origin it is not bound to and holds the declared
//     response fields (internal/credwire) before the body reaches a command.
//   - credential-endpoint-result-held: a credential endpoint's answer that IS
//     the credential, unnamed, is held too (credentialResultTripper).
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"kaseya-bms-pp-cli/internal/client"
	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/config"
	"kaseya-bms-pp-cli/internal/credkit"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured Kaseya BMS host")

// ErrDryRunSend marks a request refused because the command runs under
// --dry-run.
var ErrDryRunSend = errors.New("--dry-run: request not sent")

func init() {
	registerClientHook(func(c *client.Client) error {
		guardClient(c)
		return mintForClient(c)
	})
}

// kbmsNoMint holds the configurations whose clients must not mint: an MCP
// call that only previews sends nothing, so it needs no token (kaseya-bms handfix
// mcp-preview-never-mints).
var kbmsNoMint sync.Map // *config.Config -> struct{}

// WithoutMintFor marks cfg so the clients built from it do not mint; the
// returned func clears the mark.
func WithoutMintFor(cfg *config.Config) func() {
	kbmsNoMint.Store(cfg, struct{}{})
	return func() { kbmsNoMint.Delete(cfg) }
}

// mintForClient puts a bearer for the host and API key in effect on c, minted
// through c's own pinned client. Never under --dry-run, the verifier, or for a
// configuration marked by WithoutMintFor.
func mintForClient(c *client.Client) error {
	if c == nil || c.Config == nil || c.DryRun || cliutil.IsVerifyEnv() {
		return nil
	}
	if _, skip := kbmsNoMint.Load(c.Config); skip {
		return nil
	}
	if err := config.EnsureKbmsTokenWith(context.Background(), c.HTTPClient, c.Config); err != nil {
		return authErr(err)
	}
	return nil
}

// kbmsPins is what one client may reach: the API origin, nothing else.
type kbmsPins struct {
	api string // normalized origin of the API base URL ("" pins nothing)
}

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, ok := c.HTTPClient.Transport.(originPinTripper); ok {
		return // already guarded (a hook ran twice on one client)
	}
	c.NoCache = true
	installPins(c.HTTPClient, newKbmsPins(c.BaseURL), c.DryRun)
}

// guardTokenClient pins the token-endpoint client 'auth login' builds to the
// API host, holds what the token endpoint answers, and refuses every request
// under --dry-run.
func guardTokenClient(hc *http.Client, cfg *config.Config, dryRun bool) {
	if hc == nil || cfg == nil {
		return
	}
	if _, ok := hc.Transport.(originPinTripper); ok {
		return
	}
	installPins(hc, newKbmsPins(cfg.BaseURL), dryRun)
}

func installPins(hc *http.Client, pins kbmsPins, dryRun bool) {
	next := hc.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	// Hand-fix (handfixes.json credkit-wiring): the kit inside the pin, so an
	// off-origin request is refused before the kit reads it, and outside the
	// dial-guarded transport the generated client installed.
	var inner http.RoundTripper = credentialResultTripper{next: config.OutboundGuard(next)}
	if dryRun {
		inner = dryRunTripper{}
	}
	hc.Transport = originPinTripper{pins: pins, next: inner}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := hc.CheckRedirect
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || via[0] == nil || normalizedOrigin(req.URL) == "" || normalizedOrigin(req.URL) != normalizedOrigin(via[0].URL) {
			return fmt.Errorf("%w (a redirect to %s); set KASEYA_BMS_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
		}
		if err := pins.check(req); err != nil {
			return err
		}
		if generated != nil {
			return generated(req, via)
		}
		return nil
	}
}

func newKbmsPins(baseURL string) kbmsPins {
	var p kbmsPins
	if u, err := url.Parse(baseURL); err == nil && u.User == nil {
		p.api = normalizedOrigin(u)
	}
	return p
}

// check refuses a request to any origin but the pinned one. An empty pin (a
// URL that does not parse to an origin) matches nothing: fail closed.
func (p kbmsPins) check(req *http.Request) error {
	o := normalizedOrigin(req.URL)
	if o != "" && p.api != "" && o == p.api {
		return nil
	}
	return fmt.Errorf("%w (%s); set KASEYA_BMS_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
}

// normalizedOrigin is scheme://host:port with ASCII letters lowercased and
// the default port filled in. It is "" (never equal to a pinned origin) for a
// host holding any non-ASCII byte: Unicode case folding and IDNA can make two
// spellings compare equal while the transport dials another name.
func normalizedOrigin(u *url.URL) string {
	if u == nil || u.Host == "" || !isASCII(u.Scheme) || !isASCII(u.Host) {
		return ""
	}
	scheme := asciiLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(asciiLower(u.Hostname()), port)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

type originPinTripper struct {
	pins kbmsPins
	next http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.pins.check(req); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(req)
}

type dryRunTripper struct{}

func (dryRunTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, fmt.Errorf("%w (%s %s)", ErrDryRunSend, req.Method, displayOrigin(req.URL))
}

func displayOrigin(u *url.URL) string {
	if u == nil {
		return "no URL"
	}
	if o := normalizedOrigin(&url.URL{Scheme: u.Scheme, Host: u.Host}); o != "" {
		return o
	}
	return "a non-ASCII host"
}

// credentialResultTripper reads the response of a request to a credential
// endpoint (cliutil.CredentialPath) under the client's size cap, decodes a
// copy as the client will (Content-Encoding, then BOM and JSONP/XSSI
// prefixes), and holds the answer when it IS the credential
// (holdCredentialResult). Every other response passes untouched; the kit
// holds the declared response fields of every response.
type credentialResultTripper struct{ next http.RoundTripper }

func (t credentialResultTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil || req == nil || req.URL == nil || !cliutil.CredentialPath(req.URL.Path) {
		return resp, err
	}
	body, rerr := client.ReadBody(resp.Body)
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if plain, derr := client.DecodeBody(resp.Header.Get("Content-Encoding"), body); derr == nil {
		holdCredentialResult(plain)
	}
	return resp, nil
}

// holdCredentialResult holds a credential endpoint's answer when the answer IS
// the credential, unnamed: GET /v2/integrations/itglue/passwords/{passwordId}
// returns the IT Glue password as a bare string under Result (StringResponse
// {"Success":true,"Result":"<password>"}), which the name-based holder never
// reads as a credential, so the CLI printed it and MCP returned it. A scalar
// Result is held; an object or list Result keeps the name-based holding
// (TokenOutputDto's AccessToken and RefreshToken are held by name, its expiry
// timestamps are data); a body that is not a JSON object is held whole.
func holdCredentialResult(plain []byte) {
	trimmed := bytes.TrimSpace(plain)
	var env map[string]json.RawMessage
	if json.Unmarshal(trimmed, &env) != nil {
		if len(trimmed) > 0 && len(trimmed) <= 4096 {
			credkit.Hold("response:Result", string(trimmed))
		}
		return
	}
	for k, raw := range env {
		if !strings.EqualFold(k, "result") {
			continue
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var v any
		if d.Decode(&v) != nil {
			continue
		}
		switch x := v.(type) {
		case string:
			credkit.Hold("response:Result", x)
		case json.Number:
			credkit.Hold("response:Result", x.String())
		}
	}
}
