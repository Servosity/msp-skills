// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: every credential decision is made against one identity
// (internal/jamfauth), and client credentials are sent the way Jamf Pro
// documents them.

package cli

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"jamf-pro-pp-cli/internal/client"
	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/config"
	"jamf-pro-pp-cli/internal/credkit"
	"jamf-pro-pp-cli/internal/jamfauth"
)

// Every client, CLI or MCP, is built through this hook. It checks the client
// against the process identity with assess, drops every cached credential
// not bound to it, and installs a transport that only talks to that
// identity's server, only posts credentials to its token URL, and binds each
// minted token to it.
//
// Jamf Pro's token endpoint (POST /api/v1/oauth/token, vendor schema
// OAuthTokenRequest) requires client_id and client_secret as form fields;
// Jamf's own SDK (macadmins/jamf-pro-sdk-python clients/auth.py) sends them
// that way. The generated client sends them as HTTP Basic auth, so the
// transport moves them into the form body and drops the Authorization header
// (RFC 6749 section 2.3 allows one client authentication method per request).
func init() {
	registerClientHook(func(c *client.Client) error {
		if c.HTTPClient == nil {
			c.HTTPClient = &http.Client{Transport: credkit.Transport(nil)}
		}
		// The kit's outbound guard is the transport under all of this, so it
		// judges exactly what reaches the wire (handfixes.json credkit-wiring).
		t := jamfTransport{}
		if prev, ok := c.HTTPClient.Transport.(jamfTransport); ok {
			t.next = prev.next
		} else {
			t.next = client.OutboundGuard(c.HTTPClient.Transport)
		}
		// jamf-pro handfix (no-response-cache): raw bodies never sit in cache
		// files; --dry-run sends nothing (dry-run-sends-nothing).
		c.NoCache = true
		t.dryRun = c.DryRun
		if c.Config != nil {
			id, err := identityFor(c.Config, c.BaseURL)
			if err != nil {
				return err
			}
			t.id, t.marker = &id, tokenMarkerPath(c.Config)
		}
		c.HTTPClient.Transport = t
		return nil
	})
}

// assessment is what a config may use when acting as id.
type assessment struct {
	id                  jamfauth.Identity
	keepPair, keepToken bool
}

// assess decides, without changing cfg, which identity a client built from
// cfg acts as and which of cfg's cached credentials are bound to it. With an
// established process identity the answer must be that identity: a config
// that resolves to another server or client is refused, and the cached
// credentials are judged against the established value, never re-derived.
func assess(cfg *config.Config, base string) (assessment, error) {
	resolved := client.ResolvedBaseURL(base, cfg.TemplateVars)
	server, err := jamfauth.Origin(resolved)
	if err != nil {
		return assessment{}, err
	}
	est, established := jamfauth.Established()
	if !established && strings.TrimSpace(jamfauth.Selected()) != "" && !jamfauth.Waived() {
		return assessment{}, fmt.Errorf("%s=%s is set but this process was not pinned to that instance", jamfauth.SelectEnv, jamfauth.Selected())
	}
	if established && server != est.Server() {
		return assessment{}, fmt.Errorf("this process acts as %s; a client for %s is refused", est.Server(), server)
	}
	var a assessment
	clientID := ""
	if jamfauth.PairFromEnv() {
		clientID, _ = jamfauth.Pair(cfg)
	} else if cfg.ClientID != "" || cfg.ClientSecret != "" {
		owner, err := jamfauth.New(server, cfg.ClientID)
		if err != nil {
			return assessment{}, err
		}
		if bound(pairMarkerPath(cfg), owner.Bind(cfg.ClientSecret)) && (!established || owner.Same(est)) {
			a.keepPair, clientID = true, cfg.ClientID
		}
	}
	if a.id, err = jamfauth.New(server, clientID); err != nil {
		return assessment{}, err
	}
	if established {
		if !a.id.Same(est) {
			return assessment{}, fmt.Errorf("this process acts as client %q on %s; the configuration would act as client %q", est.ClientID(), est.Server(), clientID)
		}
		a.id = est
	}
	if cred := firstSet(cfg.AuthHeaderVal, cfg.AccessToken); cred != "" {
		a.keepToken = bound(tokenMarkerPath(cfg), a.id.Bind(cred)) || headerWrittenWithServer(cfg, server)
	}
	return a, nil
}

