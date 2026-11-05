// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring and
// credkit-profiles-held).

package cliutil

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"crowdstrike-pp-cli/internal/credkit"
)

// OutboundGuard is the credential kit's outbound guard, around every HTTP
// transport this program builds: the API client (under the origin pin), the
// token client of auth login and the fleet member_cid mint. In-process unit
// tests never run credkit.Start, so a test package sets it to the identity in
// its credkit_seam_test.go; the built binaries always use the kit.
var OutboundGuard = credkit.Transport

// HoldValues, HoldBody and HoldURL are the kit's holds, through which the
// program holds what it acquires after startup: a minted token, a config file
// read again (the MCP server reads it on every call), a URL setting. In-process
// unit tests never run credkit.Start, and one held value would withhold every
// later test's unrelated output in the same process, so a test package sets
// them to no-ops in its credkit_seam_test.go; the built binaries always hold.
var (
	HoldValues = credkit.Hold
	HoldBody   = credkit.HoldBody
	HoldURL    = credkit.HoldURL
)

// HoldRunProfileStore holds what a run-profile store (profiles.json) carries as
// a source of contract section 1, every member of a duplicate key included: a
// value saved under a flag a run profile does not apply (one this CLI does not
// have, or a credential flag's name; allowed says which), and what a value saved
// under base-url carries, read by credkit.HoldURL (this CLI has no --base-url
// flag; a whole address held would withhold every output that shares its
// scheme and host, so only one that is no address with a host is held whole).
// A store that does not parse as the program reads it is held by
// credkit.HoldBody. Free text (a description, a URL in a --deliver value) is
// no source and is not read.
func HoldRunProfileStore(data []byte, allowed func(flag string) bool) {
	var store struct {
		Profiles map[string]struct {
			Values map[string]string `json:"values"`
		} `json:"profiles"`
	}
	if json.Unmarshal(data, &store) != nil {
		HoldBody(data)
		return
	}
	eachProfileValue(data, func(flag, value string) {
		switch {
		case value == "":
		case strings.Contains(strings.ReplaceAll(strings.ToLower(flag), "_", "-"), "base-url"):
			if u, err := url.Parse(strings.TrimSpace(value)); err != nil || u.Host == "" {
				HoldValues("minted:profile", value)
			} else {
				HoldURL("minted:profile", value)
			}
		case !allowed(flag):
			HoldValues("minted:profile", value)
		}
	})
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
