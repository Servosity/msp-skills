// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command group (keeper handfix novel-literal-registration: literal AddCommand).
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelAuditTrailCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "audit-trail",
		Short:       "Keeper audit trail: raw events and counts by type",
		Example:     "  keeper-cli audit-trail events --created last_7_days --limit 50 --agent",
		Annotations: map[string]string{"pp:parent-group": "true", "mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelAuditTrailEventsCmd(flags))
	cmd.AddCommand(newNovelAuditTrailSummaryCmd(flags))
	return cmd
}
