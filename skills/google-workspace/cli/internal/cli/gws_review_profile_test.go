// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/cliutil"
)

// A saved run profile never supplies --yes: 4.35.1 applies profile values
// with FlagSet.Set, which marks the flag Changed, so confirmed() alone would
// accept a profile-carried yes as typed.
func TestRunProfileNeverConfirms(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	flags := &rootFlags{}
	root.PersistentFlags().BoolVar(&flags.yes, "yes", false, "")
	child := &cobra.Command{Use: "child", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(child)
	root.SetArgs([]string{"child"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := ApplyProfileToFlags(child, &Profile{Values: map[string]string{"yes": "true"}}); err != nil {
		t.Fatal(err)
	}
	if flags.yes || confirmed(child, flags) {
		t.Fatal("a run profile must not confirm a destructive action")
	}
}

// profile save must not store --yes: apply ignores it (reserved), so a stored
// value would never take effect (handfixes.json profile-never-confirms).
func TestProfileSaveNeverStoresYes(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"profile", "save", "p", "--yes", "--compact"})
	if err := root.Execute(); err != nil {
		t.Fatalf("profile save: %v %s", err, out.String())
	}
	p, err := GetProfile("p")
	if err != nil || p == nil {
		t.Fatalf("profile not saved: %v", err)
	}
	if _, stored := p.Values["yes"]; stored {
		t.Fatalf("profile save stored --yes: %v", p.Values)
	}
}
