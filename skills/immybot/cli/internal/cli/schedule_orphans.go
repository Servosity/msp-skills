// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: maintenance schedules that point at nothing.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// scheduleRecord is the slice of GET /api/v1/schedules (GetScheduleResponse)
// this command reads. IDs decode as json.Number so a large int never rounds.
type scheduleRecord struct {
	ID                    json.Number     `json:"id"`
	Name                  string          `json:"name"`
	Disabled              bool            `json:"disabled"`
	TargetMissing         bool            `json:"targetMissing"`
	TenantMissing         bool            `json:"tenantMissing"`
	TargetTypeMissing     bool            `json:"targetTypeMissing"`
	TargetText            string          `json:"targetText"`
	TargetTypeName        string          `json:"targetTypeName"`
	TenantID              *json.Number    `json:"tenantId"`
	MaintenanceType       json.RawMessage `json:"maintenanceType"`
	MaintenanceIdentifier string          `json:"maintenanceIdentifier"`
	NextOccurrence        string          `json:"nextOccurenceDate"`
	UpdatedDateUTC        string          `json:"updatedDateUTC"`
}

type scheduleOrphan struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	TenantID              string   `json:"tenant_id,omitempty"`
	Tenant                string   `json:"tenant,omitempty"`
	TargetText            string   `json:"target_text,omitempty"`
	TargetType            string   `json:"target_type,omitempty"`
	MaintenanceType       string   `json:"maintenance_type,omitempty"`
	MaintenanceIdentifier string   `json:"maintenance_identifier,omitempty"`
	Disabled              bool     `json:"disabled"`
	Orphaned              bool     `json:"orphaned"`
	Reasons               []string `json:"reasons"`
	NextOccurrence        string   `json:"next_occurrence,omitempty"`
	UpdatedDate           string   `json:"updated_date,omitempty"`
}

type scheduleOrphansView struct {
	Scanned       int `json:"scanned"`
	MatchedCount  int `json:"matched_count"`
	OrphanedCount int `json:"orphaned_count"`
	DisabledCount int `json:"disabled_count"`
	Unreadable    int `json:"unreadable_rows"`
	// FilteredOut counts listed-worthy schedules --tenant left out;
	// TenantUnresolved counts those whose tenant id has no mirrored name, so
	// the filter could not tell whether they match.
	FilteredOut      int              `json:"filtered_out"`
	TenantUnresolved int              `json:"tenant_unresolved"`
	Schedules        []scheduleOrphan `json:"schedules"`
	Note             string           `json:"note,omitempty"`
}

// scheduleOrphanReasons names every reason a schedule is listed. Orphaned
// means the schedule still points at a target, tenant or target type that no
// longer exists; ImmyBot reports each one on the schedule itself.
func scheduleOrphanReasons(r scheduleRecord) (reasons []string, orphaned bool) {
	reasons = []string{}
	if r.TargetMissing {
		reasons = append(reasons, "target missing")
	}
	if r.TenantMissing {
		reasons = append(reasons, "tenant missing")
	}
	if r.TargetTypeMissing {
		reasons = append(reasons, "target type missing")
	}
	orphaned = len(reasons) > 0
	if r.Disabled {
		reasons = append(reasons, "disabled")
	}
	return reasons, orphaned
}

// immyEnumText renders an enum that ImmyBot documents as "the string or
// integer value may be used": a JSON string stays as written, a number is
// shown as its number.
func immyEnumText(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	return s
}

