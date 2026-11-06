// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (jamf-pro): the one credential rule and redaction funnel,
// moved here from internal/fleet/model.go so fleet output, the credential
// holder, the output chokepoint and the local store share it.

package cliutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// credentialWords is the one list of credential words behind every
// credential test: field names, record labels, and key=value or key: value
// pairs inside strings. Jamf fields that carry credentials are passwords
// (LAPS, recovery lock, prestage admin, Open Firmware/EFI, AirPlay, webhook),
// API client secrets, bootstrap/unlock/service tokens, personal recovery keys,
// passcodes and PINs. It deliberately over-matches ("key", "token"); booleans
// that merely mention a secret (bootstrapTokenAllowed, passcodePresent) stay.
// A missing synonym is a one-line addition here.
var credentialWords = []string{`passw(or)?d`, `passphrase`, `pwd`, `passcode`, `pin`, `secret`, `token`, `key`, `credential`, `authorization`}

// ownWordOnly lists the words that match a name only as a whole word of it
// (pin, devicePin, recovery_pin, PINCode): "pin" is inside shipping and
// mapping. Inside a string's key=value pair every word matches as is.
var ownWordOnly = map[string]bool{"pin": true}

var (
	credentialNameRe  = regexp.MustCompile(`(?i)(` + strings.Join(wordsWhere(false), "|") + `)`)
	credentialValueKV = `(` + strings.Join(credentialWords, "|") + `)["']?\s*[:=]`
)

func wordsWhere(own bool) []string {
	var out []string
	for _, w := range credentialWords {
		if ownWordOnly[w] == own {
			out = append(out, w)
		}
	}
	return out
}

// credentialName reports a field name or label that names a credential.
func credentialName(name string) bool {
	if credentialNameRe.MatchString(name) {
		return true
	}
	for _, w := range nameWords(name) {
		for _, own := range wordsWhere(true) {
			if w == own || w == own+"code" || w == own+"number" {
				return true
			}
		}
	}
	return false
}

