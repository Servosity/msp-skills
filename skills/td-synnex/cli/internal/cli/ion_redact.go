// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: redact credentials ION returns inside customer, order and provisioning payloads.

package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"html"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const redacted = "[redacted]"

// maxNestedJSON bounds how many JSON-in-a-string layers are decoded. A string
// still holding JSON past it is withheld whole.
const maxNestedJSON = 8

// normalizeKey lowercases a field name and drops separators, so adminPassword,
// admin_password, Admin-Password, admin.password and "API\u00a0key" compare
// equal. An escaped name (n%61me, admin%50assword) is decoded first.
func normalizeKey(k string) string {
	if strings.ContainsAny(k, "%&\\") {
		forms := textDecodings(k)
		k = forms[len(forms)-1]
	}
	return strings.Map(func(r rune) rune {
		switch r {
		case '_', '-', '.', ' ', ':', '$', '@', '\'', '/':
			return -1 // "code d'accès" and "p/w" join like the other separators
		}
		return r
	}, strings.ToLower(canonicalText(k)))
}

// canonicalText folds look-alike characters into the ASCII forms the rules
// match: NFKC first (fullwidth letters, ligatures, compatibility forms), then
// default-ignorable characters dropped (zero-width and other format
// characters, variation selectors, Hangul fillers), every Unicode space (NBSP,
// tab) and run of spaces to one space, every dash and minus to '-',
// typographic apostrophes to ', colon and equals look-alikes to ':' and '=',
// and arrows to "->". Line breaks are kept.
func canonicalText(s string) string {
	out := make([]rune, 0, len(s))
	space := false
	// Invisible characters go before NFKC, so a letter and its accent split by
	// one still compose ("cle<ZWJ>\u0301" is "clé"); this pass serves name
	// rules, which normalize once (adversarial round 18 and Codex round 20,
	// P1). An accent left uncomposed after that has no letter to sit on and is
	// an apostrophe look-alike ("code d\u0301accès").
	s = strings.Map(func(r rune) rune {
		if defaultIgnorable(r) {
			return -1
		}
		return r
	}, s)
	for _, r := range norm.NFKC.String(s) {
		switch {
		case defaultIgnorable(r):
			continue
		case r == '\u2018' || r == '\u2019' || r == '\u201B' || r == '\u02BC' || r == '\u2032' || r == '\uFF07' || r == '`' ||
			r == '\u02CA' || r == '\u02CB':
			r = '\'' // typographic apostrophes and accent look-alikes, as in "code d’accès"
		case r == '\u0300' || r == '\u0301':
			// NFKC writes a spacing accent (´, Greek oxia or tonos) as a space
			// and this mark; either way it is one apostrophe (adversarial round 19).
			if n := len(out); n > 0 && out[n-1] == ' ' {
				out = out[:n-1]
			}
			r = '\''
		case r == '\u2215' || r == '\u2044' || r == '\u29F8' || r == '\u2571':
			r = '/' // slash look-alikes, as in "p∕w"
		case r == '\n' || r == '\r':
		case unicode.IsSpace(r):
			if !space {
				out = append(out, ' ')
			}
			space = true
			continue
		case unicode.Is(unicode.Pd, r) || r == '\u2212':
			r = '-'
		case r == '\uFF1A' || r == '\uFE55' || r == '\uFE13' || r == '\u2236' || r == '\uA789' || r == '\u02F8':
			r = ':'
		case r == '\uFF1D' || r == '\uFE66' || r == '\uA78A':
			r = '='
		case (r >= 0x2190 && r <= 0x21FF) || (r >= 0x2794 && r <= 0x27BF) || (r >= 0x27F0 && r <= 0x27FF) || (r >= 0x2900 && r <= 0x297F) ||
			(r >= 0x2B00 && r <= 0x2BFF) || (r >= 0x1F800 && r <= 0x1F8FF) || (r >= 0x1F446 && r <= 0x1F449) || r == 0x25B6 || r == 0x25BA || r == 0x00BB || r == 0x203A:
			out = append(out, '-', '>')
			space = false
			continue
		}
		space = false
		out = append(out, r)
	}
	return string(out)
}

// defaultIgnorable reports invisible characters: format characters, every
// variation selector (VS1-VS256, the Mongolian ones), the combining grapheme
// joiner and the Hangul fillers (adversarial round 14, P1: VS17-VS256 were
// judged as letters).
func defaultIgnorable(r rune) bool {
	return unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Variation_Selector, r) || unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r)
}

