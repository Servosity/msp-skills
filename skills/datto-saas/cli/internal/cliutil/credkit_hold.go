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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode"

	"datto-saas-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// MCPChildPreview is the MCPChildEnvVar value of a child the MCP server starts for
// a confirm preview: it parses its flags and arguments, says so and exits before it
// touches anything. The server drops an inherited marker, so no caller sets it.
const MCPChildPreview = "preview"

// MCPPreviewChild reports whether this process is such a child.
func MCPPreviewChild() bool {
	return os.Getenv(MCPChildEnvVar) == MCPChildPreview
}

// Hold is credkit.Hold. Production code never reassigns it. In-process unit tests
// do, to a no-op: a held value lasts for the process and the kit withholds
// whatever carries it, so one test's fixture would withhold the next test's
// output. The built binaries' leak tests (internal/credwire) prove the real thing.
var Hold = credkit.Hold

// CredentialConfigKeys are the keys of config.toml and credentials.toml whose
// value is a credential whatever characters it is made of.
var CredentialConfigKeys = []string{"auth_header", "saas_public_key", "saas_secret_key", "access_token", "refresh_token", "client_id", "client_secret"}

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

// HoldConfigFile holds each value of a config file that parses, as the parser
// decodes it. A file that does not parse is read token by token by the kit at
// startup (Declarations.Files); this program writes only files that parse.
func HoldConfigFile(data []byte) {
	var tree map[string]any
	if toml.Unmarshal(data, &tree) == nil {
		holdDecoded("", tree)
	}
}

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

var (
	secretNameExact = map[string]bool{
		"pwd": true, "pass": true, "passphrase": true, "key": true,
		"secret": true, "token": true, "auth": true, "authorization": true,
		"cookie": true, "setcookie": true, "otp": true, "pin": true, "mfasecret": true,
	}
	secretNameContains = []string{
		"password", "passwd", "secret", "apikey", "privatekey", "accesskey",
		"credential", "sessionkey", "signingkey", "encryptionkey",
	}
	secretNameSuffix = []string{"token", "apikey", "key"}
	// Names ending in "key" that identify rather than authenticate.
	secretNameKeyAllow = map[string]bool{"idempotencykey": true, "sortkey": true, "partitionkey": true}
)

