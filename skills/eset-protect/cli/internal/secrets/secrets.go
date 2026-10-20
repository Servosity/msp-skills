// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "secret-registry-masks-every-boundary".

// Package secrets is the process-wide registry of every credential value this
// process holds or learns, and the one function that masks them in text.
//
// A value is registered at the moment it is read or minted, in whichever
// process reads it (the CLI and the MCP server each keep their own):
//
//   - ESET_PROTECT_PASSWORD, ESET_PROTECT_TOKEN and ESET_PROTECT_MCP_HTTP_TOKEN
//     at process start (init below) and on every read (cliutil.EnvOverride);
//   - every credential field of the config and credentials files on every
//     load and save (config.Load, Config.SaveTokens, cliutil's credentials
//     loader);
//   - every access and refresh token the ESET token service returns, whether
//     or not the answer was a success, and every bearer the 401 re-mint
//     publishes on the transport (EsetTransport.SetAuth);
//   - every credential the API client masks with (client.maskCredentialText).
//
// Mask is applied wherever server-derived or credential-adjacent text leaves
// the process: token-service and API error text, response bodies, doctor
// rows, every MCP tool result, --deliver payloads, and, as the last line, the
// CLI's stdout/stderr writer (c1_escape.go).
//
// What is masked, ASCII case-insensitively, by "[redacted]": the raw value,
// its JSON string escapes (Go's HTML-safe form, the plain form, both with "/"
// escaped as "\/", and the all-ASCII \uXXXX form other encoders emit), its
// URL query and path escapes (the case-insensitive match covers either hex
// case) and its HTML escape. Overlapping matches are merged, so two secrets
// that overlap in the text never leave a tail of either visible.
//
// Before anything is replaced, a fail-closed backstop decodes the text (JSON
// escapes, URL %XX, HTML references, "+" as a space) pass by pass until it
// stops changing. For every registered value it finds, it decodes the text
// again with that value's own in-place matches cut out; if the value is
// still there, one of its occurrences is in a spelling no fixed form covers
// (mixed, nested, per-character), and the whole text becomes Withheld. The
// check is per value and on the original text, so masking one value (or a
// shorter value inside it) can never hide the evidence of another. Text
// still changing after maxDecodePasses passes is withheld too.
//
// base64 is deliberately not masked. No credential this CLI holds is ever
// sent base64-encoded (the password and refresh grants are form-encoded, the
// bearer is sent raw), so a server can only echo the forms it received, and
// the only base64 the CLI emits is the envelope of a binary download, whose
// bytes must stay exact.
//
// Values shorter than MinLen characters are ignored, so an ordinary word is
// never masked.
package secrets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	// MinLen is the shortest value (in characters) the registry masks.
	MinLen = 8
	// Redacted replaces a registered value.
	Redacted = "[redacted]"
	// Withheld replaces a whole text that still holds a registered value
	// after masking, in an encoding no fixed form matched. Its wording keeps
	// clear of the words doctor classifies rows by.
	Withheld = "[withheld: this text holds a credential in an encoded form and is hidden whole]"
	// maxDecodePasses bounds the backstop's decoding. Every pass that changes
	// the text shortens it, but a hostile text ("%25" repeated) shortens by
	// two bytes a pass; real text nests an escape two or three levels.
	maxDecodePasses = 16
)

// CredentialEnvVars are the environment variables that hold a credential.
var CredentialEnvVars = []string{"ESET_PROTECT_PASSWORD", "ESET_PROTECT_TOKEN", "ESET_PROTECT_MCP_HTTP_TOKEN"}

// IsCredentialEnv reports whether name is one of CredentialEnvVars.
func IsCredentialEnv(name string) bool {
	for _, n := range CredentialEnvVars {
		if n == name {
			return true
		}
	}
	return false
}

func init() {
	for _, n := range CredentialEnvVars {
		Register(os.Getenv(n))
	}
}

// snapshot is an immutable view of the registry; Register publishes a new one.
type snapshot struct {
	needles []needle // lower-cased masking forms, longest first
	values  []string // lower-cased registered values (the backstop's target)
	broken  []string // needles holding a line break (HoldBack)
}