// secretWords make a field secret wherever they appear in its normalized name:
// passwords, initialPasswordValue, clientSecret, secretAccessKey,
// userCredentials, privateKey, apiKey, accessKey.
var secretWords = []string{"password", "passwd", "passphrase", "passcode", "secret", "credential", "privatekey", "apikey", "accesskey", "authorization",
	"accountkey", "sharedkey", "authkey", "masterkey", "signingkey", "recoverykey", "encryptionkey", "sessionkey", "subscriptionkey",
	"accesscode", "doorcode", "mfacode", "recoverycode", "verificationcode", "securitycode", "backupcode", "licensekey", "productkey", "activationcode", "amzsignature", "googsignature",
	// Translations (German, French, Spanish, Portuguese, Dutch, Italian); the same words open a label in textLabelWords and are
	// listed in governance.md. A plural keeps its stem (Passwörter has its own, the umlaut changes it).
	"passwort", "passwört", "kennwort", "kennwört", "zugangsdaten", "anmeldedaten", "zugangscode", "schlüssel", "schluessel", "geheimnis",
	"motdepasse", "identifiant", "jeton", "codedacc", "cléapi", "cleapi", "clésecr", "clesecr",
	"contrasena", "contraseña", "credencial", "codigodeacceso", "códigodeacceso", "llave",
	"senha", "credenciais", "chave", "segredo",
	"wachtwoord", "geheim", "credenziali", "chiave", "segreto"}

// secretExact are names that are secret only as the whole name.
var secretExact = map[string]bool{"clave": true, "claves": true, "mdp": true, "pins": true, "pwds": true, "otp": true, "azurecustomerinfo": true, "cookie": true, "setcookie": true,
	"creds": true, "cred": true, "pw": true, "pswd": true, "psw": true, "pword": true, "passkey": true, "passkeys": true}

// paginationToken matches the token fields ION pages with, which must survive.
var paginationToken = regexp.MustCompile(`(page|next|prev|previous|continuation|cursor|sync|resume)token$`)

func isSecretKey(k string) bool {
	forms := []string{k}
	if strings.ContainsAny(k, "%&\\") {
		forms = textForms(k) // an encoded name (admin%50assword) is judged decoded
	}
	for _, f := range forms {
		c := strings.TrimSpace(canonicalText(f))
		if n := normalizeKey(f); secretByWord(n) || textSuffixAtBoundary.MatchString(c) || secretFirstWord.MatchString(strings.ToLower(c)) {
			return true
		}
	}
	return false
}

// secretFirst is a PIN or one-time code named first, on a normalized name
// (pincode, pinnumber, otpvalue). secretFirstWord judges the lowercased raw
// name, where a separator after PIN or OTP also counts ("PIN (4 digits)",
// pin_hint, "PIN value"). PINNACLE, PinPoint, pinType and otpEnabled are not
// credentials.
var (
	secretFirst     = regexp.MustCompile(`^(pin|otp)(code|number|value|secret|digits|seed|key|uri|hash|block|token)`)
	secretFirstWord = regexp.MustCompile(`^\s*(?:pin|otp)(?:$|[^a-z]|code|number|value|secret|digits|seed|key|uri|hash|block|token)`)
)

// secretByWord is isSecretKey without the short-suffix rule.
// secretJoined is a common account word run together with a short secret word
// (adminpass, rootpw, dbpass, wifipass, userpin), which the camelCase rule
// cannot see once lowercased; compass and bypass are not in it.
var secretJoined = regexp.MustCompile(`^(admin|root|user|db|wifi|wlan|temp|tmp|default|initial|new|old|login|account|sys|svc|service|master|local|ftp|smtp|vpn|domain|tenant|door|alarm|sim|phone)(pass|pw|pwd|pin)s?\d*$`)

func secretByWord(n string) bool {
	if secretExact[n] || secretFirst.MatchString(n) || secretJoined.MatchString(n) {
		return true
	}
	for _, w := range secretWords {
		if strings.Contains(n, w) {
			return true
		}
	}
	return strings.Contains(n, "token") && !paginationToken.MatchString(n)
}

