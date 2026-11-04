// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: triage.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type triageRow struct {
	Tenant       string   `json:"tenant"`
	TenantID     string   `json:"tenant_id"`
	DetectionID  string   `json:"detection_id"`
	Created      string   `json:"created"`
	Age          string   `json:"age"`
	AgeHours     float64  `json:"age_hours"`
	Type         string   `json:"type"`
	AlertCount   float64  `json:"alert_count"`
	Hostname     string   `json:"hostname"`
	Username     string   `json:"username"`
	TicketStatus string   `json:"ticket_status"`
	AlertTypes   []string `json:"alert_types"`
	age          time.Duration
}

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var olderThan, detType, ticketStatus, tenant, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "One oldest-first queue of open detections across every customer tenant",
		Long: `Use this command for the cross-tenant queue of open detections, oldest first.
Do NOT use this command for everything about one tenant; use 'overview' instead.

Reads the open alert groups fleet-sync stored for every tenant and lists them
oldest first with tenant name, age, alert count and nested ticket status.`,
		Example: strings.Trim(`
  blackpoint-cli triage --older-than 24h --type MDR --agent
  blackpoint-cli triage --ticket-status ESCALATE
  blackpoint-cli triage --tenant "Acme Dental" --limit 10`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "triage: read open detections from the local fleet store")
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be zero or more"))
			}
			minAge, err := parseFleetDuration(olderThan)
			if err != nil {
				return usageErr(fmt.Errorf("--older-than: %w", err))
			}
			detType = strings.ToUpper(strings.TrimSpace(detType))
			if detType != "" && detType != "CR" && detType != "MDR" {
				return usageErr(fmt.Errorf("--type must be CR or MDR, got %q", detType))
			}
			db, ok, err := openFleetForRead(cmd, flags, &dbPath)
			if err != nil {
				return err
			}
			if !ok {
				return emitRows(cmd, flags, []triageRow{})
			}
			defer db.Close()
			idx, tenants, err := fleetTenantIndex(db.DB())
			if err != nil {
				return err
			}
			scopes, err := filterTenantScope(tenants, tenant)
			if err != nil {
				return err
			}
			if err := checkFleetKind(cmd, flags, db.DB(), fleetKindDetection, scopes...); err != nil {
				return err
			}
			recs, err := loadCleanFleetRows(db.DB(), fleetKindDetection, fleetKindDetection, scopes...)
			if err != nil {
				return err
			}
			rows := buildTriageRows(recs, idx, time.Now().UTC(), minAge, detType, ticketStatus)
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return emitRows(cmd, flags, rows, "tenant", "age", "type", "alert_count", "hostname", "username", "ticket_status", "detection_id")
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "", "Only detections at least this old (e.g. 24h, 3d)")
	cmd.Flags().StringVar(&detType, "type", "", "Detection family: CR (Cloud Response) or MDR (endpoint)")
	cmd.Flags().StringVar(&ticketStatus, "ticket-status", "", "Only this nested ticket status (e.g. NEW, INVESTIGATE, ESCALATE)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Limit to one tenant (name or id)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's local data.db)")
	return cmd
}

func buildTriageRows(recs []fleetRecord, idx map[string]fleetTenant, now time.Time, minAge time.Duration, detType, ticketStatus string) []triageRow {
	rows := []triageRow{}
	for _, r := range recs {
		o := r.Obj
		if st := strings.ToUpper(fieldStr(o, "status")); st != "" && st != "OPEN" {
			continue
		}
		if detType != "" && !strings.EqualFold(fieldStr(o, "type"), detType) {
			continue
		}
		ts := nestedStr(o, "ticket", "status")
		if ticketStatus != "" && !strings.EqualFold(ts, ticketStatus) {
			continue
		}
		created := fieldStr(o, "created", "createdOn", "createdAt")
		var age time.Duration
		if t, ok := parseFleetTime(created); ok {
			age = now.Sub(t)
		}
		if minAge > 0 && age < minAge {
			continue
		}
		count, _ := fieldNum(o, "alertCount")
		var types []string
		if v, ok := fieldAny(o, "alertTypes"); ok {
			if arr, ok := v.([]any); ok {
				for _, a := range arr {
					if s, ok := a.(string); ok {
						types = append(types, s)
					}
				}
			}
		}
		rows = append(rows, triageRow{
			Tenant: tenantName(idx, r.Scope), TenantID: r.Scope, DetectionID: r.ID,
			Created: created, Age: humanAge(age), AgeHours: float64(int(age.Hours()*10)) / 10,
			Type: fieldStr(o, "type"), AlertCount: count, Hostname: fieldStr(o, "hostname"),
			Username: fieldStr(o, "username"), TicketStatus: ts, AlertTypes: types, age: age,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].age > rows[j].age })
	return rows
}
