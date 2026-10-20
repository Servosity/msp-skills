// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: secrets-never-persisted). A regen preserves
// this file by construction; the one hook into generated store.go is the
// ScrubPersistedSecrets call in mergeIncomingResourceData.

package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

// withheldRow is what reaches the store when a row cannot be decoded, so its
// keys cannot be checked: an empty object, never the raw bytes.
var withheldRow = json.RawMessage(`{}`)

// ScrubPersistedSecrets returns data with every secret-named key removed, at
// any depth of nested objects and arrays, before a row reaches SQLite (the
// generic resources blob, its FTS index, and every typed table, which all
// write the value mergeIncomingResourceData returns).
//
// DNSFilter GET responses carry live credentials in synced resources:
// Network.secret_key (every networks list and show) and ApiKey.token (the
// api-keys list, and the full token in a create response that is written
// through). The CLI command that fetched them still prints them; the local
// mirror never keeps them.
//
// Matching is by key name on the decoded JSON (see IsSecretKey), including
// JSON encoded inside string values, never by string search on the raw row;
// a string value that still names a secret key as text is withheld (see
// scrubSecretString). Data that is not a single decodable JSON value is
// withheld whole (fail closed).
//
// The original bytes are kept only when rawJSONNeedsNoScrub proves, by a
// token walk over the RAW input (every key occurrence, duplicates included),
// that they carry nothing to scrub. Any other row is re-encoded from the
// scrubbed decoded value, never copied: decoding keeps only the last of
// duplicate keys, so a decision made on the decoded value alone would let
// {"token":"SECRET","token":null} through byte-for-byte.
func ScrubPersistedSecrets(data json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(data)) == 0 {
		return data
	}
	if rawJSONNeedsNoScrub(data, 0) {
		return data
	}
	value, ok := decodeSingleJSON(data)
	if !ok {
		return withheldRow
	}
	cleaned, _ := scrubSecretValue(value)
	enc, ok := encodeCompactJSON(cleaned)
	if !ok {
		return withheldRow
	}
	return json.RawMessage(enc)
}

// decodeSingleJSON decodes raw as exactly one JSON value (numbers kept as
// json.Number) with nothing but whitespace after it.
func decodeSingleJSON(raw []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return value, true
}

