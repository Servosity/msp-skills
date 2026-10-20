// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored test support (not generator output); pinned by
// skills/eset-protect/handfixes.json under "credential-tier-never-persisted".
// Only _test.go files import this package, so it never reaches a binary.
//
// Package credvariants generates the property-test corpus for the
// credential redaction funnels: a sentinel secret under every credential key,
// in every key spelling, inside valid JSON shapes, and in malformed,
// truncated, concatenated and non-JSON variants of them. Recoverable is the
// oracle, written independently of cliutil so the funnels are not checked
// against their own decoder.
package credvariants

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"
)

// Rand is the corpus's deterministic generator: a 64-bit linear
// congruential generator (Knuth's MMIX constants) on a signed state, which
// wraps by definition in Go. It replaces math/rand so a test corpus is
// reproducible from its seed with no weak-RNG finding and no integer
// conversion that could overflow; it is not for anything secret.
type Rand struct{ state int64 }

// NewRand seeds a generator.
func NewRand(seed int64) *Rand { return &Rand{state: seed} }

// Intn returns a value in [0, n). n must be positive.
func (r *Rand) Intn(n int) int {
	if n <= 0 {
		panic("credvariants: Intn bound must be positive")
	}
	r.state = r.state*6364136223846793005 + 1442695040888963407
	// The top 31 bits of the state, non-negative after the mask.
	v := int(r.state>>33) & 0x7fffffff
	return v % n
}

