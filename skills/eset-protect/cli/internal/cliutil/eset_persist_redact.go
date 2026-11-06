// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "credential-tier-never-persisted".
//
// governance.md puts installer download links, GPO/SCCM deployment files and
// mobile enrollment links in the Credential tier: each one installs or
// enrolls a device into the tenant. Live output keeps them (that is why an
// MSP runs `installers list`), but they must never reach disk. Every write
// of an ESET response to disk goes through one of two funnels, and both call
// into this file:
//
//   - the SQLite mirror: store.go runs every row through PersistForm (scoped
//     by the row's resource type) before the generic resources table, its
//     FTS index and the typed tables see it (sync, write-through from live
//     reads, mutation responses), and stores nothing when PersistForm
//     refuses the row;
//   - the HTTP response cache: client.go never writes a body for which
//     HoldsPersistCredential (scoped by the request path) is true, and drops
//     any such entry it finds.
//
// Redaction is scoped by schema (round 3, adversarial P2): a Credential-tier
// property is redacted only in the responses whose schema carries it, found
// from the store resource type or the request path (PersistCredentialKeysFor).
// Free-form objects elsewhere (v2TriggeringEvent.data on every v2 detection
// and detection group, v2FeaturePolicy.configuration) keep their uri and
// downloadUrl keys, which are forensic data there, not credentials.
//
// The rule is fail closed (round 3). A body reaches disk only when it is
// exactly one JSON value followed by end of input (the second Decode returns
// io.EOF) and every Credential-tier key occurrence in it, at any depth and
// including duplicate keys, has been redacted. A body that is not one JSON
// value is never persisted, whether or not it names a credential key:
// detecting a key name in arbitrary bytes can never be complete (truncation,
// concatenation, \u and %XX escapes, case folding, encodings nobody listed),
// and an ESET Connect body that does not parse is an error the caller already
// reports, so storing or caching it has no value. The same core
// (RedactJSONKeys, NamesKeyInAnyEncoding) backs the deviceToken redaction
// that client.do applies to every response (device-token-redacted-at-decode).
package cliutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// PersistRedactedValue replaces a Credential-tier value in what is persisted.
const PersistRedactedValue = "[redacted]"

// persistCredentialKeys are the ESET Connect response properties whose value
// is Credential tier, each in the one schema that carries it: downloadUrl
// (v1Installer, in the installer-management installers list/get/create
// responses), gpoSccmFile (the v1GenerateGpoSccmFileResponse content), uri
// (v1DeviceEnrollmentLink, the values of enrollmentLinks in the
// mobile-device-management batchGetEnrollmentLinks response) and deviceToken
// (v1Device; already redacted at decode in every response, listed so this
// funnel alone is sufficient). Matched with strings.EqualFold on the decoded
// key, at any depth of a body in scope.
var persistCredentialKeys = []string{"downloadUrl", "gpoSccmFile", "uri", "deviceToken"}

// ErrNotPersistable is the store's refusal of a row PersistForm refuses.
var ErrNotPersistable = errors.New("refusing to store a response that is not exactly one JSON value (it could carry a Credential-tier value no redaction can find)")

// PersistCredentialKeys returns a copy of every Credential-tier key.
func PersistCredentialKeys() []string {
	return append([]string(nil), persistCredentialKeys...)
}

// PersistCredentialKeysFor returns the Credential-tier keys whose schema can
// appear in a body persisted under scope: a store resource type ("installers",
// "gpo-sccm-files-generate", "mobile-devices-batch-get-enrollment-links",
// "detections_v2", ...) or a request path. deviceToken is in every scope
// (device objects are embedded in many responses); downloadUrl only where an
// installer can be, gpoSccmFile only for the GPO/SCCM file, uri only for
// enrollment links. An empty scope gets every key (fail closed).
func PersistCredentialKeysFor(scope string) []string {
	s := strings.ToLower(strings.TrimSpace(scope))
	if s == "" {
		return PersistCredentialKeys()
	}
	keys := []string{"deviceToken"}
	if strings.Contains(s, "installer") {
		keys = append(keys, "downloadUrl")
	}
	if strings.Contains(s, "gpo-sccm") || strings.Contains(s, "gpo_sccm") || strings.Contains(s, "gposccm") {
		keys = append(keys, "gpoSccmFile")
	}
	if strings.Contains(s, "enrollment") {
		keys = append(keys, "uri")
	}
	return keys
}

// PersistForm returns the bytes to write to disk for body under scope (see
// PersistCredentialKeysFor), and false when body must not be written at all
// (no cache entry, no store row): it is not exactly one JSON value, in any
// scope. A body that is one JSON value and holds no Credential-tier value of
// its scope comes back byte for byte; one that does is re-encoded from the
// parsed, redacted value (numbers kept exact, HTML not escaped).
func PersistForm(scope string, body []byte) ([]byte, bool) {
	return RedactJSONKeys(body, PersistCredentialKeysFor(scope), PersistRedactedValue)
}

// HoldsPersistCredential reports whether body may not be written to disk
// under scope as it is: it is not one JSON value, or it carries a
// Credential-tier value of its scope that PersistForm replaces.
func HoldsPersistCredential(scope string, body []byte) bool {
	out, ok := PersistForm(scope, body)
	return !ok || !bytes.Equal(out, body)
}