// rawJSONNeedsNoScrub walks the tokens of raw and reports true only when raw
// is exactly one JSON value with nothing but whitespace after it, no object
// repeats a key, no secret-named key carries anything but null or a boolean,
// and every string value is one scrubSecretString leaves unchanged (at depth,
// the JSON-in-string level raw sits at). Any doubt reports false, which
// sends the caller to the re-encode path.
func rawJSONNeedsNoScrub(raw []byte, depth int) bool {
	type frame struct {
		object    bool
		expectKey bool
		keys      map[string]bool
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var stack []*frame
	secretValue := false // the next value belongs to a secret-named key
	values := 0          // complete top-level values
	finish := func() {
		secretValue = false
		if len(stack) == 0 {
			values++
			return
		}
		if top := stack[len(stack)-1]; top.object {
			top.expectKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return values == 1 && len(stack) == 0
		}
		if err != nil || (len(stack) == 0 && values > 0) {
			return false
		}
		if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].expectKey {
			if d, ok := tok.(json.Delim); ok && d == '}' {
				stack = stack[:n-1]
				finish()
				continue
			}
			key, ok := tok.(string)
			if !ok || stack[n-1].keys[key] {
				return false
			}
			stack[n-1].keys[key] = true
			stack[n-1].expectKey = false
			secretValue = IsSecretKey(key)
			continue
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{', '[':
				if secretValue {
					return false
				}
				stack = append(stack, &frame{object: t == '{', expectKey: t == '{', keys: map[string]bool{}})
			default:
				if len(stack) == 0 {
					return false
				}
				stack = stack[:len(stack)-1]
				finish()
			}
		case string:
			if secretValue {
				return false
			}
			if _, changed := scrubSecretString(t, depth); changed {
				return false
			}
			finish()
		case json.Number:
			if secretValue {
				return false
			}
			finish()
		default: // bool, nil
			finish()
		}
	}
}

// scrubSecretValue drops secret-named keys from every object reachable from
// value, including objects inside JSON that is encoded in a string value.
// A secret-named key whose value is a boolean or null is kept: it carries no
// secret and may be a flag the API reports.
func scrubSecretValue(value any) (any, bool) {
	return scrubSecretValueDepth(value, 0)
}

// maxNestedJSONDepth bounds how many times a string holding JSON is decoded
// inside another (JSON in a string in a string ...). Past it the string is
// only checked as text, which still withholds it if it names a credential.
const maxNestedJSONDepth = 8

// maxFailedSegmentDecodes bounds the embedded-JSON scan of one string value
// (a value full of stray brackets), so it cannot go quadratic.
const maxFailedSegmentDecodes = 256

// WithheldValuePlaceholder replaces a string value that names a credential
// in a shape the scrub cannot take apart (key=value text, a table, JSON cut
// mid-record). It is the read-only MCP redactor's placeholder text.
const WithheldValuePlaceholder = "[withheld: credential]"

func scrubSecretValueDepth(value any, depth int) (any, bool) {
	switch v := value.(type) {
	case map[string]any:
		removed := false
		for key, child := range v {
			if IsSecretKey(key) {
				switch child.(type) {
				case nil, bool:
				default:
					delete(v, key)
					removed = true
					continue
				}
			}
			cleaned, childRemoved := scrubSecretValueDepth(child, depth)
			if childRemoved {
				v[key] = cleaned
				removed = true
			}
		}
		return v, removed
	case []any:
		removed := false
		for i, child := range v {
			cleaned, childRemoved := scrubSecretValueDepth(child, depth)
			if childRemoved {
				v[i] = cleaned
				removed = true
			}
		}
		return v, removed
	case string:
		return scrubSecretString(v, depth)
	}
	return value, false
}

// scrubSecretString scrubs the JSON a string value carries and withholds the
// string whole when its remaining text still names a credential with a
// value:
//   - a string that is one JSON value (an object, an array, or a JSON string
//     holding more JSON) is decoded, scrubbed and re-encoded compactly;
//   - otherwise every decodable object or array embedded in the text is
//     scrubbed in place;
//   - text that then still names a secret key in a name=value / name: value
//     pair (JSON cut mid-record included), or in the header row of a
//     multi-line table, is replaced by WithheldValuePlaceholder.
//
// A string with nothing to scrub is returned unchanged (removed == false).
func scrubSecretString(s string, depth int) (any, bool) {
	if depth >= maxNestedJSONDepth {
		if textNamesSecret(s) {
			return WithheldValuePlaceholder, true
		}
		return s, false
	}
	trimmed := strings.TrimSpace(s)
	if trimmed != "" && strings.ContainsAny(trimmed[:1], "{[\"") {
		if inner, ok := decodeSingleJSON([]byte(trimmed)); ok {
			// Same rule as the top-level row: keep the string only when its
			// raw tokens prove there is nothing to scrub.
			if rawJSONNeedsNoScrub([]byte(trimmed), depth+1) {
				return s, false
			}
			cleaned, _ := scrubSecretValueDepth(inner, depth+1)
			if enc, ok := encodeCompactJSON(cleaned); ok {
				return enc, true
			}
			return WithheldValuePlaceholder, true
		}
	}
	// Not exactly one JSON value (text around or after an embedded value
	// included): the text rule runs over the WHOLE string first.
	if textNamesSecret(s) {
		return WithheldValuePlaceholder, true
	}
	text, plain, changed := scrubEmbeddedJSON(s, depth)
	if textNamesSecret(plain) {
		return WithheldValuePlaceholder, true
	}
	if changed {
		return text, true
	}
	return s, false
}

// scrubEmbeddedJSON scrubs every decodable JSON object or array inside text
// and returns the rewritten text, the non-JSON remainder (for the text
// check) and whether anything was scrubbed. After a failed decode the scan
// moves one byte on, so the complete records inside a truncated container
// are still found.
func scrubEmbeddedJSON(text string, depth int) (string, string, bool) {
	var out, plain strings.Builder
	changed := false
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
		if rawJSONNeedsNoScrub([]byte(text[start:end]), depth+1) {
			out.WriteString(text[start:end])
		} else {
			cleaned, _ := scrubSecretValueDepth(v, depth+1)
			enc, ok := encodeCompactJSON(cleaned)
			if !ok {
				enc = `"` + WithheldValuePlaceholder + `"`
			}
			out.WriteString(enc)
			changed = true
		}
		i = end
	}
	return out.String(), plain.String(), changed
}

