// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: the one list of secret-bearing ConnectSecure endpoints.

package cliutil

import (
	"html"
	"net/url"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// connectsecureSecrets are endpoints whose responses carry credential
// material (credential params, integration secrets and rule params, the agent
// uninstall secret, scheduled jobs, whose rows carry a password field per the
// vendor spec, attack-surface results, which carry creds and hashes, and
// custom domains, whose cert_details shape is undocumented). Their rows are never written to the local store (store guards)
// and their bodies never enter the HTTP response cache (client guards).
// Resource is the sync/store resource name; empty for path-only endpoints.
var connectsecureSecrets = []struct{ Resource, PathPrefix string }{
	{"credentials", "/r/company/credentials"},
	{"integration_credentials", "/r/integration/integration_credentials"},
	{"", "/r/company/get_uninstall_secret"},
	{"scheduler", "/r/company/scheduler"},
	// Attack-surface OSINT rows carry leaked creds and password hashes for
	// client domains; integration rules carry integration_params.
	{"attack_surface_results", "/r/company/attack_surface_results"},
	{"integration_rules", "/r/integration/integration_rules"},
	// Custom domains: create uploads the TLS private key and the GET's
	// cert_details object is undocumented, so assume it can echo key material.
	{"custom_domains", "/r/company/custom_domains"},
}

// ConnectsecureSecretResource reports whether rows of resourceType must not be persisted.
func ConnectsecureSecretResource(resourceType string) bool {
	for _, s := range connectsecureSecrets {
		if s.Resource != "" && s.Resource == resourceType {
			return true
		}
	}
	return false
}

// ConnectsecureSecretPath reports whether a request path returns secret material.
func ConnectsecureSecretPath(path string) bool {
	for _, s := range connectsecureSecrets {
		p := s.PathPrefix
		if path == p || strings.HasPrefix(path, p+"/") || strings.HasPrefix(path, p+"?") {
			return true
		}
	}
	return false
}

// ConnectsecureSecretResources lists the store resource names, for checks.
func ConnectsecureSecretResources() []string {
	out := make([]string, 0, len(connectsecureSecrets))
	for _, s := range connectsecureSecrets {
		if s.Resource != "" {
			out = append(out, s.Resource)
		}
	}
	return out
}

// ConnectsecureEchoesSecret reports whether text carries any of secrets in
// plaintext or in any form a reader could decode back to it: JSON/JS string
// escapes from any encoder (\uXXXX in either case, surrogate pairs, \/ \" \'),
// HTML entities, URL escapes and Latin-1 bytes, applied repeatedly so an echo
// nested inside an echo is seen too. Any PEM private key counts. The caller
// withholds the whole text rather than masking it, so no encoding, truncation
// point or secret length can leave part of a secret behind.
// Hand-authored (handfixes.json: error-body-redacts-secrets).
func ConnectsecureEchoesSecret(text string, secrets []string) bool {
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
		if u, err := url.QueryUnescape(v); err == nil {
			add(u)
		}
		if u, err := url.PathUnescape(v); err == nil {
			add(u)
		}
	}
	return views
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
// left-to-right pass, so `\\u00e9` becomes `\u00e9` (and é on the next view).
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