// hexDigit returns the value (0-15) of one hex digit the escape regexps
// already matched.
func hexDigit(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

// hexRune decodes the hex digits of a \uXXXX escape (at most four, so the
// value fits a rune) by widening each digit: no conversion can overflow.
func hexRune(s string) rune {
	var r rune
	for i := 0; i < len(s); i++ {
		r = r<<4 | rune(hexDigit(s[i]))
	}
	return r
}

var (
	jsonEscapeRE = regexp.MustCompile(`\\u([dD][89abAB][0-9a-fA-F]{2})\\u([dD][c-fC-F][0-9a-fA-F]{2})|\\u([0-9a-fA-F]{4})`)
	urlEscapeRE  = regexp.MustCompile(`%([0-9a-fA-F]{2})`)
)

// Recoverable reports whether b holds sentinel in any spelling a reader
// could recover: literally, behind JSON \u or URL %XX escapes (decoded until
// nothing changes), in any case.
func Recoverable(b []byte, sentinel string) bool {
	s := string(b)
	for i := 0; i <= len(b); i++ {
		next := jsonEscapeRE.ReplaceAllStringFunc(s, func(m string) string {
			sub := jsonEscapeRE.FindStringSubmatch(m)
			if sub[1] != "" {
				return string(utf16.DecodeRune(hexRune(sub[1]), hexRune(sub[2])))
			}
			return string(hexRune(sub[3]))
		})
		next = urlEscapeRE.ReplaceAllStringFunc(next, func(m string) string {
			return string([]byte{hexDigit(m[1])<<4 | hexDigit(m[2])})
		})
		if next == s {
			break
		}
		s = next
	}
	folded := strings.NewReplacer("\u017f", "s", "\u212a", "k").Replace(strings.ToLower(s))
	return strings.Contains(folded, strings.ToLower(sentinel))
}

// KeySpellings returns every spelling of key the corpus uses: as is, upper,
// lower, random per-letter case, each letter \u-escaped on its own (hex upper
// and lower), all letters escaped, and the two non-ASCII runes that
// strings.EqualFold folds onto k and s, literal and escaped.
func KeySpellings(key string, r *Rand) []string {
	out := []string{key, strings.ToUpper(key), strings.ToLower(key)}
	for i := 0; i < 3; i++ {
		var b strings.Builder
		for _, c := range key {
			if r.Intn(2) == 0 {
				b.WriteString(strings.ToUpper(string(c)))
			} else {
				b.WriteString(strings.ToLower(string(c)))
			}
		}
		out = append(out, b.String())
	}
	for i := range key {
		out = append(out, key[:i]+fmt.Sprintf(`\u%04x`, key[i])+key[i+1:])
		out = append(out, key[:i]+fmt.Sprintf(`\u%04X`, key[i])+key[i+1:])
	}
	var all strings.Builder
	for _, c := range key {
		fmt.Fprintf(&all, `\u%04x`, c)
	}
	out = append(out, all.String())
	for i, c := range strings.ToLower(key) {
		switch c {
		case 'k':
			out = append(out, key[:i]+"\u212a"+key[i+1:], key[:i]+`\u212a`+key[i+1:], key[:i]+`\u212A`+key[i+1:])
		case 's':
			out = append(out, key[:i]+"\u017f"+key[i+1:], key[:i]+`\u017f`+key[i+1:])
		}
	}
	return out
}

// ValueSpellings are JSON values that carry sentinel.
func ValueSpellings(sentinel string) []string {
	s := sentinel
	return []string{
		`"` + s + `"`,
		`"https://download.example/a?b=1\u0026c=` + s + `"`,
		`"\u` + fmt.Sprintf("%04x", s[0]) + s[1:] + `"`,
		`"` + s + `\"quoted\\"`,
		`{"href":"` + s + `"}`,
		`["` + s + `"]`,
		`[{"x":"` + s + `"}]`,
	}
}

// Bodies builds valid JSON bodies that hold value under key spelling k: at
// the top level, beside other fields, inside list envelopes and maps, and in
// duplicate-key shapes where a later occurrence of the same key (spelled k,
// or plainKey) is null, "" or marker.
func Bodies(k, plainKey, value, marker string) []string {
	q := `"` + k + `"`
	pq := `"` + plainKey + `"`
	return []string{
		`{` + q + `:` + value + `}`,
		`{"uuid":"00000000-0000-0000-0000-0000000000b1",` + q + ` : ` + value + `,"n":12345678901234567890}`,
		`{"items":[{` + q + `:` + value + `},{"b":2}],"nextPageToken":""}`,
		`[{` + q + `:` + value + `}]`,
		`{"m":{"x":{` + q + `:` + value + `}}}`,
		`{` + q + `:` + value + `,` + pq + `:""}`,
		`{` + q + `:` + value + `,` + q + `:null}`,
		`{` + q + `:` + value + `,` + q + `:"` + marker + `"}`,
		"\n {\t" + q + "\n:\r" + value + " }\n",
	}
}

// Malformed derives bodies that are not one JSON value from a valid body b:
// concatenations, trailing and leading junk, unterminated strings, and every
// truncation (all true) or a random sample of them.
func Malformed(b string, all bool, r *Rand) []string {
	out := []string{
		`{}]` + b, `{}` + b, b + b, b + `}`, b + ` trailing`, `[` + b, b + `,` + b, `"x"` + b,
		b + `garbage"`, `]` + b, b + `]`, `null` + b, b + `{"a":`, strings.Replace(b, `"`, ``, 1),
	}
	if i := strings.LastIndex(b, `"`); i > 0 {
		out = append(out, b[:i]+b[i+1:])
	}
	if all {
		for i := 0; i < len(b); i++ {
			out = append(out, b[:i])
		}
	} else {
		for j := 0; j < 8; j++ {
			out = append(out, b[:r.Intn(len(b))])
		}
	}
	return out
}

// NonJSONMentions are bytes that are not JSON and name key with sentinel
// beside it: as a form field, URL-encoded (one letter, all letters, and an
// escaped \u escape), upper case, and inside markup.
func NonJSONMentions(key, sentinel string) []string {
	s := sentinel
	enc := ""
	for _, c := range []byte(key) {
		enc += fmt.Sprintf("%%%02X", c)
	}
	half := key[:1] + fmt.Sprintf("%%%02x", key[1]) + key[2:]
	return []string{
		key + "=" + s,
		half + "=" + s + "&x=1",
		enc + "=" + s,
		strings.ToUpper(key) + ": " + s,
		"<html>" + key + " " + s + "</html>",
		`%5Cu00` + fmt.Sprintf("%02x", key[0]) + key[1:] + `=` + s,
	}
}

// Corpus is every generated variant for keys, deduplicated and in a stable
// order. Every truncation is generated for a sample of the valid bodies (the
// first key spellings and values); the rest get a random sample. full false
// keeps only the first seven key spellings (for funnels that are slow to
// drive, such as a SQLite write per variant).
func Corpus(keys []string, sentinel, marker string, seed int64, full bool) []string {
	r := NewRand(seed)
	seen := map[string]bool{}
	var corpus []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			corpus = append(corpus, s)
		}
	}
	for _, key := range keys {
		spellings := KeySpellings(key, r)
		if !full && len(spellings) > 7 {
			spellings = append(spellings[:4], spellings[len(spellings)-3:]...)
		}
		for ki, k := range spellings {
			for vi, v := range ValueSpellings(sentinel) {
				for bi, b := range Bodies(k, key, v, marker) {
					if !json.Valid([]byte(b)) {
						panic("credvariants: generated body is not valid JSON: " + b)
					}
					add(b)
					for _, m := range Malformed(b, full && ki < 2 && vi < 2 && bi < 3, r) {
						add(m)
					}
				}
			}
		}
		for _, m := range NonJSONMentions(key, sentinel) {
			add(m)
		}
	}
	return corpus
}
