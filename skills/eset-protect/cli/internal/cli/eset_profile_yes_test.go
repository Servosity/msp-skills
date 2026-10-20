// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestProfileNeverCarriesYes (handfixes.json: profile-never-carries-yes):
// `profile save routine --yes` followed by `respond isolate <uuid> --profile
// routine` sent StartNetworkIsolation with no --yes on that invocation.
// Save must drop "yes", and apply must refuse it from older profile files,
// while ordinary flags still round-trip.
func TestProfileNeverCarriesYes(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"profile", "save", "routine", "--yes", "--compact"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile save: %v\n%s", err, out.String())
	}
	p, err := GetProfile("routine")
	if err != nil || p == nil {
		t.Fatalf("GetProfile: %v %v", p, err)
	}
	if _, ok := p.Values["yes"]; ok {
		t.Fatalf("profile save stored yes: %v", p.Values)
	}
	if p.Values["compact"] != "true" {
		t.Fatalf("profile save lost an ordinary flag: %v", p.Values)
	}

	var confirm, compact bool
	target := &cobra.Command{Use: "x"}
	target.Flags().BoolVar(&confirm, "yes", false, "")
	target.Flags().BoolVar(&compact, "compact", false, "")
	if err := ApplyProfileToFlags(target, &Profile{Values: map[string]string{"yes": "true", "compact": "true"}}); err != nil {
		t.Fatal(err)
	}
	if confirm {
		t.Fatal("a profile value pre-confirmed --yes")
	}
	if !compact {
		t.Fatal("an ordinary profile value was not applied")
	}
}
