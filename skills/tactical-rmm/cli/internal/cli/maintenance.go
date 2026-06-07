// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live
package cli

import (
	"github.com/spf13/cobra"
)

func newNovelMaintenanceCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "maintenance",
		Short:       "Put filtered cohorts of agents into maintenance mode",
		Long:        "Cohort maintenance-mode controls. The 'set' subcommand toggles maintenance mode for a filtered cohort of agents via the live API.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelMaintenanceSetCmd(flags))
	return cmd
}
