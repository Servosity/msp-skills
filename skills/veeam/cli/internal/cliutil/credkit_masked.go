// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).

package cliutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaskedResponseFields are the response fields VSPC fills with a masked
// placeholder when it reads back a stored credential ("password": "••••••",
// also "******" and runs of U+FFFD, in the backup policy, discovery rule,
// guest OS credential and notification settings responses), and with the real
// secret elsewhere (GET /infrastructure/managementAgents/{uid}/credentials).
// credwire.Declarations leaves them out of ResponseFields, where the kit would
// hold the mask too, and HoldMaskedResponseFields holds them instead: every
// value but a mask, before any sink.
var MaskedResponseFields = []string{"password", "rootPassword", "sshPrivateKey"}

// ErrMaskedFieldUnread replaces a response body that names a MaskedResponseFields
// field the way a field is written but is no JSON document, so its values
// cannot be read one by one: the body is withheld whole (fail closed).
var ErrMaskedFieldUnread = errors.New("response withheld: it is not a JSON document and names a credential field (password, rootPassword or sshPrivateKey) whose values this program holds")

// IsCredentialMask reports whether v is a vendor's masked placeholder: valid
// UTF-8, three or more of one repeated character that is not a letter, a number
// or a mark ("••••••", "******", a run of U+FFFD). Anything else is a value,
// and is held.
func IsCredentialMask(v string) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) < 3 {
		return false
	}
	first, _ := utf8.DecodeRuneInString(v)
	if unicode.IsLetter(first) || unicode.IsNumber(first) || unicode.IsMark(first) {
		return false
	}
	for _, r := range v {
		if r != first {
			return false
		}
	}
	return true
}

// HoldMaskedResponseFields is the chokepoint every API response body passes
// through after the kit's transport and before anything reads it: the client
// (internal/client doInternal) and the Spacefast deliver sink. It holds every
// string (as decoded and as written) and number beneath a MaskedResponseFields
// name, at any depth, in arrays and nested objects, every member of a
// duplicate key, keys compared as the kit compares them (case folded, escapes
// decoded), skipping only a mask. It reports false when the body is no JSON
// document and still names one of the fields as a field is written (a name,
// an optional quote, then ':' or '='), in its bytes, its escape-decoded bytes,
// or its UTF-16 or UTF-32 reading when it declares one: the caller then
// withholds the body (ErrMaskedFieldUnread).
func HoldMaskedResponseFields(body []byte, contentType string) bool {
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	wide := declaresWide(body, contentType)
	if !wide && holdMaskedJSON(bytes.TrimPrefix(body, []byte("\xEF\xBB\xBF"))) {
		return true
	}
	return !namesMaskedField(body, wide)
}

// HoldMaskedResponseHeaders holds what a response's headers or trailers carry
// under a MaskedResponseFields name, where the kit holds a declared field: a
// header of that name, and a name=value or name: value pair inside any header
// value (a redirect Location's query or fragment), every value but a mask. The
// client calls it on the final response and, from CheckRedirect, on every
// redirect response before the next hop's URL can reach an error.
func HoldMaskedResponseHeaders(h http.Header) {
	for name, vs := range h {
		field := maskedFieldName(name)
		for _, v := range vs {
			if field != "" {
				holdUnlessMask(field, strings.TrimSpace(v))
			}
			for _, view := range []string{v, string(decodeEscapedBytes([]byte(v)))} {
				for _, m := range maskedFieldPair.FindAllStringSubmatch(view, -1) {
					name := maskedFieldName(m[1])
					for _, val := range m[2:] {
						val = strings.Trim(strings.TrimSpace(val), `"'`)
						holdUnlessMask(name, val)
						if u, err := url.QueryUnescape(val); err == nil {
							holdUnlessMask(name, u)
						}
					}
				}
			}
		}
	}
}

