// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-profiles-held): what the
// credential kit's startup discovery (internal/credwire) needs to know about
// run profiles, read through this package's own paths and flag rules.

package cli

import (
	"path/filepath"
	"slices"
	"strings"

	"crowdstrike-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

// RunProfileStorePaths is every profiles.json the program may read under the
// current --home: the config directory's, then the legacy one.
func RunProfileStorePaths() []string {
	var out []string
	if dir, err := cliutil.ConfigDir(); err == nil {
		out = append(out, filepath.Join(dir, "profiles.json"))
	}
	if legacy, err := legacyProfileStorePath(); err == nil && !slices.Contains(out, legacy) {
		out = append(out, legacy)
	}
	return out
}

// RunProfileFlagAllowed is the allow-list for a saved flag value: a flag this CLI
// has that a run profile would apply, never a credential flag's name (the
// credential-named ones, and --client-id, the public half of the API client
// pair, which credwire declares a credential flag).
func RunProfileFlagAllowed(name string) bool {
	if credentialLikeRunProfileFlag(name) || strings.EqualFold(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"), "client-id") {
		return false
	}
	var walk func(c *cobra.Command) bool
	walk = func(c *cobra.Command) bool {
		if c.Flags().Lookup(name) != nil || c.PersistentFlags().Lookup(name) != nil {
			return true
		}
		for _, sub := range c.Commands() {
			if walk(sub) {
				return true
			}
		}
		return false
	}
	return walk(RootCmd())
}
