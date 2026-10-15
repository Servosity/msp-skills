// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: cross-client exposure matrix.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type exposureRow struct {
	ProblemName        string  `json:"problem_name"`
	Severity           string  `json:"severity"`
	EPSSScore          float64 `json:"epss_score"`
	BaseScore          float64 `json:"base_score"`
	KEV                bool    `json:"kev"`
	CompanyID          string  `json:"company_id"`
	CompanyName        string  `json:"company_name"`
	AssetID            string  `json:"asset_id"`
	AssetName          string  `json:"asset_name"`
	IP                 string  `json:"ip"`
	Importance         string  `json:"importance"`
	AuthScanStatus     string  `json:"auth_scan_status"`
	LastDiscoveredTime string  `json:"last_discovered_time"`
	FirstVulDiscovered string  `json:"first_vul_discovered"`
	AssetSynced        bool    `json:"asset_synced"`
}

type exposureOpts struct {
	minEPSS    float64
	severities map[string]bool
	newSince   time.Duration
	kevOnly    bool
	company    string
	now        time.Time
}

// buildExposure fans each synced problem x company row out to its assets.
// Pure over decoded rows so it is testable without a store.
func buildExposure(data map[string][]csRow, o exposureOpts) []exposureRow {
	names := csCompanyNames(data[csResCompanies])
	kev := csKEVProblems(data[csResRemediations])
	assets := csIndex(data[csResAssets], "id")
	out := make([]exposureRow, 0)
	for _, p := range data[csResProblemsByCo] {
		epss := csFloat0(p["epss_score"])
		if epss < o.minEPSS {
			continue
		}
		sev := csString(p["severity"])
		if len(o.severities) > 0 && !o.severities[strings.ToLower(sev)] {
			continue
		}
		name := csString(p["problem_name"])
		isKEV := kev[name]
		if o.kevOnly && !isKEV {
			continue
		}
		if o.newSince > 0 {
			first, ok := csTime(p["first_vul_discovered"])
			if !ok || first.Before(o.now.Add(-o.newSince)) {
				continue
			}
		}
		coID := csString(p["company_id"])
		coName := csString(p["company_name"])
		if coName == "" {
			coName = names[coID]
		}
		if !csCompanyMatch(o.company, coID, coName) {
			continue
		}
		base := exposureRow{
			ProblemName: name, Severity: sev, EPSSScore: epss, BaseScore: csFloat0(p["base_score"]),
			KEV: isKEV, CompanyID: coID, CompanyName: coName, FirstVulDiscovered: csString(p["first_vul_discovered"]),
		}
		ids := csStringList(p["asset_ids"])
		if len(ids) == 0 {
			out = append(out, base)
			continue
		}
		for _, id := range ids {
			row := base
			row.AssetID = id
			if a, ok := assets[id]; ok {
				row.AssetSynced = true
				row.AssetName = csAssetName(a)
				row.IP = csString(a["ip"])
				row.Importance = csString(a["importance"])
				row.AuthScanStatus = csString(a["auth_scan_status"])
				row.LastDiscoveredTime = csString(a["last_discovered_time"])
			}
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.EPSSScore != b.EPSSScore {
			return a.EPSSScore > b.EPSSScore
		}
		if a.BaseScore != b.BaseScore {
			return a.BaseScore > b.BaseScore
		}
		if a.KEV != b.KEV {
			return a.KEV
		}
		if ra, rb := csSeverityRank(a.Severity), csSeverityRank(b.Severity); ra != rb {
			return ra > rb
		}
		if a.CompanyName != b.CompanyName {
			return a.CompanyName < b.CompanyName
		}
		return a.AssetName < b.AssetName
	})
	return out
}

func newNovelExposureCmd(flags *rootFlags) *cobra.Command {
	var minEPSS float64
	var severity, newSince, company, dbPath string
	var kevOnly bool
	var limit int

	cmd := &cobra.Command{
		Use:   "exposure",
		Short: "Rank open vulnerabilities across all clients at asset level (problem x asset x client)",
		Long: `Rank open vulnerability findings across every client at asset level: one row per
problem x asset x client, sorted by EPSS, then CVSS base score, then KEV.

Reads the local store (run 'connectsecure-cli sync' first). ConnectSecure's
per-client vulnerability report lists affected assets only by ID; this command
joins each one to the synced asset (host name, IP, scan status) and fills in a
missing client name from the synced companies.

Use this command to rank open vulnerability findings across all clients at asset
level, filtered by EPSS, severity, KEV or first-discovered window.
Do NOT use this command for a single named CVE; use 'cve-impact' instead.
Do NOT use it for one summary row per client; use 'portfolio' instead.`,
		Example: strings.Trim(`
  connectsecure-cli exposure --min-epss 0.5 --severity Critical,High --agent
  connectsecure-cli exposure --kev --new-since 7d
  connectsecure-cli exposure --company "Acme Dental" --limit 20 --select problem_name,asset_name,epss_score`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "exposure")
			}
			if minEPSS < 0 || minEPSS > 1 {
				return usageErr(fmt.Errorf("--min-epss must be between 0 and 1 (EPSS is a probability), got %v", minEPSS))
			}
			severities, err := csSeveritySet(severity)
			if err != nil {
				return err
			}
			window, err := csDuration("new-since", newSince)
			if err != nil {
				return err
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be >= 0"))
			}
			data, unsynced, err := csLocalData(cmd, flags, dbPath, csResProblemsByCo, csResAssets, csResCompanies, csResRemediations)
			if err != nil {
				return err
			}
			rows := buildExposure(data, exposureOpts{
				minEPSS: minEPSS, severities: severities, newSince: window,
				kevOnly: kevOnly, company: company, now: time.Now().UTC(),
			})
			if total := len(rows); limit > 0 && total > limit {
				rows = rows[:limit]
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: showing %d of %d matching rows; raise --limit (0 = all) to see more\n", limit, total)
			}
			return csEmit(cmd, flags, rows, unsynced, "No matching vulnerability findings in the local store.")
		},
	}
	cmd.Flags().Float64Var(&minEPSS, "min-epss", 0, "Minimum EPSS exploit probability, 0-1 (e.g. 0.5)")
	cmd.Flags().StringVar(&severity, "severity", "", "Comma-separated severities to keep: Critical,High,Medium,Low")
	cmd.Flags().StringVar(&newSince, "new-since", "", "Only findings first discovered within this window (e.g. 24h, 7d)")
	cmd.Flags().BoolVar(&kevOnly, "kev", false, "Only findings on the CISA Known Exploited Vulnerabilities list")
	cmd.Flags().StringVar(&company, "company", "", "Only this client (name substring or company ID)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the synced store)")
	return cmd
}
