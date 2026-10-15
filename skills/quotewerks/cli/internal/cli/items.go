// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Command group for the hand-written QuoteWerks commands; the group itself only
// prints help. Children are attached with AddCommand (see novel_groups.go).

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelItemsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "items",
		Short:       "Find, add and reprice quote line items",
		Example:     "  quotewerks-cli items add 1042 --part 20UES0X100 --qty 12 --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:parent-group": "true", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelItemsAddCmd(flags))
	cmd.AddCommand(newNovelItemsRepriceCmd(flags))
	cmd.AddCommand(newItemsListCmd(flags))
	return cmd
}