// RedactJSONKeys is the one redaction rule both funnels share. ok is false
// when body is not exactly one JSON value followed by end of input; the
// caller must then not use body as data (persistence refuses it, client.do
// withholds it when it names a credential key). With ok true, out is body
// byte for byte when no occurrence of a key in keys (strings.EqualFold on the
// decoded key, at any depth) holds a value other than null, "" or marker;
// otherwise out is the parsed value re-encoded with every such value replaced
// by marker. Duplicate keys cannot smuggle a value past it: the decision is
// made on every key occurrence in the token stream, and the re-encoding holds
// only what the parser kept, after redaction.
func RedactJSONKeys(body []byte, keys []string, marker string) ([]byte, bool) {
	if !mayHoldQuotedKey(body, keys) {
		// No key in keys can be spelled here (see mayHoldQuotedKey), so a
		// valid body has nothing to redact.
		if json.Valid(body) {
			return body, true
		}
		return nil, false
	}
	v, ok := decodeOneJSONValue(body)
	if !ok {
		return nil, false
	}
	isKey := keyMatcher(keys)
	needs, err := keyOccurrenceNeedsRedaction(body, isKey, marker)
	if err != nil {
		return nil, false
	}
	if !needs {
		return body, true
	}
	redactKeyedValues(v, isKey, marker)
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, false
	}
	return bytes.TrimRight(out.Bytes(), "\n"), true
}

// decodeOneJSONValue decodes body and reports whether it is exactly one JSON
// value: the first Decode succeeds and the second returns io.EOF (trailing
// whitespace is allowed, anything else is not).
func decodeOneJSONValue(body []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return v, true
}

func keyMatcher(keys []string) func(string) bool {
	return func(k string) bool {
		for _, c := range keys {
			if strings.EqualFold(k, c) {
				return true
			}
		}
		return false
	}
}

// Runes outside ASCII that strings.EqualFold maps onto an ASCII letter. A
// brute-force test over every rune pins that these two are the only ones.
var (
	kelvinSignUTF8 = []byte("K") // folds to k
	longSUTF8      = []byte("ſ") // folds to s
)

// mayHoldQuotedKey is the fast path, sound for a valid JSON body: a JSON key
// is a quoted string whose letters are written literally or as \u escapes
// (no other escape yields a letter), so a body with no \u escape, no rune
// that case-folds onto an ASCII letter, and no quoted key from keys in any
// ASCII case cannot hold one.
func mayHoldQuotedKey(body []byte, keys []string) bool {
	if bytes.Contains(body, []byte(`\u`)) || bytes.Contains(body, kelvinSignUTF8) || bytes.Contains(body, longSUTF8) {
		return true
	}
	lower := asciiLower(body)
	for _, k := range keys {
		if bytes.Contains(lower, []byte(`"`+strings.ToLower(k)+`"`)) {
			return true
		}
	}
	return false
}

func asciiLower(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}

// keyOccurrenceNeedsRedaction walks the token stream of a valid JSON body and
// reports whether any occurrence of a matching key (duplicates included)
// holds a value other than null, "" or marker. An object or array value
// always needs redaction.
func keyOccurrenceNeedsRedaction(body []byte, isKey func(string) bool, marker string) (bool, error) {
	type frame struct{ object, expectKey bool }
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var stack []frame
	credValue := false // the next value belongs to a matching key
	valueDone := func() {
		credValue = false
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].expectKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].expectKey {
			if d, ok := tok.(json.Delim); ok && d == '}' {
				stack = stack[:n-1]
				valueDone()
				continue
			}
			k, ok := tok.(string)
			if !ok {
				return false, errors.New("object key is not a string")
			}
			stack[n-1].expectKey = false
			credValue = isKey(k)
			continue
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{', '[':
				if credValue {
					return true, nil
				}
				stack = append(stack, frame{object: t == '{', expectKey: t == '{'})
			case ']', '}':
				if len(stack) == 0 {
					return false, errors.New("unbalanced JSON")
				}
				stack = stack[:len(stack)-1]
				valueDone()
			}
		default:
			if credValue && !trivialCredentialValue(t, marker) {
				return true, nil
			}
			valueDone()
		}
	}
}

// trivialCredentialValue is a value with nothing to hide.
func trivialCredentialValue(v any, marker string) bool {
	return v == nil || v == "" || v == marker
}

func redactKeyedValues(v any, isKey func(string) bool, marker string) {
	switch t := v.(type) {
	case map[string]any:
		for k, inner := range t {
			if isKey(k) {
				if !trivialCredentialValue(inner, marker) {
					t[k] = marker
				}
				continue
			}
			redactKeyedValues(inner, isKey, marker)
		}
	case []any:
		for _, inner := range t {
			redactKeyedValues(inner, isKey, marker)
		}
	}
}