// needle is one masking form and the values (indexes into values) it spells.
type needle struct {
	text   string
	owners []int
}

var (
	mu    sync.Mutex
	known = map[string]struct{}{}
	cur   atomic.Pointer[snapshot]
)

// Register records every value as a secret of this process. It is safe for
// concurrent use and cheap for a value already known. A value is recorded
// as given, trimmed, and, when it reads "<scheme> <token>" (an Authorization
// header), as the token alone. Empty values, values shorter than MinLen and
// an MCPB placeholder left unresolved ("${...}") are ignored.
func Register(values ...string) {
	var add []string
	for _, v := range values {
		for _, c := range candidates(v) {
			if utf8.RuneCountInString(c) >= MinLen && !unresolvedPlaceholder(c) {
				add = append(add, c)
			}
		}
	}
	if len(add) == 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	changed := false
	for _, c := range add {
		if _, ok := known[c]; !ok {
			known[c] = struct{}{}
			changed = true
		}
	}
	if changed {
		cur.Store(build())
	}
}

// Reset forgets every registered value. Tests only: a process never
// unregisters a credential.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	known = map[string]struct{}{}
	cur.Store(nil)
}

// HasLineBreakSecret reports whether a registered value's raw form holds a
// line break.
func HasLineBreakSecret() bool {
	sn := cur.Load()
	return sn != nil && len(sn.broken) > 0
}

// Active reports whether any value is registered.
func Active() bool {
	sn := cur.Load()
	return sn != nil && len(sn.values) > 0
}

// authSchemes are the Authorization schemes whose token Register records on
// its own.
var authSchemes = map[string]bool{"bearer": true, "basic": true, "token": true, "negotiate": true}

func candidates(v string) []string {
	out := []string{v}
	t := strings.TrimSpace(v)
	if t != v {
		out = append(out, t)
	}
	if scheme, tok, ok := strings.Cut(t, " "); ok && authSchemes[strings.ToLower(scheme)] {
		if tok = strings.TrimSpace(tok); tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

func unresolvedPlaceholder(v string) bool {
	return len(v) >= 3 && strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") && !strings.Contains(v[2:len(v)-1], "}")
}

// build derives the snapshot from known. Called with mu held.
func build() *snapshot {
	sn := &snapshot{}
	values := map[string]struct{}{}
	for v := range known {
		values[lowerASCII(v)] = struct{}{}
	}
	for v := range values {
		sn.values = append(sn.values, v)
	}
	sort.Strings(sn.values)
	index := map[string]int{}
	for i, v := range sn.values {
		index[v] = i
	}
	owners := map[string]map[int]struct{}{}
	for v := range known {
		owner := index[lowerASCII(v)]
		for _, f := range forms(v) {
			if f == "" {
				continue
			}
			f = lowerASCII(f)
			if owners[f] == nil {
				owners[f] = map[int]struct{}{}
			}
			owners[f][owner] = struct{}{}
		}
	}
	for text, set := range owners {
		n := needle{text: text}
		for o := range set {
			n.owners = append(n.owners, o)
		}
		sort.Ints(n.owners)
		sn.needles = append(sn.needles, n)
		if strings.ContainsAny(text, "\r\n") {
			sn.broken = append(sn.broken, text)
		}
	}
	sort.Slice(sn.needles, func(i, j int) bool {
		a, b := sn.needles[i].text, sn.needles[j].text
		if len(a) != len(b) {
			return len(a) > len(b)
		}
		return a < b
	})
	sort.Strings(sn.broken)
	return sn
}

// forms are the spellings of v masked in place.
func forms(v string) []string {
	safe, plain := jsonForm(v, true), jsonForm(v, false)
	return []string{
		v,
		safe,
		plain,
		strings.ReplaceAll(safe, "/", `\/`),
		strings.ReplaceAll(plain, "/", `\/`),
		asciiJSON(v),
		url.QueryEscape(v),
		url.PathEscape(v),
		html.EscapeString(v),
	}
}

// jsonForm is v as encoding/json writes it inside a string, without quotes.
func jsonForm(v string, htmlSafe bool) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(htmlSafe)
	if err := enc.Encode(v); err != nil {
		return ""
	}
	s := strings.TrimSuffix(b.String(), "\n")
	if len(s) < 2 {
		return ""
	}
	return s[1 : len(s)-1]
}

