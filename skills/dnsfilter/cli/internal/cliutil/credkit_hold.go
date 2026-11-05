// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// Every read of a config file, at startup or later (the MCP server reloads the
// file on every tool call), holds every value in it with the credential kit
// BEFORE the file is parsed, so neither a parse diagnostic nor anything the
// program does with the result can carry a value the kit has not seen. The
// kit's startup discovery holds the file it finds once; a file written after
// that is held here, when it is read.

package cliutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"dnsfilter-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Hold is credkit.Hold. Production code never reassigns it. In-process unit tests
// do, to a no-op: a held value lasts for the process and the kit withholds
// whatever carries it, so one test's fixture would withhold the next test's
// output. The built binaries' leak tests (internal/credwire) prove the real thing.
var Hold = credkit.Hold

// CredentialConfigKeys are the keys of config.toml and credentials.toml whose
// value is a credential whatever characters it is made of.
var CredentialConfigKeys = []string{"auth_header", "api_key", "access_token", "refresh_token", "client_id", "client_secret"}

// CredentialResponseFields are the DNSFilter spec's credential-valued members.
// The kit holds each as a response passes its transport; the client then
// replaces each by name (handfixes.json credential-fields-withheld), so a
// listing that carries one (networks rows carry secret_key) stays usable. Three
// names are left out on purpose: key (a name/value record's name),
// entitlement_key (a plan feature's name) and token_type (always "Bearer").
var CredentialResponseFields = []string{
	"access_token", "api_key", "client_secret", "ep-grd-subscriber-secret", "ep_grd_subscriber_secret",
	"id_token", "new_password", "password", "payment_token", "refresh_token", "reset_password_url",
	"secret_key", "token",
}

// NotCredentials are the top-level keys whose values are not credentials.
// internal/credwire declares each as a credkit Identifier (config:<key>), so
// the kit exempts them too; a credential inside base_url is still held, as
// userinfo and query:<name>.
var NotCredentials = []string{"base_url", "ca_cert", "tls_server_name", "token_expiry", "logged_out"}

func notCredential(section, key string) bool {
	if section != "" {
		return false
	}
	for _, k := range NotCredentials {
		if key == k {
			return true
		}
	}
	return false
}

// HoldConfigFile holds every value in data: first as the text is written, by a
// whole-file scan that trusts no parse (unknown keys, tables, inline tables,
// unterminated and multi-line strings, bare words and lines the parser rejects),
// then, when the file parses, each value as the parser decodes it.
func HoldConfigFile(data []byte) {
	for _, v := range scanConfigValues(string(data)) {
		holdConfigValue(v.source, v.value)
	}
	var tree map[string]any
	if toml.Unmarshal(data, &tree) == nil {
		holdDecoded("", tree)
	}
}

type configValue struct{ source, value string }

// credentialSource reports a source whose value is held whatever it looks like.
func credentialSource(source string) bool {
	if strings.HasPrefix(source, "header:") {
		return true
	}
	key, ok := strings.CutPrefix(source, "config:")
	if !ok {
		return false
	}
	for _, k := range CredentialConfigKeys {
		if key == k {
			return true
		}
	}
	return false
}

// holdConfigValue holds v under source, exactly as written (the kit adds its
// trimmed form). Outside the credential keys, a boolean literal and a short
// value with no letter or digit (the [] or {} of a line that did not parse)
// carry no credential and are not held.
func holdConfigValue(source, v string) {
	t := strings.TrimSpace(v)
	if t == "" {
		return
	}
	if !credentialSource(source) && (t == "true" || t == "false" || len(t) < 8 && strings.IndexFunc(t, func(r rune) bool {
		return r > 0x7f || unicode.IsLetter(r) || unicode.IsDigit(r)
	}) < 0) {
		return
	}
	Hold(credkit.Source(source), v)
	if strings.Contains(t, "://") {
		holdBaseURL(t) // a URL under any key gives up what it carries
	}
}

