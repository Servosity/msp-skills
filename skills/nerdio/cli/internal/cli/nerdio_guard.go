// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// nerdio handfixes applied to every API client (CLI, novel commands and MCP)
// through registerClientHook, so no generated file changes:
//
//   - api-origin-pinned: a credential goes only to the scheme, host and port
//     it is configured for. The bearer token and every API request go to the
//     origin of the API base URL (NERDIO_BASE_URL or the config file: the
//     MSP's own NMM instance). The shipped placeholder (nmm.example.com) is
//     no instance of anyone's, so it pins nothing and every request is
//     refused, naming NERDIO_BASE_URL. The client ID and secret go only to
//     the token URL the mint uses (client.ResolveNerdioTokenURL: the
//     operator's NERDIO_TOKEN_URL, the Entra tenant endpoint; none when it is
//     unset or still the placeholder tenant), and that origin receives
//     nothing but a POST to that exact path that carries no bearer token. Every redirect
//     hop must stay on the origin of the request it follows. Nothing given
//     later (a Location header, an MCP argument, a path) can move a credential
//     to another host. 'auth login' builds its own token client;
//     guardTokenClient pins it to the token URL the same way.
//   - no-response-cache: the on-disk response cache is off. Raw bodies must
//     not sit in cache files.
//   - credkit-wiring: the pins sit over the credential kit's transport
//     (credkit.Transport), which holds every declared response field before
//     the body reaches any command (the client masks those fields by name,
//     credential-fields-masked-by-name). A tenant secret the operator asks
//     for by name (see nerdioTenantSecretPath) is not a kit credential: it is
//     shown to the caller who asked and never stored (its path is no sync
//     partition, so it is never cached), tenantSecretTripper.
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"nerdio-pp-cli/internal/client"
	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/config"
	"nerdio-pp-cli/internal/credkit"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
// It is a client.ErrHostPinRefused, so the client treats it as final and does
// not retry it (nerdio handfix guard-refusal-is-final).
var ErrOriginChanged error = guardRefusal{"request refused: it would leave the configured Nerdio host"}

// ErrDryRunSend marks a request refused because the command runs under
// --dry-run (final too: a retry would be refused the same way).
var ErrDryRunSend error = guardRefusal{"--dry-run: request not sent"}

// guardRefusal is a refusal of this guard: errors.Is matches the exact
// sentinel and client.ErrHostPinRefused.
type guardRefusal struct{ msg string }

func (e guardRefusal) Error() string        { return e.msg }
func (e guardRefusal) Is(target error) bool { return target == client.ErrHostPinRefused }

func init() {
	registerClientHook(func(c *client.Client) error {
		guardClient(c)
		return nil
	})
}

// nerdioPins is what one client may reach: any request to api, and to token
// only the token POST itself.
type nerdioPins struct {
	api         string // normalized origin of the API base URL ("" pins nothing)
	token       string // normalized origin of the token URL ("" pins nothing)
	tokenPath   string
	placeholder bool // the API base URL is still the shipped placeholder host
}

// nerdioPlaceholderHost is the host of the shipped base_url. Every MSP runs
// its own NMM instance, so this host is nobody's API and no request may go to
// it: a bearer token sent there would reach whoever answers for the name.
const nerdioPlaceholderHost = "nmm.example.com"

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, ok := c.HTTPClient.Transport.(originPinTripper); ok {
		return // already guarded (a hook ran twice on one client)
	}
	c.NoCache = true
	installPins(c.HTTPClient, newNerdioPins(c.BaseURL, client.ResolveNerdioTokenURL(c.Config)), c.DryRun)
}

// guardTokenClient pins the token-endpoint client 'auth login' builds to the
// token URL alone (it sends no API request) and refuses every request under
// --dry-run. The kit's transport underneath holds what the token endpoint
// answers.
func guardTokenClient(hc *http.Client, cfg *config.Config, dryRun bool) {
	if hc == nil {
		return
	}
	if _, ok := hc.Transport.(originPinTripper); ok {
		return
	}
	installPins(hc, newNerdioPins("", client.ResolveNerdioTokenURL(cfg)), dryRun)
}