// Credential text is judged on the whole string, never per line or per cell:
// a string, or a body that is not JSON, holding any credential marker is
// withheld whole. How wide the net is depends on the field.
//
// Free text (notes, descriptions, activity details, report CSV, any field
// not listed below) is withheld when it mentions a credential word
// (textLabelWords: password, passcode, PIN, OTP, secret, token, credential,
// API/private/access key, recovery or verification code and translations) or
// a secret name such as tenantAdminPassword, anywhere (hasCredentialMention).
// No separator grammar: every way of writing a label is covered, and a note
// or report CSV that merely mentions such a word is withheld too.
//
// Name, ID and address fields (identityKey: customerName,
// customerOrganization, displayName, sku, mfgPartNumber, street, ...) use a
// narrow label rule so catalog names survive:
// the credential word may be followed only by punctuation, markup, modifier
// words or a "for/of ..." phrase, then ":", "=" or "is/was/are"; dashes do
// not count. "Keeper Password Manager: Business", "Duo Hardware Token - D-100"
// and CFQ7TTC0LHPW:0001 are kept.
//
// In every field: a secret name attached to a value ("tempPwd=x"), a
// secret-named tag or query parameter, a URL or address carrying userinfo
// ("https://x@host", "admin:x@host"), a bearer token, and in multi-line text a
// secret-named table header, a name,value table naming a secret, or a short
// cell naming one ("Admin PIN"). Each is looked for in the raw text and its
// HTML-, percent- and \u-decoded forms. A credential written with no
// credential word near it ("the code is in the email") is not recognized.
var (
	textLabel      = regexp.MustCompile(`(?i)\b(?:` + textLabelWords + `)\b` + textLabelGap + `(?:` + textNarrowSep + `)`)
	textWordAt     = regexp.MustCompile(`(?i)\b(?:` + textLabelWords + `)\b`)
	textToken      = regexp.MustCompile(`[\p{L}\p{N}_.\-]+`)
	textAssignment = regexp.MustCompile(`([\p{L}\p{N}_.\-]+)["']?\s*(?:` + textWideSep + `)`)
	textNameAssign = regexp.MustCompile(`([\p{L}\p{N}_.\-]+)["']?\s*(?:` + textNarrowSep + `)`)
	textTag        = regexp.MustCompile(`<\s*/?\s*([\p{L}\p{N}_.:\-]+)`)
	textQueryParam = regexp.MustCompile(`[?&;]([\p{L}\p{N}_.\-]+)=`)
	textUserinfo   = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^/\s?#@]+@`)
	// \p{M}: a user name in a script with combining marks (Devanagari कि)
	// keeps them after NFKC (Codex round 15, P1).
	textBareUserinfo = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{M}\p{N}._%+-])(\p{L}[\p{L}\p{M}\p{N}._%+-]*):[^\s@/]+@(?:[\p{L}\p{M}\p{N}-]+|\[[0-9a-f:.]+\])`)
	textBearer       = regexp.MustCompile(`(?i)\bbearer[\s:]+[a-z0-9._~+/=-]+`)
	textLines        = regexp.MustCompile(`\r\n|\r|\n`)
	textFieldSep     = regexp.MustCompile(`[\r\n,;\t|]`)
	textFieldName    = regexp.MustCompile(`^\p{L}[\p{L}\p{N} _.\-]*$`)
	textFieldNoise   = regexp.MustCompile(`[^\p{L}\p{N} _.\-]+`)
	textPercent      = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)
	textUnicode      = regexp.MustCompile(`\\u[0-9A-Fa-f]{4}`)
	textHexEscape    = regexp.MustCompile(`\\x[0-9A-Fa-f]{2}`)
	// textSurrogatePair is a \uD800-\uDBFF high half followed by a
	// \uDC00-\uDFFF low half: one rune outside the BMP.
	textSurrogatePair = regexp.MustCompile(`\\u[dD][89abAB][0-9A-Fa-f]{2}\\u[dD][c-fC-F][0-9A-Fa-f]{2}`)
)

// textLabelWords are words that, opening a label, name a credential. Whole
// words only, so Passwordless and Secretary are not labels.
const textLabelWords = `pass(?:word|wd|phrase|code)s?|pass[ _-]?(?:word|phrase|code)s?|pwd|pins?|otp|secrets?|credentials?|tokens?|` +
	`(?:api|private|access|secret|account|shared|auth|master|signing|recovery|encryption|session|subscription|license|product)[ _-]?keys?|` +
	`(?:recovery|verification|security|mfa|2fa|backup|one[ -]time|auth|activation)[ _-]?codes?|` +
	`creds?|p/w|pswd|psw|pword|passkeys?|(?:door|gate|alarm|access|entry|lock|safe|unlock)[ _-]?codes?|` +
	`passw(?:o|ö)rt(?:er)?|kennw(?:o|ö)rt(?:er)?|zugangsdaten|anmeldedaten|zugangscode|api[ _-]?schl(?:ü|ue)ssel|geheimnis|` +
	`mot de passe|mdp|identifiants?|jeton|code d'acc(?:è|e)s|cl(?:é|e) api|cl(?:é|e) secr(?:è|e)te|` +
	`contrase(?:n|ñ)as?|claves?|credenciales|c(?:ó|o)digo de acceso|llave api|senhas?|credenciais|chave api|segredo|` +
	`wachtwoord(?:en)?|geheim|credenziali|chiave api|segreto`

// textLabelGap is what the narrow (name-field) rule lets sit between a
// credential word and its separator: punctuation, markup and entities,
// modifier words ("PIN number", "secret (value)"), and one "for/of/to ..."
// phrase. Any other word ends the label.
const textLabelGap = `(?:[^\p{L}\p{N}:=\n]|<[^>\n]{0,24}>|&#?[a-z0-9]+;|\b(?:number|num|no|value|values|temporary|temp|initial|new|old|default|admin|user|field|below|here)\b)*` +
	`(?:\b(?:for|of|to|on|at|from|in)\b[^:=\n]{0,40}?)?`

