// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "error-text-secret-funnel" entry.

package client

import (
	"html"
	"strconv"
	"strings"
)

// echoWithheldText replaces the text of an answer that quotes back a credential.
const echoWithheldText = "[response text withheld: it quoted back a credential this CLI sent]"

// echoesCredential reports whether body quotes back a credential this client
// holds (the partner key, Respond access tokens) or one of extra (the request's
// webhook tokens), as written or in any mix of JSON escapes, percent-encoding
// (either case) and HTML entities, decoded repeatedly so nested encodings are
// caught. It reads the whole body before any cut or summary, and callers then
// withhold the whole text instead of guessing where a secret starts and ends,
// so no cut point and no partial match can leave part of a secret behind.
// Values under 8 characters are skipped; maskCredentialText still replaces
// them as written.
func (c *Client) echoesCredential(body []byte, extra ...string) bool {
	var secrets []string
	for _, s := range c.credentialSecrets(extra...) {
		if len(s) >= 8 {
			secrets = append(secrets, s)
		}
	}
	if len(secrets) == 0 || len(body) == 0 {
		return false
	}
	contains := func(view string) bool {
		for _, s := range secrets {
			if strings.Contains(view, s) {
				return true
			}
		}
		return false
	}
	view := string(body)
	if contains(view) {
		return true
	}
	// ponytail: 4 rounds of the three decoders covers any realistic nesting; a
	// fifth level of encoding would slip through.
	for round := 0; round < 4; round++ {
		before := view
		for _, decode := range []func(string) string{jsonUnescape, percentDecode, html.UnescapeString} {
			view = decode(view)
			if contains(view) {
				return true
			}
		}
		if view == before {
			return false
		}
	}
	return false
}

// jsonUnescape decodes JSON string escapes (\uXXXX, \/, \", \\) wherever they
// appear, without requiring the text to be valid JSON.
func jsonUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'u':
				if i+6 <= len(s) {
					if r, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil {
						b.WriteRune(rune(r))
						i += 5
						continue
					}
				}
			case '/', '"', '\\':
				b.WriteByte(s[i+1])
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// percentDecode decodes every valid %XX (either case) and leaves anything else.
func percentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+3 <= len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
