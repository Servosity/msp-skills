// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the shared secret-echo output funnel. Pinned by
// this connector's handfixes.json under the "secret-echo-decoded-funnel" entry
// (class "secret-echo"). The same file ships byte-identical in every connector
// that carries the fix: change it everywhere or nowhere.

package cliutil

import (
	"encoding/base64"
	"encoding/hex"
	"html"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// FunnelWithheldText replaces, whole, any text that repeats a secret this
// program holds. Its wording avoids every word LooksLikeAuthError matches, so a
// withheld HTTP 400 still classifies by its status line alone.
const FunnelWithheldText = "[text withheld: it repeated a secret value this program holds]"

// FunnelWithheldAuthText replaces a withheld text that read as an auth failure
// (funnelReadsAsAuth), so the error classifiers still read it as one: a 400
// "invalid token" answer that echoed the token keeps its auth exit code.
const FunnelWithheldAuthText = "[text withheld: it repeated a secret value this program holds; the answer read as an authentication error]"

// FunnelWithheld reports whether text is one of the funnel's withheld notices.
func FunnelWithheld(text string) bool {
	return text == FunnelWithheldText || text == FunnelWithheldAuthText
}

const (
	// funnelMinSecretLen: shorter values are still masked as written by the
	// caller's own needles, but are not looked for in decoded views (a short
	// value would match unrelated decoded text).
	funnelMinSecretLen = 8
	// funnelMaxInput: text longer than this is withheld without decoding.
	funnelMaxInput = 1 << 20
)

// The work cap: funnelWorkPerByte times the input length plus funnelWorkBase
// bytes processed. Measured: a 50 KB HTML error page needs 345 times its
// length (89 views), a 1 MB JSON error body 36 times. Variables, not
// constants, so a test can lower them and prove the cap fails closed.
var (
	funnelWorkPerByte = 512
	funnelWorkBase    = 4 << 20
)

// FunnelWithhold returns a withheld notice when text repeats one of secrets,
// as written or in any view FunnelEchoes reaches; otherwise text unchanged.
// The secrets are also held for the rest of the process (FunnelHold).
func FunnelWithhold(text string, secrets []string) string {
	if !FunnelEchoes(text, secrets) {
		return text
	}
	if funnelReadsAsAuth(text) {
		return FunnelWithheldAuthText
	}
	return FunnelWithheldText
}

// FunnelEchoes reports whether text carries one of secrets (each of at least
// funnelMinSecretLen characters) in any view reachable from it.
//
// The views are searched breadth-first: every view, the original included, is
// matched against the needles first, then every step below is applied to it
// and each new result is a further view (a step never replaces a view, so a
// literal backslash survives next to its unescaped form). Views are
// deduplicated, and the search is capped by total work, not by depth: past the
// cap, or for text over funnelMaxInput, it reports true (withhold).
//
// Steps: drop control and format characters and invalid UTF-8; decode JSON and
// JavaScript string escapes; drop the backslash of any other escape (the
// identity escape); decode percent escapes; decode HTML entities; strip markup
// tags; decode every base64/base64url run at each of its four alignments; and
// every hex run at both.
func FunnelEchoes(text string, secrets []string) bool {
	FunnelHold(secrets...)
	needles := funnelNeedles(secrets)
	if len(needles) == 0 || text == "" {
		return false
	}
	if len(text) > funnelMaxInput {
		return true
	}
	budget := funnelWorkPerByte*len(text) + funnelWorkBase
	seen := map[string]bool{text: true}
	queue := []string{text}
	for len(queue) > 0 {
		view := queue[0]
		queue = queue[1:]
		for _, n := range needles {
			if strings.Contains(view, n) {
				return true
			}
		}
		for _, step := range funnelSteps {
			budget -= len(view) + 1
			if budget < 0 {
				return true
			}
			for _, next := range step(view) {
				if len(next) < funnelMinSecretLen || seen[next] {
					continue
				}
				budget -= len(next) + 1
				if budget < 0 {
					return true
				}
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

// funnelSteps is the fixed set of decodings. Each returns the views it makes
// from one view (nothing when it changes nothing).
var funnelSteps = []func(string) []string{
	funnelTextStep(funnelDropUnprintable),
	funnelTextStep(funnelJSONUnescape),
	funnelTextStep(funnelIdentityUnescape),
	funnelTextStep(funnelPercentDecode),
	funnelTextStep(html.UnescapeString),
	funnelTextStep(funnelStripTagsText),
	funnelBase64Views,
	funnelHexViews,
}

func funnelTextStep(decode func(string) string) func(string) []string {
	return func(v string) []string {
		if d := decode(v); d != v {
			return []string{d}
		}
		return nil
	}
}

// heldSecrets is every value any funnel call was given, for the backstops
// (FunnelHeld) that have no credential of their own at hand.
var heldSecrets = struct {
	sync.Mutex
	values []string
	seen   map[string]bool
}{seen: map[string]bool{}}

// FunnelHold records credential values this process holds, so FunnelHeld
// withholds them too. Every FunnelEchoes and FunnelWithhold call holds its
// secrets; clients also hold theirs when they are built and when they resolve
// an auth header.
func FunnelHold(values ...string) {
	heldSecrets.Lock()
	defer heldSecrets.Unlock()
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" && !heldSecrets.seen[v] {
			heldSecrets.seen[v] = true
			heldSecrets.values = append(heldSecrets.values, v)
		}
	}
}

// FunnelHeld is FunnelWithhold over every value FunnelHold has recorded. It is
// the backstop for error text that is printed far from the code that knew the
// credential: the CLI's error writers and the MCP server's error results.
func FunnelHeld(text string) string {
	heldSecrets.Lock()
	values := append([]string(nil), heldSecrets.values...)
	heldSecrets.Unlock()
	return FunnelWithhold(text, values)
}

// funnelAuthPatterns mirror the generated LooksLikeAuthError, which classifies
// an HTTP 400 as an auth failure from its text.
var funnelAuthPatterns = regexp.MustCompile(`\bkey\b|\btoken\b|\bunauthorized\b|\bapi_key\b|missing.{0,20}key|required.{0,20}key|\bforbidden\b|\bauthenticat|\bcredential`)

// funnelReadsAsAuth reports whether text reads as an auth failure to the
// generated error classifiers.
func funnelReadsAsAuth(text string) bool {
	return funnelAuthPatterns.MatchString(strings.ToLower(text))
}

// funnelNeedles is every form of secrets a view is searched for: each value as
// written and as the text steps would show it, the part after a scheme word
// ("Bearer x", "Basic x"), a space form-encoded as '+', and both halves of a
// base64 "id:secret" pair (Basic auth and composed keys).
func funnelNeedles(secrets []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		if len(v) < funnelMinSecretLen || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	var addAll func(v string, depth int)
	addAll = func(v string, depth int) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		add(v)
		for _, chain := range funnelNeedleChains {
			add(funnelSettle(v, chain))
		}
		if strings.Contains(v, " ") {
			add(strings.ReplaceAll(v, " ", "+"))
		}
		if depth > 0 {
			return
		}
		if _, rest, ok := strings.Cut(v, " "); ok {
			addAll(rest, 1)
		}
		for _, cand := range []string{v, funnelSchemeless(v)} {
			if pair, ok := funnelBase64Pair(cand); ok {
				addAll(pair, 1)
				if id, secret, ok := strings.Cut(pair, ":"); ok {
					addAll(id, 1)
					addAll(secret, 1)
				}
			}
		}
	}
	for _, s := range secrets {
		addAll(s, 0)
	}
	return out
}

// funnelNeedleChains normalize a secret that itself holds decodable text (a
// literal %41, &amp; or A), for a server that decoded it before echoing.
var funnelNeedleChains = [][]func(string) string{
	{funnelDropUnprintable, funnelJSONUnescape, funnelPercentDecode, html.UnescapeString},
	{funnelDropUnprintable, funnelJSONUnescape, funnelIdentityUnescape, funnelPercentDecode, html.UnescapeString, funnelStripTagsText},
}

func funnelSchemeless(v string) string {
	if _, rest, ok := strings.Cut(v, " "); ok {
		return strings.TrimSpace(rest)
	}
	return v
}

// funnelBase64Pair decodes v (strict, any of the four base64 alphabets) when it
// is printable text holding a ':' (an "id:secret" pair).
func funnelBase64Pair(v string) (string, bool) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		b, err := enc.DecodeString(v)
		if err != nil || len(b) == 0 || !utf8.Valid(b) {
			continue
		}
		text := string(b)
		if !strings.Contains(text, ":") || strings.IndexFunc(text, unicode.IsControl) >= 0 {
			continue
		}
		return text, true
	}
	return "", false
}

