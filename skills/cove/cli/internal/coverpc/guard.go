// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written guards for the JSON-RPC client (Codex review rounds 1 and 2,
// press 4.34.0 reprint). Hand-fix codes: cove-rpc-plaintext-and-tenant-guard
// (plaintext refusal, endpoint binding) and cove-short-credential-funnel
// (response visa discovery).

package coverpc

import (
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"unicode/utf8"

	"cove-pp-cli/internal/cliutil"
)

// ErrPlaintextCredentials refuses a request that would carry the API token or
// the session visa over plaintext HTTP. Every coverpc request carries one of
// them (Login sends the password, every other method the visa), so the guard
// applies to every request, before anything is dialled.
var ErrPlaintextCredentials = errors.New("refusing to send Cove credentials over plaintext HTTP: set COVE_BASE_URL to an https:// URL (plain http is accepted only for a loopback test server)")

// errBadBaseURL deliberately does not echo the configured URL: a URL can carry
// userinfo.
var errBadBaseURL = errors.New("COVE_BASE_URL is not a valid http(s) URL with a host")

// errBaseURLTarget refuses a base URL whose query or fragment would swallow
// the JSON-RPC path appended to it (and so has no endpoint binding).
var errBaseURLTarget = errors.New("COVE_BASE_URL must not carry a query or a fragment: the JSON-RPC path is appended to it")

// refusePlaintextOrigin allows https to any host and plain http only to a
// loopback host (localhost or a loopback IP literal).
func refusePlaintextOrigin(base string) error {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return errBadBaseURL
	}
	if strings.ContainsAny(base, "?#") {
		return errBaseURLTarget
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return ErrPlaintextCredentials
	}
	return errBadBaseURL
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}

// endpointBindingPrefix marks the binding form Endpoint writes. Bindings
// written before it (scheme://host/path, with the host lower-cased whole and
// the query dropped) never start with it, so an old cached session or stored
// snapshot never matches a new binding.
const endpointBindingPrefix = "POST "

// Endpoint is the destination this client's requests go to, the binding a
// cached session and a stored snapshot carry: "POST " and the request URL as
// sent. Every routing-relevant part is kept: the scheme; an IP literal in its
// canonical form with any IPv6 zone kept exactly (zone names are
// case-sensitive); a DNS name lower-cased in ASCII only (sameHost's rule); the
// port (a scheme's default port is the same destination as none); and the
// request path exactly as sent. Userinfo is left out (it is not a destination
// and can hold a password). A base URL that is never dialled (no http(s)
// scheme or host, or a query or fragment, which would swallow the JSON-RPC
// path; refusePlaintextOrigin refuses it) has no binding: "" never matches.
func (c *Client) Endpoint() string {
	return normalizeEndpoint(c.BaseURL)
}

// EndpointBindingIsCurrent reports whether a stored endpoint binding was
// written in the current form. A binding in an older form (or none) may alias
// distinct destinations and must be refused, never matched.
func EndpointBindingIsCurrent(endpoint string) bool {
	return strings.HasPrefix(endpoint, endpointBindingPrefix)
}

func normalizeEndpoint(base string) string {
	if strings.ContainsAny(base, "?#") {
		return ""
	}
	u, err := url.Parse(base + rpcPath)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return ""
	}
	host := bindingHost(u.Hostname())
	if port := explicitPort(u); port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return endpointBindingPrefix + scheme + "://" + host + u.EscapedPath()
}

// bindingHost is a URL hostname as sameHost compares it: an IP literal in
// netip's canonical form (zone kept exactly), an ASCII name lower-cased, any
// other name exactly as written.
func bindingHost(host string) string {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.String()
	}
	if isASCII(host) {
		return strings.ToLower(host)
	}
	return host
}

// Visa shape. Cove publishes no visa format, but its REST endpoints take the
// visa as an "Authorization: Bearer" value, so a real visa is an RFC 6750
// b64token. A value of that shape and at least visaMinLen characters is held;
// anything else found next to a "visa" name (a word such as "required", a
// number, null) is not a visa and is never held, so it can never mask or
// withhold later output.
// ponytail: a visa outside the bearer grammar would be missed here; the
// session visa the client itself receives and sends is held directly, not by
// this discovery.
const visaMinLen = 16

// IsVisaShape reports whether v has the shape of a Cove visa.
func IsVisaShape(v string) bool {
	if len(v) < visaMinLen {
		return false
	}
	i := 0
	for i < len(v) && isVisaChar(v[i]) {
		i++
	}
	j := i
	for j < len(v) && v[j] == '=' {
		j++
	}
	return i > 0 && j == len(v)
}

func isVisaChar(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' ||
		b == '-' || b == '.' || b == '_' || b == '~' || b == '+' || b == '/'
}

