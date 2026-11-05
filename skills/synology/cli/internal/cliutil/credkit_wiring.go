// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): the seams between
// this connector and the shared credential kit. The kit holds every startup
// credential itself (internal/credwire); this file holds what is read later.

package cliutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"

	"synology-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Hold is credkit.Hold and OutboundGuard is credkit.Transport. Production code
// never reassigns either. In-process unit tests do: they never run
// credkit.Start, so the kit has no base origin and would refuse every
// credentialed request to a loopback test server, and a held value lasts for
// the process, so one test's fixture would withhold the next test's output.
// The built binaries' leak tests (internal/credwire) prove the real thing.
var (
	Hold          = credkit.Hold
	OutboundGuard = credkit.Transport
)

// credentialEnvSuffixes name a credential variable of the CLI or of a fleet NAS
// under any prefix (<prefix>_PASSWORD, <prefix>_OTP_CODE, <prefix>_DEVICE_ID).
var credentialEnvSuffixes = []string{"_PASSWORD", "_OTP_CODE", "_DEVICE_ID"}

// CredentialEnvName reports whether name holds a DSM credential.
func CredentialEnvName(name string) bool {
	up := strings.ToUpper(name)
	for _, s := range credentialEnvSuffixes {
		if strings.HasSuffix(up, s) {
			return true
		}
	}
	return false
}

// Where a held store value may go. The kit sends a value only to the origins of the
// declared sources that held it (credwire.Declarations.Origins).
const (
	// SentNowhere is a value this process can read but does not use: another NAS's
	// session cache, a config this run does not load. Output withholds it.
	SentNowhere = "minted:other-nas"
	// SentToThisNAS is a value this process uses: only the NAS it is bound to gets it.
	SentToThisNAS = "minted:this-nas"
)

// HoldConfigFile holds what a config or credentials file carries, before it is parsed:
// the MCP server reloads the file on every tool call, and a file written since startup
// is one the kit has not read. A file that does not parse has every value held, since
// which field is the credential cannot be known. All of it goes nowhere: config.Load
// binds to this NAS what it keeps of the file for this NAS.
func HoldConfigFile(data []byte) {
	var tree map[string]any
	if toml.Unmarshal(data, &tree) == nil {
		HoldCredentialTree(SentNowhere, tree, false)
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == '[' && strings.HasSuffix(line, "]") {
			continue
		}
		address := false
		if i := strings.IndexAny(line, "=:"); i >= 0 {
			address = strings.EqualFold(strings.Trim(line[:i], " \t\"'"), "base_url")
			line = line[i+1:]
		}
		for _, v := range strings.FieldsFunc(line, func(r rune) bool { return strings.ContainsRune("\"',{}[]", r) }) {
			switch v = strings.TrimSpace(v); {
			case v == "", v == "true", v == "false", v == "null":
			case address: // its pieces would withhold every request to its subnet
				HoldURL(SentNowhere, v)
			default:
				Hold(SentNowhere, v)
			}
		}
	}
}

// credentialKey names a field whose value (and everything under it) is a credential.
func credentialKey(k string) bool {
	if SecretFieldName(k) {
		return true
	}
	switch normalKey(k) {
	case "deviceid", "deviceids", "authheader", "headers", "authorization":
		return true
	}
	return false
}

func normalKey(k string) string {
	return strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(k))
}

// RememberMeCookie names the one DSM cookie that is not a credential: its value is the
// flag 0 or 1, declared in credwire's Identifiers, and the kit refuses to start on a
// held startup value that collides with public text. Every other cookie's value is held.
func RememberMeCookie(name string) bool { return strings.EqualFold(name, "stay_login") }