// holdBaseURL holds the credentials inside a base URL as it is written, whether
// or not it parses: the password (the userinfo runs to the last @ before any ?
// or #, so a raw / or @ in it is covered), a userinfo with no password as a whole
// (a key written in the username slot), and every query and fragment value.
func holdBaseURL(raw string) {
	raw = strings.TrimSpace(raw)
	head, tail := raw, ""
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		head, tail = raw[:i], raw[i+1:]
	}
	if _, rest, ok := strings.Cut(head, "://"); ok {
		head = rest
	}
	if i := strings.LastIndexByte(head, '@'); i >= 0 {
		user, p, hasPassword := strings.Cut(head[:i], ":")
		if !hasPassword {
			p = user
		}
		holdConfigValue("userinfo", p)
		if d, err := url.PathUnescape(p); err == nil {
			holdConfigValue("userinfo", d)
		}
	}
	for _, pair := range strings.FieldsFunc(tail, func(r rune) bool { return r == '&' || r == '#' || r == '?' }) {
		k, v, hasValue := strings.Cut(pair, "=")
		if !hasValue {
			v = k // a bare ?KEY or #KEY is the value
		}
		holdConfigValue("query:"+k, v)
		if d, err := url.QueryUnescape(v); err == nil {
			holdConfigValue("query:"+k, d)
		}
	}
}

// sourceFor is the kit source a value under key in section is held as:
// header:<name> for a [headers] entry (a table, a dotted key or an inline
// table), config:<key> for a top-level key, config:<section>.<key> else.
func sourceFor(section, key string) string {
	if section == "headers" {
		return "header:" + strings.ToLower(key)
	}
	if h, ok := strings.CutPrefix(key, "headers."); ok && section == "" {
		return "header:" + strings.ToLower(h)
	}
	if section == "" {
		return "config:" + key
	}
	return "config:" + section + "." + key
}

// scanConfigValues reads every value out of a config file's text without
// parsing it: no size cutoff, every line, each quoted string (an unterminated
// one runs to the end of its line; a multi-line one to its closing delimiter),
// else the bare words of a value, and every word of a line that is not an
// assignment. An exempt top-level key is skipped only when its value is one
// plain scalar; base_url then gives up its userinfo and query values.
func scanConfigValues(text string) []configValue {
	var out []configValue
	section, multi, multiSource := "", "", ""
	var multiLines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if multi != "" {
			before, after, closed := strings.Cut(line, multi)
			out = append(out, configValue{multiSource, before})
			multiLines = append(multiLines, before)
			if closed {
				out = append(out, wholeMultiLine(multiSource, multiLines)...)
				multi, multiLines, line = "", nil, after
			}
			// A delimiter never closed runs over the rest of the file, whose
			// lines are still read below for the assignments they hold.
		}
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		eq := unquotedEquals(line)
		if line[0] == '[' && eq < 0 {
			section = strings.ToLower(strings.Trim(line, "[] \t"))
			continue
		}
		if eq < 0 {
			toks, open := lexValue(line)
			for _, t := range toks {
				if t.kind != punct {
					out = append(out, configValue{"config:unclassified", t.text})
				}
			}
			if open != "" && multi == "" {
				multi, multiSource, multiLines = open, "config:unclassified", []string{toks[len(toks)-1].text}
			}
			continue
		}
		key, rhs := strings.Trim(strings.TrimSpace(line[:eq]), `"'`), line[eq+1:]
		toks, open := lexValue(rhs)
		if notCredential(section, key) && plainScalar(toks) {
			if key == "base_url" && len(toks) == 1 {
				holdBaseURL(toks[0].text)
			}
			continue
		}
		source := sourceFor(section, key)
		quoted := false
		for _, t := range toks {
			quoted = quoted || t.kind == quotedTok
		}
		for i, t := range toks {
			if t.kind == punct || t.kind == bareTok && quoted && !afterEquals(toks, i) {
				continue
			}
			src := source
			if i >= 2 && toks[i-1].text == "=" && toks[i-1].kind == punct {
				// An inline table: each of its own keys names its value.
				if key == "headers" && section == "" {
					src = "header:" + strings.ToLower(toks[i-2].text)
				} else {
					src = source + "." + toks[i-2].text
				}
			}
			if i+1 < len(toks) && toks[i+1].kind == punct && toks[i+1].text == "=" {
				continue // an inline table's key
			}
			out = append(out, configValue{src, t.text})
			if t.kind == quotedTok {
				if u := unescapeBasic(t.text); u != t.text {
					out = append(out, configValue{src, u})
				}
			}
		}
		if open != "" && multi == "" {
			multi, multiSource, multiLines = open, source, []string{toks[len(toks)-1].text}
		}
	}
	return append(out, wholeMultiLine(multiSource, multiLines)...) // never closed: it runs to the end
}

