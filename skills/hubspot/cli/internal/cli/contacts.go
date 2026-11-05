// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelContactsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "contacts",
		Short:       "Work with contacts",
		Example:     "  hubspot-cli contacts bulk-update --from-csv people.csv --map email=Email,lifecyclestage=Stage --dry-run",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelContactsBulkUpdateCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelContactsFunnelCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelContactsWinBackCmd(flags))
	return cmd
}
