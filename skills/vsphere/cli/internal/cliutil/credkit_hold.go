// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// Every read of a config file, at startup or later (the MCP server reloads
// the file on every tool call), holds every value in it with the credential
// kit BEFORE the file is parsed, so neither a parse diagnostic nor anything
// the program does with the result can carry a value the kit has not seen.
// The kit's startup discovery holds the file it finds once; a file written
// after that is held here, when it is read. A URL is read by the kit's own
// reader, credkit.HoldURL, never by one of this connector's.

package cliutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"vsphere-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// NotCredentials are the top-level keys whose values are not credentials.
// internal/credwire declares each as a credkit Identifier (config:<key>), so
// the kit exempts them too; a credential inside base_url (its password and
// query values) is still held, as userinfo and query:<name>. username is the
// vCenter SSO account name, which vCenter's own answers repeat.
var NotCredentials = []string{"base_url", "ca_cert", "tls_server_name", "token_expiry", "username"}

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

// HoldConfigFile holds every value in data: first as the text is written,
// by a whole-file scan that trusts no parse (unknown keys, tables, inline
// tables, unterminated and multi-line strings, bare words and lines the
// parser rejects), then, when the file parses, each value as the parser
// decodes it (escapes undone). A base_url gives up what credkit.HoldURL reads
// in it: its password and every query value.
func HoldConfigFile(data []byte) { holdConfig(data, false) }

// HoldConfigBaseURLs holds what each base_url in data carries, as
// HoldConfigFile does, and nothing else. Startup runs it on every file the kit
// reads: the kit's own base URL reader misses a password in an address with a
// typo (https//u:pw@h), which credkit.HoldURL reads.
func HoldConfigBaseURLs(data []byte) { holdConfig(data, true) }

func holdConfig(data []byte, baseURLOnly bool) {
	for _, v := range scanConfigValues(string(data)) {
		switch {
		case v.source == baseURLSource:
			credkit.HoldURL("userinfo", v.value)
		case !baseURLOnly:
			holdConfigValue(v.source, v.value)
		}
	}
	var tree map[string]any
	if toml.Unmarshal(data, &tree) == nil {
		holdDecoded("", tree, baseURLOnly)
	}
}

// baseURLSource is the source a top-level base_url value is scanned under.
const baseURLSource = "config:base_url"

type configValue struct{ source, value string }

// credentialSources are the config keys whose value is a credential whatever
// characters it is made of.
var credentialSources = map[string]bool{
	"config:auth_header": true, "config:password": true,
	"config:access_token": true, "config:refresh_token": true, "config:client_id": true, "config:client_secret": true,
}

