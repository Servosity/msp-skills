// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelDupesVendorsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:         "vendors",
		Short:       "Find duplicate vendors by normalized name or shared email",
		Example:     "  quickbooks-cli dupes vendors --agent\n  quickbooks-cli dupes vendors --json | jq '.[].members'",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        dupesRunE(flags, &dbPath, "vendors"),
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	return cmd
}
