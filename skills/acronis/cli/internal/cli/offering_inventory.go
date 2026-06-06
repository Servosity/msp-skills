// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type offeringInventoryRow struct {
	Offering string   `json:"offering"`
	Tenants  int      `json:"tenants"`
	Items    int      `json:"items"`
	Editions []string `json:"editions,omitempty"`
}

// computeOfferingInventory rolls up the synced offering_items across every
// tenant: per offering/SKU name, how many tenants hold it, how many item rows
// exist, and which editions appear.
func computeOfferingInventory(db *store.Store) ([]offeringInventoryRow, error) {
	rows, err := db.Query(`SELECT tenants_id, data FROM offering_items`)
	if err != nil {
		return nil, fmt.Errorf("querying offering_items: %w", err)
	}
	defer rows.Close()

	type acc struct {
		tenants  map[string]bool
		items    int
		editions map[string]bool
	}
	agg := map[string]*acc{}
	for rows.Next() {
		var tid string
		var data []byte
		if rows.Scan(&tid, &data) != nil {
			continue
		}
		obj := decodeData(data)
		if obj == nil {
			continue
		}
		name := offeringNameOf(obj)
		if name == "" {
			continue
		}
		a, ok := agg[name]
		if !ok {
			a = &acc{tenants: map[string]bool{}, editions: map[string]bool{}}
			agg[name] = a
		}
		a.tenants[tid] = true
		a.items++
		if ed, ok := obj["edition"].(string); ok && ed != "" {
			a.editions[ed] = true
		}
	}

	out := make([]offeringInventoryRow, 0, len(agg))
	for name, a := range agg {
		eds := make([]string, 0, len(a.editions))
		for e := range a.editions {
			eds = append(eds, e)
		}
		sort.Strings(eds)
		out = append(out, offeringInventoryRow{
			Offering: name,
			Tenants:  len(a.tenants),
			Items:    a.items,
			Editions: eds,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tenants != out[j].Tenants {
			return out[i].Tenants > out[j].Tenants
		}
		return out[i].Offering < out[j].Offering
	})
	return out, nil
}

// pp:data-source local
func newNovelOfferingInventoryCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Estate-wide rollup of which offering items / editions are enabled and how many tenants hold each.",
		Long: `Group the synced offering items across every tenant into one estate-wide
license inventory: per SKU/offering, the number of tenants holding it, total
item rows, and the editions seen. The API only lists offering items one
tenant at a time.

Use this for the estate-wide rollup of which SKUs/editions are enabled and
how many tenants hold each. Do NOT use it to match usage against SKUs for a
tenant; use 'reconcile usages'.

Reads the local store; run 'acronis-cli sync' first.`,
		Example: `  # Which SKUs are deployed estate-wide, most-held first
  acronis-cli tenants offering-items inventory

  # Agent JSON, top 10
  acronis-cli tenants offering-items inventory --limit 10 --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "", flags.maxAge)

			rowsOut, err := computeOfferingInventory(db)
			if err != nil {
				return err
			}
			if limit > 0 && len(rowsOut) > limit {
				rowsOut = rowsOut[:limit]
			}

			if wantJSON(flags, cmd) {
				return encodeJSON(cmd, flags, rowsOut)
			}
			if len(rowsOut) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No offering items synced — run 'acronis-cli sync' first.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-44s %8s %6s  %s\n", "OFFERING", "TENANTS", "ITEMS", "EDITIONS")
			for _, r := range rowsOut {
				fmt.Fprintf(cmd.OutOrStdout(), "%-44s %8d %6d  %s\n", truncate(r.Offering, 44), r.Tenants, r.Items, strings.Join(r.Editions, ","))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to show (0 = all)")
	return cmd
}