// asciiJSON is v as an all-ASCII JSON encoder writes it (Python's default,
// many Java and .NET encoders): every non-ASCII rune as \uXXXX, astral runes
// as a surrogate pair.
func asciiJSON(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		case r < utf8.RuneSelf:
			b.WriteRune(r)
		case r > 0xFFFF:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\u%04x\u%04x`, hi, lo)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	return b.String()
}

// lowerASCII lowers A-Z only, so byte offsets in the result match s.
func lowerASCII(s string) string {
	i := 0
	for i < len(s) && (s[i] < 'A' || s[i] > 'Z') {
		i++
	}
	if i == len(s) {
		return s
	}
	b := []byte(s)
	for ; i < len(b); i++ {
		if 'A' <= b[i] && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// Mask returns s with every registered value, in every form above, replaced
// by Redacted, or Withheld when a registered value occurs in a spelling no
// form covers (or the text is still decoding after the pass budget).
func Mask(s string) string {
	sn := cur.Load()
	if sn == nil || s == "" {
		return s
	}
	ms := findMatches(lowerASCII(s), sn)
	if leaks(s, ms, sn) {
		return Withheld
	}
	return applyMatches(s, ms)
}

// MaskBytes is Mask for bytes; b itself is returned when nothing changed.
func MaskBytes(b []byte) []byte {
	if len(b) == 0 || cur.Load() == nil {
		return b
	}
	s := string(b)
	m := Mask(s)
	if m == s {
		return b
	}
	return []byte(m)
}

// MaskJSON masks a response body. ok is false when the body must be withheld:
// a registered value is only recoverable by decoding, or masking turned a
// valid JSON value into invalid JSON (a secret spelled as a JSON number, or
// one whose raw form spans the document's structure).
func MaskJSON(b []byte) (out []byte, ok bool) {
	m := MaskBytes(b)
	if bytes.Equal(m, b) {
		return b, true
	}
	if string(m) == Withheld || (json.Valid(b) && !json.Valid(m)) {
		return nil, false
	}
	return m, true
}

// match is one occurrence of a needle: bytes [a, b) of the text.
type match struct {
	a, b   int
	owners []int
}

// findMatches returns every occurrence (overlaps included) of every needle
// in low, the ASCII-lowered text.
func findMatches(low string, sn *snapshot) []match {
	var ms []match
	for _, n := range sn.needles {
		for off := 0; off+len(n.text) <= len(low); {
			i := strings.Index(low[off:], n.text)
			if i < 0 {
				break
			}
			a := off + i
			ms = append(ms, match{a: a, b: a + len(n.text), owners: n.owners})
			off = a + 1
		}
	}
	return ms
}

// applyMatches replaces the union of the matches with Redacted, one per
// covered run, so overlapping secrets are hidden whole.
func applyMatches(s string, ms []match) string {
	if len(ms) == 0 {
		return s
	}
	cover := make([]bool, len(s))
	for _, m := range ms {
		for k := m.a; k < m.b; k++ {
			cover[k] = true
		}
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if cover[i] {
			b.WriteString(Redacted)
			for i < len(s) && cover[i] {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// leaks is the fail-closed backstop. It finds every registered value s
// spells after decoding, then, for each, decodes s again with that value's
// own in-place matches cut out: a value still there occurs in a spelling the
// in-place pass would leave visible.
func leaks(s string, ms []match, sn *snapshot) bool {
	present, exhausted := decodedValues(s, sn, -1)
	if exhausted {
		return true
	}
	for v := range present {
		if still, exhausted := decodedValues(cutOwn(s, ms, v), sn, v); len(still) > 0 || exhausted {
			return true
		}
	}
	return false
}

// cutOwn returns s with every match owned by value v replaced by a NUL, so
// neither the occurrence nor a join across it can spell v.
func cutOwn(s string, ms []match, v int) string {
	cut := make([]bool, len(s))
	any := false
	for _, m := range ms {
		for _, o := range m.owners {
			if o == v {
				for k := m.a; k < m.b; k++ {
					cut[k] = true
				}
				any = true
				break
			}
		}
	}
	if !any {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if cut[i] {
			b.WriteByte(0)
			for i < len(s) && cut[i] {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// decodedValues decodes s pass by pass and reports the registered values
// (all of them, or only value only when only >= 0) that s or any pass
// spells, and whether s was still changing when the pass budget ran out.
// "Spells" is spellsValue: every character raw or in any one encoding, so a
// value echoed with some characters encoded and others not, or one whose own
// text looks like an escape ("%3E"), is found at the depth where it occurs.
func decodedValues(s string, sn *snapshot, only int) (found map[int]bool, exhausted bool) {
	found = map[int]bool{}
	text := s
	for pass := 0; ; pass++ {
		for i, v := range sn.values {
			if (only < 0 || i == only) && !found[i] && spellsValue(text, v) {
				found[i] = true
			}
		}
		next := decodeOnce(text)
		if next == text {
			return found, false
		}
		if pass == maxDecodePasses {
			return found, true
		}
		text = next
	}
}

// spellDepth is how deeply spellsValue nests encodings inside one another
// per character (the "\\u0026" of JSON inside JSON is depth 2). Deeper
// nesting is reached by the whole-text decoding passes of decodedValues.
const spellDepth = 2

// spellsValue reports whether text holds value (ASCII lower-cased) with every
// character spelled raw (ASCII case-insensitively) or in an encoding whose
// own characters are spelled the same way, spellDepth levels deep: URL %XX
// per UTF-8 byte (either hex case), JSON \uXXXX (a surrogate pair for an
// astral rune) and short escapes, HTML decimal, hex and named references;
// a space and "+" stand for each other at every level.
func spellsValue(text, value string) bool {
	if value == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(value)
	m := &speller{text: text}
	for i := 0; i < len(text); i++ {
		if mayStart(text[i], first) && m.spells(i, value) {
			return true
		}
	}
	return false
}

// escapeStart reports whether c can begin an encoding.
func escapeStart(c byte) bool { return c == '%' || c == '\\' || c == '&' }

// plausibleEscape reports whether an encoding can begin at text[p]: "%" and
// two hex digits, a backslash and an escape letter (or a second backslash),
// or "&" and "#" or the first letter of a named reference. Every encoding,
// nested or not, begins one of these ways.
func plausibleEscape(text string, p int) bool {
	if p+1 >= len(text) {
		return false
	}
	switch text[p] {
	case '%':
		if p+2 >= len(text) {
			return false
		}
		_, ok1 := hexVal(text[p+1])
		_, ok2 := hexVal(text[p+2])
		return ok1 && ok2
	case '\\':
		return strings.IndexByte(`uU\"/bfnrt`, text[p+1]) >= 0
	case '&':
		return strings.IndexByte("#aAlLgGqQ", text[p+1]) >= 0
	}
	return false
}