// headerWrittenWithServer reports an auth_header an operator wrote by hand next
// to the base_url it belongs to, and that base_url is the server this client
// acts for. A saved credential has a marker and is judged by it alone, so one
// saved for another server never passes here (handfixes.json
// doctor-verdict-tracks-reality).
func headerWrittenWithServer(cfg *config.Config, server string) bool {
	if cfg.AuthHeaderVal == "" || cfg.Path == "" {
		return false
	}
	if _, err := os.Stat(filepath.Clean(tokenMarkerPath(cfg))); err == nil {
		return false
	}
	data, err := os.ReadFile(filepath.Clean(cfg.Path))
	if err != nil {
		return false
	}
	var file struct {
		AuthHeader string `toml:"auth_header"`
		BaseURL    string `toml:"base_url"`
	}
	if toml.Unmarshal(data, &file) != nil || file.AuthHeader != cfg.AuthHeaderVal {
		return false
	}
	origin, err := jamfauth.Origin(file.BaseURL)
	return err == nil && origin == server
}

// identityFor applies assess to a client's config: what is not bound to the
// identity is removed, so it can only act as that identity.
func identityFor(cfg *config.Config, base string) (jamfauth.Identity, error) {
	a, err := assess(cfg, base)
	if err != nil {
		return jamfauth.Identity{}, err
	}
	dropCredentialHeaders(cfg)
	if !a.keepPair && !jamfauth.PairFromEnv() {
		cfg.ClientID, cfg.ClientSecret = "", ""
	}
	if !a.keepToken {
		cfg.AuthHeaderVal, cfg.AccessToken, cfg.RefreshToken, cfg.TokenExpiry = "", "", "", time.Time{}
	}
	return a.id, nil
}

// jamfLoginNet, set only by tests, replaces the network under the login client.
var jamfLoginNet http.RoundTripper

// jamfLoginClient is the HTTP client 'auth login' mints with (hand-fix in
// auth.go): the same identity-checked transport, for the identity login is
// creating (server, the client id it was given). It wraps the generated
// OAuth client, so the TLS flags and the dial guard still apply underneath.
func jamfLoginClient(flags *rootFlags, cfg *config.Config, clientID string) (*http.Client, error) {
	id, err := jamfauth.New(client.ResolvedBaseURL(cfg.BaseURL, cfg.TemplateVars), clientID)
	if err != nil {
		return nil, err
	}
	hc, err := flags.oauthHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	next := client.OutboundGuard(hc.Transport)
	if jamfLoginNet != nil {
		next = jamfLoginNet
	}
	hc.Transport = jamfTransport{next: next, id: &id, marker: tokenMarkerPath(cfg), dryRun: flags.dryRun}
	return hc, nil
}

// bindSavedCredentials records what 'auth login' and 'auth set-token' just
// saved (hand-fixes in auth.go): the exact token, bound to the identity of
// tokenClientID, and a persisted client pair, bound to its own identity.
func bindSavedCredentials(cfg *config.Config, token, tokenClientID, pairID, pairSecret string) {
	resolved := client.ResolvedBaseURL(cfg.BaseURL, cfg.TemplateVars)
	if id, err := jamfauth.New(resolved, tokenClientID); err == nil && token != "" {
		_ = writeBinding(tokenMarkerPath(cfg), id.Bind(token))
	}
	if id, err := jamfauth.New(resolved, pairID); err == nil && pairID != "" && pairSecret != "" {
		_ = writeBinding(pairMarkerPath(cfg), id.Bind(pairSecret))
	}
}

// pairID is the client id the next client built from cfg mints as.
func pairID(cfg *config.Config) string {
	id, _ := jamfauth.Pair(cfg)
	return id
}

func tokenMarkerPath(cfg *config.Config) string { return markerPath(cfg, ".token-identity") }
func pairMarkerPath(cfg *config.Config) string  { return markerPath(cfg, ".client-identity") }

