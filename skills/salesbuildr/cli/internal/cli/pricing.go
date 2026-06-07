// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

// newNovelPricingCmd is the group parent for the hand-built pricing analytics
// commands (pricing drift).
func newNovelPricingCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "pricing",
		Short:       "Pricing analytics across pricing books and the master catalog",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelPricingDriftCmd(flags))
	return cmd
}