// funnelSettle applies decoders in order until nothing changes (bounded).
func funnelSettle(v string, decoders []func(string) string) string {
	for round := 0; round < 32; round++ {
		before := v
		for _, decode := range decoders {
			v = decode(v)
		}
		if v == before {
			break
		}
	}
	return v
}

// funnelBase64Views decodes every base64/base64url run long enough to carry a
// secret, at each of its four alignments (a run can start with text that is not
// part of the blob): one view per alignment, the runs' decodings joined by a
// space.
func funnelBase64Views(view string) []string {
	minB64 := (funnelMinSecretLen*4 + 2) / 3
	var runs []string
	funnelForRuns(view, funnelIsBase64, minB64, func(run string) { runs = append(runs, run) })
	return funnelJoinedViews(runs, 4, minB64, funnelLenientBase64)
}

// funnelHexViews decodes every hex run long enough to carry a secret, at both
// alignments.
func funnelHexViews(view string) []string {
	var runs []string
	funnelForRuns(view, funnelIsHex, 2*funnelMinSecretLen, func(run string) { runs = append(runs, run) })
	return funnelJoinedViews(runs, 2, 2*funnelMinSecretLen, funnelLenientHex)
}

func funnelJoinedViews(runs []string, alignments, minLen int, decode func(string) string) []string {
	if len(runs) == 0 {
		return nil
	}
	var out []string
	for k := 0; k < alignments; k++ {
		var parts []string
		for _, run := range runs {
			if len(run)-k >= minLen {
				if dec := decode(run[k:]); dec != "" {
					parts = append(parts, dec)
				}
			}
		}
		if len(parts) > 0 {
			out = append(out, strings.Join(parts, " "))
		}
	}
	return out
}

