// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// zoom-phone hand-fix (credkit-wiring): a run profile value under a credential
// flag's name or under base-url is a credential source (contract section 1), held
// at startup, every duplicate included. Free text in another field is not a source.
// Not through Declarations.Files: the kit answers nested JSON by holding every
// string, so a profile named "p" or a saved --limit 7 would refuse every later start.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// HoldProfileStore holds the credentials of one profile store.
func HoldProfileStore(path string) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	_ = holdProfileValue(dec, "") // a file that does not parse is held up to where it stops; the CLI refuses it
}

// HoldLegacyProfileStore holds the store under the old per-user directory.
func HoldLegacyProfileStore() {
	if home, err := os.UserHomeDir(); err == nil {
		HoldProfileStore(filepath.Join(home, ".zoom-phone-cli", "profiles.json"))
	}
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
	if strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(key)) == "baseurl" {
		HoldBaseURL(value)
	} else if IsCredentialFieldName(key) || strings.Contains(strings.ToLower(key), "client-id") || strings.Contains(strings.ToLower(key), "client_id") {
		Hold("config:"+key, value)
	}
}
