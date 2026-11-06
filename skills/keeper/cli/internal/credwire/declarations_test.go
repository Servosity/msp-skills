// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"keeper-pp-cli/internal/cli"
)

func TestSetTokenArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		rest []string
		ok   bool
	}{
		{[]string{"auth", "set-token"}, []string{}, true},
		{[]string{"--json", "auth", "set-token", "k1"}, []string{"k1"}, true},
		{[]string{"search", "set-token"}, nil, false},
		{[]string{"set-token", "auth"}, nil, false},
		{[]string{"companies", "list"}, nil, false},
	} {
		rest, ok := setTokenArguments(tc.args)
		if ok != tc.ok || !slices.Equal(rest, tc.rest) {
			t.Errorf("%q: got %q %v", tc.args, rest, ok)
		}
	}
}

func TestSetTokenFlagsMatchTheCommand(t *testing.T) {
	cmd, _, err := cli.RootCmd().Find([]string{"auth", "set-token"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	cmd.InitDefaultHelpFlag()
	cmd.Flags().AddFlagSet(cmd.InheritedFlags())
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		got[f.Name] = f.NoOptDefVal == ""
		if f.Shorthand != "" {
			got[f.Shorthand] = f.NoOptDefVal == ""
		}
	})
	if !maps.Equal(got, setTokenFlags) {
		var lines []string
		for name, takes := range got {
			lines = append(lines, fmt.Sprintf("\t%q: %v,", name, takes))
		}
		slices.Sort(lines)
		t.Errorf("setTokenFlags is out of date; the command declares:\n%s", strings.Join(lines, "\n"))
	}
}
