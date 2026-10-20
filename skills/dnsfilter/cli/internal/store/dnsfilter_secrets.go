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
			if KeyCarriesSecret(key) {
				return false
			}
			secretValue = KeyNamesSecret(key)
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
// (a value full of stray brackets), so it cannot go quadratic. A value whose
// scan runs out of budget with a '{' or '[' still unexamined is withheld
// whole (fail closed): its remaining JSON keys were never judged.
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
			if KeyCarriesSecret(key) { // the key text is the leak: no value type keeps it
				delete(v, key)
				removed = true
				continue
			}
			if KeyNamesSecret(key) {
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
		// JSON nested deeper than the scrub decodes is withheld whole: its
		// keys were never judged.
		if trimmed := strings.TrimSpace(s); trimmed != "" && strings.ContainsAny(trimmed[:1], "{[\"") {
			if _, ok := decodeSingleJSON([]byte(trimmed)); ok {
				return WithheldValuePlaceholder, true
			}
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
// check) and whether anything was scrubbed. When the decode budget runs out
// with a '{' or '[' left unexamined, the rewritten text is the placeholder
// (changed, with no remainder): the value is withheld whole. After a failed
// decode the scan moves one byte on, so the complete records inside a
// truncated container are still found. The placeholder text is skipped,
// never decoded, so withheld values never spend the budget.
func scrubEmbeddedJSON(text string, depth int) (string, string, bool) {
	var out, plain strings.Builder
	changed := false
	failures := 0
	i := 0
	for i < len(text) {
		j := strings.IndexAny(text[i:], "{[")
		if j < 0 {
			out.WriteString(text[i:])
			plain.WriteString(text[i:])
			break
		}
		start := i + j
		out.WriteString(text[i:start])
		plain.WriteString(text[i:start])
		plain.WriteByte(' ')
		if strings.HasPrefix(text[start:], WithheldValuePlaceholder) {
			out.WriteString(WithheldValuePlaceholder)
			plain.WriteString(WithheldValuePlaceholder)
			i = start + len(WithheldValuePlaceholder)
			continue
		}
		if failures >= maxFailedSegmentDecodes {
			// HAND-FIX(secrets-never-persisted): the rest was never decoded,
			// so a JSON key in it ({"p a s s w o r d":"x"}) was never judged.
			return WithheldValuePlaceholder, "", true
		}
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

// labelPrepositions lead a qualifier between a secret label and its
// separator (Uninstall PIN for Acme laptops: 4821). "with" is left out so the
// CLI's own hint "Set your API key with: export ..." is judged by the pair
// inside it.
var labelPrepositions = map[string]bool{"for": true, "of": true, "on": true, "at": true, "in": true, "from": true, "to": true, "via": true, "by": true, "per": true}

// isLabelWordByte reports a byte that continues a label word.
func isLabelWordByte(c byte) bool {
	return c == '_' || c == '.' || c == '-' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

// tableSplitRE splits a CSV, TSV, pipe or column-aligned table row.
var tableSplitRE = regexp.MustCompile(`\s*[,;\t|]\s*|\s{2,}`)

// tableCellRE is one bare identifier (token, uninstall_pin, PIN2), a header
// cell that names its column on its own.
var tableCellRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)

// tableLabelRE is a header cell of label words (API Key, Network secret key,
// Roaming Client Uninstall PIN), optionally followed by one bracketed
// qualifier (Uninstall PIN (HQ), API Key [prod]). There is no length or word
// limit: a long header still heads its column.
var tableLabelRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*(?: [A-Za-z0-9_.\-]+)*(?: [(\[][^()\[\]]*[)\]])?$`)

// plistEntryRE is a property-list entry: <key>NAME</key> and the value
// element after it, read up to the next tag (or the end of a cut preview).
// DNSFilter's macOS roaming-client deployment writes the site secret key this
// way (<key>NKEY</key><string>...</string>), with no separator for the pair
// rule to see.
var plistEntryRE = regexp.MustCompile(`(?i)<key\s*>([^<]*)</key\s*>\s*<(?:string|integer|real|data|date)\s*>([^<]*)`)

// plistNamesSecret reports a property-list entry whose key names a secret
// (KeyNamesSecret) and whose value is not exempt (exemptPairValue).
func plistNamesSecret(text string) bool {
	if !strings.Contains(text, "<") {
		return false
	}
	for _, m := range plistEntryRE.FindAllStringSubmatch(text, -1) {
		if KeyNamesSecret(strings.TrimSpace(m[1])) && !exemptPairValue(strings.TrimSpace(m[2])) {
			return true
		}
	}
	return false
}

// markupElementRE matches an XML/HTML element's opening tag and the text
// right after it (<password>hunter2</password>, <secret_key a="b">X), and
// flagValueRE a command-line flag and the token after it (--password X,
// -nkey X): two more shapes, besides the plist entry, that carry a value with
// no separator for the pair rule to see.
var (
	markupElementRE = regexp.MustCompile(`<([A-Za-z_][\w.\-]*)(?:\s[^<>]*)?>\s*([^<\s][^<]*)`)
	flagValueRE     = regexp.MustCompile(`(?:^|\s)--?([A-Za-z][\w.\-]*)[ \t]+([^\s\-][^\s]*)`)
)

// markupNamesSecret reports an element or a flag whose name names a secret
// (KeyNamesSecret) followed by a value that is not exempt (exemptPairValue).
// An unclosed tag that is a documentation placeholder (<token>) is prose,
// not an element.
func markupNamesSecret(text string) bool {
	lower := strings.ToLower(text)
	for _, m := range markupElementRE.FindAllStringSubmatch(text, -1) {
		name := strings.ToLower(m[1])
		if documentationPlaceholders["<"+name+">"] && !strings.Contains(lower, "</"+name) {
			continue
		}
		if KeyNamesSecret(m[1]) && !exemptPairValue(strings.TrimSpace(m[2])) {
			return true
		}
	}
	for _, m := range flagValueRE.FindAllStringSubmatch(text, -1) {
		if KeyNamesSecret(m[1]) && !exemptPairValue(strings.TrimSpace(m[2])) {
			return true
		}
	}
	return false
}

// textNamesSecret is the store's text rule on a raw string value; see
// TextNamesSecret.
func textNamesSecret(text string) bool {
	return TextNamesSecret(text)
}

// TextNamesSecret reports whether text names a secret key in a shape that
// can carry its value: a name=value / name: value pair whose value is not
// exactly empty, true, false, null or the placeholder (SecretPairInText), a
// property-list entry (<key>NKEY</key><string>...</string>), or a header row
// of a table that has at least one more line (a single line such as "Top
// Secret, Finance" is a name, not a table). text is RAW: the pair and
// property-list rules run on it and on each level of JSON string escaping
// undone (up to maxUnescapeRounds), so a key spelled tok\u0065n in a cut-off
// JSON blob is still read as token; the table rule reads the fully unescaped
// text.
// There is no exception for this CLI's own environment variable names.
func TextNamesSecret(text string) bool {
	// The pair rule reads the text at every escape level, raw included: a
	// value's quote boundary is only right at the level it was written in,
	// and a key's name may only show once escaping is undone.
	for round := 0; ; round++ {
		if SecretPairInText(text) || plistNamesSecret(text) || markupNamesSecret(text) {
			return true
		}
		if !strings.Contains(text, "\\") {
			break
		}
		next := unescapeOneLevel(text)
		if next == text {
			break
		}
		if round >= maxUnescapeRounds {
			return true // escaping deeper than the rule reads: fail closed
		}
		text = next
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 2 {
		return false
	}
	// nextRow[i] is the first non-empty line after line i (-1: none), found
	// in one backward pass so the header check stays linear.
	nextRow := make([]int, len(lines))
	for i, next := len(lines)-1, -1; i >= 0; i-- {
		nextRow[i] = next
		if strings.TrimSpace(lines[i]) != "" {
			next = i
		}
	}
	for i, line := range lines {
		cells := tableSplitRE.Split(strings.TrimSpace(line), -1)
		if len(cells) < 2 {
			continue
		}
		nextIsRow := -1 // unknown until a spaced label needs it
		for _, c := range cells {
			c = strings.Trim(c, `"' `)
			if c == "" {
				continue
			}
			// HAND-FIX(secrets-never-persisted): no length guard runs before
			// the secret check (a 70-byte ..._secret_key header was skipped).
			// An identifier cell is judged as a JSON key is (token,
			// uninstall_pin, PIN2, NKEY).
			if tableCellRE.MatchString(c) && KeyNamesSecret(c) {
				return true
			}
			// A spaced label (API Key, Uninstall PIN (HQ), PIN2 Code) is
			// judged as a text label (the label must name the secret, so
			// Password Policy does not) and heads a column only in a table:
			// the next non-empty line is a delimited row too. Its cell count
			// is not compared, since a quoted comma in a CSV row adds cells.
			if tableLabelRE.MatchString(c) && textLabelNamesSecret(c) {
				if nextIsRow < 0 {
					nextIsRow = 0
					if j := nextRow[i]; j >= 0 && len(tableSplitRE.Split(strings.TrimSpace(lines[j]), -1)) >= 2 {
						nextIsRow = 1
					}
				}
				if nextIsRow == 1 {
					return true
				}
			}
		}
	}
	return false
}

// KeyNamesSecret reports whether a JSON object key names a secret: by the
// key rule (IsSecretKey), or read as a text label by the label rule, since a
// key can carry a qualified label ("Uninstall PIN (HQ)", "Admin password for
// HQ") that no key rule names. Every key the scrub or the read-only filter
// judges goes through here; the JSON structure around it is honored, so a
// string value that merely ends in a secret word never arms a later key.
// No DNSFilter spec property is newly matched by the label half.
func KeyNamesSecret(key string) bool {
	return IsSecretKey(key) || TextNamesSecret(key+": x")
}

// textLabelNamesSecret reports whether label, read as the text before a
// separator (label: x), names a secret by the label rule with its qualifiers
// and glued boundaries (API Key, Uninstall PIN (HQ), PIN2 Code); it is the
// label half of KeyNamesSecret, so Password Policy does not.
func textLabelNamesSecret(label string) bool { return TextNamesSecret(label + ": x") }

// KeyCarriesSecret reports whether a JSON object key's own text holds a
// labelled credential with a value ("Uninstall PIN: 4821", "token=X"): the
// key is then the leak, so the pair is dropped whatever its value (a true,
// false or null flag included).
func KeyCarriesSecret(key string) bool { return TextNamesSecret(key) }

// IdentityNamesSecret reports whether an identity value (a row id, a parent
// key, a fingerprint input) is one the scrub would withhold or rewrite. Ids
// are stored as given (resources.id, FTS, typed tables), so such a value is
// never used as one.
func IdentityNamesSecret(v string) bool {
	if v == "" {
		return false
	}
	_, changed := scrubSecretString(v, 0)
	return changed
}

// labelNamesSecret reports whether a label of one to three space-separated
// words names a secret: its last word does (token, Client Secret, Uninstall
// PIN), or its last word is "key"/"keys" and the last two or three words
// together do (API Key, Secret Key, Network secret key), whitespace counting
// like _ or -. "Password Policy", "public key" and "the key" do not.
func labelNamesSecret(label string) bool {
	words := strings.Fields(label)
	if len(words) == 0 {
		return false
	}
	last := words[len(words)-1]
	if IsSecretKey(last) {
		return true
	}
	if lw := keyWords(last); len(lw) == 0 || (lw[len(lw)-1] != "key" && lw[len(lw)-1] != "keys") {
		return false
	}
	for k := 2; k <= 3 && k <= len(words); k++ {
		if IsSecretKey(strings.Join(words[len(words)-k:], "_")) {
			return true
		}
	}
	return false
}

// gluedSecretLabel reports whether the label word w, read after the label
// words prev (the run so far; the last two count, as in labelNamesSecret),
// holds a secret name glued to a digit or punctuation inside the word: PIN2,
// PIN.2, "PIN." before a qualifier, uninstall_pin_2, PIN-, API-Key2. The
// scanner reads such a word whole, since a key name is one word
// (uninstall_pin, X-Api-Key, api.key), so without this the secret name never
// ends and the punctuation after it never arms. A glued boundary is a letter
// followed by a digit, or by '.', '_' or '-' that is not followed by a letter;
// those between letters join a compound name that the word rule judges whole
// (token_type, pin_code_enabled, secret_key_id, api-keys.index). A word whose
// last word is an identifier (token.id, pin2_id) is a reference, never a
// secret. Linear in len(w): each boundary is judged from running state (the
// folded prefix, whether it holds a secret fragment, where its last word
// starts), never by re-reading the prefix.
func gluedSecretLabel(prev []string, w string) bool {
	if lw := keyWords(w); len(lw) == 0 || identifierWords[lw[len(lw)-1]] {
		return false
	}
	if len(prev) > 2 {
		prev = prev[len(prev)-2:]
	}
	// For the key/keys rule (API Key2, Network secret key.v2): prev's last k
	// words, folded (only the tail a fragment can span into w), whether they
	// name a secret on their own, and their last key word.
	type joinedPrev struct {
		tail   string
		secret bool
		lastKW string
	}
	var jp [3]joinedPrev
	for k := 1; k <= len(prev); k++ {
		words := keyWords(strings.Join(prev[len(prev)-k:], "_"))
		f := strings.Join(words, "")
		if len(f) > maxSecretFragmentLen {
			f = f[len(f)-maxSecretFragmentLen:]
		}
		jp[k] = joinedPrev{tail: f, secret: containsSecretFragment(strings.Join(words, "")) || hasSecretWord(words)}
		if len(words) > 0 {
			jp[k].lastKW = shortKeyWord([]byte(words[len(words)-1]))
		}
	}
	folded := make([]byte, 0, len(w))
	fragSeen, wordSeen := false, false // folded holds a secret fragment / a secret word
	kwStart := 0                       // where the current key word of folded starts
	prevKW, firstKW := "", ""          // the key word before the current one; w's first
	endKW := func() {
		if kwStart < len(folded) {
			kw := shortKeyWord(folded[kwStart:])
			wordSeen = wordSeen || secretWordPair(prevKW, kw)
			if firstKW == "" {
				firstKW = kw
			}
			prevKW = kw
		}
		kwStart = len(folded)
	}
	for p := 0; p < len(w); p++ {
		c := w[p]
		if p > 0 && isASCIILetter(w[p-1]) && gluedBoundary(w, p) {
			// IsSecretKey and labelNamesSecret on w[:p], from running state.
			if lastKW := shortKeyWord(folded[kwStart:]); !identifierWords[lastKW] {
				if fragSeen || wordSeen || secretWordPair(prevKW, lastKW) || hasSecretSuffix(folded) {
					return true
				}
				if lastKW == "key" || lastKW == "keys" {
					first := firstKW
					if first == "" {
						first = lastKW
					}
					head := folded
					if len(head) > maxSecretFragmentLen {
						head = head[:maxSecretFragmentLen]
					}
					for k := 1; k <= len(prev); k++ {
						if jp[k].secret || secretWordPair(jp[k].lastKW, first) || containsSecretFragment(jp[k].tail+string(head)) {
							return true
						}
					}
				}
			}
		}
		switch {
		case c >= 'A' && c <= 'Z':
			if p > 0 && (w[p-1] >= 'a' && w[p-1] <= 'z' || w[p-1] >= '0' && w[p-1] <= '9') {
				endKW() // a camelCase boundary, as in keyWords
			}
			folded = append(folded, c+('a'-'A'))
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			folded = append(folded, c)
		default:
			endKW()
			continue
		}
		fragSeen = fragSeen || endsWithSecretFragment(folded)
	}
	return false
}

// shortKeyWord is a key word as compared against the special word lists
// (identifierWords, secretKeyWords, key/keys), all at most five letters; a
// longer word matches none of them and is returned as "-", so the comparison
// stays constant time however long the word is.
func shortKeyWord(kw []byte) string {
	if len(kw) > 5 {
		return "-"
	}
	return string(kw)
}

// secretInstructionRun reports a label run that is a secret label followed by
// "with" (Set your API key with:), the CLI's own instruction shape; "with" is
// never a qualifier (labelPrepositions).
func secretInstructionRun(run []string) bool {
	n := len(run)
	return n >= 2 && strings.EqualFold(run[n-1], "with") && labelNamesSecret(strings.Join(run[:n-1], " "))
}

// gluedBoundary reports whether w[p] ends the secret name before it: a digit,
// or a '.', '_' or '-' that no letter follows ("PIN." then a space, PIN.2,
// PIN_2, PIN-). Between letters those join a compound name (api-keys.index,
// token_type, pin-code-enabled), which the word rule judges whole.
func gluedBoundary(w string, p int) bool {
	switch c := w[p]; {
	case c >= '0' && c <= '9':
		return true
	case c == '.', c == '_', c == '-':
		return p+1 == len(w) || !isASCIILetter(w[p+1])
	}
	return false
}

func isASCIILetter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }

