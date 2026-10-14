// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
// Reads the synced fleet list locally; availability, patches and sessions come from the live API.

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"connectwise-rmm-pp-cli/internal/fleet"
	"connectwise-rmm-pp-cli/internal/store"
)

type postureView struct {
	MinCompliance    float64                 `json:"min_compliance"`
	StaleAfter       string                  `json:"stale_after"`
	ScannedEndpoints int                     `json:"scanned_endpoints"`
	CapHit           bool                    `json:"scan_cap_hit,omitempty"`
	Companies        []fleet.PostureRow      `json:"companies"`
	Endpoints        []fleet.EndpointPosture `json:"endpoints,omitempty"`
	FetchFailures    []fetchFailure          `json:"fetch_failures,omitempty"`
	Note             string                  `json:"note,omitempty"`
}

// patchScope is shared by patch-posture, reboot-due and scorecard: load the
// fleet, scope it, and read every in-scope endpoint's patch summary through
// the local cache.
type patchScope struct {
	eps      []fleet.Endpoint
	sums     map[string]fleet.PatchSummary
	failures []fetchFailure
	capHit   bool
}

func loadPatchScope(ctx context.Context, cmd *cobra.Command, flags *rootFlags, db *store.Store, company string, maxEndpoints int, ttl time.Duration, refresh bool) (patchScope, error) {
	var ps patchScope
	eps, err := loadFleet(db)
	if err != nil {
		return ps, err
	}
	ps.eps, ps.capHit = capEndpoints(filterFleet(eps, company, ""), maxEndpoints)
	if len(ps.eps) == 0 {
		ps.sums = map[string]fleet.PatchSummary{}
		return ps, nil
	}
	c, err := flags.newClient()
	if err != nil {
		return ps, err
	}
	raw, failures := cachedEndpointData(ctx, cmd, db, c, cachePatchSummary, endpointIDs(ps.eps), ttl, refresh, patchPath)
	if len(raw) == 0 && len(failures) > 0 {
		return ps, classifyAPIErrorOnly(fmt.Errorf("patching API: %s", failures[0].Error))
	}
	sums, bad := patchSummaries(raw)
	failures = append(failures, bad...)
	if len(sums) == 0 && len(failures) > 0 {
		return ps, classifyAPIErrorOnly(fmt.Errorf("patching API: %s", failures[0].Error))
	}
	warnFailures(cmd, "patch summary", failures, len(ps.eps))
	ps.sums, ps.failures = sums, failures
	return ps, nil
}

func newNovelFleetPatchPostureCmd(flags *rootFlags) *cobra.Command {
	var company, staleAfter, cacheTTL, dbPath string
	var minCompliance float64
	var maxEndpoints int
	var refresh bool
	cmd := &cobra.Command{
		Use:   "patch-posture",
		Short: "Rank companies by patch risk: endpoints below --min-compliance, missing patches, pending reboots, out of support",
		Long: strings.Trim(`
Companies ranked by patch risk. For every synced endpoint the command reads the
patch summary (compliance score, missing, pending reboot, out of support,
assessment time), caches it locally, and rolls it up per company. With
--company it also lists that company's endpoints worst-first with their policy.

risk_score = endpoints below --min-compliance + pending-reboot endpoints +
out-of-support endpoints + assessments older than --stale-after. Endpoints
whose summary could not be read count toward endpoints but not assessed.

Use this command to rank companies or endpoints by patch risk. Do NOT use this
command for the list of machines to reboot now; use 'fleet reboot-due' instead.
Do NOT use this command for one company's all-signals card; use 'fleet
scorecard' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-rmm-cli fleet patch-posture --min-compliance 90 --agent
  connectwise-rmm-cli fleet patch-posture --company "Acme Corp" --stale-after 14d
  connectwise-rmm-cli fleet patch-posture --refresh --max-endpoints 1000`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet patch-posture")
			}
			stale, err := parseDurationFlag("stale-after", staleAfter)
			if err != nil {
				return err
			}
			ttl, err := parseDurationFlag("cache-ttl", cacheTTL)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view := postureView{MinCompliance: minCompliance, StaleAfter: staleAfter, Companies: make([]fleet.PostureRow, 0)}
			db, missing, err := openFleetStore(ctx, cmd, dbPath)
			if err != nil {
				return err
			}
			if missing {
				view.Note = noMirrorNote
				return emptyResult(cmd, flags, view)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "endpoints") {
				hintIfStale(cmd, db, "endpoints", flags.maxAge)
			}
			ps, err := loadPatchScope(ctx, cmd, flags, db, company, maxEndpoints, ttl, refresh)
			if err != nil {
				return err
			}
			view.ScannedEndpoints, view.CapHit, view.FetchFailures = len(ps.eps), ps.capHit, ps.failures
			view.Note = unsyncedNote(len(ps.eps))
			companies, detail := fleet.Posture(ps.eps, ps.sums, minCompliance, stale, time.Now().UTC())
			view.Companies = companies
			if company != "" {
				view.Endpoints = detail
			}
			if ps.capHit {
				view.Note = fmt.Sprintf("only the first %d endpoints were read; raise --max-endpoints to cover the whole fleet", len(ps.eps))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Companies) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No endpoints in scope.")
				return nil
			}
			rows := make([]map[string]any, 0, len(view.Companies))
			for _, r := range view.Companies {
				rows = append(rows, map[string]any{"company": firstNonEmpty(r.Company, r.CompanyID), "endpoints": r.Endpoints, "assessed": r.Assessed, "below_bar": r.BelowCompliance, "missing": r.MissingPatches, "reboot": r.PendingRebootHosts, "unsupported": r.OutOfSupport, "stale": r.StaleAssessments, "risk": r.RiskScore})
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&company, "company", "", "Limit to one company (ID or name) and list its endpoints")
	cmd.Flags().Float64Var(&minCompliance, "min-compliance", 90, "Compliance score (0-100) below which an endpoint counts as at risk")
	cmd.Flags().StringVar(&staleAfter, "stale-after", "14d", "Flag patch assessments older than this, e.g. 7d or 2w")
	cmd.Flags().StringVar(&cacheTTL, "cache-ttl", "12h", "Reuse cached patch summaries younger than this")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Ignore cached patch summaries and re-read every endpoint")
	cmd.Flags().IntVar(&maxEndpoints, "max-endpoints", 500, "Maximum endpoints to read patch summaries for (0 = no cap)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