func encodeCompactJSON(v any) (string, bool) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", false
	}
	return strings.TrimRight(buf.String(), "\n"), true
}

// secretPairRE finds the name and separator of name=value / name: value pairs
// in text, tolerating the escaped quotes of JSON quoted inside a string; the
// caller checks that a value follows.
var secretPairRE = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_.\-]*)(?:\\*["'])?\s*[:=]`)

// tableSplitRE splits a CSV, TSV, pipe or column-aligned table row.
var tableSplitRE = regexp.MustCompile(`\s*[,\t|]\s*|\s{2,}`)

// tableCellRE is one bare identifier, the only shape a header cell takes.
var tableCellRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)

// textNamesSecret is the store's text rule on a raw string value; see
// TextNamesSecret.
func textNamesSecret(text string) bool {
	return TextNamesSecret(text)
}

// TextNamesSecret reports whether text names a secret key in a shape that
// can carry its value: a name=value / name: value pair whose value is not
// exactly empty, true, false, null or the placeholder (SecretPairInText), or
// a header row of a table that has at least one more line (a single line
// such as "Top Secret, Finance" is a name, not a table). text is RAW: JSON
// string escaping is undone first (UnescapeForTextRule), so a key spelled
// tok\u0065n in a cut-off JSON blob is still read as token. There is no
// exception for this CLI's own environment variable names.
func TextNamesSecret(text string) bool {
	text = UnescapeForTextRule(text)
	if SecretPairInText(text) {
		return true
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 2 {
		return false
	}
	for _, line := range lines {
		cells := tableSplitRE.Split(strings.TrimSpace(line), -1)
		if len(cells) < 2 {
			continue
		}
		for _, c := range cells {
			c = strings.Trim(c, `"' `)
			if len(c) <= 64 && tableCellRE.MatchString(c) && IsSecretKey(c) {
				return true
			}
		}
	}
	return false
}

// SecretPairInText reports whether text holds a secret-named key followed by
// a separator (= or :, quotes allowed around the name) and a value. The value
// is what follows the separator once spaces, newlines, quotes and
// backslashes are skipped, up to the next delimiter (see pairValueEmpty).
// text must ALREADY be unescaped (UnescapeForTextRule); TextNamesSecret does
// that for raw text.
func SecretPairInText(text string) bool {
	if !strings.ContainsAny(text, ":=") {
		return false
	}
	for _, m := range secretPairRE.FindAllStringSubmatchIndex(text, -1) {
		if !IsSecretKey(text[m[2]:m[3]]) {
			continue
		}
		if !pairValueEmpty(strings.TrimLeft(text[m[1]:], " \t\r\n\"'\\")) {
			return true
		}
	}
	return false
}

// maxUnescapeRounds bounds how many levels of JSON string escaping the text
// rule sees through (JSON quoted in a string quoted in a string ...).
const maxUnescapeRounds = 6

// UnescapeForTextRule undoes JSON string escaping, one level per round, so a
// text rule reads a key name, a quote, a separator or a newline however
// deeply it was escaped (\u0065, \", \n). Malformed escapes stay as they
// are. It is the read-only MCP redactor's rule, shared so the store scrub is
// never narrower than what MCP output withholds.
func UnescapeForTextRule(s string) string {
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
				if i+5 < len(s) && isHexByte(s[i+2]) && isHexByte(s[i+3]) && isHexByte(s[i+4]) && isHexByte(s[i+5]) {
					var r rune
					for _, h := range []byte(s[i+2 : i+6]) {
						r = r*16 + rune(hexByteVal(h))
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

func isHexByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexByteVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

// pairValueEmpty reports whether the text after a separator (leading
// spaces, newlines, quotes and backslashes already skipped) carries no value.
// Only an EXACT match is exempt, never a prefix: nothing at all, a delimiter
// that ends an empty value (, ; & } ] )), or a value token that is exactly
// true, false, null or the placeholder, ending at the end of the text or a
// delimiter (whitespace , ; & " ' } ] )). "true!secret", "null-secret" and
// "[withheld: credential]secret" all carry a value.
func pairValueEmpty(rest string) bool {
	if rest == "" || strings.ContainsAny(rest[:1], ",;&}])") {
		return true
	}
	for _, lit := range []string{"true", "false", "null", WithheldValuePlaceholder} {
		if strings.HasPrefix(rest, lit) && (len(rest) == len(lit) || isValueDelimiter(rest[len(lit)])) {
			return true
		}
	}
	return false
}

// isValueDelimiter reports whether c ends a value token in key=value text.
func isValueDelimiter(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', ',', ';', '&', '"', '\'', '}', ']', ')':
		return true
	}
	return false
}

// secretKeyFragments match anywhere in a key once it is folded to lower-case
// letters and digits, so secret_key, client_secret, ep-grd-subscriber-secret,
// new_password, reset_password_url, private_key and X-Api-Key all match.
var secretKeyFragments = []string{
	"secret",
	"password",
	"passwd",
	"passphrase",
	"privatekey",
	"apikey",
	"sharedkey",
	"credential",
}

// secretKeySuffixes match at the end of a folded key: token, access_token,
// payment_token, refresh_token, pin, uninstall_pin. token_type, last_four,
// pin_code_enabled and counters such as total_tokens do not match.
var secretKeySuffixes = []string{
	"token",
	"pin",
}

// identifierWords end a key that names a reference to a secret-bearing
// object rather than the secret itself: api_keys_id (the parent key every
// revoke row carries), secretKeyId, token_uuid.
var identifierWords = map[string]bool{"id": true, "ids": true, "uuid": true, "uuids": true}

// IsSecretKey reports whether a JSON object key names a secret. Case, '_',
// '-' and any other non-alphanumeric separators are ignored; a key whose last
// word is id, ids, uuid or uuids is an identifier, not a secret.
func IsSecretKey(key string) bool {
	words := keyWords(key)
	if len(words) == 0 || identifierWords[words[len(words)-1]] {
		return false
	}
	folded := strings.Join(words, "")
	for _, f := range secretKeyFragments {
		if strings.Contains(folded, f) {
			return true
		}
	}
	for _, s := range secretKeySuffixes {
		if strings.HasSuffix(folded, s) {
			return true
		}
	}
	return false
}

// keyWords splits a key into lower-case words on every non-alphanumeric rune
// and on lower-to-upper camelCase boundaries: "apiKeyId" and "api_key_id"
// both give [api key id].
func keyWords(key string) []string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	var prev rune
	for _, r := range key {
		switch {
		case r >= 'A' && r <= 'Z':
			if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
				flush()
			}
			cur.WriteRune(r + ('a' - 'A'))
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			cur.WriteRune(r)
		default:
			flush()
		}
		prev = r
	}
	flush()
	return words
}
