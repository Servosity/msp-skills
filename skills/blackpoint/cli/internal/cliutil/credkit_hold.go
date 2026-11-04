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
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"blackpoint-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Hold is credkit.Hold. Production code never reassigns it. In-process unit tests
// do, to a no-op: a held value lasts for the process and the kit withholds
// whatever carries it, so one test's fixture would withhold the next test's
// output. The built binaries' leak tests (internal/credwire) prove the real thing.
var Hold = credkit.Hold

// CredentialConfigKeys are the keys of config.toml and credentials.toml whose
// value is a credential whatever characters it is made of.
var CredentialConfigKeys = []string{"auth_header", "api_token", "access_token", "refresh_token", "client_id", "client_secret"}

// CredentialResponseFields are the CompassOne response members the kit holds as a
// response passes its transport: a Duo onboarding's secret key. ikey is left out
// on purpose: Duo's integration key identifies, it does not authenticate.
var CredentialResponseFields = []string{"skey"}

// MaskedResponseFields are replaced by name at the client (client.maskCredentialFields,
// handfixes.json credential-fields-withheld), so a listing that carries one stays
// usable: the held skey, and a dark web exposure's breached password. That password
// is data about a third-party breach, not a credential this program uses, and it is
// masked before any sink. Holding it would withhold every later result sharing eight
// characters with it ("password1") for the life of an MCP server, so it is declared an
// Identifier in internal/credwire.
var MaskedResponseFields = []string{"password", "skey"}

// NotCredentials are the top-level keys whose values are not credentials.
// internal/credwire declares each as a credkit Identifier (config:<key>), so
// the kit exempts them too; a credential inside base_url is still held, as
// userinfo and query:<name>.
var NotCredentials = []string{"base_url", "ca_cert", "tls_server_name", "token_expiry", "logged_out"}

// TenantSelectorHeader selects a tenant. Its value is a UUID every tenant row
// repeats, not a credential: internal/credwire declares it an Identifier, so a
// [headers] table naming it does not withhold every row of that tenant.
const TenantSelectorHeader = "x-tenant-id"

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

// HoldConfigFile holds what a config or credential file carries. A file that
// parses is read by name, with case ignored as the decoder ignores it: a credential
// key, a [headers] entry, a secret-named key and the credentials of base_url, each
// as written and as decoded. A key the program never reads and no rule names
// (timeout = 30) stays usable. A file that does not parse holds every value, by a
// whole-file scan that trusts no parse (tables, inline tables, unterminated and
// multi-line strings, bare words).
func HoldConfigFile(data []byte) {
	var tree map[string]any
	parsed := toml.Unmarshal(data, &tree) == nil
	for _, v := range scanConfigValues(string(data)) {
		if !parsed || credentialNamed(v.source) {
			holdConfigValue(v.source, v.value)
		}
	}
	if parsed {
		holdDecoded("", tree)
	}
}

// credentialNamed reports a source a parsed file's value is held under.
func credentialNamed(source string) bool {
	if credentialSource(source) {
		return true
	}
	key, ok := strings.CutPrefix(source, "config:")
	if !ok {
		return false
	}
	for _, part := range strings.Split(key, ".") {
		if SecretFieldName(part) {
			return true
		}
	}
	return false
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
	if source == "config:base_url" {
		holdBaseURL(t)
	}
}

// holdBaseURL holds the credentials inside a base URL as it is written, whether
// or not it parses: the password (the userinfo runs to the last @ before any ?
// or #, so a raw / or @ in it is covered), a userinfo with no password as a whole
// (a key written in the username slot), and every query and fragment value. A URL
// anywhere else is free text, which contract section 1 does not name.
func holdBaseURL(raw string) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		return
	}
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
		holdURLPart("userinfo", p)
		if d, err := url.PathUnescape(p); err == nil {
			holdURLPart("userinfo", d)
		}
	}
	for _, pair := range strings.FieldsFunc(tail, func(r rune) bool { return r == '&' || r == '#' || r == '?' }) {
		k, v, _ := strings.Cut(pair, "=")
		holdURLPart("query:"+k, v)
		if d, err := url.QueryUnescape(v); err == nil {
			holdURLPart("query:"+k, d)
		}
	}
}

func holdURLPart(source, v string) {
	if strings.TrimSpace(v) != "" {
		Hold(credkit.Source(source), v)
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
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if multi != "" {
			before, after, closed := strings.Cut(line, multi)
			out = append(out, configValue{multiSource, before})
			if closed {
				multi, line = "", after
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
				multi, multiSource = open, "config:unclassified"
			}
			continue
		}
		// go-toml lower-cases a key to find its field (unmarshaler.go), so a key here
		// is read the same way, or AUTH_HEADER loads and goes unheld.
		key, rhs := strings.ToLower(strings.Trim(strings.TrimSpace(line[:eq]), `"'`)), line[eq+1:]
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
			multi, multiSource = open, source
		}
	}
	return out
}

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

