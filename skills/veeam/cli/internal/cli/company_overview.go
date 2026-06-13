// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
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
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
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

			names := veeamCompanyNames(ctx, db)
			orgUID, companyName, matched := "", target, false
			for uid, name := range names {
				if strings.EqualFold(name, target) || strings.EqualFold(uid, target) {
					orgUID, companyName, matched = uid, name, true
					break
				}
			}
			if orgUID == "" {
				orgUID = target // treat the input as a uid
			}
			match := func(m veeamRow) bool {
				org := vstr(m, "organizationUid")
				if org == "" {
					org = vstr(m, "mappedOrganizationUid")
				}
				return strings.EqualFold(org, orgUID)
			}

			ov := companyOverview{Company: companyName, CompanyUID: orgUID, Matched: matched}
			now := time.Now()
			cutoff := now.Add(-rpo)
			servers := map[string]bool{}

			jobs, _ := veeamLoad(ctx, db,
				"infrastructure-backup-servers-jobs",
				"infrastructure-backup-agents-jobs",
				"infrastructure-backup-agents-windows-jobs",
				"infrastructure-backup-agents-linux-jobs",
				"infrastructure-backup-agents-mac-jobs",
			)
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
			ov.BackupServers = len(servers)

			agents, _ := veeamLoad(ctx, db,
				"infrastructure-backup-agents",
				"infrastructure-backup-agents-windows",
				"infrastructure-backup-agents-linux",
				"infrastructure-backup-agents-mac",
			)
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

			alarms, _ := veeamLoad(ctx, db, "alarms")
			for _, al := range alarms {
				if !strings.EqualFold(vstr(al, "object.organizationUid"), orgUID) {
					continue
				}
				ov.ActiveAlarms++
				if veeamSeverityRank(vstr(al, "lastActivation.status")) == 0 {
					ov.AlarmErrors++
				}
			}

			workloads, _ := veeamLoad(ctx, db,
				"protected-workloads-computers-managed-by-backup-server",
				"protected-workloads-computers-managed-by-console",
			)
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

			usage, _ := veeamLoad(ctx, db, "licensing-usage-organizations", "organizations-companies-usage")
			for _, u := range usage {
				if strings.EqualFold(vstr(u, "organizationUid"), orgUID) {
					ov.LicenseUsedPoints += vnum(u, "usedPoints")
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
