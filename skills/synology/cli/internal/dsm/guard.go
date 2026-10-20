// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"synology-pp-cli/internal/termio"
)

// --- redirects --------------------------------------------------------------

// RedirectError is a DSM request answered with a redirect. DSM's WebAPI never
// redirects, so no DSM request (the SYNO.API.Info probe, a login, an
// authenticated read or write, a logout) ever follows one, same origin
// included: a NAS (or anything on the path) answering an authenticated read
// with 302 Location: /webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=backup
// would otherwise have the client carry the session and SynoToken into a
// backup start through a read-only tool. Status is the HTTP status the NAS
// answered; the Location is never kept, because it can carry anything.
type RedirectError struct{ Status int }

func (e *RedirectError) Error() string {
	what := "a redirect"
	if e.Status > 0 {
		what = fmt.Sprintf("a redirect (status %d)", e.Status)
	}
	return "the NAS answered with " + what + "; DSM's WebAPI never redirects, so it was not followed - check the NAS address (SYNOLOGY_BASE_URL, or the fleet entry's --url); if DSM redirects http:// to https://, use the https:// address"
}

// Is makes errors.Is(err, ErrRedirectRefused) true.
func (e *RedirectError) Is(target error) bool { return target == errRedirectRefused }

// RefuseRedirect is the CheckRedirect of every DSM HTTP client (the
// generated client, its session manager, the fleet client, Login and the
// probe): it refuses every redirect and names only the status.
func RefuseRedirect(req *http.Request, _ []*http.Request) error {
	status := 0
	if req != nil && req.Response != nil {
		status = req.Response.StatusCode
	}
	return &RedirectError{Status: status}
}

// NoRedirect returns a copy of hc whose CheckRedirect is RefuseRedirect, so
// a request sent through it never follows a redirect whatever client the
// caller handed in.
func NoRedirect(hc *http.Client) *http.Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	c := *hc
	c.CheckRedirect = RefuseRedirect
	return &c
}

// RedirectStatus refuses a 3xx answer that reached the caller anyway
// (net/http returns a 3xx without a Location header, and a 300 or 304,
// instead of following it). nil for any other status.
func RedirectStatus(code int) error {
	if code >= 300 && code < 400 {
		return &RedirectError{Status: code}
	}
	return nil
}

// --- transport errors -------------------------------------------------------

// TransportError is a request that never got a usable answer (unreachable
// host, timeout, a certificate refused or not the pinned one, a redirect).
// Error() names the kind only (describeTransport), never Go's text, which
// can quote server-chosen bytes. Unwrap keeps the cause - the error under
// net/url's *url.Error, whose own text carries the request URL and with it
// the session id - so errors.Is / errors.As still see what happened: the
// CLI's offline fallback (--data-source auto) looks for a *net.OpError.
type TransportError struct{ cause error }

func (e *TransportError) Error() string { return describeTransport(e.cause) }
func (e *TransportError) Unwrap() error { return e.cause }

// SanitizeTransport turns a transport error into a TransportError: the
// message names the kind, the type survives for classifiers.
func SanitizeTransport(err error) error {
	if err == nil {
		return nil
	}
	var te *TransportError
	if errors.As(err, &te) {
		return te
	}
	cause := err
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		cause = ue.Err
	}
	return &TransportError{cause: cause}
}

// scrubbedError is an error whose text had every known secret masked. Its
// Unwrap exposes only the transport kind of the original (a *TransportError,
// whose own text is safe), never the unmasked original.
type scrubbedError struct {
	msg  string
	kind error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.kind }

// ScrubError masks every secret value (and credential-bearing parameter) in
// err's text and keeps its transport kind for errors.Is / errors.As.
func ScrubError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	out := &scrubbedError{msg: scrubSecrets(err.Error(), secrets...)}
	var te *TransportError
	if errors.As(err, &te) {
		out.kind = te
	}
	return out
}

// --- secret values in successful answers -----------------------------------

// secretSubstringMin is the shortest secret masked wherever it appears inside
// a response string. A shorter one (a 6-digit one-time code, a short
// password) is masked only when it is an entire string value: masking
// "123456" inside every timestamp and id would destroy the output. Every
// non-empty secret, however short, is masked as an entire string value.
const secretSubstringMin = 8

// ResponseMask replaces a secret value found in a successful answer.
const ResponseMask = "[redacted]"

