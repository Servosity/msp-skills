// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): handfixes.json profile-values-held.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"pagerduty-pp-cli/internal/cliutil"
	"pagerduty-pp-cli/internal/credkit"
)

// HoldProfileStores holds what the run profile stores under the current home
// carry as a credential, for the kit's startup discovery: a value under a
// credential flag's name or under base-url, every duplicate included. Not
// through Declarations.Files: the kit answers nested JSON by holding every
// string, so a profile named "p" or a saved --limit 7 would refuse every later
// start. Free text (a description, a URL in another field) is not a source:
// contract section 1. runProfileAllowedFlags still decides which values a run
// applies.
func HoldProfileStores() {
	var paths []string
	if dir, err := cliutil.ConfigDir(); err == nil { // not profileStorePath, which creates the directory
		paths = append(paths, filepath.Join(dir, "profiles.json"))
	}
	if legacy, err := legacyProfileStorePath(); err == nil {
		paths = append(paths, legacy)
	}
	for _, path := range paths {
		dec := json.NewDecoder(bytes.NewReader(readRegularFile(path)))
		dec.UseNumber()
		_ = holdProfileValue(dec, "") // a file that does not parse is held up to where it stops; the CLI refuses it
	}
}

// readRegularFile is path's bytes when it is a regular file (a FIFO would
// block the start), else nothing.
func readRegularFile(path string) []byte {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil
	}
	return data
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

// profileCredentialFlags are the credential flags credentialLikeRunProfileFlag
// does not name by its markers (credwire's CredentialFlags).
var profileCredentialFlags = []string{"webhook-subscription-delivery-method-custom-headers"}

func holdProfileString(key, value string) {
	name := strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(key))
	switch {
	case name == "baseurl":
		credkit.HoldURL("userinfo", value)
	case credentialLikeRunProfileFlag(key):
		credkit.Hold("config:"+key, value)
	default:
		for _, f := range profileCredentialFlags {
			if strings.EqualFold(strings.ReplaceAll(key, "_", "-"), f) {
				credkit.Hold("config:"+key, value)
			}
		}
	}
}