// maxVisaDepth bounds the JSON documents carried inside string values that
// visa discovery opens.
const maxVisaDepth = 4

// responseVisas is the one discovery of every visa a body (or a header line)
// carries. A visa the server just returned is a credential before any text of
// that answer is shown. Two signals, both filtered by IsVisaShape:
//   - every JSON member whose DECODED name is "visa" (any case, any depth,
//     arrays and duplicate names, and inside a JSON document carried in a
//     string value), via the shared parser (cliutil.FunnelJSONScan);
//   - the name scanner (scanVisaNames), which starts at EVERY occurrence of the
//     name "visa" and reads the value after it, so an earlier name/value pair
//     can never swallow it. It runs over the text as written, over every
//     decoded string of a JSON text, and, for text that is not a complete
//     JSON stream, over every decoded view of it (cliutil.FunnelTextViews).
//
// Every value is held as written and decoded.
func responseVisas(body []byte) []string {
	var out []string
	seen := map[string]bool{}
	add := func(v string) {
		for _, form := range []string{v, cliutil.FunnelSettle(v)} {
			if form = strings.TrimSpace(form); IsVisaShape(form) && !seen[form] {
				seen[form] = true
				out = append(out, form)
			}
		}
	}
	text := string(body)
	scanVisaNames(text, add)
	jsonOrFallbackVisas(text, 0, add)
	return out
}

// Visas is responseVisas for the other packages that read a Cove request or
// answer (the generated endpoint commands): the one visa discovery.
func Visas(body []byte) []string {
	return responseVisas(body)
}

// jsonOrFallbackVisas discovers the visas in text: by decoded member name and
// the name scanner over every decoded string when text is a complete JSON
// stream in valid UTF-8, and by the name scanner over every decoded view of it
// otherwise (the members visited before a fault are kept too).
func jsonOrFallbackVisas(text string, depth int, add func(string)) {
	complete := cliutil.FunnelJSONScan([]byte(text), func(s cliutil.FunnelJSONScalar) bool {
		if v, ok := s.Value.(string); ok && containsFoldASCII(v, "visa") {
			for _, view := range cliutil.FunnelTextViews(v) {
				scanVisaNames(view, add)
			}
		}
		if s.Key {
			return false
		}
		if underVisaMember(s.Members) {
			switch v := s.Value.(type) {
			case string:
				add(v)
			case json.Number:
				add(string(v))
			}
		}
		if v, ok := s.Value.(string); ok && depth < maxVisaDepth && looksLikeJSON(v) {
			jsonOrFallbackVisas(v, depth+1, add)
		}
		return false
	})
	if !complete || !utf8.ValidString(text) {
		for _, view := range cliutil.FunnelTextViews(text) {
			scanVisaNames(view, add)
		}
	}
}

func underVisaMember(members []string) bool {
	for _, m := range members {
		if strings.EqualFold(strings.TrimSpace(m), "visa") {
			return true
		}
	}
	return false
}

func looksLikeJSON(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}

// scanVisaNames starts at every occurrence of the name "visa" (any case, not
// part of a longer name) and reads the value after it: an optional closing
// quote, then ':' or '=' or '>' (a JSON member, a header, a query or form
// parameter, an XML tag) or just whitespace ("renew with visa X"), then an
// optional opening quote, then the longest run of visa characters. That run is
// offered to add, which keeps it only when it has the visa shape.
func scanVisaNames(text string, add func(string)) {
	for p := 0; p+4 <= len(text); p++ {
		if !strings.EqualFold(text[p:p+4], "visa") {
			continue
		}
		if p > 0 && isNameChar(text[p-1]) || p+4 < len(text) && isNameChar(text[p+4]) {
			continue
		}
		q := p + 4
		if q < len(text) && (text[q] == '"' || text[q] == '\'') {
			q++
		}
		q = skipBlank(text, q)
		if q < len(text) && (text[q] == ':' || text[q] == '=' || text[q] == '>') {
			q = skipBlank(text, q+1)
		}
		if q < len(text) && (text[q] == '"' || text[q] == '\'') {
			q++
		}
		r := q
		for r < len(text) && (isVisaChar(text[r]) || text[r] == '=') {
			r++
		}
		if r > q {
			add(text[q:r])
		}
	}
}

func isNameChar(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '$'
}

func skipBlank(text string, i int) int {
	for i < len(text) && (text[i] == ' ' || text[i] == '\t' || text[i] == '\r' || text[i] == '\n') {
		i++
	}
	return i
}

func containsFoldASCII(s, sub string) bool {
	for p := 0; p+len(sub) <= len(s); p++ {
		if strings.EqualFold(s[p:p+len(sub)], sub) {
			return true
		}
	}
	return false
}
