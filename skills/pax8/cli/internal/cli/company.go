// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-edited (Phase 3): real parent help for the customer-360 command group.

package cli

import (
	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelCompanyCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "company",
		Short:       "Customer-360 views joining a company to its subscriptions, contacts, invoices, and usage",
		Long:        "Customer-360 views for a single company. Use 'company show <companyId>' to join a company to its subscriptions, contacts, invoices, and usage from the local store in one view.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelCompanyShowCmd(flags))
	return cmd
}
