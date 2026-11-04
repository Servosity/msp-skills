// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// nerdio hand-written helpers kept when the shared credential kit replaced
// this connector's own holder and output chokepoint (handfixes.json
// credkit-wiring): the field-name rule a tenant-secret answer is read by
// (tenant-secrets-shown-never-stored), the markup walk it uses
// (markup-walk-never-fails), and what config.toml may keep
// (config-file-holds-no-credential). None of them holds a credential: the kit
// holds only what internal/credwire declares.

import (
	"encoding/xml"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// secretFieldPattern matches field names that carry credentials, read on the
// normalized name (normalizedFieldName: lowercase, letters and digits only, so
// clientSecret, client_secret, client-secret and CLIENT-SECRET are one name):
// OAuth access and refresh tokens, client secrets, passwords, and the generic
// credential names (private keys, API keys, pass phrases, communities), and
// the standard auth carriers (Authorization and Proxy-Authorization, the
// authenticate challenges, cookies, sessions, bearer and JWT values,
// signatures, assertions, one-time codes). It guards the config,
// request params, MCP arguments and any body that echoes a credential. The
// public half of a credential pair (clientId, accessKeyId) counts too. It
// deliberately over-matches ("token", "apikey", "session"); the only Nerdio
// Manager for MSP API request names it matches are real credentials (the
// provisioning link-tenant ARM and Graph access tokens) plus the client pair
// (TestNoNerdioSpecNameReadsAsACredential). Codex r1 (4.34.2 reprint) class
// holder-incomplete: "authorization" was not a credential name, so a response
// credential under it was neither held nor redacted. Codex r1 (4.34.2 review)
// class holder-incomplete: a PIN, passcode or verification code (often a
// number) is a credential too.
var secretFieldPattern = regexp.MustCompile(`(passw(or)?d|pass$|passhash|passphrase|secret|credential|token|apikey|privatekey|community|^key$|licensekey|^psk$|preshared|clientid|accesskeyid|authoriz|authenticat|^auth$|^xauth|cookie|session|bearer|jwt|signature|^sig$|assertion|saml|^otp$|^totp$|^mfa|^pin$|pincode|passcode|verificationcode)`)

// publicSecretLikeFields match the pattern but are data, not a credential
// (normalized names; nerdio handfix credential-names-match-nerdio-data).
// The Nerdio client ID is half of the client-credentials pair, so clientId
// stays a credential name. Exempt:
//   - token_type: always "Bearer" on the token response.
//   - token_expiry: the credential store's record of when the minted token
//     expires (a timestamp). Held, it withheld every output carrying a time
//     close to it (a feedback entry's own timestamp spelled it).
//   - token_url: the Entra token endpoint config.toml names (a setting, like
//     NERDIO_TOKEN_URL, which is never held). Held, every config.toml that
//     named it could not be saved again (the last check refused bytes
//     repeating a held value), so auth login and the token cached beside a
//     stored pair failed. Its userinfo password and credential-named query
//     values are still held (HoldURLCredentials).
var publicSecretLikeFields = map[string]bool{
	"tokentype":   true,
	"tokenexpiry": true,
	"tokenurl":    true,
}

// SecretFieldName reports a property, flag, parameter or header name that
// looks like a credential, in any spelling.
func SecretFieldName(name string) bool {
	n := normalizedFieldName(name)
	if publicSecretLikeFields[n] || !secretFieldPattern.MatchString(n) {
		return false
	}
	return !avdSessionVocabulary(n) && !nmmMFAStatusVocabulary(n)
}

// nmmMFAStatusVocabulary reports a name that starts with "mfa" and names a
// status, not a code (nerdio handfix credential-names-match-nerdio-data):
// NMM's only MFA surface is GET users mfaStatus, a registration status read
// (mfaStatus, mfaEnabled, mfaMethods), and no NMM request or response
// carries a one-time code. Held, those values ("Enabled", a method name)
// withheld whole outputs. A name with a code, OTP, PIN or seed word, or any
// other credential word after "mfa" (mfaToken, mfaSecret), stays a credential.
func nmmMFAStatusVocabulary(n string) bool {
	rest, ok := strings.CutPrefix(n, "mfa")
	if !ok {
		return false
	}
	for _, w := range []string{"code", "otp", "pin", "seed", "passcode"} {
		if strings.Contains(rest, w) {
			return false
		}
	}
	return !secretFieldPattern.MatchString(strings.ReplaceAll(rest, "session", "\x00"))
}

// avdSessionVocabulary reports a name whose only credential word is
// "session" (nerdio handfix credential-names-match-nerdio-data). The NMM
// Partner API authenticates with a bearer token only, never a web session,
// and Azure Virtual Desktop's own vocabulary is sessions: host pools have
// session hosts, user sessions (sessionId, sessionState, sessionHostName),
// session timeouts and session limits. Held, every one of those values (a
// host name, "Active", a count) withheld whole outputs of host-pool, host and
// workspace commands. A name with any other credential word beside it
// (sessionToken, sessionSecret, sessionCookie) and a session key stay
// credentials.
func avdSessionVocabulary(n string) bool {
	if !strings.Contains(n, "session") || strings.Contains(n, "sessionkey") {
		return false
	}
	return !secretFieldPattern.MatchString(strings.ReplaceAll(n, "session", "\x00"))
}

// normalizedFieldName lowercases ASCII letters and drops every separator
// (anything that is not a letter or digit), keeping non-ASCII bytes.
func normalizedFieldName(name string) string {
	b := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		switch c := name[i]; {
		case 'A' <= c && c <= 'Z':
			b = append(b, c+('a'-'A'))
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9', c >= 0x80:
			b = append(b, c)
		}
	}
	return string(b)
}

