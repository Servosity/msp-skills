// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring). What the shared
// credential kit cannot read for itself: a config or credentials file read
// after startup, every value of a token answer, the URL in a named URL setting,
// and a saved run profile's credential values.

package cliutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"quickbooks-pp-cli/internal/credkit"
)

// OutboundGuard is the credential kit's outbound guard, around every request
// the API client sends. Production code never reassigns it. In-process unit
// tests do, to the identity: they never run credkit.Start, so the kit has no
// declarations and no base origin and would refuse every credentialed request
// to a loopback test server. The built binaries' leak tests prove the guard.
var OutboundGuard = credkit.Transport

// HoldCredential is credkit.Hold, through which the connector holds every
// credential it reads. Production code never reassigns it. In-process unit
// tests do, to a no-op: a hold lasts until the process exits, so one test's
// token would be checked against every later test's files and statements.
var HoldCredential = credkit.Hold

var (
	qboTableLine = regexp.MustCompile(`^\[+\s*([^\]]*?)\s*\]+$`)
	qboQuoted    = regexp.MustCompile(`"((?:[^"\\]|\\.)*)("|$)|'([^']*)('|$)`)
)

// qboNotCredentialKeys names the keys credwire declares Identifiers. A
// credential inside base_url is still held (QBOHoldURL).
var qboNotCredentialKeys = map[string]bool{"base_url": true, "ca_cert": true, "tls_server_name": true, "token_expiry": true, "supersedes_config": true}

// QBOHoldConfigBytes holds every value in the bytes of a config or credentials
// file, trusting no parse: a file that is malformed, permission-refused or
// written by another process after this one started gives up its values like
// an accepted one. Each value is held under its own key, so it keeps the
// origins credwire binds that key to.
func QBOHoldConfigBytes(data []byte) {
	table := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m := qboTableLine.FindStringSubmatch(line); m != nil && !strings.Contains(line, "=") {
			table = strings.ToLower(m[1])
			continue
		}
		key, rhs, ok := strings.Cut(line, "=")
		if !ok {
			HoldCredential("config:unclassified", qboLineValues(line)...)
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		vals := qboLineValues(rhs)
		switch {
		case table == "headers":
			HoldCredential("header:"+strings.ToLower(key), vals...)
		case table == "" && qboNotCredentialKeys[key]:
			if key == "base_url" {
				for _, v := range vals {
					QBOHoldURL(v)
				}
			}
		case table == "":
			HoldCredential("config:"+key, vals...)
		default:
			HoldCredential("config:"+table+"."+key, vals...)
		}
	}
}

// qboLineValues is every quoted string on the right of an assignment (an
// unterminated one runs to the end of the line), else its bare words up to a
// comment.
func qboLineValues(rhs string) []string {
	var out []string
	for _, m := range qboQuoted.FindAllStringSubmatch(rhs, -1) {
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

// QBODecodeTOML is the one decode of a config or credentials file. It holds
// every value first, and its error names only where the file is malformed,
// never the file's text.
func QBODecodeTOML(data []byte, v any) error {
	QBOHoldConfigBytes(data)
	err := toml.Unmarshal(data, v)
	if err == nil {
		return nil
	}
	var de *toml.DecodeError
	if errors.As(err, &de) {
		row, col := de.Position()
		return fmt.Errorf("invalid TOML at line %d, column %d", row, col)
	}
	return errors.New("invalid TOML")
}

// QBOHoldTOMLCredentials holds every credential a config or credentials value
// carries, set in memory or decoded: every string field with a TOML key except
// qboNotCredentialKeys, and every value of a string table such as [headers]. A
// field a reprint adds is held with no change here.
func QBOHoldTOMLCredentials(v any) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		key, _, _ := strings.Cut(f.Tag.Get("toml"), ",")
		if key == "" {
			key = f.Name
		}
		if !f.IsExported() || key == "-" {
			continue
		}
		switch fv := rv.Field(i); fv.Kind() {
		case reflect.String:
			if key == "base_url" {
				QBOHoldURL(fv.String())
			} else if !qboNotCredentialKeys[key] && fv.String() != "" {
				HoldCredential("config:"+key, fv.String())
			}
		case reflect.Map:
			if fv.Type().Elem().Kind() == reflect.String {
				for it := fv.MapRange(); it.Next(); {
					HoldCredential("header:"+strings.ToLower(it.Key().String()), it.Value().String())
				}
			}
		}
	}
}

// qboNotCredentialJSONKeys are the only token-answer members whose values are
// not secrets: RFC 6749 section 5.1's type, lifetime and scope, and Intuit's
// refresh-token lifetime.
var qboNotCredentialJSONKeys = map[string]bool{"token_type": true, "expires_in": true, "x_refresh_token_expires_in": true, "scope": true}

// QBODecodeCredentialJSON is the one decode of an answer that carries
// credentials (a token endpoint's). It holds every string and number in the
// answer, at any depth, as it is read, except under qboNotCredentialJSONKeys:
// a member a duplicate key overwrites, and everything read before a syntax
// error, is held too. Its error names only a byte offset, never the answer's
// text (a value a decode error quotes may be a credential).
func QBODecodeCredentialJSON(data []byte, v any) error {
	qboHoldCredentialJSON(data, nil)
	err := json.Unmarshal(data, v)
	if err == nil {
		return nil
	}
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &se):
		return fmt.Errorf("invalid JSON at byte %d", se.Offset)
	case errors.As(err, &te):
		return fmt.Errorf("a JSON value of the wrong type at byte %d", te.Offset)
	}
	return errors.New("invalid JSON")
}