// holdDecoded holds each leaf of a parsed config as the parser decoded it.
func holdDecoded(section string, tree map[string]any) {
	for k, v := range tree {
		k = strings.ToLower(k) // as go-toml finds a field; see scanConfigValues
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
		if source := sourceFor(section, k); credentialNamed(source) {
			holdLeaf(source, v)
		}
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

// HoldProfileStore holds what contract section 1 names in a run profile store: every
// value under a credential flag's name, each duplicate key included, and the
// credentials of a base-url value. Only the path profiles.<name>.values.<flag> means
// anything: a profile's name, description and every other value are data, and an
// allow-list in loadProfileStore decides what of them runs. The store is read token
// by token because a map keeps only the last duplicate, and with a stack of its own
// so no nesting can overflow. It stops where the JSON stops: the loader refuses such
// a store, so what follows cannot print.
func HoldProfileStore(data []byte) {
	type frame struct {
		object, wantKey bool
		key             string
	}
	var stack []frame
	holdAt := func(v string) {
		var keys []string
		for _, f := range stack {
			if f.object {
				keys = append(keys, f.key)
			}
		}
		if len(keys) < 4 || !strings.EqualFold(keys[0], "profiles") || !strings.EqualFold(keys[2], "values") {
			return
		}
		switch flag := strings.ToLower(keys[3]); {
		case flag == "base-url" || flag == "base_url":
			holdBaseURL(v)
		case SecretFieldName(flag) && strings.TrimSpace(v) != "":
			Hold(credkit.Source("flag:--"+flag), v)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch t := tok.(type) {
		case json.Delim:
			if t == '{' || t == '[' {
				stack = append(stack, frame{object: t == '{', wantKey: t == '{'})
				continue
			}
			stack = stack[:len(stack)-1]
		case string:
			if n := len(stack); n > 0 && stack[n-1].wantKey {
				stack[n-1].key, stack[n-1].wantKey = t, false
				continue
			}
			holdAt(t)
		case json.Number:
			holdAt(t.String())
		}
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].wantKey = true
		}
	}
}

// HoldProfileStoreFile is HoldProfileStore on a file. Only a regular file is opened.
func HoldProfileStoreFile(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldProfileStore(data)
	}
}

// HoldTLSKeyFile holds the private key the MCP server would serve TLS with: each
// PEM body line and the decoded key, so neither its text nor its bytes in any
// encoding the kit reads can be printed. Armor and header lines are not the key,
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
	// The decoded key is held as hex, the one text form of it the PEM lines do not
	// already hold. Its raw bytes are not held: the kit's startup check of a binary
	// value runs for minutes and then refuses the start, so the HTTPS server never
	// served (kit-findings.md). A key file is not a source contract section 1 names.
	for rest := data; ; {
		block, more := pem.Decode(rest)
		if block == nil {
			return
		}
		Hold("minted:tls-key", hex.EncodeToString(block.Bytes))
		rest = more
	}
}

// StartupConfigFlag and StartupHomeFlag are the --config and --home values startup
// read the files with (credwire.credentialFiles): the last occurrence of each on the
// command line, or "". The kit took the allowed origin from that reading.
var StartupConfigFlag, StartupHomeFlag string

// CheckStartupFlags refuses a run whose parsed --config or --home is not what startup
// read. A flag that takes a value can swallow one (--select --config x), and then the
// allowed origin came from a file this run does not use.
func CheckStartupFlags(config, home string) error {
	if config != StartupConfigFlag || home != StartupHomeFlag {
		return fmt.Errorf("--config or --home is also the value of another flag here, so it is not clear which file this run uses: write each once, as its own flag")
	}
	return nil
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
	return os.ReadFile(filepath.Clean(path))
}

// CompactStdinBody puts a --stdin JSON body on one line before the kit reads it.
// Declarations.StdinFlags holds a body whole and line by line, and a line of "{" alone
// collides with public text and refuses the start, so a body from `jq .` would end
// the run. The credentials stay held: StdinFlags does that. Anything that is not one
// JSON document stays as it came.
func CompactStdinBody(args []string) {
	body := false
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		body = body || name == "--stdin" || name == "-stdin"
	}
	if !body {
		return
	}
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return // the kit's own read of what is left fails closed
	}
	var one bytes.Buffer
	if json.Compact(&one, data) == nil {
		data = one.Bytes()
	}
	r, w, err := os.Pipe()
	if err != nil {
		return // os.Stdin is spent, so the command reads an empty body and refuses it
	}
	go func() {
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	os.Stdin = r
}
