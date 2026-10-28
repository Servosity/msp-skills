// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"os"
	"slices"
	"strings"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/credkit"
)

// keySource is the kit source of each key PRTG_FLEET_KEYS carries. The whole
// value is exempt from holding (env:PRTG_FLEET_KEYS is a declared Identifier): it
// names each core beside its key, and holding it would withhold any output
// naming a core.
const keySource = "minted:fleet-key"

// HoldCredentials holds what no declaration can name, before anything prints
// (handfixes.json credkit-wiring): every key in PRTG_FLEET_KEYS, and, as a
// defense, any PRTG_KEY_* variable left over from an earlier build. A value that
// does not parse is held piece by piece (keyFragments), so a malformed variable
// fails closed. A fleet parent never sends these itself, but its children's
// output passes through it.
func HoldCredentials() {
	var vals []string
	for _, kv := range os.Environ() {
		if k, v, _ := strings.Cut(kv, "="); strings.HasPrefix(strings.ToUpper(k), "PRTG_KEY_") {
			vals = append(vals, cliutil.EffectiveEnv(v))
		}
	}
	raw := rawKeys()
	if keys, err := ParseKeys(raw); err == nil {
		for _, k := range keys {
			vals = append(vals, k)
		}
	} else {
		// A registered core's name is known not to be a key, and output names it.
		var names []string
		if reg, err := Load(); err == nil {
			names = reg.Names()
		}
		for _, f := range keyFragments(raw) {
			if !slices.Contains(names, f) {
				vals = append(vals, f)
			}
		}
	}
	HoldKeys(keySource, vals...)
}

// HoldKeys holds each key as given and without a pasted "Bearer " prefix.
func HoldKeys(source credkit.Source, keys ...string) {
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		cliutil.Hold(source, k)
		if len(k) > 7 && strings.EqualFold(k[:7], "bearer ") {
			cliutil.Hold(source, strings.TrimSpace(k[7:]))
		}
	}
}