func fieldNameLetters(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SecretFieldName reports whether a JSON key or flag name names a secret.
// Matching is case- and separator-insensitive (client_secret, api-key and
// X-Api-Key all match).
func SecretFieldName(name string) bool {
	n := fieldNameLetters(name)
	if n == "" || secretNameKeyAllow[n] {
		return false
	}
	if secretNameExact[n] {
		return true
	}
	for _, s := range secretNameContains {
		if strings.Contains(n, s) {
			return true
		}
	}
	for _, s := range secretNameSuffix {
		if strings.HasSuffix(n, s) {
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
	Hold(fixedSource(source), v)
	if strings.Contains(t, "://") {
		holdBaseURL(t) // a URL under any key gives up what it carries
	}
}

// fixedSource is the name a value is held under. The kit prints a source when it
// refuses a start, and a file that does not parse can put a credential where a key
// name belongs, so only a name this program knows survives.
func fixedSource(source string) credkit.Source {
	switch {
	case strings.HasPrefix(source, "header:"):
		return "header:value"
	case strings.HasPrefix(source, "query:"):
		return "query:value"
	case !strings.HasPrefix(source, "config:"), credentialSource(source), source == "config:unclassified":
		return credkit.Source(source)
	}
	if notCredential("", strings.TrimPrefix(source, "config:")) {
		return credkit.Source(source)
	}
	return "config:value"
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
		if p != "" { // no strength floor: a password is one whatever its characters
			Hold("userinfo", p)
		}
		if d, err := url.PathUnescape(p); err == nil && d != "" {
			Hold("userinfo", d)
		}
	}
	for _, pair := range strings.FieldsFunc(tail, func(r rune) bool { return r == '&' || r == ';' || r == '#' || r == '?' }) {
		k, v, _ := strings.Cut(pair, "=")
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

// HoldProfileStore holds what a run profile store can carry that the kit's reader
// of Declarations.Files does not look for: every value under a credential-named
// key, and the password and credential query values of a value under base-url (by
// config.toml's base_url rule), every occurrence of a duplicate key included
// (contract section 1, brief item 8(c)). What precedes a syntax error is held too. A profile's name is the user's own
// word, not a field name: a profile called token holds nothing. Under a
// credential-named flag true and false are a switch. Free text (a description, a
// --deliver URL) is not a source and is not read.
func HoldProfileStore(data []byte) {
	var found bodyValues
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	walkBody(dec, false, profileNameLevels, &found)
	for _, v := range found.secrets {
		if v != "true" && v != "false" {
			Hold("config:profile", v)
		}
	}
	for _, u := range found.baseURLs {
		holdBaseURL(u)
	}
}

// profileNameLevels is how deep a profile's name sits: {"profiles":{"<name>":...}}.
const profileNameLevels = 2

// HoldProfileStoreFile is HoldProfileStore on a file. Only a regular file is opened.
func HoldProfileStoreFile(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldProfileStore(data)
	}
}

// HoldTLSKeyFile holds the private key the MCP server would serve TLS with: each
// PEM body line and the decoded key as hex and base64, so neither can be printed. Armor and header lines are not the key,
// and holding them would withhold any PEM text. Only a regular file is opened.
func HoldTLSKeyFile(path string) {
	data, err := readRegularFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "-----") && !strings.Contains(line, ":") {
			Hold("minted:tls-key", line)
		}
	}
	for rest := data; ; {
		block, more := pem.Decode(rest)
		if block == nil {
			return
		}
		// As text, in the forms output can carry: the kit's collision check does not
		// finish in reasonable time on binary bytes, and it would refuse the start.
		Hold("minted:tls-key", hex.EncodeToString(block.Bytes), strings.ToUpper(hex.EncodeToString(block.Bytes)), base64.StdEncoding.EncodeToString(block.Bytes))
		rest = more
	}
}

// HoldConfigFilePath holds every value of a config or credential file this run
// does not use, by the same rule as a file it reads. Only a regular file is
// opened, so a FIFO named on the command line cannot hang startup.
func HoldConfigFilePath(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldConfigFile(data)
	}
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.ReadFile(path) // #nosec G304 -- the operator's own config, store or key path, read to hold its values
}

// ReadRequestBody reads a --stdin request body and holds its credentials with the
// kit (credkit.HoldBody: the declared CredentialFields at any depth, every duplicate,
// and every token of a body that is no document) before the caller can do anything
// with it, previews included. --stdin is not in Declarations.StdinFlags, where the
// kit holds each line at startup, and a line reading "{" then refuses every start.
func ReadRequestBody(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err == nil {
		credkit.HoldBody(data)
	}
	return data, err
}

// bodyValues is what walkBody found: values under a credential-named member, and
// values under a base-url member (inBase counts the open ones).
type bodyValues struct {
	secrets, baseURLs []string
	inBase            int
}

// walkBody reads one JSON value with the token reader, which sees every occurrence
// of a key where decoding into a map keeps the last. secret says the value sits
// under a credential-named member, and it carries to everything inside. The object
// keys of the first dataLevels levels are data, so their names are not tested.
func walkBody(dec *json.Decoder, secret bool, dataLevels int, found *bodyValues) bool {
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '[':
			for dec.More() {
				if !walkBody(dec, secret, dataLevels, found) {
					return false
				}
			}
		case '{':
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return false
				}
				name, isName := key.(string)
				if !isName {
					return false
				}
				base := dataLevels == 0 && fieldNameLetters(name) == "baseurl"
				if base {
					found.inBase++
				}
				if !walkBody(dec, secret || dataLevels == 0 && SecretFieldName(name), max(dataLevels-1, 0), found) {
					return false
				}
				if base {
					found.inBase--
				}
			}
		}
		_, err = dec.Token()
		return err == nil
	case string:
		if secret && t != "" {
			found.secrets = append(found.secrets, t)
		}
		if found.inBase > 0 {
			found.baseURLs = append(found.baseURLs, t)
		}
	case json.Number:
		if secret {
			found.secrets = append(found.secrets, t.String())
		}
	}
	return true
}
