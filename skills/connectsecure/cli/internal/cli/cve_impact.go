// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: which clients one CVE hits.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"connectsecure-pp-cli/internal/client"
)

var cveIDPattern = regexp.MustCompile(`(?i)^CVE-\d{4}-\d{4,}$`)

type cveClient struct {
	CompanyName string   `json:"company_name"`
	AssetCount  int      `json:"asset_count"`
	Assets      []string `json:"assets"`
	// SharedBy is set when several companies carry this name: the report
	// rows carry no company ID, so their assets cannot be split.
	SharedBy int `json:"companies_sharing_name,omitempty"`
}

// cveMaxNameChecks bounds the per-name company lookups.
const cveMaxNameChecks = 50

// markSharedCVEClientNames flags clients whose name several companies share.
func markSharedCVEClientNames(ctx context.Context, c *client.Client, view *cveImpactView) {
	for i := range view.Clients {
		cl := &view.Clients[i]
		if cl.CompanyName == "(unattributed)" {
			continue
		}
		if i >= cveMaxNameChecks {
			view.FetchFailures = append(view.FetchFailures, csFetchFailure{Section: "companies", Error: fmt.Sprintf("only the first %d client names were checked for companies sharing a name", cveMaxNameChecks)})
			return
		}
		rows, _, err := csGetRows(ctx, c, "/r/company/companies", map[string]string{"condition": "name ILIKE " + csQuote(cl.CompanyName)}, 10, 1)
		if err != nil {
			view.FetchFailures = append(view.FetchFailures, csFetchFailure{Section: "companies", Error: "could not check whether client names are shared: " + err.Error()})
			return
		}
		n := 0
		for _, r := range rows {
			if strings.EqualFold(csString(r["name"]), cl.CompanyName) {
				n++
			}
		}
		if n > 1 {
			cl.SharedBy = n
		}
	}
}

type cveImpactView struct {
	CVE                string           `json:"cve"`
	Details            any              `json:"details"`
	Summary            any              `json:"summary"`
	Clients            []cveClient      `json:"clients"`
	AttributedAssets   *int             `json:"attributed_assets"`
	UnattributedAssets *int             `json:"unattributed_assets"`
	ReportedAssets     *int             `json:"reported_assets"`
	AffectedSoftware   []csRow          `json:"affected_software"`
	Fixes              []any            `json:"fixes"`
	Note               string           `json:"note,omitempty"`
	Truncated          []string         `json:"truncated_sections"`
	FetchFailures      []csFetchFailure `json:"fetch_failures"`
}

// cveReportShapeErr turns a non-object CVE report into a fetch failure.
func cveReportShapeErr(report any, err error) error {
	if _, ok := report.(map[string]any); err == nil && report != nil && !ok {
		return fmt.Errorf("/r/cve/report: expected an object, got %T", report)
	}
	return err
}

// buildCVEImpact assembles the view from the two lookups. Fields that depend
// on a lookup stay nil (JSON null) when that lookup failed, so a failure can
// never read as "no fixes" or "no affected clients".
func buildCVEImpact(cve string, report any, reportErr error, assets []csRow, truncated bool, assetsErr error, maxAssets, maxPages int) cveImpactView {
	view := cveImpactView{CVE: cve, Truncated: make([]string, 0), FetchFailures: make([]csFetchFailure, 0)}
	reportErr = cveReportShapeErr(report, reportErr)
	if reportErr != nil {
		view.FetchFailures = append(view.FetchFailures, csFetchFailure{Section: "cve_report", Error: reportErr.Error()})
	} else {
		view.Fixes, view.AffectedSoftware = make([]any, 0), make([]csRow, 0)
		reported := 0
		if m, ok := report.(map[string]any); ok {
			view.Details, view.Summary = m["cve_details"], m["summary"]
			if fixes, ok := m["available_fixes"].([]any); ok {
				view.Fixes = fixes
			}
			if sw, ok := m["affected_software"].([]any); ok {
				for _, item := range sw {
					if sm, ok := item.(map[string]any); ok {
						if info, ok := sm["software_info"].(map[string]any); ok {
							view.AffectedSoftware = append(view.AffectedSoftware, info)
						}
					}
				}
			}
			if summary, ok := m["summary"].(map[string]any); ok {
				reported = int(csFloat0(summary["total_affected_assets"]))
			}
		}
		view.ReportedAssets = &reported
	}
	if assetsErr != nil {
		view.FetchFailures = append(view.FetchFailures, csFetchFailure{Section: "get_assets_by_problem", Error: assetsErr.Error()})
		return view
	}
	unattributed := 0
	for _, a := range assets {
		if csString(a["company_name"]) == "" {
			unattributed++
		}
	}
	attributed := len(assets) - unattributed
	if truncated {
		view.Truncated = append(view.Truncated, "get_assets_by_problem")
	}
	view.Clients = groupCVEClients(assets, maxAssets)
	view.AttributedAssets, view.UnattributedAssets = &attributed, &unattributed
	switch {
	case truncated:
		view.Note = fmt.Sprintf("stopped after %d pages of affected assets; raise --max-pages for the full list", maxPages)
	case unattributed > 0:
		view.Note = fmt.Sprintf("%d affected assets carry no client name; they are grouped under (unattributed)", unattributed)
	case view.ReportedAssets != nil && *view.ReportedAssets > attributed:
		view.Note = fmt.Sprintf("the CVE report counts %d affected assets but only %d could be attributed to a client", *view.ReportedAssets, attributed)
	case len(assets) == 0:
		view.Note = "no assets in this tenant are recorded with this CVE"
	}
	return view
}

