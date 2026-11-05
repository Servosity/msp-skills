// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"synology-pp-cli/internal/cliutil"
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
		hc = &http.Client{Transport: cliutil.OutboundGuard(nil)} // Hand-fix (handfixes.json credkit-wiring)
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
var PlainHTTPWarnings io.Writer

var plainHTTPWarned sync.Map // origin -> struct{}{}: one warning per NAS per process

// IsPlainHTTP reports an http:// NAS address.
func IsPlainHTTP(raw string) bool {
	scheme, _, ok := strings.Cut(strings.TrimSpace(raw), "://")
	return ok && strings.EqualFold(scheme, "http")
}

// ErrPlainHTTPCredentials refuses a DSM login or session over plain http://
// to a host that is not this computer. It is the same rule the generated
// client applies to every request since press 4.34.2
// (client.ErrPlaintextCredential), so the password is never sent where the
// session that follows it would be refused. HAND-FIX(dsm-plain-http-refused).
var ErrPlainHTTPCredentials = errors.New("refusing to send DSM credentials over plain http://")

// PlainHTTPRefused reports an http:// NAS address whose host is not
// localhost or a loopback address: no credential or session goes there.
func PlainHTTPRefused(raw string) bool {
	if !IsPlainHTTP(raw) {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return true
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	if strings.EqualFold(host, "localhost") {
		return false
	}
	addr, err := netip.ParseAddr(host)
	return err != nil || !addr.Unmap().IsLoopback()
}

// RefusePlainHTTP returns ErrPlainHTTPCredentials (naming the display
// address, never the raw one) for a refused plain-http:// NAS.
func RefusePlainHTTP(origin string) error {
	if !PlainHTTPRefused(origin) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrPlainHTTPCredentials, PlainHTTPWarning(origin))
}

// PlainHTTPWarning is the text of the warning printed before credentials go
// to an http:// NAS on this computer, and of the refusal for any other
// http:// NAS (and doctor's transport row).
func PlainHTTPWarning(origin string) string {
	if PlainHTTPRefused(origin) {
		return fmt.Sprintf("%s is plain http://: the DSM password and session would cross the network unencrypted, so neither is sent; use the NAS's https:// address (https://<nas>:5001; pin DSM's self-signed certificate with SYNOLOGY_TLS_FINGERPRINT or 'synology-cli fleet add <name> --tls-fingerprint <sha256>')", DisplayAddress(origin))
	}
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
	w := PlainHTTPWarnings
	if w == nil {
		w = termio.Stderr()
	}
	fmt.Fprintln(w, "warning: "+PlainHTTPWarning(origin))
}

// ForgetPlainHTTPWarnings lets the next login to each NAS warn again (a
// long-running caller, tests).
func ForgetPlainHTTPWarnings() {
	plainHTTPWarned.Range(func(k, _ any) bool {
		plainHTTPWarned.Delete(k)
		return true
	})
}
