// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-withholds-credentials" and
// "mcp-read-only-redacts-before-bounding" entries.

// Package redact keeps credentials out of MCP tool results while the
// DNSFILTER_MCP_READ_ONLY switch is on. It has two entry points:
//
//   - Raw runs on a full, unbounded body BEFORE internal/mcp/bound budgets it
//     (bound calls it from EndpointResponse, EndpointPageResponse, JSON and
//     Text), so a truncated preview can only ever quote already-redacted text.
//   - Final runs on every result text in the MCP result funnel, after
//     bounding, and fails closed: it also withholds what it cannot decode.
//
// Matching is by key name on decoded JSON (store.IsSecretKey plus a trailing
// "pin"), never by guessing at values.
package redact

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/store"
)

// WithheldFieldPlaceholder replaces the value of a credential-named field;
// it is the store scrub's placeholder text (one definition).
const WithheldFieldPlaceholder = store.WithheldValuePlaceholder

// WithheldBlockMessage replaces a whole text block that names a credential
// field outside JSON (CSV or table headers, key=value prose) or carries a
// value this process already withheld.
const WithheldBlockMessage = "withheld: this result carries a credential-bearing field and " + cliutil.MCPReadOnlyEnvVar + " is set; read-only MCP never returns credentials (API key tokens, network secret keys, uninstall PINs, client secrets)"

// Enabled reports whether the read-only switch is on.
func Enabled() bool { return cliutil.IsMCPReadOnlyEnv() }

// IsCredentialKey is the store's secret-key rule, one definition for the
// local mirror and the MCP funnel (store.IsSecretKey: secret, password,
// apikey, credential, a trailing token or pin, case and separator
// insensitive, a trailing id/uuid word excluded).
func IsCredentialKey(key string) bool { return store.IsSecretKey(key) }

// CredentialPathSegments are the DNSFilter API path segments whose GET answer
// IS a credential: /v1/api_keys and /v1/api_keys/{id} (ApiKey.token),
// /v1/networks/{id}/secret_key (the site deployment secret),
// /v1/user_agents/uninstall_pin (the roaming client uninstall PIN, a bare JSON
// string no key-name redaction can see) and
// /v1/psa_integrations/redirect_link (a one-time token, and a GET that creates
// an integration). It is the same set the response cache refuses to store
// (internal/client responseCacheCredentialPath); TestCredentialPathSegmentsMatchResponseCache
// fails if the two drift.
var CredentialPathSegments = map[string]bool{
	"api_keys":      true,
	"uninstall_pin": true,
	"secret_key":    true,
	"redirect_link": true,
}

// maxPathDecodeRounds bounds the repeated percent-decoding of a request path,
// so double and triple encoding (uninstall%255Fpin) are seen through.
const maxPathDecodeRounds = 4

// IsCredentialRequestPath reports whether a request path, as it will be sent
// (placeholders filled, values percent-encoded), reaches a credential
// endpoint. It runs on the RESOLVED path, never a template, and normalizes the
// ways a caller-supplied id can spell a segment: the query and fragment are
// dropped, percent-encoding is decoded repeatedly, backslash counts as a
// separator, case is folded, empty segments (duplicate or trailing slashes)
// are skipped, a segment is cut at ';' (matrix params), '.' (a format
// suffix such as uninstall_pin.json; never inside a hostname such as
// secret-key.io, which is a domain value), any '?' or '#' an encoded value
// smuggled in and a malformed escape left after decoding, and '-' reads as
// '_'.
func IsCredentialRequestPath(path string) bool {
	p := path
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	for round := 0; round < maxPathDecodeRounds && strings.Contains(p, "%"); round++ {
		decoded := lenientPercentDecode(p)
		if decoded == p {
			break
		}
		p = decoded
	}
	p = strings.ToLower(strings.ReplaceAll(p, "\\", "/"))
	for _, seg := range strings.Split(p, "/") {
		seg = strings.TrimFunc(seg, func(r rune) bool { return r <= ' ' || r == 0x7f })
		// A hostname (secret-key.io, api-keys.example.com) is a domain value,
		// not a format suffix: only a non-hostname segment is cut at '.'.
		cut := ";?#%"
		if !hostnameLike(seg) {
			cut += "."
		}
		if i := strings.IndexAny(seg, cut); i >= 0 {
			seg = seg[:i]
		}
		seg = strings.ReplaceAll(strings.TrimFunc(seg, func(r rune) bool { return r <= ' ' || r == 0x7f }), "-", "_")
		if seg != "" && CredentialPathSegments[seg] {
			return true
		}
	}
	return false
}

// formatSuffixes are the response-format extensions a Rails-style route
// accepts after a segment (uninstall_pin.json). None is a DNS top-level
// domain, so a dotted token ending in one is a path, never a hostname.
var formatSuffixes = map[string]bool{"json": true, "xml": true, "csv": true, "txt": true, "text": true, "html": true, "htm": true, "js": true, "yaml": true, "yml": true, "atom": true, "rss": true, "pdf": true}

