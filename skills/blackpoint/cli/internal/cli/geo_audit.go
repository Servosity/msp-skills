// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: geo-audit.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type geoAuditRow struct {
	Tenant     string `json:"tenant"`
	TenantID   string `json:"tenant_id"`
	Provider   string `json:"provider"`
	Connection string `json:"connection"`
	User       string `json:"user"`
	Country    string `json:"country"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	Status     string `json:"status"`
	Finding    string `json:"finding"`
}

func newNovelGeoAuditCmd(flags *rootFlags) *cobra.Command {
	var expiring, tenant, dbPath string
	var maxCountries int
	var all bool

	cmd := &cobra.Command{
		Use:   "geo-audit",
		Short: "Audit Cloud Response approved countries; flag expired or expiring travel exceptions and broad allow-lists",
		Long: `Joins the Cloud Response approved countries fleet-sync stored: connection-level
allow-lists (M365, Cisco Duo, Google) and per-user travel exceptions with start
and end dates. By default it lists only findings: expired exceptions still on
record, exceptions ending within --expiring, and connections whose allow-list
holds more than --max-countries countries. --all lists every exception.`,
		Example: strings.Trim(`
  blackpoint-cli geo-audit --expiring 7d --agent
  blackpoint-cli geo-audit --max-countries 5 --tenant "Acme Dental"
  blackpoint-cli geo-audit --all`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "geo-audit: audit local Cloud Response approved countries")
			}
			window, err := parseFleetDuration(expiring)
			if err != nil {
				return usageErr(fmt.Errorf("--expiring: %w", err))
			}
			if maxCountries < 1 {
				return usageErr(fmt.Errorf("--max-countries must be at least 1"))
			}
			db, ok, err := openFleetForRead(cmd, flags, &dbPath)
			if err != nil {
				return err
			}
			if !ok {
				return emitRows(cmd, flags, []geoAuditRow{})
			}
			defer db.Close()
			_, tenants, err := fleetTenantIndex(db.DB())
			if err != nil {
				return err
			}
			scopes, err := filterTenantScope(tenants, tenant)
			if err != nil {
				return err
			}
			if err := checkFleetKind(cmd, flags, db.DB(), fleetKindCloudStatus, scopes...); err != nil {
				return err
			}
			rows, err := buildGeoAuditRows(db.DB(), tenant, time.Now().UTC(), window, maxCountries, all)
			if err != nil {
				return err
			}
			return emitRows(cmd, flags, rows, "tenant", "provider", "user", "country", "end_date", "status", "finding")
		},
	}
	cmd.Flags().StringVar(&expiring, "expiring", "7d", "Flag user exceptions ending within this window (e.g. 7d, 48h)")
	cmd.Flags().IntVar(&maxCountries, "max-countries", 10, "Flag connection allow-lists with more countries than this")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Limit to one tenant (name or id)")
	cmd.Flags().BoolVar(&all, "all", false, "List every exception and allow-list entry, not only findings")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's local data.db)")
	return cmd
}

// classifyException returns status and finding for one user-level exception.
func classifyException(start, end string, now time.Time, window time.Duration) (string, string) {
	if s, ok := parseFleetTime(start); ok && s.After(now) {
		return "upcoming", "starts in " + humanAge(s.Sub(now))
	}
	e, ok := parseFleetTime(end)
	switch {
	case !ok:
		return "open-ended", "exception has no end date"
	case e.Before(now):
		return "expired", "ended " + humanAge(now.Sub(e)) + " ago but is still on record"
	case e.Sub(now) <= window:
		return "expiring", "ends in " + humanAge(e.Sub(now))
	}
	return "active", "ends in " + humanAge(e.Sub(now))
}

func countryLabel(m map[string]any) string {
	code := firstNonEmpty(nestedStr(m, "isoCountry", "code"), fieldStr(m, "isoCountryCode", "code"))
	if name := firstNonEmpty(nestedStr(m, "isoCountry", "name"), fieldStr(m, "name")); name != "" && name != code {
		return code + " " + name
	}
	return code
}

func buildGeoAuditRows(db *sql.DB, tenantQuery string, now time.Time, window time.Duration, maxCountries int, all bool) ([]geoAuditRow, error) {
	idx, tenants, err := fleetTenantIndex(db)
	if err != nil {
		return nil, err
	}
	scopes, err := filterTenantScope(tenants, tenantQuery)
	if err != nil {
		return nil, err
	}
	load := func(kind string) ([]fleetRecord, error) {
		return loadCleanFleetRows(db, kind, fleetKindCloudStatus, scopes...)
	}
	m365Conns, err := load(fleetKindM365Conn)
	if err != nil {
		return nil, err
	}
	m365Users, err := load(fleetKindM365User)
	if err != nil {
		return nil, err
	}
	connCountries, err := load(fleetKindConnCountry)
	if err != nil {
		return nil, err
	}
	connUsers, err := load(fleetKindConnUser)
	if err != nil {
		return nil, err
	}

	rows := []geoAuditRow{}
	keep := func(r geoAuditRow) {
		if all || r.Status == "expired" || r.Status == "expiring" || r.Status == "broad" {
			rows = append(rows, r)
		}
	}
	// Connection-level allow-lists.
	allow := map[string][]string{} // tenant|provider|connection -> countries
	for _, c := range m365Conns {
		key := c.Scope + "|m365|" + firstNonEmpty(fieldStr(c.Obj, "primaryDomain"), c.ID)
		if _, seen := allow[key]; !seen {
			allow[key] = nil // register the connection even when its list is empty
		}
		for _, ctry := range fieldList(c.Obj, "authorizedCountries") {
			allow[key] = append(allow[key], countryLabel(ctry))
		}
	}
	for _, c := range connCountries {
		key := c.Scope + "|" + fieldStr(c.Obj, "_provider") + "|" + fieldStr(c.Obj, "_connectionId")
		allow[key] = append(allow[key], countryLabel(c.Obj))
	}
	for key, countries := range allow {
		parts := strings.SplitN(key, "|", 3)
		base := geoAuditRow{Tenant: tenantName(idx, parts[0]), TenantID: parts[0], Provider: parts[1], Connection: parts[2]}
		if len(countries) > maxCountries {
			r := base
			r.Status, r.Finding = "broad", fmt.Sprintf("%d approved countries (more than %d)", len(countries), maxCountries)
			r.Country = strings.Join(countries, ", ")
			keep(r)
			continue
		}
		for _, ctry := range countries {
			r := base
			r.Country, r.Status, r.Finding = ctry, "allowed", "connection-level approved country"
			keep(r)
		}
	}
	// User-level travel exceptions.
	for _, u := range m365Users {
		for _, ex := range fieldList(u.Obj, "authorizedCountries") {
			start, end := fieldStr(ex, "startDate"), fieldStr(ex, "endDate")
			status, finding := classifyException(start, end, now, window)
			keep(geoAuditRow{Tenant: tenantName(idx, u.Scope), TenantID: u.Scope, Provider: "m365",
				Connection: fieldStr(u.Obj, "_connectionId"), User: firstNonEmpty(fieldStr(u.Obj, "email"), fieldStr(u.Obj, "name")),
				Country: countryLabel(ex), StartDate: start, EndDate: end, Status: status, Finding: finding})
		}
	}
	for _, u := range connUsers {
		for _, list := range []string{"activeApprovedCountries", "upcomingApprovedCountries"} {
			for _, ex := range fieldList(u.Obj, list) {
				start, end := fieldStr(ex, "startDate"), fieldStr(ex, "endDate")
				status, finding := classifyException(start, end, now, window)
				keep(geoAuditRow{Tenant: tenantName(idx, u.Scope), TenantID: u.Scope, Provider: fieldStr(u.Obj, "_provider"),
					Connection: fieldStr(u.Obj, "_connectionId"), User: firstNonEmpty(fieldStr(u.Obj, "email"), fieldStr(u.Obj, "name")),
					Country: countryLabel(ex), StartDate: start, EndDate: end, Status: status, Finding: finding})
			}
		}
	}
	rank := map[string]int{"expired": 0, "expiring": 1, "broad": 2, "open-ended": 3, "upcoming": 4, "active": 5, "allowed": 6}
	sort.SliceStable(rows, func(i, j int) bool {
		if rank[rows[i].Status] != rank[rows[j].Status] {
			return rank[rows[i].Status] < rank[rows[j].Status]
		}
		if rows[i].Tenant != rows[j].Tenant {
			return rows[i].Tenant < rows[j].Tenant
		}
		return rows[i].EndDate < rows[j].EndDate
	})
	return rows, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
