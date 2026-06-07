// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type orgScorecardRow struct {
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Endpoints      int    `json:"endpoints"`
	MissingCrit    int    `json:"missing_critical_updates"`
	MissingOther   int    `json:"missing_other_updates"`
	OpenCVEs       int    `json:"open_cves"`
	KEVCVEs        int    `json:"kev_cves"`
	StaleEndpoints int    `json:"stale_endpoints"`
	RebootPending  int    `json:"reboot_pending"`
}

// pp:data-source local
func newNovelFleetOrgScorecardCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var staleDays int

	cmd := &cobra.Command{
		Use:         "org-scorecard",
		Short:       "One posture row per client organization: endpoints, missing updates, CVEs, KEV exposure, stale agents.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Per-client posture scorecard across the whole book of business. Groups the
locally synced endpoints, missing updates, and vulnerabilities by organization
into the per-client posture number MSPs report to account managers. The
org-scoped Action1 API has no cross-organization rollup.

This command always spans every synced organization by design — it intentionally
ignores --org and $ACTION1_ORG_ID, which the per-endpoint fleet siblings honor.

Use this command for a per-client posture summary across every organization.
Do NOT use it for per-endpoint ranking; use 'fleet patch-posture' or
'fleet health-score' instead.`,
		Example: `  action1-cli fleet org-scorecard --agent
  action1-cli fleet org-scorecard --stale-days 30`,
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
			// The scorecard's posture columns come from endpoints (counts,
			// missing updates, stale, reboot) and vulnerabilities (CVEs, KEV);
			// organizations only supplies the name lookup. Key the sync hint on
			// the posture data this command actually reports, matching the
			// sibling fleet commands, so a fresh/partial sync gets guidance
			// instead of a silently empty result. Check both posture resources
			// but emit at most one hint: an unsynced-store hint on endpoints
			// covers vulnerabilities too, so only fall through to the second
			// resource when endpoints is already synced.
			if !hintIfUnsynced(cmd, db, "endpoints") && !hintIfStale(cmd, db, "endpoints", flags.maxAge) {
				if !hintIfUnsynced(cmd, db, "vulnerabilities") {
					hintIfStale(cmd, db, "vulnerabilities", flags.maxAge)
				}
			}

			orgs, err := fleetLoadAll(cmd.Context(), db, "organizations")
			if err != nil {
				return err
			}
			endpoints, err := fleetLoadAll(cmd.Context(), db, "endpoints")
			if err != nil {
				return err
			}
			vulns, err := fleetLoadAll(cmd.Context(), db, "vulnerabilities")
			if err != nil {
				return err
			}

			names := make(map[string]string, len(orgs))
			for _, o := range orgs {
				id := fleetStrField(o, "id")
				if id == "" {
					continue
				}
				names[id] = fleetStrField(o, "name")
			}

			byOrg := make(map[string]*orgScorecardRow)
			rowFor := func(org string) *orgScorecardRow {
				r, ok := byOrg[org]
				if !ok {
					r = &orgScorecardRow{OrganizationID: org, Name: names[org]}
					byOrg[org] = r
				}
				return r
			}

			now := time.Now()
			staleCutoff := time.Duration(staleDays) * 24 * time.Hour
			for _, e := range endpoints {
				r := rowFor(fleetOrgID(e))
				r.Endpoints++
				if mu := fleetNested(e, "missing_updates"); mu != nil {
					c, _ := fleetNum(mu["critical"])
					o, _ := fleetNum(mu["other"])
					r.MissingCrit += int(c)
					r.MissingOther += int(o)
				}
				if fleetBoolish(fleetField(e, "reboot_required")) {
					r.RebootPending++
				}
				if t, ok := fleetParseTime(fleetStrField(e, "last_seen")); ok && now.Sub(t) >= staleCutoff {
					r.StaleEndpoints++
				}
			}
			for _, v := range vulns {
				r := rowFor(fleetOrgID(v))
				r.OpenCVEs++
				if fleetBoolish(fleetField(v, "cisa_kev")) {
					r.KEVCVEs++
				}
			}

			rows := make([]orgScorecardRow, 0, len(byOrg))
			for _, r := range byOrg {
				rows = append(rows, *r)
			}
			// Worst posture first: KEV exposure, then critical updates, then stale.
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].KEVCVEs != rows[j].KEVCVEs {
					return rows[i].KEVCVEs > rows[j].KEVCVEs
				}
				if rows[i].MissingCrit != rows[j].MissingCrit {
					return rows[i].MissingCrit > rows[j].MissingCrit
				}
				return rows[i].StaleEndpoints > rows[j].StaleEndpoints
			})

			header := []string{"ORG", "NAME", "ENDPOINTS", "CRIT_UPDATES", "OTHER_UPDATES", "CVES", "KEV", "STALE", "REBOOT"}
			matrix := make([][]string, 0, len(rows))
			for _, r := range rows {
				matrix = append(matrix, []string{r.OrganizationID, r.Name,
					fleetItoa(float64(r.Endpoints)), fleetItoa(float64(r.MissingCrit)), fleetItoa(float64(r.MissingOther)),
					fleetItoa(float64(r.OpenCVEs)), fleetItoa(float64(r.KEVCVEs)), fleetItoa(float64(r.StaleEndpoints)), fleetItoa(float64(r.RebootPending))})
			}
			return fleetEmit(cmd, flags, rows, header, matrix)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: standard cache location)")
	cmd.Flags().IntVar(&staleDays, "stale-days", 14, "Days without check-in before an endpoint counts as stale")
	return cmd
}
