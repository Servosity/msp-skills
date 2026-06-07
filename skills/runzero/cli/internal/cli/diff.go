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
func newNovelDiffCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show which assets, services, ports, and software appeared or disappeared between two local syncs.",
		Long: strings.Trim(`
Compare two local sync snapshots and report what changed on your attack surface:
assets, services/ports, and software that appeared or disappeared. By default it
compares the two most recent syncs; --since picks the baseline as the most
recent sync at or before now minus the given window (e.g. 7d, 24h, 2w).

Run 'inventory sync' at least twice for diff to have two snapshots to compare.`, "\n"),
		Example: strings.Trim(`
  # Compare the two most recent syncs
  runzero-cli diff --agent

  # Compare the latest sync against the surface a week ago
  runzero-cli diff --since 7d`, "\n"),
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
			res, err := inventory.Diff(cmd.Context(), st.DB(), since)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Baseline window for the comparison (e.g. 7d, 24h, 2w); default compares the two most recent syncs")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}
