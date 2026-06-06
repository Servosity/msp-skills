// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelDupesCustomersCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:         "customers",
		Short:       "Find duplicate customers by normalized name or shared email",
		Example:     "  quickbooks-cli dupes customers --agent\n  quickbooks-cli dupes customers --json | jq '.[].members'",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        dupesRunE(flags, &dbPath, "customers"),
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	return cmd
}