// maskedFieldPair is a MaskedResponseFields name, its separator, and its value
// twice: up to the first query, list or quote delimiter, and up to the end of
// the line or pair, as the kit's lenient reader takes a bare value.
var maskedFieldPair = func() *regexp.Regexp {
	names := make([]string, len(MaskedResponseFields))
	for i, n := range MaskedResponseFields {
		names[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`(?i)(` + strings.Join(names, "|") + `)\\?["']?\s*[:=]\s*(([^&;,#\s"'<>]*)[^\r\n,&}\]]*)`)
}()

func holdUnlessMask(field, v string) {
	if field != "" && v != "" && !IsCredentialMask(v) {
		Hold("response:"+field, v)
	}
}

// holdMaskedJSON holds the fields of b read as JSON values (one, or JSON
// lines). It reports false when b is not that; what it read before the error is
// held all the same.
func holdMaskedJSON(b []byte) bool {
	type frame struct {
		obj, wantKey bool
		under, key   string
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var stack []frame
	var prev int64
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return len(stack) == 0
		}
		if err != nil {
			return false
		}
		raw := b[prev:dec.InputOffset()]
		prev = dec.InputOffset()
		var f *frame
		if len(stack) > 0 {
			f = &stack[len(stack)-1]
		}
		under := ""
		if f != nil {
			under = f.under
			if under == "" && f.obj && !f.wantKey {
				under = maskedFieldName(f.key)
			}
		}
		switch t := tok.(type) {
		case json.Delim:
			if t == '{' || t == '[' {
				stack = append(stack, frame{obj: t == '{', wantKey: t == '{', under: under})
				continue
			}
			stack = stack[:len(stack)-1]
			f = nil
			if len(stack) > 0 {
				f = &stack[len(stack)-1]
			}
		case string:
			if f != nil && f.obj && f.wantKey {
				f.key, f.wantKey = t, false
				continue
			}
			if under != "" {
				written := writtenString(raw)
				if !(utf8.Valid(written) && IsCredentialMask(t)) {
					Hold("response:"+under, t, string(written))
				}
			}
		case json.Number:
			if under != "" {
				Hold("response:"+under, t.String())
			}
		}
		if f != nil && f.obj {
			f.wantKey = true
		}
	}
}

// writtenString is a string token as the body writes it, between its quotes.
func writtenString(raw []byte) []byte {
	raw = bytes.TrimLeft(raw, " \t\r\n,:")
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return raw[1 : len(raw)-1]
	}
	return raw
}

// maskedFieldName is the MaskedResponseFields name key is, compared case
// folded, or "".
func maskedFieldName(key string) string {
	for _, n := range MaskedResponseFields {
		if strings.EqualFold(n, key) {
			return n
		}
	}
	return ""
}

// maskedFieldKey is a MaskedResponseFields name written as a field's key:
// case folded, then an optional (escaped) quote, white space, and ':' or '='.
var maskedFieldKey = func() *regexp.Regexp {
	names := make([]string, len(MaskedResponseFields))
	for i, n := range MaskedResponseFields {
		names[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`(?i)(?:` + strings.Join(names, "|") + `)\\?["']?\s*[:=]`)
}()

// escapedByte is one escaped byte as a key in a lenient body may spell it.
var escapedByte = regexp.MustCompile(`\\u00[0-9A-Fa-f]{2}|\\x[0-9A-Fa-f]{2}|%[0-9A-Fa-f]{2}`)

func namesMaskedField(b []byte, wide bool) bool {
	if wide {
		b = bytes.ReplaceAll(b, []byte{0}, nil)
	}
	return maskedFieldKey.Match(b) || maskedFieldKey.Match(decodeEscapedBytes(b))
}

// decodeEscapedBytes is b with every escapedByte decoded to its byte.
func decodeEscapedBytes(b []byte) []byte {
	return escapedByte.ReplaceAllFunc(b, func(m []byte) []byte {
		var v byte
		for _, c := range m[len(m)-2:] {
			v = v<<4 | unhex(c)
		}
		return []byte{v}
	})
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

// declaresWide reports whether b is read as UTF-16 or UTF-32: a byte-order
// mark, or a charset the Content-Type declares.
func declaresWide(b []byte, contentType string) bool {
	for _, bom := range [][]byte{{0xFE, 0xFF}, {0xFF, 0xFE}, {0, 0, 0xFE, 0xFF}} {
		if bytes.HasPrefix(b, bom) {
			return true
		}
	}
	_, params, _ := mime.ParseMediaType(contentType)
	cs := strings.ToLower(params["charset"])
	return strings.HasPrefix(cs, "utf-16") || strings.HasPrefix(cs, "utf-32") || strings.HasPrefix(cs, "ucs-")
}