// maxSecretFragmentLen is the length of the longest secretKeyFragments entry.
const maxSecretFragmentLen = 10

func containsSecretFragment(folded string) bool {
	for _, f := range secretKeyFragments {
		if strings.Contains(folded, f) {
			return true
		}
	}
	return false
}

func endsWithSecretFragment(folded []byte) bool {
	for _, f := range secretKeyFragments {
		if bytes.HasSuffix(folded, []byte(f)) {
			return true
		}
	}
	return false
}

func hasSecretSuffix(folded []byte) bool {
	for _, s := range secretKeySuffixes {
		if bytes.HasSuffix(folded, []byte(s)) {
			return true
		}
	}
	return false
}

// SecretPairInText reports whether text holds a secret label followed by a
// separator (= or :) and a value that is not exempt (pairCarriesValue). One
// linear pass, no window: a separator is labelled when the one to three
// space-separated words right before it (closing quotes, backslashes and
// whitespace between them allowed) name a secret (labelNamesSecret), or when
// earlier in the same segment a secret label was followed by a qualifier,
// however long: punctuation or a digit (PIN (Acme), password #2, PIN - HQ,
// PIN, HQ, PIN [Acme], password#2), a preposition (PIN for Acme laptops), or
// an opening quote after a space (PIN "Acme" (laptops)). A segment ends at a
// real line break only: this is the rule for text anyone can write (a stored
// value, a string in a result), where quotes are prose (an inch mark 19" or a
// quoted name), so nothing else ends a qualifier. An escaped line break (\n)
// is text at its level (UNC paths such as \\dc01\netlogon); the unescape
// rounds supply the real breaks. Punctuation or a digit glued inside the word
// counts the same (PIN2, PIN.2, "PIN." then HQ; see gluedSecretLabel). A
// secret word followed by another word is a different label (Password Policy:
// strict is kept), and so is a compound name (token_type, api-keys.index). It
// reads text at one escape level, as given, and never unescapes;
// TextNamesSecret runs it on every level of raw text.
func SecretPairInText(text string) bool { return secretPairScan(text, scanPlain) }

