// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelReportCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:     "report",
		Short:   "Per-customer reports: a customer QBR one-pager and a fleet vulnerability rollup",
		Example: "  eset-protect-cli report customer 'Acme Dental' --since 90d --agent",
		// Help-only parent: pp:parent-group keeps it out of the MCP catalog,
		// where its subcommands are the callable tools.
		Annotations: map[string]string{"mcp:read-only": "true", "pp:parent-group": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelReportCustomerCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelReportVulnerabilitiesCmd(flags))
	return cmd
}
