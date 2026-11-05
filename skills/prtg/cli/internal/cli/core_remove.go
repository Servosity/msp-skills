// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: core remove deletes a core and its local data.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/fleet"
)

func newCoreRemoveCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a core from the fleet and delete its local mirror and cache",
		Example: strings.Trim(`
  prtg-cli core remove acme`, "\n"),
		Annotations: map[string]string{"pp:data-source": "local", "mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "core remove")
			}
			if cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "core remove")
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one core name is required"))
			}
			name := strings.TrimSpace(args[0])
			found, err := fleet.RemoveCore(name)
			if err != nil {
				return configErr(err)
			}
			if !found {
				reg, _ := fleet.Load()
				names := []string{}
				if reg != nil {
					names = reg.Names()
				}
				return notFoundErr(fmt.Errorf("no core named %q (registered: %s)", name, strings.Join(names, ", ")))
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"removed": name, "purged_local_data": true}, flags)
		},
	}
	return cmd
}
