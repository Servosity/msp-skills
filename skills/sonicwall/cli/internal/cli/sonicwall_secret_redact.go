// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"sonicwall-pp-cli/internal/posture"
)

// SonicWall's own API schemas document secrets on reads this CLI exposes: a
// password per local user, assign_token, authentication_code and
// registration_code in SonicOS system status, SNMP community names, a device
// password and authCode in NSM's device_info, and in MySonicWall an api_key per
// firewall, tcpassword in service info, authCode in the product list and
// activationKey in the activity log. The typed commands, sync, the cache and the
// MCP execute tool used to pass them through as SonicWall returned them. This
// hook blanks every such field in every NSM, SonicOS passthrough and MySonicWall
// response, before the client caches it or any command, sync or MCP tool reads it.
//
// Exact names only (from a scan of the three vendor schemas, matched
// case-insensitively): a key heuristic would also blank one_time_password and
// totp, which posture and mfa-audit must read. The token exchanges
// (nsm-login, nsm-session, msw-tenants access-code) are skipped: returning a
// token is their job, and they are the Credential tier.
var schemaSecretFields = map[string]bool{
	// SonicOS (beyond nsmCredentialFields, which passthrough reads also use)
	"authentication_code": true, "registration_code": true, "get_community_name": true,
	"trap_community_name": true, "encrypted_user_name_password": true, "extended_storage_auth_code": true,
	"primary_storage_auth_code": true, "wep_key": true,
	// NSM API Gateway
	"authcode": true, "authtoken": true, "devicepassword": true, "licensekey": true,
	"password": true, "pemdatakey": true, "temptoken": true, "token": true,
	"totpbarcodeurl": true, "authentication_key": true, "encryption_key": true,
	// MySonicWall
	"api_key": true, "apikey": true, "tcpassword": true, "activationkey": true,
	"assigntoken": true, "authenticationcode": true, "hespassword": true,
	"refreshtoken": true, "registrationcode": true, "transauthcode": true,
}

// tokenExchangePaths return a token on purpose; their callers need it intact.
var tokenExchangePaths = []string{"/auth/sso", "/auth/refreshSessionToken", "/api/generate-cscAccessCode"}

// The transport is installed by the envelope hook in sonicwall_nsm_envelope.go,
// inside it, so this redaction always sees the server's raw body.

type sonicOSRedactTransport struct{ base http.RoundTripper }

// redactFieldsFor reports whether req's response must be scanned for secrets
// (every call except a token exchange) and whether it is a SonicOS passthrough
// read (a /firewall/ route or any request addressed to one device), which also
// gets the generated SonicOS list nsm-get uses. A token exchange is the CLI's
// own POST to one of tokenExchangePaths: never a redirect hop (the server picks
// that path) and never a path whose /auth/sso came from an escaped value.
func redactFieldsFor(req *http.Request) (scan, passthrough bool) {
	if req.Method == http.MethodPost && req.Response == nil {
		for _, p := range tokenExchangePaths {
			if strings.HasSuffix(req.URL.EscapedPath(), p) {
				return false, false
			}
		}
	}
	return true, req.Header.Get("X-DEVICE-ID") != "" || strings.Contains(req.URL.Path, "/firewall/")
}

// secretKeySuffixes catch the vendor's other spellings of the same secrets
// (register_code, regCode, transAuthCode, extended_storage_auth_code), the
// Virtual Assist assistance_code and login session IDs, compared on the key
// lowercased with everything but letters and digits removed. Suffixes, not
// substrings, so error codes (msgCode, code) and product_code stay readable.
var secretKeySuffixes = []string{"regcode", "registercode", "registrationcode", "authcode",
	"authenticationcode", "assistancecode", "activationcode", "activationkey", "sessionid"}

