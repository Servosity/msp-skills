// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"runzero-pp-cli/internal/inventory"
)

// pp:data-source local
func newNovelStaleCmd(flags *rootFlags) *cobra.Command {
	var flagDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Bucket assets by last-seen age, end-of-life OS, and missing tags/owners for hygiene cleanup.",
		Long: strings.Trim(`
List assets that need hygiene attention: not seen in the last N days, running an
end-of-life OS, or missing tags/owners. Each row carries the reasons it was
flagged plus its age in days, oldest first. Emit IDs with
'--json --select asset_id' and pipe them into the bulk asset commands to retag
or retire. Reads the local store; run 'inventory sync' first.`, "\n"),
		Example: strings.Trim(`
  # Assets not seen in 30 days (default), plus EOL/untagged/unowned
  runzero-cli stale --agent

  # Stricter staleness window
  runzero-cli stale --days 90 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			rows, err := inventory.Stale(cmd.Context(), st.DB(), flagDays)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().IntVar(&flagDays, "days", 30, "Flag assets not seen in at least this many days")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}