func newNovelScheduleOrphansCmd(flags *rootFlags) *cobra.Command {
	var (
		flagOrphanedOnly bool
		flagTenant       string
		flagLimit        int
		dbPath           string
	)
	cmd := &cobra.Command{
		Use:   "schedule-orphans",
		Short: "Maintenance schedules whose target, tenant or target type no longer exists, plus disabled schedules",
		Long: "List maintenance schedules from the local mirror that ImmyBot itself flags as pointing at a " +
			"deleted target, tenant or target type (targetMissing, tenantMissing, targetTypeMissing on " +
			"GET /api/v1/schedules), plus disabled schedules. A schedule like this still looks configured " +
			"in the console while its maintenance window does nothing. Run 'sync --resources schedules,tenants' first.",
		Example: strings.Trim(`
  immybot-cli schedule-orphans --agent
  immybot-cli schedule-orphans --orphaned-only --tenant Contoso
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "schedule-orphans")
			}
			if flagLimit < 0 {
				return usageErr(fmt.Errorf("--limit must be 0 (all) or more, got %d", flagLimit))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			view := scheduleOrphansView{Schedules: make([]scheduleOrphan, 0)}
			dbPath = immyMirrorPath(dbPath)
			if immyMirrorMissing(cmd, dbPath, "schedules,tenants") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			db, err := immyOpenMirror(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "schedules") {
				hintIfStale(cmd, db, "schedules", flags.maxAge)
			}

			tenants, err := immyTenantNames(ctx, db)
			if err != nil {
				return err
			}
			rows, err := db.DB().QueryContext(ctx, `SELECT id, data FROM resources WHERE resource_type = 'schedules'`)
			if err != nil {
				if immyNoSuchTableErr(err) {
					view.Note = "no schedules in the local mirror; run 'immybot-cli sync --resources schedules,tenants'"
					return printScheduleOrphans(cmd, flags, view)
				}
				return fmt.Errorf("querying schedules: %w", err)
			}
			tenantFilter := strings.ToLower(strings.TrimSpace(flagTenant))
			for rows.Next() {
				var id string
				var data []byte
				if err := rows.Scan(&id, &data); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning schedule row: %w", err)
				}
				view.Scanned++
				var rec scheduleRecord
				dec := json.NewDecoder(strings.NewReader(string(data)))
				dec.UseNumber()
				if err := dec.Decode(&rec); err != nil {
					// Counted, never silently dropped: an unreadable row is a
					// schedule this report cannot vouch for.
					view.Unreadable++
					continue
				}
				reasons, orphaned := scheduleOrphanReasons(rec)
				if len(reasons) == 0 || (flagOrphanedOnly && !orphaned) {
					continue
				}
				o := scheduleOrphan{
					ID:                    rec.ID.String(),
					Name:                  rec.Name,
					TargetText:            rec.TargetText,
					TargetType:            rec.TargetTypeName,
					MaintenanceType:       immyEnumText(rec.MaintenanceType),
					MaintenanceIdentifier: rec.MaintenanceIdentifier,
					Disabled:              rec.Disabled,
					Orphaned:              orphaned,
					Reasons:               reasons,
					NextOccurrence:        rec.NextOccurrence,
					UpdatedDate:           rec.UpdatedDateUTC,
				}
				if o.ID == "" {
					o.ID = id
				}
				if rec.TenantID != nil {
					o.TenantID = rec.TenantID.String()
					o.Tenant = tenants[o.TenantID]
				}
				if tenantFilter != "" && !strings.Contains(strings.ToLower(o.Tenant), tenantFilter) {
					// A tenant id with no mirrored name cannot be matched or
					// ruled out by name: count it apart so the result never
					// reads as an all-clear for it.
					if o.TenantID != "" && o.Tenant == "" {
						view.TenantUnresolved++
					} else {
						view.FilteredOut++
					}
					continue
				}
				view.MatchedCount++
				if orphaned {
					view.OrphanedCount++
				}
				if rec.Disabled {
					view.DisabledCount++
				}
				view.Schedules = append(view.Schedules, o)
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("iterating schedules: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("closing schedules: %w", err)
			}
			// Orphans first (they run and do nothing), then by name.
			sort.SliceStable(view.Schedules, func(i, j int) bool {
				a, b := view.Schedules[i], view.Schedules[j]
				if a.Orphaned != b.Orphaned {
					return a.Orphaned
				}
				if a.Name != b.Name {
					return a.Name < b.Name
				}
				return a.ID < b.ID
			})
			// Totals above are computed before the limit cuts the list.
			if flagLimit > 0 && len(view.Schedules) > flagLimit {
				view.Schedules = view.Schedules[:flagLimit]
			}
			switch {
			case view.Scanned == 0:
				view.Note = "no schedules in the local mirror; run 'immybot-cli sync --resources schedules,tenants'"
			case view.TenantUnresolved > 0:
				view.Note = fmt.Sprintf("%d orphaned or disabled schedule(s) have a tenant id with no tenant name in the local mirror, "+
					"so --tenant could not check them; run 'immybot-cli sync --resources tenants' and retry", view.TenantUnresolved)
			case view.MatchedCount == 0 && view.FilteredOut > 0:
				view.Note = fmt.Sprintf("scanned %d schedule(s); %d orphaned or disabled schedule(s) belong to tenants --tenant left out", view.Scanned, view.FilteredOut)
			case view.MatchedCount == 0:
				view.Note = fmt.Sprintf("scanned %d schedule(s); none is orphaned or disabled", view.Scanned)
			}
			if view.Unreadable > 0 {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" %d schedule row(s) could not be read and are not reported on.", view.Unreadable))
			}
			return printScheduleOrphans(cmd, flags, view)
		},
	}
	cmd.Flags().BoolVar(&flagOrphanedOnly, "orphaned-only", false, "Leave out schedules that are only disabled")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only schedules whose tenant name contains this substring")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum schedules to list (0 for all); totals count every match")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

func printScheduleOrphans(cmd *cobra.Command, flags *rootFlags, view scheduleOrphansView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), view, flags)
	}
	w := cmd.OutOrStdout()
	if len(view.Schedules) == 0 {
		if view.TenantUnresolved > 0 || view.FilteredOut > 0 {
			fmt.Fprintf(w, "No orphaned or disabled schedules matched --tenant (scanned %d).\n", view.Scanned)
		} else {
			fmt.Fprintf(w, "No orphaned or disabled schedules (scanned %d).\n", view.Scanned)
		}
		if view.Note != "" {
			fmt.Fprintln(w, immyPrintable(view.Note))
		}
		return nil
	}
	fmt.Fprintf(w, "%-8s %-32s %-24s %s\n", "ID", "SCHEDULE", "TENANT", "REASONS")
	for _, s := range view.Schedules {
		fmt.Fprintf(w, "%-8s %-32s %-24s %s\n", immyPrintable(s.ID), immyTruncate(immyPrintable(s.Name), 32),
			immyTruncate(immyPrintable(s.Tenant), 24), strings.Join(s.Reasons, ", "))
	}
	fmt.Fprintf(w, "\n%d listed: %d orphaned, %d disabled (scanned %d).\n",
		view.MatchedCount, view.OrphanedCount, view.DisabledCount, view.Scanned)
	if view.Note != "" {
		fmt.Fprintln(w, immyPrintable(view.Note))
	}
	return nil
}
