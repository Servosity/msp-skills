// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live
package cli

import (
	"github.com/spf13/cobra"
)

func newNovelActionsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "actions",
		Short:       "Cross-fleet agent pending-action views",
		Long:        "Cross-fleet views of queued agent actions. The 'pending' subcommand fans out live to surface stuck dispatches grouped by agent and age.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelActionsPendingCmd(flags))
	return cmd
}
