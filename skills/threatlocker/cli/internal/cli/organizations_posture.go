// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatlocker-pp-cli/internal/store"
)

// tenantPosture is one managed tenant's card, computed from the local store.
type tenantPosture struct {
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
	Endpoints        int    `json:"endpoints"`
	Online           int    `json:"online"`
	Isolated         int    `json:"isolated"`
	PendingApprovals int    `json:"pendingApprovals"`
	OpenMaintenance  int    `json:"openMaintenance"`
	NewestAudit      string `json:"newestAudit"`
	AuditAgeDays     *int   `json:"auditAgeDays"`
	Attention        int    `json:"attention"`
}

type postureBuilder struct {
	cards map[string]*tenantPosture
}

func (b *postureBuilder) card(orgID, orgName string) *tenantPosture {
	c := b.cards[orgID]
	if c == nil {
		c = &tenantPosture{OrganizationID: orgID}
		b.cards[orgID] = c
	}
	if c.OrganizationName == "" && orgName != "" {
		c.OrganizationName = orgName
	}
	return c
}

// buildTenantPosture joins the synced organizations, computers, online-devices,
// approvals, maintenance and audit tables into one card per tenant. org limits
// the result to one tenant when non-empty.
func buildTenantPosture(ctx context.Context, db *store.Store, org string, now time.Time) ([]tenantPosture, error) {
	b := &postureBuilder{cards: map[string]*tenantPosture{}}
	keep := func(id string) bool { return id != "" && (org == "" || id == org) }

	each := func(query string, fn func(*sql.Rows) error) error {
		rows, err := db.DB().QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := fn(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}

	if err := each(`SELECT organization_id, name FROM organizations`, func(r *sql.Rows) error {
		var id, name *string
		if err := r.Scan(&id, &name); err != nil {
			return err
		}
		if keep(tlString(id)) {
			b.card(tlString(id), tlString(name))
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading organizations: %w", err)
	}

	if err := each(`SELECT c.organization_id, c.organization_name, c.is_isolated, c.is_online, od.computer_id
		FROM computers c `+tlFreshOnlineJoin, func(r *sql.Rows) error {
		var id, name, odMatch *string
		var isolated, online *int
		if err := r.Scan(&id, &name, &isolated, &online, &odMatch); err != nil {
			return err
		}
		if !keep(tlString(id)) {
			return nil
		}
		c := b.card(tlString(id), tlString(name))
		c.Endpoints++
		// The devices health classes: an isolated endpoint is isolated, not
		// online, so the two commands report the same numbers.
		switch {
		case isolated != nil && *isolated == 1:
			c.Isolated++
		case (online != nil && *online == 1) || odMatch != nil:
			c.Online++
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading computers: %w", err)
	}

	if err := each(`SELECT organization_id, organization_name FROM approvals WHERE status_id = 1`, func(r *sql.Rows) error {
		var id, name *string
		if err := r.Scan(&id, &name); err != nil {
			return err
		}
		if keep(tlString(id)) {
			b.card(tlString(id), tlString(name)).PendingApprovals++
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading approvals: %w", err)
	}

	scan, err := scanOpenMaintenance(ctx, db, org, now, 0)
	if err != nil {
		return nil, err
	}
	for _, w := range scan.Open {
		if keep(w.OrganizationID) {
			b.card(w.OrganizationID, w.OrganizationName).OpenMaintenance++
		}
	}

	// Newest audit row per tenant, compared as instants: raw timestamp strings
	// do not sort when the API mixes layouts.
	newest := map[string]time.Time{}
	newestRaw := map[string]string{}
	if err := each(`SELECT organization_id, date_created FROM audit`, func(r *sql.Rows) error {
		var id, created *string
		if err := r.Scan(&id, &created); err != nil {
			return err
		}
		orgID := tlString(id)
		if !keep(orgID) {
			return nil
		}
		t, ok := parseTLTime(tlString(created))
		if !ok {
			return nil
		}
		b.card(orgID, "")
		if t.After(newest[orgID]) {
			newest[orgID] = t
			newestRaw[orgID] = tlString(created)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("reading audit: %w", err)
	}

	out := make([]tenantPosture, 0, len(b.cards))
	for id, c := range b.cards {
		if t, ok := newest[id]; ok {
			c.NewestAudit = newestRaw[id]
			days := int(now.Sub(t) / (24 * time.Hour))
			c.AuditAgeDays = &days
		}
		// Endpoints - Online counts isolated, offline and stale endpoints once each.
		c.Attention = (c.Endpoints - c.Online) + c.PendingApprovals + c.OpenMaintenance
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Attention != out[j].Attention {
			return out[i].Attention > out[j].Attention
		}
		if out[i].OrganizationName != out[j].OrganizationName {
			return out[i].OrganizationName < out[j].OrganizationName
		}
		return out[i].OrganizationID < out[j].OrganizationID
	})
	return out, nil
}

func newNovelOrganizationsPostureCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "posture",
		Short: "One card per managed tenant: endpoints, online, isolated, pending approvals, open maintenance, audit age",
		Long: `Posture joins six locally synced tables into one row per managed tenant:
endpoint count, online and isolated endpoints (the devices health classes: an
isolated endpoint is not counted online), pending approval requests, open
maintenance-mode windows, and the newest audit row with its age in days.

Rows sort by attention, the sum of endpoints not online, pending approvals and
open maintenance windows, so the tenant that needs a look first comes first.
--org limits the card to one tenant.

Sync first: threatlocker-cli sync, then threatlocker-cli audit pull --since 30d
for the audit column.`,
		Example: strings.Trim(`
  threatlocker-cli organizations posture --agent
  threatlocker-cli organizations posture --select organizationName,attention,pendingApprovals,openMaintenance
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "organizations posture")
			}
			db, err := tlOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli sync' first.", err)
			}
			defer db.Close()
			cards, err := buildTenantPosture(cmd.Context(), db, tlNovelTenantScope(false), time.Now().UTC())
			if err != nil {
				return err
			}
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{"tenants": cards, "tenant_count": len(cards)})
			}
			if len(cards) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No synced tenants. Run: threatlocker-cli sync")
				return nil
			}
			headers := []string{"ORG", "ENDPOINTS", "ONLINE", "ISOLATED", "PENDING", "OPEN_MAINT", "AUDIT_AGE_DAYS", "ATTENTION"}
			tableRows := make([][]string, 0, len(cards))
			for _, c := range cards {
				org := c.OrganizationName
				if org == "" {
					org = c.OrganizationID
				}
				age := "-"
				if c.AuditAgeDays != nil {
					age = fmt.Sprintf("%d", *c.AuditAgeDays)
				}
				tableRows = append(tableRows, []string{org, fmt.Sprintf("%d", c.Endpoints), fmt.Sprintf("%d", c.Online),
					fmt.Sprintf("%d", c.Isolated), fmt.Sprintf("%d", c.PendingApprovals), fmt.Sprintf("%d", c.OpenMaintenance), age, fmt.Sprintf("%d", c.Attention)})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
