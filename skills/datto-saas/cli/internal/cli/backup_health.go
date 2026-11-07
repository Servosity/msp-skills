// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type healthRow struct {
	SaasCustomerID        string   `json:"saas_customer_id"`
	SaasCustomerName      string   `json:"saas_customer_name"`
	Domains               []string `json:"domains"`
	ActiveServices        *float64 `json:"active_services"`
	RecentBackupServices  *float64 `json:"recent_backup_services"`
	CoveragePct           *float64 `json:"coverage_pct"`
	PerfectBackupPct      *float64 `json:"perfect_backup_pct"`
	WorstApp              string   `json:"worst_app,omitempty"`
	WorstAppCoveragePct   *float64 `json:"worst_app_coverage_pct"`
	ShortWindowStreak     int      `json:"short_window_streak"`
	ApplicationReportSeen bool     `json:"application_report_synced"`
	Notes                 []string `json:"notes"`
}

type appHealthRow struct {
	SaasCustomerID    string   `json:"saas_customer_id"`
	SuiteType         string   `json:"suite_type"`
	AppType           string   `json:"app_type"`
	Windows           int      `json:"windows"`
	LatestWindow      string   `json:"latest_window"`
	LatestStatus      string   `json:"latest_status"`
	ActiveServices    *float64 `json:"active_services"`
	WithBackup        *float64 `json:"with_backup"`
	PerfectBackup     *float64 `json:"perfect_backup"`
	CoveragePct       *float64 `json:"coverage_pct"`
	PerfectBackupPct  *float64 `json:"perfect_backup_pct"`
	ShortWindowStreak int      `json:"short_window_streak"`
	// CountsComplete is true only when the latest window is identifiable and
	// carries present, consistent active, with-backup and perfect-backup counts.
	CountsComplete bool `json:"counts_complete"`
	// MirrorIncomplete is true when the last applications sync did not finish.
	MirrorIncomplete bool `json:"mirror_incomplete,omitempty"`
	// LatestUnknown is true when windows cannot be ordered (missing startTime),
	// so no window can be called the latest.
	LatestUnknown bool `json:"latest_window_unknown"`
}

// appHealth computes per-app rows from numeric counts only; the backup status
// string has no documented enum, so it is passed through untouched.
func appHealth(r saasAppReport) []appHealthRow {
	out := make([]appHealthRow, 0)
	for _, suite := range r.Suites {
		for _, app := range suite.AppTypes {
			h := append([]saasWindow(nil), app.BackupHistory...)
			if len(h) == 0 {
				// Keep the app visible: an app with no history is missing evidence.
				out = append(out, appHealthRow{SaasCustomerID: r.parentID, SuiteType: suite.SuiteType, AppType: app.AppType})
				continue
			}
			dated := true
			for _, w := range h {
				if !w.StartTime.ok {
					dated = false
				}
			}
			if !dated {
				// Without startTime the latest window (even a single one) cannot be
				// placed in time, so report the app as unusable evidence.
				out = append(out, appHealthRow{SaasCustomerID: r.parentID, SuiteType: suite.SuiteType, AppType: app.AppType, Windows: len(h), LatestUnknown: true})
				continue
			}
			sort.SliceStable(h, func(a, b int) bool { return h[a].StartTime.v < h[b].StartTime.v })
			last := h[len(h)-1]
			row := appHealthRow{SaasCustomerID: r.parentID, SuiteType: suite.SuiteType, AppType: app.AppType, Windows: len(h),
				LatestWindow: string(last.TimeWindow), LatestStatus: string(last.Status),
				ActiveServices: last.Active.ptr(), WithBackup: last.WithBackup.ptr(), PerfectBackup: last.PerfectBackup.ptr(),
				CoveragePct: pctOf(last.WithBackup, last.Active), PerfectBackupPct: pctOf(last.PerfectBackup, last.Active)}
			// Completeness is about presence and consistency, not about whether a
			// percentage exists: zero active services is complete, with null rates.
			row.CountsComplete = validPair(last.Active, last.WithBackup) && validPair(last.WithBackup, last.PerfectBackup)
			// Consecutive most-recent windows where not every active service backed up.
			for i := len(h) - 1; i >= 0; i-- {
				w := h[i]
				if !validPair(w.Active, w.WithBackup) || w.WithBackup.v >= w.Active.v {
					break
				}
				row.ShortWindowStreak++
			}
			out = append(out, row)
		}
	}
	return out
}

func newNovelBackupHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath, customer string
	var below float64
	cmd := &cobra.Command{
		Use:   "backup-health",
		Short: "Rank clients worst first by backup coverage, then consecutive short backup windows and perfect-backup rate.",
		Long: strings.Trim(`
Ranks synced clients worst first: by coverage_pct, then the longest short
backup-window streak, then perfect_backup_pct, then worst-app coverage.
Clients whose coverage is unknown sort last and are kept by --below.
coverage_pct comes from each domain's backupStats (services with a recent
backup over active services, summed per client); it is null when a client has
no active services or any domain's counts are missing or inconsistent. From the synced application reports it adds the latest-window
perfect-backup rate, the worst app, and short_window_streak: the number of
most recent windows in which not every active service backed up. Only the
numeric counts are used; backup status strings are passed through as-is.

--customer switches to one row per application for that client.

Use this command to rank clients by how well their protected services are backing up (coverage %, perfect-backup %, consecutive short windows), or with --customer to see one client per app.
Do NOT use this command to find seats that are not protected at all; use 'seats gaps' instead.
Do NOT use it for a client's full exit manifest; use 'exit-inventory' instead.`, "\n"),
		Example: strings.Trim(`
  datto-saas-cli backup-health --below 95 --agent
  datto-saas-cli backup-health --customer 123456`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "backup-health")
			}
			if below < 0 || below > 100 {
				return usageErr(fmt.Errorf("--below must be between 0 and 100"))
			}
			rows := make([]healthRow, 0)
			db, missing, err := openSaasStore(cmd, dbPath)
			if err != nil {
				return err
			}
			if missing {
				if customer != "" {
					return printSaasRows(cmd, flags, make([]appHealthRow, 0), "No local data. Run: datto-saas-cli sync")
				}
				return printSaasRows(cmd, flags, rows, "No local data. Run: datto-saas-cli sync")
			}
			defer db.Close()
			rd, err := beginSaasRead(cmd, db, flags, "domains", "applications")
			if err != nil {
				return err
			}
			defer rd.Close()
			partial := rd.partial
			domainsPartial, appsPartial := containsStr(partial, "domains"), containsStr(partial, "applications")
			domains, err := loadDomains(cmd.Context(), rd.tx)
			if err != nil {
				return err
			}
			customers := groupCustomers(domains)
			cid, err := customerFilter(customers, customer)
			if err != nil {
				return err
			}
			apps, err := loadApplications(cmd.Context(), rd.tx, cid)
			if err != nil {
				return err
			}
			if cid != "" {
				appRows := make([]appHealthRow, 0)
				for _, a := range apps {
					appRows = append(appRows, appHealth(a)...)
				}
				if appsPartial {
					for i := range appRows {
						appRows[i] = appHealthRow{SaasCustomerID: appRows[i].SaasCustomerID, SuiteType: appRows[i].SuiteType,
							AppType: appRows[i].AppType, Windows: appRows[i].Windows, MirrorIncomplete: true}
					}
				}
				sort.SliceStable(appRows, func(a, b int) bool { return lessPct(appRows[a].CoveragePct, appRows[b].CoveragePct) })
				return printSaasRows(cmd, flags, appRows, "No application backup history synced for this client.")
			}
			appsBy := map[string][]appHealthRow{}
			emptyReport := map[string]bool{}
			seen := map[string]bool{}
			for _, a := range apps {
				rows := appHealth(a)
				if len(rows) == 0 {
					emptyReport[a.parentID] = true // a report with no applications is missing evidence
				}
				appsBy[a.parentID] = append(appsBy[a.parentID], rows...)
				seen[a.parentID] = true
			}
			for _, c := range customers {
				r := healthRow{SaasCustomerID: c.ID, SaasCustomerName: c.Name, Domains: c.domainNames(), Notes: make([]string, 0), ApplicationReportSeen: seen[c.ID]}
				// Sum only complete, consistent per-domain pairs; one incomplete
				// domain makes the client's coverage unknown rather than inflated.
				var active, recent flexNum
				complete := len(c.Domains) > 0
				for _, d := range c.Domains {
					a, rc := d.BackupStats.Active, d.BackupStats.Recent
					if !validPair(a, rc) {
						complete = false
						continue
					}
					active = flexNum{v: active.v + a.v, ok: true}
					recent = flexNum{v: recent.v + rc.v, ok: true}
				}
				r.ActiveServices, r.RecentBackupServices = active.ptr(), recent.ptr()
				if complete && !domainsPartial {
					r.CoveragePct = pctOf(recent, active)
				} else {
					r.Notes = append(r.Notes, "coverage_incomplete")
				}
				if r.CoveragePct == nil {
					r.Notes = append(r.Notes, "coverage_unknown")
				}
				var perfect, latestActive flexNum
				appsComplete := len(appsBy[c.ID]) > 0 && !emptyReport[c.ID] && !appsPartial
				for _, a := range appsBy[c.ID] {
					if !a.CountsComplete {
						appsComplete = false
					} else {
						perfect = flexNum{v: perfect.v + *a.PerfectBackup, ok: true}
						latestActive = flexNum{v: latestActive.v + *a.ActiveServices, ok: true}
					}
					if a.CoveragePct != nil && (r.WorstAppCoveragePct == nil || *a.CoveragePct < *r.WorstAppCoveragePct) {
						v := *a.CoveragePct
						r.WorstAppCoveragePct, r.WorstApp = &v, a.AppType
					}
					if a.ShortWindowStreak > r.ShortWindowStreak {
						r.ShortWindowStreak = a.ShortWindowStreak
					}
				}
				if appsPartial {
					// An unfinished applications sync: no app-derived metric is claimed.
					r.WorstApp, r.WorstAppCoveragePct, r.ShortWindowStreak = "", nil, 0
				}
				if appsComplete {
					r.PerfectBackupPct = pctOf(perfect, latestActive)
				} else if len(appsBy[c.ID]) > 0 || emptyReport[c.ID] || appsPartial {
					r.Notes = append(r.Notes, "app_counts_incomplete")
				}
				if !r.ApplicationReportSeen {
					r.Notes = append(r.Notes, "no_application_report")
				}
				// Unknown coverage and incomplete app evidence stay in a --below view:
				// they need a look too.
				if below > 0 && r.CoveragePct != nil && !belowPct(r.CoveragePct, below) && !belowPct(r.WorstAppCoveragePct, below) &&
					(appsComplete || (!seen[c.ID] && !appsPartial)) {
					continue
				}
				rows = append(rows, r)
			}
			sort.SliceStable(rows, func(a, b int) bool {
				if !samePct(rows[a].CoveragePct, rows[b].CoveragePct) {
					return lessPct(rows[a].CoveragePct, rows[b].CoveragePct)
				}
				if rows[a].ShortWindowStreak != rows[b].ShortWindowStreak {
					return rows[a].ShortWindowStreak > rows[b].ShortWindowStreak
				}
				if !samePct(rows[a].PerfectBackupPct, rows[b].PerfectBackupPct) {
					return lessPct(rows[a].PerfectBackupPct, rows[b].PerfectBackupPct)
				}
				return lessPct(rows[a].WorstAppCoveragePct, rows[b].WorstAppCoveragePct)
			})
			return printSaasRows(cmd, flags, rows, emptyMsg(domains, "No clients match."))
		},
	}
	cmd.Flags().Float64Var(&below, "below", 0, "Only clients whose coverage or worst-app coverage is under this percent (0 = all)")
	cmd.Flags().StringVar(&customer, "customer", "", "Show per-application rows for one client: saasCustomerId, customer name, or domain")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's synced store)")
	return cmd
}

// lessPct orders known percentages ascending and unknown (nil) last.
func lessPct(a, b *float64) bool {
	switch {
	case a == nil:
		return false
	case b == nil:
		return true
	}
	return *a < *b
}

func samePct(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func belowPct(v *float64, threshold float64) bool { return v != nil && *v < threshold }