// squash lowercases s and drops everything but letters and digits, so every
// spelling of a name (pop3_password, pop3-password, Pop3Password) compares equal.
func squash(s string) string {
	return strings.Map(func(r rune) rune {
		r = foldFullWidth(r)
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, strings.ToLower(s))
}

// foldFullWidth reads full-width ASCII (U+FF01-FF5E) as the ASCII it stands for.
func foldFullWidth(r rune) rune {
	if r >= 0xFF01 && r <= 0xFF5E {
		return r - 0xFEE0
	}
	return r
}

// squashedSchemaNames and squashedCredentialNames are the two secret lists in
// squashed form; the credential list leaves out the generic passthrough words.
var squashedSchemaNames, squashedCredentialNames = func() (map[string]bool, map[string]bool) {
	a, b := map[string]bool{}, map[string]bool{}
	for k := range schemaSecretFields {
		a[squash(k)] = true
	}
	for k := range nsmCredentialFields {
		if !genericPassthroughNames[k] {
			b[squash(k)] = true
		}
	}
	return a, b
}()

// isSchemaSecret reports whether key names a schema-documented secret, in any
// spelling, on any route: a credential name SonicOS documents (pop3_password,
// shared_secret) is a credential in an NSM or MySonicWall body too. Only the
// generic words (key, new, old) are limited to SonicOS passthrough reads.
// There is one lookup, on the squashed key, so every spelling of a name
// (KEY, ｋｅｙ, k.e.y) gets exactly the rule the plain name gets.
func isSchemaSecret(key string, passthrough bool) bool {
	n := squash(key)
	if squashedSchemaNames[n] || squashedCredentialNames[n] || (passthrough && genericPassthroughNames[n]) {
		return true
	}
	for _, suffix := range secretKeySuffixes {
		if strings.HasSuffix(n, suffix) {
			return true
		}
	}
	return false
}

func (t sonicOSRedactTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	scan, passthrough := redactFieldsFor(req)
	if err != nil || resp == nil || resp.Body == nil || !scan {
		return resp, err
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("reading %s response: %w", req.URL.Path, readErr)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	// One rule closes every shape: the client is handed exactly the bytes judged
	// here, decoded and re-encoded, never the server's original bytes.
	plain, derr := decodeForInspection(resp.Header.Get("Content-Encoding"), body)
	if derr != nil {
		return withheldSecretResponse(resp, req.URL.Path, "it could not be decoded for inspection"), nil
	}
	resp = replaceBody(resp, plain)
	if len(bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(plain), []byte("\xEF\xBB\xBF")))) == 0 {
		return resp, nil
	}
	// Every route this CLI reads answers JSON. Anything else (text, HTML, a file
	// download, a binary body, concatenated values) is withheld whole, keeping an
	// error's status so the CLI still classifies the failure.
	values, isJSON := decodeJSONStream(bytes.TrimSpace(stripJSONPrefixes(plain)))
	if !isJSON || len(values) != 1 {
		return withheldSecretResponse(resp, req.URL.Path, "it is not one JSON document"), nil
	}
	doc := values[0]
	if !isContainer(doc) {
		if str, ok := doc.(string); ok && bareTextHasSecret(str) {
			return withheldSecretResponse(resp, req.URL.Path, "it names a credential"), nil
		}
		return resp, nil
	}
	redactSchemaSecrets(doc, passthrough)
	// Re-encoded even when nothing changed: a duplicate key or an XSSI guard in
	// the original bytes must not reach the client.
	out, merr := json.Marshal(doc)
	if merr != nil {
		return nil, fmt.Errorf("re-encoding redacted %s response: %w", req.URL.Path, merr)
	}
	return replaceBody(resp, out), nil
}

// bareTextHasSecret judges a bare-string body, which no command grades on,
// more strictly than a nested string, by construction: withheld when the
// nested rule would blank it (textAssignsSecret), when it or its decoded form
// names a secret or a free-text credential word anywhere, or when it cannot be
// decoded.
func bareTextHasSecret(s string) bool {
	d, ok := decodeLenient(s)
	return !ok || textAssignsSecret(s) || textHasSecret(s) || textHasSecret(d) || containsFreeTextWord(d)
}

// containsFreeTextWord reports whether s, squashed, contains a free-text
// credential word (passwd, pwd, community...).
func containsFreeTextWord(s string) bool {
	sq := squash(s)
	for _, w := range freeTextSecretWords {
		if strings.Contains(sq, w) {
			return true
		}
	}
	return false
}

