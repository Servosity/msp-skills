// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// crowdstrike handfixes applied to every API client (CLI, novel commands and MCP)
// through registerClientHook, so no generated file changes:
//
//   - api-origin-pinned: a credential goes only to the scheme, host and port
//     it is configured for. The bearer token and every API request go to the
//     origin of the API base URL (CROWDSTRIKE_BASE_URL or the config file,
//     default https://api.crowdstrike.com). The client ID and secret
//     go only to the token URL the mint uses (client.ResolveCrowdStrikeTokenURL:
//     CROWDSTRIKE_TOKEN_URL, else the base URL's marketplace origin plus
//     /oauth2/token), and when that is another origin it receives nothing but
//     a POST to that exact path that carries no bearer token. Every redirect
//     hop must stay on the origin of the request it follows. Nothing given
//     later (a Location header, an MCP argument, a path) can move a credential
//     to another host. 'auth login' builds its own token client;
//     guardTokenClient pins it to the token URL the same way.
//   - no-response-cache: the on-disk response cache is off. Raw bodies must
//     not sit in cache files; the local store keeps credential fields out
//     (store/crowdstrike_secrets.go).
//   - credkit-wiring: under the pin, every request leaves through the shared
//     credential kit's outbound guard (cliutil.OutboundGuard), which refuses
//     a held value sent anywhere its declaration does not allow and holds the
//     credentials a response carries (Declarations.ResponseFields) before the
//     body reaches any command.
//   - dry-run-sends-nothing: a client built under --dry-run refuses every
//     request at its transport, whatever the command does with it.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"crowdstrike-pp-cli/internal/client"
	"crowdstrike-pp-cli/internal/cliutil"
	"crowdstrike-pp-cli/internal/config"
)

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured CrowdStrike host")

// ErrDryRunSend marks a request refused because the command runs under
// --dry-run.
var ErrDryRunSend = errors.New("--dry-run: request not sent")

func init() {
	registerClientHook(func(c *client.Client) error {
		guardClient(c)
		return nil
	})
}

// crowdstrikePins is what one client may reach: any request to api, and to token
// only the token POST itself.
type crowdstrikePins struct {
	api       string // normalized origin of the API base URL ("" pins nothing)
	token     string // normalized origin of the token URL ("" pins nothing)
	tokenPath string
}

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	if _, ok := c.HTTPClient.Transport.(originPinTripper); ok {
		return // already guarded (a hook ran twice on one client)
	}
	c.NoCache = true
	installPins(c.HTTPClient, newCrowdStrikePins(c.BaseURL, client.ResolveCrowdStrikeTokenURL(c.Config)), c.DryRun)
}

// guardTokenClient pins the token-endpoint client 'auth login' and the fleet
// member_cid mint build to the same two origins every API client has (the
// API base URL's origin, and the token URL's exact POST when the token URL is
// on another host), sends through the credential kit, and refuses every
// request under --dry-run. With the token URL on the API origin (every Falcon
// cloud) a same-origin redirect of the token POST is followed, as the API
// client follows one (the member_cid mint's shipped
// TestMemberCIDMintFollowsASameOriginRedirect); the hop still may not leave
// the origin, so the form carrying the secret never reaches another host.
func guardTokenClient(hc *http.Client, cfg *config.Config, dryRun bool) {
	if hc == nil {
		return
	}
	if _, ok := hc.Transport.(originPinTripper); ok {
		return
	}
	tokenURL := client.ResolveCrowdStrikeTokenURL(cfg)
	api := ""
	if cfg != nil && tokenURL != "" {
		api = strings.TrimSpace(cfg.BaseURL)
	}
	installPins(hc, newCrowdStrikePins(api, tokenURL), dryRun)
}

func installPins(hc *http.Client, pins crowdstrikePins, dryRun bool) {
	next := hc.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	inner := cliutil.OutboundGuard(next) // handfixes.json credkit-wiring: the kit under the origin pin
	if dryRun {
		inner = dryRunTripper{}
	}
	hc.Transport = originPinTripper{pins: pins, next: inner}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := hc.CheckRedirect
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || via[0] == nil || normalizedOrigin(req.URL) == "" || normalizedOrigin(req.URL) != normalizedOrigin(via[0].URL) {
			return fmt.Errorf("%w (a redirect to %s); set CROWDSTRIKE_BASE_URL or CROWDSTRIKE_TOKEN_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
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

func newCrowdStrikePins(baseURL, tokenURL string) crowdstrikePins {
	var p crowdstrikePins
	if baseURL != "" {
		if u, err := url.Parse(baseURL); err == nil {
			p.api = normalizedOrigin(u)
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
// URL that does not parse to an origin) matches nothing: fail closed. CrowdStrike
// takes the client pair in the token request's form body (a Basic header is
// also accepted here), so the token host may see the client pair, never the
// bearer token.
func (p crowdstrikePins) check(req *http.Request) error {
	o := normalizedOrigin(req.URL)
	if o == "" {
		return fmt.Errorf("%w (%s); set CROWDSTRIKE_BASE_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
	}
	if p.api != "" && o == p.api {
		return nil
	}
	if p.token != "" && o == p.token {
		auth := strings.TrimSpace(req.Header.Get("Authorization"))
		if req.Method == http.MethodPost && req.URL.EscapedPath() == p.tokenPath && (auth == "" || strings.HasPrefix(auth, "Basic ")) {
			return nil
		}
		return fmt.Errorf("%w (%s %s on the token host takes only the token request); set CROWDSTRIKE_TOKEN_URL to change it", ErrOriginChanged, req.Method, displayOrigin(req.URL))
	}
	return fmt.Errorf("%w (%s); set CROWDSTRIKE_BASE_URL or CROWDSTRIKE_TOKEN_URL to change hosts", ErrOriginChanged, displayOrigin(req.URL))
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
	pins crowdstrikePins
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
