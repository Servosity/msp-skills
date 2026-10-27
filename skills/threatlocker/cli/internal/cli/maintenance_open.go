// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatlocker-pp-cli/internal/store"
)

// openMaintenanceWindow is one maintenance-mode window that is open at the
// moment of the query: it has started and has not ended.
type openMaintenanceWindow struct {
	ComputerID       string `json:"computerId"`
	ComputerName     string `json:"computerName"`
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
	MaintenanceType  string `json:"maintenanceType"`
	StartDateTime    string `json:"startDateTime"`
	EndDateTime      string `json:"endDateTime"`
	CreatedBy        string `json:"createdBy"`
	OpenHours        int64  `json:"openHours"`
	Overdue          bool   `json:"overdue"`
}

// maintenanceScan is the result of classifying every synced window. Rows whose
// start cannot be parsed are counted, never guessed open or closed.
type maintenanceScan struct {
	Open      []openMaintenanceWindow
	Scheduled int
	Unparsed  int
	Scanned   int
}

// tlLatestMaintenanceRows selects one maintenance row per window identity
// (computer, type, start), the most recently synced. Windows carry no vendor
// id, so the store keys them by a fingerprint that includes the end time: a
// window whose end was edited lands as a second row and the superseded
// version is never pruned. A row with no start is its own identity so it is
// still counted as unparsed. Hand-wired (class: superseded snapshot rows).
const tlLatestMaintenanceRows = `(SELECT * FROM (SELECT mm.*, ROW_NUMBER() OVER (
		PARTITION BY mm.computer_id, mm.maintenance_type, COALESCE(NULLIF(mm.start_date_time, ''), mm.id)
		ORDER BY julianday(mm.synced_at) DESC, mm.rowid DESC) AS tl_version
	FROM maintenance mm) WHERE tl_version = 1)`

// scanOpenMaintenance reads the synced maintenance history and returns the
// windows open at now. org filters to one tenant when non-empty. A window with
// no parseable end is open-ended, so it counts as open: an endpoint left in
// maintenance with no end is exactly what this command exists to find.
func scanOpenMaintenance(ctx context.Context, db *store.Store, org string, now time.Time, overdueAfter time.Duration) (maintenanceScan, error) {
	query := `SELECT m.computer_id, c.computer_name, c.organization_id, c.organization_name,
		m.maintenance_type, m.start_date_time, m.end_date_time, m.created_by
		FROM ` + tlLatestMaintenanceRows + ` m
		LEFT JOIN computers c ON c.computer_id = m.computer_id`
	var args []any
	if org != "" {
		query += ` WHERE c.organization_id = ?`
		args = append(args, org)
	}
	rows, err := db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return maintenanceScan{}, fmt.Errorf("querying maintenance windows: %w", err)
	}
	defer rows.Close()
	var scan maintenanceScan
	for rows.Next() {
		var compID, compName, orgID, orgName, mType, start, end, createdBy *string
		if err := rows.Scan(&compID, &compName, &orgID, &orgName, &mType, &start, &end, &createdBy); err != nil {
			return maintenanceScan{}, fmt.Errorf("scanning maintenance window: %w", err)
		}
		scan.Scanned++
		startAt, ok := parseTLTime(tlString(start))
		if !ok {
			scan.Unparsed++
			continue
		}
		if startAt.After(now) {
			scan.Scheduled++
			continue
		}
		if endAt, ok := parseTLTime(tlString(end)); ok && !endAt.After(now) {
			continue
		}
		open := now.Sub(startAt)
		scan.Open = append(scan.Open, openMaintenanceWindow{
			ComputerID: tlString(compID), ComputerName: tlString(compName),
			OrganizationID: tlString(orgID), OrganizationName: tlString(orgName),
			MaintenanceType: tlString(mType), StartDateTime: tlString(start),
			EndDateTime: tlString(end), CreatedBy: tlString(createdBy),
			OpenHours: int64(open / time.Hour), Overdue: open > overdueAfter,
		})
	}
	if err := rows.Err(); err != nil {
		return maintenanceScan{}, fmt.Errorf("iterating maintenance windows: %w", err)
	}
	// Longest-open first; the computer id breaks ties so output is stable.
	sort.SliceStable(scan.Open, func(i, j int) bool {
		if scan.Open[i].OpenHours != scan.Open[j].OpenHours {
			return scan.Open[i].OpenHours > scan.Open[j].OpenHours
		}
		return scan.Open[i].ComputerID < scan.Open[j].ComputerID
	})
	return scan, nil
}