// QBOHoldTokenErrorAnswer holds every value of a refused token answer (a
// non-200 body can still name an access or refresh token), at any depth and
// under any key, "error" and duplicate keys included. The only values it
// leaves are strings exactly equal to one of publicCodes (the RFC 6749 error
// codes, public words the refusal shows). Hand-fix (handfixes.json
// qbo-token-refresh): a token answer is held whatever its status.
func QBOHoldTokenErrorAnswer(data []byte, publicCodes map[string]bool) {
	qboHoldCredentialJSON(data, publicCodes)
}

// qboHoldCredentialJSON holds every string and number in data, at any depth,
// as it is read, except under qboNotCredentialJSONKeys and except a string
// exactly equal to one of publicValues.
func qboHoldCredentialJSON(data []byte, publicValues map[string]bool) {
	qboWalkJSON(data, func(key, value string) {
		if !qboNotCredentialJSONKeys[key] && !publicValues[value] {
			HoldCredential("minted:token-answer", value)
		}
	})
}

// qboWalkJSON gives visit every string and number in data with the key it sits
// under (an array's members under the array's key), token by token: a member a
// duplicate key overwrites and everything before a syntax error are visited too.
func qboWalkJSON(data []byte, visit func(key, value string)) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	type frame struct {
		object, wantKey bool
		key             string
	}
	stack := []frame{{}}
	for {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		top := &stack[len(stack)-1]
		if s, ok := tok.(string); ok && top.object && top.wantKey {
			top.key, top.wantKey = s, false
			continue
		}
		key := top.key
		if top.object {
			top.wantKey = true
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, frame{object: true, wantKey: true})
			case '[':
				stack = append(stack, frame{key: key})
			default:
				if len(stack) > 1 {
					stack = stack[:len(stack)-1]
				}
			}
		case string:
			visit(key, t)
		case json.Number:
			visit(key, t.String())
		}
	}
}

// qboCredentialQueryKey names a URL query parameter whose value is a secret.
var qboCredentialQueryKey = regexp.MustCompile(`(?i)token|secret|passw|key|auth|sig|credential|code`)

// QBOHoldURL holds the credentials in the URL of one named setting (base_url, a
// token or feedback endpoint): its password, or a username given without one,
// and a query value under a credential-named parameter. Free text is never read:
// a URL in a description or a non-credential flag is outside the leak contract's
// closed list of sources (docs/credential-leak-contract.md section 1).
func QBOHoldURL(raw string) {
	HoldCredential("minted:input-url", qboURLCredentials(raw)...)
}

// qboURLCredentials does not parse: a URL url.Parse refuses still states these.
func qboURLCredentials(raw string) []string {
	var out []string
	add := func(v string) {
		out = append(out, v)
		if u, err := url.QueryUnescape(v); err == nil && u != v {
			out = append(out, u)
		}
	}
	if _, after, ok := strings.Cut(raw, "//"); ok {
		authority := after
		if end := strings.IndexAny(after, "/?#"); end >= 0 {
			authority = after[:end]
		}
		if at := strings.LastIndex(authority, "@"); at >= 0 {
			if user, pass, hasPass := strings.Cut(authority[:at], ":"); hasPass && pass != "" {
				add(pass)
			} else if user != "" {
				add(user)
			}
		}
	}
	_, after, _ := strings.Cut(raw, "?")
	query, _, _ := strings.Cut(strings.TrimSpace(after), "#")
	for _, pair := range strings.FieldsFunc(query, func(r rune) bool { return r == '&' || r == ';' }) {
		k, v, _ := strings.Cut(pair, "=")
		if u, err := url.QueryUnescape(k); err == nil {
			k = u
		}
		if v != "" && qboCredentialQueryKey.MatchString(k) {
			add(v)
		}
	}
	return out
}

func qboProfileFlagName(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
}

// QBOCredentialProfileFlag names a flag whose value is a credential: profile
// save refuses it, and a saved one is held at startup.
func QBOCredentialProfileFlag(name string) bool {
	name = qboProfileFlagName(name)
	for _, marker := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key", "client-secret"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// QBOHoldProfileStore holds what the leak contract makes a source in a saved
// profiles.json, at startup: every value under a credential flag's name, whole,
// and the credentials in a base-url value, every member of a duplicate key
// included. The file is not on the kit's list, which would hold every string in
// it, its JSON keys too, and withhold any output naming a profile.
func QBOHoldProfileStore(data []byte) {
	qboWalkJSON(data, func(flag, value string) {
		if QBOCredentialProfileFlag(flag) {
			HoldCredential("profile:value", value)
		} else if qboProfileFlagName(flag) == "base-url" {
			QBOHoldURL(value)
		}
	})
}

// QBOHoldEnvInputs holds the credentials in the endpoints the environment names,
// inside credkit.Start, before it can refuse. The kit reads QUICKBOOKS_BASE_URL
// and the proxy variables itself. Its URLEnv is not used here: it holds every
// query value, so ?v=2 on a webhook would refuse every command.
func QBOHoldEnvInputs() {
	for _, name := range []string{"QUICKBOOKS_TOKEN_URL", "QUICKBOOKS_FEEDBACK_ENDPOINT"} {
		QBOHoldURL(os.Getenv(name))
	}
}
