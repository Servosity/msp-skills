// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type healthScoreRow struct {
	OrganizationID  string  `json:"organization_id"`
	EndpointID      string  `json:"endpoint_id"`
	Name            string  `json:"name"`
	OS              string  `json:"os"`
	RiskScore       float64 `json:"risk_score"`
	Health          int     `json:"health"`
	CriticalUpdates int     `json:"critical_updates"`
	OtherUpdates    int     `json:"other_updates"`
	CriticalVulns   int     `json:"critical_vulns"`
	RebootRequired  bool    `json:"reboot_required"`
	DaysSinceSeen   float64 `json:"days_since_seen"`
}

// pp:data-source local
func newNovelFleetHealthScoreCmd(flags *rootFlags) *cobra.Command {
	var dbPath, orgFilter string
	var limit int

	cmd := &cobra.Command{
		Use:         "health-score",
		Short:       "Rank endpoints fleet-wide by a composite risk/health score.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Per-endpoint health score across the whole fleet. Composes locally synced
signals — missing critical/other updates, open critical vulnerabilities,
reboot-required, and days since last check-in — into one risk score (higher is
worse) and a 0-100 health value. No single Action1 API call yields this; it is
a local join across signals that only exist together in the store.`,
		Example: `  action1-cli fleet health-score --agent --limit 50
  action1-cli fleet health-score --org <org-uuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := fleetOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "endpoints", flags.maxAge)

			endpoints, err := fleetLoadAll(cmd.Context(), db, "endpoints")
			if err != nil {
				return err
			}

			now := time.Now()
			rows := make([]healthScoreRow, 0, len(endpoints))
			for _, e := range endpoints {
				org := fleetOrgID(e)
				if orgFilter != "" && org != orgFilter {
					continue
				}
				var crit, other, critVuln int
				if mu := fleetNested(e, "missing_updates"); mu != nil {
					c, _ := fleetNum(mu["critical"])
					o, _ := fleetNum(mu["other"])
					crit, other = int(c), int(o)
				}
				if vu := fleetNested(e, "vulnerabilities"); vu != nil {
					c, _ := fleetNum(vu["critical"])
					critVuln = int(c)
				}
				reboot := fleetBoolish(fleetField(e, "reboot_required"))

				daysSince := -1.0
				if t, ok := fleetParseTime(fleetStrField(e, "last_seen")); ok {
					daysSince = round1(now.Sub(t).Hours() / 24)
				}

				// Weighted risk: critical updates and vulns dominate; other
				// updates, reboot, and staleness add smaller penalties.
				risk := float64(crit)*10 + float64(critVuln)*12 + float64(other)*1
				if reboot {
					risk += 5
				}
				if daysSince > 7 {
					risk += daysSince // older agents add proportional risk
				}
				health := 100 - int(risk)
				if health < 0 {
					health = 0
				}
				name := fleetStrField(e, "name")
				if name == "" {
					name = fleetStrField(e, "device_name")
				}
				rows = append(rows, healthScoreRow{
					OrganizationID:  org,
					EndpointID:      fleetStrField(e, "id"),
					Name:            name,
					OS:              fleetStrField(e, "OS"),
					RiskScore:       round1(risk),
					Health:          health,
					CriticalUpdates: crit,
					OtherUpdates:    other,
					CriticalVulns:   critVuln,
					RebootRequired:  reboot,
					DaysSinceSeen:   daysSince,
				})
			}

			sort.SliceStable(rows, func(i, j int) bool {
				return rows[i].RiskScore > rows[j].RiskScore
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			header := []string{"ORG", "ENDPOINT", "NAME", "OS", "RISK", "HEALTH", "CRIT_UPD", "CRIT_VULN", "REBOOT", "DAYS"}
			matrix := make([][]string, 0, len(rows))
			for _, r := range rows {
				reboot := "no"
				if r.RebootRequired {
					reboot = "yes"
				}
				matrix = append(matrix, []string{r.OrganizationID, r.EndpointID, r.Name, r.OS,
					fmtFloat(r.RiskScore), fleetItoa(float64(r.Health)),
					fleetItoa(float64(r.CriticalUpdates)), fleetItoa(float64(r.CriticalVulns)),
					reboot, fmtFloat(r.DaysSinceSeen)})
			}
			return fleetEmit(cmd, flags, rows, header, matrix)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: standard cache location)")
	cmd.Flags().StringVar(&orgFilter, "org", fleetEnvOrg(), "Limit to one organization ID (default: $ACTION1_ORG_ID)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum endpoints to return (0 = all)")
	return cmd
}
