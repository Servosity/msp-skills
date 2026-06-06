// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence feature: documentation-completeness scoring.
// Joins each asset's custom_fields against its layout's field schema in the
// local SQLite mirror — a populated-vs-required percentage no single Hudu API
// call returns.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
)

type completenessRow struct {
	CompanyID       int     `json:"company_id"`
	CompanyName     string  `json:"company_name"`
	LayoutID        int     `json:"layout_id,omitempty"`
	LayoutName      string  `json:"layout_name,omitempty"`
	Assets          int     `json:"assets_scored"`
	CompletenessPct float64 `json:"completeness_pct"`
	MissingFields   int     `json:"missing_fields"`
}

func newNovelAuditCompletenessCmd(flags *rootFlags) *cobra.Command {
	var flagCrossTenant bool
	var flagCompany int
	var flagLayout int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "completeness",
		Short:       "Score how completely each company's assets fill their layout's required fields, worst-first.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Score documentation completeness from the local mirror (run 'sync' first).

For every asset, the command joins its custom_fields against its asset layout's
field schema and computes the percentage of required fields that are populated
(falling back to all fields when a layout marks none required). Results are
aggregated per company and ranked worst-first. With --cross-tenant, results are
broken out per layout per company so you can spot the outlier tenant for a given
layout.`,
		Example: `  # Worst-documented clients
  hudu-cli audit completeness

  # One layout across every client
  hudu-cli audit completeness --cross-tenant --layout 7 --agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openAuditStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			schemas, err := loadLayoutSchemas(cmd.Context(), db)
			if err != nil {
				return err
			}

			q := `SELECT data FROM assets`
			var qargs []any
			if flagLayout > 0 {
				q += ` WHERE asset_layout_id = ?`
				qargs = append(qargs, flagLayout)
			}
			assetRows, err := queryDataRows(cmd.Context(), db, q, qargs...)
			if err != nil {
				return fmt.Errorf("reading assets: %w", err)
			}

			type agg struct {
				row    completenessRow
				pctSum float64
			}
			groups := map[string]*agg{}
			for _, raw := range assetRows {
				var m map[string]any
				if json.Unmarshal(raw, &m) != nil {
					continue
				}
				companyID := intField(m, "company_id")
				if flagCompany > 0 && companyID != flagCompany {
					continue
				}
				layoutID := intField(m, "asset_layout_id")
				sc, ok := schemas[layoutID]
				if !ok || len(sc.All) == 0 {
					continue // can't assess an asset whose layout schema we don't have
				}
				denom := sc.Required
				if len(denom) == 0 {
					denom = sc.All
				}
				cf, _ := json.Marshal(m["custom_fields"])
				filled := filledLabels(cf)
				have := 0
				for _, label := range denom {
					if filled[label] {
						have++
					}
				}
				missing := len(denom) - have
				pct := 100.0
				if len(denom) > 0 {
					pct = float64(have) / float64(len(denom)) * 100
				}

				key := strconv.Itoa(companyID)
				row := completenessRow{CompanyID: companyID, CompanyName: asString(m["company_name"])}
				if flagCrossTenant {
					key = key + "|" + strconv.Itoa(layoutID)
					row.LayoutID = layoutID
					row.LayoutName = sc.Name
				}
				g := groups[key]
				if g == nil {
					g = &agg{row: row}
					groups[key] = g
				}
				g.row.Assets++
				g.row.MissingFields += missing
				g.pctSum += pct
			}

			rows := make([]completenessRow, 0, len(groups))
			for _, g := range groups {
				if g.row.Assets > 0 {
					g.row.CompletenessPct = round1(g.pctSum / float64(g.row.Assets))
				}
				rows = append(rows, g.row)
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].CompletenessPct != rows[j].CompletenessPct {
					return rows[i].CompletenessPct < rows[j].CompletenessPct
				}
				return rows[i].MissingFields > rows[j].MissingFields
			})

			return emitAudit(cmd, flags, rows, func(w io.Writer) {
				if len(rows) == 0 {
					fmt.Fprintln(w, "No scored assets. Run 'hudu-cli sync' first (need assets + asset_layouts).")
					return
				}
				tw := newTabWriter(w)
				if flagCrossTenant {
					fmt.Fprintln(tw, "COMPLETE%\tCOMPANY\tLAYOUT\tASSETS\tMISSING")
					for _, r := range rows {
						fmt.Fprintf(tw, "%.1f\t%s\t%s\t%d\t%d\n", r.CompletenessPct, r.CompanyName, r.LayoutName, r.Assets, r.MissingFields)
					}
				} else {
					fmt.Fprintln(tw, "COMPLETE%\tCOMPANY\tASSETS\tMISSING")
					for _, r := range rows {
						fmt.Fprintf(tw, "%.1f\t%s\t%d\t%d\n", r.CompletenessPct, r.CompanyName, r.Assets, r.MissingFields)
					}
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().BoolVar(&flagCrossTenant, "cross-tenant", false, "Break results out per layout per company (spot the outlier tenant)")
	cmd.Flags().IntVar(&flagCompany, "company", 0, "Limit to a single company id")
	cmd.Flags().IntVar(&flagLayout, "layout", 0, "Limit to a single asset layout id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func intField(m map[string]any, key string) int {
	switch x := m[key].(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
