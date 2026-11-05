// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"slices"
	"testing"

	"apple-business-pp-cli/internal/cliutil"
)

// The kit's own read of a config file and the per-read hold (cliutil.HoldConfigFile)
// must agree on what is not a credential: a key one side treats as a credential and
// the other as a name is held in one reading and printed in the other.
func TestIdentifiersMatchTheHold(t *testing.T) {
	var declared []string
	for _, id := range Declarations.Identifiers {
		if id.Reason == "" {
			t.Errorf("%s has no reason", id.Source)
		}
		if len(id.Source) > 7 && id.Source[:7] == "config:" {
			declared = append(declared, id.Source[7:])
		}
	}
	want := append(slices.Clone(cliutil.NotCredentials), cliutil.PrivateKeyFields...)
	slices.Sort(want)
	slices.Sort(declared)
	if !slices.Equal(want, declared) {
		t.Errorf("config identifiers differ from the hold's:\n declared %v\n hold     %v", declared, want)
	}
}
