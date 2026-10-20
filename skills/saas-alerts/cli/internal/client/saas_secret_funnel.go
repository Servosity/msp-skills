// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "error-text-secret-funnel" entry.

package client

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"strings"
	"unicode"
	"unicode/utf16"

	"saas-alerts-pp-cli/internal/platform"
)

// echoWithheldText replaces the text of an answer that quotes back a credential.
// Its wording avoids the words the auth-error classifier looks for, so a
// withheld 400 still classifies as the API error it was.
const echoWithheldText = "[response text withheld: it repeated a secret value this CLI sent]"

// webhookWithheldText replaces shown text that carries a webhook delivery token.
const webhookWithheldText = "[response text withheld: it held a webhook delivery value]"

// funnelSecrets lists the values the funnel looks for: every credential this
// client holds (credentialSecrets: the partner key, Respond access tokens) and
// extra (the request's webhook tokens). Values under 8 characters are skipped;
// maskCredentialText still replaces them as written.
func (c *Client) funnelSecrets(extra ...string) []string {
	var secrets []string
	for _, s := range c.credentialSecrets(extra...) {
		if len(s) >= 8 {
			secrets = append(secrets, s)
			// A secret that itself holds decodable text (a literal %41,
			// &amp; or \u0041) is also looked for fully decoded, since the
			// answer's views are decoded the same way before matching.
			if d := fullyDecoded(s); d != s && len(d) >= 8 {
				secrets = append(secrets, d)
			}
		}
	}
	return secrets
}

// fullyDecoded applies the funnel decoders until nothing changes (capped at
// 32 rounds, as anyDecodedView is).
func fullyDecoded(s string) string {
	for round := 0; round < 32; round++ {
		before := s
		for _, decode := range funnelDecoders {
			s = decode(s)
		}
		if s == before {
			break
		}
	}
	return s
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
	return anyDecodedView(view, func(v string) bool {
		for _, s := range secrets {
			if strings.Contains(v, s) {
				return true
			}
		}
		return false
	})
}

// funnelDecoders are the decodings every secret check looks through.
var funnelDecoders = []func(string) string{dropUnprintable, jsonUnescape, percentDecode, html.UnescapeString}

// anyDecodedView reports whether match holds for view or for any view the
// funnel decoders produce, decoding until nothing changes. Every decoder only
// shortens the text or leaves it alone, so this reaches a fixed point.
// ponytail: capped at 32 rounds, so pathological input stays linear-ish; 32
// nested encodings of one echo would slip through.
func anyDecodedView(view string, match func(string) bool) bool {
	if match(view) {
		return true
	}
	for round := 0; round < 32; round++ {
		before := view
		for _, decode := range funnelDecoders {
			view = decode(view)
			if match(view) {
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

// dropUnprintable deletes invalid UTF-8, control and format (Cf) characters,
// as the error display does (ToValidUTF8 at the cut, control characters in HTML titles), so
// an echo split by one of them is seen joined, as the user would see it.
func dropUnprintable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// unreadableAnswer marks an error where the server answered but its answer
// could not be read (a failed body read, an unsupported Content-Encoding, an
// HTML page where JSON was expected). For a write, whether it was applied is
// then unknown. The message is the wrapped error's, unchanged.
type unreadableAnswer struct{ err error }

func (e unreadableAnswer) Error() string { return e.err.Error() }
func (e unreadableAnswer) Unwrap() error { return e.err }

// IsAnswerUnreadable reports whether err is an answer the server sent but this
// client could not read, so a write's outcome is unknown, not refused.
func IsAnswerUnreadable(err error) bool {
	var u unreadableAnswer
	return errors.As(err, &u)
}

// notSent marks an error from before the request reached the wire (a budget
// or rate-limit wait, the Respond token mint, building the request, a dial
// or TLS failure): the server never saw it, so a write was not applied.
type notSent struct{ err error }

func (e notSent) Error() string { return e.err.Error() }
func (e notSent) Unwrap() error { return e.err }

// IsNotSent reports whether err happened before the request reached the wire.
func IsNotSent(err error) bool {
	var n notSent
	return errors.As(err, &n)
}

// WriteOutcomeUnknown reports whether a failed write may still have been
// applied. Only a request that never reached the wire, or an answer that
// refused it (a 4xx, including a 429), is a known "no"; a 5xx, an unreadable
// answer, or any failure after the request was written (a reset stream, a
// timeout waiting for the answer) leaves the server's state unknown.
func WriteOutcomeUnknown(err error) bool {
	if err == nil || IsNotSent(err) {
		return false
	}
	var rl *platform.RateLimitedError
	if errors.As(err, &rl) {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500
	}
	return true
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

// funnelShown is funnelText for an answer's shown text on path: on a Webhooks
// path the text as shown (after HTML tags, entities and whitespace were folded
// for display) is checked for a token too.
func (c *Client) funnelShown(path, text string, extra ...string) string {
	text = c.funnelText(text, extra...)
	if strings.Contains(path, "/outgoingWebhookApi/") && webhookTokenLeftInText([]byte(text)) {
		return webhookWithheldText
	}
	return text
}

// jsonUnescape decodes JSON string escapes (\uXXXX, \/, \", \\, \b \f \n \r \t) wherever they
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
				if r, ok := hexRune(s, i+2); ok {
					// A UTF-16 surrogate pair is two escapes for one character.
					if utf16.IsSurrogate(r) && i+12 <= len(s) && s[i+6] == '\\' && s[i+7] == 'u' {
						if r2, ok := hexRune(s, i+8); ok {
							if d := utf16.DecodeRune(r, r2); d != unicode.ReplacementChar {
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
			case '/', '"', '\\':
				b.WriteByte(s[i+1])
				i++
				continue
			case 'b', 'f', 'n', 'r', 't':
				b.WriteByte(map[byte]byte{'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t'}[s[i+1]])
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// hexRune reads the 4 hex digits at s[at:] as one UTF-16 unit.
func hexRune(s string, at int) (rune, bool) {
	if at+4 > len(s) {
		return 0, false
	}
	b, err := hex.DecodeString(s[at : at+4])
	if err != nil {
		return 0, false
	}
	return rune(b[0])<<8 | rune(b[1]), true
}

// percentDecode decodes every valid %XX (either case) and leaves anything else.
func percentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+3 <= len(s) {
			if v, err := hex.DecodeString(s[i+1 : i+3]); err == nil {
				b.WriteByte(v[0])
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
