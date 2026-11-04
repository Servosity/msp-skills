// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: gaps.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type gapsRow struct {
	Tenant           string            `json:"tenant"`
	TenantID         string            `json:"tenant_id"`
	Type             string            `json:"type"`
	Gaps             []string          `json:"gaps"`
	LastScanComplete string            `json:"last_scan_completed"`
	LastScanByType   map[string]string `json:"last_scan_by_type"`
	ContactGroup     string            `json:"contact_group"`
	ContactMembers   int               `json:"contact_members"`
	PostureScore     *float64          `json:"posture_score"`
}

func newNovelGapsCmd(flags *rootFlags) *cobra.Command {
	var scanDays, limit int
	var tenant, dbPath string
	var all bool

	cmd := &cobra.Command{
		Use:   "gaps",
		Short: "Find tenants with no recent completed scan, no or empty escalation contact group, or no posture rating",
		Long: `Use this command to find tenants missing coverage (scans, escalation contacts, posture).
Do NOT use this command for individual devices that stopped checking in; use 'stale-agents' instead.

Joins the tenants, scans, contact groups and posture ratings fleet-sync stored.
Gap codes: no_recent_scan (no completed scan within --scan-days), no_contact_group,
empty_contact_group, no_posture_rating. A *_unknown code means that kind was
never synced for the tenant or its last fetch failed (for example 403 when the
account is not entitled), so absence cannot be concluded.`,
		Example: strings.Trim(`
  blackpoint-cli gaps --scan-days 30 --agent
  blackpoint-cli gaps --all --tenant "Acme Dental"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "gaps: join local tenants, scans, contact groups and posture")
			}
			if scanDays < 1 || limit < 0 {
				return usageErr(fmt.Errorf("--scan-days must be at least 1 and --limit zero or more"))
			}
			db, ok, err := openFleetForRead(cmd, flags, &dbPath)
			if err != nil {
				return err
			}
			if !ok {
				return emitRows(cmd, flags, []gapsRow{})
			}
			defer db.Close()
			// gaps reports *_unknown codes per tenant, so a failed kind only warns here.
			if err := checkFleetKind(cmd, flags, db.DB(), fleetKindScan); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", err)
			}
			if err := checkFleetKind(cmd, flags, db.DB(), fleetKindPosture); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", err)
			}
			if err := checkFleetKind(cmd, flags, db.DB(), fleetKindContactGroup); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", err)
			}
			rows, err := buildGapsRows(db.DB(), tenant, time.Now().UTC(), scanDays, all)
			if err != nil {
				return err
			}
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return emitRows(cmd, flags, rows, "tenant", "type", "gaps", "last_scan_completed", "contact_group", "contact_members", "posture_score")
		},
	}
	cmd.Flags().IntVar(&scanDays, "scan-days", 30, "A completed scan within this many days counts as coverage")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Limit to one tenant (name or id)")
	cmd.Flags().BoolVar(&all, "all", false, "List every tenant, including those with no gaps")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's local data.db)")
	return cmd
}

// fleetKindHealthy reports whether (kind, tenant) has a clean sync on record.
func fleetKindHealthy(states []fleetSyncState, kind, scope string) bool {
	for _, s := range states {
		if s.Kind == kind && s.Scope == scope {
			return s.Error == ""
		}
	}
	return false
}

func buildGapsRows(db *sql.DB, tenantQuery string, now time.Time, scanDays int, all bool) ([]gapsRow, error) {
	_, tenants, err := fleetTenantIndex(db)
	if err != nil {
		return nil, err
	}
	scopes, err := filterTenantScope(tenants, tenantQuery)
	if err != nil {
		return nil, err
	}
	states, err := loadFleetSyncState(db)
	if err != nil {
		return nil, err
	}
	scans, err := loadCleanFleetRows(db, fleetKindScan, fleetKindScan, scopes...)
	if err != nil {
		return nil, err
	}
	postures, err := loadCleanFleetRows(db, fleetKindPosture, fleetKindPosture, scopes...)
	if err != nil {
		return nil, err
	}
	contacts, err := loadFleetContacts(db, states)
	if err != nil {
		return nil, err
	}
	scansBy := map[string][]map[string]any{}
	for _, s := range scans {
		scansBy[s.Scope] = append(scansBy[s.Scope], s.Obj)
	}
	postureBy := map[string]map[string]any{}
	for _, p := range postures {
		postureBy[p.Scope] = p.Obj
	}
	want := map[string]bool{}
	for _, s := range scopes {
		want[s] = true
	}
	cutoff := now.Add(-time.Duration(scanDays) * 24 * time.Hour)
	rows := []gapsRow{}
	for _, t := range tenants {
		if len(want) > 0 && !want[t.ID] {
			continue
		}
		row := gapsRow{Tenant: t.Name, TenantID: t.ID, Type: t.Type, Gaps: []string{}, LastScanByType: map[string]string{}}

		var latest time.Time
		for _, sc := range scansBy[t.ID] {
			if !strings.EqualFold(fieldStr(sc, "status"), "completed") {
				continue
			}
			ts, ok := parseFleetTime(fieldStr(sc, "completedOn", "updatedOn", "updated", "createdOn", "created"))
			if !ok {
				continue
			}
			typ := fieldStr(sc, "type")
			if prev, ok := parseFleetTime(row.LastScanByType[typ]); !ok || ts.After(prev) {
				row.LastScanByType[typ] = ts.Format(time.RFC3339)
			}
			if ts.After(latest) {
				latest = ts
			}
		}
		if !latest.IsZero() {
			row.LastScanComplete = latest.Format(time.RFC3339)
		}
		switch {
		case !fleetKindHealthy(states, fleetKindScan, t.ID):
			row.Gaps = append(row.Gaps, "scans_unknown")
		case latest.IsZero() || latest.Before(cutoff):
			row.Gaps = append(row.Gaps, "no_recent_scan")
		}

		if t.ContactGroupID == "" {
			row.Gaps = append(row.Gaps, "no_contact_group")
		} else if g, found, healthy := contacts.lookup(t); !healthy || !found {
			row.Gaps = append(row.Gaps, "contacts_unknown")
		} else {
			row.ContactGroup = fieldStr(g, "name")
			n, known := contactMembers(g)
			row.ContactMembers = n
			switch {
			case !known:
				row.Gaps = append(row.Gaps, "contacts_unknown")
			case n == 0:
				row.Gaps = append(row.Gaps, "empty_contact_group")
			}
		}

		// A stored score is only current coverage when the latest posture
		// fetch for this tenant succeeded.
		if !fleetKindHealthy(states, fleetKindPosture, t.ID) {
			row.Gaps = append(row.Gaps, "posture_unknown")
		} else if p, ok := postureBy[t.ID]; ok {
			if s, ok := fieldNum(p, "score"); ok {
				row.PostureScore = &s
			}
		}
		if row.PostureScore == nil && fleetKindHealthy(states, fleetKindPosture, t.ID) {
			row.Gaps = append(row.Gaps, "no_posture_rating")
		}

		if all || len(row.Gaps) > 0 {
			rows = append(rows, row)
		}
	}
	return rows, nil
}