// hostnameLabelRE is one DNS label (letters, digits, hyphen; no underscore).
var hostnameLabelRE = regexp.MustCompile(`^[a-z0-9-]+$`)

// hostnameLike reports whether a lowercased segment reads as a hostname:
// two or more DNS labels and a last label that is not a format suffix.
// uninstall_pin.json (underscore, .json) and uninstall-pin.json (.json) are
// not; secret-key.io and api-keys.example.com are.
func hostnameLike(seg string) bool {
	labels := strings.Split(seg, ".")
	if len(labels) < 2 || formatSuffixes[labels[len(labels)-1]] {
		return false
	}
	for _, l := range labels {
		if !hostnameLabelRE.MatchString(l) {
			return false
		}
	}
	return true
}

// lenientPercentDecode decodes every valid %XX escape and leaves a malformed
// one in place, so one bad escape cannot hide the valid ones beside it (which
// url.PathUnescape would refuse to decode at all).
func lenientPercentDecode(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			v, _ := url.PathUnescape(s[i : i+3])
			b.WriteString(v)
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// Raw replaces the value of every credential-named field in every decodable
// JSON value of text (the whole text, NDJSON, JSON quoted in an error, JSON
// nested in a string). It never withholds a whole block: a body bigger than
// the MCP budget is only ever partly emitted, so the block decision belongs
// to Final, on the text that is actually returned. Off the read-only switch it
// returns text unchanged.
func Raw(text string) string {
	if text == "" || !Enabled() {
		return text
	}
	out, _ := redactJSONSegments(text)
	return out
}

// Final is the fail-closed filter on a result text that is about to leave the
// server: Raw's redaction, plus a string value that cannot be decoded (a
// bounded preview cut mid-record) but still names a credential field with a
// value is replaced whole, plain text that names one as a CSV/table header or
// key=value pair is withheld whole, and any text carrying a value this process
// already withheld is withheld whole.
func Final(text string) string {
	if text == "" || !Enabled() {
		return text
	}
	out, plain := redactJSONSegments(text)
	if textNamesCredential(out, plain) || carriesWithheldValue(out) {
		return WithheldBlockMessage
	}
	return out
}

// maxFailedSegmentDecodes bounds the embedded-JSON scan on text that is not
// one JSON stream (a capped, truncated stdout; a result full of stray
// brackets), so it cannot go quadratic.
const maxFailedSegmentDecodes = 256

// redactJSONSegments returns text with every decodable JSON object or array
// redacted, plus the concatenated non-JSON remainder for the plain-text check.
// After a failed decode the scan moves one byte on, so the complete records
// inside a truncated container are still decoded and redacted one by one.
func redactJSONSegments(text string) (string, string) {
	var out, plain strings.Builder
	failures := 0
	i := 0
	for i < len(text) {
		j := strings.IndexAny(text[i:], "{[")
		if j < 0 || failures >= maxFailedSegmentDecodes {
			out.WriteString(text[i:])
			plain.WriteString(text[i:])
			break
		}
		start := i + j
		out.WriteString(text[i:start])
		plain.WriteString(text[i:start])
		plain.WriteByte(' ')
		dec := json.NewDecoder(strings.NewReader(text[start:]))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			failures++
			out.WriteByte(text[start])
			plain.WriteByte(text[start])
			i = start + 1
			continue
		}
		end := start + int(dec.InputOffset())
		// The raw bytes are kept only when a token walk of them shows no
		// credential-named key with a value and no duplicate key: a decoded
		// map keeps the LAST of {"token":"S","token":null}, so a clean decoded
		// value does not prove clean bytes.
		if cleaned, changed := redactValue(v); changed || !rawJSONClean(text[start:end]) {
			out.WriteString(encodeJSONCompact(cleaned))
		} else {
			out.WriteString(text[start:end])
		}
		i = end
	}
	return out.String(), plain.String()
}

func encodeJSONCompact(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return `"` + WithheldFieldPlaceholder + `"`
	}
	return strings.TrimRight(buf.String(), "\n")
}

