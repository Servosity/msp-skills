// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Command group for the hand-written QuoteWerks commands; the group itself only
// prints help. Children are attached with AddCommand (see novel_groups.go).

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelProductsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "products",
		Short:       "Search parts across every QuoteWerks product database",
		Example:     "  quotewerks-cli products search ThinkPad --limit 20 --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:parent-group": "true", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelProductsSearchCmd(flags))
	return cmd
}