// textNarrowSep ends a label in a name field: ":", "=" or "is/was/are".
// textWideSep, for free text, also takes dashes and arrows; a colon between
// digits (10:00) is not a separator.
const (
	textNarrowSep = `[:=\x{FF1A}\x{FF1D}]|\s(?i:is|was|are)\b`
	textWideSep   = `(?:^|[^\d\r\n]):|\d:(?:\D|$)|[=\x{FF1A}\x{FF1D}]|\s(?i:is|was|are)\b|[\s\x{00A0}][-\x{2013}\x{2014}](?:[^\d]|$|\d+(?:[^\d.,]|$)|\d+[.,]\d{3,})|[\x{2013}\x{2014}]|->|\x{2192}`
)

// identityKeySuffix names fields that hold a name, an ID or a code rather
// than prose; their strings get the narrow label rule.
var identityKeySuffix = regexp.MustCompile(`(name|names|id|ids|title|sku|skus|label|number|key|type|status|email|domain|url|uri|currency|country|region|` +
	`organization|org|company|vendor|manufacturer|publisher|brand|edition|plan|offer|product|program|category|tier|` +
	`address|street|street\d|addressline\d|city|state|zip|postalcode|phone)$`)

func identityKey(k string) bool { return identityKeySuffix.MatchString(normalizeKey(k)) }

// queryCredentialNames are query parameters that carry a credential although
// isSecretKey does not name them.
var queryCredentialNames = map[string]bool{"key": true, "sig": true, "signature": true, "code": true,
	"x-amz-signature": true, "x-amz-credential": true, "x-goog-signature": true, "x-goog-credential": true}

// textSuffixAtBoundary is a short secret suffix that starts a word in the raw
// name: pin, Admin PIN, admin_pw, adminPass, tempPwd. In a run of capitals
// or digits it does not (the SKU IDs CFQ7TTC0LHPW, CFQ7TTC0L1PW).
var textSuffixAtBoundary = regexp.MustCompile(`(?:(?:^|[^A-Za-z0-9])[Pp](?:ass|ASS|wd|WD|w|W|in|IN)|[a-z]P(?:ass|ASS|wd|WD|w|W|in|IN))\d*$`)

// textAddressLabel is a whole word that precedes a colon and an address
// without being a login (mailto:jdoe@contoso.com, Email:jdoe@contoso.com).
// It is removed in one pass before the userinfo scan, so sip:alice:pw@host is
// still judged; svc-mail: is part of a login name and is not removed.
var textAddressLabel = regexp.MustCompile(`(^|[^\p{L}\p{N}._%+-])(?i:mailto|sips?|tel|xmpp|e-?mail|mail|contact|owner|from|to|cc|bcc|reply-to):`)

// textDottedUserinfo is user:password@host.domain where the user may start
// with a digit (1234:x@host.example); a dotless host needs a letter-led user,
// so a time such as 10:30@Contoso is not a login.
var textDottedUserinfo = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{M}\p{N}._%+-])([\p{L}\p{N}][\p{L}\p{M}\p{N}._%+-]*):[^\s@/]+@(?:[\p{L}\p{M}\p{N}-]+\.[\p{L}\p{M}\p{N}.-]+|\[[0-9a-f.]*:[0-9a-f:.]*\]|\[\d{1,3}(?:\.\d{1,3}){3}\])`) // an IP literal: 10:30@[cafe] is no login

// textSIPLogin is a SIP or SIPS URI carrying user:password, whatever the user
// or host looks like (sip:1234:pw@[::1]; Codex round 28, P1).
var textSIPLogin = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}._%+-])sips?:[^\s@:]+:[^\s@]+@`) // the user and password may hold '/' (sip:1234/5:a/b@pbx; adversarial rounds 27 and 28)

// hasBareUserinfo reports user:password@host after address labels are
// removed (linear: one replace, two scans).
func hasBareUserinfo(s string) bool {
	if textSIPLogin.MatchString(s) {
		return true
	}
	s = textAddressLabel.ReplaceAllString(s, "$1 ")
	return textBareUserinfo.MatchString(s) || textDottedUserinfo.MatchString(s)
}