// redactValue replaces the value of every credential-named key at any depth.
// A boolean or null value is kept (it is a flag, not a secret). A string that
// holds JSON is redacted inside; a string that does not decode but names a
// credential field with a value (a preview cut mid-record) is replaced whole.
func redactValue(value any) (any, bool) {
	switch v := value.(type) {
	case map[string]any:
		changed := false
		for key, child := range v {
			if IsCredentialKey(key) {
				switch child.(type) {
				case nil, bool:
				default:
					rememberWithheld(child)
					if s, ok := child.(string); ok && s == WithheldFieldPlaceholder {
						continue
					}
					v[key] = WithheldFieldPlaceholder
					changed = true
					continue
				}
			}
			if cleaned, childChanged := redactValue(child); childChanged {
				v[key] = cleaned
				changed = true
			}
		}
		return v, changed
	case []any:
		changed := false
		for i, child := range v {
			if cleaned, childChanged := redactValue(child); childChanged {
				v[i] = cleaned
				changed = true
			}
		}
		return v, changed
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed != "" && strings.ContainsAny(trimmed[:1], "{[\"") {
			// JSON only if the WHOLE string is exactly one JSON value
			// (Decoder.More is not an EOF test: {"ok":1}} token=S would pass).
			if inner, ok := decodeExactlyOne(trimmed); ok {
				if cleaned, changed := redactValue(inner); changed || !rawJSONClean(trimmed) {
					return encodeJSONCompact(cleaned), true
				}
				return v, false
			}
		}
		// Not exactly one JSON value: redact the complete records inside it,
		// then run the text rule over the whole (redacted) string.
		inner, plain := redactJSONSegments(v)
		if textNamesCredential(inner, plain) {
			return WithheldFieldPlaceholder, true
		}
		if inner != v {
			return inner, true
		}
	}
	return value, false
}

// decodeExactlyOne decodes s as JSON only when it is exactly one JSON value
// followed by nothing but whitespace (Token returns io.EOF).
func decodeExactlyOne(s string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return v, true
}

// rawJSONClean walks the raw tokens of one JSON value and reports whether it
// can be returned byte-for-byte: no object repeats a key, and no
// credential-named key carries anything but null or a boolean at any
// occurrence. Strings are not descended into here; redactValue already
// judged every string value, and reports a change when one needed it.
func rawJSONClean(raw string) bool {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	type frame struct {
		object    bool
		expectKey bool
		keys      map[string]bool
	}
	var stack []*frame
	credValue := false
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].expectKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return len(stack) == 0
		}
		if err != nil {
			return false
		}
		if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].expectKey {
			if d, ok := tok.(json.Delim); ok && d == '}' {
				stack = stack[:n-1]
				valueDone()
				continue
			}
			k, ok := tok.(string)
			if !ok || stack[n-1].keys[k] {
				return false
			}
			stack[n-1].keys[k] = true
			stack[n-1].expectKey = false
			credValue = IsCredentialKey(k)
			continue
		}
		if credValue {
			switch tok.(type) {
			case nil, bool:
			default:
				return false
			}
			credValue = false
		}
		switch d := tok.(type) {
		case json.Delim:
			switch d {
			case '{':
				stack = append(stack, &frame{object: true, expectKey: true, keys: map[string]bool{}})
			case '[':
				stack = append(stack, &frame{})
			case ']', '}':
				if len(stack) == 0 {
					return false
				}
				stack = stack[:len(stack)-1]
				valueDone()
			}
		default:
			valueDone()
		}
	}
}

// maxUnescapeRounds bounds how many levels of JSON string escaping the text
// rule sees through (JSON quoted in a string quoted in a string ...).
const maxUnescapeRounds = 6

