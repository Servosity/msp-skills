// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type tenantAuditRow struct {
	TenantID      string   `json:"tenant_id"`
	Name          string   `json:"name,omitempty"`
	Kind          string   `json:"kind,omitempty"`
	Users         int      `json:"users"`
	OfferingItems int      `json:"offering_items"`
	Agents        int      `json:"agents"`
	OAuthClients  int      `json:"oauth_clients"`
	Missing       []string `json:"missing"`
}

// computeTenantAudit flags enabled customer tenants that deviate from the
// standard provisioning shape: missing users, offering items, agents, or
// OAuth clients. includeComplete reports conforming tenants too; kindFilter
// restricts the audited tenant kind (default customer).
func computeTenantAudit(db *store.Store, kindFilter string, includeComplete bool) ([]tenantAuditRow, error) {
	countBy := func(query string) map[string]int {
		m := map[string]int{}
		rows, err := db.Query(query)
		if err != nil {
			return m
		}
		defer rows.Close()
		for rows.Next() {
			var tid *string
			var n int
			if rows.Scan(&tid, &n) == nil {
				m[deref(tid)] = n
			}
		}
		return m
	}
	users := countBy(`SELECT tenants_id, COUNT(*) FROM users GROUP BY tenants_id`)
	offerings := countBy(`SELECT tenants_id, COUNT(*) FROM offering_items GROUP BY tenants_id`)
	agents := countBy(`SELECT tenant_id, COUNT(*) FROM agent_manager GROUP BY tenant_id`)
	clients := countBy(`SELECT tenant_id, COUNT(*) FROM clients GROUP BY tenant_id`)

	rows, err := db.Query(`SELECT id, name, kind, enabled FROM tenants`)
	if err != nil {
		return nil, fmt.Errorf("querying tenants: %w", err)
	}
	defer rows.Close()

	out := []tenantAuditRow{}
	for rows.Next() {
		var id string
		var name, kind *string
		var enabled *int
		if rows.Scan(&id, &name, &kind, &enabled) != nil {
			continue
		}
		if enabled != nil && *enabled == 0 {
			continue // disabled tenants are expected to be empty
		}
		k := strings.ToLower(deref(kind))
		if kindFilter != "" && k != strings.ToLower(kindFilter) {
			continue
		}
		r := tenantAuditRow{
			TenantID:      id,
			Name:          deref(name),
			Kind:          deref(kind),
			Users:         users[id],
			OfferingItems: offerings[id],
			Agents:        agents[id],
			OAuthClients:  clients[id],
			Missing:       []string{},
		}
		if r.Users == 0 {
			r.Missing = append(r.Missing, "users")
		}
		if r.OfferingItems == 0 {
			r.Missing = append(r.Missing, "offering_items")
		}
		if r.Agents == 0 {
			r.Missing = append(r.Missing, "agents")
		}
		if r.OAuthClients == 0 {
			r.Missing = append(r.Missing, "oauth_clients")
		}
		if len(r.Missing) == 0 && !includeComplete {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Missing) != len(out[j].Missing) {
			return len(out[i].Missing) > len(out[j].Missing)
		}
		return out[i].TenantID < out[j].TenantID
	})
	return out, nil
}

// pp:data-source local
func newNovelTenantsAuditCmd(flags *rootFlags) *cobra.Command {
	var dbPath, kindFilter string
	var includeComplete bool
	var limit int

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Flag customer tenants that deviate from the standard provisioning shape (no users, licenses, agents, or API clients).",
		Long: `Audit enabled tenants against the standard provisioning shape: at least
one user, one offering item, one registered agent, and one OAuth API client.
Tenants missing any component are flagged with the exact gap list — the
onboarding drift no single Acronis API call reports.

Reads the local store; run 'acronis-cli sync' first.`,
		Example: `  # Customer tenants with provisioning gaps, worst first
  acronis-cli tenants audit

  # Audit every kind, include conforming tenants, agent JSON
  acronis-cli tenants audit --kind "" --include-complete --agent`,
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

			rowsOut, err := computeTenantAudit(db, kindFilter, includeComplete)
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
				fmt.Fprintln(cmd.OutOrStdout(), "No provisioning gaps found.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24s %-10s %5s %5s %6s %7s  %s\n", "TENANT_ID", "NAME", "KIND", "USERS", "OFFER", "AGENTS", "CLIENTS", "MISSING")
			for _, r := range rowsOut {
				fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-24s %-10s %5d %5d %6d %7d  %s\n",
					r.TenantID, truncate(r.Name, 24), r.Kind, r.Users, r.OfferingItems, r.Agents, r.OAuthClients, strings.Join(r.Missing, ","))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().StringVar(&kindFilter, "kind", "customer", "Tenant kind to audit (empty = all kinds)")
	cmd.Flags().BoolVar(&includeComplete, "include-complete", false, "Also list tenants with no gaps")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to show (0 = all)")
	return cmd
}
