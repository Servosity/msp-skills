// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: reconcile-licenses.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type reconcileRow struct {
	TenantID           string `json:"tenant_id"`
	TenantName         string `json:"tenant_name"`
	OrgID              string `json:"org_id"`
	SubscriptionID     string `json:"subscription_id"`
	SubscriptionStatus string `json:"subscription_status"`
	LicensedResources  int64  `json:"licensed_resources"`
	ProtectedResources int64  `json:"protected_resources"`
	Delta              int64  `json:"delta"`
	Verdict            string `json:"verdict"` // over-provisioned | under-licensed | aligned
}

type reconcileView struct {
	Tenants []reconcileRow `json:"tenants"`
	Note    string         `json:"note,omitempty"`
}

func newNovelReconcileLicensesCmd(flags *rootFlags) *cobra.Command {
	var flagOrg string
	var flagDB string

	cmd := &cobra.Command{
		Use:   "reconcile-licenses",
		Short: "Compare purchased subscription quantities against actually-protected resource counts per tenant",
		Long: strings.TrimSpace(`
Join locally synced org subscriptions (licensed resource quantities) against
per-tenant protected-resource counts and flag over- and under-provisioned
tenants (run 'afi-cli fleet-sync' first). Only resource-kind subscription
items are compared; storage and other quota kinds appear in 'tenant-scorecard'.

Use this command to compare purchased subscription quantities against actually-protected resource counts.
Do NOT use this command for raw subscription history; use 'orgs licensing get-subscription-history' instead.`),
		Example: strings.Trim(`
  # Fleet-wide licensing drift
  afi-cli reconcile-licenses --json

  # One partner org
  afi-cli reconcile-licenses --org 01F0ORG0000000000000000000 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would join local subscriptions against protected-resource counts")
				return nil
			}
			if err := errNoLiveEquivalent(flags, "reconcile-licenses"); err != nil {
				return err
			}
			db, err := openAfiStore(flagDB)
			if err != nil {
				return err
			}
			defer db.Close()
			fleetHint(cmd, db, "subscriptions", flags)

			// Protected-resource counts per tenant.
			protected := map[string]int64{}
			prs, err := db.DB().QueryContext(cmd.Context(), `
				SELECT COALESCE(json_extract(data,'$.tenant_id'), ''),
				       COUNT(DISTINCT json_extract(data,'$.resource_id'))
				FROM resources WHERE resource_type = 'protections' GROUP BY 1`)
			if err != nil {
				return fmt.Errorf("counting protections: %w", err)
			}
			for prs.Next() {
				var tid string
				var n int64
				if err := prs.Scan(&tid, &n); err != nil {
					_ = prs.Close()
					return fmt.Errorf("scanning protection count: %w", err)
				}
				protected[tid] = n
			}
			_ = prs.Close()
			if err := prs.Err(); err != nil {
				return fmt.Errorf("iterating protection counts: %w", err)
			}

			// Tenant names.
			names := map[string]string{}
			trs, err := db.DB().QueryContext(cmd.Context(), `
				SELECT id, COALESCE(json_extract(data,'$.name'), '')
				FROM resources WHERE resource_type = 'tenants'`)
			if err != nil {
				return fmt.Errorf("querying tenants: %w", err)
			}
			for trs.Next() {
				var id, name string
				if err := trs.Scan(&id, &name); err != nil {
					_ = trs.Close()
					return fmt.Errorf("scanning tenant name: %w", err)
				}
				names[id] = name
			}
			_ = trs.Close()
			if err := trs.Err(); err != nil {
				return fmt.Errorf("iterating tenant names: %w", err)
			}

			// Subscriptions: sum resource-kind item quantities per tenant.
			where := "resource_type = 'subscriptions'"
			params := []any{}
			if flagOrg != "" {
				where += " AND json_extract(data,'$.org_id') = ?"
				params = append(params, flagOrg)
			}
			srs, err := db.DB().QueryContext(cmd.Context(), `
				SELECT id,
				       COALESCE(json_extract(data,'$.tenant_id'), ''),
				       COALESCE(json_extract(data,'$.org_id'), ''),
				       COALESCE(json_extract(data,'$.status'), ''),
				       data
				FROM resources WHERE `+where, params...)
			if err != nil {
				return fmt.Errorf("querying subscriptions: %w", err)
			}
			defer srs.Close()

			view := reconcileView{Tenants: make([]reconcileRow, 0)}
			subsSeen := 0
			for srs.Next() {
				var subID, tid, oid, status string
				var data []byte
				if err := srs.Scan(&subID, &tid, &oid, &status, &data); err != nil {
					return fmt.Errorf("scanning subscription: %w", err)
				}
				subsSeen++
				var obj struct {
					Items []map[string]any `json:"items"`
				}
				var licensed int64
				if err := json.Unmarshal(data, &obj); err == nil {
					for _, item := range obj.Items {
						kind, _ := item["kind"].(string)
						if kind != "resource" {
							continue
						}
						if qty, ok := jsonInt64(item["qty"]); ok {
							licensed += qty
						}
					}
				}
				row := reconcileRow{
					TenantID:           tid,
					TenantName:         names[tid],
					OrgID:              oid,
					SubscriptionID:     subID,
					SubscriptionStatus: status,
					LicensedResources:  licensed,
					ProtectedResources: protected[tid],
				}
				row.Delta = row.LicensedResources - row.ProtectedResources
				switch {
				case row.Delta > 0:
					row.Verdict = "over-provisioned"
				case row.Delta < 0:
					row.Verdict = "under-licensed"
				default:
					row.Verdict = "aligned"
				}
				view.Tenants = append(view.Tenants, row)
			}
			if err := srs.Err(); err != nil {
				return fmt.Errorf("iterating subscriptions: %w", err)
			}
			sort.Slice(view.Tenants, func(i, j int) bool {
				di, dj := view.Tenants[i].Delta, view.Tenants[j].Delta
				if di < 0 {
					di = -di
				}
				if dj < 0 {
					dj = -dj
				}
				if di != dj {
					return di > dj
				}
				return view.Tenants[i].TenantID < view.Tenants[j].TenantID
			})
			if subsSeen == 0 {
				view.Note = "no subscriptions in the local store; run 'afi-cli fleet-sync' to populate it"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagOrg, "org", "", "Restrict to subscriptions managed by one org ID")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