func markerPath(cfg *config.Config, suffix string) string {
	if cfg.Path == "" {
		return ""
	}
	return cfg.Path + suffix
}

func bound(marker, binding string) bool {
	if marker == "" {
		return false
	}
	b, err := os.ReadFile(filepath.Clean(marker))
	return err == nil && strings.TrimSpace(string(b)) == binding
}

// writeBinding replaces the marker atomically, so a concurrent reader sees
// the old or the new binding, never a partial one.
func writeBinding(marker, binding string) error {
	if marker == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(marker), filepath.Base(marker)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := credkit.WriteFile(tmp.Name(), []byte(binding+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), marker)
}

// dropCredentialHeaders removes credential headers from the config's
// [headers] table: the client sets them after Authorization, so they would
// override the bound token on every server. Jamf Pro authenticates only with
// the OAuth bearer, so nothing legitimate is lost.
func dropCredentialHeaders(cfg *config.Config) {
	for k := range cfg.Headers {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "authorization", "proxy-authorization", "cookie":
			delete(cfg.Headers, k)
		}
	}
}

// jamfTransport enforces the identity on the wire.
type jamfTransport struct {
	next   http.RoundTripper
	id     *jamfauth.Identity // nil only for a client built without a config
	marker string
	dryRun bool // refuse every request: the command runs under --dry-run
}

// postsCredentials reports a request that carries a client credential:
// HTTP Basic, a form body, or a token-like path. It is judged by what the
// request carries, not by an exact path, so a variant spelling of the token
// URL is still recognised (and then refused by CheckMint).
func postsCredentials(req *http.Request) bool {
	if _, _, ok := req.BasicAuth(); ok {
		return true
	}
	if req.Method != http.MethodPost {
		return false
	}
	p := strings.ToLower(path.Clean("/" + req.URL.Path))
	return strings.Contains(p, "/oauth") || strings.HasSuffix(p, "/token") ||
		strings.HasPrefix(strings.ToLower(req.Header.Get("Content-Type")), "application/x-www-form-urlencoded")
}

func (t jamfTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.dryRun {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, &requestError{msg: "--dry-run: request not sent"}
	}
	// Anything that changes the destination after the identity was resolved
	// (a {server} filled by --path-context or a tool input, a redirect) would
	// carry this identity's credentials elsewhere.
	if t.id != nil && !t.id.Allows(req.URL) {
		return nil, fmt.Errorf("request to %s refused: this client acts as %s; choose the server with JAMF_PRO_SERVER or --instance", req.URL.Host, t.id.Server())
	}
	if !postsCredentials(req) {
		resp, err := t.next.RoundTrip(req)
		if err != nil {
			return nil, apiRequestError(err)
		}
		// No request follows a redirect: refused here, before http.Client
		// reads Location (whose text Go would quote in its own errors).
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			_ = resp.Body.Close()
			return nil, redirectRefused(resp.StatusCode, "")
		}
		if err := sanitizeResponse(resp); err != nil {
			return nil, err
		}
		holdResponseCredentials(resp)
		return guardBody(resp), nil
	}
	var raw []byte
	if req.Body != nil {
		var err error
		raw, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
	}
	form, _ := url.ParseQuery(string(raw))
	clientID, secret, basic := req.BasicAuth()
	if !basic {
		clientID, secret = form.Get("client_id"), form.Get("client_secret")
	}
	cliutil.Hold("field:client_secret", secret)
	if t.id != nil {
		if err := t.id.CheckMint(req.URL, clientID); err != nil {
			return nil, err
		}
	}
	clone := req.Clone(req.Context())
	body := raw
	if form.Get("grant_type") == "client_credentials" && basic {
		form.Set("client_id", clientID)
		form.Set("client_secret", secret)
		clone.Header.Del("Authorization")
		clone.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		body = []byte(form.Encode())
	}
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.ContentLength = int64(len(body))
	clone.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	resp, err := t.next.RoundTrip(clone)
	if err != nil {
		// A transport error can quote what the server sent (a malformed
		// status line, for one): never pass its text on.
		return nil, credentialRequestError(0, err)
	}
	// A credential request never follows a redirect: the next hop would be a
	// new request outside this rule (its body unsanitised, its target
	// unchecked). Returning an error stops http.Client from following it.
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_ = resp.Body.Close()
		return nil, redirectRefused(resp.StatusCode, "token request failed: ")
	}
	// Credential responses: headers and encoding here, the body by
	// tokenError (which keeps the RFC 6749 code) or by canonicalToken below.
	if err := sanitizeHeaders(resp); err != nil {
		return nil, err
	}
	if err := decodeBody(resp); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return tokenError(resp)
	}
	if form.Get("grant_type") != "client_credentials" {
		return guardBody(resp), nil
	}
	return canonicalToken(resp, t.id, t.marker)
}

