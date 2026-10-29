// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// umbrella handfixes applied to every API client (CLI, per-customer fan-out
// and MCP) through registerClientHook:
//
//   - api-origin-pinned: every request, redirect hops and token mints
//     included, must go to the scheme, host and port the configuration
//     resolved when the client was built: the API base URL
//     (CISCO_UMBRELLA_BASE_URL or the config file, default
//     https://api.umbrella.com) or the token URL (CISCO_UMBRELLA_TOKEN_URL,
//     default https://api.umbrella.com/auth/v2/token). Nothing given later
//     (a Location header, an MCP argument, a path) can move the API key,
//     secret or bearer token to another host.
//   - no-response-cache: the on-disk response cache is off. Umbrella bodies
//     carry API key secrets, tunnel PSKs and S3 secret keys; the local store
//     keeps them out (store/umbrella_secrets.go), and raw bodies must not sit
//     in cache files.
//   - response-credentials-held: every credential a request body or a
//     response carries is held with the credential kit before the request is
//     sent or the body reaches any command: a minted access token (POST
//     /auth/v2/token, the child-org token included), a new API key's
//     clientSecret (POST /admin/v2/apiKeys, /refresh), the S3 secretAccessKey
//     (POST /admin/v2/iam/rotateKey), a tunnel PSK. A response then has those
//     members replaced by "[redacted]", so a listing that carries a clientId
//     stays readable rather than being withheld whole by the kit.
//   - one-time-secret-refused: a request whose documented response carries a
//     secret Umbrella generates and shows only once is refused before anything
//     leaves. This program withholds that secret, so the only copy would be lost.
//   - credkit-wiring: the kit's outbound guard is the transport under all of
//     this, so it judges exactly what reaches the wire, each redirect hop too.
//
// The token-endpoint client 'auth login' builds (root.go oauthHTTPClient, one
// anchored line) gets the same pin from guardTokenClient.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"cisco-umbrella-pp-cli/internal/client"
	"cisco-umbrella-pp-cli/internal/cliutil"
	"cisco-umbrella-pp-cli/internal/config"
)

// defaultMintEndpoint is the token endpoint the generated client mints against
// when no token URL is configured.
const defaultMintEndpoint = "https://api.umbrella.com/auth/v2/token"

// ErrOriginChanged marks a request refused because it left the pinned origin.
var ErrOriginChanged = errors.New("request refused: it would leave the configured Umbrella API host")

// ErrOneTimeSecret marks a request refused because Umbrella would answer it with
// a newly generated secret, once.
var ErrOneTimeSecret = errors.New("request refused: Umbrella returns this new secret only once and this program withholds it, so the only copy would be lost. Do this in the Umbrella dashboard, where the secret is shown.")

func init() {
	registerClientHook(func(c *client.Client) error {
		guardClient(c)
		return nil
	})
}

func guardClient(c *client.Client) {
	if c == nil || c.HTTPClient == nil {
		return
	}
	c.NoCache = true
	holdConfiguredCredentials(c.Config)
	c.RefuseRequest = refuseOneTimeSecret
	pinClient(c.HTTPClient, pinnedOrigins(c.BaseURL, mintEndpoint(c.Config)))
}

// guardTokenClient pins the token-endpoint client 'auth login' builds to the
// same origins an API client is pinned to, under the kit's outbound guard, and
// holds what the token endpoint answers.
func guardTokenClient(hc *http.Client, cfg *config.Config) {
	if hc == nil {
		return
	}
	base := ""
	if cfg != nil {
		base = strings.TrimRight(cfg.BaseURL, "/")
	}
	pinClient(hc, pinnedOrigins(base, mintEndpoint(cfg)))
}

func mintEndpoint(cfg *config.Config) string {
	if cfg != nil && cfg.TokenURL != "" {
		return cfg.TokenURL
	}
	return defaultMintEndpoint
}

