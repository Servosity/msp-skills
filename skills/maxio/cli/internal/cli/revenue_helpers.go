package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math"

	"maxio-pp-cli/internal/revenue"
	"maxio-pp-cli/internal/store"
)

// round2 rounds a percentage or ratio to two decimal places. Revenue metrics
// are derived from integer-cent division, so raw float64 results otherwise
// surface 15-digit precision (87.62207034685262) that is noise to a reader.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// revenueCLIName is the local data dir / store name shared by every revenue
// command, matching defaultDBPath.
const revenueCLIName = "maxio-cli"

// openRevenueStore opens the local SQLite store and guarantees the revenue
// schema exists. Callers must Close the returned store.
func openRevenueStore(ctx context.Context, dbPath string) (*store.Store, *sql.DB, error) {
	if dbPath == "" {
		dbPath = defaultDBPath(revenueCLIName)
	}
	st, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening database: %w", err)
	}
	if err := revenue.EnsureSchema(st.DB()); err != nil {
		_ = st.Close()
		return nil, nil, err
	}
	return st, st.DB(), nil
}

// hintRunMrrSync prints a stderr hint telling the user to populate the revenue
// snapshot tables. Written to stderr so JSON stdout stays clean.
func hintRunMrrSync(w io.Writer) {
	fmt.Fprintln(w, "hint: no MRR snapshots yet — run `maxio-cli mrr sync` first (it backfills movement history and writes the first snapshot).")
}

// emitRevenue writes a Go value as filtered JSON (honoring --json/--select/etc.)
// when machine output is wanted, otherwise invokes the human renderer.
func emitRevenue(cmd interface{ OutOrStdout() io.Writer }, flags *rootFlags, v any, human func(io.Writer)) error {
	w := cmd.OutOrStdout()
	if wantsHumanTable(w, flags) && human != nil {
		human(w)
		return nil
	}
	return printJSONFiltered(w, v, flags)
}

// revenueMoney is the site's currency and a renderer for cents in it (Codex
// r1, maxio 4.34.2 reprint, class currency-dropped: every amount printed a
// dollar sign whatever the site's currency).
func revenueMoney(ctx context.Context, db *sql.DB) (string, func(int64) string) {
	cur := revenue.SiteCurrency(ctx, db)
	return cur, func(c int64) string { return revenue.Money(c, cur) }
}

// warnIfMovementsIncomplete tells a reader of the movement log that the
// stored history does not reach the log's first page (Codex r1 class
// incomplete-backfill-read-as-complete). Written to stderr so JSON stays clean.
func warnIfMovementsIncomplete(ctx context.Context, db *sql.DB, w io.Writer) {
	if !revenue.MovementsComplete(ctx, db) {
		fmt.Fprintln(w, "warning: the stored MRR movement history does not reach the start of Maxio's log (a capped or failed `mrr sync`, or one from an older version); totals from it may be incomplete. Run `maxio-cli mrr sync --max-pages 0`.")
	}
}
