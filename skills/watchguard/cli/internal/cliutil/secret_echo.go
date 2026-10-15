// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: error-body-redacts-secrets), ported from
// skills/connectsecure/cli/internal/cliutil/connectsecure_secrets.go.

package cliutil

import (
	"encoding/base64"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// EchoesSecret reports whether text carries any of secrets in plaintext or in
// any form a reader could decode back to it: JSON/JS string escapes from any
// encoder (\uXXXX in either case, surrogate pairs, \/ \" \'), HTML entities,
// URL escapes, UTF-16 and Latin-1 bytes, applied repeatedly so an echo nested
// inside an echo is seen too. Any PEM private key counts. The caller withholds
// the whole text rather than masking it, so no encoding, truncation point or
// secret length can leave part of a secret behind.
func EchoesSecret(text string, secrets []string) bool {
	if strings.Contains(text, "PRIVATE KEY") {
		return true
	}
	views := decodedViews(text)
	for _, s := range secrets {
		if s == "" {
			continue
		}
		needles := []string{s}
		if len(s) >= 32 {
			// A server may echo a long value cut down to its head and tail.
			needles = append(needles, s[len(s)-16:])
			if !strings.HasPrefix(s, "eyJ") { // a JWT's head is its constant header
				needles = append(needles, s[:16])
			}
		}
		for _, v := range views {
			for _, n := range needles {
				if strings.Contains(v, n) {
					return true
				}
			}
		}
	}
	return false
}

// ponytail: 32 views bounds the work on a large error body; an echo needs at
// most a few decode steps.
func decodedViews(text string) []string {
	views := []string{text}
	seen := map[string]bool{text: true}
	add := func(v string) {
		if !seen[v] && len(views) < 32 {
			seen[v] = true
			views = append(views, v)
		}
	}
	if strings.ContainsRune(text, 0) {
		add(strings.ReplaceAll(text, "\x00", "")) // UTF-16 bodies, ASCII part
		for off := 0; off < 2 && off < len(text); off++ {
			add(decodeUTF16(text[off:], false)) // UTF-16LE, any alignment
			add(decodeUTF16(text[off:], true))  // UTF-16BE
		}
	}
	for i := 0; i < len(views); i++ {
		v := views[i]
		if !utf8.ValidString(v) {
			r := make([]rune, 0, len(v))
			for j := 0; j < len(v); j++ {
				r = append(r, rune(v[j])) // Latin-1: each byte is its code point
			}
			add(string(r))
		}
		add(unescapeJS(v))
		add(html.UnescapeString(v))
		if d := decodeBase64Runs(v); d != "" {
			add(d)
		}
		if u, err := url.QueryUnescape(v); err == nil {
			add(u)
		}
		if u, err := url.PathUnescape(v); err == nil {
			add(u)
		}
	}
	return views
}

var base64Run = regexp.MustCompile(`[A-Za-z0-9+/_-]{12,}={0,2}`)

// decodeBase64Runs decodes every base64-looking run in s (standard or URL
// alphabet, padded or not, starting at any of the 4 alignments so a run glued
// to a preceding word still decodes) and joins the results, so a server that
// echoes a submitted secret base64-encoded is caught. Hand-fix
// (handfixes.json error-body-redacts-secrets): Codex review 2026-10-15 echoed
// pvt_key back as base64 and it was printed.
func decodeBase64Runs(s string) string {
	var parts []string
	for _, run := range base64Run.FindAllString(s, -1) {
		run = strings.TrimRight(run, "=")
		for off := 0; off < 4 && off < len(run); off++ {
			r := run[off:]
			for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
				if d, err := enc.DecodeString(r); err == nil {
					parts = append(parts, string(d))
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

// decodeUTF16 reads s as UTF-16 code units so a non-ASCII secret echoed in
// UTF-16 is seen whole, not just its ASCII bytes.
func decodeUTF16(s string, bigEndian bool) string {
	u := make([]uint16, 0, len(s)/2)
	for i := 0; i+1 < len(s); i += 2 {
		hi, lo := uint16(s[i+1]), uint16(s[i])
		if bigEndian {
			hi, lo = lo, hi
		}
		u = append(u, hi<<8|lo)
	}
	return string(utf16.Decode(u))
}

// unescapeJS decodes one level of JSON/JS string escapes in a single
// left-to-right pass, so `\\u00e9` becomes `é` (and é on the next view).
func unescapeJS(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		switch n := s[i+1]; n {
		case 'u':
			if r, ok := hexRune(s, i+2, 4); ok {
				if utf16.IsSurrogate(r) && s[i+6:min(i+8, len(s))] == `\u` {
					if r2, ok := hexRune(s, i+8, 4); ok {
						if d := utf16.DecodeRune(r, r2); d != utf8.RuneError {
							b.WriteRune(d)
							i += 11
							continue
						}
					}
				}
				b.WriteRune(r)
				i += 5
				continue
			}
			b.WriteByte(c)
		case 'x':
			if r, ok := hexRune(s, i+2, 2); ok {
				b.WriteByte(byte(r & 0xff)) // Python/Ruby byte escapes; runs of them rebuild UTF-8
				i += 3
				continue
			}
			b.WriteByte(c)
		case 'b':
			b.WriteByte('\b')
			i++
		case 'f':
			b.WriteByte('\f')
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
		case '/', '"', '\'', '\\':
			b.WriteByte(n)
			i++
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// hexRune reads n hex digits of s at off without integer narrowing.
func hexRune(s string, off, n int) (rune, bool) {
	if off+n > len(s) {
		return 0, false
	}
	var r rune
	for _, c := range []byte(s[off : off+n]) {
		var v byte
		switch {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = c - 'A' + 10
		default:
			return 0, false
		}
		r = r<<4 | rune(v)
	}
	return r, true
}