func pinClient(hc *http.Client, origins map[string]bool) {
	next := client.OutboundGuard(hc.Transport)
	next = oneTimeSecretTripper{next: next}
	hc.Transport = originPinTripper{origins: origins, next: responseHoldTripper{next: next}}
	// The generated redirect check may resolve an off-origin hop's host in
	// DNS; a query is itself a disclosure, so the pin runs first.
	generated := hc.CheckRedirect
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := checkPinned(origins, req.URL); err != nil {
			return err
		}
		if generated != nil {
			return generated(req, via)
		}
		return nil
	}
}

type oneTimeSecretRoute int

const (
	notOneTimeSecretRoute oneTimeSecretRoute = iota
	alwaysGeneratesSecret
	tunnelAddRoute
	tunnelKeysRoute
)

// classifyOneTimeSecretRoute matches by suffix on the cleaned, lower-cased path, so
// a base URL with a path, a trailing or doubled slash, or another case is the same
// route. An unparsable URL is not sent: http.NewRequest parses the same string.
func classifyOneTimeSecretRoute(method, rawURL string) oneTimeSecretRoute {
	if !strings.EqualFold(method, http.MethodPost) {
		return notOneTimeSecretRoute
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return notOneTimeSecretRoute
	}
	p := path.Clean(strings.ToLower(u.Path))
	switch {
	case strings.HasSuffix(p, "/admin/v2/apikeys"),
		strings.HasSuffix(p, "/admin/v2/iam/rotatekey"),
		strings.Contains(p, "/admin/v2/apikeys/") && strings.HasSuffix(p, "/refresh"):
		return alwaysGeneratesSecret
	case strings.HasSuffix(p, "/deployments/v2/tunnels"):
		return tunnelAddRoute
	case strings.Contains(p, "/deployments/v2/tunnels/") && strings.HasSuffix(p, "/keys"):
		return tunnelKeysRoute
	}
	return notOneTimeSecretRoute
}

// refuseOneTimeSecret refuses a request that would make Umbrella generate a secret
// it shows only once. A tunnel route goes out only when its body supplies the
// secret, read in the shape the API reads (spec.yaml's request schemas): anything
// else, an unreadable body included, may make Umbrella generate one.
func refuseOneTimeSecret(method, rawURL string, body []byte) error {
	switch classifyOneTimeSecretRoute(method, rawURL) {
	case alwaysGeneratesSecret:
		return ErrOneTimeSecret
	case tunnelAddRoute:
		if m, ok := unambiguousObject(body); ok && nonBlankString(m, "authentication", "parameters", "secret") {
			return nil
		}
		return fmt.Errorf("%w Or choose the secret yourself: --authentication-parameters-secret, or authentication.parameters.secret in the body.", ErrOneTimeSecret)
	case tunnelKeysRoute:
		if m, ok := unambiguousObject(body); ok && m["autoRotate"] == false && nonBlankString(m, "psk", "secret") {
			return nil
		}
		return fmt.Errorf("%w Or choose the secret yourself: --auto-rotate false with --psk-secret, or autoRotate false and psk.secret in the body.", ErrOneTimeSecret)
	}
	return nil
}

// unambiguousObject decodes a JSON object no server can read two ways: exactly one
// document, no member repeated at any depth, in any case.
func unambiguousObject(body []byte) (map[string]any, bool) {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil || m == nil {
		return nil, false
	}
	return m, noRepeatedMembers(json.NewDecoder(bytes.NewReader(body)))
}

func noRepeatedMembers(dec *json.Decoder) bool {
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	switch tok {
	case json.Delim('{'):
		seen := map[string]bool{}
		for dec.More() {
			name, err := dec.Token()
			if err != nil {
				return false
			}
			key := strings.ToLower(fmt.Sprint(name))
			if seen[key] || !noRepeatedMembers(dec) {
				return false
			}
			seen[key] = true
		}
	case json.Delim('['):
		for dec.More() {
			if !noRepeatedMembers(dec) {
				return false
			}
		}
	default:
		return true
	}
	_, err = dec.Token()
	return err == nil
}

