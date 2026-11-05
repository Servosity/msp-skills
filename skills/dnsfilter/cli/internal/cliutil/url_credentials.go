// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a credential inside a URL is never displayed.
// Pinned by this connector's handfixes.json under the
// "base-url-credentials-held-and-redacted" entry (class "secret-echo"). The
// credential kit holds the value (internal/credwire, internal/cliutil/credkit_hold.go);
// this masks it by shape, so an error stays readable rather than withheld.

package cliutil

import (
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// URLCredentialMask is what a redacted URL credential reads as.
const URLCredentialMask = "****"

// URLCredentialName reports a URL query (or fragment) parameter whose value is
// a credential, judged by its name: every header name IsCredentialHeaderName
// names (api_key, access_token, token, secret, password, session, signature,
// auth, credential), anything naming a pass or pwd, and the short names URLs
// carry credentials under (sig, pw, code, jwt, otp, pin, sid, ticket).
func URLCredentialName(name string) bool {
	if IsCredentialHeaderName(name) {
		return true
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	folded := b.String()
	if strings.Contains(folded, "pass") || strings.Contains(folded, "pwd") || strings.Contains(folded, "jwt") {
		return true
	}
	switch folded {
	case "sig", "pw", "code", "otp", "pin", "sid", "ticket":
		return true
	}
	return false
}

// urlParamIsCredential reports a parameter name (as written, possibly
// percent-encoded) that names a credential.
func urlParamIsCredential(name string) bool {
	if dk, err := url.QueryUnescape(name); err == nil {
		name = dk
	}
	return URLCredentialName(name)
}

func urlTokenBreak(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("\"'`<>", r)
}

// urlSchemeSeparators is how a URL's scheme separator is written as text:
// plain, and with its slashes JSON-escaped.
var urlSchemeSeparators = []string{"://", `:\/\/`}

// RedactURLCredentials returns text with every URL credential in it shown as
// ****, by shape, held or not: the whole userinfo of every URL (username and
// password, never user:****), and the value of every credential-named query or
// fragment parameter to the end of that parameter (the next '&' or '#', or the
// end of the URL), not to the first space or quote. It keeps an error readable
// that the credential kit would otherwise withhold whole.
//
// A URL runs from its scheme separator
// to its end: a URL inside quotes (Go's %q, JSON text, escaped JSON) to its
// closing quote, through spaces and escaped quotes; any other URL to the next
// space or quote, except that a credential-named value runs on past them to
// the end of its line (fail closed: the value's end cannot be told). The
// userinfo runs to the last '@' before any '?' or '#', so a raw '/' or '@' in
// a password is covered (fail closed: a URL whose path holds an '@' after a
// port is redacted too). A URL url.Parse rejects is redacted by the same
// shape. A URL without credentials (https://api.dnsfilter.com) is unchanged,
// and a value already shown as **** keeps the text after it.
func RedactURLCredentials(text string) string {
	if text == "" || !strings.Contains(text, "://") && !strings.Contains(text, `:\/\/`) {
		return text
	}
	return maskSpans(text, urlCredentialSpans(text))
}

// redactURLShapes masks the credentials of every URL in text by shape alone.
func redactURLShapes(text string) string {
	return maskSpans(text, urlCredentialSpans(text))
}

// textSpan is a byte range [start, end) of a text to mask; userinfo marks a
// URL's userinfo.
type textSpan struct {
	start, end int
	userinfo   bool
}

// appendOccurrences appends every place text writes needle, left to right
// and not overlapping (as strings.ReplaceAll finds them).
func appendOccurrences(spans []textSpan, text, needle string) []textSpan {
	if needle == "" {
		return spans
	}
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], needle)
		if i < 0 {
			break
		}
		spans = append(spans, textSpan{start: from + i, end: from + i + len(needle)})
		from += i + len(needle)
	}
	return spans
}

// mergeSpans sorts spans and joins those that overlap or touch.
func mergeSpans(spans []textSpan) []textSpan {
	sorted := append([]textSpan(nil), spans...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].start < sorted[j].start })
	var out []textSpan
	for _, s := range sorted {
		if s.end <= s.start {
			continue
		}
		if n := len(out); n > 0 && s.start <= out[n-1].end {
			if s.end > out[n-1].end {
				out[n-1].end = s.end
			}
			continue
		}
		out = append(out, textSpan{start: s.start, end: s.end})
	}
	return out
}

