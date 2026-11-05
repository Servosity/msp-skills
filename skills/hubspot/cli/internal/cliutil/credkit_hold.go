// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring). Every read of a
// config or credentials file holds its values with the credential kit BEFORE
// the file is parsed: the kit's startup discovery holds the files it names
// once; a file written after that, or named another way, is held here.

package cliutil

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"hubspot-pp-cli/internal/credkit"
)

var (
	hsURLInText   = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)
	hsURLPassword = regexp.MustCompile(`://[^/?#:@]*:([^/?#@]+)@`)
	hsTableLine   = regexp.MustCompile(`^\[+\s*([^\]]*?)\s*\]+$`)
	hsQuoted      = regexp.MustCompile(`"((?:[^"\\]|\\.)*)("|$)|'([^']*)('|$)`)
	hsEscape      = regexp.MustCompile(`\\(?:u[0-9a-fA-F]{4}|U[0-9a-fA-F]{8}|x[0-9a-fA-F]{2}|.)`)
	hsLineEnd     = regexp.MustCompile(`\\[ \t]*\r?\n\s*`) // a line-ending backslash in a multi-line string
)

// hsNotCredential names the top-level keys credwire declares Identifiers.
var hsNotCredential = map[string]bool{"base_url": true, "ca_cert": true, "tls_server_name": true, "token_expiry": true}

// OutboundGuard is the credential kit's outbound guard, around every request
// the API client sends (handfixes.json credkit-wiring). Production code never
// reassigns it. In-process unit tests do, to the identity: they never run
// credkit.Start, so the kit has no declarations and no base origin and would
// refuse every credentialed request to a loopback test server. The built
// binaries' leak tests prove the guard itself.
var OutboundGuard = credkit.Transport

// HoldCredential is credkit.Hold, through which the connector holds every
// credential it reads. Production code never reassigns it. In-process unit
// tests do, to a no-op: a hold lasts until the process exits, so one test's
// token would be checked against every later test's files and statements.
// Tests of holding run in a child process (runInChild in their _test files).
var HoldCredential = credkit.Hold

// HoldConfigBytes holds every value in the bytes of a config or credentials
// file, trusting no parse: a file that is malformed, permission-refused or
// legacy gives up its values like an accepted one. Each value is held as
// written and with its escapes undone; only a
// top-level key credwire declares an identifier is left, and base_url still
// gives up the credentials inside it.
func HoldConfigBytes(data []byte) {
	hsHoldConfigLines(string(data))
	if folded := hsLineEnd.ReplaceAllString(string(data), ""); folded != string(data) {
		hsHoldConfigLines(folded)
	}
}

func hsHoldConfigLines(text string) {
	table := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m := hsTableLine.FindStringSubmatch(line); m != nil && !strings.Contains(line, "=") {
			table = strings.ToLower(m[1])
			continue
		}
		key, rhs, ok := strings.Cut(line, "=")
		if !ok {
			HoldCredential("config:unclassified", hsLineValues(line)...)
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		vals := hsLineValues(rhs)
		switch {
		case table == "headers":
			HoldCredential("header:"+strings.ToLower(key), vals...)
		case table == "" && key == "base_url":
			for _, v := range vals {
				HoldBaseURL(v)
			}
		case table == "" && hsNotCredential[key]:
		case table == "":
			HoldCredential("config:"+key, vals...)
		default:
			HoldCredential("config:"+table+"."+key, vals...)
		}
	}
}

// hsLineValues is every quoted string on the right of an assignment (an
// unterminated one runs to the end of the line), else its bare words up to a
// comment, each as written and as its escapes spell it.
func hsLineValues(rhs string) []string {
	var out []string
	for _, m := range hsQuoted.FindAllStringSubmatch(rhs, -1) {
		if m[1] != "" {
			out = append(out, m[1])
		} else if m[3] != "" {
			out = append(out, m[3])
		}
	}
	if len(out) == 0 {
		bare, _, _ := strings.Cut(rhs, "#")
		out = slices.DeleteFunc(strings.Fields(bare), func(w string) bool { return strings.Trim(w, `"'`) == "" }) // the """ that opens a multi-line string
	}
	for _, v := range out {
		if spelled := hsSpelled(v); spelled != v {
			out = append(out, spelled)
		}
	}
	return out
}

// hsSpelled undoes every escape TOML knows and keeps any other as written, so one
// escape it cannot read loses none of the others.
func hsSpelled(s string) string {
	return hsEscape.ReplaceAllStringFunc(s, func(m string) string {
		if m == `\e` { // TOML's, not Go's
			return "\x1b"
		}
		if v, err := strconv.Unquote(`"` + m + `"`); err == nil {
			return v
		}
		return m
	})
}

// HoldBaseURL holds the password and every query value inside a base URL.
func HoldBaseURL(raw string) {
	holdURLText(raw) // a URL that does not parse gives them up as well
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return
	}
	if u.User != nil {
		if p, ok := u.User.Password(); ok {
			HoldCredential("userinfo", p)
		}
	}
	for name, vals := range u.Query() {
		HoldCredential("query:"+name, vals...)
	}
}

// holdURLText holds the password and the credential query values of a base URL,
// read from the text itself so a URL that does not parse gives them up too. A
// query value counts when its name says so or when it is long enough to be a
// credential: a short one (page=1) would withhold every output that shows it.
func holdURLText(text string) {
	for _, raw := range hsURLInText.FindAllString(text, -1) {
		if m := hsURLPassword.FindStringSubmatch(raw); m != nil {
			hsHoldDecoded("userinfo", m[1])
		}
		_, query, _ := strings.Cut(raw, "?")
		query, _, _ = strings.Cut(query, "#")
		for _, pair := range strings.Split(query, "&") {
			name, value, _ := strings.Cut(pair, "=")
			if decoded, err := url.QueryUnescape(name); err == nil {
				name = decoded
			}
			if value != "" && (len(value) >= 8 || CredentialFieldName(name)) {
				hsHoldDecoded("query:"+name, value)
			}
		}
	}
}

func hsHoldDecoded(source, value string) {
	HoldCredential(source, value)
	if decoded, err := url.QueryUnescape(value); err == nil && decoded != value {
		HoldCredential(source, decoded)
	}
}

// CredentialFieldName reports whether a field or query name says it holds a credential.
func CredentialFieldName(name string) bool {
	name = strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(name))
	for _, marker := range []string{"token", "secret", "passw", "pwd", "key", "auth", "credential", "sig", "cookie", "bearer", "clientid"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// ReadRegular is the bytes of a regular file, nil when it is missing, is not a
// regular file or cannot be read: this process cannot print what it cannot read.
func ReadRegular(path string) []byte {
	if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	data, err := fs.ReadFile(os.DirFS(filepath.Dir(path)), filepath.Base(path))
	if err != nil {
		return nil
	}
	return data
}