// canonicalToken checks a 200 token response completely and hands the
// generated parsers a re-encoded copy: no byte the server wrote can reach a
// parse error. access_token must be a non-empty string without control
// characters, expires_in (if present) a whole number of seconds up to a
// year. The new token is bound to the identity before the client saves it.
func canonicalToken(resp *http.Response, id *jamfauth.Identity, marker string) (*http.Response, error) {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if err != nil {
		return nil, credentialRequestError(resp.StatusCode, err)
	}
	var tok struct {
		AccessToken string      `json:"access_token"`
		ExpiresIn   json.Number `json:"expires_in"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	invalid := &requestError{msg: "token request failed: the token response is not valid"}
	if dec.Decode(&tok) != nil || tok.AccessToken == "" || len(tok.AccessToken) > 8192 || strings.IndexFunc(tok.AccessToken, func(r rune) bool { return r < 0x21 || r == 0x7f }) >= 0 {
		return nil, invalid
	}
	out := map[string]any{"access_token": tok.AccessToken, "token_type": "Bearer"}
	if tok.ExpiresIn != "" {
		n, err := tok.ExpiresIn.Int64()
		if err != nil || n < 0 || n > 366*24*3600 {
			return nil, invalid
		}
		out["expires_in"] = n
	}
	body, _ := json.Marshal(out)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Del("Content-Encoding")
	cliutil.Hold("minted:access_token", tok.AccessToken) // Hand-fix (handfixes.json credkit-wiring)
	if id != nil && marker != "" {
		_ = writeBinding(marker, id.Bind(tok.AccessToken))
	}
	return resp, nil
}

// tokenError replaces a non-200 credential response body before the
// generated client quotes it in an error. Deny by default: nothing the token
// endpoint wrote is printed except its OAuth error code when that is one of
// the six RFC 6749 codes ({"error":"invalid_client"}); the status stays.
func tokenError(resp *http.Response) (*http.Response, error) {
	raw, rerr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, credentialRequestError(resp.StatusCode, rerr)
	}
	var e struct {
		Error string `json:"error"`
	}
	out := "{}"
	if json.Unmarshal(raw, &e) == nil && jamfauth.OAuthErrorCodes[e.Error] {
		out = `{"error":"` + e.Error + `"}`
	}
	resp.Body = io.NopCloser(strings.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Del("Content-Encoding")
	return guardBody(resp), nil
}

// requestError replaces a transport or read error. Its text is fixed, never
// bytes a server sent; Unwrap keeps the original, so errors.Is and errors.As
// (timeouts, DNS failures, retries) still see it.
type requestError struct {
	msg string
	err error
}

func (e *requestError) Error() string { return e.msg }
func (e *requestError) Unwrap() error { return e.err }

// credentialRequestError is every failure of a credential request: the
// HTTP status if one arrived, else nothing but "token request failed".
func credentialRequestError(status int, err error) error {
	msg := "token request failed"
	if status > 0 {
		msg = fmt.Sprintf("token request failed: HTTP %d", status)
	}
	return &requestError{msg: msg, err: err}
}

// redirectRefused is the error for any 3xx answer. Jamf Pro and Classic
// API calls do not rely on redirects, and a followed redirect would leave
// every rule here (target, credentials, error text), so none is followed.
func redirectRefused(status int, prefix string) error {
	return &requestError{msg: fmt.Sprintf("%sthe server answered HTTP %d (a redirect); redirects are refused, so check the server URL (JAMF_PRO_SERVER, or the instance's server in 'instance add')", prefix, status)}
}

// apiRequestError is a transport failure of any other request, named by
// its kind only.
func apiRequestError(err error) error {
	var (
		netErr  net.Error
		dnsErr  *net.DNSError
		opErr   *net.OpError
		certErr *tls.CertificateVerificationError
		unknown x509.UnknownAuthorityError
		host    x509.HostnameError
	)
	msg := "request failed"
	switch {
	case errors.Is(err, context.Canceled):
		msg = "request canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		msg = "request timed out"
	case errors.As(err, &dnsErr):
		msg = "could not resolve the server host"
	case errors.As(err, &certErr), errors.As(err, &unknown), errors.As(err, &host):
		msg = "the server's TLS certificate is not trusted"
	case errors.As(err, &opErr):
		msg = "could not connect to the server"
	}
	return &requestError{msg: msg, err: err}
}

// guardBody makes every read of a response body fail with fixed text: a
// body-read error can quote bytes the server sent (a malformed chunk
// trailer, for one), and the generated client reads API bodies after the
// transport has returned. io.EOF passes through untouched; any other error
// becomes "reading the response failed", with Unwrap keeping the cause.
func guardBody(resp *http.Response) *http.Response {
	if resp != nil && resp.Body != nil {
		resp.Body = guardedBody{resp.Body}
	}
	return resp
}

type guardedBody struct{ rc io.ReadCloser }

func (g guardedBody) Read(p []byte) (int, error) {
	n, err := g.rc.Read(p)
	if err != nil && err != io.EOF {
		err = &requestError{msg: "reading the response failed", err: err}
	}
	return n, err
}

func (g guardedBody) Close() error { return g.rc.Close() }

// sanitizeResponse keeps server-chosen text out of every message built from
// a response. Headers the generated client reads are normalised: an
// unsupported Content-Encoding is refused with fixed text (the generated
// decoder would quote it), Content-Type is reduced to a clean media type,
// Retry-After kept only as seconds or an HTTP date, and request-id headers
// dropped. An error body (status 400 or more) is replaced by the status and
// Jamf's machine error codes and field names, never its description.
func sanitizeResponse(resp *http.Response) error {
	if err := sanitizeHeaders(resp); err != nil {
		return err
	}
	if err := decodeBody(resp); err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return sanitizeErrorBody(resp)
	}
	return checkSuccessBody(resp)
}

// checkSuccessBody refuses a 2xx body the generated client would turn into
// an error that quotes it: an HTML page where an API response was expected
// (its title would be printed) or JSON that does not parse, or holds a
// number no float64 can hold (the parse error would quote the bytes). The
// spec declares no HTML response, so no legitimate call is affected.
func checkSuccessBody(resp *http.Response) error {
	if resp.Body == nil || resp.Body == http.NoBody {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBodyBytes)+1))
	_ = resp.Body.Close()
	if err != nil {
		return &requestError{msg: "reading the response failed", err: err}
	}
	if len(raw) > maxBodyBytes {
		return &requestError{msg: "the response is too large; refused"}
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/html") || looksLikeHTMLPage(raw, ct) {
		return &requestError{msg: "the server answered with an HTML page where an API response was expected; check the server URL"}
	}
	if (ct == "application/json" || strings.HasPrefix(ct, "application/json;") || strings.HasSuffix(strings.SplitN(ct, ";", 2)[0], "+json")) && len(bytes.TrimSpace(raw)) > 0 && !finiteJSON(raw) {
		return &requestError{msg: "the server's response is not valid JSON; refused"}
	}
	// jamf-pro handfix one-rule-everywhere: a body that is not a JSON object
	// or array (a script, CSV or XML download, one bare JSON string) has no fields for the one
	// credential rule to redact one by one, so it is judged whole, and one
	// that carries a credential is withheld whole here, before any command
	// prints, delivers, stores or returns it.
	if t := bytes.TrimSpace(raw); len(t) > 0 && (!json.Valid(t) || (t[0] != '{' && t[0] != '[')) && cliutil.DocumentCarriesCredential(raw) {
		return &requestError{msg: "the response was withheld: it is a document that carries a credential (a password, key or token written in it), which this CLI never prints or saves"}
	}
	return nil
}

// looksLikeHTMLPage mirrors the generated client's HTML detection, so any
// body it would summarise (with the page title) is refused first. A response
// labelled XML is an HTML page only if it starts as one: a documented XML
// template can hold <scriptContentsMac> or <title> (jamf-pro handfix
// xml-template-is-not-html).
func looksLikeHTMLPage(raw []byte, contentType string) bool {
	sample := strings.ToLower(strings.TrimSpace(string(raw[:min(len(raw), 4096)])))
	if strings.HasPrefix(sample, "<!doctype html") || strings.HasPrefix(sample, "<html") {
		return true
	}
	if strings.HasPrefix(contentType, "application/xml") || strings.HasPrefix(contentType, "text/xml") {
		return false
	}
	if len(sample) > 2048 {
		sample = sample[:2048]
	}
	return strings.HasPrefix(sample, "<") && (strings.Contains(sample, "<body") || strings.Contains(sample, "<head") || strings.Contains(sample, "<title") || strings.Contains(sample, "<script"))
}

// finiteJSON reports a single well-formed JSON value whose numbers all fit
// a float64.
func finiteJSON(raw []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return true
		}
		if err != nil {
			return false
		}
		if n, ok := tok.(json.Number); ok {
			if _, err := strconv.ParseFloat(string(n), 64); err != nil {
				return false
			}
		}
	}
}

// sanitizeHeaders is the header half of sanitizeResponse. Deny by default:
// only the headers the CLI needs survive, each in a form that carries no
// server text (generated formatters quote some headers, cf-mitigated for
// one, in errors). An unsupported Content-Encoding is refused with fixed
// text, since the generated decoder would quote it.
func sanitizeHeaders(resp *http.Response) error {
	for _, enc := range strings.Split(resp.Header.Get("Content-Encoding"), ",") {
		switch strings.ToLower(strings.TrimSpace(enc)) {
		case "", "identity", "gzip", "x-gzip", "deflate":
		default:
			_ = resp.Body.Close()
			return &requestError{msg: "the server answered with an unsupported content encoding; refused"}
		}
	}
	kept := http.Header{}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		// Constants only: a server-chosen string kept because it looks
		// harmless can be a secret of the same shape.
		clean := "application/octet-stream"
		if mt, params, err := mime.ParseMediaType(ct); err == nil {
			if strings.HasSuffix(mt, "+json") {
				mt = "application/json"
			}
			if knownMediaTypes[mt] {
				clean = mt
				if cs := strings.ToLower(params["charset"]); knownCharsets[cs] {
					clean += "; charset=" + cs
				}
			}
		}
		kept.Set("Content-Type", clean)
	}
	if enc := strings.TrimSpace(resp.Header.Get("Content-Encoding")); enc != "" {
		kept.Set("Content-Encoding", strings.ToLower(enc))
	}
	if ra := strings.TrimSpace(resp.Header.Get("Retry-After")); ra != "" {
		if _, err := http.ParseTime(ra); err == nil || retryAfterSeconds.MatchString(ra) {
			kept.Set("Retry-After", ra)
		}
	}
	if cl := resp.Header.Get("Content-Length"); retryAfterSeconds.MatchString(cl) || digitsOnly.MatchString(cl) {
		kept.Set("Content-Length", cl)
	}
	for _, h := range []string{"Date", "Last-Modified", "Expires"} {
		if v := resp.Header.Get(h); v != "" {
			if _, err := http.ParseTime(v); err == nil {
				kept.Set(h, v)
			}
		}
	}
	for _, h := range []string{"Etag", "Cache-Control"} {
		if v := resp.Header.Get(h); cacheValue.MatchString(v) {
			kept.Set(h, v)
		}
	}
	resp.Header = kept
	return nil
}

// sanitizeErrorBody is the body half of sanitizeResponse: an error body
// (status 400 or more) is replaced by its status alone. Codes and field
// names are server-chosen strings too, so none is kept.
func sanitizeErrorBody(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	body := []byte(fmt.Sprintf(`{"httpStatus":%d}`, resp.StatusCode))
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Type", "application/json")
	return nil
}

var (
	retryAfterSeconds = regexp.MustCompile(`^[0-9]{1,6}$`)
	digitsOnly        = regexp.MustCompile(`^[0-9]{1,15}$`)
	cacheValue        = regexp.MustCompile(`^[A-Za-z0-9 ,=/"_.:-]{1,128}$`)
)

// maxBodyBytes caps every response body this transport reads, encoded or
// not (a variable so tests can lower it).
var maxBodyBytes = 256 << 20

// decodeBody removes the content encoding here, before any check: every
// check then sees the bytes the generated client would see, and the client
// receives a plain body (no Content-Encoding) it never decodes itself.
// Encodings are undone last-applied first; a body that does not decode, or
// decodes past the cap, is refused with fixed text.
func decodeBody(resp *http.Response) error {
	enc := resp.Header.Get("Content-Encoding")
	if enc == "" || resp.Body == nil || resp.Body == http.NoBody {
		resp.Header.Del("Content-Encoding")
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBodyBytes)+1))
	_ = resp.Body.Close()
	if err != nil {
		return &requestError{msg: "reading the response failed", err: err}
	}
	tokens := strings.Split(enc, ",")
	for i := len(tokens) - 1; i >= 0; i-- {
		var r io.Reader
		switch strings.ToLower(strings.TrimSpace(tokens[i])) {
		case "", "identity":
			continue
		case "gzip", "x-gzip":
			zr, err := gzip.NewReader(bytes.NewReader(raw))
			if err != nil {
				return &requestError{msg: "the response could not be decoded; refused", err: err}
			}
			r = zr
		case "deflate":
			// zlib-wrapped first; a stream whose zlib decode fails for any
			// reason but size is retried as raw DEFLATE.
			if out, err := readCapped(zlibReader(raw)); err == nil || errors.Is(err, errTooLarge) {
				if err != nil {
					return &requestError{msg: "the decoded response is too large; refused"}
				}
				raw = out
				continue
			}
			r = flate.NewReader(bytes.NewReader(raw))
		default:
			return &requestError{msg: "the server answered with an unsupported content encoding; refused"}
		}
		out, err := readCapped(r)
		if errors.Is(err, errTooLarge) {
			return &requestError{msg: "the decoded response is too large; refused"}
		}
		if err != nil {
			return &requestError{msg: "the response could not be decoded; refused", err: err}
		}
		raw = out
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	resp.ContentLength = int64(len(raw))
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Length", strconv.Itoa(len(raw)))
	return nil
}

var errTooLarge = errors.New("body over the size cap")

// readCapped reads r to the end, failing past maxBodyBytes.
func readCapped(r io.Reader) ([]byte, error) {
	out, err := io.ReadAll(io.LimitReader(r, int64(maxBodyBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(out) > maxBodyBytes {
		return nil, errTooLarge
	}
	return out, nil
}

// zlibReader opens a zlib stream, or a reader that fails if the header is bad.
func zlibReader(raw []byte) io.Reader {
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return errReader{err}
	}
	return zr
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

var (
	knownMediaTypes = map[string]bool{"application/json": true, "application/xml": true, "text/xml": true,
		"text/plain": true, "text/html": true, "text/csv": true, "application/octet-stream": true}
	knownCharsets = map[string]bool{"utf-8": true, "us-ascii": true, "iso-8859-1": true}
)

// holdResponseCredentials applies the one rule to a sanitized 2xx JSON body
// before any command reads it: its credentials are held and redacted here,
// whole, so no command that splits a body by line or by record can separate a
// label from its value (jamf-pro handfix: response-credentials-held).
// sanitizeResponse has already decoded the body and buffered it in memory.
func holdResponseCredentials(resp *http.Response) {
	if resp == nil || resp.Body == nil || resp.Body == http.NoBody || resp.StatusCode >= 300 {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBodyBytes)+1))
	_ = resp.Body.Close()
	if err == nil {
		raw = cliutil.RedactJSONBody(raw)
		resp.ContentLength = int64(len(raw))
		resp.Header.Del("Content-Length")
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
}
