// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"runzero-pp-cli/internal/cliutil"
	"runzero-pp-cli/internal/inventory"
)

// pp:data-source local
func newNovelExposureDeltaCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "exposure-delta",
		Short: "List services and ports that became newly exposed or newly vulnerable since the last sync, ranked by asset criticality.",
		Long: strings.Trim(`
Use this command for what newly became exposed or vulnerable since the last
sync, ranked by criticality. Do NOT use it for the whole-surface
appeared/disappeared inventory delta; use 'diff' instead. Do NOT use it for a
point-in-time exposure ranking; use 'triage' instead.

Joins the two most recent local sync snapshots to isolate the security-relevant
change: services that first appeared after the baseline and vulnerabilities
first detected after the baseline, each joined to its asset and ordered by
asset criticality. --since picks the baseline as the most recent sync at or
before now minus the given window (e.g. 7d, 24h, 2w).

Run 'inventory sync' at least twice for exposure-delta to have a baseline.`, "\n"),
		Example: strings.Trim(`
  # What newly became exposed or vulnerable since the previous sync
  runzero-cli exposure-delta --agent

  # Compare against the surface as of a week ago
  runzero-cli exposure-delta --since 7d --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			var since time.Duration
			if strings.TrimSpace(flagSince) != "" {
				d, err := cliutil.ParseDurationLoose(flagSince)
				if err != nil {
					return fmt.Errorf("invalid --since %q: %w (try 7d, 24h, 2w)", flagSince, err)
				}
				since = d
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			res, err := inventory.ExposureDelta(cmd.Context(), st.DB(), since)
			if err != nil {
				return err
			}
			if res.SyncRunCount < 2 {
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: exposure-delta needs at least two snapshots; run 'runzero-cli inventory sync' again to establish a baseline.")
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Baseline window for the comparison (e.g. 7d, 24h, 2w); default compares the two most recent syncs")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}