// HoldCredentialTree holds under source every credential in a decoded config or
// session record: a credential key's values, cookie values and what a base URL carries.
// A URL anywhere else is free text, which the credential contract's closed list of
// sources leaves out.
func HoldCredentialTree(source string, v any, secret bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if strings.EqualFold(k, "cookies") {
				holdCookies(source, child)
				HoldCredentialTree(source, child, false)
				continue
			}
			if !secret && normalKey(k) == "baseurl" {
				holdURLs(source, child) // and a table under it is searched below
			}
			HoldCredentialTree(source, child, secret || credentialKey(k))
		}
	case []any:
		for _, child := range t {
			HoldCredentialTree(source, child, secret)
		}
	case string:
		if secret {
			Hold(source, t)
		}
	case json.Number, int64, float64: // numbers count
		if secret {
			Hold(source, fmt.Sprint(t))
		}
	}
}

// holdURLs is holdURL on a base URL field's value, a duplicate key's list included.
func holdURLs(source string, v any) {
	switch t := v.(type) {
	case string:
		HoldURL(source, t)
	case []any:
		for _, child := range t {
			holdURLs(source, child)
		}
	}
}

// holdCookies holds the value of every cookie in a cookie list but the remember-me flag.
func holdCookies(source string, v any) {
	switch t := v.(type) {
	case []any:
		for _, child := range t {
			holdCookies(source, child)
		}
	case map[string]any:
		if n, _ := t["name"].(string); !RememberMeCookie(n) {
			HoldCredentialTree(source, t["value"], true)
		}
	}
}

// maxJSONDepth stops a hostile file before it overflows the stack, which no Go
// program can recover from. Past it the file reads as torn.
const maxJSONDepth = 1000

// memberDecoder decodes JSON keeping every member of a duplicate key: a repeated key's
// values become a list, which HoldCredentialTree walks under that key. On a file that
// stops parsing it keeps what came before, and says what the cut value was.
type memberDecoder struct {
	dec     *json.Decoder
	inName  bool // the cut came where a key or a named value's name was expected
	pending bool // the cut value belongs to a credential name
	named   bool // the cut value has a name, so the name decides
}

func newMemberDecoder(data []byte) *memberDecoder {
	d := &memberDecoder{dec: json.NewDecoder(bytes.NewReader(data))}
	d.dec.UseNumber()
	return d
}

// decodeEveryMember is a whole file through memberDecoder.
func decodeEveryMember(data []byte) (any, error) {
	d := newMemberDecoder(data)
	v, err := d.value(false, false, 0)
	if err != nil {
		return nil, err
	}
	if _, err := d.dec.Token(); err != io.EOF {
		return nil, errors.New("data after the JSON value")
	}
	return v, nil
}

func (d *memberDecoder) value(secret, cookie bool, depth int) (any, error) {
	if depth > maxJSONDepth {
		return nil, errors.New("JSON nested too deeply")
	}
	tok, err := d.dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	if delim == '[' {
		list := []any{}
		for d.dec.More() {
			v, err := d.value(secret, cookie, depth+1)
			if v != nil {
				list = append(list, v)
			}
			if err != nil {
				return list, err
			}
		}
		_, err := d.dec.Token()
		return list, err
	}
	members := map[string]any{}
	for d.dec.More() {
		k, err := d.dec.Token()
		if err != nil {
			d.inName = true
			return members, err
		}
		key, _ := k.(string)
		isCookies := strings.EqualFold(key, "cookies")
		v, err := d.value(secret || credentialKey(key) && !isCookies, cookie || isCookies, depth+1)
		if v != nil {
			if old, dup := members[key]; dup {
				members[key] = []any{old, v}
			} else {
				members[key] = v
			}
		}
		if err != nil {
			if !d.named && !d.inName {
				name, _ := members["name"].(string)
				d.named = true
				d.pending = secret || credentialKey(key) && !strings.EqualFold(key, "cookies") || cookie && key == "value" && !RememberMeCookie(name)
			}
			return members, err
		}
	}
	_, err = d.dec.Token()
	return members, err
}

var storeStringRE = regexp.MustCompile(`"((?:[^"\\]|\\.)*)("|$)`)

