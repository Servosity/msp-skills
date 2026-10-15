// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: one summary row per client.
// pp:data-source local

package cli

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type portfolioRow struct {
	CompanyID          string  `json:"company_id"`
	CompanyName        string  `json:"company_name"`
	Assets             int     `json:"assets"`
	StaleAssets        int     `json:"stale_assets"`
	Critical           int     `json:"critical"`
	High               int     `json:"high"`
	MaxEPSS            float64 `json:"max_epss"`
	KEV                int     `json:"kev"`
	RiskScore          string  `json:"risk_score"`
	ActiveSuppressions int     `json:"active_suppressions"`
	OpenOver30Days     int     `json:"open_over_30_days"`
}

// buildPortfolio rolls each synced table up per company. Companies with no
// synced rows anywhere still appear, so a quiet client is visible as zeros.
func buildPortfolio(data map[string][]csRow, stale time.Duration, now time.Time) []portfolioRow {
	byID := map[string]*portfolioRow{}
	deleted := map[string]bool{}
	get := func(id, name string) *portfolioRow {
		if id == "" || deleted[id] {
			return nil
		}
		r, ok := byID[id]
		if !ok {
			r = &portfolioRow{CompanyID: id}
			byID[id] = r
		}
		if r.CompanyName == "" {
			r.CompanyName = name
		}
		return r
	}
	for _, c := range data[csResCompanies] {
		if csBool(c["is_deleted"]) {
			deleted[csString(c["id"])] = true
			continue
		}
		get(csString(c["id"]), csString(c["name"]))
	}
	for _, a := range data[csResAssets] {
		if csBool(a["is_deprecated"]) {
			continue
		}
		r := get(csString(a["company_id"]), csString(a["company_name"]))
		if r == nil {
			continue
		}
		r.Assets++
		if seen, ok := csTime(a["last_discovered_time"]); stale > 0 && (!ok || seen.Before(now.Add(-stale))) {
			r.StaleAssets++
		}
	}
	for _, p := range data[csResProblemsByCo] {
		r := get(csString(p["company_id"]), csString(p["company_name"]))
		if r == nil {
			continue
		}
		switch csSeverityRank(csString(p["severity"])) {
		case 4:
			r.Critical++
		case 3:
			r.High++
		}
		if e := csFloat0(p["epss_score"]); e > r.MaxEPSS {
			r.MaxEPSS = e
		}
	}
	kev := csKEVProblems(data[csResRemediations])
	kevSeen := map[string]bool{}
	for _, rem := range data[csResRemediations] {
		name, co := csString(rem["problem_name"]), csString(rem["company_id"])
		if !kev[name] || co == "" || kevSeen[co+"\x00"+name] {
			continue
		}
		kevSeen[co+"\x00"+name] = true
		if r := get(co, ""); r != nil {
			r.KEV++
		}
	}
	for _, s := range data[csResSuppressions] {
		if !suppressionActive(s, now) {
			continue
		}
		if r := get(csString(s["company_id"]), csString(s["company_name"])); r != nil {
			r.ActiveSuppressions++
		}
	}
	for _, v := range data[csResVelocity] {
		if r := get(csString(v["company_id"]), ""); r != nil {
			r.OpenOver30Days += int(csFloat0(v["affected_software_graterthan_30_days"]))
		}
	}
	// The risk-score report carries only company_name: a name shared by two
	// companies (or two report rows) is left blank rather than guessed.
	risk := map[string]string{}
	riskRows := map[string]int{}
	for _, rs := range data[csResRiskScore] {
		n := strings.ToLower(csString(rs["company_name"]))
		risk[n] = csString(rs["risk_score"])
		riskRows[n]++
	}
	names := map[string]int{}
	for _, r := range byID {
		names[strings.ToLower(r.CompanyName)]++
	}
	out := make([]portfolioRow, 0, len(byID))
	for _, r := range byID {
		if n := strings.ToLower(r.CompanyName); names[n] == 1 && riskRows[n] <= 1 {
			r.RiskScore = risk[n]
		}
		r.MaxEPSS = csRound(r.MaxEPSS, 4)
		out = append(out, *r)
	}
	return out
}