// SecretPairInJSONText is SecretPairInText for text that is JSON as a whole
// (the read-only MCP filter's pass over a result it decoded and re-encoded):
// quotes are tracked as JSON strings at every nesting level (a quote after
// 2^d-1 backslashes delimits a string d levels down, JSON in a string in a
// string), so a qualifier armed inside a string ends where that string
// closes: an org named "Office of the Secretary of State" arms nothing past
// its own value, in a result or in a bounded preview of one. Never use it on
// text anyone can write (one stray quote would invert the phase); the plain
// rule still runs on every string value and on the non-JSON remainder.
func SecretPairInJSONText(text string) bool { return secretPairScan(text, scanJSON) }

// SecretLabelPairInText is the label-only form: a separator counts only when
// a secret label is right before it (no qualifier arming). It is for text
// whose quote structure was destroyed by undoing escapes, where only a key
// name hidden by escaping (tok\u0065n=) is new.
func SecretLabelPairInText(text string) bool { return secretPairScan(text, scanLabelOnly) }

// Scan modes for secretPairScan.
const (
	scanPlain = iota
	scanJSON
	scanLabelOnly
)

// maxJSONDepth bounds the JSON-in-string nesting levels the JSON scan tracks.
const maxJSONDepth = 8

// quoteDepth is the nesting level a quote after k backslashes delimits
// (0, 1, 3, 7 ... backslashes: level 0, 1, 2, 3), or -1 for a literal quote.
func quoteDepth(k int) int {
	for d := 0; d < maxJSONDepth; d++ {
		if k == 1<<d-1 {
			return d
		}
	}
	return -1
}