// HoldJSONStore holds what a session cache carries, under the rules a whole file gets
// for what parsed before any cut. The value at the cut is held when its name is a
// credential's. Only text no name reaches is held by shape: any address's password, and
// each other string of 8 or more characters.
//
// ponytail: that floor keeps a torn file's stray words from refusing every start, and
// DSM's credentials are long random strings. It applies to no named value.
func HoldJSONStore(data []byte) {
	d := newMemberDecoder(data)
	rec, err := d.value(false, false, 0)
	HoldCredentialTree(SentNowhere, rec, false)
	if err == nil {
		if _, err = d.dec.Token(); err == io.EOF {
			return
		}
	}
	tail := bytes.TrimLeft(data[d.dec.InputOffset():], " \t\r\n,:")
	switch {
	case d.inName:
	case d.pending:
		Hold(SentNowhere, strings.TrimSuffix(strings.TrimPrefix(string(tail), `"`), `"`))
	case d.named:
		if strings.Contains(string(tail), "://") {
			HoldURL(SentNowhere, string(tail))
		}
	default:
		for _, m := range storeStringRE.FindAllSubmatch(tail, -1) {
			if v := string(m[1]); strings.Contains(v, "://") {
				HoldURL(SentNowhere, v)
			} else if len(v) >= 8 {
				Hold(SentNowhere, v)
			}
		}
	}
}

// HoldProfileStore holds what a run profile store carries. A profile's name is the
// operator's word, so it is not read as a credential's name.
func HoldProfileStore(data []byte) {
	rec, err := decodeEveryMember(data)
	if err != nil {
		HoldJSONStore(data)
		return
	}
	top, _ := rec.(map[string]any)
	stores := []any{top["profiles"]}
	if dup, ok := top["profiles"].([]any); ok {
		stores = dup
	}
	for _, store := range stores {
		profiles, _ := store.(map[string]any)
		for _, profile := range profiles {
			HoldCredentialTree(SentNowhere, profile, false)
		}
	}
}

// HoldURL is the kit's URL reader (credkit.HoldURL): a URL's password and every query
// and fragment value, whatever the name, in any spelling and whether or not it parses.
// A test seam like Hold.
var HoldURL = credkit.HoldURL

// HoldURLCredentials is HoldURL on only the password and the query and fragment values
// whose name is a credential's: for an address sent only to its own host, where an
// ordinary query (?v=1) held would withhold every line carrying it. An address that
// does not parse goes to HoldURL whole (fail closed).
func HoldURLCredentials(source, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	u, err := url.Parse(raw)
	if err != nil {
		HoldURL(source, raw)
		return
	}
	keep := url.Values{}
	for _, part := range []string{u.RawQuery, u.Fragment} {
		vs, err := url.ParseQuery(part)
		if err != nil {
			HoldURL(source, raw)
			return
		}
		for k, v := range vs {
			if credentialKey(k) {
				keep[k] = append(keep[k], v...)
			}
		}
	}
	_, hasPassword := u.User.Password()
	if !hasPassword && len(keep) == 0 {
		return
	}
	held := url.URL{Scheme: u.Scheme, User: u.User, Host: u.Host, RawQuery: keep.Encode()}
	HoldURL(source, held.String())
}

// HoldConfigFilePath is HoldConfigFile on a file. Only a regular file is opened.
func HoldConfigFilePath(path string) {
	if data, err := ReadRegularFile(path); err == nil {
		HoldConfigFile(data)
	}
}

// HoldTLSKeyFile holds the private key the MCP server would serve TLS with: each PEM
// body line. Armor and header lines are not the key, and holding them would withhold
// any PEM text. Only a regular file is opened.
func HoldTLSKeyFile(path string) {
	data, err := ReadRegularFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "-----") && !strings.Contains(line, ":") {
			Hold("minted:tls-key", line)
		}
	}
}

// ReadRegularFile reads path when it is a regular file: a pipe named as a config or
// store would hang startup.
func ReadRegularFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.ReadFile(path) // #nosec G304 -- the operator's own config, store or key path, read to hold its values
}
