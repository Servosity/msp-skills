// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelCompanyCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "company",
		Short:       "Work with company",
		Example:     "  pax8-cli company show '<companyId>' --agent --select subscriptions.productName,subscriptions.quantity",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelCompanyShowCmd(flags)) // pax8 handfix: dogfood path walk follows AddCommand only
	return cmd
}
