// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring). Every read of a
// config or credentials file holds its values with the credential kit BEFORE
// the file is parsed: the kit's startup discovery holds the files it names
// once; a file written after that, or named another way, is held here.

package cliutil

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"immybot-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

var (
	configTableLine = regexp.MustCompile(`^\[+\s*([^\]]*?)\s*\]+$`)
	configQuoted    = regexp.MustCompile(`"((?:[^"\\]|\\.)*)("|$)|'([^']*)('|$)`)
)

// configIdentifierKey names the top-level keys credwire declares Identifiers.
var configIdentifierKey = map[string]bool{"ca_cert": true, "tls_server_name": true, "token_expiry": true, "client_id": true, "tenant_id": true}

// HoldConfigBytes holds every value in the bytes of a config or credentials
// file, trusting no parse: a file that is malformed, permission-refused or
// legacy gives up its values like an accepted one. Only a top-level key
// credwire declares an identifier, and the template variables (the instance's
// host name label), are left; a URL key gives up the credentials inside it.
func HoldConfigBytes(data []byte) {
	table := ""
	for _, line := range strings.FieldsFunc(string(data), func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m := configTableLine.FindStringSubmatch(line); m != nil && !strings.Contains(line, "=") {
			table = strings.ToLower(m[1])
			continue
		}
		key, rhs, ok := strings.Cut(line, "=")
		if !ok {
			HoldValues("config:unclassified", configLineValues(line)...)
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		vals := configLineValues(rhs)
		lower := strings.ToLower(key)
		switch {
		case table == "template_vars", table == "" && strings.HasPrefix(lower, "template_vars."):
		case table == "headers":
			HoldValues("header:"+lower, vals...)
		case table == "" && (lower == "base_url" || lower == "token_url"):
			for _, v := range vals {
				HoldURLCredentials(v)
			}
		case table == "" && configIdentifierKey[lower]:
		case table == "":
			HoldValues("config:"+key, vals...)
		default:
			HoldValues("config:"+table+"."+key, vals...)
		}
	}
}

// configLineValues is every quoted string on the right of an assignment (an
// unterminated one runs to the end of the line), else its bare words up to a
// comment.
func configLineValues(rhs string) []string {
	var out []string
	for _, m := range configQuoted.FindAllStringSubmatch(rhs, -1) {
		if m[1] != "" {
			out = append(out, m[1])
		} else if m[3] != "" {
			out = append(out, m[3])
		}
	}
	if len(out) > 0 {
		return out
	}
	bare, _, _ := strings.Cut(rhs, "#")
	return strings.Fields(bare)
}

// HoldURLCredentials holds the password and every query value inside one URL,
// with the kit's own reader: a URL that does not parse, a schemeless authority
// and a network path included.
func HoldURLCredentials(raw string) {
	credkit.HoldURL("userinfo", raw)
}

// Hold holds every credential a credentials file carries. ClientID, the tenant
// id and the base_url itself are identifiers.
func (c *Credentials) Hold() {
	if c == nil {
		return
	}
	HoldValues("config:auth_header", c.AuthHeaderVal)
	HoldValues("config:access_token", c.AccessToken)
	HoldValues("config:refresh_token", c.RefreshToken)
	HoldValues("config:client_secret", c.ClientSecret)
	HoldURLCredentials(c.BaseURL)
}

// HoldFrom holds every credential the decoded c carries and every value its
// file's bytes carry even where the decode failed.
func (c *Credentials) HoldFrom(data []byte) {
	c.Hold()
	HoldConfigBytes(data)
}

// HoldCredentialsFile holds every credential in the credentials file at path,
// without applying it or printing anything. It reads through the loader's own
// trusted reader; a file that reader refuses is held by the reader itself
// (credentialsBytesHaveValues), never applied.
func HoldCredentialsFile(path string) {
	data, err := readGlobalCredentialBytes(path)
	if err != nil {
		return
	}
	var creds Credentials
	_ = toml.Unmarshal(data, &creds)
	creds.HoldFrom(data)
}

// OutboundGuard is the credential kit's outbound guard, around every HTTP
// client this connector builds (handfixes.json credkit-wiring). Production
// code never reassigns it. In-process unit tests do, to the identity: they
// never run credkit.Start, so the kit has no declarations and no base origin
// and would refuse every credentialed request to a loopback test server. The
// built binaries' leak tests (cmd/immybot-mcp) prove the guard itself.
var OutboundGuard = credkit.Transport

// HoldValues is credkit.Hold, the one way this connector's own code holds a
// value. Production code never reassigns it. In-process unit tests do, to a
// no-op: any held value makes every SQL statement pass the kit's decoders, and a
// test process that never ran credkit.Start only piles them up.
var HoldValues = credkit.Hold

// JSONLeaves calls leaf with the keys leading to every string and number of a JSON
// document, read token by token: every member of a duplicate key is seen, and a
// number keeps its spelling. Each object key is reported too, with an empty value.
// A document that does not parse has no leaves.
func JSONLeaves(data []byte, leaf func(keys []string, value string)) {
	if !json.Valid(data) { // also bounds the recursion: Valid refuses a deep nest
		return
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var walk func(keys []string)
	walk = func(keys []string) {
		t, err := d.Token()
		if err != nil {
			return
		}
		switch v := t.(type) {
		case string:
			leaf(keys, v)
		case json.Number:
			leaf(keys, v.String())
		case json.Delim:
			for d.More() {
				next := keys
				if v == '{' {
					name, _ := d.Token()
					key, _ := name.(string)
					next = append(slices.Clone(keys), key)
					leaf(next, "")
				}
				walk(next)
			}
			_, _ = d.Token() // the closing delimiter
		}
	}
	walk(nil)
}
