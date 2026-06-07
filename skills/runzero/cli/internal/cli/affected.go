// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"runzero-pp-cli/internal/inventory"
)

// pp:data-source local
func newNovelAffectedCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "affected <cve>",
		Short: "Given a CVE, list every affected asset with its services and criticality.",
		Long: strings.Trim(`
Pivot from a single CVE to its blast radius: every locally-synced asset with a
matching vulnerability finding, joined to that asset's services and criticality.
Matching is a case-insensitive substring, so 'CVE-2024-3094' and '2024-3094'
both work. Reads the local store; run 'inventory sync' first.`, "\n"),
		Example: strings.Trim(`
  runzero-cli affected CVE-2024-3094 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			cve := strings.TrimSpace(args[0])
			if cve == "" {
				return fmt.Errorf("a CVE id is required, e.g. affected CVE-2024-3094")
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			rows, err := inventory.Affected(cmd.Context(), st.DB(), cve)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}
