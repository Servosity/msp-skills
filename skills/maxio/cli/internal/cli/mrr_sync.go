package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/cliutil"
	"maxio-pp-cli/internal/revenue"
)

// pp:data-source live
//
// newNovelMrrSyncCmd populates the revenue snapshot/movement tables from the
// live Insights surfaces. Read-only against the API (GET only); the only writes
// are to the local SQLite store, so it is annotated mcp:read-only.
func newNovelMrrSyncCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var maxPages int
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Snapshot current MRR and backfill movement history into the local store",
		Long: strings_TrimLeadingNewline(`
Fetches the live Insights surfaces (site MRR, per-subscription MRR, and the MRR
movement log) and writes a timestamped snapshot plus an incremental movement
backfill. Run this alongside 'maxio-cli sync' so the revenue commands
(mrr waterfall/client, retention, cohort, triage, usage-drivers) have data to
compute on. Read-only against the API; writes only the local SQLite store.

The MRR movement endpoint is deprecated upstream — backfilling it locally is
exactly how this CLI keeps your movement history after Maxio sunsets it.`),
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli mrr sync\n  maxio-cli mrr sync --max-pages 0   # full movement backfill",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would snapshot MRR and backfill movements")
				return nil
			}
			if cliutil.IsDogfoodEnv() && (maxPages == 0 || maxPages > 1) {
				maxPages = 1
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			st, db, err := openRevenueStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			res, err := revenue.Sync(cmd.Context(), c, db, revenue.SyncOptions{
				MaxMovementPages: maxPages,
				SnapshotAt:       time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				return err
			}
			return emitRevenue(cmd, flags, res, func(w io.Writer) {
				fmt.Fprintf(w, "snapshot %s: MRR %s, %d active subs; %d per-subscription rows; movements +%d (fetched %d across %d page(s))\n",
					res.SnapshotAt, revenue.Cents(res.SiteMRRCents), res.ActiveSubs, res.SubSnapshotRows,
					res.MovementsInserted, res.MovementsFetched, res.MovementPagesRead)
			})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().IntVar(&maxPages, "max-pages", 50, "Max movement pages to backfill (200 movements/page; 0 = unlimited)")
	return cmd
}

// strings_TrimLeadingNewline trims a single leading newline so multi-line Long
// strings can start on their own line in source without a blank first line.
func strings_TrimLeadingNewline(s string) string {
	if len(s) > 0 && s[0] == '\n' {
		return s[1:]
	}
	return s
}
