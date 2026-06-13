// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// fleetHealthCompany is one tenant's rolled-up backup posture.
type fleetHealthCompany struct {
	Company       string `json:"company"`
	CompanyUID    string `json:"company_uid,omitempty"`
	JobsTotal     int    `json:"jobs_total"`
	Success       int    `json:"success"`
	Warning       int    `json:"warning"`
	Failed        int    `json:"failed"`
	Running       int    `json:"running"`
	OtherStatus   int    `json:"other_status"`
	AgentsOnline  int    `json:"agents_online"`
	AgentsOffline int    `json:"agents_offline"`
	ActiveAlarms  int    `json:"active_alarms"`
	AlarmErrors   int    `json:"alarm_errors"`
}

type fleetHealthView struct {
	Companies []fleetHealthCompany `json:"companies"`
	Totals    fleetHealthCompany   `json:"totals"`
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var problemsOnly bool

	cmd := &cobra.Command{
		Use:   "fleet-health",
		Short: "One-pane cross-company backup health: jobs by last status, agents online/offline, and active alarms per tenant.",
		Long: strings.Trim(`
Roll up every tenant's backup posture from the local mirror: per company, the
count of jobs by last status (success/warning/failed/running), backup agents
online vs. offline, and active alarms (with error-severity called out).

Reads only the local SQLite mirror — run `+"`veeam-cli sync`"+` first. Use
--problems-only to hide tenants that are fully green.`, "\n"),
		Example: strings.Trim(`
  veeam-cli fleet-health
  veeam-cli fleet-health --problems-only
  veeam-cli fleet-health --agent --select companies.company,companies.failed,companies.agents_offline`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
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
			buckets := map[string]*fleetHealthCompany{}
			order := []string{}
			get := func(uid string) *fleetHealthCompany {
				key := strings.ToLower(uid)
				b, ok := buckets[key]
				if !ok {
					b = &fleetHealthCompany{Company: veeamCompanyLabel(names, uid), CompanyUID: uid}
					buckets[key] = b
					order = append(order, key)
				}
				return b
			}
			// Seed buckets from known companies so fully-quiet tenants appear.
			for uid, name := range names {
				get(uid).Company = name
			}

			jobs, _ := veeamLoad(ctx, db,
				"infrastructure-backup-servers-jobs",
				"infrastructure-backup-agents-jobs",
				"infrastructure-backup-agents-windows-jobs",
				"infrastructure-backup-agents-linux-jobs",
				"infrastructure-backup-agents-mac-jobs",
			)
			for _, j := range jobs {
				org := vstr(j, "organizationUid")
				if org == "" {
					org = vstr(j, "mappedOrganizationUid")
				}
				b := get(org)
				b.JobsTotal++
				switch veeamJobHealth(vstr(j, "status")) {
				case "success":
					b.Success++
				case "warning":
					b.Warning++
				case "failed":
					b.Failed++
				case "running":
					b.Running++
				default:
					b.OtherStatus++
				}
			}

			agents, _ := veeamLoad(ctx, db,
				"infrastructure-backup-agents",
				"infrastructure-backup-agents-windows",
				"infrastructure-backup-agents-linux",
				"infrastructure-backup-agents-mac",
			)
			for _, a := range agents {
				b := get(vstr(a, "organizationUid"))
				if veeamAgentOnline(vstr(a, "status")) {
					b.AgentsOnline++
				} else {
					b.AgentsOffline++
				}
			}

			alarms, _ := veeamLoad(ctx, db, "alarms")
			for _, al := range alarms {
				b := get(vstr(al, "object.organizationUid"))
				b.ActiveAlarms++
				if veeamSeverityRank(vstr(al, "lastActivation.status")) == 0 {
					b.AlarmErrors++
				}
			}

			view := fleetHealthView{Companies: make([]fleetHealthCompany, 0, len(order))}
			for _, key := range order {
				b := buckets[key]
				if problemsOnly && b.Failed == 0 && b.Warning == 0 && b.AgentsOffline == 0 && b.AlarmErrors == 0 {
					continue
				}
				view.Companies = append(view.Companies, *b)
				view.Totals.JobsTotal += b.JobsTotal
				view.Totals.Success += b.Success
				view.Totals.Warning += b.Warning
				view.Totals.Failed += b.Failed
				view.Totals.Running += b.Running
				view.Totals.OtherStatus += b.OtherStatus
				view.Totals.AgentsOnline += b.AgentsOnline
				view.Totals.AgentsOffline += b.AgentsOffline
				view.Totals.ActiveAlarms += b.ActiveAlarms
				view.Totals.AlarmErrors += b.AlarmErrors
			}
			view.Totals.Company = "TOTAL"
			// Worst tenants first: failures, then offline agents, then warnings.
			sort.SliceStable(view.Companies, func(i, j int) bool {
				a, b := view.Companies[i], view.Companies[j]
				if a.Failed != b.Failed {
					return a.Failed > b.Failed
				}
				if a.AgentsOffline != b.AgentsOffline {
					return a.AgentsOffline > b.AgentsOffline
				}
				if a.Warning != b.Warning {
					return a.Warning > b.Warning
				}
				return strings.ToLower(a.Company) < strings.ToLower(b.Company)
			})

			table := make([]map[string]any, 0, len(view.Companies))
			for _, c := range view.Companies {
				table = append(table, map[string]any{
					"company":        c.Company,
					"jobs":           c.JobsTotal,
					"failed":         c.Failed,
					"warning":        c.Warning,
					"running":        c.Running,
					"agents_offline": c.AgentsOffline,
					"active_alarms":  c.ActiveAlarms,
				})
			}
			return veeamEmit(cmd, flags, view, table, "No tenants in the local mirror. Run `veeam-cli sync` first.")
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite mirror path (default: standard cache location)")
	cmd.Flags().BoolVar(&problemsOnly, "problems-only", false, "Hide tenants with no failures, warnings, offline agents, or error alarms")
	return cmd
}
