// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
)

// --- the probe gate ---------------------------------------------------------

// verifiedAddresses holds every NAS address (CanonicalAddress) that answered
// the SYNO.API.Info probe as a DSM WebAPI in this process. Only a success is
// remembered: a host that is not DSM, or that did not answer, is probed again
// next time.
var verifiedAddresses sync.Map // address -> *verifyEntry

type verifyEntry struct {
	mu sync.Mutex // one probe at a time per address; concurrent callers wait for it
	ok bool
}

// VerifyDSM is the rule that decides whether anything secret may go to a NAS
// address: nil only when baseURL has answered ProbeDSM as a DSM WebAPI in
// this process. The first call per address probes it (without credentials,
// without cookies, never following a redirect); later calls reuse the
// answer, so each address is probed once per process. Every request that
// carries a credential or a session comes through here first: the login
// (password, one-time code, trusted-device token), every request with a sid
// (from a login, the configured auth_header or the session cache), the
// SynoToken and session cookies, in the generated client and the fleet
// client alike. A host that is not DSM returns *NotDSMError; one that did not
// answer returns the probe's *TransportError (its kind kept for errors.Is /
// errors.As, so an unreachable NAS still reads as unreachable).
func VerifyDSM(ctx context.Context, hc *http.Client, baseURL string) error {
	key := CanonicalAddress(baseURL)
	if key == "" {
		return &NotDSMError{Reason: "the address is not a usable URL"}
	}
	v, _ := verifiedAddresses.LoadOrStore(key, &verifyEntry{})
	e := v.(*verifyEntry)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ok {
		return nil
	}
	if err := ProbeDSM(ctx, hc, baseURL); err != nil {
		return err
	}
	e.ok = true
	return nil
}

// ForgetVerifiedAddresses makes the next VerifyDSM of every address probe
// again (tests; a caller that wants a fresh answer).
func ForgetVerifiedAddresses() {
	verifiedAddresses.Range(func(k, _ any) bool {
		verifiedAddresses.Delete(k)
		return true
	})
}

// SessionRefusedError is a request that carried a session (a sid, the
// SynoToken, session cookies) and was not sent, because the NAS address does
// not answer as a DSM WebAPI. errors.As still finds the *NotDSMError.
type SessionRefusedError struct{ cause *NotDSMError }

func (e *SessionRefusedError) Error() string {
	return "DSM session refused: " + e.cause.Error() + "; the session was not sent - check SYNOLOGY_BASE_URL (or the fleet entry's --url)"
}

func (e *SessionRefusedError) Unwrap() error { return e.cause }

// SessionNotSent turns VerifyDSM's refusal of a host that is not DSM into a
// SessionRefusedError; any other error (a transport kind) comes back as it
// was, so the caller handles it like the request's own transport failure.
func SessionNotSent(err error) error {
	var notDSM *NotDSMError
	if errors.As(err, &notDSM) {
		return &SessionRefusedError{cause: notDSM}
	}
	return err
}

// CarriesSession reports a request that would carry a session or a
// credential: a _sid parameter, the X-SYNO-TOKEN header, a Cookie or
// Authorization header, or a cookie jar entry for its URL.
func CarriesSession(req *http.Request, jar http.CookieJar) bool {
	if req == nil || req.URL == nil {
		return false
	}
	if req.URL.Query().Get("_sid") != "" || req.Header.Get("X-SYNO-TOKEN") != "" ||
		req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
		return true
	}
	return jar != nil && len(jar.Cookies(req.URL)) > 0
}

// CanonicalOrigin is raw's scheme://host:port in lower case with the
// default port written out (https -> 443, http -> 80), or "" for an address
// that is not an http(s) URL with a host and no user info. Two spellings of
// one origin compare equal; http://nas and https://nas never do, nor do two
// ports of one host.
func CanonicalOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	switch {
	case scheme != "http" && scheme != "https":
		return ""
	case port == "" && scheme == "https":
		port = "443"
	case port == "":
		port = "80"
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

// CanonicalAddress is CanonicalOrigin plus raw's path prefix (a NAS behind a
// reverse proxy at https://proxy/nas1 is a different DSM from
// https://proxy/nas2): the key VerifyDSM remembers.
func CanonicalAddress(raw string) string {
	origin := CanonicalOrigin(raw)
	if origin == "" {
		return ""
	}
	u, _ := url.Parse(strings.TrimSpace(raw))
	return origin + strings.TrimRight(u.EscapedPath(), "/")
}

// --- the session cookie jar -------------------------------------------------

// OriginJar is the session manager's cookie jar, bound to exactly one NAS
// origin (scheme://host:port): it stores cookies only from that origin and
// hands them only to requests for that origin, so a cookie DSM set over
// https://nas:5001 is never sent to http://nas:5000 or to another port, and
// for an https:// origin every cookie is stored Secure. Bind resets it:
// pointing the session manager at another NAS (or the same one again)
// starts from an empty jar. Unbound, it holds and returns nothing.
type OriginJar struct {
	mu     sync.Mutex
	origin string
	jar    *cookiejar.Jar
}

// NewOriginJar returns an unbound, empty jar.
func NewOriginJar() *OriginJar {
	return &OriginJar{}
}

// Bind drops every cookie and binds the jar to baseURL's origin ("" or an
// unusable address leaves it bound to nothing).
func (j *OriginJar) Bind(baseURL string) {
	jar, _ := cookiejar.New(nil)
	j.mu.Lock()
	defer j.mu.Unlock()
	j.origin, j.jar = CanonicalOrigin(baseURL), jar
}

// Origin is the origin the jar is bound to ("" = none).
func (j *OriginJar) Origin() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.origin
}

func (j *OriginJar) bound(u *url.URL) *cookiejar.Jar {
	if j == nil || u == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.origin == "" || j.jar == nil || CanonicalOrigin(u.String()) != j.origin {
		return nil
	}
	return j.jar
}

// SetCookies implements http.CookieJar: cookies from any other origin are
// dropped; for an https:// origin each stored cookie is marked Secure.
func (j *OriginJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	jar := j.bound(u)
	if jar == nil {
		return
	}
	secure := strings.EqualFold(u.Scheme, "https")
	kept := make([]*http.Cookie, 0, len(cookies))
	for _, c := range cookies {
		if c == nil {
			continue
		}
		if secure {
			// The cookie was just parsed from the NAS's answer or the session
			// cache for this jar alone; storing it Secure means even the
			// host-scoped jar underneath never hands it to http://.
			c.Secure = true
		}
		kept = append(kept, c)
	}
	jar.SetCookies(u, kept)
}

// Cookies implements http.CookieJar: nothing for any other origin.
func (j *OriginJar) Cookies(u *url.URL) []*http.Cookie {
	jar := j.bound(u)
	if jar == nil {
		return nil
	}
	return jar.Cookies(u)
}
