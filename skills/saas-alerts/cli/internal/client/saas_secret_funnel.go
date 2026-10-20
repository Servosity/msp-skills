// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "error-text-secret-funnel" entry.

package client

import (
	"bytes"
	"encoding/json"
	"html"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
)

// echoWithheldText replaces the text of an answer that quotes back a credential.
// Its wording avoids the words the auth-error classifier looks for, so a
// withheld 400 still classifies as the API error it was.
const echoWithheldText = "[response text withheld: it repeated a secret value this CLI sent]"

// funnelSecrets lists the values the funnel looks for: every credential this
// client holds (credentialSecrets: the partner key, Respond access tokens) and
// extra (the request's webhook tokens). Values under 8 characters are skipped;
// maskCredentialText still replaces them as written.
func (c *Client) funnelSecrets(extra ...string) []string {
	var secrets []string
	for _, s := range c.credentialSecrets(extra...) {
		if len(s) >= 8 {
			secrets = append(secrets, s)
		}
	}
	return secrets
}

// echoesCredential reports whether body quotes back a funnel secret. It reads
// the whole body before any cut or summary, and callers then withhold the whole
// text instead of guessing where a secret starts and ends, so no cut point and
// no partial match can leave part of a secret behind.
func (c *Client) echoesCredential(body []byte, extra ...string) bool {
	return echoesAny(string(body), c.funnelSecrets(extra...))
}

// echoesAny reports whether view contains one of secrets as written or after
// any mix of dropping unprintable characters, JSON escapes, percent-encoding
// (either case) and HTML entities, decoded until nothing changes.
func echoesAny(view string, secrets []string) bool {
	if len(secrets) == 0 || view == "" {
		return false
	}
	contains := func(v string) bool {
		for _, s := range secrets {
			if strings.Contains(v, s) {
				return true
			}
		}
		return false
	}
	if contains(view) {
		return true
	}
	// Every decoder only shortens the text or leaves it alone, so this reaches
	// a fixed point. ponytail: capped at 32 rounds, so pathological input stays
	// linear-ish; 32 nested encodings of one echo would slip through.
	for round := 0; round < 32; round++ {
		before := view
		for _, decode := range []func(string) string{dropUnprintable, jsonUnescape, percentDecode, html.UnescapeString} {
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

// withholdEchoedValues keeps a successful answer that quotes back a funnel
// secret from reaching output, the cache or the store: each JSON string (key
// or value) that echoes one is replaced whole by WebhookTokenWithheld; if the
// echo cannot be pinned to single strings, or the answer is not one JSON
// value, the whole answer is withheld.
func (c *Client) withholdEchoedValues(body json.RawMessage, extra ...string) json.RawMessage {
	secrets := c.funnelSecrets(extra...)
	if !echoesAny(string(body), secrets) {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) == nil && len(bytes.TrimSpace(body[dec.InputOffset():])) == 0 {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if enc.Encode(withholdEchoedStrings(v, secrets)) == nil {
			if out := bytes.TrimRight(buf.Bytes(), "\n"); !echoesAny(string(out), secrets) {
				return json.RawMessage(out)
			}
		}
	}
	whole, _ := json.Marshal(echoWithheldText)
	return whole
}

func withholdEchoedStrings(v any, secrets []string) any {
	switch t := v.(type) {
	case string:
		if echoesAny(t, secrets) {
			return WebhookTokenWithheld
		}
		return t
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if echoesAny(k, secrets) {
				k = WebhookTokenWithheld
			}
			out[k] = withholdEchoedStrings(val, secrets)
		}
		return out
	case []any:
		for i, val := range t {
			t[i] = withholdEchoedStrings(val, secrets)
		}
		return t
	}
	return v
}

// dropUnprintable deletes invalid UTF-8 and control characters, as the error
// display does (ToValidUTF8 at the cut, control characters in HTML titles), so
// an echo split by one of them is seen joined, as the user would see it.
func dropUnprintable(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// funnelText is the last step before text is shown: the raw mask, then the
// echo check on what would be shown, which can differ from what was checked
// before the cut (a control character or invalid byte the display dropped).
func (c *Client) funnelText(text string, extra ...string) string {
	text = c.maskCredentialText(text, extra...)
	if c.echoesCredential([]byte(text), extra...) {
		return echoWithheldText
	}
	return text
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
						// A UTF-16 surrogate pair is two escapes for one character.
						if utf16.IsSurrogate(rune(r)) && i+12 <= len(s) && s[i+6] == '\\' && s[i+7] == 'u' {
							if r2, err := strconv.ParseUint(s[i+8:i+12], 16, 32); err == nil {
								if d := utf16.DecodeRune(rune(r), rune(r2)); d != unicode.ReplacementChar {
									b.WriteRune(d)
									i += 11
									continue
								}
							}
						}
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