// decodeOneJSONValue reports whether body is exactly one JSON object or array
// (trailing data, such as a second concatenated value, does not count).
func decodeOneJSONValue(body []byte, doc *any) bool {
	if len(body) == 0 || (body[0] != '{' && body[0] != '[') {
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if dec.Decode(doc) != nil {
		return false
	}
	return len(bytes.TrimSpace(body[dec.InputOffset():])) == 0
}

// replaceBody swaps resp's body for a plain, already-decoded one.
func replaceBody(resp *http.Response, body []byte) *http.Response {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	resp.ContentLength = int64(len(body))
	return resp
}

// withheldSecretResponse replaces a body that could not be redacted with a
// note saying why. A success becomes a non-retryable 422 so the command fails
// instead of printing it; an error keeps its status and loses only its body.
func withheldSecretResponse(resp *http.Response, path, why string) *http.Response {
	msg := []byte("response body from " + path + " withheld: " + why + ", so it cannot be redacted")
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		resp.StatusCode = http.StatusUnprocessableEntity
		resp.Status = http.StatusText(http.StatusUnprocessableEntity)
	}
	resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
	return replaceBody(resp, msg)
}

// redactSchemaSecrets blanks the value under every schema-named secret key
// (judged as written and decoded), anywhere in doc, and every nested string
// that assigns a value to a credential name (textAssignsSecret). It reports
// whether anything changed.
func redactSchemaSecrets(v any, passthrough bool) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		changed = dropAssigningKeys(t)
		for k, val := range t {
			var c bool
			dk, _ := decodeLenient(k)
			if isSchemaSecret(k, passthrough) || isSchemaSecret(dk, passthrough) {
				t[k], c = redactSecretValue(k, val, passthrough)
			} else {
				t[k], c = redactNestedValue(val, passthrough)
			}
			changed = changed || c
		}
	case []any:
		for i, val := range t {
			var c bool
			t[i], c = redactNestedValue(val, passthrough)
			changed = changed || c
		}
	}
	return changed
}

// dropAssigningKeys removes every entry whose key assigns a value to a
// credential name ({"VPN token: X": ...}), key and value together: the value
// is what the key assigns. Every walker calls it before it reads a map, so no
// container shape keeps such a key, and nothing is renamed (a numbered
// placeholder would read as a name).
func dropAssigningKeys(m map[string]any) bool {
	changed := false
	for k := range m {
		if textAssignsSecret(k) {
			delete(m, k)
			changed = true
		}
	}
	return changed
}

// redactedText replaces a secret. posture.Redacted is the same string, so a
// grader never joins two different names on it.
const redactedText = posture.Redacted

// redactNestedValue judges a value under a non-secret key or array slot: a
// string is blanked when it assigns a value to a credential name, a container
// is walked, and everything else is kept.
func redactNestedValue(val any, passthrough bool) (any, bool) {
	switch x := val.(type) {
	case string:
		if textAssignsSecret(x) {
			return redactedText, true
		}
		return x, false
	case map[string]any, []any:
		return x, redactSchemaSecrets(x, passthrough)
	}
	return val, false
}

// textAssignsSecret reports whether a nested string assigns a value to a
// credential name: somewhere in its text or its decoded form, a separator
// (isAssignSeparator) follows text whose
// squashed form ends in a secret name, or whose last word is one of
// wholeWordSecrets ("authentication_code: X", "pass%77ord=X", "Temp pw: X", a
// JSON string holding {"password":"X"}). Where the value ends is never inferred: one assignment
// blanks the whole string. A name with nothing assigned carries no value and
// is kept, so the user, group and zone names posture and mfa-audit join on
// ("Token Users", "WAN-secret") and enumerations such as ssl_vpn auth_type
// "password" stay readable. Text that cannot be decoded to a fixed point
// counts as assigning one.
func textAssignsSecret(s string) bool {
	d, ok := decodeLenient(s)
	if !ok {
		return true
	}
	for _, t := range [2]string{s, d} {
		// b is the squashed text so far; word is the last word, kept across
		// the spaces before a separator ("pw : X").
		var b, word strings.Builder
		gap := false
		for _, r := range strings.ToLower(t) {
			r = foldFullWidth(r)
			switch {
			case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
				if gap {
					word.Reset()
					gap = false
				}
				b.WriteRune(r)
				word.WriteRune(r)
			case unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError:
				// whitespace, invisible format characters (zero-width space,
				// BOM, soft hyphen) and an undecodable byte never end a word
				gap = true
			case isAssignSeparator(r):
				if endsWithSecretName(b.String()) || wholeWordSecrets[word.String()] {
					return true
				}
				word.Reset()
				gap = false
			default:
				word.Reset()
				gap = false
			}
		}
	}
	return false
}

