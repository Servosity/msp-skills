// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (not generated): vCenter session login for the two vSphere JSON
// APIs, plus TLS certificate pinning. Installed on every client (CLI and MCP) by
// the cli-package hook in internal/cli/vsphere_guard.go, so the generated
// client.go is untouched.
//
// The generated client sends HTTP Basic credentials on every request. vCenter
// accepts them only at login, so this transport trades them for a session id:
//   - Automation API (/api/...): POST /api/session with Basic auth; the body is
//     the session id as a JSON string.
//   - VI/JSON API (/sdk/vim25/<release>/...): POST
//     .../SessionManager/<moId>/Login with {"userName","password"}; the session
//     id comes back in the vmware-api-session-id response header. The
//     SessionManager moId is read from the unauthenticated ServiceInstance
//     content.
// Data calls carry only the vmware-api-session-id header. Sessions are kept
// in memory per vCenter origin, user and API, held by the credential kit
// the moment they are minted, and replaced once when vCenter reports them
// expired. They are never written to disk (vsphere handfix
// no-credential-in-files): only the 0600 credential store holds a credential.
// ponytail: one login per process; a long scripted loop of one-shot CLI calls
// opens one vCenter session each (they expire on vCenter's idle timeout), so
// use the MCP server or one process for bulk work.

package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"vsphere-pp-cli/internal/cliutil"
	"vsphere-pp-cli/internal/credkit"
)

// SessionHeader is the vCenter session header for both JSON APIs.
const SessionHeader = "vmware-api-session-id"

// TLSFingerprintEnv pins vCenter certificates: comma-separated SHA-256
// fingerprints (AB:CD:... or plain hex). With pins set, only a leaf
// certificate in the list is accepted; without pins, standard verification.
const TLSFingerprintEnv = "VSPHERE_TLS_FINGERPRINT"

type apiKind string

const (
	kindAutomation apiKind = "automation"
	kindVI         apiKind = "vi"
)

const viPathPrefix = "/sdk/vim25/"

// InstallVsphereTransport wraps the client's HTTP transport with the session
// exchange and, when VSPHERE_TLS_FINGERPRINT is set, certificate pinning.
func (c *Client) InstallVsphereTransport() error {
	if c == nil || c.HTTPClient == nil {
		return errors.New("vsphere: nil client")
	}
	if _, done := c.HTTPClient.Transport.(*sessionTransport); done {
		return nil
	}
	base, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok {
		return errors.New("vsphere: unexpected HTTP transport; cannot install the vCenter session layer")
	}
	pins, err := ParsePins(os.Getenv(TLSFingerprintEnv))
	if err != nil {
		return err
	}
	tr := base.Clone()
	if len(pins) > 0 {
		tr.TLSClientConfig = pinnedTLSConfig(pins)
	}
	st := &sessionTransport{base: OutboundGuard(tr), sessions: map[sessionKey]string{}, refused: map[string]*loginError{}}
	if u, perr := url.Parse(strings.TrimSpace(c.BaseURL)); perr == nil && u.Host != "" {
		st.origin = u
	}
	c.HTTPClient.Transport = st
	return nil
}

type sessionKey struct {
	kind    apiKind
	user    string
	release string // VI/JSON release path segment; empty for the Automation API
}

type sessionTransport struct {
	base   http.RoundTripper
	origin *url.URL // configured vCenter; nil when VSPHERE_BASE_URL is unusable

	mu       sync.Mutex
	sessions map[sessionKey]string
	refused  map[string]*loginError // terminal login refusals, by key + password hash
}

