// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type reconcileRow struct {
	TenantID      string   `json:"tenant_id"`
	TenantName    string   `json:"tenant_name"`
	Offering      string   `json:"offering"`
	ApplicationID string   `json:"application_id,omitempty"`
	Edition       string   `json:"edition,omitempty"`
	InfraID       string   `json:"infra_id,omitempty"`
	UsageValue    *float64 `json:"usage_value"`
	HasOffering   bool     `json:"has_offering"`
	HasUsage      bool     `json:"has_usage"`
	Finding       string   `json:"finding"`
}

// computeReconcile joins usages against offering_items per tenant on the full
// usage identity (application, name, edition, infrastructure - the key the
// store keeps these rows by), flagging unlicensed usage and idle licenses. Only
// an offering item whose status proves it active is a license. A usage whose
// value is not reported is "usage_value_unknown", never a measured zero.
// tenantFilter (when non-empty) restricts to one tenant.
func computeReconcile(db *store.Store, tenantFilter string) ([]reconcileRow, error) {
	names, err := tenantNames(db)
	if err != nil {
		return nil, err
	}

	type key struct {
		tenant string
		id     offeringIdentity
	}
	type acc struct {
		hasOffering  bool
		hasUsage     bool
		value        float64
		valueUnknown bool
	}
	merged := map[key]*acc{}

	get := func(k key) *acc {
		a, ok := merged[k]
		if !ok {
			a = &acc{}
			merged[k] = a
		}
		return a
	}

	scan := func(table string, each func(tid string, obj map[string]any)) error {
		query := `SELECT tenants_id, data FROM ` + table
		var args []any
		if tenantFilter != "" {
			query += ` WHERE tenants_id = ?`
			args = append(args, tenantFilter)
		}
		rows, err := db.Query(query, args...)
		if err != nil {
			return fmt.Errorf("querying %s: %w", table, err)
		}
		defer rows.Close()
		for rows.Next() {
			var tid string
			var data []byte
			if err := rows.Scan(&tid, &data); err != nil {
				return fmt.Errorf("reading %s: %w", table, err)
			}
			obj, err := decodeRow(table, data)
			if err != nil {
				return err
			}
			each(tid, obj)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("reading %s: %w", table, err)
		}
		return nil
	}

	if err := scan("usages", func(tid string, obj map[string]any) {
		a := get(key{tid, offeringIdentityOf(obj)})
		a.hasUsage = true
		if v, ok := usageValueOf(obj); ok {
			a.value += v
		} else {
			a.valueUnknown = true
		}
	}); err != nil {
		return nil, err
	}
	if err := scan("offering_items", func(tid string, obj map[string]any) {
		if offeringStatusOf(obj) != "active" {
			return
		}
		get(key{tid, offeringIdentityOf(obj)}).hasOffering = true
	}); err != nil {
		return nil, err
	}

	out := make([]reconcileRow, 0, len(merged))
	for k, a := range merged {
		var value *float64
		if a.hasUsage && !a.valueUnknown {
			v := a.value
			value = &v
		}
		finding := "ok"
		switch {
		case a.hasUsage && !a.hasOffering:
			finding = "unlicensed_usage"
		case a.hasOffering && !a.hasUsage:
			finding = "idle_license"
		case value == nil:
			finding = "usage_value_unknown"
		case *value == 0:
			finding = "idle_license"
		}
		out = append(out, reconcileRow{
			TenantID:      k.tenant,
			TenantName:    names[k.tenant],
			Offering:      k.id.Name,
			ApplicationID: k.id.ApplicationID,
			Edition:       k.id.Edition,
			InfraID:       k.id.InfraID,
			UsageValue:    value,
			HasOffering:   a.hasOffering,
			HasUsage:      a.hasUsage,
			Finding:       finding,
		})
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
func newNovelReconcileUsagesCmd(flags *rootFlags) *cobra.Command {
	var flagTenant string
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:         "usages",
		Short:       "Flag usage with no matching offering item and offering items with zero usage, per tenant.",
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

			rowsOut, err := computeReconcile(db, flagTenant)
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
				fmt.Fprintln(cmd.OutOrStdout(), "No usage/offering data — run 'acronis-cli sync' first.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %-16s %12s %s\n", "TENANT", "OFFERING", "EDITION", "USAGE", "FINDING")
			for _, r := range rowsOut {
				label := r.TenantName
				if label == "" {
					label = r.TenantID
				}
				val := "-"
				if r.UsageValue != nil {
					val = fmt.Sprintf("%g", *r.UsageValue)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %-16s %12s %s\n", truncate(label, 24), truncate(r.Offering, 28), truncate(r.Edition, 16), val, r.Finding)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Restrict to a single tenant id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to show (0 = all)")
	return cmd
}
