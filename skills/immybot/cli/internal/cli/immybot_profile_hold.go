// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring). The run profile
// store is a credential source, and not one of the kit's Files: it is nested JSON,
// which the kit holds string by string, and its own keys and every saved value
// would then withhold any output naming them.

package cli

import (
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/credkit"
	"immybot-pp-cli/internal/credwire"
)

// HoldProfileStores holds what each profile store this command line could mean
// carries, before any output, whether or not this run opens it. Both mains call it
// right after credkit.Start, which has held the credential-named values already
// (credwire.HoldProfileCredentials, from Files).
func HoldProfileStores(args []string) {
	credwire.ReadProfileStores(credwire.ProfileStorePaths(credkit.FlagValues(args, "--home")), holdProfileStore)
}

// holdProfileStore holds what credwire.HoldProfileCredentials holds, and every
// other saved value unless it is under a flag the allow-list names. A description
// and an allowed flag's value are not sources (contract section 1): nothing here
// reads credentials out of text. A store that does not parse is refused when read,
// printing none of it.
func holdProfileStore(data []byte) {
	credwire.HoldProfileCredentials(data)
	cliutil.JSONLeaves(data, func(keys []string, value string) {
		if len(keys) >= 4 && value != "" && strings.EqualFold(keys[0], "profiles") && strings.EqualFold(keys[2], "values") && !profileFlagAllowed(keys[3]) {
			cliutil.HoldValues("minted:profile", value)
		}
	})
}

// profileFlagAllowed is the allow-list: a flag this CLI has that
// ApplyProfileToFlags would apply.
func profileFlagAllowed(name string) bool {
	switch name {
	case "profile", "client-profile", "config", "home", "help", "stdin":
		return false
	}
	return !credentialLikeRunProfileFlag(name) && !credwire.CredentialFlag(name) && everyFlagName()[name]
}

var (
	flagNamesOnce sync.Once
	flagNames     map[string]bool
)

// everyFlagName walks the command tree once, and only when a store has a saved value.
func everyFlagName() map[string]bool {
	flagNamesOnce.Do(func() {
		flagNames = map[string]bool{}
		var walk func(c *cobra.Command)
		walk = func(c *cobra.Command) {
			mark := func(f *pflag.Flag) { flagNames[f.Name] = true }
			c.Flags().VisitAll(mark)
			c.PersistentFlags().VisitAll(mark)
			for _, sub := range c.Commands() {
				walk(sub)
			}
		}
		walk(RootCmd())
	})
	return flagNames
}