// suppressionActive: not unsuppressed, and either open-ended or not yet expired.
func suppressionActive(s csRow, now time.Time) bool {
	if csString(s["unsuppressed_on"]) != "" {
		return false
	}
	till, ok := csTime(s["suppressed_till"])
	return !ok || !till.Before(now)
}

// portfolioOrder holds the --sort keys; each compares a before b (negative = a first).
var portfolioOrder = map[string]func(a, b portfolioRow) int{
	"max-epss": func(a, b portfolioRow) int { return cmp.Compare(b.MaxEPSS, a.MaxEPSS) },
	"critical": func(a, b portfolioRow) int { return cmp.Compare(b.Critical, a.Critical) },
	"kev":      func(a, b portfolioRow) int { return cmp.Compare(b.KEV, a.KEV) },
	"risk":     func(a, b portfolioRow) int { return cmp.Compare(csFloat0(b.RiskScore), csFloat0(a.RiskScore)) },
	"assets":   func(a, b portfolioRow) int { return cmp.Compare(b.Assets, a.Assets) },
	"stale":    func(a, b portfolioRow) int { return cmp.Compare(b.StaleAssets, a.StaleAssets) },
	"name": func(a, b portfolioRow) int {
		return cmp.Compare(strings.ToLower(a.CompanyName), strings.ToLower(b.CompanyName))
	},
}

// sortPortfolio orders by the --sort key, then company name, then id.
func sortPortfolio(rows []portfolioRow, by string) error {
	key, ok := portfolioOrder[by]
	if !ok {
		return usageErr(fmt.Errorf("--sort must be one of max-epss, critical, kev, risk, assets, stale, name; got %q", by))
	}
	slices.SortStableFunc(rows, func(a, b portfolioRow) int {
		return cmp.Or(key(a, b), cmp.Compare(a.CompanyName, b.CompanyName), cmp.Compare(a.CompanyID, b.CompanyID))
	})
	return nil
}

func newNovelPortfolioCmd(flags *rootFlags) *cobra.Command {
	var sortBy, staleFlag, dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:   "portfolio",
		Short: "One row per client: assets, critical/high, max EPSS, KEV, risk score, stale assets, suppressions",
		Long: `One summary row per client across the whole client base, merged from the
synced assets, vulnerability, KEV, suppression, remediation-velocity and risk
score tables. Reads the local store (run 'connectsecure-cli sync' first).

Use this command for one summary row per client across the whole client base.
Do NOT use this command for one client's QBR detail; use 'qbr' instead.
Do NOT use it for asset-level vulnerability rows; use 'exposure' instead.`,
		Example: strings.Trim(`
  connectsecure-cli portfolio --sort max-epss
  connectsecure-cli portfolio --sort critical --limit 10 --agent
  connectsecure-cli portfolio --stale 14d --select company_name,critical,max_epss`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "portfolio")
			}
			stale, err := csDuration("stale", staleFlag)
			if err != nil {
				return err
			}
			if err := sortPortfolio(nil, sortBy); err != nil {
				return err
			}
			data, unsynced, err := csLocalData(cmd, flags, dbPath, csResCompanies, csResAssets, csResProblemsByCo, csResRemediations, csResSuppressions, csResVelocity, csResRiskScore)
			if err != nil {
				return err
			}
			rows := buildPortfolio(data, stale, time.Now().UTC())
			_ = sortPortfolio(rows, sortBy)
			if limit > 0 && len(rows) > limit {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: showing %d of %d clients; raise --limit (0 = all)\n", limit, len(rows))
				rows = rows[:limit]
			}
			return csEmit(cmd, flags, rows, unsynced, "No clients in the local store.")
		},
	}
	cmd.Flags().StringVar(&sortBy, "sort", "max-epss", "Sort by: max-epss, critical, kev, risk, assets, stale, name")
	cmd.Flags().StringVar(&staleFlag, "stale", "30d", "Count assets not discovered within this window as stale (0 disables)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum clients to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the synced store)")
	return cmd
}
