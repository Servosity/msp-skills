// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"slices"
	"testing"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/credkit"
)

// recordHolds replaces the kit's hold for one test: the kit's held set is
// process-wide with no reset.
func recordHolds(t *testing.T) *[]string {
	t.Helper()
	var held []string
	prior := cliutil.Hold
	cliutil.Hold = func(_ credkit.Source, values ...string) { held = append(held, values...) }
	t.Cleanup(func() { cliutil.Hold = prior })
	return &held
}

// A fleet parent holds every core's key from PRTG_FLEET_KEYS, with or without a
// pasted "Bearer " prefix. (Variables an earlier build used: the legacy-field
// test, keys_test.go.)
func TestHoldCredentialsCoversEveryCoreKey(t *testing.T) {
	t.Setenv(DirEnv, t.TempDir())
	t.Setenv("PRTG_FLEET_KEYS", "holdunit=Bearer fleetHoldUnitKey-1a2b3c,\nother=fleetHoldOtherKey-4d5e6f")
	held := recordHolds(t)
	HoldCredentials()
	for _, v := range []string{"fleetHoldUnitKey-1a2b3c", "fleetHoldOtherKey-4d5e6f"} {
		if !slices.Contains(*held, v) {
			t.Errorf("%q not held: %q", v, *held)
		}
	}
	// A parsed value holds its keys, not the core names beside them.
	for _, v := range []string{"holdunit", "other"} {
		if slices.Contains(*held, v) {
			t.Errorf("the core name %q from a well-formed PRTG_FLEET_KEYS is held", v)
		}
	}
}

// A PRTG_FLEET_KEYS that does not parse still has every piece held: nothing
// in it can be told apart from a key.
func TestHoldCredentialsFailsClosedOnUnparseableKeys(t *testing.T) {
	t.Setenv(DirEnv, t.TempDir())
	t.Setenv("PRTG_FLEET_KEYS", "acme=fleetBadAcmeKey-2b4d6f\nfleetBareKey-8a1c3e\nglobex\tfleetTabKey-5e7a9c,=fleetNoNameKey-1f3b5d")
	if _, err := Keys(); err == nil {
		t.Fatal("the value under test must not parse")
	}
	held := recordHolds(t)
	HoldCredentials()
	for _, v := range []string{"fleetBadAcmeKey-2b4d6f", "fleetBareKey-8a1c3e", "fleetTabKey-5e7a9c", "fleetNoNameKey-1f3b5d"} {
		if !slices.Contains(*held, v) {
			t.Errorf("%q from an unparseable PRTG_FLEET_KEYS not held", v)
		}
	}
}