func secretPairScan(text string, mode int) bool {
	if !strings.ContainsAny(text, ":=") {
		return false
	}
	var run []string // the last one to three words of the current run
	// runSecret: the run names a secret. pending: it does and nothing has
	// followed yet. armed: a secret label then a qualifier in this segment.
	// clean: only whitespace, quotes and backslashes since the last word.
	// joined: the next word continues the run (only spaces since a word).
	runSecret, pending, armed, clean, joined := false, false, false, false, false
	// open[d]: a JSON string d levels down is open (JSON mode only);
	// armedDepth: the deepest open string when the latest qualifier armed
	// (-1: none).
	var open [maxJSONDepth]bool
	armedDepth := -1
	// armedIn: bit d+1 is set while an arming made with depth d deepest open
	// is in force. A later arming never replaces an earlier one: each ends
	// only where its own string closes, so re-arming cannot narrow the scope.
	var armedIn uint16
	// armedByGlue: the segment is armed only by a glued boundary (PIN2,
	// "PIN." then a space), which a later secret instruction supersedes.
	armedByGlue := false
	armAt := func(glued bool) {
		if !pending || mode == scanLabelOnly {
			return
		}
		armedByGlue = glued && (!armed || armedByGlue)
		armed, armedDepth = true, -1
		for d := maxJSONDepth - 1; d >= 0; d-- {
			if open[d] {
				armedDepth = d
				break
			}
		}
		armedIn |= 1 << uint(armedDepth+1)
	}
	arm := func() { armAt(false) }
	for i := 0; i < len(text); {
		c := text[i]
		switch {
		case c == '[' && strings.HasPrefix(text[i:], WithheldValuePlaceholder):
			i += len(WithheldValuePlaceholder)
			runSecret, pending, clean, joined = false, false, false, false
			continue
		case c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z':
			j := i + 1
			for j < len(text) && isLabelWordByte(text[j]) {
				j++
			}
			w := text[i:j]
			if labelPrepositions[strings.ToLower(w)] {
				arm()
			}
			if !joined {
				run = run[:0]
			}
			// HAND-FIX(secrets-never-persisted): a secret name glued to a
			// digit or punctuation inside the word (PIN2, PIN.2, "PIN." then
			// a qualifier) names the secret and arms the segment, as the same
			// punctuation would after a space.
			glued := gluedSecretLabel(run, w)
			if run = append(run, w); len(run) > 3 {
				run = append(run[:0], run[1:]...)
			}
			runSecret = glued || labelNamesSecret(strings.Join(run, " "))
			pending, clean, joined = runSecret, true, true
			if glued {
				armAt(true)
			}
			i = j
			continue
		case c == ' ' || c == '\t':
		case c == '\n' || c == '\r':
			armed, pending, joined, armedIn = false, false, false, 0
		case c == '\\' || c == '"':
			k := 0
			for i+k < len(text) && text[i+k] == '\\' {
				k++
			}
			if i+k < len(text) && text[i+k] == '"' {
				// A quote after a secret label and a space opens a quoted
				// qualifier (PIN "Acme" (laptops): x); a key's quote sits
				// right against its label ("password":), so it never arms.
				// In JSON text the quote also opens or closes a string at
				// its nesting level, and closing the string a qualifier was
				// armed in ends it.
				spaced := i > 0 && (text[i-1] == ' ' || text[i-1] == '\t')
				if d := quoteDepth(k); mode != scanPlain && d >= 0 {
					if open[d] {
						armedIn &^= 1 << uint(d+1)
						if armed && armedDepth == d {
							armed = armedIn != 0 // an earlier arming may still be in force
						}
						for e := d; e < maxJSONDepth; e++ {
							open[e] = false
						}
					} else {
						if spaced {
							arm()
						}
						open[d] = true
					}
				} else if spaced {
					arm()
				}
				// In plain text a quote that closes a quoted label ("Uninstall
				// PIN" (HQ): x) keeps it pending for a qualifier after it; in
				// JSON text the quote ends a string value, which arms nothing.
				keep := mode == scanPlain && i > 0 && isLabelWordByte(text[i-1])
				pending, joined = pending && keep, false
				i += k + 1
				continue
			}
			if k%2 == 1 && i+k < len(text) && text[i+k] == 't' {
				i += k + 1 // an escaped tab is whitespace
				continue
			}
			// Literal backslashes (a path, an escaped line break read as
			// text): punctuation.
			arm()
			pending, clean, joined = false, false, false
			i += k
			continue
		case c == '\'':
			arm()
			pending, joined = false, false
		case c == ':' || c == '=':
			if clean && (!armed || armedByGlue) && secretInstructionRun(run) {
				// "Set your API key with: export ..." says how to set the
				// secret, not its value, so a glued arming ("check your API
				// key. Set your API key with: ...") does not reach it: the
				// separator is not labelled, the arming ends, and the pairs in
				// its value are judged on their own. An arming by any other
				// punctuation still labels it.
				armed, runSecret, pending, clean, joined, armedIn = false, false, false, false, false, 0
				break
			}
			labelled := armed || clean && runSecret
			if labelled && pairCarriesValue(text[i+1:]) {
				return true
			}
			runSecret, pending, clean, joined = false, false, false, false
			if labelled {
				// An exempt value (<token>, "your-token-here") is skipped:
				// its own words never arm a qualifier.
				i += 1 + pairValueLen(text[i+1:])
				continue
			}
		default:
			arm()
			pending, clean, joined = false, false, false
		}
		i++
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
		next := unescapeOneLevel(s)
		if next == s {
			break
		}
		s = next
	}
	return s
}

