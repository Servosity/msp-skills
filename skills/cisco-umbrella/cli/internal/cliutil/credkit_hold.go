// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// Every read of a config file, at startup or later (the MCP server reloads
// the file on every tool call), holds every value in it with the credential
// kit BEFORE the file is parsed, so neither a parse diagnostic nor anything
// the program does with the result can carry a value the kit has not seen.
// The kit's startup discovery holds the file it finds once; a file written
// after that is held here, when it is read.

package cliutil

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"cisco-umbrella-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// NotCredentials are the top-level keys whose values are not credentials.
// internal/credwire declares each as a credkit Identifier (config:<key>), so
// the kit exempts them too; a credential inside base_url (its password and
// query values) is still held, as userinfo and query:<name>. client_id is not
// here: Cisco's client ID is the public half of a credential pair.
var NotCredentials = []string{"base_url", "ca_cert", "tls_server_name", "token_url", "token_expiry"}

func notCredential(section, key string) bool {
	return section == "" && slices.Contains(NotCredentials, key)
}

// HoldConfigFile holds every value in data: first as the text is written,
// by a whole-file scan that trusts no parse (unknown keys, tables, inline
// tables, unterminated and multi-line strings, bare words and lines the
// parser rejects), then, when the file parses, each value as the parser
// decodes it (escapes undone).
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

// holdConfigValue holds v under source as written, whatever characters it is
// made of: there is no filter on its class or length, and the kit holds it
// trimmed too. Empty is not a value.
func holdConfigValue(source, v string) {
	if v == "" {
		return
	}
	Hold(credkit.Source(source), v)
}

// bareBoolean reports a TOML boolean literal assigned to a key that names no
// credential: a setting (unknown_switch = true), and the one thing a config
// can hold that is not text. Holding it would withhold every JSON record that
// has a true in it. A boolean literal under a credential key or [headers], or
// any quoted "true", is held like every other value.
func bareBoolean(source, rhs string) bool {
	if strings.HasPrefix(source, "header:") || slices.Contains(CredentialKeys, strings.TrimPrefix(source, "config:")) {
		return false
	}
	word, _, _ := strings.Cut(strings.TrimSpace(rhs), "#")
	word = strings.TrimSpace(word)
	return word == "true" || word == "false"
}

// CredentialKeys are the config keys that name a credential; internal/credwire
// declares the same list as ConfigFields and StoredRecords.
var CredentialKeys = []string{"auth_header", "client_id", "client_secret", "access_token", "refresh_token", "umbrella_api_key", "umbrella_api_secret"}

// holdBaseURL holds the credentials inside a base URL: its password and
// every query value, also when the URL does not parse.
func holdBaseURL(raw string) {
	raw = strings.TrimSpace(raw)
	query := ""
	if u, err := url.Parse(raw); err == nil {
		if u.User != nil {
			if p, ok := u.User.Password(); ok {
				holdConfigValue("userinfo", p)
			}
		}
		query = u.RawQuery
	} else if _, q, ok := strings.Cut(raw, "?"); ok {
		query, _, _ = strings.Cut(q, "#")
	}
	if m := userinfoPattern.FindStringSubmatch(raw); m != nil {
		if _, p, ok := strings.Cut(m[1], ":"); ok {
			holdConfigValue("userinfo", p)
			if d, err := url.PathUnescape(p); err == nil {
				holdConfigValue("userinfo", d)
			}
		}
	}
	for _, pair := range strings.Split(query, "&") {
		k, v, _ := strings.Cut(pair, "=")
		holdConfigValue("query:"+k, v)
		if d, err := url.QueryUnescape(v); err == nil {
			holdConfigValue("query:"+k, d)
		}
	}
}

var userinfoPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://([^/?#]*)@`)

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
// scalar; base_url then gives up its password and query values.
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
		if notCredential(section, key) && len(vals) == 1 && plainScalar(rhs) {
			if key == "base_url" {
				holdBaseURL(vals[0])
			}
			continue
		}
		source := sourceFor(section, key)
		if bareBoolean(source, rhs) {
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
			switch t := v.(type) {
			case map[string]any, []any: // not the plain scalar this key should be: held below
			case string:
				if k == "base_url" {
					holdBaseURL(t)
				}
				continue
			default: // token_expiry decodes to a time
				continue
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
		if strings.HasPrefix(source, "header:") || slices.Contains(CredentialKeys, strings.TrimPrefix(source, "config:")) {
			holdConfigValue(source, strconv.FormatBool(x))
		}
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