// isAssignSeparator reports whether r can introduce an assigned value: ':' or
// '=', or a non-ASCII rune in the Unicode categories where every colon and
// equals lookalike sits (Po other punctuation, Sm math symbols, Sk modifier
// symbols), so none needs listing. A code point this Go build's Unicode tables
// do not assign counts too (a newer Unicode version may make it punctuation),
// so the tables' age opens no gap. Dashes, quotes, brackets and marks such as
// the registered sign are not separators: names use them ("Hardware Token -
// Pilot" with an en dash) and they do not assign.
func isAssignSeparator(r rune) bool {
	if r == ':' || r == '=' {
		return true
	}
	if r <= unicode.MaxASCII {
		return false
	}
	return unicode.In(r, unicode.Po, unicode.Sm, unicode.Sk, unassigned)
}

// unassigned is Cn, the code points this Go build's Unicode tables do not
// assign (Go's unicode.C includes them, so it cannot be used to exclude them).
var unassigned = unicode.Categories["Cn"]

// wholeWordSecrets are credential words too short to match as the end of a
// longer word ("Bypass:", "hairpin:"), so they count only as the whole word
// before the separator ("Temp pw: X", "Wi-Fi PIN: X").
var wholeWordSecrets = map[string]bool{"pass": true, "pw": true, "pin": true}

// freeTextSecretWords are the credential words nsm-get's credentialKey also
// treats as secrets (pwd, passwd, passcode...) plus SNMP community names, for
// free text only: as JSON keys they would blank fields posture reads. pass, pw
// and pin are wholeWordSecrets instead: as word endings they would match
// "Authentication Bypass:" and "NAT hairpin:".
var freeTextSecretWords = []string{"passwd", "pwd", "passcode", "credential", "credentials",
	"privatekey", "presharedkey", "sharedkey", "secretkey", "recoverycode", "community", "communityname", "communitystring"}

// endsWithSecretName reports whether squashed text ends in a secret name,
// suffix or free-text credential word.
func endsWithSecretName(sq string) bool {
	if sq == "" {
		return false
	}
	for _, names := range []map[string]bool{squashedSchemaNames, squashedCredentialNames} {
		for name := range names {
			if strings.HasSuffix(sq, name) {
				return true
			}
		}
	}
	for _, list := range [][]string{secretKeySuffixes, freeTextSecretWords} {
		for _, name := range list {
			if strings.HasSuffix(sq, name) {
				return true
			}
		}
	}
	return false
}

// settingsObjectNames are secret names the SonicOS schema also uses for
// settings objects: administration.password is the password policy, and
// cli.ftp.password, admin_password, passphrase and shared_secret nest their
// real secrets beside policy fields.
var settingsObjectNames = map[string]bool{"password": true, "adminpassword": true, "passphrase": true, "sharedsecret": true}

