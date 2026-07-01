package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"maxio-pp-cli/internal/revenue"
)

type usageDriver struct {
	Component       string `json:"component"`
	ExpansionCents  int64  `json:"expansion_cents"`
	ContractionCent int64  `json:"contraction_cents"`
	NetCents        int64  `json:"net_cents"`
	MovementCount   int    `json:"movement_count"`
}

type usageDriversView struct {
	Since   string        `json:"since,omitempty"`
	Drivers []usageDriver `json:"drivers"`
}

// lineItem mirrors one element of a movement's line_items JSON array. Each
// element carries a component/product name and its own mrr_movements breakdown.
// Amounts are signed INTEGER CENTS (e.g. -215 == -$2.15), same unit as the
// line item's mrr field (7955 == $79.55) — do not rescale.
type lineItem struct {
	Name         string  `json:"name"`
	MRR          float64 `json:"mrr"`
	MRRMovements []struct {
		Amount   float64 `json:"amount"`
		Category string  `json:"category"`
	} `json:"mrr_movements"`
}

// pp:data-source local
//
// newNovelUsageDriversCmd attributes expansion vs contraction MRR to the
// individual components/products that drove it, by unrolling each stored
// movement's line_items breakdown. It reads only the local movement log.
func newNovelUsageDriversCmd(flags *rootFlags) *cobra.Command {
	var flagSince, dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:         "usage-drivers",
		Short:       "Which metered/usage components drove expansion versus contraction MRR, ranked.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  maxio-cli usage-drivers --since 6m\n  maxio-cli usage-drivers --limit 30 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			cutoff, err := revenue.ParseSince(flagSince, time.Now())
			if err != nil {
				return usageErr(err)
			}
			st, db, err := openRevenueStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer st.Close()

			if has, _ := revenue.HasMovements(cmd.Context(), db); !has {
				hintRunMrrSync(cmd.ErrOrStderr())
				return emitRevenue(cmd, flags, usageDriversView{Since: cutoff, Drivers: []usageDriver{}}, func(w io.Writer) {
					fmt.Fprintln(w, "No MRR movements stored yet.")
				})
			}

			var rows *sql.Rows
			var qerr error
			if cutoff != "" {
				rows, qerr = db.QueryContext(cmd.Context(),
					`SELECT line_items FROM rev_mrr_movements WHERE ts >= ? AND line_items <> '' AND line_items <> '[]'`, cutoff)
			} else {
				rows, qerr = db.QueryContext(cmd.Context(),
					`SELECT line_items FROM rev_mrr_movements WHERE line_items <> '' AND line_items <> '[]'`)
			}
			if qerr != nil {
				return fmt.Errorf("querying movement line items: %w", qerr)
			}
			defer rows.Close()

			type agg struct {
				expansion, contraction int64
				count                  int
			}
			byName := map[string]*agg{}
			var order []string

			for rows.Next() {
				var raw sql.NullString
				if err := rows.Scan(&raw); err != nil {
					continue
				}
				if !raw.Valid || raw.String == "" {
					continue
				}
				var items []lineItem
				if json.Unmarshal([]byte(raw.String), &items) != nil {
					continue
				}
				for _, it := range items {
					name := strings.TrimSpace(it.Name)
					if name == "" {
						continue
					}
					a := byName[name]
					if a == nil {
						a = &agg{}
						byName[name] = a
						order = append(order, name)
					}
					hadMovement := false
					for _, mv := range it.MRRMovements {
						cents := int64(math.Round(mv.Amount))
						if cents == 0 {
							continue
						}
						hadMovement = true
						if cents > 0 {
							a.expansion += cents
						} else {
							a.contraction += cents
						}
					}
					if hadMovement {
						a.count++
					}
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("scanning movement line items: %w", err)
			}

			drivers := make([]usageDriver, 0, len(order))
			for _, name := range order {
				a := byName[name]
				net := a.expansion + a.contraction
				drivers = append(drivers, usageDriver{
					Component:       name,
					ExpansionCents:  a.expansion,
					ContractionCent: a.contraction,
					NetCents:        net,
					MovementCount:   a.count,
				})
			}
			sort.SliceStable(drivers, func(i, j int) bool {
				return absInt64(drivers[i].NetCents) > absInt64(drivers[j].NetCents)
			})
			if limit > 0 && len(drivers) > limit {
				drivers = drivers[:limit]
			}

			view := usageDriversView{Since: cutoff, Drivers: drivers}
			if view.Drivers == nil {
				view.Drivers = []usageDriver{}
			}

			return emitRevenue(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "%-34s %12s %12s %12s %6s\n", "component", "expansion", "contraction", "net", "moves")
				for _, d := range view.Drivers {
					fmt.Fprintf(w, "%-34s %12s %12s %12s %6d\n", truncate(d.Component, 34),
						revenue.Cents(d.ExpansionCents), revenue.Cents(d.ContractionCent),
						revenue.Cents(d.NetCents), d.MovementCount)
				}
			})
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only movements on/after this date (YYYY-MM-DD or window like 6m, 90d)")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max components to return (ranked by |net|)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
