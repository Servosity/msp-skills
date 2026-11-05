// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a username or password in a base URL is never
// shown (superops-base-url-never-printed). Pinned by this connector's
// handfixes.json.

package cliutil

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
)

// URLWithheld stands in, in any output, for a URL that does not parse as
// scheme://host. It holds none of the URL's text, so a URL whose userinfo the
// parser could not separate is never shown at all.
const URLWithheld = "[url withheld: it does not parse as scheme://host]"

// URLWithoutUserinfo is the one parse rule for a base URL: it keeps scheme,
// host, port and path (the trailing slash dropped) and reports false for text
// that does not parse, names no scheme and host, or has its userinfo
// displaced (URLUserinfoDisplaced). The userinfo
// (http://user:password@host), the query and the fragment can carry a
// credential and never name the server, so they are dropped. The SuperOps
// tenant identity is this value lowercased (config.SuperOpsTenant).
func URLWithoutUserinfo(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" || URLUserinfoDisplaced(raw) {
		return "", false
	}
	return u.Scheme + "://" + u.Host + strings.TrimRight(u.EscapedPath(), "/"), true
}

// URLUserinfoDisplaced reports whether raw holds an '@', in any percent
// encoding (decoded to a fixed point, so %40 and %2540 count), anywhere but
// the userinfo url.Parse separates: the text up to the last '@' of the
// authority, which ends at the first '/', '?' or '#'. Such an '@' means a
// username or password held an unescaped '/', '?' or '#', or an escaped '@',
// so the parser read part of it as the host, port, path, query or fragment.
// No part of such a URL is shown, recorded or used.
func URLUserinfoDisplaced(raw string) bool {
	s := urlTextWithoutUserinfo(raw)
	for !strings.Contains(s, "@") {
		decoded := percentDecodeOnce(s)
		if decoded == s {
			return false
		}
		s = decoded
	}
	return true
}

// DisplayURL is the URL as any output may show it: URLWithoutUserinfo, so
// never a username, a password, a "user@" or a masked "***", and never the
// query or fragment (nothing shows they are free of a credential). A URL that
// does not parse as scheme://host is URLWithheld, never its raw text, and so
// is one that parses with an '@' left in its path, query or fragment (in any
// percent encoding, URLUserinfoDisplaced): the
// parser took no userinfo from it (an unescaped '/', '?' or '#' in the
// userinfo), so what it calls the host may be the username. A path reference
// ("/msp", "/graphql?first=50") has no authority and so no userinfo; it is
// shown as written, unless it holds an '@' ("user@host" written without a
// scheme). Empty stays empty.
func DisplayURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return URLWithheld
	}
	if u.Scheme == "" && u.Host == "" && u.User == nil && u.Opaque == "" {
		if strings.Contains(trimmed, "@") {
			return URLWithheld
		}
		return trimmed
	}
	if strings.Contains(u.EscapedPath()+"?"+u.RawQuery+"#"+u.EscapedFragment(), "@") {
		return URLWithheld
	}
	shown, ok := URLWithoutUserinfo(trimmed)
	if !ok {
		return URLWithheld
	}
	return shown
}

// MinDetectableSecretLen is the shortest API token, auth header token or base
// URL secret config.Load accepts (superops-credential-floor). A shorter one is
// still held and withheld in every encoding by the kit, at the cost of
// withholding much more output, so it is refused before anything is sent.
const MinDetectableSecretLen = 8

// URLUserinfoTooShort reports whether raw's userinfo carries a secret shorter
// than MinDetectableSecretLen once decoded: the password, or the username
// when there is no password (then the username is the secret). A short
// username beside a password still loads. The userinfo is read with the authority rule
// URLUserinfoSecrets uses.
func URLUserinfoTooShort(raw string) bool {
	authority := strings.TrimSpace(raw)
	if _, rest, ok := strings.Cut(authority, "://"); ok {
		authority = rest
	} else {
		authority = strings.TrimPrefix(authority, "//")
	}
	if k := strings.IndexAny(authority, "/?#"); k >= 0 {
		authority = authority[:k]
	}
	at := strings.LastIndex(authority, "@")
	if at < 0 {
		return false
	}
	user, pass, _ := strings.Cut(authority[:at], ":")
	secret := pass
	if secret == "" {
		secret = user
	}
	if v, err := url.PathUnescape(secret); err == nil {
		secret = v
	}
	return secret != "" && len(secret) < MinDetectableSecretLen
}