// textSuffixSeparated is a short secret suffix set off by a separator or
// standing alone: pin, Admin PIN, admin_pw, temp-pwd (not adminPass, LastPass).
var textSuffixSeparated = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:pass|pwd|pw|pin)\d*$`)

// textNameIsSecret judges a name found in text. A name that starts with a
// digit is a product (1Password), not a field, and a short secret suffix
// counts only where it starts a word.
func textNameIsSecret(name string) bool {
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		return false
	}
	return secretByWord(normalizeKey(name)) || textSuffixAtBoundary.MatchString(name) || secretFirstWord.MatchString(strings.ToLower(name))
}

// textForms returns s, every distinct decoding of it up to a fixed point (so
// any depth of encoding is seen), and the canonicalText of each. Percent
// escapes decode to bytes, so %C3%B1 is ñ. Every decoding step shortens the
// text, so the loop ends; textDecodeLimit only stops a pathological input,
// whose last form is then the redacted marker (withheld, fail closed).
func textForms(s string) []string {
	decoded := textDecodings(s)
	seen := map[string]bool{}
	var forms []string
	add := func(f string) {
		if !seen[f] {
			seen[f] = true
			forms = append(forms, f)
		}
	}
	for _, f := range decoded {
		add(f)
		add(canonicalText(f))
	}
	// NFKC can reveal escapes (％３Ａ); only the fully decoded form is folded
	// and decoded again, so the work stays linear in the encoding depth.
	for _, f := range textDecodings(canonicalText(decoded[len(decoded)-1])) {
		add(f)
		add(canonicalText(f))
	}
	return forms
}

// textDecodeLimit bounds the escape depth decoded, and with it the number of
// forms judged (each form is judged over the full string, so the cost is
// depth x length). Real data is encoded one to three levels deep; a string
// nested deeper is withheld (fail closed) instead of judged at every level.
// Adversarial round 13, P1: at 64 levels an 8 KB note took 2.2 s.
const textDecodeLimit = 8

func textDecodings(s string) []string {
	forms := []string{s}
	for cur, i := s, 0; ; i++ {
		if i == textDecodeLimit {
			return append(forms, "password: "+redacted)
		}
		// One level of backslash escaping per pass: a JSON string nested in
		// a JSON string carries \\u0070 for \u0070.
		next := strings.ReplaceAll(html.UnescapeString(cur), `\\`, `\`)
		next = textPercent.ReplaceAllStringFunc(next, func(m string) string {
			b, err := hex.DecodeString(m[1:])
			if err != nil {
				return m
			}
			return string(b)
		})
		next = decodeUnicodeEscapes(next)
		if next == cur {
			break
		}
		forms, cur = append(forms, next), next
	}
	// An escape the decoder could not consume is judged as credential text:
	// what it hides is unknown, so the string is withheld (fail closed).
	// Unreachable while every escape form above has a decoder; kept so a
	// new escape regex fails closed until its decoder exists.
	if last := forms[len(forms)-1]; textUnicode.MatchString(last) || textPercent.MatchString(last) || textHexEscape.MatchString(last) {
		return append(forms, "password: "+redacted)
	}
	return forms
}

// decodeUnicodeEscapes consumes every \uXXXX and \xXX escape: a surrogate
// pair becomes its rune (\uD835\uDC29 is a mathematical bold p, which NFKC
// folds to p), a \xXX escape its byte. A lone surrogate half has no
// character to become, so it is left in place and the string fails closed
// in textDecodings. strconv.Unquote refused each half of a pair and left the
// escape in place, so an encoded credential label passed the rules unjudged.
func decodeUnicodeEscapes(s string) string {
	s = textSurrogatePair.ReplaceAllStringFunc(s, func(m string) string {
		return string(utf16.DecodeRune(hexRune(m[2:6]), hexRune(m[8:12])))
	})
	s = textUnicode.ReplaceAllStringFunc(s, func(m string) string {
		r := hexRune(m[2:])
		if utf16.IsSurrogate(r) {
			return m // a lone half: undecodable, withheld by the caller
		}
		return string(r)
	})
	return textHexEscape.ReplaceAllStringFunc(s, func(m string) string {
		b, _ := hex.DecodeString(m[2:])
		return string(b)
	})
}

// hexRune reads the four hex digits the escape regexes matched as one UTF-16
// unit, widening byte by byte so no integer conversion can overflow.
func hexRune(h string) rune {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 2 {
		return utf8.RuneError
	}
	return rune(b[0])<<8 | rune(b[1])
}

// hasSecretText reports credential text in free text (narrow false) or in a
// name or ID field (narrow true).
func hasSecretText(s string, narrow bool) bool {
	for _, f := range textForms(s) {
		if hasSecretMarker(f, narrow) {
			return true
		}
	}
	return false
}

func hasSecretMarker(s string, narrow bool) bool {
	if textUserinfo.MatchString(s) || textBearer.MatchString(s) || textLabel.MatchString(s) {
		return true
	}
	if hasBareUserinfo(s) {
		return true
	}
	for _, m := range textTag.FindAllStringSubmatch(s, -1) {
		if textNameIsSecret(m[1]) {
			return true
		}
	}
	if narrow {
		// A name field: "name=value" counts for any secret name; with ":" or
		// "is" the name must end in its secret word (Secretary and the slug ID
		// KeeperPasswordManager-smp:0001 do not), and a short suffix name
		// (Proton Pass: Business, ACRN:Tier9:PW:M) counts only with "=".
		for _, m := range textNameAssign.FindAllStringSubmatch(s, -1) {
			name, n := m[1], normalizeKey(m[1])
			if !textNameIsSecret(name) {
				continue
			}
			if strings.HasSuffix(strings.TrimSpace(m[0]), "=") || (secretByWord(n) && secretCellEnd.MatchString(n)) {
				return true
			}
		}
	} else {
		for _, m := range textAssignment.FindAllStringSubmatch(s, -1) {
			if textNameIsSecret(m[1]) {
				return true
			}
		}
	}
	for _, m := range textQueryParam.FindAllStringSubmatch(s, -1) {
		if textNameIsSecret(m[1]) || queryCredentialNames[strings.ToLower(m[1])] {
			return true
		}
	}
	table := strings.ContainsAny(s, "\r\n")
	if !narrow && hasCredentialMention(s) {
		return true
	}
	return table && hasSecretTableField(s)
}

// hasCredentialMention is the free-text rule: a credential word
// (textLabelWords) or a secret name (tenantAdminPassword) anywhere withholds
// the string. No separator grammar is needed, so no way of writing a label
// gets past it; free text that merely mentions a credential word is withheld
// too.
func hasCredentialMention(s string) bool {
	if textWordAt.MatchString(s) {
		return true
	}
	for _, loc := range textToken.FindAllStringIndex(s, -1) {
		if textNameIsSecret(s[loc[0]:loc[1]]) {
			return true
		}
	}
	return false
}

// hasSecretTableField judges multi-line text as a table: a header cell naming
// a secret, a name,value table naming one, or a short cell naming one.
func hasSecretTableField(s string) bool {
	for _, line := range headerCandidates(s) {
		for _, field := range textFieldSep.Split(line, -1) {
			if fieldIsSecret(field, true) {
				return true
			}
		}
	}
	header := textFieldSep.Split(headerLine(s), -1)
	nameCol, valueCol := false, false
	for _, field := range header {
		n := normalizeKey(strings.Trim(field, " \"'"))
		nameCol = nameCol || attributeNameKeys[n]
		valueCol = valueCol || isAttributeValueKey(n)
	}
	// A name,value table carries attribute pairs, so its cells are judged as
	// names the way redactAttributePair judges JSON pairs (adminPass counts).
	pairs := nameCol && valueCol
	for _, field := range textFieldSep.Split(s, -1) {
		if fieldIsSecret(field, false) {
			return true
		}
		if f := cleanField(field); pairs && textFieldName.MatchString(f) && textNameIsSecret(f) {
			return true
		}
	}
	return false
}

// secretCellEnd is how a data cell that names a credential ends: a secret
// word (Admin PIN, tenant admin password), or token/key/code after a word
// that makes it one (access token, API key, recovery code). "Text Input
// Tokens" and "Duo Hardware Token" are product cells, not names.
var secretCellEnd = regexp.MustCompile(`(password|passwd|passphrase|passcode|secret|credential|pwd|pin|otp)s?\d*$|` +
	`(access|refresh|id|auth|bearer|session|api|private|account|shared|master|signing|recovery|encryption|license|product|activation|client|secret|verification|security|backup)(token|key|code)s?\d*$`)

// fieldIsSecret judges one separator-delimited field. A header field is judged
// like a JSON key (any secret word anywhere); any other
// field only when it is label-like (up to three words) and secretCellEnd
// matches, so "Azure Key Vault Secrets" or "Secret Renewal" does not withhold
// a whole report while "name,value\npassword,x" and "Admin PIN" still do.
func fieldIsSecret(field string, header bool) bool {
	field = cleanField(field)
	if !textFieldName.MatchString(field) {
		return false
	}
	if header {
		return headerCellIsSecret(field) // a header cell naming a secret marks its column, however long
	}
	return textNameIsSecret(field) && dataCellIsSecret(field)
}

// headerCellIsSecret judges a header cell word by word as well, so a short
// secret word anywhere in it counts ("Pwd (temp)", "Door PIN set by tech",
// "PINs").
func headerCellIsSecret(field string) bool {
	if textNameIsSecret(field) {
		return true
	}
	for _, w := range strings.Fields(field) {
		if textNameIsSecret(w) || textNameIsSecret(strings.TrimSuffix(strings.TrimSuffix(w, "s"), "S")) {
			return true
		}
	}
	return false
}

// textNumericCell is a cell holding a number, amount or date: a line with one
// is a data row, not a header.
var textNumericCell = regexp.MustCompile(`^[-+$\x{20AC}\x{00A3}]?\d[\d.,:/ T+-]*%?$`)

// headerCandidates are the lines that may be a table header: the first
// non-empty line (a one-column export), the first line with a field
// separator, and among the first three separated lines any with no numeric
// cell (a title such as "New starters, March 2026" may sit above the real
// header). A names-only table whose first rows mention a credential word
// (Duo Hardware Token) cannot be told from a header, so it is withheld.
func headerCandidates(s string) []string {
	var out []string
	separated := 0
	for _, line := range textLines.Split(s, -1) {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(strings.ToLower(t), "sep=") {
			continue
		}
		if len(out) == 0 && !strings.ContainsAny(t, ",;\t|") {
			out = append(out, t)
			continue
		}
		if !strings.ContainsAny(t, ",;\t|") {
			continue
		}
		if separated++; separated > 3 {
			break
		}
		numeric := false
		for _, c := range textFieldSep.Split(t, -1) {
			if textNumericCell.MatchString(strings.Trim(strings.TrimSpace(c), `"'`)) {
				numeric = true
				break
			}
		}
		if len(out) == 0 || !numeric {
			out = append(out, t)
		}
	}
	return out
}

