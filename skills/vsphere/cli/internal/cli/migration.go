// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelMigrationCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "migration",
		Short:       "Migration planning: blockers, capacity totals and Proxmox VE mapping",
		Example:     "  vsphere-cli migration blockers --cluster domain-c8 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelMigrationBlockersCmd(flags))
	cmd.AddCommand(newNovelMigrationCapacityCmd(flags))
	cmd.AddCommand(newNovelMigrationProxmoxMapCmd(flags))
	return cmd
}
