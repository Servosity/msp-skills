// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contract 1's body fields and URLs, for a connector that meets one outside
// the kit's own sources: a dry-run preview, an import, a profile field that
// holds a URL. They are the kit's own readers, so a connector needs no parser
// of its own. Neither scans free text, which section 1 does not name: pass a
// document to HoldBody and one URL to HoldURL.

import (
	"bytes"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"unicode"
)

// bodyFields are held in a body whatever a connector declares.
var bodyFields = []string{"auth_header", "authorization", "proxy-authorization", "cookie"}

// HoldBody holds the credentials in a document as a request body's are read:
// every member of a duplicate key, numbers as written, nested objects, under
// the declared CredentialFields, ConfigFields, StoredRecords, MCPArguments and
// the built-in names. Call it before anything prints the document. Bytes that
// parse as no document are read as a credential file that does not parse is:
// every string in them is held, so what they say may be withheld.
func HoldBody(b []byte) {
	if len(b) > maxDeclared {
		markUnknown() // limits.go: too large to read is not read clean
		return
	}
	rs, ok := readings(b, nil)
	reg.Lock()
	defer reg.Unlock()
	d := &reg.decl
	names := slices.Concat(d.CredentialFields, d.ConfigFields, d.StoredRecords, d.MCPArguments, bodyFields)
	for _, r := range rs {
		holdFieldsLocked(r, names, "field:", nil)
	}
	if !ok || !slices.ContainsFunc(rs, documentParses) { // any reading: a byte-order mark is one
		for _, r := range rs {
			holdEveryStringLocked(r)
			holdEveryTokenLocked(r)
		}
	}
}

// holdEveryTokenLocked holds what holdEveryStringLocked leaves as no credential
// of a file's (a number, punctuation, structure), which in bytes that are no
// document may be one: the bytes whole, so every piece of 8 characters is held,
// and each shorter run between white space and structure. One character is left,
// as a value of one would withhold everything, its own notice included.
func holdEveryTokenLocked(b []byte) {
	holdLocked("field:unparsed", string(b))
	for _, t := range strings.FieldsFunc(string(b), func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("{}[],:=\"'", r)
	}) {
		if len(t) > 1 {
			holdLocked("field:unparsed", t)
		}
	}
}

// documentParses reports whether b is one document the field reader reads the
// one way: JSON, JSON lines, a config of the canonical subset, or a form.
func documentParses(b []byte) bool {
	const space = " \t\r\n" // JSON's: the readers take no other for white space
	b = bytes.Trim(b, space)
	if len(b) == 0 || json.Valid(b) {
		return true
	}
	// A config or a form outside ASCII: a key the reader would not know for one.
	if isASCIIBytes(b) && (preciseConfig(b) || formShaped(b)) {
		return true
	}
	for line := range bytes.SplitSeq(b, []byte{'\n'}) {
		if line = bytes.Trim(line, space); len(line) > 0 && !json.Valid(line) {
			return false
		}
	}
	return true
}

// formShaped: one line of name=value pairs, each escape valid.
func formShaped(b []byte) bool {
	s := string(b)
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	for p := range strings.SplitSeq(s, "&") {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return false
		}
		if _, err := url.QueryUnescape(k); err != nil {
			return false
		}
		if _, err := url.QueryUnescape(v); err != nil {
			return false
		}
	}
	return true
}

// HoldURL holds the password and every query value of one URL, under source,
// as a base URL's are (a query name a declared Identifier exempts is left, by
// its decoded name): a full URL, a schemeless authority
// (ops:secret@proxy.example:3128), a network path (//ops:secret@host/), and one
// that does not parse. It reads what a browser's fragment parameters carry too.
func HoldURL(source Source, raw string) {
	reg.Lock()
	defer reg.Unlock()
	holdURLLocked(source, raw)
	if t := strings.TrimSpace(raw); t != raw {
		holdURLLocked(source, t)
	}
}

func holdURLLocked(source Source, raw string) {
	pairs := func(q string) {
		for k, vs := range formPairs(q) {
			name, err := url.QueryUnescape(k)
			if err != nil {
				name = k
			}
			if !exemptLocked("query:" + name) {
				holdLocked(source, vs...)
			}
		}
	}
	u, err := parseProxyEnv(raw)
	if strings.HasPrefix(raw, "//") {
		u, err = url.Parse(raw) // a network path: a URL with no scheme
	}
	if err == nil {
		if p, ok := u.User.Password(); ok {
			holdLocked(source, p)
			if _, esc, ok := strings.Cut(u.User.String(), ":"); ok {
				holdLocked(source, esc)
			}
		}
		pairs(u.RawQuery)
		pairs(u.EscapedFragment())
		return
	}
	if _, p, ok := rawUserinfo(raw); ok && p != "" {
		holdLocked(source, p)
		if u, err := url.PathUnescape(p); err == nil {
			holdLocked(source, u)
		}
	}
	s, frag, _ := strings.Cut(raw, "#")
	if _, q, ok := strings.Cut(s, "?"); ok {
		pairs(q)
	}
	pairs(frag)
}
