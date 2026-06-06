// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// captureUsageSnapshot reads current usages rows and inserts a usage_snapshots
// row per (tenant, offering) for the given date. Returns the number of rows
// written.
func captureUsageSnapshot(db *store.Store, date string) (int, error) {
	rows, err := db.Query(`SELECT tenants_id, data FROM usages`)
	if err != nil {
		return 0, fmt.Errorf("querying usages: %w", err)
	}
	defer rows.Close()

	type rec struct {
		tenant, offering, edition string
		value                     *float64
	}
	var recs []rec
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
		name := usageNameOf(obj)
		if name == "" {
			continue
		}
		r := rec{tenant: tid, offering: name, edition: usageEditionOf(obj)}
		if v, ok := usageValueOf(obj); ok {
			vv := v
			r.value = &vv
		}
		recs = append(recs, r)
	}

	written := 0
	for _, r := range recs {
		var val any
		if r.value != nil {
			val = *r.value
		}
		_, err := db.DB().Exec(
			`INSERT OR REPLACE INTO usage_snapshots(snapshot_date, tenant_id, offering, value, edition) VALUES(?,?,?,?,?)`,
			date, r.tenant, r.offering, val, r.edition,
		)
		if err != nil {
			return written, fmt.Errorf("inserting snapshot: %w", err)
		}
		written++
	}
	return written, nil
}

// pp:data-source local
func newNovelUsagesSnapshotCmd(flags *rootFlags) *cobra.Command {
	var flagDate string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "snapshot",
		Short:       "Capture today's usage metrics into a point-in-time snapshot for later drift analysis.",
		Annotations: map[string]string{},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			date := flagDate
			if date == "" {
				date = time.Now().Format("2006-01-02")
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

			written, err := captureUsageSnapshot(db, date)
			if err != nil {
				return err
			}
			if wantJSON(flags, cmd) {
				return encodeJSON(cmd, flags, map[string]any{
					"snapshot_date": date,
					"rows_captured": written,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Captured %d usage rows for snapshot %s\n", written, date)
			return nil
		},
	}
	cmd.Flags().StringVar(&flagDate, "date", "", "Snapshot date (YYYY-MM-DD); defaults to today")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	return cmd
}

type driftRow struct {
	TenantID  string   `json:"tenant_id"`
	Offering  string   `json:"offering"`
	FromValue *float64 `json:"from_value"`
	ToValue   *float64 `json:"to_value"`
	Delta     *float64 `json:"delta"`
}

// distinctSnapshotDates returns the snapshot dates present, ascending.
func distinctSnapshotDates(db *store.Store) ([]string, error) {
	rows, err := db.Query(`SELECT DISTINCT snapshot_date FROM usage_snapshots ORDER BY snapshot_date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if rows.Scan(&d) == nil {
			out = append(out, d)
		}
	}
	return out, nil
}

// computeDrift diffs two snapshot dates per (tenant, offering).
func computeDrift(db *store.Store, from, to, tenantFilter string) ([]driftRow, error) {
	load := func(date string) (map[string]float64, map[string]bool, error) {
		vals := map[string]float64{}
		present := map[string]bool{}
		q := `SELECT tenant_id, offering, value FROM usage_snapshots WHERE snapshot_date = ?`
		args := []any{date}
		if tenantFilter != "" {
			q += ` AND tenant_id = ?`
			args = append(args, tenantFilter)
		}
		rows, err := db.Query(q, args...)
		if err != nil {
			return nil, nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var tid, off string
			var v *float64
			if rows.Scan(&tid, &off, &v) != nil {
				continue
			}
			k := tid + "\x00" + off
			present[k] = true
			if v != nil {
				vals[k] = *v
			}
		}
		return vals, present, nil
	}

	fromVals, fromPresent, err := load(from)
	if err != nil {
		return nil, err
	}
	toVals, toPresent, err := load(to)
	if err != nil {
		return nil, err
	}

	keys := map[string]bool{}
	for k := range fromPresent {
		keys[k] = true
	}
	for k := range toPresent {
		keys[k] = true
	}

	out := make([]driftRow, 0, len(keys))
	for k := range keys {
		tid, off := splitKey(k)
		row := driftRow{TenantID: tid, Offering: off}
		if fromPresent[k] {
			fv := fromVals[k]
			row.FromValue = &fv
		}
		if toPresent[k] {
			tv := toVals[k]
			row.ToValue = &tv
		}
		if row.FromValue != nil && row.ToValue != nil {
			d := *row.ToValue - *row.FromValue
			row.Delta = &d
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID != out[j].TenantID {
			return out[i].TenantID < out[j].TenantID
		}
		return out[i].Offering < out[j].Offering
	})
	return out, nil
}

func splitKey(k string) (string, string) {
	for i := 0; i < len(k); i++ {
		if k[i] == 0 {
			return k[:i], k[i+1:]
		}
	}
	return k, ""
}

// pp:data-source local
func newNovelUsagesDriftCmd(flags *rootFlags) *cobra.Command {
	var flagFrom string
	var flagTo string
	var flagTenant string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "drift",
		Short:       "Compare per-tenant, per-metric usage between two stored snapshots to see what grew or shrank.",
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

			dates, err := distinctSnapshotDates(db)
			if err != nil {
				return fmt.Errorf("reading snapshots: %w", err)
			}
			if len(dates) < 2 {
				msg := "need two snapshots; run 'acronis-cli usages snapshot' on two dates"
				if wantJSON(flags, cmd) {
					return encodeJSON(cmd, flags, map[string]any{
						"drift":           []driftRow{},
						"message":         msg,
						"snapshots_found": dates,
					})
				}
				fmt.Fprintln(cmd.OutOrStdout(), msg)
				return nil
			}

			from := flagFrom
			to := flagTo
			if from == "" {
				from = dates[0]
			}
			if to == "" {
				to = dates[len(dates)-1]
			}

			rowsOut, err := computeDrift(db, from, to, flagTenant)
			if err != nil {
				return fmt.Errorf("computing drift: %w", err)
			}

			if wantJSON(flags, cmd) {
				return encodeJSON(cmd, flags, map[string]any{
					"from":  from,
					"to":    to,
					"drift": rowsOut,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Drift %s -> %s:\n", from, to)
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %10s %10s %10s\n", "TENANT", "OFFERING", "FROM", "TO", "DELTA")
			for _, r := range rowsOut {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %10s %10s %10s\n",
					truncate(r.TenantID, 24), truncate(r.Offering, 28), fmtPtr(r.FromValue), fmtPtr(r.ToValue), fmtPtr(r.Delta))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagFrom, "from", "", "From snapshot date (YYYY-MM-DD); defaults to earliest")
	cmd.Flags().StringVar(&flagTo, "to", "", "To snapshot date (YYYY-MM-DD); defaults to latest")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Restrict to a single tenant id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	return cmd
}

func fmtPtr(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%g", *v)
}
