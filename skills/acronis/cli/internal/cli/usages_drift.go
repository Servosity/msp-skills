// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// captureUsageSnapshot replaces the snapshot for date with the current usages
// rows, one row per (tenant, full usage identity), and records the capture.
// The replacement is atomic: a metric absent from the current mirror is absent
// from the recaptured date, and a failure leaves the previous capture intact.
// Returns the number of rows written.
func captureUsageSnapshot(db *store.Store, date string) (int, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return 0, usageErr(fmt.Errorf("snapshot date must be YYYY-MM-DD, got %q", date))
	}
	rows, err := db.Query(`SELECT tenants_id, data FROM usages`)
	if err != nil {
		return 0, fmt.Errorf("querying usages: %w", err)
	}
	type key struct {
		tenant string
		id     offeringIdentity
	}
	type sample struct {
		value   float64
		unknown bool
	}
	samples := map[key]*sample{}
	var order []key
	for rows.Next() {
		var tid string
		var data []byte
		if err := rows.Scan(&tid, &data); err != nil {
			rows.Close()
			return 0, fmt.Errorf("reading usages: %w", err)
		}
		obj, err := decodeRow("usages", data)
		if err != nil {
			rows.Close()
			return 0, err
		}
		k := key{tid, offeringIdentityOf(obj)}
		s, ok := samples[k]
		if !ok {
			s = &sample{}
			samples[k] = s
			order = append(order, k)
		}
		if v, ok := usageValueOf(obj); ok {
			s.value += v
		} else {
			s.unknown = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, fmt.Errorf("reading usages: %w", err)
	}

	tx, err := db.DB().Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM usage_snapshot_rows WHERE snapshot_date = ?`, date); err != nil {
		return 0, fmt.Errorf("replacing snapshot: %w", err)
	}
	for _, k := range order {
		var val any
		if s := samples[k]; !s.unknown {
			val = s.value
		}
		if _, err := tx.Exec(
			`INSERT INTO usage_snapshot_rows(snapshot_date, tenant_id, application_id, offering, edition, infra_id, value) VALUES(?,?,?,?,?,?,?)`,
			date, k.tenant, k.id.ApplicationID, k.id.Name, k.id.Edition, k.id.InfraID, val,
		); err != nil {
			return 0, fmt.Errorf("inserting snapshot: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO usage_snapshot_captures(snapshot_date, rows, captured_at) VALUES(?,?,CURRENT_TIMESTAMP)`, date, len(order)); err != nil {
		return 0, fmt.Errorf("recording snapshot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing snapshot: %w", err)
	}
	return len(order), nil
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
	TenantID      string   `json:"tenant_id"`
	Offering      string   `json:"offering"`
	ApplicationID string   `json:"application_id,omitempty"`
	Edition       string   `json:"edition,omitempty"`
	InfraID       string   `json:"infra_id,omitempty"`
	FromPresent   bool     `json:"from_present"`
	ToPresent     bool     `json:"to_present"`
	FromValue     *float64 `json:"from_value"`
	ToValue       *float64 `json:"to_value"`
	Delta         *float64 `json:"delta"`
}

// distinctSnapshotDates returns the dates with a completed capture, ascending.
func distinctSnapshotDates(db *store.Store) ([]string, error) {
	rows, err := db.Query(`SELECT snapshot_date FROM usage_snapshot_captures ORDER BY snapshot_date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// computeDrift diffs two captured snapshot dates per (tenant, full usage
// identity). Both dates must have a completed capture. A value the API did not
// report stays unknown (null) and no delta is computed from it; from_present /
// to_present say whether the metric existed in each capture at all.
func computeDrift(db *store.Store, from, to, tenantFilter string) ([]driftRow, error) {
	for _, date := range []string{from, to} {
		var n int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM usage_snapshot_captures WHERE snapshot_date = ?`, date).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, fmt.Errorf("no usage snapshot was captured on %q; run 'acronis-cli usages snapshot --date %s' or pick a captured date", date, date)
		}
	}
	type key struct {
		tenant string
		id     offeringIdentity
	}
	load := func(date string) (map[key]*float64, error) {
		out := map[key]*float64{}
		q := `SELECT tenant_id, application_id, offering, edition, infra_id, value FROM usage_snapshot_rows WHERE snapshot_date = ?`
		args := []any{date}
		if tenantFilter != "" {
			q += ` AND tenant_id = ?`
			args = append(args, tenantFilter)
		}
		rows, err := db.Query(q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var k key
			var v *float64
			if err := rows.Scan(&k.tenant, &k.id.ApplicationID, &k.id.Name, &k.id.Edition, &k.id.InfraID, &v); err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, rows.Err()
	}

	fromVals, err := load(from)
	if err != nil {
		return nil, err
	}
	toVals, err := load(to)
	if err != nil {
		return nil, err
	}

	keys := map[key]bool{}
	for k := range fromVals {
		keys[k] = true
	}
	for k := range toVals {
		keys[k] = true
	}

	out := make([]driftRow, 0, len(keys))
	for k := range keys {
		row := driftRow{TenantID: k.tenant, Offering: k.id.Name, ApplicationID: k.id.ApplicationID, Edition: k.id.Edition, InfraID: k.id.InfraID}
		row.FromValue, row.FromPresent = fromVals[k]
		row.ToValue, row.ToPresent = toVals[k]
		if row.FromValue != nil && row.ToValue != nil {
			d := *row.ToValue - *row.FromValue
			row.Delta = &d
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.TenantID != b.TenantID {
			return a.TenantID < b.TenantID
		}
		if a.Offering != b.Offering {
			return a.Offering < b.Offering
		}
		if a.ApplicationID != b.ApplicationID {
			return a.ApplicationID < b.ApplicationID
		}
		if a.Edition != b.Edition {
			return a.Edition < b.Edition
		}
		return a.InfraID < b.InfraID
	})
	return out, nil
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
			// Fewer than two captures is not "no drift": it is no comparison.
			if len(dates) < 2 && (flagFrom == "" || flagTo == "") {
				return fmt.Errorf("need two usage snapshots to compare, found %d (%s); run 'acronis-cli usages snapshot' on two dates", len(dates), strings.Join(dates, ", "))
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
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %-16s %10s %10s %10s\n", "TENANT", "OFFERING", "EDITION", "FROM", "TO", "DELTA")
			for _, r := range rowsOut {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %-16s %10s %10s %10s\n",
					truncate(r.TenantID, 24), truncate(r.Offering, 28), truncate(r.Edition, 16), fmtDriftSide(r.FromPresent, r.FromValue), fmtDriftSide(r.ToPresent, r.ToValue), fmtPtr(r.Delta))
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

// fmtDriftSide renders one side of a drift row: "absent" when the metric was
// not in that capture, "unknown" when it was but had no value.
func fmtDriftSide(present bool, v *float64) string {
	if !present {
		return "absent"
	}
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("%g", *v)
}

func fmtPtr(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%g", *v)
}
