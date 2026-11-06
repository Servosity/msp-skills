// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"jamf-pro-pp-cli/internal/cli"
)

// Every string flag named like a credential is declared to the kit, and every
// declared flag exists: a flag a reprint adds fails here until it is judged.
func TestDeclaredCredentialFlagsMatchTheCommands(t *testing.T) {
	looks := regexp.MustCompile(`(^|-)(password|passcode|secret|token|pin|api-key|access-key|private-key)(-|$)`)
	// Named like a credential, carries none: the variable holding a secret, an
	// endpoint, a validity flag, a file name, a request body's shape.
	notCredentials := map[string]bool{"--token-url": true}
	declared := KitDeclarations.CredentialFlags
	seen := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) {
			if f.Value.Type() != "string" {
				return
			}
			name := "--" + f.Name
			seen[name] = true
			if looks.MatchString(f.Name) && !notCredentials[name] && !slices.Contains(declared, name) &&
				!strings.HasSuffix(f.Name, "-name") && !strings.HasSuffix(f.Name, "-type") {
				t.Errorf("%s %s looks like a credential and is not in Declarations.CredentialFlags", c.CommandPath(), name)
			}
		}
		c.Flags().VisitAll(visit)
		c.PersistentFlags().VisitAll(visit)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())
	for _, f := range declared {
		if !seen[f] {
			t.Errorf("%s is declared but no command has it", f)
		}
	}
}
