// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: one client's QBR risk pack.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"connectsecure-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

type qbrFix struct {
	Fix              string  `json:"fix"`
	Product          string  `json:"product"`
	Severity         string  `json:"severity"`
	EPSSVulns        float64 `json:"epss_vuls"`
	CriticalProblems int     `json:"critical_problems"`
	HighProblems     int     `json:"high_problems"`
	AffectedAssets   int     `json:"affected_assets"`
	Patchable        bool    `json:"is_patchable"`
}

type qbrAge struct {
	Severity   string `json:"severity"`
	Under7Days int    `json:"under_7_days"`
	Days8To15  int    `json:"days_8_15"`
	Days16To30 int    `json:"days_16_30"`
	Over30Days int    `json:"over_30_days"`
	Total      int    `json:"total"`
}

type qbrRemediated struct {
	Window               string  `json:"window"`
	Remediations         int     `json:"remediations"`
	ProblemsResolved     int     `json:"problems_resolved"`
	MedianResolutionDays float64 `json:"median_resolution_days"`
	Scanned              int     `json:"scanned_records"`
	Truncated            bool    `json:"truncated"`
}

type qbrPack struct {
	CompanyID     string           `json:"company_id"`
	CompanyName   string           `json:"company_name"`
	GeneratedAt   string           `json:"generated_at"`
	RiskScore     *string          `json:"risk_score"`
	Stats         csRow            `json:"stats"`
	OpenByAge     []qbrAge         `json:"open_by_age"`
	TopFixes      []qbrFix         `json:"top_fixes"`
	Remediated    *qbrRemediated   `json:"remediated"`
	Compliance    any              `json:"compliance"`
	Truncated     []string         `json:"truncated_sections"`
	FetchFailures []csFetchFailure `json:"fetch_failures"`
}

type qbrOpts struct {
	since                             time.Duration
	sinceLabel                        string
	top                               int
	complianceType, platform, section string
	now                               time.Time
}

// qbrPageSize and qbrMaxPages bound each section's read; a section that hits
// the cap is listed in truncated_sections instead of silently ranking a subset.
const (
	qbrPageSize = 100
	qbrMaxPages = 10
)

// resolveQBRCompany finds one company by id or by name (exact match preferred).
func resolveQBRCompany(ctx context.Context, c *client.Client, name, id string) (string, string, error) {
	if id != "" {
		for _, r := range id {
			if r < '0' || r > '9' {
				return "", "", usageErr(fmt.Errorf("--company-id must be a numeric ConnectSecure company id, got %q", id))
			}
		}
		data, err := csGetData(ctx, c, "/r/company/companies/"+id, nil)
		if err != nil {
			return "", "", classifyAPIErrorOnly(err)
		}
		row := csFirstRow(data)
		if row == nil {
			return "", "", notFoundErr(fmt.Errorf("company id %s not found", id))
		}
		return csString(row["id"]), csString(row["name"]), nil
	}
	// Exact (case-insensitive) name first, so a common substring cannot crowd
	// the real company out of a capped substring page.
	exact, _, err := csGetRows(ctx, c, "/r/company/companies", map[string]string{"condition": "name ILIKE " + csQuote(name)}, 10, 1)
	if err != nil {
		return "", "", classifyAPIErrorOnly(err)
	}
	var same []csRow
	for _, r := range exact {
		if strings.EqualFold(csString(r["name"]), name) {
			same = append(same, r)
		}
	}
	if len(same) == 1 {
		return csString(same[0]["id"]), csString(same[0]["name"]), nil
	}
	if len(same) > 1 {
		ids := make([]string, 0, len(same))
		for _, r := range same {
			ids = append(ids, fmt.Sprintf("%s (id %s)", csString(r["name"]), csString(r["id"])))
		}
		return "", "", usageErr(fmt.Errorf("%d companies are named %q: %s; pass --company-id", len(same), name, strings.Join(ids, ", ")))
	}
	matches, capped, err := csGetRows(ctx, c, "/r/company/companies", map[string]string{"condition": "name ILIKE " + csQuote("%"+name+"%")}, qbrPageSize, 1)
	if err != nil {
		return "", "", classifyAPIErrorOnly(err)
	}
	switch len(matches) {
	case 0:
		return "", "", notFoundErr(fmt.Errorf("no company matches %q; list them with: connectsecure-cli companies list --condition \"name ILIKE '%%%s%%'\"", name, name))
	case 1:
		// Do not trust the server to have applied the condition.
		if strings.Contains(strings.ToLower(csString(matches[0]["name"])), strings.ToLower(name)) {
			return csString(matches[0]["id"]), csString(matches[0]["name"]), nil
		}
		return "", "", notFoundErr(fmt.Errorf("no company matches %q; list them with: connectsecure-cli companies list --condition \"name ILIKE '%%%s%%'\"", name, name))
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, fmt.Sprintf("%s (id %s)", csString(m["name"]), csString(m["id"])))
	}
	more := ""
	if capped {
		more = fmt.Sprintf(" (first %d shown)", qbrPageSize)
	}
	return "", "", usageErr(fmt.Errorf("%q matches %d companies%s: %s; pass the exact name or --company-id", name, len(matches), more, strings.Join(names, ", ")))
}