// mayStart is a cheap filter: can a spelling of r begin with byte c?
func mayStart(c byte, r rune) bool {
	switch {
	case escapeStart(c):
		return true
	case c == ' ' || c == '+':
		return r == ' ' || r == '+'
	case r < utf8.RuneSelf:
		return rune(lowerByte(c)) == lowerRune(r)
	}
	return c >= utf8.RuneSelf
}

// lowerRune lowers A-Z only; the comparison stays in rune space, so no
// conversion can narrow.
func lowerRune(r rune) rune {
	if 'A' <= r && r <= 'Z' {
		return r + 'a' - 'A'
	}
	return r
}

func lowerByte(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// rawRune matches r spelled raw at text[p] (ASCII case-insensitively, a
// space and "+" for each other).
func rawRune(text string, p int, r rune) (int, bool) {
	if p >= len(text) {
		return p, false
	}
	if r < utf8.RuneSelf {
		c := text[p]
		if rune(lowerByte(c)) == lowerRune(r) || ((r == ' ' || r == '+') && (c == ' ' || c == '+')) {
			return p + 1, true
		}
		return p, false
	}
	if strings.HasPrefix(text[p:], string(r)) {
		return p + utf8.RuneLen(r), true
	}
	return p, false
}

// speller matches values against one text, reusing a frontier buffer pair
// per nesting depth (spellString never recurses at the same depth).
type speller struct {
	text string
	bufs [spellDepth + 1][2][]int
}

// spells reports whether a spelling of value starts at text[p]. Up to the
// first escape character the only spelling is raw, so that prefix is matched
// without the frontier machinery.
func (m *speller) spells(p int, value string) bool {
	q := p
	for i, r := range value {
		if q < len(m.text) && escapeStart(m.text[q]) {
			return len(m.spellString(q, value[i:], spellDepth)) > 0
		}
		end, ok := rawRune(m.text, q, r)
		if !ok {
			return false
		}
		q = end
	}
	return true
}

// spellString returns every position in text where a spelling of s that
// starts at p can end. The result aliases a scratch buffer of this depth.
func (m *speller) spellString(p int, s string, depth int) []int {
	cur := append(m.bufs[depth][0][:0], p)
	alt := m.bufs[depth][1][:0]
	for _, r := range s {
		alt = alt[:0]
		for _, q := range cur {
			alt = m.appendSpellings(alt, q, r, depth)
		}
		if len(alt) == 0 {
			m.bufs[depth][0], m.bufs[depth][1] = cur, alt
			return nil
		}
		alt = dedupeInts(alt)
		cur, alt = alt, cur
	}
	m.bufs[depth][0], m.bufs[depth][1] = cur, alt
	return cur
}

func dedupeInts(xs []int) []int {
	if len(xs) < 2 {
		return xs
	}
	sort.Ints(xs)
	out := xs[:1]
	for _, x := range xs[1:] {
		if x != out[len(out)-1] {
			out = append(out, x)
		}
	}
	return out
}

// appendSpellings appends every end position of a spelling of r that starts
// at text[p].
func (m *speller) appendSpellings(ends []int, p int, r rune, depth int) []int {
	if p >= len(m.text) {
		return ends
	}
	if end, ok := rawRune(m.text, p, r); ok {
		ends = append(ends, end)
	}
	// Every encoding begins with "%", a backslash or "&", raw or nested.
	if depth == 0 || !plausibleEscape(m.text, p) {
		return ends
	}
	ends = m.appendEncoded(ends, p, r, depth)
	// Values are lower-cased; an encoded letter may be either case.
	if 'a' <= r && r <= 'z' {
		ends = m.appendEncoded(ends, p, r-'a'+'A', depth)
	}
	return ends
}

// appendEncoded appends the end of every encoding of r spelled at text[p]
// with its own characters spelled depth-1 levels deep. At depth 1 the
// encoding must appear literally (hex and letters in either case).
func (m *speller) appendEncoded(ends []int, p int, r rune, depth int) []int {
	for _, enc := range encodingsOf(r) {
		if depth == 1 {
			if hasPrefixFold(m.text[p:], enc) {
				ends = append(ends, p+len(enc))
			}
			continue
		}
		ends = append(ends, m.spellString(p, enc, depth-1)...)
	}
	return ends
}

// hasPrefixFold is strings.HasPrefix with ASCII letters in either case.
func hasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if lowerByte(s[i]) != lowerByte(prefix[i]) {
			return false
		}
	}
	return true
}

