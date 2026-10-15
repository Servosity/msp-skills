// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Command group for the hand-written QuoteWerks commands; the group itself only
// prints help. Children are attached with AddCommand (see novel_groups.go).

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelDocsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "docs",
		Short:       "Find, read, create and audit QuoteWerks quotes, orders and invoices",
		Example:     "  quotewerks-cli docs create --customer 'Acme Dental' --name 'Q4 laptop refresh' --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:parent-group": "true", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelDocsCreateCmd(flags))
	cmd.AddCommand(newNovelDocsGetCmd(flags))
	cmd.AddCommand(newNovelDocsLintCmd(flags))
	cmd.AddCommand(newNovelDocsSearchCmd(flags))
	return cmd
}
