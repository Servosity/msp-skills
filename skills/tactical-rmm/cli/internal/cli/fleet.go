// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"github.com/spf13/cobra"
)

func newNovelFleetCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "fleet",
		Short:       "Fleet-wide cross-entity views (health snapshot)",
		Long:        "Fleet-wide views that join agents, checks, alerts and inventory in the local store to answer questions no single API call can.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelFleetHealthCmd(flags))
	return cmd
}
