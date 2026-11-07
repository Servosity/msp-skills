// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type companyOverview struct {
	Company            string  `json:"company"`
	CompanyUID         string  `json:"company_uid,omitempty"`
	Matched            bool    `json:"matched"`
	BackupServers      int     `json:"backup_servers"`
	JobsTotal          int     `json:"jobs_total"`
	JobsSuccess        int     `json:"jobs_success"`
	JobsWarning        int     `json:"jobs_warning"`
	JobsFailed         int     `json:"jobs_failed"`
	JobsRunning        int     `json:"jobs_running"`
	AgentsOnline       int     `json:"agents_online"`
	AgentsOffline      int     `json:"agents_offline"`
	ActiveAlarms       int     `json:"active_alarms"`
	AlarmErrors        int     `json:"alarm_errors"`
	ProtectedWorkloads int     `json:"protected_workloads"`
	WorkloadsAtRisk    int     `json:"workloads_at_risk"`
	LicenseUsedPoints  float64 `json:"license_used_points"`
	Note               string  `json:"note,omitempty"`
}

// pp:data-source local
func newNovelCompanyOverviewCmd(flags *rootFlags) *cobra.Command {
	var flagCompany, dbPath, flagRPO string

	cmd := &cobra.Command{
		Use:   "company-overview",
		Short: "Single-tenant 360: backup servers, job status breakdown, agents, active alarms, protected workloads, and license usage.",
		Long: strings.Trim(`
Assemble one tenant's full picture from the local mirror: backup servers in
use, job status breakdown, backup agents online/offline, active alarms (errors
called out), protected workloads (and how many are past RPO), and license used
points. Pass --company as the company name or its organization uid.

Reads only the local SQLite mirror — run `+"`veeam-cli sync`"+` first.`, "\n"),
		Example: strings.Trim(`
  veeam-cli company-overview --company "Contoso Ltd"
  veeam-cli company-overview --company 3fa85f64-5717-4562-b3fc-2c963f66afa6 --agent
  veeam-cli company-overview --company "Contoso Ltd" --rpo 4h`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			target := strings.TrimSpace(flagCompany)
			if target == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--company is required (company name or organization uid)"))
			}
			rpo, err := veeamParseWindow(flagRPO, 24*time.Hour)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --rpo: %w", err))
			}
			ctx := cmd.Context()
			db, ok, err := veeamOpenStoreRead(dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			if ok {
				defer db.Close()
			}

			names, err := veeamCompanyNames(ctx, db)
			if err != nil {
				return err
			}
			// A UID names one company; a display name may name several
			// (Company.name is not unique), and then the match is refused.
			orgUID, companyName, matched := target, target, false
			if name, ok := names[strings.ToLower(target)]; ok {
				orgUID, companyName, matched = strings.ToLower(target), name, true
			} else {
				var uids []string
				for uid, name := range names {
					if strings.EqualFold(name, target) {
						uids = append(uids, uid)
					}
				}
				sort.Strings(uids)
				switch len(uids) {
				case 0: // not a known name: treat the input as a uid
				case 1:
					orgUID, companyName, matched = uids[0], names[uids[0]], true
				default:
					return usageErr(fmt.Errorf("%d companies are named %q (%s): pass the organization uid instead", len(uids), target, strings.Join(uids, ", ")))
				}
			}
			match := func(m veeamRow) bool {
				return strings.EqualFold(veeamOrgOf(m), orgUID)
			}

			ov := companyOverview{Company: companyName, CompanyUID: orgUID, Matched: matched}
			now := time.Now()
			cutoff := now.Add(-rpo)
			servers := map[string]bool{}

			jobs, err := veeamLoad(ctx, db, veeamPathServerJobs, veeamPathAgentJobs)
			if err != nil {
				return err
			}
			for _, j := range jobs {
				if !match(j) {
					continue
				}
				ov.JobsTotal++
				if bs := vstr(j, "backupServerUid"); bs != "" {
					servers[strings.ToLower(bs)] = true
				}
				switch veeamJobHealth(vstr(j, "status")) {
				case "success":
					ov.JobsSuccess++
				case "warning":
					ov.JobsWarning++
				case "failed":
					ov.JobsFailed++
				case "running":
					ov.JobsRunning++
				}
			}
			// Servers the company owns (BackupServer.organizationUid), plus
			// any other server that runs one of its jobs.
			inventory, err := veeamLoad(ctx, db, veeamPathBackupServers)
			if err != nil {
				return err
			}
			for _, s := range inventory {
				if uid := vstr(s, "instanceUid"); uid != "" && match(s) {
					servers[strings.ToLower(uid)] = true
				}
			}
			ov.BackupServers = len(servers)

			agents, err := veeamLoad(ctx, db, veeamPathAgents)
			if err != nil {
				return err
			}
			for _, a := range agents {
				if !match(a) {
					continue
				}
				if veeamAgentOnline(vstr(a, "status")) {
					ov.AgentsOnline++
				} else {
					ov.AgentsOffline++
				}
			}

			alarms, err := veeamLoad(ctx, db, veeamPathActiveAlarms)
			if err != nil {
				return err
			}
			for _, al := range alarms {
				if !veeamAlarmActive(al) || !strings.EqualFold(vstr(al, "object.organizationUid"), orgUID) {
					continue
				}
				ov.ActiveAlarms++
				if veeamSeverityRank(vstr(al, "lastActivation.status")) == 0 {
					ov.AlarmErrors++
				}
			}

			workloads, err := veeamLoad(ctx, db, veeamPathWorkloadsServer, veeamPathWorkloadsConsole)
			if err != nil {
				return err
			}
			for _, w := range workloads {
				if !match(w) {
					continue
				}
				ov.ProtectedWorkloads++
				last, has := vtime(w, "latestRestorePointDate")
				if !has || last.Before(cutoff) {
					ov.WorkloadsAtRisk++
				}
			}

			usage, err := veeamLoad(ctx, db, veeamPathLicenseUsage)
			if err != nil {
				return err
			}
			for _, u := range usage {
				if strings.EqualFold(vstr(u, "organizationUid"), orgUID) {
					ov.LicenseUsedPoints += veeamUsedPoints(u)
				}
			}

			if !matched && ov.JobsTotal == 0 && ov.AgentsOnline == 0 && ov.AgentsOffline == 0 &&
				ov.ActiveAlarms == 0 && ov.ProtectedWorkloads == 0 {
				ov.Note = "No matching tenant data in the local mirror. Check the name/uid and run `veeam-cli sync` first."
			}

			table := []map[string]any{{
				"company":        ov.Company,
				"backup_servers": ov.BackupServers,
				"jobs":           ov.JobsTotal,
				"failed":         ov.JobsFailed,
				"agents_offline": ov.AgentsOffline,
				"active_alarms":  ov.ActiveAlarms,
				"at_risk":        ov.WorkloadsAtRisk,
				"used_points":    ov.LicenseUsedPoints,
			}}
			return veeamEmit(cmd, flags, ov, table, ov.Note)
		},
	}
	cmd.Flags().StringVar(&flagCompany, "company", "", "Company name or organization uid (required)")
	cmd.Flags().StringVar(&flagRPO, "rpo", "", "RPO window for the at-risk count (default 24h)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite mirror path (default: standard cache location)")
	return cmd
}
