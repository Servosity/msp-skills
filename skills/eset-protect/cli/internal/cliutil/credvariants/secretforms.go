// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored test support (not generator output); pinned by
// skills/eset-protect/handfixes.json under "secret-registry-masks-every-boundary".
// Only _test.go files import this package, so it never reaches a binary.

package credvariants

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// SecretForm is one spelling of a secret a server could echo. Fixed forms
// are the ones the registry masks in place; the others (mixed, nested and
// per-character encodings) are only caught by its decoding backstop.
type SecretForm struct {
	Name  string
	Text  string
	Fixed bool
}

const (
	alphabetPlain   = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	alphabetSpecial = "-_.~!*'()\"\\/<>&%+=?#@:;,[]{}$^| "
)

var secretRunes = []string{"é", "ß", "日", "本", "\U0001F600", "ü"}

// RandomSecret returns a credential-shaped value of 8 to 48 characters:
// a mixed-alphabet password with specials, spaces and non-ASCII runes, or a
// JWT-shaped bearer. It never starts or ends with a space (Register trims).
func RandomSecret(r *Rand) string {
	if r.Intn(4) == 0 {
		return "eyJ" + randomFrom(r, alphabetPlain+"-_", 12+r.Intn(20)) + "." + randomFrom(r, alphabetPlain+"-_", 10+r.Intn(10))
	}
	n := 8 + r.Intn(41)
	var b strings.Builder
	for i := 0; i < n; i++ {
		switch k := r.Intn(10); {
		case k < 6:
			b.WriteByte(alphabetPlain[r.Intn(len(alphabetPlain))])
		case k < 9:
			c := alphabetSpecial[r.Intn(len(alphabetSpecial))]
			if c == ' ' && (i == 0 || i == n-1) {
				c = 'x'
			}
			b.WriteByte(c)
		default:
			b.WriteString(secretRunes[r.Intn(len(secretRunes))])
		}
	}
	return b.String()
}

func randomFrom(r *Rand, alphabet string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[r.Intn(len(alphabet))]
	}
	return string(b)
}

// RandomContext is filler text around an echoed secret, drawn from an
// alphabet that cannot spell an eight-character run of a secret by chance.
func RandomContext(r *Rand) string {
	words := []string{"Rejected", "credential", "for", "user", "request", "id", "denied", "token", "value", "was"}
	n := r.Intn(5)
	parts := make([]string, n)
	for i := range parts {
		parts[i] = words[r.Intn(len(words))]
	}
	return strings.Join(parts, " ")
}

// SecretForms returns every spelling of s the property tests feed back.
func SecretForms(s string) []SecretForm {
	goJSON := jsonInner(s, true)
	plainJSON := jsonInner(s, false)
	query := url.QueryEscape(s)
	var everyByte, everyRune, entities, swapped strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&everyByte, "%%%02x", s[i])
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z':
			c -= 'a' - 'A'
		case 'A' <= c && c <= 'Z':
			c += 'a' - 'A'
		}
		swapped.WriteByte(c)
	}
	k := 0
	var asciiJSON strings.Builder
	for _, r := range s {
		hexFormat := `\u%04x`
		if k%2 == 0 {
			hexFormat = `\u%04X`
		}
		if r > 0xFFFF {
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&everyRune, hexFormat+hexFormat, hi, lo)
			fmt.Fprintf(&asciiJSON, `\u%04x\u%04x`, hi, lo)
		} else {
			fmt.Fprintf(&everyRune, hexFormat, r)
			switch {
			case r == '"' || r == '\\':
				asciiJSON.WriteByte('\\')
				asciiJSON.WriteRune(r)
			case r >= utf8.RuneSelf:
				fmt.Fprintf(&asciiJSON, `\u%04x`, r)
			default:
				asciiJSON.WriteRune(r)
			}
		}
		fmt.Fprintf(&entities, "&#%d;", r)
		k++
	}
	half := len(s) / 2
	for half > 0 && !utf8.RuneStart(s[half]) {
		half--
	}
	return []SecretForm{
		{"raw", s, true},
		{"case-swapped", swapped.String(), true},
		{"json", goJSON, true},
		{"json-no-html-escape", plainJSON, true},
		{"json-escaped-slash", strings.ReplaceAll(plainJSON, "/", `\/`), true},
		{"json-ascii", asciiJSON.String(), true},
		{"query", query, true},
		{"query-lower-hex", lowerHex(query), true},
		{"path", url.PathEscape(s), true},
		{"html", html.EscapeString(s), true},
		{"every-byte-percent", everyByte.String(), false},
		{"every-rune-backslash-u", everyRune.String(), false},
		{"double-query", url.QueryEscape(query), false},
		{"json-in-json", jsonInner(goJSON, true), false},
		{"html-numeric", entities.String(), false},
		{"half-raw-half-percent", s[:half] + url.PathEscape(s[half:]), false},
		{"half-percent-half-json", url.QueryEscape(s[:half]) + jsonInner(s[half:], true), false},
	}
}

func jsonInner(s string, htmlSafe bool) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(htmlSafe)
	_ = enc.Encode(s)
	out := strings.TrimSuffix(b.String(), "\n")
	return out[1 : len(out)-1]
}

var pctRE = regexp.MustCompile(`%[0-9A-F]{2}`)

func lowerHex(s string) string {
	return pctRE.ReplaceAllStringFunc(s, strings.ToLower)
}

var (
	shortEscRE = regexp.MustCompile(`\\["\\/bfnrt]`)
	shortEsc   = map[string]string{`\"`: `"`, `\\`: `\`, `\/`: `/`, `\b`: "\b", `\f`: "\f", `\n`: "\n", `\r`: "\r", `\t`: "\t"}
)

// SecretRecoverable is the property tests' oracle, written apart from the
// masker: it reports whether out holds secret, or any run of at least
// partialRun of its bytes (a credential shown in part), in any case, after
// decoding JSON escapes, URL %XX escapes and HTML references in every order
// until nothing changes, with "+" also read as a space.
func SecretRecoverable(out []byte, secret string) bool {
	const partialRun = 10
	needles := []string{strings.ToLower(secret)}
	if len(secret) > partialRun {
		needles = needles[:0]
		for i := 0; i+partialRun <= len(secret); i++ {
			if utf8.RuneStart(secret[i]) {
				needles = append(needles, strings.ToLower(secret[i:i+partialRun]))
			}
		}
	}
	hit := func(text string) bool {
		low := strings.ToLower(text)
		plus := strings.ReplaceAll(low, "+", " ")
		for _, n := range needles {
			if strings.Contains(low, n) || strings.Contains(plus, n) {
				return true
			}
		}
		return false
	}
	text := string(out)
	for i := 0; i <= len(out); i++ {
		if hit(text) || Recoverable([]byte(text), secret) {
			return true
		}
		next := jsonEscapeRE.ReplaceAllStringFunc(text, func(m string) string {
			sub := jsonEscapeRE.FindStringSubmatch(m)
			if sub[1] != "" {
				hi, lo := hexRune(sub[1]), hexRune(sub[2])
				return string(rune(0x10000 + (hi-0xD800)<<10 + (lo - 0xDC00)))
			}
			return string(hexRune(sub[3]))
		})
		next = urlEscapeRE.ReplaceAllStringFunc(next, func(m string) string {
			return string([]byte{hexDigit(m[1])<<4 | hexDigit(m[2])})
		})
		next = shortEscRE.ReplaceAllStringFunc(next, func(m string) string { return shortEsc[m] })
		next = html.UnescapeString(next)
		if next == text {
			return false
		}
		text = next
	}
	return false
}
