// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"

	"github.com/spf13/cobra"
)

type vulnTriageRow struct {
	CVEID             string  `json:"cve_id"`
	CVSS              float64 `json:"cvss_score"`
	CISAKEV           bool    `json:"cisa_kev"`
	EndpointsAffected int     `json:"endpoints_affected"`
	Priority          float64 `json:"priority"`
	RemediationStatus string  `json:"remediation_status"`
	Software          string  `json:"software"`
	OrganizationID    string  `json:"organization_id"`
}

// pp:data-source local
func newNovelFleetVulnTriageCmd(flags *rootFlags) *cobra.Command {
	var dbPath, orgFilter string
	var limit int
	var kevOnly bool
	var minCVSS float64

	cmd := &cobra.Command{
		Use:         "vuln-triage",
		Short:       "Rank CVEs across all organizations by blast radius, CVSS, and CISA KEV status.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Exploit-aware vulnerability triage across the whole fleet. Reads locally
synced vulnerabilities from every organization and ranks them by a priority
score = CVSS x affected endpoints, doubled when the CVE is in the CISA
Known-Exploited Vulnerabilities catalog. The org-scoped Action1 API lists
vulnerabilities one organization at a time and offers no cross-org ranking.`,
		Example: `  action1-cli fleet vuln-triage --kev-only --agent
  action1-cli fleet vuln-triage --min-cvss 7.0 --limit 20`,
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
			maybeEmitSyncHints(cmd, db, "vulnerabilities", flags.maxAge)

			vulns, err := fleetLoadAll(cmd.Context(), db, "vulnerabilities")
			if err != nil {
				return err
			}

			rows := make([]vulnTriageRow, 0, len(vulns))
			for _, v := range vulns {
				org := fleetOrgID(v)
				if orgFilter != "" && org != orgFilter {
					continue
				}
				kev := fleetBoolish(fleetField(v, "cisa_kev"))
				if kevOnly && !kev {
					continue
				}
				cvss, _ := fleetNum(fleetField(v, "cvss_score"))
				if cvss < minCVSS {
					continue
				}
				count := int(fleetNumField(v, "endpoints_count"))
				weight := 1.0
				if kev {
					weight = 2.0
				}
				// Priority blends severity and reach; +1 reach floor so a high-CVSS
				// CVE with an unknown endpoint count still ranks above noise.
				priority := cvss * float64(count+1) * weight
				rows = append(rows, vulnTriageRow{
					CVEID:             fleetStrField(v, "cve_id"),
					CVSS:              cvss,
					CISAKEV:           kev,
					EndpointsAffected: count,
					Priority:          priority,
					RemediationStatus: fleetStrField(v, "remediation_status"),
					Software:          vulnSoftwareName(v),
					OrganizationID:    org,
				})
			}

			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].CISAKEV != rows[j].CISAKEV {
					return rows[i].CISAKEV // KEV first
				}
				return rows[i].Priority > rows[j].Priority
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			header := []string{"CVE", "CVSS", "KEV", "ENDPOINTS", "PRIORITY", "REMEDIATION", "SOFTWARE", "ORG"}
			matrix := make([][]string, 0, len(rows))
			for _, r := range rows {
				kev := "no"
				if r.CISAKEV {
					kev = "YES"
				}
				matrix = append(matrix, []string{r.CVEID, fmtFloat(r.CVSS), kev,
					fleetItoa(float64(r.EndpointsAffected)), fmtFloat(r.Priority), r.RemediationStatus, r.Software, r.OrganizationID})
			}
			return fleetEmit(cmd, flags, rows, header, matrix)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: standard cache location)")
	cmd.Flags().StringVar(&orgFilter, "org", fleetEnvOrg(), "Limit to one organization ID (default: $ACTION1_ORG_ID)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum CVEs to return (0 = all)")
	cmd.Flags().BoolVar(&kevOnly, "kev-only", false, "Only CVEs in the CISA Known-Exploited Vulnerabilities catalog")
	cmd.Flags().Float64Var(&minCVSS, "min-cvss", 0, "Only CVEs with CVSS score >= this value")
	return cmd
}

// vulnSoftwareName extracts a readable software label from the vulnerability's
// software field, which may be a string, an object with name/product, or a list.
func vulnSoftwareName(v map[string]any) string {
	sw := fleetField(v, "software")
	switch t := sw.(type) {
	case string:
		return t
	case map[string]any:
		if n := fleetStr(t["name"]); n != "" {
			return n
		}
		if n := fleetStr(t["product"]); n != "" {
			return n
		}
	case []any:
		if len(t) > 0 {
			if m, ok := t[0].(map[string]any); ok {
				if n := fleetStr(m["name"]); n != "" {
					return n
				}
			}
			return fleetStr(t[0])
		}
	}
	return ""
}
