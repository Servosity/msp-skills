// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestUserHelpCarriesNoGeneratorBrand guards help-text-no-generator-brand:
// respond's help told MSPs that "Under a Printing Press test harness --yes is
// refused", a test-only detail named after the generator. No visible
// command's help or flag text may name the generator.
func TestUserHelpCarriesNoGeneratorBrand(t *testing.T) {
	brand := regexp.MustCompile(`(?i)printing[ -]press`)
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}
		for field, text := range map[string]string{"Short": cmd.Short, "Long": cmd.Long, "Example": cmd.Example} {
			if m := brand.FindString(text); m != "" {
				t.Errorf("%s %s names the generator (%q)", cmd.CommandPath(), field, m)
			}
		}
		cmd.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
			if !f.Hidden && brand.MatchString(f.Usage) {
				t.Errorf("%s --%s usage names the generator: %q", cmd.CommandPath(), f.Name, f.Usage)
			}
		})
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(RootCmd())
}