// MaskResponse masks every secret value the client holds (password, one-time
// code, trusted-device token, sid, SynoToken) in a successful DSM answer
// before it is printed, cached, stored or exported. JSON string values are
// decoded first, so any JSON escaping (\" \/ \uXXXX) is covered; inside a
// value the literal, percent-encoded (once or twice), HTML-escaped and any
// other decodable form is masked. A value whose decoded form still carries a
// secret is replaced whole, and so is a string value that is exactly a
// secret, whatever its length. Masking is type-preserving: only string
// values change; a number, true, false or null is never rewritten (a
// one-time code 482913 that equals a result_id leaves the id a number the
// fleet parser can read), and an object key keeps its name unless a secret
// of 8 or more characters sits inside it. Text that is not JSON is masked as
// plain text. The bytes outside an affected string are left exactly as they
// were.
func MaskResponse(body []byte, secrets ...string) []byte {
	m := newResponseMasker(secrets)
	if m == nil || len(body) == 0 {
		return body
	}
	if !json.Valid(body) {
		return []byte(m.maskString(string(body)))
	}
	var out bytes.Buffer
	changed := false
	for i := 0; i < len(body); {
		if body[i] != '"' {
			out.WriteByte(body[i]) // numbers, true, false, null and structure pass through untouched
			i++
			continue
		}
		end := jsonStringEnd(body, i)
		lit := body[i:end]
		var s string
		if json.Unmarshal(lit, &s) == nil {
			masked := m.maskString(s)
			if isObjectKey(body, end) {
				masked = m.maskInside(s)
			}
			if masked != s {
				out.Write(encodeJSONString(masked))
				changed = true
				i = end
				continue
			}
		}
		out.Write(lit)
		i = end
	}
	if !changed {
		return body
	}
	return out.Bytes()
}

// isObjectKey reports a JSON string literal ending at body[end] that is an
// object key (the next non-blank byte is ':').
func isObjectKey(body []byte, end int) bool {
	for ; end < len(body); end++ {
		switch body[end] {
		case ' ', '\t', '\n', '\r':
			continue
		case ':':
			return true
		}
		return false
	}
	return false
}

// responseMasker holds one call's secrets: sub are masked wherever they
// appear (longest first, with their encoded forms); exact lists every
// non-empty secret, each masked when it is an entire string value.
type responseMasker struct {
	sub   []string
	forms []string
	exact map[string]bool
}

func newResponseMasker(secrets []string) *responseMasker {
	m := &responseMasker{exact: map[string]bool{}}
	for _, raw := range secrets {
		// The secret exactly as held and its trimmed form are both secrets:
		// trimming must never drop a held value below secretSubstringMin.
		for _, v := range []string{raw, strings.TrimSpace(raw)} {
			if v == "" || m.exact[v] {
				continue
			}
			m.exact[v] = true
			// A whitespace-only secret is masked as a whole field only:
			// masking every run of spaces inside text would destroy output.
			if len(v) >= secretSubstringMin && strings.TrimSpace(v) != "" {
				m.sub = append(m.sub, v)
				m.forms = append(m.forms, v, url.QueryEscape(v), url.PathEscape(v), url.QueryEscape(url.QueryEscape(v)),
					url.PathEscape(url.PathEscape(v)), html.EscapeString(v))
			}
		}
	}
	if len(m.exact) == 0 {
		return nil
	}
	sort.SliceStable(m.forms, func(i, j int) bool { return len(m.forms[i]) > len(m.forms[j]) })
	return m
}

// decodeSteps are the single decoding steps an answer's text can carry: HTML
// entities (named or numeric), percent escapes in either hex case, and the
// form encoding's '+' for a space. Each is its own step, never fused with
// another, so every intermediate text is its own state: a secret that
// itself contains an escape (pw%2Fab) echoed once more encoded (pw%252Fab),
// or entity-encoded (pw&#37;2Fab), equals the secret part-way, not at the end.
var decodeSteps = []func(string) string{
	html.UnescapeString,
	func(s string) string { return pctEscape.ReplaceAllStringFunc(s, decodePct) },
	func(s string) string { return strings.ReplaceAll(s, "+", " ") },
}

// maxDecodedStates bounds the search below. Every step shortens the text or
// removes a '+', so the reachable set is finite and normally a handful of
// texts; a value that exceeds the bound is treated as carrying a secret.
const maxDecodedStates = 256

// decodedStates returns s and every text reachable from it by any sequence of
// decodeSteps, and false when the search hit maxDecodedStates (the caller
// then fails closed).
func decodedStates(s string) ([]string, bool) {
	if !strings.ContainsAny(s, "%&+") {
		return []string{s}, true
	}
	seen := map[string]bool{s: true}
	states := []string{s}
	for i := 0; i < len(states); i++ {
		for _, step := range decodeSteps {
			next := step(states[i])
			if next == states[i] || seen[next] {
				continue
			}
			if len(seen) >= maxDecodedStates {
				return states, false
			}
			seen[next] = true
			states = append(states, next)
		}
	}
	return states, true
}

