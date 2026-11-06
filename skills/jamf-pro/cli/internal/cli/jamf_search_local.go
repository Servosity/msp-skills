// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: keep the framework search on the local store.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Jamf Pro has no global search endpoint. The generator wires any path that
// contains "search" as the live search, which here is the Classic
// advancedcomputersearches list: a live 'search mac-01' would return saved
// search definitions, not devices. Search therefore always reads the synced
// local store; live device lookups are 'fleet find' or a list --filter.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		search, _, err := root.Find([]string{"search"})
		if err != nil || search == nil || search.Name() != "search" || search.RunE == nil {
			return
		}
		next := search.RunE
		search.RunE = func(cmd *cobra.Command, args []string) error {
			if flags.dataSource == "live" {
				return usageErr(fmt.Errorf("search reads the local store (Jamf Pro has no global search endpoint): run 'sync' first, or use 'fleet find' or 'computer-inventory list --filter' for a live lookup"))
			}
			flags.dataSource = "local"
			return next(cmd, args)
		}
	})
}