// maskSpans writes text with each (merged) span shown as one ****.
func maskSpans(text string, spans []textSpan) string {
	merged := mergeSpans(spans)
	if len(merged) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, s := range merged {
		b.WriteString(text[last:s.start])
		b.WriteString(URLCredentialMask)
		last = s.end
	}
	b.WriteString(text[last:])
	return b.String()
}

// urlCredentialSpans is every URL credential in text by shape: each URL's
// whole userinfo and each credential-named parameter value.
func urlCredentialSpans(text string) []textSpan {
	var spans []textSpan
	for _, u := range scanURLs(text, urlSchemeSeparators) {
		spans = append(spans, u.spans...)
	}
	return spans
}

// urlScan is one URL of a text: text[start:end] (from its first scheme
// character), and the credential spans in it.
type urlScan struct {
	start, end int
	spans      []textSpan
}

// scanURLs finds every URL in text whose scheme separator is one of seps,
// each separator searched for once per URL at most (linear in text).
func scanURLs(text string, seps []string) []urlScan {
	var out []urlScan
	next := make([]int, len(seps)) // the next index of each separator; -1 when none is left
	for k := range next {
		next[k] = -2 // not searched yet
	}
	for pos := 0; pos < len(text); {
		i, sepLen := -1, 0
		for k, s := range seps {
			if next[k] == -2 || (next[k] >= 0 && next[k] < pos) {
				if j := strings.Index(text[pos:], s); j >= 0 {
					next[k] = pos + j
				} else {
					next[k] = -1
				}
			}
			if next[k] >= 0 && (i < 0 || next[k] < i) {
				i, sepLen = next[k], len(s)
			}
		}
		if i < 0 {
			break
		}
		u := scanURL(text, i, sepLen)
		out = append(out, u)
		pos = i + sepLen
		if u.end > pos {
			pos = u.end
		}
	}
	return out
}

// urlBound is how a URL in a text ends. A line end always ends it. A quoted
// URL (closer, with escapes backslashes written before its opening quote)
// ends at its closing quote; a URL that is the whole text (whole) only at a
// line end; any other URL at the next space or quote.
type urlBound struct {
	closer  byte
	escapes int
	whole   bool
}

func urlBoundBefore(text string, start int) urlBound {
	if start == 0 {
		return urlBound{}
	}
	switch q := text[start-1]; q {
	case '"', '\'':
		n := 0
		for j := start - 2; j >= 0 && text[j] == '\\'; j-- {
			n++
		}
		return urlBound{closer: q, escapes: n}
	case '`':
		return urlBound{closer: '`'}
	case '<':
		return urlBound{closer: '>'}
	}
	return urlBound{}
}

// endsAt reports whether the URL ends at text[j], and where its text ends. A
// quoted URL's closing quote may be escaped at the opening quote's own escape
// level; its backslashes then stay outside, so masking never unescapes a
// quote. An unquoted URL also ends at the backslash of an escaped quote.
func (b urlBound) endsAt(text string, j int) (int, bool) {
	c := text[j]
	if c == '\n' || c == '\r' {
		return j, true
	}
	if b.whole {
		return j, false
	}
	if b.closer == 0 {
		if c == '\\' && j+1 < len(text) && (text[j+1] == '"' || text[j+1] == '\'') {
			return j, true
		}
		if c < utf8.RuneSelf {
			return j, urlTokenBreak(rune(c))
		}
		r, _ := utf8.DecodeRuneInString(text[j:])
		return j, urlTokenBreak(r)
	}
	if c != b.closer {
		return j, false
	}
	if c != '"' && c != '\'' {
		return j, true
	}
	m := 0
	for k := j - 1; k >= 0 && text[k] == '\\'; k-- {
		m++
	}
	if m%2 == 0 {
		return j, true
	}
	if m <= b.escapes {
		return j - m, true
	}
	return j, false
}

// urlParamStart reports a byte after which a parameter name may start. Fail
// closed: ';' and a second '?' start one too, so a credential name behind
// them is found; only a credential-named value runs past them.
func urlParamStart(c byte) bool {
	return c == '&' || c == '#' || c == ';' || c == '?'
}