func nonBlankString(m map[string]any, keys ...string) bool {
	for _, k := range keys[:len(keys)-1] {
		m, _ = m[k].(map[string]any)
	}
	s, _ := m[keys[len(keys)-1]].(string)
	return strings.TrimSpace(s) != ""
}

// oneTimeSecretTripper refuses on every hop, so a same-origin redirect into a
// refused route is refused too. client.doInternal has already refused a request
// aimed there, before its token mint.
type oneTimeSecretTripper struct{ next http.RoundTripper }

func (t oneTimeSecretTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	route := classifyOneTimeSecretRoute(req.Method, req.URL.String())
	if (route == tunnelAddRoute || route == tunnelKeysRoute) && req.Body != nil {
		read, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err == nil {
			body = read // A read that failed leaves nil, which these routes refuse.
		}
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(bytes.NewReader(read))
	}
	if err := refuseOneTimeSecret(req.Method, req.URL.String(), body); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(req)
}

// holdConfiguredCredentials holds every credential a client was configured with:
// the key and secret, the stored token and the built Authorization header, before
// anything can print them.
func holdConfiguredCredentials(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for _, v := range []string{cfg.CiscoUmbrellaApiKey, cfg.CiscoUmbrellaApiSecret, cfg.ClientID, cfg.ClientSecret, cfg.AccessToken, cfg.RefreshToken, cfg.AuthHeaderVal, cfg.AuthHeader()} {
		if strings.TrimSpace(v) != "" {
			cliutil.Hold("minted:authorization", v)
		}
	}
}

// pinnedOrigins returns the normalized origin of each configured URL; an
// unparseable one pins nothing (its requests are refused).
func pinnedOrigins(urls ...string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range urls {
		if u, err := url.Parse(raw); err == nil {
			if o := normalizedOrigin(u); o != "" {
				out[o] = true
			}
		}
	}
	return out
}

func checkPinned(origins map[string]bool, u *url.URL) error {
	if o := normalizedOrigin(u); o == "" || !origins[o] {
		return fmt.Errorf("%w (%s); set CISCO_UMBRELLA_BASE_URL or CISCO_UMBRELLA_TOKEN_URL to change hosts; a token URL must stay on the API base's host or Cisco's", ErrOriginChanged, displayOrigin(u))
	}
	return nil
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
	origins map[string]bool
	next    http.RoundTripper
}

func (t originPinTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkPinned(t.origins, req.URL); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.next.RoundTrip(req)
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

// responseHoldTripper reads each response (under the client's size cap),
// decodes a copy as the client will (Content-Encoding, then BOM and JSONP/XSSI
// prefixes), holds its credential values with the one shared walker
// (cliutil.HoldCredentialFields: any length, masks excluded) and replaces them
// by name before any command reads it. A token exchange is left whole: the client
// needs the token it just minted, which is held all the same. Request bodies and
// params are held in the client itself (doInternal), before a --dry-run preview
// or a send.
type responseHoldTripper struct{ next http.RoundTripper }

func (t responseHoldTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
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
	cliutil.HoldCredentialFields(plain)
	if isTokenExchange(req) {
		return resp, nil
	}
	if redacted := cliutil.RedactBody(plain); !bytes.Equal(redacted, plain) {
		resp.Body = io.NopCloser(bytes.NewReader(redacted))
		resp.ContentLength = int64(len(redacted))
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
	}
	return resp, nil
}

// isTokenExchange reports an OAuth token request: the only form POST this program
// sends (the Umbrella API takes JSON), whatever path the token URL names.
func isTokenExchange(req *http.Request) bool {
	return req.Method == http.MethodPost && strings.HasPrefix(strings.ToLower(req.Header.Get("Content-Type")), "application/x-www-form-urlencoded")
}