// unescapeOneLevel undoes one level of JSON string escaping.
func unescapeOneLevel(s string) string {
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
	return b.String()
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

// pairCarriesValue takes the value that follows a separator and reports
// whether it carries anything. One tokenizer, and every exemption applies to
// the value it extracts, never to a prefix of the text:
//   - spaces and newlines after the separator are skipped;
//   - a value opening with a quote (" or ', possibly escaped as \" at this
//     escape level) runs to the matching closing quote, honoring backslash
//     escapes, or to the end of the text when unclosed; a closed quoted value
//     followed by anything but the end of the text or a delimiter runs on and
//     carries a value;
//   - any other value runs to the first delimiter (isValueDelimiter), except
//     the placeholder, which is matched whole;
//   - a value is exempt only when it is exactly empty, true, false, null or
//     WithheldValuePlaceholder (quoted content compared with its quotes and
//     surrounding whitespace removed), or an unclosed quote followed by
//     nothing but delimiters.
func pairCarriesValue(rest string) bool {
	// Fail closed: a value that starts on a later line (password:\nhunter2,
	// Uninstall PIN:\n482915, YAML block form at any indent) is judged like
	// a same-line value, so prose after a label that ends its line is
	// withheld too. A credential written on the next line is the common
	// shape; telling it from prose by indent or word count leaked it.
	rest = skipInlineSpace(rest)
	if line, ok := afterNewline(rest); ok {
		rest = skipInlineSpace(line)
	}
	if rest == "" {
		return false
	}
	n := 0
	for n < len(rest) && rest[n] == '\\' {
		n++
	}
	if n < len(rest) && (rest[n] == '"' || rest[n] == '\'') {
		content, after, closed := quotedValue(rest[n+1:], rest[n], n)
		if closed && !quotedValueEnds(after) {
			return true
		}
		if !closed && onlyDelimiters(content) {
			// An unclosed quote followed only by delimiters (a preview cut
			// right after the separator, then the enclosing JSON's own "})
			// carries nothing.
			return false
		}
		// Whitespace around the quoted content cannot carry a secret.
		return !exemptPairValue(strings.Trim(content, " \t\r\n"))
	}
	if strings.HasPrefix(rest, WithheldValuePlaceholder) {
		if after := rest[len(WithheldValuePlaceholder):]; after == "" || isValueDelimiter(after[0]) {
			return false
		}
	}
	end := 0
	for end < len(rest) && !isValueDelimiter(rest[end]) {
		end++
	}
	if end == 0 {
		// A value that starts with a delimiter (password=;hunter2) is empty
		// only when nothing but delimiters follows.
		return !onlyDelimiters(rest)
	}
	return !exemptPairValue(rest[:end])
}

// pairValueLen is the length of rest that pairCarriesValue reads as the
// value: leading whitespace and line breaks, then a quoted value to its
// closing quote, the placeholder whole, or a bare value to the first
// delimiter.
func pairValueLen(rest string) int {
	v := skipInlineSpace(rest)
	if line, ok := afterNewline(v); ok {
		v = skipInlineSpace(line)
	}
	n := 0
	for n < len(v) && v[n] == '\\' {
		n++
	}
	if n < len(v) && (v[n] == '"' || v[n] == '\'') {
		_, after, _ := quotedValue(v[n+1:], v[n], n)
		return len(rest) - len(after)
	}
	if strings.HasPrefix(v, WithheldValuePlaceholder) {
		return len(rest) - len(v) + len(WithheldValuePlaceholder)
	}
	end := 0
	for end < len(v) && !isValueDelimiter(v[end]) {
		end++
	}
	return len(rest) - len(v) + end
}

// skipInlineSpace drops leading spaces, tabs and escaped tabs (\t).
func skipInlineSpace(s string) string {
	for {
		switch {
		case strings.HasPrefix(s, " "), strings.HasPrefix(s, "\t"):
			s = s[1:]
		case strings.HasPrefix(s, `\t`):
			s = s[2:]
		default:
			return s
		}
	}
}

// afterNewline reports whether s starts with a line break (CR, LF, or one
// escaped as \r / \n at this level) and returns the text after the run of
// blank lines that follows.
func afterNewline(s string) (string, bool) {
	broke := false
	for {
		t := skipInlineSpace(s)
		switch {
		case strings.HasPrefix(t, "\r"), strings.HasPrefix(t, "\n"):
			s, broke = t[1:], true
		case strings.HasPrefix(t, `\r`), strings.HasPrefix(t, `\n`):
			s, broke = t[2:], true
		default:
			return s, broke
		}
	}
}

// quotedValue scans s, the text after an opening quote q that was written
// with n escaping backslashes before it, for the matching closing quote: the
// same quote with exactly n backslashes before it (with n == 0, a backslash
// escapes the next character). An unclosed value is the rest of the text.
func quotedValue(s string, q byte, n int) (content, after string, closed bool) {
	for i := 0; i < len(s); {
		k := 0
		for i+k < len(s) && s[i+k] == '\\' {
			k++
		}
		if i+k < len(s) && s[i+k] == q && k == n {
			return s[:i], s[i+k+1:], true
		}
		switch {
		case n == 0 && k > 0:
			i += k + 1 // a backslash escapes the character after it
		case k > 0:
			i += k
		default:
			i++
		}
	}
	return s, "", false
}

// quotedValueEnds reports whether the text after a closed quoted value ends
// it: the end of the text, a delimiter, or an escaped whitespace (\n, \r, \t
// at this escape level).
func quotedValueEnds(after string) bool {
	if after == "" || isValueDelimiter(after[0]) {
		return true
	}
	return len(after) > 1 && after[0] == '\\' && strings.IndexByte("nrt", after[1]) >= 0
}

// onlyDelimiters reports whether s holds nothing but delimiters (and
// backslashes, which only escape them at deeper levels).
func onlyDelimiters(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' && !isValueDelimiter(s[i]) {
			return false
		}
	}
	return true
}

