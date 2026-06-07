// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"runzero-pp-cli/internal/inventory"
)

// pp:data-source local
func newNovelSoftwareRollupCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "rollup [name]",
		Short: "Group installed software by product and version with a count of assets running each.",
		Long: strings.Trim(`
Group the locally-synced software inventory by product and version with a
distinct-asset count, most-deployed first — the rollup the live API never
returns. Pass a name to filter to a product or vendor (case-insensitive
substring); omit it to roll up everything. Reads the local store; run
'inventory sync' first.`, "\n"),
		Example: strings.Trim(`
  # How many distinct OpenSSL builds, and on how many assets each
  runzero-cli software rollup openssl --agent

  # Roll up the entire software inventory
  runzero-cli software rollup --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			name := ""
			if len(args) > 0 {
				name = strings.TrimSpace(args[0])
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			rows, err := inventory.SoftwareRollup(cmd.Context(), st.DB(), name)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}