// wholeMultiLine is a multi-line string's complete value, as its lines join and
// with a basic string's escapes and line-ending backslashes undone: its lines
// alone can each be exempt (true, false, null) while the value they make is not.
func wholeMultiLine(source string, lines []string) []configValue {
	if len(lines) == 0 {
		return nil
	}
	var out []configValue
	decoded := make([]string, len(lines))
	for i, l := range lines {
		decoded[i] = unescapeBasic(l)
	}
	for _, eol := range []string{"\n", "\r\n"} { // the scan took the line ends off
		joined := strings.Join(lines, eol)
		out = append(out, configValue{source, joined}, configValue{source, strings.Join(decoded, eol)})
		if u := lineEndingBackslash.ReplaceAllString(joined, ""); u != joined {
			out = append(out, configValue{source, u}, configValue{source, unescapeBasic(u)})
		}
	}
	return out
}

var lineEndingBackslash = regexp.MustCompile(`\\[ \t]*\r?\n[ \t\r\n]*`)

func afterEquals(toks []tok, i int) bool {
	return i >= 1 && toks[i-1].kind == punct && toks[i-1].text == "="
}

// unquotedEquals is the index of the first = in line outside a quoted string, or -1.
func unquotedEquals(line string) int {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote == 0 && c == '=':
			return i
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == '"' && c == '\\':
			i++
		case c == quote:
			quote = 0
		}
	}
	return -1
}

// plainScalar reports one quoted string or one bare word, or nothing at all.
func plainScalar(toks []tok) bool {
	return len(toks) == 0 || len(toks) == 1 && toks[0].kind != punct
}

const (
	bareTok = iota
	quotedTok
	punct
)

type tok struct {
	text string
	kind int
}

// lexValue splits the value text of one line into quoted strings (a \" does
// not end a basic string), bare words and the punctuation between them. A # at
// the start of a token outside a string ends the line. An unterminated string
// runs to the end of the line; open is the delimiter of a multi-line string
// left open there.
func lexValue(rhs string) (toks []tok, open string) {
	bareStart := -1
	flush := func(end int) {
		if bareStart >= 0 {
			toks = append(toks, tok{rhs[bareStart:end], bareTok})
			bareStart = -1
		}
	}
	for i := 0; i < len(rhs); {
		c := rhs[i]
		switch {
		case c == '"' || c == '\'':
			flush(i)
			delim := rhs[i : i+1]
			if strings.HasPrefix(rhs[i:], strings.Repeat(delim, 3)) {
				delim = strings.Repeat(delim, 3)
			}
			body := rhs[i+len(delim):]
			end := closingDelim(body, delim)
			if end < 0 {
				toks = append(toks, tok{body, quotedTok})
				if len(delim) == 3 {
					open = delim
				}
				return toks, open
			}
			toks = append(toks, tok{body[:end], quotedTok})
			i += 2*len(delim) + end
		case strings.IndexByte(" \t{}[],=", c) >= 0:
			flush(i)
			if c != ' ' && c != '\t' {
				toks = append(toks, tok{rhs[i : i+1], punct})
			}
			i++
		case c == '#' && bareStart < 0:
			return toks, ""
		default:
			if bareStart < 0 {
				bareStart = i
			}
			i++
		}
	}
	flush(len(rhs))
	return toks, ""
}

// closingDelim is the index of delim in body, honoring a basic string's backslash.
func closingDelim(body, delim string) int {
	if delim[0] != '"' {
		return strings.Index(body, delim)
	}
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' {
			i++
			continue
		}
		if strings.HasPrefix(body[i:], delim) {
			return i
		}
	}
	return -1
}

// unescapeBasic undoes a basic string's escapes when they read as one, for a
// file the parser rejects (a file it accepts is held as decoded too).
func unescapeBasic(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	if u, err := strconv.Unquote(`"` + s + `"`); err == nil {
		return u
	}
	return strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(s)
}

