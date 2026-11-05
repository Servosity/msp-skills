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
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"apple-business-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// NotCredentials are the top-level keys whose values are not credentials.
// internal/credwire declares each as a credkit Identifier (config:<key>), so
// the kit exempts them too; a credential inside base_url or token_url (its
// password and query values) is still held, as userinfo and query:<name>. The
// Client ID and Key ID are the API account's public names: auth status prints
// them and the store is scoped by the Client ID.
var NotCredentials = []string{"base_url", "token_url", "ca_cert", "tls_server_name", "token_expiry", "client_id", "business_client_id", "business_key_id"}

// PrivateKeyFields are the keys that name the API account's private key, as PEM
// text or as a path to the .pem file. HoldPrivateKeyRef reads them.
var PrivateKeyFields = []string{"client_secret", "business_private_key"}

func notCredential(section, key string) bool {
	return section == "" && slices.Contains(NotCredentials, key)
}

func privateKeyField(section, key string) bool {
	return section == "" && slices.Contains(PrivateKeyFields, key)
}

// HoldPrivateKeyRef holds what a private-key setting really is. PEM text gives
// up its body lines, the body joined and the decoded key in text forms; its
// armor is fixed text in every PEM file. A path to a readable file gives up that
// file's contents the same way; the path itself is no credential, and held
// whole, any 8 characters of it (a home directory) would withhold every printed
// path. Anything else may be a mistyped key, so it is held whole.
func HoldPrivateKeyRef(ref string) {
	raw := strings.TrimSpace(ref)
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	if raw == "" {
		return
	}
	if text := strings.ReplaceAll(raw, `\n`, "\n"); strings.Contains(text, "-----BEGIN") {
		holdPEMText(text)
		return
	}
	path := raw
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
		if data, err := os.ReadFile(filepath.Clean(path)); err == nil { // the operator's key file
			holdPEMText(string(data))
			return
		}
	}
	credkit.Hold(PrivateKeySource, raw)
}

// PrivateKeySource is the source every held piece of the private key has. Its
// declared origins are none: the key signs assertions and is never sent.
const PrivateKeySource = "minted:private-key"

// holdPEMText holds every line of text but armor lines, the body lines joined,
// and each block's decoded bytes as hex and base64. Not the raw bytes: holding
// DER makes the kit's startup collision check run out of budget.
func holdPEMText(text string) {
	body := strings.Fields(pemArmor.ReplaceAllString(text, "\n"))
	for _, word := range body {
		credkit.Hold(PrivateKeySource, word)
	}
	if len(body) > 1 {
		credkit.Hold(PrivateKeySource, strings.Join(body, ""))
	}
	decoded := false
	for rest := []byte(text); ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		decoded = true
		credkit.Hold(PrivateKeySource, hex.EncodeToString(block.Bytes), base64.StdEncoding.EncodeToString(block.Bytes))
	}
	if !decoded { // flattened onto one line, which pem.Decode refuses
		if der, err := base64.StdEncoding.DecodeString(strings.Join(body, "")); err == nil && len(der) > 0 {
			credkit.Hold(PrivateKeySource, hex.EncodeToString(der))
		}
	}
}

// pemArmor is a PEM boundary, wherever flattening left it.
var pemArmor = regexp.MustCompile(`-----[A-Z0-9 ]+-----`)

// secretProfileKey names a run profile key whose value is a credential by its name.
var secretProfileKey = regexp.MustCompile(`(?i)passw|secret|token|api.?key|private.?key|access.?key|credential|cookie|authorization|^auth$|auth_?(header|token)`)

// SecretProfileKey reports whether a run profile key names a credential.
func SecretProfileKey(key string) bool { return secretProfileKey.MatchString(key) }

// urlProfileKeys are the run profile keys whose value is a URL that can carry a
// credential: its password and query values are held.
var urlProfileKeys = []string{"base-url", "base_url", "deliver"}

