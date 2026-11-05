// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix posture-report).
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"keeper-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

func newNovelPostureRunsCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List stored posture runs with their time, status and inactivity window",
		Long: `Lists the posture runs in the local store, newest first (no network call). Use the ids with
'posture diff --from <id> --to <id>'.`,
		Example: strings.Trim(`
  keeper-cli posture runs --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if limit < 1 || limit > 1000 {
				return usageErr(fmt.Errorf("--limit must be between 1 and 1000"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "posture runs")
			}
			runs := []store.PostureRun{}
			db, err := openStoreForRead(cmd.Context(), "keeper-cli")
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
				if runs, err = db.PostureRuns(cmd.Context(), limit); err != nil {
					return err
				}
			}
			out := cmd.OutOrStdout()
			if !wantsHumanTable(out, flags) {
				// --json, --agent, piped: always a JSON array, [] when empty.
				return flags.printJSON(cmd, runs)
			}
			if len(runs) == 0 {
				fmt.Fprintln(out, "No posture runs stored yet. Run 'keeper-cli posture report' first.")
				return nil
			}
			items := make([]map[string]any, 0, len(runs))
			for _, r := range runs {
				items = append(items, map[string]any{"id": r.ID, "started_at": r.StartedAt, "finished_at": r.FinishedAt,
					"status": r.Status, "inactive_days": r.InactiveDays})
			}
			return printAutoTable(out, items)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum runs (1 to 1000)")
	return cmd
}