func dataCellIsSecret(field string) bool {
	if len(strings.Fields(field)) > 3 || !secretCellEnd.MatchString(normalizeKey(field)) {
		return false
	}
	// A data cell joined to a short suffix is more likely a product (LastPass)
	// than a credential name; a header cell (above) is still judged.
	return secretByWord(normalizeKey(field)) || textSuffixSeparated.MatchString(field)
}

// cleanField keeps a table cell's letters, digits, spaces and name
// punctuation, so "Password (temp)", "Password*" and "Contraseña:" are judged
// as Password temp, Password and Contraseña.
func cleanField(field string) string {
	return strings.Join(strings.Fields(textFieldNoise.ReplaceAllString(canonicalText(field), " ")), " ")
}

// headerLine is the first non-empty line that has a field separator,
// skipping an Excel "sep=," line and a title line ("New starters") above it.
func headerLine(s string) string {
	first := ""
	for _, line := range textLines.Split(s, -1) {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(strings.ToLower(t), "sep=") {
			continue
		}
		if first == "" {
			first = t
		}
		if strings.ContainsAny(t, ",;\t|") {
			return t
		}
	}
	return first
}

// redactSecretsJSON returns data with secret-named fields, the values of
// secret-named attribute pairs, credential objects (down to who and where) and
// strings holding credential text replaced. A body that is not JSON is
// withheld whole when it holds credential text.
func redactSecretsJSON(data json.RawMessage) json.RawMessage {
	v, ok := decodeJSON(data)
	if !ok {
		if hasSecretText(string(data), false) {
			return json.RawMessage(redacted)
		}
		return data
	}
	out, err := encodeJSON(redactValueDepth(v, 0))
	if err != nil {
		return json.RawMessage(redacted)
	}
	return out
}