// HoldProfileStore holds what a run profile store carries (contract: a value under a
// credential flag's name or under base-url), reading its tokens in order so that a
// duplicate member, a value nested under the name and whatever came before a syntax
// error all count. A store that does not parse is read again by name. Other values are
// no source: a profile's free text and plain "true" stay out of the held set.
func HoldProfileStore(data []byte) {
	type frame struct {
		object, atKey, credential bool
		name                      string
	}
	var stack []*frame
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			holdUnparsedProfiles(data)
			return
		}
		var top *frame
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		if d, ok := tok.(json.Delim); ok && (d == '}' || d == ']') {
			stack = stack[:len(stack)-1]
			if len(stack) > 0 {
				stack[len(stack)-1].atKey = stack[len(stack)-1].object
			}
			continue
		}
		if top != nil && top.atKey {
			top.name, _ = tok.(string)
			top.atKey = false
			continue
		}
		name, credential := "", false
		if top != nil {
			name, credential = top.name, top.credential || credentialName(top.name)
		}
		switch x := tok.(type) {
		case json.Delim:
			stack = append(stack, &frame{object: x == '{', atKey: x == '{', credential: credential, name: name})
			continue
		case json.Number:
			if credential {
				Hold(credkit.Source("field:"+name), x.String())
			}
		case string:
			holdProfileValue(name, credential, x)
		}
		if top != nil {
			top.atKey = top.object
		}
	}
}

func holdProfileValue(name string, credential bool, v string) {
	switch {
	case v == "":
	case credential:
		Hold(credkit.Source("field:"+name), v)
	case name == "base-url" || name == "base_url":
		holdBaseURL(v)
	}
}

// holdUnparsedProfiles reads a store with no parser to tell its names: the word after a
// credential's name or base-url is held. A key (a word before ":" or "=") is no value.
func holdUnparsedProfiles(data []byte) {
	key := ""
	for _, m := range bodyWord.FindAllSubmatchIndex(data, -1) {
		word := string(data[m[0]:m[1]])
		switch {
		case m[2] >= 0:
			word = string(data[m[2]:m[3]])
		case m[4] >= 0:
			word = string(data[m[4]:m[5]])
		}
		if rest := bytes.TrimLeft(data[m[1]:], " \t\r\n"); len(rest) > 0 && (rest[0] == ':' || rest[0] == '=') {
			key = word
			continue
		}
		holdProfileValue(key, credentialName(key), word)
		if credentialName(key) {
			Hold(credkit.Source("field:"+key), unescapeBasic(word))
		}
	}
}

var bodyWord = regexp.MustCompile(`"((?:[^"\\\n]|\\.)*)"|'((?:[^'\\\n]|\\.)*)'|[^\s"',:;={}\[\]]+`)

// credentialName is a declared credential's name, as a field or as a flag.
func credentialName(name string) bool {
	n := strings.ReplaceAll(strings.ToLower(name), "-", "_")
	if slices.Contains(CredentialConfigKeys, n) || slices.Contains(CredentialResponseFields, n) {
		return true
	}
	return strings.HasSuffix(n, "_password") || strings.HasSuffix(n, "_secret") || strings.HasSuffix(n, "_token")
}

// holdDecoded holds each leaf of a parsed config as the parser decoded it.
func holdDecoded(section string, tree map[string]any) {
	for k, v := range tree {
		switch {
		case section == "" && k == "headers":
			if m, ok := v.(map[string]any); ok {
				for name, hv := range m {
					holdLeaf("header:"+strings.ToLower(name), hv)
				}
				continue
			}
		case notCredential(section, k):
			switch s := v.(type) {
			case string:
				if k == "base_url" {
					holdBaseURL(s)
				}
				continue
			case bool, fmt.Stringer:
				continue // logged_out, token_expiry
			}
		}
		if m, ok := v.(map[string]any); ok {
			holdDecoded(strings.TrimPrefix(section+"."+k, "."), m)
			continue
		}
		holdLeaf(sourceFor(section, k), v)
	}
}

func holdLeaf(source string, v any) {
	switch x := v.(type) {
	case string:
		holdConfigValue(source, x)
	case int64:
		holdConfigValue(source, strconv.FormatInt(x, 10))
	case float64:
		holdConfigValue(source, strconv.FormatFloat(x, 'g', -1, 64))
	case bool:
	case []any:
		for _, e := range x {
			holdLeaf(source, e)
		}
	case map[string]any:
		for k, e := range x {
			holdLeaf(source+"."+k, e)
		}
	default:
		holdConfigValue(source, fmt.Sprint(x))
	}
}