// holdConfigValue holds v under source. Outside the credential keys, a
// boolean literal and a short value with no letter or digit (the [] or {} of
// a line that did not parse) carry no credential and are not held.
func holdConfigValue(source, v string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	if !credentialSources[source] && (v == "true" || v == "false" || len(v) < 8 && strings.IndexFunc(v, func(r rune) bool {
		return r > 0x7f || unicode.IsLetter(r) || unicode.IsDigit(r)
	}) < 0) {
		return
	}
	credkit.Hold(credkit.Source(source), v)
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
// parsing it (the retired hold_values.go tokenizer, with kit sources): no
// size cutoff, every line, each quoted string (an unterminated one runs to
// the end of its line; a multi-line one to its closing delimiter), else the
// bare words of a value, and every word of a line that is not an assignment.
// An exempt top-level key is skipped only when its value is one plain
// scalar, but for base_url, whose value is returned to be read as a URL.
func scanConfigValues(text string) []configValue {
	var out []configValue
	section, multi, multiSource := "", "", ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if multi != "" { // inside a multi-line string
			before, _, closed := strings.Cut(line, multi)
			out = append(out, configValue{multiSource, before})
			if closed {
				multi = ""
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && !strings.Contains(line, "=") {
			section = strings.ToLower(strings.Trim(line, "[] \t"))
			continue
		}
		key, rhs, ok := strings.Cut(line, "=")
		if !ok {
			vals, _ := configValueParts(line)
			for _, v := range append(vals, strings.Fields(line)...) {
				out = append(out, configValue{"config:unclassified", v})
			}
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		vals, open := configValueParts(rhs)
		source := sourceFor(section, key)
		if notCredential(section, key) && len(vals) == 1 && plainScalar(rhs) {
			if source == baseURLSource {
				out = append(out, configValue{source, vals[0]})
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(rhs), "{") {
			// An inline table: each of its own keys names its value.
			for _, kv := range inlinePairs(rhs) {
				inner := kv.source
				if key == "headers" && section == "" {
					out = append(out, configValue{"header:" + strings.ToLower(inner), kv.value})
				} else {
					out = append(out, configValue{source + "." + inner, kv.value})
				}
			}
		}
		for _, v := range vals {
			out = append(out, configValue{source, v})
		}
		if open != "" {
			multi, multiSource = open, source
		}
	}
	return out
}

// plainScalar reports whether rhs is one quoted string or one bare word,
// with nothing after it but a comment.
func plainScalar(rhs string) bool {
	rhs = strings.TrimSpace(rhs)
	if rhs == "" {
		return false
	}
	if q := rhs[0]; q == '"' || q == '\'' {
		if strings.HasPrefix(rhs, strings.Repeat(string(q), 3)) {
			return false
		}
		end := strings.IndexByte(rhs[1:], q)
		if end < 0 {
			return false
		}
		rest := strings.TrimSpace(rhs[end+2:])
		return rest == "" || strings.HasPrefix(rest, "#")
	}
	word, rest, _ := strings.Cut(rhs, " ")
	rest = strings.TrimSpace(rest)
	return !strings.ContainsAny(word, "{}[],=\t") && (rest == "" || strings.HasPrefix(rest, "#"))
}

// inlinePairs is each key = value of an inline table as written.
func inlinePairs(rhs string) []configValue {
	body := strings.TrimSpace(rhs)
	body = strings.TrimPrefix(body, "{")
	if i := strings.LastIndexByte(body, '}'); i >= 0 {
		body = body[:i]
	}
	var out []configValue
	for _, part := range strings.Split(body, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		vals, _ := configValueParts(v)
		for _, x := range vals {
			out = append(out, configValue{strings.Trim(strings.TrimSpace(k), `"'`), x})
		}
	}
	return out
}

// configValueParts returns every quoted string in rhs (an unterminated one
// runs to the end of the line) and, when there is none, its bare words. open
// is the delimiter of a multi-line string left open at the end of the line.
func configValueParts(rhs string) (vals []string, open string) {
	rest := rhs
	for {
		i := strings.IndexAny(rest, `"'`)
		if i < 0 {
			break
		}
		delim := rest[i : i+1]
		if strings.HasPrefix(rest[i:], strings.Repeat(delim, 3)) {
			delim = strings.Repeat(delim, 3)
		}
		body := rest[i+len(delim):]
		end := strings.Index(body, delim)
		if end < 0 {
			vals = append(vals, body)
			if len(delim) == 3 {
				open = delim
			}
			return vals, open
		}
		vals = append(vals, body[:end])
		rest = body[end+len(delim):]
	}
	if len(vals) == 0 {
		vals = strings.FieldsFunc(rhs, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '{' || r == '}' || r == '[' || r == ']' || r == ',' || r == '='
		})
	}
	return vals, ""
}

// holdDecoded holds each leaf of a parsed config as the parser decoded it, or
// with baseURLOnly only what a base_url carries.
func holdDecoded(section string, tree map[string]any, baseURLOnly bool) {
	for k, v := range tree {
		if section == "" && k == "base_url" {
			if s, ok := v.(string); ok {
				credkit.HoldURL("userinfo", s)
				continue
			}
		}
		if baseURLOnly {
			continue
		}
		switch {
		case section == "" && k == "headers":
			if m, ok := v.(map[string]any); ok {
				for name, hv := range m {
					holdLeaf("header:"+strings.ToLower(name), hv)
				}
				continue
			}
		case notCredential(section, k):
			if _, ok := v.(string); ok {
				continue
			}
		}
		if m, ok := v.(map[string]any); ok {
			holdDecoded(strings.TrimPrefix(section+"."+k, "."), m, false)
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

// HoldRunProfileStore holds what a run-profile store (profiles.json) carries as
// a source of contract section 1, every member of a duplicate key included: a
// value saved under a flag a run profile does not apply (one this CLI does not
// have, or a credential flag's name; allowed says which), and what a value saved
// under a base-url flag carries, read by credkit.HoldURL. A store that does not
// parse as the program reads it is held by credkit.HoldBody. Free text (a
// description, a URL in a --deliver value) is no source and is not read.
func HoldRunProfileStore(data []byte, allowed func(flag string) bool) {
	var store struct {
		Profiles map[string]struct {
			Values map[string]string `json:"values"`
		} `json:"profiles"`
	}
	if json.Unmarshal(data, &store) != nil {
		credkit.HoldBody(data)
		return
	}
	eachProfileValue(data, func(flag, value string) {
		switch {
		case value == "":
		case !allowed(flag):
			credkit.Hold("minted:profile", value)
		case strings.Contains(strings.ReplaceAll(strings.ToLower(flag), "_", "-"), "base-url"):
			credkit.HoldURL("minted:profile", value)
		}
	})
}

// eachProfileValue calls f for every flag and value under profiles.<name>.values
// in data, which json.Unmarshal has already read, a key repeated at any level
// included: Unmarshal keeps only the last of each.
func eachProfileValue(data []byte, f func(flag, value string)) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func(path []string)
	walk = func(path []string) {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch tok {
		case json.Delim('{'):
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return
				}
				name, _ := key.(string)
				walk(append(path, name))
			}
			_, _ = dec.Token()
		case json.Delim('['):
			for dec.More() {
				walk(append(path, ""))
			}
			_, _ = dec.Token()
		default:
			// Unmarshal matches a member name to its field ignoring case.
			if s, ok := tok.(string); ok && len(path) == 4 && strings.EqualFold(path[0], "profiles") && strings.EqualFold(path[2], "values") {
				f(path[3], s)
			}
		}
	}
	walk(nil)
}