var (
	encodingCache sync.Map // rune -> []string, for non-ASCII runes
	asciiEncoding [utf8.RuneSelf][]string
)

func init() {
	for r := rune(0); r < utf8.RuneSelf; r++ {
		asciiEncoding[r] = buildEncodings(r)
	}
}

// encodingsOf lists the encodings of r that spellsValue accepts, in their
// canonical spelling (hex case and nesting are handled by the matcher).
func encodingsOf(r rune) []string {
	if r >= 0 && r < utf8.RuneSelf {
		return asciiEncoding[r]
	}
	if v, ok := encodingCache.Load(r); ok {
		return v.([]string)
	}
	out := buildEncodings(r)
	encodingCache.Store(r, out)
	return out
}

func buildEncodings(r rune) []string {
	var out []string
	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], r)
	var pct strings.Builder
	for k := 0; k < n; k++ {
		fmt.Fprintf(&pct, "%%%02X", buf[k])
	}
	out = append(out, pct.String())
	if r > 0xFFFF {
		hi, lo := utf16.EncodeRune(r)
		out = append(out, fmt.Sprintf(`\u%04X\u%04X`, hi, lo))
	} else {
		out = append(out, fmt.Sprintf(`\u%04X`, r))
	}
	switch r {
	case '"', '\\', '/':
		out = append(out, `\`+string(r))
	case '\b':
		out = append(out, `\b`)
	case '\f':
		out = append(out, `\f`)
	case '\n':
		out = append(out, `\n`)
	case '\r':
		out = append(out, `\r`)
	case '\t':
		out = append(out, `\t`)
	}
	out = append(out, fmt.Sprintf("&#%d;", r), fmt.Sprintf("&#x%X;", r))
	for name, v := range htmlNamed {
		if v == r {
			out = append(out, "&"+name+";")
		}
	}
	sort.Strings(out)
	return out
}

// htmlNamed are the named references html.EscapeString and common encoders
// write for ASCII.
var htmlNamed = map[string]rune{"amp": '&', "lt": '<', "gt": '>', "quot": '"', "apos": '\''}

// decodeOnce decodes, in one left-to-right pass, JSON string escapes
// (\uXXXX with surrogate pairs, \" \\ \/ \b \f \n \r \t) and URL %XX escapes,
// then HTML character references.
func decodeOnce(s string) string {
	if !strings.ContainsAny(s, `%\&`) {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			if s[i+1] == 'u' {
				if r, ok := hex4(s[i+2:]); ok {
					n := 6
					if 0xD800 <= r && r < 0xDC00 && i+12 <= len(s) && s[i+6] == '\\' && s[i+7] == 'u' {
						if lo, ok := hex4(s[i+8:]); ok && 0xDC00 <= lo && lo < 0xE000 {
							r = utf16.DecodeRune(r, lo)
							n = 12
						}
					}
					out = utf8.AppendRune(out, r)
					i += n
					continue
				}
			}
			if d, ok := shortEscape(s[i+1]); ok {
				out = append(out, d)
				i += 2
				continue
			}
		}
		if c == '%' && i+2 < len(s) {
			if hi, ok := hexVal(s[i+1]); ok {
				if lo, ok := hexVal(s[i+2]); ok {
					out = append(out, hi<<4|lo)
					i += 3
					continue
				}
			}
		}
		out = append(out, c)
		i++
	}
	text := string(out)
	if strings.Contains(text, "&") {
		text = html.UnescapeString(text)
	}
	return text
}

func shortEscape(c byte) (byte, bool) {
	switch c {
	case '"', '\\', '/':
		return c, true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	}
	return 0, false
}

// hexVal returns the value (0-15) of one hex digit, as a byte so callers
// build a byte or a rune by widening only.
func hexVal(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

func hex4(s string) (rune, bool) {
	if len(s) < 4 {
		return 0, false
	}
	var r rune
	for i := 0; i < 4; i++ {
		v, ok := hexVal(s[i])
		if !ok {
			return 0, false
		}
		r = r<<4 | rune(v)
	}
	return r, true
}

// HoldBack moves a writer's cut point back so that a registered value whose
// form holds a line break is never split across two units. cut is the
// length of the complete-line prefix of b; the result is <= cut. Only forms
// with a line break can span a cut made right after a newline.
func HoldBack(b []byte, cut int) int {
	sn := cur.Load()
	if sn == nil || len(sn.broken) == 0 {
		return cut
	}
	best := cut
	for _, n := range sn.broken {
		for k := len(n) - 1; k >= 1; k-- {
			if k > cut {
				continue
			}
			if lowerASCII(string(b[cut-k:cut])) == n[:k] {
				if cut-k < best {
					best = cut - k
				}
				break
			}
		}
	}
	return best
}