func (t *sessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	user, pass, hasBasic := req.BasicAuth()
	hasCredential := req.Header.Get("Authorization") != "" || req.Header.Get(SessionHeader) != ""
	if t.origin == nil || !sameOrigin(req.URL, t.origin) {
		// Never carry any credential or session id anywhere but the vCenter.
		if hasCredential {
			req = withoutCredentials(req)
		}
		return t.base.RoundTrip(req)
	}
	loopback := isLoopbackHost(t.origin.Hostname())
	if hasCredential && !strings.EqualFold(t.origin.Scheme, "https") && !loopback {
		return refusal(req, http.StatusMisdirectedRequest, "INVALID_ARGUMENT", fmt.Sprintf("refusing to send vCenter credentials over %s; set VSPHERE_BASE_URL to https://<vcenter>", t.origin.Scheme)), nil
	}
	if !hasBasic {
		return t.plain(req) // unauthenticated read (ServiceInstance content) or an explicit session
	}
	if loopback && cliutil.IsAnyHarness() {
		// Printing Press verify/dogfood mock server: it serves the spec's
		// declared Basic scheme directly and has no login endpoints.
		return t.base.RoundTrip(req)
	}
	kind, release := classifyPath(req.URL.Path)
	if kind == "" {
		return t.plain(withoutCredentials(req)) // not an API path: send nothing secret
	}
	if kind == kindVI && strings.HasSuffix(req.URL.Path, "/ServiceInstance/ServiceInstance/content") {
		return t.plain(withoutCredentials(req)) // answers before login
	}
	key := sessionKey{kind: kind, user: user, release: release}
	id, err := t.session(req.Context(), key, pass, "")
	if err != nil {
		return loginFailure(req, err)
	}
	resp, err := t.send(req, id)
	if err != nil {
		return loginFailure(req, err)
	}
	if !sessionRejected(kind, resp) {
		return resp, nil
	}
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return resp, nil // cannot replay the body; surface vCenter's answer
	}
	_ = resp.Body.Close()
	id, err = t.session(req.Context(), key, pass, id)
	if err != nil {
		return loginFailure(req, err)
	}
	if resp, err = t.send(req, id); err != nil {
		return loginFailure(req, err)
	}
	if sessionRejected(kind, resp) {
		// A session created a moment ago was refused too: stop here instead
		// of letting the client's 5xx retry loop log in again and again, and
		// remember it so the rest of a fan-out does not log in either.
		_ = resp.Body.Close()
		le := &loginError{Status: http.StatusUnauthorized, Detail: "vCenter rejected a freshly created session; check the account's permissions and that VSPHERE_BASE_URL points at the vCenter itself"}
		t.mu.Lock()
		if t.sessions[key] == id {
			delete(t.sessions, key)
		}
		t.refused[refusalKey(key, pass)] = le
		t.mu.Unlock()
		return refusal(req, http.StatusUnauthorized, "UNAUTHENTICATED", le.Detail), nil
	}
	return resp, nil
}

func (t *sessionTransport) plain(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return loginFailure(req, err)
	}
	return resp, nil
}

func (t *sessionTransport) send(req *http.Request, id string) (*http.Response, error) {
	clone := withoutCredentials(req)
	clone.Header.Set(SessionHeader, id)
	return t.base.RoundTrip(clone)
}

// pinError is a vCenter certificate outside VSPHERE_TLS_FINGERPRINT.
type pinError struct{ got string }

func (e *pinError) Error() string {
	return fmt.Sprintf("TLS: vCenter certificate fingerprint %s is not in %s; if the certificate was renewed, pin the new one", e.got, TLSFingerprintEnv)
}

// session returns a usable session id. stale, when set, is an id vCenter just
// rejected: it is dropped and a fresh login runs, unless a concurrent request
// already replaced it.
func (t *sessionTransport) session(ctx context.Context, key sessionKey, pass, stale string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if le := t.refused[refusalKey(key, pass)]; le != nil {
		return "", le
	}
	if cur := t.sessions[key]; cur != "" && cur != stale {
		return cur, nil
	}
	delete(t.sessions, key)
	// A refused login is remembered for these exact credentials, so a fan-out
	// of parallel reads cannot retry a wrong password over and over (and lock
	// the account). Transport errors are not remembered.
	refusedKey := refusalKey(key, pass)
	if le := t.refused[refusedKey]; le != nil {
		return "", le
	}
	var id string
	var err error
	if key.kind == kindAutomation {
		id, err = t.loginAutomation(ctx, key.user, pass)
	} else {
		id, err = t.loginVI(ctx, key.release, key.user, pass)
	}
	if err != nil {
		var le *loginError
		if errors.As(err, &le) {
			t.refused[refusedKey] = le
		}
		return "", err
	}
	credkit.Hold("minted:session", id) // Hand-fix (handfixes.json credkit-wiring): a minted session id is a credential
	t.sessions[key] = id
	return id, nil
}

func refusalKey(k sessionKey, pass string) string {
	sum := sha256.Sum256([]byte(pass))
	return strings.Join([]string{string(k.kind), k.user, k.release, hex.EncodeToString(sum[:8])}, "\x00")
}