// URLHasQueryOrFragment reports whether raw holds a '?' or '#' anywhere but
// its userinfo. A base URL never needs one (request paths are appended to it,
// so a query or fragment there breaks every request), and request errors
// quote the whole URL, so a base URL with one is refused, never shown.
func URLHasQueryOrFragment(raw string) bool {
	return strings.ContainsAny(urlTextWithoutUserinfo(raw), "?#")
}

// urlTextWithoutUserinfo is raw (trimmed) with the userinfo url.Parse
// separates removed: the text up to the last '@' of the authority, which
// starts after "://" (or a leading "//") and ends at the first '/', '?' or '#'.
func urlTextWithoutUserinfo(raw string) string {
	s := strings.TrimSpace(raw)
	start := 0
	if k := strings.Index(s, "://"); k >= 0 {
		start = k + 3
	} else if strings.HasPrefix(s, "//") {
		start = 2
	}
	end := len(s)
	if k := strings.IndexAny(s[start:], "/?#"); k >= 0 {
		end = start + k
	}
	if at := strings.LastIndex(s[start:end], "@"); at >= 0 {
		s = s[:start] + s[start+at+1:]
	}
	return s
}

// RedactURLUserinfo removes the userinfo of every URL in text: "scheme://"
// followed by an authority that holds an '@' loses everything up to and
// including its last '@'. It is the backstop for text this program did not
// build (net/http's url.Error keeps the username and masks only the password;
// url.Parse errors quote the whole URL). It also reads the JSON form with
// escaped slashes (http:\/\/) and the percent-encoded form (http%3A%2F%2F,
// with %40 for '@'). Text without a URL authority holding an '@' is returned
// unchanged, so an e-mail address or a path with an '@' stays as written.
func RedactURLUserinfo(text string) string {
	low := asciiLower(text)
	if !strings.Contains(text, "@") && !strings.Contains(low, "%40") {
		return text
	}
	markers := []urlSchemeMarker{{"://", true, urlMarkerUnknown}, {`:\/\/`, true, urlMarkerUnknown}, {"%3a%2f%2f", false, urlMarkerUnknown}}
	var b strings.Builder
	i := 0
	for i < len(text) {
		m := nextURLSchemeMarker(low, i, markers)
		if m == nil {
			break
		}
		start := m.at + len(m.marker)
		end, userinfoEnd := urlAuthorityEnd(text, start, m.plain)
		b.WriteString(text[i:start])
		if userinfoEnd > start {
			b.WriteString(text[userinfoEnd:end])
		} else {
			b.WriteString(text[start:end])
		}
		i = end
	}
	b.WriteString(text[i:])
	return b.String()
}

// urlSchemeMarker is one scheme separator form RedactURLUserinfo reads: "://",
// ":\/\/" (JSON) or "%3a%2f%2f" (percent-encoded, matched in either case);
// plain is false for the percent-encoded form. at caches the marker's next
// position (urlMarkerUnknown before the first search, -1 once none is left),
// so the whole scan stays linear.
type urlSchemeMarker struct {
	marker string
	plain  bool
	at     int
}

const urlMarkerUnknown = -2

// nextURLSchemeMarker returns the earliest marker at or after from in low (the
// ASCII-lowercased text), or nil when none is left.
func nextURLSchemeMarker(low string, from int, markers []urlSchemeMarker) *urlSchemeMarker {
	var best *urlSchemeMarker
	for k := range markers {
		m := &markers[k]
		if m.at == urlMarkerUnknown || (m.at >= 0 && m.at < from) {
			if j := strings.Index(low[from:], m.marker); j >= 0 {
				m.at = from + j
			} else {
				m.at = -1
			}
		}
		if m.at >= 0 && (best == nil || m.at < best.at) {
			best = m
		}
	}
	return best
}

