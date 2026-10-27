// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cipp/handfixes.json under the
// "success-body-credential-funnel" entry.

package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"net/url"
	"strings"
	"unicode"
)

// ErrSuccessBodyWithheld replaces a successful response that still carries a
// configured credential after redaction. Nothing of that body is returned,
// cached or stored.
var ErrSuccessBodyWithheld = errors.New("[response withheld: it contained a configured credential]")

// ErrSuccessBodyTooLarge replaces a successful response over the response size
// limit. Nothing of it is returned, cached or stored.
var ErrSuccessBodyTooLarge = errors.New("[response withheld: it is larger than the response size limit; set " + maxResponseBytesEnv + " to raise it]")

// ErrSuccessBodyValueTooLarge replaces a successful response holding a single
// value (a JSON string, key or number, or the whole text of a body that is not
// JSON) longer than recursiveDecodeCap once whitespace is removed: the full
// recursive decoding cannot read it whole. Nothing of it is returned, cached
// or stored.
var ErrSuccessBodyValueTooLarge = errors.New("[response withheld: one of its values is over 64 KiB, too large to check for a configured credential]")

// recursiveDecodeCap is the longest text the full recursive decoding reads
// whole: the client's own decoder (credentialTextReveals) stops past 65536
// bytes, below the shared funnel's 1 MiB.
const recursiveDecodeCap = 65536

// successFunnelMaxScan is the most the shared detector scans whole (the
// funnel's own input limit). A larger body, up to the response size limit,
// gets the large-body scan instead (largeBodyRevealsCredential).
const successFunnelMaxScan = 1 << 20

// redactSuccessBody passes a successful response body through the configured-
// credential funnel before it is returned, printed, cached or stored.
//
//  1. Normalize: every character the detector treats as removable (control and
//     Unicode format characters such as U+200B, U+200C, U+200D, U+2060 and
//     U+FEFF, and invalid UTF-8) is stripped from each JSON string and key
//     (decoded, so a JSON-escaped one goes too), or from the text of a body
//     that is not JSON.
//  2. Detect over the WHOLE normalized body, no windowing (maskCredentialText:
//     exact, URL-escaped and every decoded view the shared funnel reaches). A
//     body over successFunnelMaxScan, up to the response size limit, gets the
//     large-body scan instead: withheld (ErrSuccessBodyWithheld) on a hit,
//     returned byte for byte otherwise.
//  3. If something is found, redact each string, number and key on its own,
//     serialize, and run the same whole-body detector over the final bytes; any
//     residue withholds the body (ErrSuccessBodyWithheld).
//
// A clean body that fits the scan comes back byte for byte.
//
// Then, whatever would be returned (the body as it came, or its redacted
// form) gets the per-value pass (valuesReveal): every JSON string, key and
// number, or the whole text of a body that is not JSON, with whitespace and
// format characters removed first. Any hit withholds the body.
func (c *Client) redactSuccessBody(body []byte, extraCredentials ...string) ([]byte, error) {
	out, err := c.redactSuccessBodyWhole(body, extraCredentials...)
	if err != nil || len(out) == 0 {
		return out, err
	}
	if err := c.valuesReveal(out, extraCredentials); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) redactSuccessBodyWhole(body []byte, extraCredentials ...string) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	isJSON := dec.Decode(&v) == nil && !dec.More()

	normalized := body
	if isJSON {
		var changed bool
		v = normalizeJSONValue(v, &changed)
		if changed {
			s, err := marshalPlain(v)
			if err != nil {
				return nil, ErrSuccessBodyWithheld
			}
			normalized = []byte(s)
		}
	} else if s := stripRemovable(string(body)); s != string(body) {
		normalized = []byte(s)
	}

	if len(normalized) > successFunnelMaxScan {
		if int64(len(body)) > maxResponseBytes() {
			return nil, ErrSuccessBodyTooLarge
		}
		if c.largeBodyRevealsCredential(body, isJSON, extraCredentials) {
			return nil, ErrSuccessBodyWithheld
		}
		return body, nil
	}
	// The original bytes are scanned too when they fit (the decoder keeps
	// only the last of duplicate keys), and are what a clean body returns, so
	// a clean body with format characters (an emoji joiner) keeps them.
	rawScanned := len(body) <= successFunnelMaxScan
	detected := c.credentialDetected(normalized, extraCredentials) ||
		(rawScanned && !bytes.Equal(normalized, body) && c.credentialDetected(body, extraCredentials))
	if !detected {
		if rawScanned {
			return body, nil
		}
		return normalized, nil
	}

	out := []byte(c.maskCredentialText(string(normalized), extraCredentials...))
	if isJSON {
		if s, err := marshalPlain(c.redactJSONValue(v, extraCredentials)); err == nil {
			out = []byte(s)
		}
	}
	if len(out) > successFunnelMaxScan || c.credentialDetected(out, extraCredentials) {
		return nil, ErrSuccessBodyWithheld
	}
	return out, nil
}