// groupCVEClients groups get_assets_by_problem rows by client.
func groupCVEClients(rows []csRow, maxAssets int) []cveClient {
	byName := map[string]*cveClient{}
	for _, r := range rows {
		name := csString(r["company_name"])
		if name == "" {
			name = "(unattributed)"
		}
		cl, ok := byName[name]
		if !ok {
			cl = &cveClient{CompanyName: name, Assets: make([]string, 0)}
			byName[name] = cl
		}
		cl.AssetCount++
		if len(cl.Assets) < maxAssets {
			cl.Assets = append(cl.Assets, csAssetName(r))
		}
	}
	out := make([]cveClient, 0, len(byName))
	for _, cl := range byName {
		out = append(out, *cl)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AssetCount != out[j].AssetCount {
			return out[i].AssetCount > out[j].AssetCount
		}
		return out[i].CompanyName < out[j].CompanyName
	})
	return out
}

func newNovelCveImpactCmd(flags *rootFlags) *cobra.Command {
	var maxAssets, maxPages int
	cmd := &cobra.Command{
		Use:   "cve-impact <cve>",
		Short: "For one CVE: affected clients with asset counts, affected software, fixes and CVSS/EPSS",
		Long: `Answer "which clients does this CVE hit" from live data. Calls the CVE report
(details, affected software, available fixes) and the assets-by-problem report,
then groups affected assets by client. When the CVE report counts more affected
assets than could be attributed to a client, the gap is reported in note.

Use this command to answer which clients and assets are affected by one named
CVE and what fixes exist.
Do NOT use this command to rank all open vulnerabilities across clients; use 'exposure' instead.`,
		Example: strings.Trim(`
  connectsecure-cli cve-impact CVE-2024-3400 --agent
  connectsecure-cli cve-impact cve-2021-44228 --select clients,fixes`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "cve=CVE-2024-3400"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cve-impact")
			}
			if len(args) != 1 || !cveIDPattern.MatchString(strings.TrimSpace(args[0])) {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass exactly one CVE id like CVE-2024-3400"))
			}
			if maxPages < 1 || maxAssets < 0 {
				return usageErr(fmt.Errorf("--max-pages must be >= 1 and --max-assets >= 0"))
			}
			cve := strings.ToUpper(strings.TrimSpace(args[0]))
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			report, reportErr := csGetData(ctx, c, "/r/cve/report", map[string]string{"cve": cve})
			reportErr = cveReportShapeErr(report, reportErr)
			assets, truncated, assetsErr := csGetRows(ctx, c, "/r/report_queries/get_assets_by_problem", map[string]string{"problem_name": cve}, 100, maxPages)
			if reportErr != nil && assetsErr != nil {
				return classifyAPIErrorOnly(reportErr)
			}
			view := buildCVEImpact(cve, report, reportErr, assets, truncated, assetsErr, maxAssets, maxPages)
			markSharedCVEClientNames(ctx, c, &view)
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d lookup(s) failed; see fetch_failures\n", len(view.FetchFailures))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			if assetsErr != nil {
				fmt.Fprintf(w, "%s: affected clients unavailable (asset lookup failed)\n", cve)
			} else {
				fmt.Fprintf(w, "%s: %d clients, %d attributed assets\n", cve, len(view.Clients), *view.AttributedAssets)
			}
			for _, cl := range view.Clients {
				name := cl.CompanyName
				if cl.SharedBy > 1 {
					name = fmt.Sprintf("%s [name shared by %d companies]", name, cl.SharedBy)
				}
				fmt.Fprintf(w, "  %-40s %d assets  %s\n", name, cl.AssetCount, strings.Join(cl.Assets, ", "))
			}
			if view.Fixes == nil { // nil exactly when the CVE report failed or was malformed
				fmt.Fprintln(w, "  fixes: unavailable (CVE report failed)")
			} else {
				fmt.Fprintf(w, "  fixes available: %d\n", len(view.Fixes))
			}
			for _, ff := range view.FetchFailures {
				fmt.Fprintf(w, "  (failed: %s: %s)\n", ff.Section, ff.Error)
			}
			if view.Note != "" {
				fmt.Fprintf(w, "  note: %s\n", view.Note)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&maxAssets, "max-assets", 20, "Asset names listed per client (asset_count covers every asset read)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, "Pages of 100 affected assets to read before stopping")
	return cmd
}