// decodeJSON parses exactly one JSON value, keeping numbers exact (json.Number)
// so large IDs survive the round trip.
func decodeJSON(data []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false // anything after the value (even text that is not JSON): not one JSON value
	}
	return v, true
}

func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func redactValue(v any) any { return redactValueDepth(v, 0) }

func redactValueDepth(v any, depth int) any { return redactValueKind(v, depth, false) }

// redactValueKind redacts v; narrow is true under a name or ID key, whose
// strings get the narrow label rule.
func redactValueKind(v any, depth int, narrow bool) any {
	switch t := v.(type) {
	case map[string]any:
		redactAttributePair(t)
		// The value of a name-like pair ({name: friendlyName, value: x}) is a
		// name, so it gets the narrow rule too.
		pairNarrow := false
		for k, v := range t {
			if s, isStr := v.(string); isStr && attributeNameKeys[normalizeKey(k)] && identityKey(s) {
				pairNarrow = true
			}
		}
		var withheld []string
		for k, val := range t {
			if keyHoldsCredential(k) {
				withheld = append(withheld, k)
				continue
			}
			if inner, isObj := val.(map[string]any); isObj && strings.Contains(normalizeKey(k), "credential") {
				t[k] = identityOnly(inner)
				continue
			}
			if isSecretKey(k) {
				if val != nil {
					t[k] = redacted
				}
				continue
			}
			t[k] = redactValueKind(val, depth, identityKey(k) || (pairNarrow && isAttributeValueKey(normalizeKey(k))))
		}
		withholdPairs(t, withheld)
		return t
	case []any:
		for i := range t {
			t[i] = redactValueKind(t[i], depth, narrow)
		}
		return t
	case string:
		return redactString(t, depth, narrow)
	}
	return v
}

// attributeNameKeys describe the name half of a name/value pair in any shape
// ION or Partner Center uses: {name,value}, {key,value}, {attributeName,
// attributeValue}, {name,values:[...]}, {field,data}, {type,value}.
var attributeNameKeys = map[string]bool{
	"name": true, "key": true, "attribute": true, "attributename": true, "attributekey": true,
	"field": true, "fieldname": true, "label": true, "property": true, "propertyname": true,
	"parameter": true, "parametername": true, "setting": true, "settingname": true, "type": true, "id": true,
	"question": true, "title": true,
}

// attributeValueKeys are the value half of a pair. Exact names only, so a
// product named 1PasswordBusiness keeps msrpValue and costValue.
var attributeValueKeys = map[string]bool{
	"value": true, "values": true, "attributevalue": true, "attributevalues": true, "fieldvalue": true,
	"propertyvalue": true, "parametervalue": true, "settingvalue": true, "data": true, "content": true, "text": true,
	"stringvalue": true, "val": true, "defaultvalue": true, "answer": true, "secretvalue": true, "body": true,
}