// unescapeForTextRule undoes JSON string escaping, one level per round, so the
// text rule reads a key name, a quote, a separator or a newline however
// deeply it was escaped (\u0065, \", \n). Malformed escapes stay as they are.
func unescapeForTextRule(s string) string {
	for round := 0; round < maxUnescapeRounds && strings.Contains(s, "\\"); round++ {
		var b strings.Builder
		b.Grow(len(s))
		for i := 0; i < len(s); i++ {
			if s[i] != '\\' || i+1 >= len(s) {
				b.WriteByte(s[i])
				continue
			}
			switch c := s[i+1]; c {
			case '"', '\\', '/':
				b.WriteByte(c)
				i++
			case 'n':
				b.WriteByte('\n')
				i++
			case 'r':
				b.WriteByte('\r')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case 'b', 'f':
				b.WriteByte(' ')
				i++
			case 'u':
				if i+5 < len(s) && isHex(s[i+2]) && isHex(s[i+3]) && isHex(s[i+4]) && isHex(s[i+5]) {
					var r rune
					for _, h := range s[i+2 : i+6] {
						r = r*16 + rune(hexVal(byte(h)))
					}
					b.WriteRune(r)
					i += 5
					continue
				}
				b.WriteByte(s[i])
			default:
				b.WriteByte(s[i])
			}
		}
		next := b.String()
		if next == s {
			break
		}
		s = next
	}
	return s
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

// textNamesCredential is the text rule: a key=value / key: value pair
// anywhere in the whole (redacted) text, or a CSV/table header in its
// non-JSON remainder, after undoing JSON string escaping.
func textNamesCredential(whole, plain string) bool {
	return keyValueNamesCredential(unescapeForTextRule(whole)) || headerNamesCredential(unescapeForTextRule(plain))
}

var (
	withheldMu     sync.Mutex
	withheldValues = map[string]bool{}
)

// minWithheldValueLen keeps short values (a 4-digit PIN, "true") out of the
// backstop, where they would withhold unrelated text.
const minWithheldValueLen = 8

// maxWithheldValues caps the backstop's memory.
const maxWithheldValues = 4096

func rememberWithheld(v any) {
	var s string
	switch tv := v.(type) {
	case string:
		s = tv
	case json.Number:
		s = tv.String()
	default:
		return
	}
	s = strings.TrimSpace(s)
	if len(s) < minWithheldValueLen || s == WithheldFieldPlaceholder {
		return
	}
	withheldMu.Lock()
	defer withheldMu.Unlock()
	if len(withheldValues) < maxWithheldValues {
		withheldValues[s] = true
	}
}

func carriesWithheldValue(text string) bool {
	withheldMu.Lock()
	defer withheldMu.Unlock()
	for v := range withheldValues {
		if strings.Contains(text, v) {
			return true
		}
	}
	return false
}

// keyValueRE finds the name and separator of name=value / name: value pairs in
// plain text, tolerating the escaped quotes of JSON quoted inside a string;
// the caller checks that a value follows, so the value's first character is
// never consumed (it may start the next name).
var keyValueRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_.\-]*)(?:\\*["'])?\s*[:=]`)

// headerSplitRE splits a CSV, TSV, pipe or column-aligned table header line.
var headerSplitRE = regexp.MustCompile(`\s*[,\t|]\s*|\s{2,}`)

// headerCellRE is one bare identifier, the only shape a header cell takes.
var headerCellRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)

// valueCarriesNothing reports whether the text after a separator is not a
// credential value: nothing (a cut right after the separator), a JSON
// literal true/false/null, or the placeholder (or a prefix of it, where a
// preview cut through it).
func valueCarriesNothing(rest string) bool {
	if rest == "" || strings.ContainsAny(rest[:1], "\r\n,;}]") {
		return true
	}
	for _, lit := range []string{"true", "false", "null"} {
		if strings.HasPrefix(rest, lit) && (len(rest) == len(lit) || !isWordByte(rest[len(lit)])) {
			return true
		}
	}
	// The value ends at a quote, a backslash or the line end (a preview
	// string's own closing quote can follow a placeholder cut short).
	val := rest
	if i := strings.IndexAny(val, "\"'\\\r\n"); i >= 0 {
		val = val[:i]
	}
	if val == "" {
		return true
	}
	// A placeholder, or a placeholder cut short (at least "[w", so a JSON
	// array value such as ["S"] never reads as one).
	return (len(val) >= 2 && strings.HasPrefix(WithheldFieldPlaceholder, val)) || strings.HasPrefix(val, WithheldFieldPlaceholder)
}

func isWordByte(c byte) bool {
	return c == '_' || c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// keyValueNamesCredential reports a key=value / key: value pair (including a
// JSON fragment that no longer decodes) whose key names a credential and
// whose value carries something. This CLI's own environment variable names
// (DNSFILTER_API_KEY in the auth hint `export DNSFILTER_API_KEY="your-token-here"`)
// are names, not data fields, and are skipped so an auth error stays readable.
func keyValueNamesCredential(text string) bool {
	for _, m := range keyValueRE.FindAllStringSubmatchIndex(text, -1) {
		name := text[m[2]:m[3]]
		if strings.HasPrefix(name, "DNSFILTER_") || !IsCredentialKey(name) {
			continue
		}
		if !valueCarriesNothing(strings.TrimLeft(text[m[1]:], " \t\"'\\")) {
			return true
		}
	}
	return false
}

// headerNamesCredential reports a delimited header line (CSV, TSV, a pipe or
// column-aligned table) with a credential-named column. A header needs a
// table under it: like the store's textNamesSecret, a one-line string
// ("Finance, password reset desk", or a prose list of field names) is not a
// CSV header.
func headerNamesCredential(plain string) bool {
	lines := strings.Split(strings.TrimSpace(plain), "\n")
	if len(lines) < 2 {
		return false
	}
	for _, line := range lines {
		fields := headerSplitRE.Split(strings.TrimSpace(line), -1)
		if len(fields) < 2 {
			continue
		}
		for _, f := range fields {
			f = strings.Trim(f, `"' `)
			// A header cell is one identifier; prose such as "invalid token"
			// between commas, or a "key":"value" fragment, is not a column
			// name (the key=value scan owns fragments).
			if len(f) > 64 || !headerCellRE.MatchString(f) || strings.HasPrefix(f, "DNSFILTER_") {
				continue
			}
			if IsCredentialKey(f) {
				return true
			}
		}
	}
	return false
}