// isMaskValue reports a value that already hides a credential: a "****"
// mask, the kit's notice, and placeholder words such as the learn journal's
// "redacted" argv class.
func isMaskValue(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || strings.Trim(v, "*•") == "" || strings.HasPrefix(v, "[credkit: withheld") {
		return true
	}
	switch strings.ToLower(strings.Trim(v, "[]<>()")) {
	case "redacted", "masked", "hidden", "withheld":
		return true
	}
	return false
}

// authSchemes are the Authorization schemes whose value is a credential.
var authSchemes = []string{"bearer ", "basic ", "token ", "digest ", "negotiate ", "ntlm ", "apikey ", "sso-key "}

// URLWithoutCredentials returns raw with every credential it carries taken
// out (nerdio handfix: config-file-holds-no-credential): the userinfo, each
// query parameter whose name reads as a credential, and each whose value
// repeats one of creds (Echoes). changed reports a removal. A URL that does not parse and carries userinfo comes back "" (the
// caller must not keep any of it). The last check before config.toml is
// written (refuseCredentialInConfigFile) still refuses bytes that repeat a
// credential of the configuration in any form Echoes finds.
func URLWithoutCredentials(raw string, creds ...string) (clean string, changed bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return raw, false
	}
	u, err := url.Parse(s)
	if err != nil {
		authority := s
		if _, after, ok := strings.Cut(s, "://"); ok {
			authority = after
		}
		if i := strings.IndexAny(authority, "?#"); i >= 0 {
			authority = authority[:i]
		}
		if strings.Contains(authority, "@") || (len(creds) > 0 && Echoes(s, creds)) {
			return "", true
		}
		return raw, false
	}
	if u.User != nil {
		u.User = nil
		changed = true
	}
	if u.RawQuery != "" {
		var kept []string
		for _, pair := range strings.FieldsFunc(u.RawQuery, func(r rune) bool { return r == '&' || r == ';' }) {
			k, v, _ := strings.Cut(pair, "=")
			dk, dv := k, v
			if d, derr := url.QueryUnescape(k); derr == nil {
				dk = d
			}
			if d, derr := url.QueryUnescape(v); derr == nil {
				dv = d
			}
			if SecretFieldName(dk) || (v != "" && len(creds) > 0 && (Echoes(v, creds) || Echoes(dv, creds))) {
				changed = true
				continue
			}
			kept = append(kept, pair)
		}
		u.RawQuery = strings.Join(kept, "&")
	}
	if !changed {
		return raw, false
	}
	return u.String(), true
}