// credentialDetected runs the configured-credential detector over the whole of
// b: true when maskCredentialText would change it.
func (c *Client) credentialDetected(b []byte, extra []string) bool {
	text := string(b)
	return c.maskCredentialText(text, extra...) != text
}

// stripRemovable drops what the shared detector drops before it looks:
// control and Unicode format (Cf) characters and invalid UTF-8.
func stripRemovable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// normalizeJSONValue strips removable characters from every string and key.
func normalizeJSONValue(v any, changed *bool) any {
	switch x := v.(type) {
	case string:
		s := stripRemovable(x)
		if s != x {
			*changed = true
		}
		return s
	case []any:
		for i := range x {
			x[i] = normalizeJSONValue(x[i], changed)
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, child := range x {
			nk := stripRemovable(k)
			if nk != k {
				*changed = true
			}
			out[nk] = normalizeJSONValue(child, changed)
		}
		return out
	}
	return v
}

func (c *Client) redactJSONValue(v any, extra []string) any {
	switch x := v.(type) {
	case string:
		return c.maskCredentialText(x, extra...)
	case json.Number:
		s := string(x)
		if masked := c.maskCredentialText(s, extra...); masked != s {
			return masked
		}
		return x
	case []any:
		for i := range x {
			x[i] = c.redactJSONValue(x[i], extra)
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, child := range x {
			out[c.maskCredentialText(k, extra...)] = c.redactJSONValue(child, extra)
		}
		return out
	}
	return v
}

// largeBodyRevealsCredential is the scan for a body past successFunnelMaxScan:
// no windowing, every view full length. The views are the body's text and its
// decoded JSON strings, keys and numbers (every token, duplicate keys
// included), each with removable characters dropped, as written, with %XX,
// \xXX and \uXXXX escapes decoded, and with HTML entities decoded, all
// lowercased. The needles are every mask credentialMaskSet builds for each
// configured credential (exact, URL-escaped, JSON \u-escaped, hex, base64,
// base32 and the rest) plus base64 and base64url at all three byte alignments
// (alignedBase64), lowercased, so hex and escapes match in any letter case.
//
// ponytail: one strings.Contains pass per needle and view, O(needles x bytes);
// an Aho-Corasick matcher if bodies near the response cap get slow.
func (c *Client) largeBodyRevealsCredential(body []byte, isJSON bool, extra []string) bool {
	needles := c.largeBodyNeedles(extra)
	if len(needles) == 0 {
		return false
	}
	if textRevealsKnownEncoding(string(body), needles) {
		return true
	}
	return isJSON && textRevealsKnownEncoding(decodedJSONText(body), needles)
}

// textRevealsKnownEncoding is the full-length known-encodings scan of one
// text: as written, with %XX, \xXX and \uXXXX escapes decoded, and with HTML
// entities decoded, each with removable characters dropped and lowercased,
// searched for every needle.
func textRevealsKnownEncoding(text string, needles []string) bool {
	for _, view := range []func(string) string{
		func(s string) string { return s },
		lenientUnescape,
		html.UnescapeString,
	} {
		v := strings.ToLower(stripRemovable(view(text)))
		for _, n := range needles {
			if strings.Contains(v, n) {
				return true
			}
		}
	}
	return false
}

// valuesReveal is the per-value pass of the success funnel (hand-fix
// handfixes.json success-body-credential-funnel): every string, key and number
// of a JSON body, or the whole text of a body that is not JSON, goes through
// valueCheck.
func (c *Client) valuesReveal(body []byte, extra []string) error {
	check := c.valueCheck(extra)
	if !json.Valid(body) {
		return check(string(body))
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		var s string
		switch t := tok.(type) {
		case string:
			s = t
		case json.Number:
			s = string(t)
		default:
			continue
		}
		if err := check(s); err != nil {
			return err
		}
	}
}

// valueCheck checks one value. FIRST all whitespace and every removable
// character (control, Unicode format and zero-width, invalid UTF-8) is
// removed, so "WVdG aFlX RmhZ" reads as one run; THEN the size rule applies to
// what is left: within recursiveDecodeCap it gets the full recursive decoding
// (maskCredentialText: the client's decoder and the shared funnel), and a hit
// withholds the body (ErrSuccessBodyWithheld); over the cap the body is
// withheld (ErrSuccessBodyValueTooLarge). There is no partial scan for an
// oversized value: a partial scan can be beaten by composing encodings (HTML
// entities over base64 over base64, then padding). Text shorter than every
// configured credential cannot carry one in any decoded form and is skipped;
// a repeated text is checked once. The mask set is built once.
func (c *Client) valueCheck(extra []string) func(string) error {
	masks, secrets, held := c.credentialMaskSet(true, extra)
	if len(held) == 0 {
		return func(string) error { return nil }
	}
	shortest := len(held[0])
	for _, v := range held[1:] {
		if len(v) < shortest {
			shortest = len(v)
		}
	}
	seen := map[string]bool{}
	return func(v string) error {
		s := stripSpaceAndRemovable(v)
		if len(s) > recursiveDecodeCap {
			return ErrSuccessBodyValueTooLarge
		}
		if len(s) < shortest || seen[s] {
			return nil
		}
		seen[s] = true
		if maskCredentialTextWith(s, masks, secrets, held) != s {
			return ErrSuccessBodyWithheld
		}
		return nil
	}
}

// stripSpaceAndRemovable drops all whitespace and what stripRemovable drops.
func stripSpaceAndRemovable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// largeBodyNeedles is every lowercased needle largeBodyRevealsCredential looks
// for.
func (c *Client) largeBodyNeedles(extra []string) []string {
	masks, _, held := c.credentialMaskSet(true, extra)
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if n = strings.ToLower(n); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, m := range masks {
		add(m.needle)
	}
	for _, v := range held {
		if len(v) < minEncodedSecretLen {
			continue
		}
		for _, form := range []string{v, url.QueryEscape(v), url.PathEscape(v), credentialJSONSlash(v), string(credentialUTF16BE(v)), string(credentialUTF16LE(v))} {
			for _, n := range alignedBase64(form) {
				add(n)
			}
		}
	}
	return out
}

// alignedBase64 is s in standard and URL-safe base64 as it appears inside a
// longer blob, at each of the three byte alignments: only the characters that
// depend on s's bytes alone.
func alignedBase64(s string) []string {
	var out []string
	for k := 0; k < 3; k++ {
		buf := append(make([]byte, k), s...)
		start, end := (8*k+5)/6, 8*len(buf)/6
		if end-start < minEncodedSecretLen {
			continue
		}
		for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
			out = append(out, enc.EncodeToString(buf)[start:end])
		}
	}
	return out
}

// decodedJSONText is every string, key and number token of a JSON body,
// decoded, with removable characters dropped, NUL-separated.
func decodedJSONText(body []byte) string {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return b.String()
		}
		switch t := tok.(type) {
		case string:
			b.WriteString(stripRemovable(t))
			b.WriteByte(0)
		case json.Number:
			b.WriteString(string(t))
			b.WriteByte(0)
		}
	}
}

// lenientUnescape decodes every %XX, \xXX and \uXXXX escape in s, leaving
// anything else as written, in one pass.
func lenientUnescape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '%' && i+2 < len(s) && credentialIsHex(s[i+1]) && credentialIsHex(s[i+2]):
			b.WriteByte(unhex(s[i+1])<<4 | unhex(s[i+2]))
			i += 2
		case s[i] == '\\' && i+3 < len(s) && s[i+1] == 'x' && credentialIsHex(s[i+2]) && credentialIsHex(s[i+3]):
			b.WriteByte(unhex(s[i+2])<<4 | unhex(s[i+3]))
			i += 3
		case s[i] == '\\' && i+5 < len(s) && s[i+1] == 'u' && credentialIsHex(s[i+2]) && credentialIsHex(s[i+3]) && credentialIsHex(s[i+4]) && credentialIsHex(s[i+5]):
			b.WriteRune(rune(unhex(s[i+2]))<<12 | rune(unhex(s[i+3]))<<8 | rune(unhex(s[i+4]))<<4 | rune(unhex(s[i+5])))
			i += 5
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}