// nameWords splits a name into lower-case words at separators and case
// changes: "lockPINCode" -> lock, pin, code; "recovery_pin" -> recovery, pin.
func nameWords(name string) []string {
	var words []string
	var cur []rune
	rs := []rune(name)
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	isUp := func(r rune) bool { return r >= 'A' && r <= 'Z' }
	isLow := func(r rune) bool { return r >= 'a' && r <= 'z' }
	for i, r := range rs {
		if !isUp(r) && !isLow(r) && !(r >= '0' && r <= '9') {
			flush()
			continue
		}
		if i > 0 && isUp(r) && (isLow(rs[i-1]) || (isUp(rs[i-1]) && i+1 < len(rs) && isLow(rs[i+1]))) {
			flush()
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// publicFields match credentialName but carry status, policy or metadata,
// not a credential (checked against every property in this CLI's spec).
var publicFields = map[string]bool{
	"accessTokenLifetimeSeconds": true, "anchorCertificates": true, "bootstrapTokenEscrowed": true,
	"bootstrapTokenEscrowedStatus": true, "certificateStatus": true, "certificates": true,
	"individualRecoveryKeyValidityStatus": true, "key_type": true, "lastPasswordChange": true,
	"managed_password_length": true, "passwordHistoryDepth": true, "passwordMaxAge": true,
	"passwordMinComplexCharacters": true, "passwordMinLength": true, "passwordRotationTime": true,
	"productKey":               true, // a software-update product identifier, not a licence key
	"recoveryLockPasswordType": true, "redeploy_Dayss_before_certificate_expires": true,
	"remediate_key_type": true, "tokenExpiration": true, "tokenExpirationDate": true, "tokenFileName": true,
	"totalCount": true, "authorizationScopes": true, "passcodeLockGracePeriod": true,
	"passcodeLockGracePeriodEnforcedSeconds": true,
	// The fleet registry's secret_env (flag --secret-env) names the variable
	// that holds an instance's secret, never the secret; a token URL is an
	// endpoint (jamf-pro handfix: output-chokepoint).
	"secret_env": true, "secret-env": true, "token_url": true, "token-url": true, "tokenUrl": true,
	// A usage count the generated local store keeps (its own tests store
	// it); no Jamf object has the field (jamf-pro handfix one-rule-everywhere:
	// the store now applies this rule, numbers included).
	"total_tokens": true,
}

// opaqueFields hold free-form documents or execution inputs that can embed
// credentials anywhere inside: profile plists (Wi-Fi passwords), app
// configuration preferences, script bodies, a policy's run_command, and the
// values passed to script parameters 4-11. They are replaced by a content
// digest: never printed, still comparable between instances.
var opaqueFields = map[string]bool{
	"payloads": true, "payload": true, "preferences": true, "scriptContents": true, "script_contents": true,
	"script_contents_encoded": true, "run_command": true, "runCommand": true,
	"parameter4": true, "parameter5": true, "parameter6": true, "parameter7": true,
	"parameter8": true, "parameter9": true, "parameter10": true, "parameter11": true,
}

// SecretField reports a property name that looks like a credential.
// The name is judged as is and in every form it cleanly decodes to; a name
// that looks encoded but does not decode cleanly is treated as secret.
func SecretField(name string) bool { return !publicFields[name] && namesCredential(name) }

// namesCredential applies credentialName to a name or label and to every
// form the shared decoder reaches from it (deny by default on a bad decode).
func namesCredential(name string) bool {
	forms, clean := decodedForms(name, 0)
	if !clean {
		return true
	}
	for _, f := range forms {
		if credentialName(f) {
			return true
		}
	}
	return false
}

// decodedForms returns s and every form reachable from it through
// decodings, to maxDecodeDepth: the same decoder and bound the string funnel
// uses for values. clean is false when any encoding s shows a sign of does
// not decode cleanly, or nesting goes deeper than the bound.
func decodedForms(s string, depth int) (forms []string, clean bool) {
	forms = []string{s}
	for _, d := range decodings(s) {
		if !d.ok || depth >= maxDecodeDepth {
			return forms, false
		}
		more, ok := decodedForms(d.s, depth+1)
		forms = append(forms, more...)
		if !ok {
			return forms, false
		}
	}
	return forms, true
}

// Redacted replaces credential-bearing values with "[redacted]".
const Redacted = "[redacted]"

// RedactedDigestPrefix starts the replacement for an opaque document.
const RedactedDigestPrefix = "[redacted sha256:"

// labelFields name the label of a name/value record (lower-cased).
var labelFields = map[string]bool{"name": true, "key": true, "displayname": true, "label": true, "title": true,
	"attributename": true, "fieldname": true, "definitionname": true}

// Redact is the one output funnel every fleet result passes through.
//
// Threat model. Redaction covers (1) the credentials Jamf Pro stores in its
// own credential fields (LAPS and recovery-lock passwords, FileVault and
// recovery keys, API client secrets, tokens, PINs) and in name/value records
// labelled as credentials, and (2) credentials written plainly, or in one of
// the standard encodings below, inside any value. It does not claim to catch
// a credential deliberately obfuscated to hide it (an encoded or disguised
// label, a value split or encoded to evade detection): whoever can write
// that can equally store a password under an innocent label such as "Notes",
// which no redactor can recognise.
//
// It walks every map value, array element and scalar, and withholds a WHOLE
// value, never part of one, when:
//   - its key path is secret: it sits under a credential-named field, or it
//     is the value of a name/value record whose label names a credential
//     (booleans stay: they are flags such as bootstrapTokenEscrowed);
//   - its field holds an opaque document (replaced by a digest);
//   - it is a string that fails the string funnel (scrubString).
//
// Strings are deny by default: an embedded JSON document is parsed and
// walked like any other value; any encoding the string shows a sign of
// (JSON escapes, %XX, HTML entities, base64 runs) must decode cleanly, and
// what it decodes to must pass the funnel unchanged, or the whole string is
// withheld. Finally the string itself must not carry a credential: a
// credential word followed by = or :, a Bearer or Basic credential, a
// private key, a JWT, or a URL with userinfo, a query or a fragment (under
// a URL-named field, any path too).
func Redact(v any) any {
	return redactValue(v, "", map[string]bool{})
}

// RedactString applies the string funnel to free text (key unknown).
func RedactString(s string) string { return scrubString(s, "", map[string]bool{}, 0) }

// redactValue memoises the key test per field name: a large collection
// repeats the same few dozen keys, and the regex dominates redaction time.
func redactValue(v any, key string, seen map[string]bool) any {
	switch t := v.(type) {
	case map[string]any:
		record := secretRecord(t)
		for k, val := range t {
			if _, isBool := val.(bool); !isBool && val != nil && (secretName(k, seen) || (record && valueField(k))) {
				t[k] = Redacted
				continue
			}
			t[k] = redactValue(val, k, seen)
		}
	case []any:
		for i := range t {
			t[i] = redactValue(t[i], key, seen)
		}
	case string:
		return scrubString(t, key, seen, 0)
	}
	return v
}

// maxDecodeDepth bounds nested encodings; deeper nesting is withheld.
const maxDecodeDepth = 4

// scrubString is the string half of the funnel (see Redact).
func scrubString(s, key string, seen map[string]bool, depth int) string {
	if s == "" {
		return s
	}
	if opaqueFields[key] {
		if s == Redacted || isDigest(s) {
			return s // already applied: the rule is idempotent
		}
		return Digest(s)
	}
	if depth > maxDecodeDepth {
		return Redacted
	}
	if jsonDocument(s) {
		var inner any
		if err := json.Unmarshal([]byte(s), &inner); err != nil || duplicateKeys(s) {
			return Redacted
		}
		out, err := json.Marshal(redactValue(inner, key, seen))
		if err != nil {
			return Redacted
		}
		return string(out)
	}
	if jsonFragmentChanged(s, key, seen) {
		return Redacted
	}
	for _, d := range decodings(s) {
		if !d.ok || scrubString(d.s, key, seen, depth+1) != d.s {
			return Redacted
		}
	}
	if secretValue.MatchString(s) || urlCarriesSecret(key, s) || redactXML(s, seen, depth) != s {
		return Redacted
	}
	return s
}

// xmlLeaf is an XML element with text (or CDATA) content only; xmlStartTag
// a start or empty-element tag with attributes; xmlAttr one attribute.
var (
	xmlLeaf     = regexp.MustCompile(`<([A-Za-z_][\w.:-]*)((?:\s[^<>]*?)?)>(<!\[CDATA\[[\s\S]*?\]\]>|[^<]*)</([A-Za-z_][\w.:-]*)\s*>`)
	xmlStartTag = regexp.MustCompile(`<[A-Za-z_][\w.:-]*\s[^<>]*>`)
	xmlAttr     = regexp.MustCompile(`([A-Za-z_][\w.:-]*)(\s*=\s*)("[^"]*"|'[^']*')`)
)

// redactXML applies the rule to XML markup in s (Jamf's Classic API speaks
// XML): an element or an attribute is a field. A credential-named one has
// its content replaced by Redacted; any other has its content judged by the
// string funnel under its name (an opaque element such as a script body is
// replaced by its digest, a credential written inside it withholds it).
// Text without markup is returned unchanged.
func redactXML(s string, seen map[string]bool, depth int) string {
	if !strings.Contains(s, "<") {
		return s
	}
	if found := xmlContainerValues(s, seen); len(found) > 0 {
		for _, v := range found {
			holdFound(v)
		}
		return Redacted
	}
	judge := func(name, content string) string {
		inner := content
		if strings.HasPrefix(inner, "<![CDATA[") {
			inner = strings.TrimSuffix(strings.TrimPrefix(inner, "<![CDATA["), "]]>")
		}
		if strings.TrimSpace(inner) == "" || isMaskValue(inner) {
			return content
		}
		if secretName(name, seen) {
			holdFound(inner)
			return Redacted
		}
		if out := scrubString(inner, name, seen, depth+1); out != inner {
			return out
		}
		return content
	}
	out := xmlLeaf.ReplaceAllStringFunc(s, func(m string) string {
		sub := xmlLeaf.FindStringSubmatch(m)
		if sub[1] != sub[4] {
			return m
		}
		content := judge(sub[1], sub[3])
		if content == sub[3] {
			return m
		}
		return "<" + sub[1] + sub[2] + ">" + content + "</" + sub[4] + ">"
	})
	return xmlStartTag.ReplaceAllStringFunc(out, func(tag string) string {
		return xmlAttr.ReplaceAllStringFunc(tag, func(a string) string {
			sub := xmlAttr.FindStringSubmatch(a)
			q := sub[3][:1]
			val := sub[3][1 : len(sub[3])-1]
			if content := judge(sub[1], val); content != val {
				return sub[1] + sub[2] + q + content + q
			}
			return a
		})
	})
}

// xmlContainerValues returns the text and attribute values inside a
// credential-named element that holds child elements, the shape the leaf match
// above cannot judge. It reads as far as s parses.
func xmlContainerValues(s string, seen map[string]bool) (found []string) {
	dec := xml.NewDecoder(strings.NewReader(strings.ToValidUTF8(s, "?")))
	dec.Strict = false
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	depth, inside := 0, 0 // inside: the depth of the open credential element, 0 if none
	nested := false
	var vals []string
	keep := func(v string) {
		if strings.TrimSpace(v) != "" && !isMaskValue(v) {
			vals = append(vals, v)
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			if inside > 0 && nested {
				found = append(found, vals...)
			}
			return found
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if inside > 0 {
				nested = true
				for _, a := range t.Attr {
					keep(a.Value)
				}
			} else if secretName(t.Name.Local, seen) {
				inside, nested, vals = depth, false, nil
			}
		case xml.CharData:
			if inside > 0 {
				keep(string(t))
			}
		case xml.EndElement:
			if inside == depth {
				if nested {
					found = append(found, vals...)
				}
				inside = 0
			}
			depth--
		}
	}
}

// isDigest reports a value the rule already replaced by Digest.
func isDigest(s string) bool {
	return strings.HasPrefix(s, RedactedDigestPrefix) && strings.HasSuffix(s, "]") && len(s) == len(RedactedDigestPrefix)+17
}

// jsonFragmentChanged reports JSON inside a string that the funnel would
// change: every object or array that parses, starting anywhere in s, is
// walked like any other value ("Result: {\"name\":\"API Token\",...}").
func jsonFragmentChanged(s, key string, seen map[string]bool) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '{' && s[i] != '[' {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(s[i:]))
		var v any
		if dec.Decode(&v) != nil {
			continue
		}
		switch v.(type) {
		case map[string]any, []any:
		default:
			continue
		}
		// A repeated key decodes to its last value only, so the walk could
		// not see the first: such a fragment is withheld outright.
		end := i + int(dec.InputOffset())
		if duplicateKeys(s[i:end]) {
			return true
		}
		before, _ := json.Marshal(v)
		after, _ := json.Marshal(redactValue(v, key, seen))
		if !bytes.Equal(before, after) {
			return true
		}
		i = end - 1
	}
	return false
}

// duplicateKeys reports a JSON text in which some object repeats a key.
func duplicateKeys(raw string) bool {
	type frame struct {
		object, wantKey bool
		keys            map[string]bool
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	var stack []*frame
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{':
				stack = append(stack, &frame{object: true, wantKey: true, keys: map[string]bool{}})
				continue
			case '[':
				stack = append(stack, &frame{})
				continue
			default: // '}' or ']': a value ends
				stack = stack[:len(stack)-1]
			}
		case string:
			if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].wantKey {
				if stack[n-1].keys[v] {
					return true
				}
				stack[n-1].keys[v] = true
				stack[n-1].wantKey = false
				continue
			}
		}
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].wantKey = true
		}
	}
}