// settingsPolicyPaths are the non-secret leaves the SonicOS schema documents
// under those settings objects, by squashed path from the settings key, with
// the JSON type the schema gives each (from a scan of every definition in the
// SonicOS API schema). The secrets beside them (password.default from
// cli.ftp, passphrase.password, shared_secret.shared_secret, every
// admin_password child) are not listed. A closed set judged by path AND type:
// a child at any other path, or of another type, is blanked whatever its key.
var settingsPolicyPaths = map[string]string{
	"password.aging.duration": "number", "password.complexity.digital": "number",
	"password.complexity.lowercase": "number", "password.complexity.symbolic": "number",
	"password.complexity.type": "enum", "password.complexity.uppercase": "number",
	"password.constraintsapplyto.auditadmins": "boolean", "password.constraintsapplyto.builtinadmin": "boolean",
	"password.constraintsapplyto.cryptoadmins": "boolean", "password.constraintsapplyto.fulladmins": "boolean",
	"password.constraintsapplyto.guestadmins": "boolean", "password.constraintsapplyto.limitedadmins": "boolean",
	"password.constraintsapplyto.localusers": "boolean", "password.constraintsapplyto.systemadmins": "boolean",
	"password.enforcecharacterdifference": "boolean", "password.lastchanged.period": "number",
	"password.minimumlength": "number", "password.uniqueness.count": "number",
	"passphrase.activetime": "number", "passphrase.page.custom": "string", "passphrase.page.default": "boolean",
	"sharedsecret.apclientid": "string", "sharedsecret.usedefaultkey": "boolean",
	"sharedsecret.ikeid.local.domainname": "string", "sharedsecret.ikeid.local.emailaddress": "string",
	"sharedsecret.ikeid.local.firewallid": "string", "sharedsecret.ikeid.local.ipv4": "string",
	"sharedsecret.ikeid.local.ipv6": "string", "sharedsecret.ikeid.local.keyid": "string",
	"sharedsecret.ikeid.peer.domainname": "string", "sharedsecret.ikeid.peer.emailaddress": "string",
	"sharedsecret.ikeid.peer.firewallid": "string", "sharedsecret.ikeid.peer.ipv4": "string",
	"sharedsecret.ikeid.peer.ipv6": "string", "sharedsecret.ikeid.peer.keyid": "string",
}

// settingsPolicyEnums are the values the schema allows for an "enum" path.
var settingsPolicyEnums = map[string]map[string]bool{
	"password.complexity.type": {"alpha-and-numeric": true, "alpha-and-numeric-and-symbols": true},
}

// settingsPolicyObjects are the intermediate objects on the way to those
// leaves (password.complexity, sharedsecret.ikeid.local, ...).
var settingsPolicyObjects = func() map[string]bool {
	m := map[string]bool{}
	for p := range settingsPolicyPaths {
		for i := strings.LastIndexByte(p, '.'); i > 0; i = strings.LastIndexByte(p[:i], '.') {
			m[p[:i]] = true
		}
	}
	return m
}()

// redactSecretValue blanks the value of secret key name: a string or number
// whole, every string and number inside any other container (wep_key holds its
// keys as keyN.value), and walks the settings objects with redactSettingsObject.
// Booleans are flags (psk, password) and null is absent, so both are kept, as is
// the numeric session_id SonicOS uses for a switch mirror.
func redactSecretValue(name string, val any, passthrough bool) (any, bool) {
	n := squash(name)
	if m, ok := val.(map[string]any); ok && settingsObjectNames[n] {
		return m, redactSettingsObject(n, m, passthrough)
	}
	switch x := val.(type) {
	case string:
		if x == "" {
			return x, false
		}
		return redactedText, true
	case json.Number, float64:
		if n == "sessionid" {
			return x, false
		}
		return redactedText, true
	case map[string]any, []any:
		return x, blankLeaves(x)
	}
	return val, false
}

// redactSettingsObject walks an object filed under a secret name, path being
// its squashed path from that name: a child is kept only at a path and of the
// type settingsPolicyPaths documents (an enum only with a documented value,
// and a string still judged as text), a documented intermediate object is
// walked, and every other child is dropped, key and value, because the key
// itself is the server's text and may be the secret its parent's name says.
func redactSettingsObject(path string, t map[string]any, passthrough bool) bool {
	changed := dropAssigningKeys(t)
	for k, val := range t {
		p := path + "." + squash(k)
		var c bool
		m, isMap := val.(map[string]any)
		switch {
		case isMap && settingsPolicyObjects[p]:
			c = redactSettingsObject(p, m, passthrough)
		case policyValueFits(p, val):
			t[k], c = redactNestedValue(val, passthrough)
		default:
			delete(t, k)
			c = true
		}
		changed = changed || c
	}
	return changed
}

// policyValueFits reports whether val has the type the schema documents for
// policy path p.
func policyValueFits(p string, val any) bool {
	if val == nil {
		// null is the schema's "unconfigured" (password.aging is x-nullable)
		return settingsPolicyPaths[p] != "" || settingsPolicyObjects[p]
	}
	switch settingsPolicyPaths[p] {
	case "number":
		switch val.(type) {
		case json.Number, float64:
			return true
		}
		return false
	case "boolean":
		_, ok := val.(bool)
		return ok
	case "string":
		_, ok := val.(string)
		return ok
	case "enum":
		s, ok := val.(string)
		return ok && settingsPolicyEnums[p][s]
	}
	return false
}