// parseWindow reads a relative window ("24h", "7d", "90m"). Unlike resolveSince
// it refuses an empty or absolute value: a threshold must be a duration.
func parseWindow(s string) (time.Duration, error) {
	now := time.Now()
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if _, ok := parseTLTime(s); ok {
		return 0, fmt.Errorf("%q is a date, not a duration (use 24h, 7d or 90m)", s)
	}
	cutoff, ok := resolveSince(s, now)
	if !ok || cutoff.After(now) {
		return 0, fmt.Errorf("invalid duration %q (use 24h, 7d or 90m)", s)
	}
	return now.Sub(cutoff), nil
}

func newNovelMaintenanceOpenCmd(flags *rootFlags) *cobra.Command {
	var flagOlderThan string
	var flagAllTenants bool
	var flagOverdueOnly bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "open",
		Short: "Endpoints whose maintenance-mode window is open right now, across tenants, longest-open first",
		Long: `Open reads the locally synced maintenance-mode history and lists every
endpoint whose window has started and not ended: learning, installation,
monitor-only or any other mode that relaxes protection. Windows with no end
count as open. Windows open longer than --older-than are flagged overdue,
which is how an endpoint a technician forgot in maintenance shows up.

Maintenance history syncs as a dependent of computers:
  threatlocker-cli sync --resources computers`,
		Example: strings.Trim(`
  threatlocker-cli maintenance open --older-than 24h --agent
  threatlocker-cli maintenance open --all-tenants --overdue-only --older-than 7d
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			overdueAfter, err := parseWindow(flagOlderThan)
			if err != nil {
				return usageErr(fmt.Errorf("--older-than: %w", err))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "maintenance open")
			}
			db, err := tlOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli sync --resources computers' first.", err)
			}
			defer db.Close()
			scan, err := scanOpenMaintenance(cmd.Context(), db, tlNovelTenantScope(flagAllTenants), time.Now().UTC(), overdueAfter)
			if err != nil {
				return err
			}
			windows := make([]openMaintenanceWindow, 0, len(scan.Open))
			overdue := 0
			for _, w := range scan.Open {
				if w.Overdue {
					overdue++
				}
				if !flagOverdueOnly || w.Overdue {
					windows = append(windows, w)
				}
			}
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"open":           windows,
					"open_count":     len(scan.Open),
					"overdue_count":  overdue,
					"scheduled":      scan.Scheduled,
					"unparsed_start": scan.Unparsed,
					"scanned":        scan.Scanned,
					"older_than":     flagOlderThan,
				})
			}
			out := cmd.OutOrStdout()
			if scan.Scanned == 0 {
				fmt.Fprintln(out, "No synced maintenance windows. Run: threatlocker-cli sync --resources computers")
				return nil
			}
			if scan.Unparsed > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d maintenance window(s) have an unreadable start time and were not classified\n", scan.Unparsed)
			}
			if len(windows) == 0 {
				fmt.Fprintf(out, "No open maintenance windows (%d scanned, %d scheduled for later).\n", scan.Scanned, scan.Scheduled)
				return nil
			}
			headers := []string{"ORG", "COMPUTER", "TYPE", "STARTED", "ENDS", "OPEN_HOURS", "OVERDUE"}
			tableRows := make([][]string, 0, len(windows))
			for _, w := range windows {
				org := w.OrganizationName
				if org == "" {
					org = w.OrganizationID
				}
				computer := w.ComputerName
				if computer == "" {
					computer = w.ComputerID
				}
				end := w.EndDateTime
				if end == "" {
					end = "(none)"
				}
				tableRows = append(tableRows, []string{org, computer, w.MaintenanceType, w.StartDateTime, end, fmt.Sprintf("%d", w.OpenHours), fmt.Sprintf("%t", w.Overdue)})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&flagOlderThan, "older-than", "24h", "Flag windows open longer than this as overdue (24h, 7d, 90m)")
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", false, "Include windows from every synced organization (default scopes to --org when set)")
	cmd.Flags().BoolVar(&flagOverdueOnly, "overdue-only", false, "List only overdue windows")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
