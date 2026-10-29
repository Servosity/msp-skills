// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring).
// The connector reads no document of its own: a config, credential or profile
// file goes to credkit.HoldBody, a URL to credkit.HoldURL. What stays here is
// what HoldBody does not read: a config's [headers] values and the credentials
// inside its base_url, and which fields of a run profile store are a URL or a
// credential.

package cliutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pax8-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// Hold, HoldBody and HoldURL are the kit's. Production code never reassigns them.
// In-process unit tests may, to no-ops: a held value lasts for the process and the
// kit withholds whatever carries it, so one test's fixture would withhold the next
// test's output. The built binaries' leak tests (internal/credwire) prove the real thing.
var (
	Hold     = credkit.Hold
	HoldBody = credkit.HoldBody
	HoldURL  = credkit.HoldURL
)

// HoldConfigFile holds a config or credential file's credentials before it is
// parsed, with HoldBody. Kept beside it: a [headers] value and the
// password or a query value inside base_url, which contract 1 names and HoldBody
// does not read from a config.
func HoldConfigFile(data []byte) {
	HoldBody(data)
	var parsed struct {
		BaseURL string         `toml:"base_url"`
		Headers map[string]any `toml:"headers"`
	}
	if toml.Unmarshal(data, &parsed) != nil {
		return // bytes that do not parse: HoldBody held every string in them
	}
	HoldURL("userinfo", parsed.BaseURL) // not config:base_url, which an Identifier exempts
	for name, v := range parsed.Headers {
		switch v.(type) {
		case string, int64, float64:
			Hold("header:"+strings.ToLower(name), fmt.Sprint(v))
		}
	}
}

// HoldFile is HoldConfigFile on a file. Only a regular file is opened, so a FIFO
// named on the command line cannot hang startup.
func HoldFile(path string) {
	if data, err := readRegularFile(path); err == nil {
		HoldConfigFile(data)
	}
}

// HoldProfileFile holds what a run profile store carries as a credential. HoldBody
// holds the declared names at any depth. Read here, by name: a value under base-url
// or token-url, through HoldURL, and a value under any other credential's name.
// Not through Declarations.Files: the kit answers nested JSON by holding every
// string, so a profile named "p" or a saved --limit 7 would refuse every later
// start. Free text (a description, a URL in another field) is not a source:
// contract section 1.
func HoldProfileFile(path string) {
	data, err := readRegularFile(path)
	if err != nil {
		return
	}
	HoldBody(data) // bytes that do not parse: every string in them, and the CLI refuses the store
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	_ = holdProfileValue(dec, "")
}

// holdProfileValue walks tokens, not a decoded tree, which keeps only the last
// member of a duplicate key.
func holdProfileValue(dec *json.Decoder, key string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch x := tok.(type) {
	case json.Delim: // '{' or '[': the decoder never returns a closing one here
		for dec.More() {
			if x == '{' {
				if tok, err = dec.Token(); err != nil {
					return err
				}
				key, _ = tok.(string)
			}
			if err := holdProfileValue(dec, key); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case json.Number:
		holdProfileString(key, x.String())
	case string:
		holdProfileString(key, x)
	}
	return nil
}

func holdProfileString(key, value string) {
	switch name := strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(key)); {
	case name == "baseurl", name == "tokenurl":
		HoldURL("userinfo", value) // not config:base_url, which an Identifier exempts
	case CredentialFieldName(key):
		Hold("config:"+key, value)
	}
}

// CredentialFieldName reports whether a profile field's name says it holds a
// credential. It covers every name run profiles refuse to apply
// (credentialLikeRunProfileFlag); internal/cli checks that against every flag.
func CredentialFieldName(name string) bool {
	name = strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(name))
	for _, marker := range []string{"token", "secret", "passw", "pwd", "key", "auth", "credential", "sig", "cookie", "bearer", "clientid"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.ReadFile(filepath.Clean(path)) // the operator's own config, store or profile path, read to hold its values
}