func funnelForRuns(view string, in func(byte) bool, minLen int, fn func(string)) {
	start := -1
	for i := 0; i <= len(view); i++ {
		if i < len(view) && in(view[i]) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 && i-start >= minLen {
			fn(view[start:i])
		}
		start = -1
	}
}

func funnelIsBase64(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '/' || b == '-' || b == '_'
}

func funnelIsHex(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

// funnelLenientBase64 decodes run in either alphabet ('+'/'-' = 62, '/'/'_' = 63),
// without padding, keeping a trailing partial group. Byte arithmetic only, so
// no value is ever narrowed.
func funnelLenientBase64(run string) string {
	var b strings.Builder
	var q [4]byte
	n := 0
	for i := 0; i < len(run); i++ {
		q[n] = funnelBase64Value(run[i])
		n++
		if n == 4 {
			b.WriteByte(q[0]<<2 | q[1]>>4)
			b.WriteByte(q[1]<<4 | q[2]>>2)
			b.WriteByte(q[2]<<6 | q[3])
			n = 0
		}
	}
	switch n {
	case 2:
		b.WriteByte(q[0]<<2 | q[1]>>4)
	case 3:
		b.WriteByte(q[0]<<2 | q[1]>>4)
		b.WriteByte(q[1]<<4 | q[2]>>2)
	}
	return b.String()
}

// funnelBase64Value is the 6-bit value of one character of a run (callers pass
// only funnelIsBase64 characters).
func funnelBase64Value(c byte) byte {
	switch {
	case c >= 'A' && c <= 'Z':
		return c - 'A'
	case c >= 'a' && c <= 'z':
		return c - 'a' + 26
	case c >= '0' && c <= '9':
		return c - '0' + 52
	case c == '+' || c == '-':
		return 62
	}
	return 63
}

func funnelLenientHex(run string) string {
	var b strings.Builder
	for i := 0; i+1 < len(run); i += 2 {
		v, err := hex.DecodeString(run[i : i+2])
		if err == nil {
			b.WriteByte(v[0])
		}
	}
	return b.String()
}

// funnelTagRE matches one markup tag.
var funnelTagRE = regexp.MustCompile(`<[^<>]*>`)

// funnelStripTagsText deletes markup tags, as an HTML page's title summary
// does, so a value split by a tag is seen joined.
func funnelStripTagsText(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	return funnelTagRE.ReplaceAllString(s, "")
}

// funnelDropUnprintable deletes invalid UTF-8, control and format (Cf)
// characters, as a display drops them, so an echo split by one is seen joined.
func funnelDropUnprintable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// funnelIdentityUnescape drops the backslash of every escape, as JavaScript's
// identity escape does (\q is q). It is a separate step from
// funnelJSONUnescape, so the view with the backslash kept is searched too.
func funnelIdentityUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// funnelJSONUnescape decodes JSON string escapes (\uXXXX incl. surrogate pairs,
// \/, \", \\, \b \f \n \r \t) and the JavaScript forms \', \v, \0, \xHH and
// \u{H...} wherever they appear, without requiring the text to be valid JSON.
// Any other backslash is kept (funnelIdentityUnescape handles it as its own
// view).
func funnelJSONUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'u':
				if i+3 < len(s) && s[i+2] == '{' {
					if end := strings.IndexByte(s[i+3:min(len(s), i+10)], '}'); end > 0 && end <= 6 {
						if v, err := hex.DecodeString(strings.Repeat("0", end%2) + s[i+3:i+3+end]); err == nil {
							r := rune(0)
							for _, x := range v {
								r = r<<8 | rune(x)
							}
							if utf8.ValidRune(r) {
								b.WriteRune(r)
								i += 3 + end
								continue
							}
						}
					}
				}
				if r, ok := funnelHexRune(s, i+2); ok {
					if utf16.IsSurrogate(r) && i+12 <= len(s) && s[i+6] == '\\' && s[i+7] == 'u' {
						if r2, ok := funnelHexRune(s, i+8); ok {
							if d := utf16.DecodeRune(r, r2); d != unicode.ReplacementChar {
								b.WriteRune(d)
								i += 11
								continue
							}
						}
					}
					b.WriteRune(r)
					i += 5
					continue
				}
			case '/', '"', '\\', '\'':
				b.WriteByte(s[i+1])
				i++
				continue
			case 'x':
				if i+4 <= len(s) {
					if v, err := hex.DecodeString(s[i+2 : i+4]); err == nil {
						b.WriteByte(v[0])
						i += 3
						continue
					}
				}
			case 'b', 'f', 'n', 'r', 't', 'v', '0':
				b.WriteByte(map[byte]byte{'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v', '0': 0}[s[i+1]])
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// funnelHexRune reads the 4 hex digits at s[at:] as one UTF-16 unit.
func funnelHexRune(s string, at int) (rune, bool) {
	if at+4 > len(s) {
		return 0, false
	}
	v, err := hex.DecodeString(s[at : at+4])
	if err != nil {
		return 0, false
	}
	return rune(v[0])<<8 | rune(v[1]), true
}

// funnelPercentDecode decodes every valid %XX (either case) and leaves anything else.
func funnelPercentDecode(s string) string {
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
