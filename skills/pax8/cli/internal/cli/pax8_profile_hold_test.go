// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"pax8-pp-cli/internal/cliutil"
)

// The names a saved profile's values are held under, against the real flags: what
// run profiles refuse to apply is held, and no flag outside this list has its
// saved value held, or saving it would withhold every later output carrying it.
func TestProfileCredentialNamesAgainstEveryFlag(t *testing.T) {
	credentialFlags := map[string]bool{"client-id": true, "client-secret": true, "credential-ref": true, "secret-file": true, "token": true}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		check := func(f *pflag.Flag) {
			if credentialLikeRunProfileFlag(f.Name) && !cliutil.CredentialFieldName(f.Name) {
				t.Errorf("--%s: run profiles refuse it, but its saved value is not held", f.Name)
			}
			if cliutil.CredentialFieldName(f.Name) && !credentialFlags[f.Name] {
				t.Errorf("--%s (%s): a saved value would be held; a credential goes in this test's list, an ordinary flag needs a narrower rule", f.Name, c.CommandPath())
			}
		}
		c.Flags().VisitAll(check)
		c.PersistentFlags().VisitAll(check)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd())
	for _, name := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key", "client-secret", "API_KEY"} {
		if !credentialLikeRunProfileFlag(name) || !cliutil.CredentialFieldName(name) {
			t.Errorf("%q: credentialLikeRunProfileFlag %v, CredentialFieldName %v, want both", name, credentialLikeRunProfileFlag(name), cliutil.CredentialFieldName(name))
		}
	}
}