// urlAuthorityEnd returns where the authority starting at start ends and,
// when it holds an '@' (or %40 in the percent-encoded form), the index just
// past the last one; otherwise userinfoEnd is -1. The authority ends at a
// '/', '?', '#', a backslash, a quote, '<', '>', '`', whitespace or a control
// character, and in the percent-encoded form also at %2F, %3F or %23.
func urlAuthorityEnd(text string, start int, plain bool) (end, userinfoEnd int) {
	userinfoEnd = -1
	end = len(text)
	for k := start; k < len(text); k++ {
		c := text[k]
		if c <= ' ' || c == 0x7f || strings.IndexByte(`/?#\"'<>`+"`", c) >= 0 {
			end = k
			break
		}
		if !plain && c == '%' && k+3 <= len(text) {
			switch asciiLower(text[k : k+3]) {
			case "%2f", "%3f", "%23":
				end = k
			case "%40":
				userinfoEnd = k + 3
			}
			if end == k {
				break
			}
		}
		if c == '@' {
			userinfoEnd = k + 1
		}
	}
	return end, userinfoEnd
}

// asciiLower lowercases ASCII letters only, so byte offsets are kept.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// minHeldUsername is the shortest username held beside a password. A shorter
// one matches nearly every output in some decoded view, and the kit then drops
// even its own notice, so the run goes silent. The password beside it is held
// at any length, and it is the secret.
const minHeldUsername = 3

// URLUserinfoSecrets is the userinfo of raw as credential values: the password
// at any length, the username (minHeldUsername beside a password; alone, as in
// "token@host", it is the secret and is held at any length), the
// "user:password" pair, each decoded and as written in the URL, and the HTTP
// Basic header net/http builds from it ("Basic " + base64 of the decoded
// pair), which a server can echo back. Userinfo is read with the authority
// rule url.Parse uses (up to the last '@' before the first '/', '?' or '#'),
// so a URL that does not otherwise parse still gives its userinfo, and so
// does "user:password@host" written without a scheme. Nil when raw has none.
func URLUserinfoSecrets(raw string) []string {
	authority := strings.TrimSpace(raw)
	if _, rest, ok := strings.Cut(authority, "://"); ok {
		authority = rest
	} else {
		authority = strings.TrimPrefix(authority, "//")
	}
	if k := strings.IndexAny(authority, "/?#"); k >= 0 {
		authority = authority[:k]
	}
	at := strings.LastIndex(authority, "@")
	if at < 0 {
		return nil
	}
	var out []string
	add := func(user, pass string, hasPass bool) {
		secretPass := hasPass && pass != ""
		if user != "" && (!secretPass || len(user) >= minHeldUsername) {
			out = append(out, user)
		}
		if secretPass {
			out = append(out, pass, user+":"+pass)
		}
	}
	user, pass, hasPass := strings.Cut(authority[:at], ":")
	add(user, pass, hasPass)
	decodedUser, decodedPass := user, pass
	if v, err := url.PathUnescape(user); err == nil {
		decodedUser = v
	}
	if v, err := url.PathUnescape(pass); err == nil {
		decodedPass = v
	}
	if decodedUser != user || decodedPass != pass {
		add(decodedUser, decodedPass, hasPass)
	}
	// The kit holds both parts of a Basic value, so one naming a short username
	// is left to the password held above, which it finds in the decoded pair.
	if decodedPass == "" || len(decodedUser) >= minHeldUsername {
		out = append(out, "Basic "+base64.StdEncoding.EncodeToString([]byte(decodedUser+":"+decodedPass)))
	}
	return out
}

// HoldURLUserinfo records the userinfo of every URL in each text
// (URLUserinfoSecrets at the text and at each "://" in it, so a URL after the
// first is held too) with the shared credential kit.
func HoldURLUserinfo(raws ...string) {
	for _, raw := range raws {
		HoldValues(urlUserinfoIn(raw)...)
	}
}

// urlUserinfoIn is URLUserinfoSecrets at the text and at each "://" in it.
func urlUserinfoIn(raw string) []string {
	out := URLUserinfoSecrets(raw)
	for i := strings.Index(raw, "://"); i >= 0; {
		out = append(out, URLUserinfoSecrets(raw[i:])...)
		next := strings.Index(raw[i+3:], "://")
		if next < 0 {
			break
		}
		i += 3 + next
	}
	return out
}

// percentDecodeOnce decodes every %XX in s once, leaving any other byte.
func percentDecodeOnce(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+3 <= len(s) {
			if v, err := hex.DecodeString(s[i+1 : i+3]); err == nil {
				b.WriteByte(v[0])
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