// documentationPlaceholders are the values this CLI prints next to a secret
// name in its own auth hints and docs (export DNSFILTER_API_KEY=
// "your-token-here", DNSFILTER_MCP_HTTP_TOKEN=<long-random-value>). A real
// credential never equals one of them, so, like WithheldValuePlaceholder,
// they are exempt on an exact match of the extracted value only.
var documentationPlaceholders = map[string]bool{
	"your-token-here":     true,
	"<your-token-here>":   true,
	"<token>":             true,
	"<value>":             true,
	"<long-random-value>": true,
}

// exemptPairValue reports whether an extracted value carries nothing.
func exemptPairValue(v string) bool {
	switch v {
	case "", "true", "false", "null", WithheldValuePlaceholder:
		return true
	}
	return documentationPlaceholders[v]
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
// payment_token, refresh_token, pin, uninstall_pin, and the plurals
// (uninstall_pins, "Uninstall PINs:", "API tokens:"), since a plural label
// still heads secrets. token_type, last_four and pin_code_enabled do not
// match. A counter such as total_tokens is withheld too; no DNSFilter field
// has that shape, so that over-withholding costs nothing.
var secretKeySuffixes = []string{
	"token",
	"tokens",
	"pin",
	"pins",
}

// secretKeyWords match only as a whole key word (keyWords): nkey is
// DNSFilter's roaming-client deployment property for the site secret key
// (msiexec ... NKEY="...", a macOS plist <key>NKEY</key>), and as a fragment it
// would match monkey and donkey. "n" then "key" (nKey) is its camelCase
// spelling. No DNSFilter spec property is matched.
var secretKeyWords = map[string]bool{"nkey": true, "nkeys": true}

// secretWordPair reports whether key word kw, after key word prev, is a
// secret word.
func secretWordPair(prev, kw string) bool {
	return secretKeyWords[kw] || prev == "n" && (kw == "key" || kw == "keys")
}

// hasSecretWord reports whether words holds a secret word (secretWordPair).
func hasSecretWord(words []string) bool {
	prev := ""
	for _, w := range words {
		if secretWordPair(prev, w) {
			return true
		}
		prev = w
	}
	return false
}

// identifierWords end a key that names a reference to a secret-bearing
// object rather than the secret itself: api_keys_id (the parent key every
// revoke row carries), secretKeyId, token_uuid.
var identifierWords = map[string]bool{"id": true, "ids": true, "uuid": true, "uuids": true}

// IsSecretKey reports whether a JSON object key names a secret. Case, '_',
// '-' and any other non-alphanumeric separators are ignored; a key whose last
// word is id, ids, uuid or uuids is an identifier, not a secret. A key
// matches a fragment anywhere, a suffix at its end, or a whole secret word
// (NKEY, site_nkey; not monkey).
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
	return hasSecretWord(words)
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