func installPins(hc *http.Client, pins nerdioPins, dryRun bool) {
	next := hc.Transport
	if next == nil {
		next = credkit.Transport(nil) // Hand-fix (handfixes.json credkit-wiring)
	}
	var inner http.RoundTripper = tenantSecretTripper{next: next}
	if dryRun {
		inner = dryRunTripper{}
	}
	hc.Transport = originPinTripper{pins: pins, next: inner}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := hc.CheckRedirect
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || via[0] == nil || normalizedOrigin(req.URL) == "" || normalizedOrigin(req.URL) != normalizedOrigin(via[0].URL) {
			return fmt.Errorf("%w (a redirect to %s); set NERDIO_BASE_URL or NERDIO_TOKEN_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
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

func newNerdioPins(baseURL, tokenURL string) nerdioPins {
	var p nerdioPins
	if baseURL != "" {
		if u, err := url.Parse(baseURL); err == nil {
			if asciiLower(u.Hostname()) == nerdioPlaceholderHost {
				p.placeholder = true // pins nothing: fail closed, naming the variable
			} else {
				p.api = normalizedOrigin(u)
			}
		}
	}
	if u, err := url.Parse(tokenURL); err == nil && u.User == nil {
		if p.token = normalizedOrigin(u); p.token != "" {
			p.tokenPath = u.EscapedPath()
		}
	}
	return p
}

// check refuses a request to any origin but the pinned ones. An empty pin (a
// URL that does not parse to an origin) matches nothing: fail closed. Nerdio
// authenticates the token request with HTTP Basic, so the token host may see
// the client pair, never the bearer token.
func (p nerdioPins) check(req *http.Request) error {
	o := normalizedOrigin(req.URL)
	if o == "" {
		return fmt.Errorf("%w (%s); set NERDIO_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
	}
	if p.placeholder && p.token != o {
		return fmt.Errorf("%w: %s is the shipped placeholder, not an NMM instance; set NERDIO_BASE_URL to your own NMM instance root (e.g. https://nmm.contoso.com)", ErrOriginChanged, nerdioPlaceholderHost)
	}
	if p.api != "" && o == p.api {
		return nil
	}
	if p.token != "" && o == p.token {
		auth := strings.TrimSpace(req.Header.Get("Authorization"))
		if req.Method == http.MethodPost && req.URL.EscapedPath() == p.tokenPath && (auth == "" || strings.HasPrefix(auth, "Basic ")) {
			return nil
		}
		return fmt.Errorf("%w (%s %s on the token host takes only the token request); set NERDIO_TOKEN_URL to change it", ErrOriginChanged, req.Method, displayOrigin(req.URL))
	}
	return fmt.Errorf("%w (%s); set NERDIO_BASE_URL or NERDIO_TOKEN_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
}

// normalizedOrigin is scheme://host:port with ASCII letters lowercased and
// the default port filled in. It only ever folds spellings the transport
// dials identically, so it is "" (never equal to a pinned origin) for a host
// holding any non-ASCII byte (Unicode case folding and IDNA can make two
// spellings compare equal while the transport dials another name) and for an
// IPv6 host with a zone (Codex review r1 class credential sent to another
// host: a zone is a case-sensitive interface name, so lowercasing it made
// fe80::1%LAN0 match a pin on fe80::1%lan0; no NMM instance or Entra
// endpoint is reached through a link-local zone).
func normalizedOrigin(u *url.URL) string {
	if u == nil || u.Host == "" || !isASCII(u.Scheme) || !isASCII(u.Host) || strings.Contains(u.Host, "%") {
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
	pins nerdioPins
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
	if strings.Contains(u.Host, "%") && isASCII(u.Host) {
		return "a host with an IPv6 zone"
	}
	return "a non-ASCII host"
}

// tenantSecretTripper registers the tenant secrets a request or a response
// carries (cliutil.ShowTenantSecrets): shown to the caller who asked, never
// written to a file, the store or a child's argv. The response credentials the
// kit declares (access_token, refresh_token, id_token) are held by the kit's
// transport underneath (handfixes.json credkit-wiring).
type tenantSecretTripper struct{ next http.RoundTripper }

func (t tenantSecretTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet && nerdioSecureVariablesPath(req.URL.Path) && req.Body != nil {
		// A secure variable write carries the customer secret as "value": it
		// is a tenant secret from here on (shown on the caller's output,
		// withheld from every file and the store), whichever surface sent it
		// (a CLI flag, an MCP nerdio_execute call).
		sent, rerr := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if rerr != nil {
			return nil, rerr
		}
		req.Body = io.NopCloser(bytes.NewReader(sent))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(sent)), nil }
		cliutil.ShowTenantSecrets(sent, "value")
	}
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil || !nerdioTenantSecretPath(req.Method, req.URL.Path) {
		return resp, err
	}
	body, rerr := client.ReadBody(resp.Body)
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	plain, derr := client.DecodeBody(resp.Header.Get("Content-Encoding"), body)
	if derr != nil {
		return resp, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Never cached either (no sync partition, no response cache).
		cliutil.ShowTenantSecrets(plain, nerdioTenantSecretValueNames(req.URL.Path)...)
		return resp, nil
	}
	// An error answer on these paths registers its secret members whatever
	// the status (Codex r2): a 400 echoing a secure variable's "value" was
	// shown and then accepted back as a file-writing argument.
	cliutil.ShowTenantSecretMembers(plain, nerdioTenantSecretValueNames(req.URL.Path)...)
	return resp, nil
}

// nerdioTenantSecretValueNames names the members of a tenant-secret response
// that carry the secret beside the credential-named ones (the LAPS password
// and the BitLocker key are under credential names; a secure variable's
// secret is its "value").
func nerdioTenantSecretValueNames(path string) []string {
	if nerdioSecureVariablesPath(path) {
		return []string{"value"}
	}
	return nil
}

// nerdioTenantSecretPath reports a read whose whole purpose is to return a
// tenant secret to the operator who asked for it by name: a device's LAPS
// local admin password, its BitLocker recovery keys, and secure variable
// values (nerdio handfix tenant-secrets-shown-never-stored). These are the
// customer's secrets, not this connector's credentials: holding them would
// withhold the very answer the command exists to give. None of these paths
// is a sync partition, so the data-source chokepoint never caches them, and
// the response cache is off; every other file sink and the local store
// withhold them (cliutil.ShowTenantSecrets).
func nerdioTenantSecretPath(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	p := strings.TrimRight(path, "/")
	if strings.HasSuffix(p, "/local-admin-password") || strings.HasSuffix(p, "/bitlocker-keys") {
		return strings.HasPrefix(p, "/rest-api/v1-beta/accounts/") && strings.Contains(p, "/devices/")
	}
	return nerdioSecureVariablesPath(p)
}

// nerdioSecureVariablesPath reports the MSP-level or an account's secure
// variables collection (/rest-api/v1/secure-variables,
// /rest-api/v1/accounts/{id}/secure-variables).
func nerdioSecureVariablesPath(path string) bool {
	p := strings.TrimRight(path, "/")
	if p == "/rest-api/v1/secure-variables" {
		return true
	}
	segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	return len(segs) == 5 && segs[0] == "rest-api" && segs[1] == "v1" && segs[2] == "accounts" && segs[4] == "secure-variables"
}