// maskString masks secrets in one decoded string value: replaced whole when
// any decoding of it is exactly a secret (any length), otherwise maskInside.
func (m *responseMasker) maskString(s string) string {
	states, complete := decodedStates(s)
	if !complete {
		return ResponseMask
	}
	for _, state := range states {
		if m.exact[state] || m.exact[strings.TrimSpace(state)] {
			return ResponseMask
		}
	}
	return m.maskInside(s)
}

// maskInside masks the secrets of 8 or more characters inside a string (an
// object key, or a value that is not exactly a secret).
func (m *responseMasker) maskInside(s string) string {
	out := s
	for _, f := range m.forms {
		if strings.Contains(out, f) {
			out = strings.ReplaceAll(out, f, ResponseMask)
		}
	}
	// Any other encoding (lower-case percent escapes, nested entities, a
	// mix): if what a reader would see still carries a secret, the whole
	// value goes.
	states, complete := decodedStates(out)
	if !complete && len(m.sub) > 0 {
		return ResponseMask
	}
	for _, decoded := range states {
		for _, v := range m.sub {
			if strings.Contains(decoded, v) {
				return ResponseMask
			}
		}
	}
	return out
}

// jsonStringEnd returns the index just past the string literal starting at
// body[start] (a '"').
func jsonStringEnd(body []byte, start int) int {
	for i := start + 1; i < len(body); i++ {
		switch body[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return len(body)
}

func encodeJSONString(s string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return bytes.TrimRight(b.Bytes(), "\n")
}

// --- addresses in output ----------------------------------------------------

// DisplayAddress renders a configured NAS address (SYNOLOGY_BASE_URL, a
// fleet entry's URL) for output: scheme, host, port and path only. A user or
// password (https://admin:pw@nas:5001), a query and a fragment are dropped,
// even from an address that does not parse: everything up to the last '@'
// after the scheme is user info, and everything from the first '?' or '#'
// after that is query or fragment. Every place that prints a NAS address
// (doctor, auth login, dry runs, errors, fleet list and add) uses this.
func DisplayAddress(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok {
		scheme, rest = "", s
	}
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		rest = rest[i+1:]
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	if scheme == "" {
		return rest
	}
	return scheme + "://" + rest
}

// --- plain HTTP -------------------------------------------------------------

// PlainHTTPWarnings receives the plain-http:// login warning (the escaping
// stderr writer; tests swap it).
var PlainHTTPWarnings io.Writer = termio.Stderr()

var plainHTTPWarned sync.Map // origin -> struct{}{}: one warning per NAS per process

// IsPlainHTTP reports an http:// NAS address.
func IsPlainHTTP(raw string) bool {
	scheme, _, ok := strings.Cut(strings.TrimSpace(raw), "://")
	return ok && strings.EqualFold(scheme, "http")
}

// PlainHTTPWarning is the text of the warning printed before credentials go
// to an http:// NAS (and doctor's transport row).
func PlainHTTPWarning(origin string) string {
	return fmt.Sprintf("%s is plain http://: the DSM password and session cross the network unencrypted; use the NAS's https:// address (https://<nas>:5001; pin DSM's self-signed certificate with SYNOLOGY_TLS_FINGERPRINT or 'synology-cli fleet add <name> --tls-fingerprint <sha256>')", DisplayAddress(origin))
}

// WarnPlainHTTP prints the plain-http:// warning once per NAS per process:
// every login (auth login, every live command, fleet add --check, fleet
// sync, abb triage, doctor) about to send credentials over http:// comes
// through here, and so does fleet add registering an http:// NAS.
func WarnPlainHTTP(origin string) {
	if !IsPlainHTTP(origin) {
		return
	}
	key := strings.ToLower(strings.TrimRight(DisplayAddress(origin), "/"))
	if _, already := plainHTTPWarned.LoadOrStore(key, struct{}{}); already {
		return
	}
	fmt.Fprintln(PlainHTTPWarnings, "warning: "+PlainHTTPWarning(origin))
}

// ForgetPlainHTTPWarnings lets the next login to each NAS warn again (a
// long-running caller, tests).
func ForgetPlainHTTPWarnings() {
	plainHTTPWarned.Range(func(k, _ any) bool {
		plainHTTPWarned.Delete(k)
		return true
	})
}