// jsonDocument reports a string that is an embedded JSON object or array.
func jsonDocument(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) < 2 {
		return false
	}
	switch {
	case t[0] == '{' && t[len(t)-1] == '}':
		return true
	case t[0] == '[' && t[len(t)-1] == ']':
		rest := strings.TrimSpace(t[1:])
		return rest != "" && strings.ContainsRune(`{["-0123456789]`, rune(rest[0]))
	}
	return false
}

type decoded struct {
	s  string
	ok bool
}

var (
	percentSign = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)
	entitySign  = regexp.MustCompile(`&(#[0-9]+|#[xX][0-9A-Fa-f]+|[A-Za-z]+);`)
	base64Run   = regexp.MustCompile(`[A-Za-z0-9+/_-]{16,}={0,2}`)
)

// decodings tries every encoding s shows a sign of. It is the one decoder
// for values, labels and field names. ok is false when s looks encoded but
// does not decode cleanly (a malformed JSON or percent escape).
func decodings(s string) []decoded {
	var out []decoded
	if strings.Contains(s, `\`) {
		var d string
		err := json.Unmarshal([]byte(`"`+s+`"`), &d)
		out = append(out, decoded{d, err == nil})
	}
	if strings.Contains(s, "%") && percentSign.MatchString(s) {
		d, err := url.PathUnescape(s)
		out = append(out, decoded{d, err == nil})
	}
	if entitySign.MatchString(s) {
		out = append(out, decoded{html.UnescapeString(s), true})
	}
	// Every run is tried, with no exemption: one that decodes to text is
	// judged in that form; one that decodes to binary (or not at all) holds
	// no readable credential and is left as it is.
	for _, run := range base64Run.FindAllString(s, -1) {
		if d, ok := decodeBase64Text(run); ok {
			out = append(out, decoded{d, true})
		}
	}
	return out
}

// decodeBase64Text decodes a base64 or base64url run (padded or not) that
// holds text.
func decodeBase64Text(tok string) (string, bool) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		b, err := enc.DecodeString(tok)
		if err != nil || len(b) == 0 || !utf8.Valid(b) {
			continue
		}
		printable := 0
		for _, r := range string(b) {
			if r == '\n' || r == '\t' || r == '\r' || (r >= 0x20 && r != 0x7f) {
				printable++
			}
		}
		if printable == utf8.RuneCount(b) {
			return string(b), true
		}
	}
	return "", false
}

// secretName memoises SecretField per field name.
func secretName(k string, seen map[string]bool) bool {
	hit, ok := seen[k]
	if !ok {
		hit = SecretField(k)
		seen[k] = hit
	}
	return hit
}

func valueField(k string) bool {
	k = strings.ToLower(k)
	return k == "value" || k == "values"
}

// secretRecord reports a name/value record labelled with a credential word.
// Labels are free text, so the publicFields exemptions (field names) do not
// apply; the label goes through namesCredential, the same decoder and word
// list as field names and values.
func secretRecord(m map[string]any) bool {
	for k, lv := range m {
		if n, ok := lv.(string); ok && labelFields[strings.ToLower(k)] && namesCredential(n) {
			return true
		}
	}
	return false
}

// secretValue matches a string that carries a credential: a credential word
// (the same list as names) followed by = or :, and the shapes below.
var secretValue = regexp.MustCompile(`(?i)(` + credentialValueKV +
	`|\b(bearer|basic)\s+[a-z0-9._~+/=-]{8,}` +
	`|-----begin [a-z ]*private key-----` +
	`|\beyj[a-z0-9_-]{8,}\.[a-z0-9_-]{4,}\.` +
	`)`)

// urlInText finds absolute URLs anywhere in a string.
var urlInText = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s"'<>]+`)

// urlKey matches fields that hold a destination URL (webhook url, callback,
// endpoint). Their path can carry a token (Slack and Teams webhook paths).
var urlKey = regexp.MustCompile(`(?i)(url|uri|endpoint|callback|hook)s?$`)

// urlCarriesSecret reports a URL in s that has userinfo, a query or a
// fragment, or, under a URL-named field, a path.
func urlCarriesSecret(key, s string) bool {
	for _, raw := range urlInText.FindAllString(s, -1) {
		u, err := url.Parse(raw)
		if err != nil {
			return true // unparseable: cannot prove it is clean
		}
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Contains(raw, "@") {
			return true
		}
		if urlKey.MatchString(key) && u.Path != "" && u.Path != "/" {
			return true
		}
	}
	return false
}

// Digest is the printable stand-in for an opaque document's contents.
func Digest(s string) string {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(s, "\r\n", "\n")))
	return RedactedDigestPrefix + hex.EncodeToString(sum[:8]) + "]"
}

// RedactJSON masks credential fields in a raw JSON body.
func RedactJSON(body json.RawMessage) (json.RawMessage, error) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return json.Marshal(Redact(v))
}
