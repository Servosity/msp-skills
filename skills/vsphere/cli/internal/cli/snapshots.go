// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelSnapshotsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "snapshots",
		Short:       "Snapshot hygiene across the vCenter",
		Example:     "  vsphere-cli snapshots audit --older-than 7d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelSnapshotsAuditCmd(flags))
	return cmd
}