func csFirstRow(data any) csRow {
	switch t := data.(type) {
	case map[string]any:
		if len(t) == 0 {
			return nil
		}
		return t
	case []any:
		if len(t) > 0 {
			if m, ok := t[0].(map[string]any); ok {
				return m
			}
		}
	}
	return nil
}

// buildQBR composes the pack. It returns the first section error only when
// every section failed, so a single broken report does not sink the pack.
func buildQBR(ctx context.Context, c *client.Client, id, name string, o qbrOpts) (qbrPack, error) {
	pack := qbrPack{CompanyID: id, CompanyName: name, GeneratedAt: o.now.Format(time.RFC3339),
		Truncated: make([]string, 0), FetchFailures: make([]csFetchFailure, 0)}
	// A section that fails stays nil (JSON null, "unavailable" for humans) so a
	// failed read can never be mistaken for a measured zero.
	var firstErr error
	sections := 0
	fetch := func(section, path string, params map[string]string, maxPages int) (rows []csRow, capped, ok bool) {
		sections++
		rows, capped, err := csGetRows(ctx, c, path, params, qbrPageSize, maxPages)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			pack.FetchFailures = append(pack.FetchFailures, csFetchFailure{Section: section, Error: err.Error()})
			return nil, false, false
		}
		if capped {
			pack.Truncated = append(pack.Truncated, section)
		}
		return rows, capped, true
	}
	byCompany := map[string]string{"condition": "company_id=" + id}

	if rows, _, ok := fetch("risk_score", "/r/report_queries/risk_score", map[string]string{"condition": "company_name=" + csQuote(name)}, 1); ok {
		// The risk-score report carries only company_name, so a name two
		// clients share cannot be attributed (even when the report merged
		// them into one row); say so instead of picking one.
		owners, _, ownersOK := fetch("risk_score", "/r/company/companies", map[string]string{"condition": "name ILIKE " + csQuote(name)}, 1)
		shared := 0
		for _, o := range owners {
			if strings.EqualFold(csString(o["name"]), name) {
				shared++
			}
		}
		switch {
		case !ownersOK:
			// recorded by fetch; an unchecked name-keyed score stays unset
		case len(rows) > 1 || shared > 1:
			pack.FetchFailures = append(pack.FetchFailures, csFetchFailure{Section: "risk_score", Error: fmt.Sprintf("%d companies (%d risk-score rows) are named %q and the report carries no company ID, so the score cannot be attributed", shared, len(rows), name)})
		default:
			score := ""
			if len(rows) > 0 {
				score = csString(rows[0]["risk_score"])
			}
			pack.RiskScore = &score
		}
	}

	if rows, _, ok := fetch("company_stats", "/r/company/company_stats", byCompany, qbrMaxPages); ok {
		var latest time.Time
		for _, r := range rows {
			t, _ := csTime(r["date"])
			if pack.Stats == nil || t.After(latest) {
				pack.Stats, latest = r, t
			}
		}
	}

	if rows, _, ok := fetch("remediation_velocity_company", "/r/report_queries/remediation_velocity_company", byCompany, qbrMaxPages); ok {
		pack.OpenByAge = make([]qbrAge, 0, len(rows))
		for _, r := range rows {
			pack.OpenByAge = append(pack.OpenByAge, qbrAge{
				Severity: csString(r["severity"]), Under7Days: int(csFloat0(r["affected_software_lessthan_7_days"])),
				Days8To15: int(csFloat0(r["affected_software_8_15_days"])), Days16To30: int(csFloat0(r["affected_software_16_30_days"])),
				Over30Days: int(csFloat0(r["affected_software_graterthan_30_days"])), Total: int(csFloat0(r["total_count"])),
			})
		}
		sort.SliceStable(pack.OpenByAge, func(i, j int) bool {
			return csSeverityRank(pack.OpenByAge[i].Severity) > csSeverityRank(pack.OpenByAge[j].Severity)
		})
	}

	if rows, _, ok := fetch("remediation_plan_by_company", "/r/report_queries/remediation_plan_by_company", map[string]string{"company_id": id}, qbrMaxPages); ok {
		pack.TopFixes = make([]qbrFix, 0, len(rows))
		for _, r := range rows {
			pack.TopFixes = append(pack.TopFixes, qbrFix{
				Fix: csString(r["fix"]), Product: csString(r["product"]), Severity: csString(r["severity"]),
				EPSSVulns: csFloat0(r["epss_vuls"]), CriticalProblems: int(csFloat0(r["critical_problems"])),
				HighProblems: int(csFloat0(r["high_problems"])), AffectedAssets: int(csFloat0(r["affected_assets"])),
				Patchable: csBool(r["is_patchable"]),
			})
		}
		sort.SliceStable(pack.TopFixes, func(i, j int) bool {
			a, b := pack.TopFixes[i], pack.TopFixes[j]
			if a.EPSSVulns != b.EPSSVulns {
				return a.EPSSVulns > b.EPSSVulns
			}
			return a.CriticalProblems > b.CriticalProblems
		})
		if o.top > 0 && len(pack.TopFixes) > o.top {
			pack.TopFixes = pack.TopFixes[:o.top]
		}
	}

	if rows, capped, ok := fetch("resolved_remediation", "/r/report_queries/resolved_remediation", map[string]string{"company_id": id}, qbrMaxPages); ok {
		pack.Remediated = &qbrRemediated{Window: o.sinceLabel, Scanned: len(rows), Truncated: capped}
		days := make([]float64, 0)
		for _, r := range rows {
			if o.since > 0 {
				if t, ok := csTime(r["remediated_on"]); !ok || t.Before(o.now.Add(-o.since)) {
					continue
				}
			}
			// Each row is one remediation (a fix applied to an asset) that can
			// close several findings; total_problems carries that count.
			pack.Remediated.Remediations++
			pack.Remediated.ProblemsResolved += int(csFloat0(r["total_problems"]))
			if d, ok := csFloat(r["resolution_days"]); ok {
				days = append(days, d)
			}
		}
		pack.Remediated.MedianResolutionDays = csRound(csMedian(days), 1)
	}

	if o.complianceType == "" || o.platform == "" || o.section == "" {
		pack.Compliance = map[string]string{"skipped": "pass --compliance-type, --compliance-platform and --compliance-section (list types with: connectsecure-cli compliance)"}
	} else if rows, _, ok := fetch("compliance_check_company_count", "/r/report_queries/compliance_check_company_count",
		map[string]string{"condition": "company_id=" + id, "compliance_type": o.complianceType, "platform": o.platform, "section": o.section}, qbrMaxPages); ok {
		// Rows are per check x status buckets carrying asset_count, so a check
		// with 99 passing assets and 1 failing one arrives as two rows. A check
		// passes only when none of its assets fail (Codex round 21).
		failing := map[string]bool{}
		var checks []string
		assetsPass, assetsNotPass := 0.0, 0.0
		for i, r := range rows {
			check := csString(r["compliance_id"])
			if check == "" {
				check = fmt.Sprintf("row-%d", i)
			}
			if _, seen := failing[check]; !seen {
				failing[check] = false
				checks = append(checks, check)
			}
			n := 1.0 // a bucket that omits asset_count still counts
			if v, ok := csFloat(r["asset_count"]); ok {
				n = v
			}
			if st := strings.ToLower(csString(r["compliance_status"])); st == "pass" || st == "passed" {
				assetsPass += n
			} else {
				assetsNotPass += n
				if n > 0 {
					failing[check] = true
				}
			}
		}
		notPass := 0
		for _, c := range checks {
			if failing[c] {
				notPass++
			}
		}
		pack.Compliance = map[string]any{"checks": len(checks), "pass": len(checks) - notPass, "not_pass": notPass, "assets_pass": assetsPass, "assets_not_pass": assetsNotPass, "compliance_type": o.complianceType}
	}
	if firstErr != nil && len(pack.FetchFailures) == sections {
		return pack, classifyAPIErrorOnly(firstErr)
	}
	return pack, nil
}