// blankSecret blanks a value judged secret by its position: a non-empty string
// or a number whole, a container's leaves; booleans and null carry nothing.
func blankSecret(val any) (any, bool) {
	switch x := val.(type) {
	case string:
		if x == "" {
			return x, false
		}
		return redactedText, true
	case json.Number, float64:
		return redactedText, true
	case map[string]any, []any:
		return x, blankLeaves(x)
	}
	return val, false
}

// blankLeaves replaces every non-empty string and every number in v.
func blankLeaves(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		changed = dropAssigningKeys(t)
		for k, val := range t {
			var c bool
			t[k], c = blankSecret(val)
			changed = changed || c
		}
	case []any:
		for i, val := range t {
			var c bool
			t[i], c = blankSecret(val)
			changed = changed || c
		}
	}
	return changed
}

// genericPassthroughNames are nsmCredentialFields entries that are ordinary
// words in prose; they count only as JSON keys on passthrough reads.
var genericPassthroughNames = map[string]bool{"key": true, "new": true, "old": true}

// containsSecretWord tests every identifier-like word in body with the same
// rules as JSON keys.
func containsSecretWord(body []byte) bool {
	for _, w := range secretWordSplit.Split(string(body), -1) {
		if w == "" {
			continue
		}
		lw := strings.ToLower(w)
		if isSchemaSecret(w, false) || (nsmCredentialFields[lw] && !genericPassthroughNames[lw]) {
			return true
		}
	}
	return false
}

var secretWordSplit = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// decodeJSONStream decodes every JSON value in body. ok is false when body is
// not entirely JSON values (text, HTML, or JSON followed by anything else).
func decodeJSONStream(body []byte) (values []any, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	for {
		var v any
		err := dec.Decode(&v)
		if err == io.EOF {
			return values, true
		}
		if err != nil {
			return nil, false
		}
		values = append(values, v)
	}
}

func isContainer(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

// textHasSecret applies the key rules to free text: every word, and every
// secret name or suffix found anywhere once separators are removed, so
// "auth-code" and "session id" count like the keys authCode and sessionId.
func textHasSecret(s string) bool {
	if containsSecretWord([]byte(s)) {
		return true
	}
	squashed := squash(s)
	for _, suffix := range secretKeySuffixes {
		if strings.Contains(squashed, suffix) {
			return true
		}
	}
	for _, names := range []map[string]bool{squashedSchemaNames, squashedCredentialNames} {
		for name := range names {
			if strings.Contains(squashed, name) {
				return true
			}
		}
	}
	return false
}

var (
	charEscape    = regexp.MustCompile(`\\u[0-9a-fA-F]{4}|\\x[0-9a-fA-F]{2}`)
	percentEscape = regexp.MustCompile(`%[0-9a-fA-F]{2}`)
)

// decodeLenient undoes \uXXXX and \xHH escapes, HTML entities and
// percent-encoding, each escape on its own (a malformed one elsewhere in the
// string does not stop the rest), repeated until nothing changes, so mixed and
// double encodings ("%5Cu0070assword", "pass&amp;#119;ord") decode too. ok is
// false when the text is still changing after maxDecodeRounds: a caller then
// treats it as unjudgeable.
func decodeLenient(s string) (string, bool) {
	for i := 0; i < maxDecodeRounds; i++ {
		d := charEscape.ReplaceAllStringFunc(s, func(m string) string {
			// UnquoteChar decodes the escape itself (no integer-to-rune
			// conversion to overflow); an unpaired surrogate is left as written.
			r, _, _, err := strconv.UnquoteChar(m, 0)
			if err != nil {
				return m
			}
			return string(r)
		})
		d = html.UnescapeString(d)
		d = percentEscape.ReplaceAllStringFunc(d, func(m string) string {
			b, err := hex.DecodeString(m[1:])
			if err != nil {
				return m
			}
			return string(b)
		})
		if d == s {
			return s, true
		}
		s = d
	}
	return s, false
}

const maxDecodeRounds = 16
