// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Pinned by handfixes.json entry
// "config-reads-held-before-parse" (class secret-echo).
//
// The credential kit holds the declared keys of the files named at startup.
// This holds what it does not reach: a file read after startup (the MCP server
// reloads the config on every tool call), a file that does not parse, every
// unknown key, and each part of a header value. Every value goes to credkit.Hold.

package cliutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"avanan-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// configSource is the kit source of a value read from a config or credentials
// file outside the declared keys; every source shares the "*" origin rule.
const configSource = "config:unclassified"

// Hold is credkit.Hold for every credential the connector reads or mints. A
// variable because the kit's held set is process-wide with no reset: in-process
// tests (no credkit.Start) would check every later statement and file against
// their fixtures. The built binaries are the proof of the hold.
var Hold = credkit.Hold

// CommandPath is the command a command line runs, below the root, as Execute
// will resolve it; internal/cli sets it at init. Nil before then.
var CommandPath func(args []string) []string

// CredentialLikeProfileFlag reports a run-profile key named for a credential.
func CredentialLikeProfileFlag(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
	for _, marker := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key", "client-secret", "app-id", "client-id"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// HoldProfileFile holds what a run-profile file carries as a credential, for the
// kit's startup: a value under a credential flag's name (declared in
// ConfigFields) through credkit.HoldBody, which reads every duplicate and nested
// value and holds every string of a file that does not parse; and each value
// beneath base-url through credkit.HoldURL. Not through Declarations.Files: the
// kit holds every string of nested JSON there, so a profile named "p" or a saved
// --data-source local would withhold every later output. Free text is not a
// source: contract section 1.
func HoldProfileFile(path string) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return
	}
	opened, err := ReadOpenedConfig(path)
	if err != nil {
		return
	}
	credkit.HoldBody(opened.Data)
	dec := json.NewDecoder(bytes.NewReader(opened.Data))
	dec.UseNumber()
	_ = holdBaseURLs(dec, false) // where it stops, HoldBody held every string
}

// holdBaseURLs walks tokens, not a decoded tree, which would keep only the last
// member of a duplicate key; under says a base-url key stands above the value.
func holdBaseURLs(dec *json.Decoder, under bool) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch x := tok.(type) {
	case json.Delim: // '{' or '[': the decoder never returns a closing one here
		for dec.More() {
			u := under
			if x == '{' {
				if tok, err = dec.Token(); err != nil {
					return err
				}
				key, _ := tok.(string)
				u = under || strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(key)) == "baseurl"
			}
			if err := holdBaseURLs(dec, u); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case json.Number:
		if under {
			credkit.HoldURL("config:base-url", x.String())
		}
	case string:
		if under {
			credkit.HoldURL("config:base-url", x)
		}
	}
	return nil
}

// AtomicWriteCheckedFile is AtomicWritePrivateFile for a file that is not a
// credential source: the bytes pass the kit (a withheld write leaves the old
// file in place). The config and the credentials store stay on
// AtomicWritePrivateFile, the declared exemption for credential sources.
func AtomicWriteCheckedFile(path string, data []byte, fileMode, dirMode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("creating private file dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary private file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	err = tmp.Chmod(fileMode)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("securing temporary private file: %w", err)
	}
	if err := credkit.WriteFile(tmpPath, data, fileMode); err != nil {
		return err
	}
	if err := renamePrivateFileWithRetry(tmpPath, path); err != nil {
		return fmt.Errorf("publishing private file: %w", err)
	}
	return nil
}

// HoldTOMLValues holds every string a TOML file sets, at any depth (tables,
// dotted keys, inline tables, arrays, multiline strings), except the values of
// the top-level keys isSetting names (a lower-cased key: the loader matches
// keys case-insensitively), each with its parts and its escapes decoded. A file
// that parses is read with the loader's own TOML library; one that does not (a
// malformed legacy config or credentials file, later skipped or refused) is
// lexed for every string literal and bare value right of an '=', so it still
// holds what it carries. It runs before the file is parsed for use.
func HoldTOMLValues(data []byte, isSetting func(key string) bool) {
	var values []string
	hold := func(v string) {
		for _, form := range []string{v, tomlUnescape(v)} {
			values = append(values, form)
			values = append(values, HeldHeaderComponents(form)...)
		}
	}
	var doc map[string]any
	if toml.Unmarshal(data, &doc) == nil {
		var walk func(v any)
		walk = func(v any) {
			switch v := v.(type) {
			case string:
				hold(v)
			case int64:
				hold(strconv.FormatInt(v, 10))
			case float64:
				hold(strconv.FormatFloat(v, 'g', -1, 64))
			case map[string]any:
				for _, e := range v {
					walk(e)
				}
			case []any:
				for _, e := range v {
					walk(e)
				}
			}
		}
		for k, v := range doc {
			if !isSetting(strings.ToLower(k)) {
				walk(v)
			}
		}
	} else {
		tomlLenientValues(string(data), func(table, key, v string) {
			if table != "" || !isSetting(key) {
				hold(v)
			}
		})
	}
	Hold(configSource, values...)
}

// tomlEscape is one backslash sequence as written, whatever follows it: the
// library decides what it means, so no escape table lives here.
var tomlEscape = regexp.MustCompile(`\\(?:u[0-9a-fA-F]{4}|U[0-9a-fA-F]{8}|[ \t]*\r?\n\s*|.)`)