func newNovelQbrCmd(flags *rootFlags) *cobra.Command {
	var companyID, sinceFlag, complianceType, platform, section string
	var top int
	cmd := &cobra.Command{
		Use:   "qbr [company]",
		Short: "One client's QBR numbers: risk score, totals, top fixes, open-by-age, remediation speed",
		Long: `Build one client's quarterly business review risk pack from live ConnectSecure
data: risk score, latest company stats, open findings by age and severity, the
top fixes by EPSS-weighted vulnerabilities, and the remediations completed in
--since with the findings they closed and the median resolution days. Compliance pass/fail is added only
when --compliance-type, --compliance-platform and --compliance-section are all given.

Sections that fail are listed in fetch_failures and sections that hit the read
cap are listed in truncated_sections, instead of being dropped silently.

Use this command to build one client's risk report for a QBR.
Do NOT use this command to compare or rank all clients; use 'portfolio' instead.`,
		Example: strings.Trim(`
  connectsecure-cli qbr "Acme Dental" --since 90d --agent
  connectsecure-cli qbr --company-id 12 --top 10`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "company=Acme Dental"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "qbr")
			}
			name := strings.TrimSpace(strings.Join(args, " "))
			if name == "" && companyID == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass a company name or --company-id"))
			}
			if top < 0 {
				return usageErr(fmt.Errorf("--top must be >= 0"))
			}
			since, err := csDuration("since", sinceFlag)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			id, resolved, err := resolveQBRCompany(ctx, c, name, companyID)
			if err != nil {
				return err
			}
			pack, err := buildQBR(ctx, c, id, resolved, qbrOpts{since: since, sinceLabel: sinceFlag, top: top,
				complianceType: complianceType, platform: platform, section: section, now: time.Now().UTC()})
			if err != nil {
				return err
			}
			if len(pack.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d section(s) failed; see fetch_failures\n", len(pack.FetchFailures))
			}
			if len(pack.Truncated) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: read cap hit for %s; see truncated_sections\n", strings.Join(pack.Truncated, ", "))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), pack, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "QBR risk pack: %s (id %s)\n", pack.CompanyName, pack.CompanyID)
			if pack.RiskScore != nil {
				fmt.Fprintf(w, "  Risk score: %s\n", *pack.RiskScore)
			} else {
				fmt.Fprintln(w, "  Risk score: unavailable")
			}
			if r := pack.Remediated; r != nil {
				fmt.Fprintf(w, "  Remediated in %s: %d remediations closing %d findings (median %.1f days)\n", r.Window, r.Remediations, r.ProblemsResolved, r.MedianResolutionDays)
			} else {
				fmt.Fprintln(w, "  Remediated: unavailable")
			}
			if pack.OpenByAge == nil {
				fmt.Fprintln(w, "  Open findings by age: unavailable")
			}
			if pack.TopFixes == nil {
				fmt.Fprintln(w, "  Top fixes: unavailable")
			}
			for _, a := range pack.OpenByAge {
				fmt.Fprintf(w, "  Open %-8s <7d %d  8-15d %d  16-30d %d  >30d %d\n", a.Severity, a.Under7Days, a.Days8To15, a.Days16To30, a.Over30Days)
			}
			for i, f := range pack.TopFixes {
				fmt.Fprintf(w, "  Fix %d: %s (%s) EPSS-weighted %.2f, %d assets\n", i+1, f.Fix, f.Product, f.EPSSVulns, f.AffectedAssets)
			}
			for _, ff := range pack.FetchFailures {
				fmt.Fprintf(w, "  (failed: %s: %s)\n", ff.Section, ff.Error)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company-id", "", "ConnectSecure company ID (instead of a name)")
	cmd.Flags().StringVar(&sinceFlag, "since", "90d", "Remediation window for the remediated count (e.g. 30d, 90d; 0 = all time)")
	cmd.Flags().IntVar(&top, "top", 5, "How many top fixes to include")
	cmd.Flags().StringVar(&complianceType, "compliance-type", "", "Compliance framework to summarize (list types with: connectsecure-cli compliance)")
	cmd.Flags().StringVar(&platform, "compliance-platform", "", "Compliance platform filter, required with --compliance-type")
	cmd.Flags().StringVar(&section, "compliance-section", "", "Compliance section filter, required with --compliance-type")
	return cmd
}