// loginError is a refused login. Status is what vCenter answered (or 404 when
// the VI/JSON API is absent); Detail never contains credentials.
type loginError struct {
	Status int
	Detail string
}

func (e *loginError) Error() string { return e.Detail }

func (t *sessionTransport) loginAutomation(ctx context.Context, user, pass string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.origin.Scheme+"://"+t.origin.Host+"/api/session", nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(user, pass)
	req.Header.Set("Accept", "application/json")
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", &loginError{Status: resp.StatusCode, Detail: fmt.Sprintf("vCenter login failed (POST /api/session: HTTP %d%s); check VSPHERE_USERNAME (user@domain) and VSPHERE_PASSWORD", resp.StatusCode, faultSuffix(body))}
	}
	var id string
	if json.Unmarshal(body, &id) != nil || strings.TrimSpace(id) == "" {
		return "", &loginError{Status: http.StatusBadGateway, Detail: "vCenter login (POST /api/session) returned no session id"}
	}
	return strings.TrimSpace(id), nil
}

func (t *sessionTransport) loginVI(ctx context.Context, release, user, pass string) (string, error) {
	root := t.origin.Scheme + "://" + t.origin.Host + viPathPrefix + release
	mgr := "SessionManager"
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, root+"/ServiceInstance/ServiceInstance/content", nil); err == nil {
		req.Header.Set("Accept", "application/json")
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return "", err
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return "", &loginError{Status: http.StatusNotFound, Detail: "this vCenter has no VI/JSON API at " + viPathPrefix + release + " (it needs vCenter 8.0 Update 1 or later)"}
		}
		var content struct {
			SessionManager struct {
				Value string `json:"value"`
			} `json:"sessionManager"`
		}
		if resp.StatusCode < 300 && json.Unmarshal(body, &content) == nil && validMoID(content.SessionManager.Value) {
			mgr = content.SessionManager.Value
		}
	}
	payload, _ := json.Marshal(map[string]string{"userName": user, "password": pass})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, root+"/SessionManager/"+url.PathEscape(mgr)+"/Login", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		status := resp.StatusCode
		if strings.Contains(string(body), "InvalidLogin") {
			status = http.StatusUnauthorized
		}
		return "", &loginError{Status: status, Detail: fmt.Sprintf("vCenter VI/JSON login failed (SessionManager Login: HTTP %d%s); check VSPHERE_USERNAME (user@domain) and VSPHERE_PASSWORD", resp.StatusCode, faultSuffix(body))}
	}
	id := strings.TrimSpace(resp.Header.Get(SessionHeader))
	if id == "" {
		return "", &loginError{Status: http.StatusBadGateway, Detail: "vCenter VI/JSON login returned no " + SessionHeader + " header"}
	}
	return id, nil
}

// sessionRejected reports whether vCenter refused the session id itself. The
// Automation API answers 401; VI/JSON answers 401 or a NotAuthenticated fault
// (HTTP 500). The body stays readable for the caller.
func sessionRejected(kind apiKind, resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return true
	}
	if kind != kindVI || resp.StatusCode != http.StatusInternalServerError {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return faultType(body) == "NotAuthenticated"
}

// loginFailure turns a refused login into a vCenter-shaped error response, so
// the generated client reports it as an API error (exit code, no retry loop
// that could lock the account). Transport errors pass through unchanged.
func loginFailure(req *http.Request, err error) (*http.Response, error) {
	var le *loginError
	if errors.As(err, &le) {
		// Never a status the client retries: a retry would log in again.
		// 401 = credentials refused, 404 = no VI/JSON API, 424 = anything
		// else (the original status is in the detail).
		status := http.StatusFailedDependency
		switch le.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			status = http.StatusUnauthorized
		case http.StatusNotFound:
			status = http.StatusNotFound
		}
		return refusal(req, status, "UNAUTHENTICATED", le.Detail), nil
	}
	var pe *pinError
	if errors.As(err, &pe) {
		// 495 (certificate error) is a 4xx without a permission hint.
		return refusal(req, 495, "TLS_PIN_MISMATCH", pe.Error()), nil
	}
	return nil, err
}