// NamesKeyInAnyEncoding reports whether raw bytes that need not be JSON
// mention any of keys: plain, with JSON \uXXXX escapes, URL %XX escapes
// (nested escapes are decoded pass by pass), in any case (ASCII case plus the
// two non-ASCII runes strings.EqualFold maps onto an ASCII letter). The text
// is checked before the first pass and after every pass, so a spelling that a
// later pass would consume (a literal key right after "%6") still counts.
// The match is on the bare name, quoted or not, so it over-reports; callers
// use it to refuse, never to allow.
//
// The work is bounded (handfixes.json: escape-decoding-is-bounded): at most
// maxEscapeDecodePasses passes, each one linear in the body. A body whose
// escapes are still unwinding after that many passes is nested deeper than
// any real response and reports true, so client.do withholds it whole.
func NamesKeyInAnyEncoding(body []byte, keys []string) bool {
	names, undecodable := NamesKeyOrUndecodable(body, keys)
	return names || undecodable
}

// NamesKeyOrUndecodable is NamesKeyInAnyEncoding with the cause split out
// (handfixes.json: withheld-body-names-its-cause): names is true when a
// spelling of a key was found, undecodable when none was but the escapes
// were still unwinding at the pass budget. A caller that refuses on either
// can then say which.
func NamesKeyOrUndecodable(body []byte, keys []string) (names, undecodable bool) {
	lowered := make([][]byte, len(keys))
	for i, k := range keys {
		lowered[i] = []byte(strings.ToLower(k))
	}
	holds := func(text []byte) bool {
		folded := FoldKeyText(text)
		for _, k := range lowered {
			if bytes.Contains(folded, k) {
				return true
			}
		}
		return false
	}
	cur := body
	for pass := 0; ; pass++ {
		if holds(cur) {
			return true, false
		}
		if pass == maxEscapeDecodePasses {
			// Still unwinding: refuse rather than decode further.
			return false, !bytes.Equal(decodeEscapesOnce(cur), cur)
		}
		next := decodeEscapesOnce(cur)
		if bytes.Equal(next, cur) {
			return false, false
		}
		cur = next
	}
}

// maxEscapeDecodePasses bounds escape decoding. Every pass that changes the
// text shortens it by at least two bytes, so an unbounded loop ends, but a
// hostile body ("%" then "25" repeated) loses only two bytes per pass:
// quadratic work that ignores the request deadline. Real responses nest an
// escape two or three levels at most.
const maxEscapeDecodePasses = 16

// DecodeEscapesToFixedPoint decodes JSON \uXXXX escapes (a surrogate pair as
// one rune) and URL %XX escapes pass by pass until a pass changes nothing,
// for at most maxEscapeDecodePasses passes. ok is false when the text was
// still changing at the budget; out is then the last pass, not a fixed
// point, and a caller deciding what to refuse must treat the body as
// undecodable.
func DecodeEscapesToFixedPoint(b []byte) (out []byte, ok bool) {
	cur := b
	for pass := 0; pass < maxEscapeDecodePasses; pass++ {
		next := decodeEscapesOnce(cur)
		if bytes.Equal(next, cur) {
			return cur, true
		}
		cur = next
	}
	return cur, bytes.Equal(decodeEscapesOnce(cur), cur)
}

// hexVal returns the value (0-15) of one hex digit. It stays a byte so the
// callers build a byte (%XX) or a rune (\uXXXX) by widening only: no
// conversion can overflow.
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

func hex4(b []byte) (rune, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var r rune
	for _, c := range b[:4] {
		v, ok := hexVal(c)
		if !ok {
			return 0, false
		}
		r = r<<4 | rune(v)
	}
	return r, true
}

func decodeEscapesOnce(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		if b[i] == '\\' && i+1 < len(b) && b[i+1] == 'u' {
			if r, ok := hex4(b[i+2:]); ok {
				n := 6
				if 0xD800 <= r && r < 0xDC00 && i+12 <= len(b) && b[i+6] == '\\' && b[i+7] == 'u' {
					if lo, ok := hex4(b[i+8:]); ok && 0xDC00 <= lo && lo < 0xE000 {
						r = 0x10000 + (r-0xD800)<<10 + (lo - 0xDC00)
						n = 12
					}
				}
				if utf8.ValidRune(r) {
					out = utf8.AppendRune(out, r)
					i += n
					continue
				}
			}
		}
		if b[i] == '%' && i+2 < len(b) {
			if hi, ok := hexVal(b[i+1]); ok {
				if lo, ok := hexVal(b[i+2]); ok {
					out = append(out, hi<<4|lo)
					i += 3
					continue
				}
			}
		}
		out = append(out, b[i])
		i++
	}
	return out
}

// FoldKeyText lowers ASCII letters and maps U+212A (Kelvin sign) to k and
// U+017F (long s) to s, so a substring search for a lower-case ASCII key
// finds every spelling strings.EqualFold accepts. Other bytes are kept.
func FoldKeyText(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		switch {
		case r == 0x212A:
			out = append(out, 'k')
		case r == 0x017F:
			out = append(out, 's')
		case size == 1 && 'A' <= b[i] && b[i] <= 'Z':
			out = append(out, b[i]+'a'-'A')
		default:
			out = append(out, b[i:i+size]...)
		}
		i += size
	}
	return out
}
