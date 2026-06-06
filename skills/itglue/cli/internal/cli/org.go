// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature parent. See internal/cli/itglue_records.go for shared helpers.

package cli

import (
	"github.com/spf13/cobra"
)

// newNovelOrgCmd groups organization-level offline views built from the local
// store. Distinct from `organizations` (the live API resource commands): `org`
// children read the synced SQLite mirror and make zero API calls.
func newNovelOrgCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Organization-level offline views from the local store",
		Long: `Organization-level offline views assembled from the local SQLite mirror.

Use 'org show <id>' to pull the full local picture for one client. For live API
reads and writes on organization records, use 'organizations' instead.`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelOrgShowCmd(flags))
	return cmd
}