func isAttributeValueKey(n string) bool { return attributeValueKeys[n] }

// redactAttributePair blanks every value-like sibling of a name-like key that
// names a secret, whatever else the object carries.
func redactAttributePair(m map[string]any) {
	secret := false
	for k, v := range m {
		if s, isStr := v.(string); isStr && attributeNameKeys[normalizeKey(k)] && isSecretKey(s) {
			secret = true
			break
		}
	}
	if !secret {
		return
	}
	// Every string sibling goes, not only the value half: a validation error
	// echoes the rejected value under description or rejectedValue
	// (adversarial round 13, P2).
	for k, v := range m {
		if attributeNameKeys[normalizeKey(k)] {
			continue
		}
		if _, isStr := v.(string); (isStr || isAttributeValueKey(normalizeKey(k))) && v != nil {
			m[k] = redacted
		}
	}
}

// credentialIdentityKeys are the only children a credentials object keeps:
// who the login is and where to use it. Everything else (password, PIN,
// one-time codes, fields ION adds later) is withheld.
var credentialIdentityKeys = map[string]bool{
	"username": true, "user": true, "login": true, "loginurl": true, "email": true,
	"domain": true, "domainname": true, "userprincipalname": true, "upn": true,
}

func identityOnly(m map[string]any) map[string]any {
	var withheld []string
	for k, v := range m {
		if !plainFieldName(k) {
			withheld = append(withheld, k)
			continue
		}
		if s, isStr := v.(string); isStr && credentialIdentityKeys[normalizeKey(k)] {
			m[k] = redactString(s, 0, true)
			continue
		}
		if v != nil {
			m[k] = redacted
		}
	}
	withholdPairs(m, withheld)
	return m
}

// plainFieldNameRE is the shape of a JSON property name that is a field name
// and nothing more: letters, digits and the separators normalizeKey folds.
// Anything else (a space, a colon, quotes, a line break) makes the key text.
var plainFieldNameRE = regexp.MustCompile(`^[\pL\pN_.$@-]+$`)

func plainFieldName(k string) bool { return plainFieldNameRE.MatchString(canonicalText(k)) }

// keyHoldsCredential judges a property name that is not a plain field name
// as text, because such a key can carry the credential itself ("password:
// Hunter2", a label in any encoding): a secret-named one, or one that
// mentions a credential, is withheld with its value. A plain field name is
// schema: a secret-named one keeps its name and loses its value. A credential
// typed into a plain field name with no separator ("passwordHunter2") cannot
// be told from the name; governance.md says so.
func keyHoldsCredential(k string) bool {
	if plainFieldName(k) {
		return false
	}
	return isSecretKey(k) || hasSecretText(k, false)
}

// withholdPairs removes every listed key and leaves one "[redacted]" pair in
// their place, so a consumer sees that something was withheld but never what,
// nor under which name.
func withholdPairs(m map[string]any, keys []string) {
	if len(keys) == 0 {
		return
	}
	for _, k := range keys {
		delete(m, k)
	}
	m[redacted] = redacted
}

// redactString decodes a string that holds JSON and redacts inside it; a
// string with credential text that is not JSON is withheld whole.
func redactString(s string, depth int, narrow bool) any {
	trimmed := strings.TrimLeft(strings.TrimSpace(s), "\ufeff")
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, `"`) {
		if inner, ok := decodeJSON([]byte(trimmed)); ok {
			if depth >= maxNestedJSON {
				return redacted
			}
			out, err := encodeJSON(redactValueDepth(inner, depth+1))
			if err != nil {
				return redacted
			}
			return string(out)
		}
	}
	if hasSecretText(s, narrow) || hasEncodedSecretJSON(s, depth) {
		return redacted
	}
	return s
}

// hasEncodedSecretJSON reports a string whose escaped form decodes to JSON
// that the structural rules would change ("%7B%22clientSecret%22..."). It
// cannot be re-encoded the way it arrived, so it is withheld whole.
func hasEncodedSecretJSON(s string, depth int) bool {
	if !strings.ContainsAny(s, "%&\\") {
		return false
	}
	for _, f := range textDecodings(s)[1:] {
		t := strings.TrimLeft(strings.TrimSpace(f), "\ufeff")
		if !strings.HasPrefix(t, "{") && !strings.HasPrefix(t, "[") {
			continue
		}
		v, ok := decodeJSON([]byte(t))
		if !ok {
			continue
		}
		before, err1 := encodeJSON(v)
		after, err2 := encodeJSON(redactValueDepth(v, depth+1))
		if err1 != nil || err2 != nil || !bytes.Equal(before, after) {
			return true
		}
	}
	return false
}
