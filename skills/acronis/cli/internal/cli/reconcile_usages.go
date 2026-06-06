// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type reconcileRow struct {
	TenantID    string   `json:"tenant_id"`
	TenantName  string   `json:"tenant_name"`
	Offering    string   `json:"offering"`
	UsageValue  *float64 `json:"usage_value"`
	HasOffering bool     `json:"has_offering"`
	HasUsage    bool     `json:"has_usage"`
	Finding     string   `json:"finding"`
}

// computeReconcile LEFT JOINs (conceptually, in Go) usages against
// offering_items per tenant on the offering name, flagging unlicensed usage
// and idle licenses. tenantFilter (when non-empty) restricts to one tenant.
func computeReconcile(db *store.Store, tenantFilter string) ([]reconcileRow, error) {
	names := tenantNames(db)

	type key struct{ tenant, offering string }
	type acc struct {
		hasOffering bool
		hasUsage    bool
		value       *float64
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

	// Usages side.
	uRows, err := db.Query(`SELECT tenants_id, data FROM usages`)
	if err != nil {
		return nil, fmt.Errorf("querying usages: %w", err)
	}
	for uRows.Next() {
		var tid string
		var data []byte
		if uRows.Scan(&tid, &data) != nil {
			continue
		}
		if tenantFilter != "" && tid != tenantFilter {
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
		a := get(key{tid, name})
		a.hasUsage = true
		if v, ok := usageValueOf(obj); ok {
			vv := v
			a.value = &vv
		}
	}
	uRows.Close()

	// Offering items side.
	oRows, err := db.Query(`SELECT tenants_id, data FROM offering_items`)
	if err != nil {
		return nil, fmt.Errorf("querying offering_items: %w", err)
	}
	for oRows.Next() {
		var tid string
		var data []byte
		if oRows.Scan(&tid, &data) != nil {
			continue
		}
		if tenantFilter != "" && tid != tenantFilter {
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
		get(key{tid, name}).hasOffering = true
	}
	oRows.Close()

	out := make([]reconcileRow, 0, len(merged))
	for k, a := range merged {
		finding := "ok"
		switch {
		case a.hasUsage && !a.hasOffering:
			finding = "unlicensed_usage"
		case a.hasOffering && !a.hasUsage:
			finding = "idle_license"
		case a.hasOffering && a.hasUsage && (a.value == nil || *a.value == 0):
			finding = "idle_license"
		}
		out = append(out, reconcileRow{
			TenantID:    k.tenant,
			TenantName:  names[k.tenant],
			Offering:    k.offering,
			UsageValue:  a.value,
			HasOffering: a.hasOffering,
			HasUsage:    a.hasUsage,
			Finding:     finding,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID != out[j].TenantID {
			return out[i].TenantID < out[j].TenantID
		}
		return out[i].Offering < out[j].Offering
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
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %12s %s\n", "TENANT", "OFFERING", "USAGE", "FINDING")
			for _, r := range rowsOut {
				label := r.TenantName
				if label == "" {
					label = r.TenantID
				}
				val := "-"
				if r.UsageValue != nil {
					val = fmt.Sprintf("%g", *r.UsageValue)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %12s %s\n", truncate(label, 24), truncate(r.Offering, 28), val, r.Finding)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Restrict to a single tenant id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to show (0 = all)")
	return cmd
}
