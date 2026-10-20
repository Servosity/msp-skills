// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-withholds-credentials" and
// "mcp-read-only-redacts-before-bounding" entries.

// Package redact keeps credentials out of MCP tool results while the
// DNSFILTER_MCP_READ_ONLY switch is on, and out of every result and error of
// a tool advertised readOnlyHint=true in any mode. It has two entry points:
//
//   - Raw runs on a full, unbounded body BEFORE internal/mcp/bound budgets it
//     (bound calls it from EndpointResponse, EndpointPageResponse, JSON and
//     Text), so a truncated preview can only ever quote already-redacted text.
//   - Final runs on every result text in the MCP result funnel, after
//     bounding, and fails closed: it also withholds what it cannot decode.
//
// RawAlways and FinalAlways apply the same rules without the switch check;
// the funnel uses them for a read-only-hinted tool (WithForced, Forced).
//
// Matching is by key name on decoded JSON (store.IsSecretKey plus a trailing
// "pin"), never by guessing at values.
package redact

import (
	"bytes"
	"context"
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
// local mirror and the MCP funnel (store.KeyNamesSecret: secret, password,
// apikey, credential, a trailing token(s) or pin(s), case and separator
// insensitive, a trailing id/uuid word excluded; or a key that reads as a
// qualified secret label).
func IsCredentialKey(key string) bool { return store.KeyNamesSecret(key) }

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
	if !Enabled() {
		return text
	}
	return RawAlways(text)
}

// RawAlways is Raw without the switch check, for a tool advertised
// readOnlyHint=true (see Forced).
func RawAlways(text string) string {
	if text == "" {
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
	if !Enabled() {
		return text
	}
	return final(text, WithheldBlockMessage)
}

// FinalAlways is Final without the switch check, for a tool advertised
// readOnlyHint=true (see Forced); a withheld block reads
// WithheldHintedBlockMessage, which does not claim the switch is set.
func FinalAlways(text string) string {
	return final(text, WithheldHintedBlockMessage)
}

func final(text, block string) string {
	if text == "" {
		return text
	}
	out, plain := redactJSONSegments(text)
	// A base64 binary envelope is opaque to every rule here, also when a
	// bounded preview cut it so it no longer decodes (handfixes.json:
	// binary-envelope-never-hides-text).
	if textNamesCredential(out, plain) || carriesWithheldValue(out) || strings.Contains(out, "_pp_binary") {
		return block
	}
	return out
}

// WithheldHintedBlockMessage replaces a whole text block, as
// WithheldBlockMessage does, for a tool advertised readOnlyHint=true while
// the read-only switch is off.
const WithheldHintedBlockMessage = "withheld: this result carries a credential-bearing field and this tool is advertised read-only, so it never returns credentials (API key tokens, network secret keys, uninstall PINs, client secrets) in any mode; credential reads go through dnsfilter_execute, which is not advertised read-only"

// IsWithheldBlock reports whether text is a whole-block withheld message
// from Final or FinalAlways.
func IsWithheldBlock(text string) bool {
	return text == WithheldBlockMessage || text == WithheldHintedBlockMessage
}

// forcedKey marks a context whose tool call serves a tool advertised
// readOnlyHint=true.
type forcedKey struct{}

// WithForced marks ctx for one call of a tool advertised readOnlyHint=true:
// MCP hosts auto-approve such a tool, so its results and errors are filtered
// with RawAlways/FinalAlways (the read-only switch's rules) whatever the
// switch says. Hand-wired (handfixes.json:
// mcp-read-only-hinted-tools-filter-credentials): the MCP result funnel marks
// the call; the shell-out layer reads the mark to redact captured companion
// CLI output before it selects, tails or cuts lines from it.
func WithForced(ctx context.Context) context.Context {
	return context.WithValue(ctx, forcedKey{}, true)
}

// Forced reports whether ctx was marked by WithForced.
func Forced(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(forcedKey{}).(bool)
	return v
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
		// A decoded segment leaves its opening bracket in the plain
		// remainder, so a qualifier written as JSON (Uninstall PIN ["HQ"]
		// laptops: x) still arms the plain rule there (handfixes.json:
		// secrets-never-persisted).
		plain.WriteByte(text[start])
		plain.WriteByte(' ')
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
		// Hand-wired (handfixes.json: binary-envelope-never-hides-text): a
		// base64 binary envelope is opaque to every rule here, so read-only
		// MCP withholds it whole.
		if b, _ := v["_pp_binary"].(bool); b {
			return WithheldFieldPlaceholder, true
		}
		changed := false
		for key, child := range v {
			if store.KeyCarriesSecret(key) { // the key text is the leak
				delete(v, key)
				changed = true
				continue
			}
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
				// A key carrying a qualified label ({"Uninstall PIN (HQ)":"x"})
				// is a credential key (store.KeyNamesSecret).
				if cleaned, changed := redactValue(inner); changed || !rawJSONClean(trimmed) {
					return encodeJSONCompact(cleaned), true
				}
				return v, false
			}
		}
		// Not exactly one JSON value: the store's order (scrubSecretString).
		// The text rule runs over the WHOLE raw string first, at every escape
		// level (a quoted value's boundary is only right at the level it was
		// written in: token="\",SECRET" carries a value there and looks empty
		// once unescaped); then the complete records inside it are redacted
		// and the rule runs over what is not JSON.
		// Every string gets the whole-string plain rule, a bounded preview
		// included: it is text a tenant can shape (it starts with { or [ too),
		// and the JSON-aware rule is never run on such text (one stray quote
		// would invert it). A preview holding an ordinary value that reads
		// like a qualifier loses its preview field, not the result.
		if store.TextNamesSecret(v) {
			return WithheldFieldPlaceholder, true
		}
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
			if store.KeyCarriesSecret(k) {
				return false
			}
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

// textNamesCredential is the text rule for a result text, built only from the
// store's exported rule: store.TextNamesSecret over the non-JSON remainder (a
// secret pair at every escape level, raw included, or a CSV/table header once
// escaping is undone), plus one MCP-only widening: the JSON-aware secret-pair
// check over the WHOLE (redacted) text at its raw level, so a pair whose value
// is itself JSON (token=["S"]) is caught, and the label-only check once fully
// unescaped (a key name hidden by escaping; unescaping destroys the quote
// structure a qualifier check needs). The plain
// remainder keeps the plain-text rule, so a stray quote there cannot invert
// the JSON pass into missing it. The header part is not
// run over the whole text: a pretty-printed JSON result is not a table. A
// string VALUE gets the store's full rule over the whole string first (see
// redactValue).
func textNamesCredential(whole, plain string) bool {
	return store.TextNamesSecret(plain) || store.SecretPairInJSONText(whole) || store.SecretLabelPairInText(store.UnescapeForTextRule(whole))
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