// refusal is a synthetic vCenter-shaped error response. The generated client
// reports it as an API error and does not retry a 4xx.
func refusal(req *http.Request, status int, errType, detail string) *http.Response {
	body, _ := json.Marshal(map[string]any{
		"error_type": errType,
		"messages":   []map[string]string{{"id": "vsphere-cli", "default_message": detail}},
	})
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

func faultType(body []byte) string {
	var f struct {
		TypeName string `json:"_typeName"`
	}
	_ = json.Unmarshal(body, &f)
	return f.TypeName
}

// faultSuffix is a short, credential-free description of a vCenter error body.
func faultSuffix(body []byte) string {
	var f struct {
		TypeName    string `json:"_typeName"`
		FaultString string `json:"faultstring"`
		ErrorType   string `json:"error_type"`
	}
	if json.Unmarshal(body, &f) != nil {
		return ""
	}
	parts := []string{}
	for _, s := range []string{f.ErrorType, f.TypeName, f.FaultString} {
		if s = strings.TrimSpace(s); s != "" {
			if len(s) > 160 {
				s = s[:160]
			}
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return ", " + strings.Join(parts, ": ")
}

// classifyPath maps a request path to its API and, for VI/JSON, its release.
func classifyPath(p string) (apiKind, string) {
	if strings.HasPrefix(p, "/api/") {
		return kindAutomation, ""
	}
	if rest, ok := strings.CutPrefix(p, viPathPrefix); ok {
		release, _, _ := strings.Cut(rest, "/")
		if isRelease(release) {
			return kindVI, release
		}
	}
	return "", ""
}

// isRelease accepts a VI/JSON release segment such as 8.0.1.0.
func isRelease(s string) bool {
	if s == "" || len(s) > 16 || s[0] == '.' || s[len(s)-1] == '.' || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r == '.') {
			return false
		}
	}
	return true
}

// validMoID accepts managed-object ids and release strings (vm-12, SessionManager, 8.0.1.0).
func validMoID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' || r == ':') {
			return false
		}
	}
	return true
}

func withoutCredentials(req *http.Request) *http.Request {
	clone := req.Clone(req.Context())
	clone.Header.Del("Authorization")
	clone.Header.Del(SessionHeader)
	if req.GetBody != nil {
		if body, err := req.GetBody(); err == nil {
			clone.Body = body
		}
	}
	return clone
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(hostPort(a), hostPort(b))
}

func hostPort(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if strings.EqualFold(u.Scheme, "http") {
			port = "80"
		} else {
			port = "443"
		}
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// --- TLS certificate pinning ---

// ParsePins parses comma-separated SHA-256 certificate fingerprints. A
// malformed entry is an error: a typo must not silently fall back to
// unpinned verification.
func ParsePins(raw string) (map[string]bool, error) {
	pins := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		fp := normalizeFingerprint(p)
		if fp == "" {
			return nil, fmt.Errorf("%s: %q is not a SHA-256 fingerprint (want 64 hex digits, colons optional)", TLSFingerprintEnv, strings.TrimSpace(p))
		}
		pins[fp] = true
	}
	if strings.TrimSpace(raw) != "" && len(pins) == 0 {
		// A setting with no fingerprint in it (",,") must not quietly turn
		// pinning off.
		return nil, fmt.Errorf("%s is set but contains no SHA-256 fingerprint", TLSFingerprintEnv)
	}
	return pins, nil
}

func pinnedTLSConfig(pins map[string]bool) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Chain verification is replaced by VerifyConnection, which runs on
		// every handshake and accepts only a pinned leaf certificate (vCenter
		// usually serves a self-signed VMCA certificate).
		// #nosec G402 -- VerifyConnection below rejects every unpinned leaf.
		InsecureSkipVerify: true, //nolint:gosec // replaced by VerifyConnection
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("TLS: vCenter sent no certificate")
			}
			got := CertFingerprint(cs.PeerCertificates[0])
			if pins[normalizeFingerprint(got)] {
				return nil
			}
			return &pinError{got: got}
		},
	}
}

// CertFingerprint formats a certificate's SHA-256 as colon-separated hex.
func CertFingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.Raw)
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(sum))
	for i := 0; i < len(h); i += 2 {
		parts = append(parts, h[i:i+2])
	}
	return strings.Join(parts, ":")
}

func normalizeFingerprint(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(":", "", " ", "").Replace(s)
	if len(s) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(s); err != nil {
		return ""
	}
	return s
}
