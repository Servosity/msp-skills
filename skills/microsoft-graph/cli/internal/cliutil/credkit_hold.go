// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): what a
// run-profile store (profiles.json) carries, held at startup through the
// credential kit (internal/credwire).

package cliutil

import (
	"bytes"
	"encoding/json"
	"strings"

	"microsoft-graph-pp-cli/internal/credkit"
)

// HoldRunProfileStore holds what a run-profile store carries as a source of
// contract section 1, every member of a duplicate key included: a value saved
// under a credential-named flag (ApplyProfileToFlags refuses those), a saved
// --client-id or --client-secret under the flag's own source (auth login sends
// them to Entra), and what a value saved under a base-url flag carries, read by
// credkit.HoldURL. A store that does not parse as the program reads it is held
// by credkit.HoldBody. Free text (a description, a --deliver URL) is no source
// and is not read.
func HoldRunProfileStore(data []byte) {
	var store struct {
		Profiles map[string]struct {
			Values map[string]string `json:"values"`
		} `json:"profiles"`
	}
	if json.Unmarshal(data, &store) != nil {
		credkit.HoldBody(data)
		return
	}
	eachProfileValue(data, func(flag, value string) {
		name := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(flag), "_", "-"))
		switch {
		case value == "":
		case name == "client-id", name == "client-secret":
			credkit.Hold(credkit.Source("flag:--"+name), value)
		case CredentialProfileFlag(name):
			credkit.Hold("minted:profile", value)
		case strings.Contains(name, "base-url"):
			credkit.HoldURL("minted:profile", value)
		}
	})
}

// CredentialProfileFlag is cli.credentialLikeRunProfileFlag's rule: a flag name
// whose saved value is a credential. TestCredentialProfileFlagMatchesTheCLI
// (internal/cli) keeps the two the same.
func CredentialProfileFlag(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
	for _, marker := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key", "client-secret"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// eachProfileValue calls f for every flag and value under profiles.<name>.values
// in data, which json.Unmarshal has already read, a key repeated at any level
// included: Unmarshal keeps only the last of each.
func eachProfileValue(data []byte, f func(flag, value string)) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func(path []string)
	walk = func(path []string) {
		tok, err := dec.Token()
		if err != nil {
			return
		}
		switch tok {
		case json.Delim('{'):
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return
				}
				name, _ := key.(string)
				walk(append(path, name))
			}
			_, _ = dec.Token()
		case json.Delim('['):
			for dec.More() {
				walk(append(path, ""))
			}
			_, _ = dec.Token()
		default:
			// Unmarshal matches a member name to its field ignoring case.
			if s, ok := tok.(string); ok && len(path) == 4 && strings.EqualFold(path[0], "profiles") && strings.EqualFold(path[2], "values") {
				f(path[3], s)
			}
		}
	}
	walk(nil)
}