// HoldProfileStoreFile holds what a run profile store can carry: every value under a
// credential-named key and the password and query values of a saved URL, every member
// of a duplicate key included, so it reads tokens and never a map. The kit's file reader
// is not used for it: it would hold every string of nested JSON, so a profile named "p"
// or a saved --limit 7 would withhold every later output.
//
// It returns every string value, so the caller can hold the credential fields of a JSON
// value bound for a body flag from its bytes. A returned list, not a callback: a call
// through a func value reads, to the security gate's taint check, as a call to every
// function of that type, HoldPrivateKeyRef's file read included.
func HoldProfileStoreFile(path string) []string {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil
	}
	if !json.Valid(data) {
		// A store that does not parse gives up every word.
		for _, w := range strings.Fields(string(data)) {
			credkit.Hold("config:profiles", strings.Trim(w, `"',:{}[]`))
		}
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var strs []string
	_ = holdProfileValue(dec, "", &strs)
	return strs
}

func holdProfileValue(dec *json.Decoder, key string, strs *[]string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch t := tok.(type) {
	case json.Delim:
		for dec.More() {
			member := key // an array's values stay under its key
			if t == '{' {
				name, err := dec.Token()
				if err != nil {
					return err
				}
				member = name.(string)
			}
			if err := holdProfileValue(dec, member, strs); err != nil {
				return err
			}
		}
		_, err = dec.Token() // the closing delimiter
		return err
	case json.Number:
		if secretProfileKey.MatchString(key) {
			credkit.Hold(credkit.Source("config:"+key), t.String())
		}
	case string:
		*strs = append(*strs, t)
		if secretProfileKey.MatchString(key) {
			credkit.Hold(credkit.Source("config:"+key), t)
		}
		if slices.Contains(urlProfileKeys, key) {
			holdBaseURL(strings.TrimPrefix(t, "webhook:"))
		}
	}
	return nil
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

// privateKeyMultiline marks a multi-line string under a private-key field.
const privateKeyMultiline = "\x00private-key"

// holdConfigValue holds v under source, whatever it is made of. Only the
// sources internal/credwire declares as Identifiers are exempt, by the kit.
func holdConfigValue(source, v string) {
	credkit.Hold(credkit.Source(source), v)
}

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
			if multiSource == privateKeyMultiline {
				HoldPrivateKeyRef(before)
			} else {
				out = append(out, configValue{multiSource, before})
			}
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
				if !tomlPunctuation(v) {
					out = append(out, configValue{"config:unclassified", v})
				}
			}
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		vals, open := configValueParts(rhs)
		if notCredential(section, key) && len(vals) == 1 && plainScalar(rhs) {
			if key == "base_url" || key == "token_url" {
				holdBaseURL(vals[0])
			}
			continue
		}
		if privateKeyField(section, key) {
			for _, v := range vals {
				HoldPrivateKeyRef(v)
			}
			if open != "" {
				multi, multiSource = open, privateKeyMultiline
			}
			continue
		}
		source := sourceFor(section, key)
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
		trimmed := strings.TrimSpace(rhs)
		structured := strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{")
		// Each word, and each piece of one split at TOML's punctuation. Only an
		// array or inline table has punctuation that is not a value.
		pieces := strings.FieldsFunc(rhs, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '{' || r == '}' || r == '[' || r == ']' || r == ',' || r == '='
		})
		for _, w := range append(pieces, strings.Fields(rhs)...) {
			if w != "#" && !slices.Contains(vals, w) && (!structured || !tomlPunctuation(w)) {
				vals = append(vals, w)
			}
		}
	}
	return vals, ""
}

// tomlPunctuation reports whether word is only the syntax of an array, an
// inline table or a comment, on a line of its own or inside one: "]" closing a
// multi-line array is not a value.
func tomlPunctuation(word string) bool {
	switch word {
	case "[", "]", "{", "}", "[]", "{}", ",", "],", "},", "#":
		return true
	}
	return false
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
			if s, ok := v.(string); ok {
				if k == "base_url" || k == "token_url" {
					holdBaseURL(s)
				}
				continue
			}
		case privateKeyField(section, k):
			if s, ok := v.(string); ok {
				HoldPrivateKeyRef(s)
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
		holdConfigValue(source, strconv.FormatBool(x))
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