// embeddedParam finds the name=value pairs of a query string, cookie or
// parameter list; embeddedHeader a "Name: value" line. A tenant-secret answer
// that is not JSON is read with them (showTenantSecretsText).
var (
	embeddedParam  = regexp.MustCompile(`([A-Za-z0-9_.\-\[\]%]+)=("[^"]*"|[^&;,\s"'<>]*)`)
	embeddedHeader = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_\-]+)\s*:\s*(.+?)\s*$`)
)

// markupTreeBudget bounds the tokens one markup walk reads.
const markupTreeBudget = 200000

// walkMarkupTree reads markup as an element tree and passes to hold every
// text chunk (CDATA and entity-decoded text included) and attribute value
// under an element whose local name secretName accepts, at any depth, and
// every attribute value whose own name it accepts: a credential-named element
// marks its whole subtree, the way a credential-named JSON member marks every
// value under it (Codex review r1 class holder incomplete:
// <credentials><item>x</item></credentials> printed x, since the leaf
// "item" is no credential name). The decoder is lenient (unclosed and
// mismatched tags close, HTML entities decode); a malformed tail stops the
// walk, and what was read stays held beside the regex signals above.
func walkMarkupTree(s string, secretName func(string) bool, hold func(string)) {
	// Codex r2 (class holder incomplete, fixed after the stop rule): the
	// decoder below stops at its first error, so a malformed token before a
	// credential element (<1>, a stray '<', an unterminated attribute) left
	// everything after it unwalked. A lexer that cannot fail walks the whole
	// body first; the decoder's walk stays beside it as a second signal.
	walkMarkupLexer(s, secretName, hold)
	dec := xml.NewDecoder(strings.NewReader(s))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	var stack []bool // whether each open element is in a credential subtree
	for steps := 0; steps < markupTreeBudget; steps++ {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch e := tok.(type) {
		case xml.StartElement:
			secret := (len(stack) > 0 && stack[len(stack)-1]) || secretName(e.Name.Local)
			for _, a := range e.Attr {
				if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
					continue // a namespace declaration names a schema, it carries no data
				}
				if secret || secretName(a.Name.Local) {
					hold(a.Value)
				}
			}
			stack = append(stack, secret)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] {
				hold(string(e))
			}
		}
	}
}

// walkMarkupLexer walks markup with a lexer that never fails: start tags,
// end tags, comments and declarations, CDATA and text, nothing else. A
// credential-named start tag (or any tag inside one) marks its attributes and
// every text up to its end tag; a tag that never closes marks the rest of the
// body (fail closed). A '<' ends a tag name, and an unterminated quote ends at
// the tag's next '>', so neither hides the markup after it. An end tag closes the nearest open element of its name
// and everything opened after it; an end tag with no open match is ignored.
// Text and attribute values are HTML-entity decoded. Bounded by
// markupTreeBudget tags.
func walkMarkupLexer(s string, secretName func(string) bool, hold func(string)) {
	type open struct {
		name   string
		secret bool
	}
	var stack []open
	inSecret := func() bool { return len(stack) > 0 && stack[len(stack)-1].secret }
	text := func(v string) {
		if inSecret() {
			if v = strings.TrimSpace(html.UnescapeString(v)); v != "" {
				hold(v)
			}
		}
	}
	nameEnd := func(i int) int {
		for i < len(s) && !strings.ContainsRune(" \t\r\n/>=<", rune(s[i])) { // a '<' starts the next tag
			i++
		}
		return i
	}
	for i, tags := 0, 0; i < len(s) && tags < markupTreeBudget; {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			text(s[i:])
			return
		}
		text(s[i : i+lt])
		i += lt
		tags++
		switch {
		case strings.HasPrefix(s[i:], "<![CDATA["):
			end := strings.Index(s[i+9:], "]]>")
			if end < 0 {
				text(s[i+9:])
				return
			}
			if inSecret() {
				if v := strings.TrimSpace(s[i+9 : i+9+end]); v != "" {
					hold(v)
				}
			}
			i += 9 + end + 3
		case strings.HasPrefix(s[i:], "<!--"):
			end := strings.Index(s[i+4:], "-->")
			if end < 0 {
				return
			}
			i += 4 + end + 3
		case strings.HasPrefix(s[i:], "<!") || strings.HasPrefix(s[i:], "<?"):
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				return
			}
			i += end + 1
		case strings.HasPrefix(s[i:], "</"):
			j := nameEnd(i + 2)
			name := strings.ToLower(s[i+2 : j])
			for k := len(stack) - 1; k >= 0; k-- {
				if stack[k].name == name {
					stack = stack[:k]
					break
				}
			}
			end := strings.IndexByte(s[j:], '>')
			if end < 0 {
				return
			}
			i = j + end + 1
		default:
			j := nameEnd(i + 1)
			if j == i+1 {
				text("<") // a lone '<' is text
				i++
				continue
			}
			name := s[i+1 : j]
			secret := inSecret() || secretName(name)
			// attributes up to the tag's '>' (a quoted value may hold '>');
			// an unterminated quote runs to the end of the body
			k := j
			selfClose := false
			for k < len(s) && s[k] != '>' {
				switch c := s[k]; {
				case c == '/' && k+1 < len(s) && s[k+1] == '>':
					selfClose = true
					k++
				case c == ' ' || c == '\t' || c == '\r' || c == '\n':
					k++
				default:
					an := nameEnd(k)
					if an == k {
						k++
						continue
					}
					attr := s[k:an]
					k = an
					for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
						k++
					}
					if k >= len(s) || s[k] != '=' {
						continue
					}
					k++
					for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
						k++
					}
					var val string
					if k < len(s) && (s[k] == '"' || s[k] == '\'') {
						q := s[k]
						end := strings.IndexByte(s[k+1:], q)
						if end < 0 {
							// An unterminated quote ends at the tag's next '>', so
							// the markup after it is still walked.
							if gt := strings.IndexByte(s[k+1:], '>'); gt >= 0 {
								val, k = s[k+1:k+1+gt], k+1+gt
							} else {
								val, k = s[k+1:], len(s)
							}
						} else {
							val, k = s[k+1:k+1+end], k+1+end+1
						}
					} else {
						vs := k
						for k < len(s) && !strings.ContainsRune(" \t\r\n>", rune(s[k])) {
							k++
						}
						val = s[vs:k]
					}
					if lower := strings.ToLower(attr); lower == "xmlns" || strings.HasPrefix(lower, "xmlns:") {
						continue // a namespace declaration names a schema, it carries no data (as the decoder walk)
					}
					if secret || secretName(attr) {
						if v := strings.TrimSpace(html.UnescapeString(val)); v != "" {
							hold(v)
						}
					}
				}
			}
			if !selfClose {
				stack = append(stack, open{name: strings.ToLower(name), secret: secret})
			}
			i = k + 1
		}
	}
}

// holdEmbeddedCredentials holds the credentials a string carries under any
// name (Codex r1 4.34.2 class holder-incomplete): the userinfo password and
// credential-named query values of every URL in it, the value of every
// credential-named parameter of a query string, cookie or parameter list and
// of every credential-named "Name: value" line, every auth-scheme value and
// JWT in it, and the credential members of JSON that starts inside it.

// CredentialShaped reports a value that is a credential by its shape whatever
// it is named: an auth-scheme value ("Bearer x", "Basic x", ...) or a JWT.
func CredentialShaped(v string) bool {
	t := strings.TrimSpace(v)
	for _, scheme := range authSchemes {
		if len(t) > len(scheme) && strings.EqualFold(t[:len(scheme)], scheme) && strings.TrimSpace(t[len(scheme):]) != "" {
			return true
		}
	}
	return LooksLikeJWT(t)
}