// alreadyMasked reports a value that an earlier pass already showed as ****,
// followed by nothing a credential could go on with (the end, a space, a
// quote or punctuation): an unquoted URL keeps the text after it.
func alreadyMasked(text string, v int) bool {
	if !strings.HasPrefix(text[v:], URLCredentialMask) {
		return false
	}
	after := text[v+len(URLCredentialMask):]
	if after == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(after)
	return urlTokenBreak(r) || strings.ContainsRune("&#:;,.)]}!?", r)
}

// scanURLParams reads the parameters of a URL from text[k:] (just after its
// '?' or '#') and calls credential for each credential-named value: to the
// next '&' or '#', or the URL's end (a line end, a quoted URL's closing
// quote, the end of the text), never only to a space or quote. Every other
// name and value ends at the next '&', '#', ';' or '?' or where b ends the
// URL. It returns where the URL ends.
func scanURLParams(text string, k int, b urlBound, credential func(start, end int)) int {
	for {
		p, eq := k, false
		for ; p < len(text); p++ {
			if e, ok := b.endsAt(text, p); ok {
				return e
			}
			if c := text[p]; c == '=' {
				eq = true
				break
			} else if urlParamStart(c) {
				break
			}
		}
		if p >= len(text) {
			return len(text)
		}
		if !eq {
			k = p + 1
			continue
		}
		v := p + 1
		if urlParamIsCredential(text[k:p]) && !(b.closer == 0 && !b.whole && alreadyMasked(text, v)) {
			e := v
			for ; e < len(text); e++ {
				if c := text[e]; c == '&' || c == '#' {
					break
				}
				if b.closer != 0 || text[e] == '\n' || text[e] == '\r' {
					if end, ok := b.endsAt(text, e); ok {
						if end > v {
							credential(v, end)
						}
						return end
					}
				}
			}
			if e > v {
				credential(v, e)
			}
			if e >= len(text) {
				return len(text)
			}
			k = e + 1
			continue
		}
		q := v
		for ; q < len(text); q++ {
			if e, ok := b.endsAt(text, q); ok {
				return e
			}
			if c := text[q]; c == '&' || c == '#' || c == ';' || c == '?' {
				break
			}
		}
		if q >= len(text) {
			return len(text)
		}
		k = q + 1
	}
}

// scanURL reads the URL whose scheme separator (sepLen bytes) is at sepAt.
func scanURL(text string, sepAt, sepLen int) urlScan {
	start := 0
	if i := strings.LastIndexFunc(text[:sepAt], urlTokenBreak); i >= 0 {
		_, size := utf8.DecodeRuneInString(text[i:])
		start = i + size
	}
	b := urlBoundBefore(text, start)
	u := urlScan{start: start, end: len(text)}
	from := sepAt + sepLen
	// The head (authority and path) runs to the first '?' or '#'.
	j, ended := from, false
	for ; j < len(text); j++ {
		if e, ok := b.endsAt(text, j); ok {
			u.end, ended = e, true
			break
		}
		if text[j] == '?' || text[j] == '#' {
			break
		}
	}
	headEnd := j
	if ended {
		headEnd = u.end
	}
	if at := userinfoEnd(text[from:headEnd]); at > 0 {
		u.spans = append(u.spans, textSpan{start: from, end: from + at, userinfo: true})
	}
	if ended || j >= len(text) {
		return u
	}
	u.end = scanURLParams(text, j+1, b, func(s, e int) {
		u.spans = append(u.spans, textSpan{start: s, end: e})
	})
	return u
}

// userinfoEnd is where the userinfo of a URL's head (its text after the
// scheme separator, to the first '?' or '#') ends: the last '@' of the
// authority, or, when a raw '/' sits in a password (a ':' before it), the
// last '@' of the head (fail closed); -1 when there is none.
func userinfoEnd(head string) int {
	authority := head
	if i := strings.IndexByte(head, '/'); i >= 0 {
		authority = head[:i]
	}
	if at := strings.LastIndex(authority, "@"); at > 0 {
		return at
	}
	if at := strings.LastIndex(head, "@"); at > 0 && strings.Contains(head[:at], ":") {
		return at
	}
	return -1
}