// tomlUnescape decodes every backslash sequence the TOML library accepts, by
// asking it about that sequence alone, so a value from a file that does not
// parse is held as the loader would decode it. One it refuses stays as written.
func tomlUnescape(s string) string {
	return tomlEscape.ReplaceAllStringFunc(s, func(esc string) string {
		var doc struct {
			V string `toml:"v"`
		}
		if toml.Unmarshal([]byte(`v = """`+esc+`"""`), &doc) != nil {
			return esc
		}
		return doc.V
	})
}

// tomlLenientValues reads a TOML text that does not parse and calls hold with
// every string literal (all four quote forms, a multiline one whole) and bare
// value right of a statement's '=', with the statement's table and key
// (lower-cased, quotes dropped). Comments, key names and table headers are not
// values; an array or inline table runs on to its closing bracket. A
// statement without an '=' has its literals held as values of no key.
func tomlLenientValues(data string, hold func(table, key, v string)) {
	table := ""
	for i := 0; i < len(data); {
		for i < len(data) && strings.IndexByte(" \t\r\n", data[i]) >= 0 {
			i++
		}
		if i >= len(data) {
			break
		}
		eol := strings.IndexByte(data[i:], '\n')
		if eol < 0 {
			eol = len(data)
		} else {
			eol += i
		}
		switch data[i] {
		case '#':
			i = eol
			continue
		case '[':
			name, _, _ := strings.Cut(data[i:eol], "#")
			table = strings.ToLower(strings.Trim(strings.TrimSpace(name), "[] \t\r\"'"))
			i = eol
			continue
		}
		key, eq := "", tomlIndexOutsideQuotes(data[i:eol], '=')
		if eq >= 0 {
			key = strings.ToLower(strings.Trim(strings.TrimSpace(data[i:i+eq]), `"'`))
			i += eq + 1
		}
		i = tomlValue(data, i, func(v string) { hold(table, key, v) })
	}
}

// tomlValue reads one value from data[i:] (to the end of its line, or past a
// multiline string or bracketed value), calling lit for each string literal
// and for a bare value, and returns where the next statement starts.
func tomlValue(data string, i int, lit func(string)) int {
	depth, bare := 0, ""
	for i < len(data) {
		c := data[i]
		switch {
		case c == '\n' && depth == 0:
			if b := strings.TrimSpace(bare); b != "" && !tomlScalar(b) {
				lit(b)
			}
			return i + 1
		case c == '#':
			if eol := strings.IndexByte(data[i:], '\n'); eol >= 0 {
				i += eol
			} else {
				i = len(data)
			}
			continue
		case c == '"' || c == '\'':
			v, end := tomlString(data, i)
			lit(v)
			i = end
			continue
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			bare += string(c)
		}
		i++
	}
	if b := strings.TrimSpace(bare); b != "" && !tomlScalar(b) {
		lit(b)
	}
	return i
}

// tomlString reads the string literal starting at data[i] (basic or literal,
// single- or multi-line; an unterminated one runs to its line's end, or to
// the end of the text when multiline) and returns its content, as written,
// and the index just past it.
func tomlString(data string, i int) (string, int) {
	q := data[i : i+1]
	if strings.HasPrefix(data[i:], q+q+q) {
		start := i + 3
		if end := strings.Index(data[start:], q+q+q); end >= 0 {
			return data[start : start+end], start + end + 3
		}
		return data[start:], len(data)
	}
	for j := i + 1; j < len(data); j++ {
		switch {
		case q == `"` && data[j] == '\\':
			j++
		case data[j] == q[0]:
			return data[i+1 : j], j + 1
		case data[j] == '\n':
			return data[i+1 : j], j
		}
	}
	return data[i+1:], len(data)
}

// tomlIndexOutsideQuotes is the index of the first c in line outside a quoted
// key, or -1.
func tomlIndexOutsideQuotes(line string, c byte) int {
	var quote byte
	for i := 0; i < len(line); i++ {
		switch {
		case quote != 0:
			if line[i] == quote {
				quote = 0
			}
		case line[i] == '"' || line[i] == '\'':
			quote = line[i]
		case line[i] == c:
			return i
		}
	}
	return -1
}

// tomlScalar reports whether a bare value is a boolean: the one unquoted
// scalar that carries no credential. A bare number is held: a malformed file's
// `auth_header = 12345` is that number in the loader's diagnostic.
func tomlScalar(v string) bool { return v == "true" || v == "false" }

// HeldHeaderComponents is every part of a header value that can carry a
// credential on its own: each piece between commas, semicolons and spaces
// (a leading scheme word excepted) and the part after a piece's first '=',
// each with any quotes around it removed. No length or shape floor: the one
// artifact dropped is a part that is only base64 padding, left by the split at
// '='. The kit splits a scheme's Basic pair and password itself (credkit.Hold).
func HeldHeaderComponents(v string) []string {
	var out []string
	add := func(c string) {
		c = strings.Trim(strings.TrimSpace(c), `"'`)
		if strings.Trim(c, "=") != "" {
			out = append(out, c)
		}
	}
	fields := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' || unicode.IsSpace(r) })
	for i, f := range fields {
		if i == 0 && len(fields) > 1 && !strings.Contains(f, "=") {
			continue
		}
		add(f)
		if _, after, ok := strings.Cut(f, "="); ok {
			add(after)
		}
	}
	return out
}
