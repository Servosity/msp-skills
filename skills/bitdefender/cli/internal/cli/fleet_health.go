// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"bitdefender-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

type companyHealth struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	Endpoints        int    `json:"endpoints"`
	Managed          int    `json:"managed"`
	Unmanaged        int    `json:"unmanaged"`
	Outdated         int    `json:"outdated"`
	PolicyNotApplied int    `json:"policy_not_applied"`
	Isolated         int    `json:"isolated"`
	Unscanned        int    `json:"unscanned"`
	Issues           int    `json:"issues"`
	// Status survives --compact (--agent), so a company whose fetch failed is
	// never read as a clean row of zeros (handfixes.json: fleet-health-row-status).
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

type endpointDetail struct {
	CompanyID         string `json:"company_id"`
	CompanyName       string `json:"company_name"`
	EndpointID        string `json:"endpoint_id"`
	Name              string `json:"name"`
	LastSeen          string `json:"last_seen,omitempty"`
	SignatureOutdated *bool  `json:"signature_outdated,omitempty"`
	Infected          *bool  `json:"infected,omitempty"`
	MalwareDetected   *bool  `json:"malware_detected_24h,omitempty"`
	Error             string `json:"error,omitempty"`
}

type fleetHealthView struct {
	TakenAt          string           `json:"taken_at"`
	SnapshotID       int64            `json:"snapshot_id,omitempty"`
	ScanDays         int              `json:"scan_days"`
	Companies        []companyHealth  `json:"companies"`
	Totals           companyHealth    `json:"totals"`
	Details          []endpointDetail `json:"details,omitempty"`
	ScannedCompanies int              `json:"scanned_companies"`
	ScannedPages     int              `json:"scanned_pages"`
	FetchFailures    []fetchFailure   `json:"fetch_failures"`
	Note             string           `json:"note,omitempty"`
}

var fleetHealthMetrics = []string{"unmanaged", "outdated", "policy_not_applied", "isolated", "unscanned"}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var scanDays, details, maxPages, maxCompanies int
	var failOn string
	var noSave bool
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Rank every child company by unmanaged, outdated, unscanned, policy-not-applied and isolated endpoints; saves a snapshot",
		Long: `Walks the partner tree, pages getEndpointsList (v1.1, recursive, with productOutdated and
last-scan options) for every company, and returns one row per company ranked by issue count.
Each run is saved as a dated local snapshot that 'fleet changes', 'fleet locate' and
'fleet rmm-gap' read. --details N adds getManagedEndpointDetails (last seen, signature
outdated, infected) for the N managed endpoints with the oldest or missing scan.
--fail-on unmanaged,outdated exits 1 when any company has one of those issues, and 5 when
any fetch failed (an incomplete fleet is never certified clean).

Use this command for a per-company rollup of current endpoint protection status (unmanaged,
outdated, unscanned, policy not applied, isolated). Do NOT use this command to see what
changed since an earlier run; use 'fleet changes' instead. Do NOT use it to locate one
machine by hostname, IP or MAC; use 'fleet locate' instead.`,
		Example:     "  bitdefender-cli fleet health --agent\n  bitdefender-cli fleet health --scan-days 14 --details 20 --json\n  bitdefender-cli fleet health --fail-on unmanaged,outdated --quiet",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			failMetrics, err := parseFailOn(failOn)
			if err != nil {
				return fleetUsageErr(cmd, flags, err)
			}
			// No whole-command deadline: a fan-out over many companies runs far
			// longer than --timeout, which the generated client applies per request.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			s, err := openGZStore(ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			tree, treeFailures, err := companyTreeFor(ctx, c, s.DB(), flags.maxAge, maxCompanies)
			if err != nil && len(tree) == 0 {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			results, pages := collectFleet(ctx, c, tree, maxPages)
			view := buildFleetHealth(results, scanDays, time.Now().UTC())
			view.ScannedPages = pages
			view.FetchFailures = failureList(results, treeFailures)
			if err != nil {
				view.FetchFailures = append(view.FetchFailures, fetchFailure{Scope: "company tree", Error: err.Error()})
			}
			if err != nil {
				view.Note = err.Error()
			}
			if len(results) < len(tree) {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" live dogfood: scanned %d of %d companies.", len(results), len(tree)))
			}
			if details > 0 {
				view.Details = enrichEndpoints(ctx, c, results, details)
				for _, d := range view.Details {
					if d.Error != "" {
						view.FetchFailures = append(view.FetchFailures, fetchFailure{Scope: "endpoint detail " + d.Name + " (" + d.EndpointID + ")", Error: d.Error})
					}
				}
			}
			if !noSave {
				treeComplete := err == nil && len(treeFailures) == 0 && len(results) == len(tree)
				id, at, serr := saveFleetSnapshot(ctx, s.DB(), results, treeComplete)
				if serr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: snapshot not saved: %v\n", serr)
				} else {
					view.SnapshotID, view.TakenAt = id, at.Format(time.RFC3339)
				}
			}
			if n := countFailed(results); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d company fetches failed; totals cover the remaining %d\n", n, len(results), len(results)-n)
			}
			if n := len(treeFailures); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d company-tree calls failed; companies below them are missing\n", n)
			}
			if err := emitFleetHealth(cmd, flags, view); err != nil {
				return err
			}
			complete := err == nil && len(treeFailures) == 0 && len(results) == len(tree) && len(view.FetchFailures) == 0
			if len(failMetrics) > 0 && !complete {
				return apiErr(fmt.Errorf("--fail-on: the fleet scan was incomplete (%d fetch failures, %d of %d companies scanned), so it cannot be certified clean", len(view.FetchFailures), len(results), len(tree)))
			}
			if failing := companiesFailing(view.Companies, failMetrics); len(failing) > 0 {
				return &cliError{code: 1, err: fmt.Errorf("--fail-on: %d companies have %s", len(failing), strings.Join(failMetrics, "/"))}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&scanDays, "scan-days", 7, "Count a managed endpoint as unscanned when its last successful scan is older than this many days (or missing)")
	cmd.Flags().IntVar(&details, "details", 0, "Fetch getManagedEndpointDetails for up to N managed endpoints with the oldest or missing scan")
	cmd.Flags().IntVar(&maxPages, "max-pages", 20, "Maximum getEndpointsList pages (1000 endpoints each) per company")
	cmd.Flags().IntVar(&maxCompanies, "max-companies", 5000, "Stop walking the company tree after this many companies")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "Exit 1 when any company has these issues: unmanaged,outdated,policy_not_applied,isolated,unscanned")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Do not record this run as a local snapshot")
	return cmd
}

func boolText(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "true"
	}
	return "false"
}

func parseFailOn(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []string
	for _, m := range strings.Split(s, ",") {
		m = strings.TrimSpace(strings.ReplaceAll(m, "-", "_"))
		ok := false
		for _, valid := range fleetHealthMetrics {
			if m == valid {
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("--fail-on: unknown issue %q (use %s)", m, strings.Join(fleetHealthMetrics, ","))
		}
		out = append(out, m)
	}
	return out, nil
}

func metricValue(h companyHealth, m string) int {
	switch m {
	case "unmanaged":
		return h.Unmanaged
	case "outdated":
		return h.Outdated
	case "policy_not_applied":
		return h.PolicyNotApplied
	case "isolated":
		return h.Isolated
	case "unscanned":
		return h.Unscanned
	}
	return 0
}

func companiesFailing(rows []companyHealth, metrics []string) []string {
	var out []string
	for _, r := range rows {
		for _, m := range metrics {
			if r.Error == "" && metricValue(r, m) > 0 {
				out = append(out, r.ID)
				break
			}
		}
	}
	return out
}

func countFailed(results []fleetCompanyResult) int {
	n := 0
	for _, r := range results {
		if r.Err != nil {
			n++
		}
	}
	return n
}

// buildFleetHealth rolls endpoints up per company. Failed companies keep a row
// with the error but are excluded from the totals.
func buildFleetHealth(results []fleetCompanyResult, scanDays int, now time.Time) fleetHealthView {
	view := fleetHealthView{TakenAt: now.Format(time.RFC3339), ScanDays: scanDays, Companies: make([]companyHealth, 0, len(results)), ScannedCompanies: len(results)}
	cutoff := now.Add(-time.Duration(scanDays) * 24 * time.Hour)
	view.Totals.Name = "TOTAL"
	for _, r := range results {
		h := companyHealth{ID: r.Company.ID, Name: r.Company.Name, Path: r.Company.Path}
		if r.Err != nil {
			h.Status = "fetch_failed"
			h.Error = r.Err.Error()
			view.Companies = append(view.Companies, h)
			continue
		}
		h.Status = "ok"
		for _, e := range r.Endpoints {
			h.Endpoints++
			if !e.IsManaged {
				h.Unmanaged++
				continue
			}
			h.Managed++
			if e.ProductOutdated {
				h.Outdated++
			}
			if e.Policy != nil && !e.Policy.Applied {
				h.PolicyNotApplied++
			}
			if e.IsIsolated {
				h.Isolated++
			}
			if e.LastScan == nil {
				h.Unscanned++
			} else if t, ok := parseGZTime(e.LastScan.Date); !ok || t.Before(cutoff) {
				h.Unscanned++
			}
		}
		h.Issues = h.Unmanaged + h.Outdated + h.PolicyNotApplied + h.Isolated + h.Unscanned
		view.Totals.Endpoints += h.Endpoints
		view.Totals.Managed += h.Managed
		view.Totals.Unmanaged += h.Unmanaged
		view.Totals.Outdated += h.Outdated
		view.Totals.PolicyNotApplied += h.PolicyNotApplied
		view.Totals.Isolated += h.Isolated
		view.Totals.Unscanned += h.Unscanned
		view.Totals.Issues += h.Issues
		view.Companies = append(view.Companies, h)
	}
	sort.SliceStable(view.Companies, func(i, j int) bool {
		a, b := view.Companies[i], view.Companies[j]
		if a.Issues != b.Issues {
			return a.Issues > b.Issues
		}
		return a.Name < b.Name
	})
	return view
}

// enrichEndpoints fetches detail for the managed endpoints whose last scan is
// oldest (missing first), up to limit, one call each.
func enrichEndpoints(ctx context.Context, c *client.Client, results []fleetCompanyResult, limit int) []endpointDetail {
	type cand struct {
		co   gzCompany
		e    gzEndpoint
		scan time.Time
	}
	var cands []cand
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		for _, e := range r.Endpoints {
			if !e.IsManaged {
				continue
			}
			var t time.Time
			if e.LastScan != nil {
				t, _ = parseGZTime(e.LastScan.Date)
			}
			cands = append(cands, cand{r.Company, e, t})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].scan.Before(cands[j].scan) })
	if len(cands) > limit {
		cands = cands[:limit]
	}
	out := make([]endpointDetail, 0, len(cands))
	for _, cd := range cands {
		d := endpointDetail{CompanyID: cd.co.ID, CompanyName: cd.co.Name, EndpointID: cd.e.ID, Name: cd.e.Name}
		raw, err := gzRead(ctx, c, gzPathEndpointDetail, map[string]any{"endpointId": cd.e.ID})
		if err != nil {
			d.Error = err.Error()
			out = append(out, d)
			continue
		}
		var det struct {
			LastSeen string `json:"lastSeen"`
			Agent    struct {
				SignatureOutdated *bool `json:"signatureOutdated"`
			} `json:"agent"`
			MalwareStatus struct {
				Detection *bool `json:"detection"`
				Infected  *bool `json:"infected"`
			} `json:"malwareStatus"`
		}
		if err := json.Unmarshal(raw, &det); err != nil {
			d.Error = "unexpected getManagedEndpointDetails response"
		} else {
			d.LastSeen, d.SignatureOutdated = det.LastSeen, det.Agent.SignatureOutdated
			d.Infected, d.MalwareDetected = det.MalwareStatus.Infected, det.MalwareStatus.Detection
		}
		out = append(out, d)
	}
	return out
}

func emitFleetHealth(cmd *cobra.Command, flags *rootFlags, view fleetHealthView) error {
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		humanNote(cmd, view.Note)
		rows := make([]map[string]any, 0, len(view.Companies)+1)
		for _, h := range append(view.Companies, view.Totals) {
			row := map[string]any{"company": h.Name, "endpoints": h.Endpoints, "unmanaged": h.Unmanaged, "outdated": h.Outdated,
				"policy_not_applied": h.PolicyNotApplied, "isolated": h.Isolated, "unscanned": h.Unscanned, "issues": h.Issues}
			if h.Error != "" {
				row["issues"] = "error: " + h.Error
			}
			rows = append(rows, row)
		}
		if len(view.Companies) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No companies found under this API key.")
			return nil
		}
		if err := printAutoTable(cmd.OutOrStdout(), rows); err != nil {
			return err
		}
		if len(view.Details) > 0 {
			fmt.Fprintln(cmd.OutOrStdout())
			det := make([]map[string]any, 0, len(view.Details))
			for _, d := range view.Details {
				row := map[string]any{"company": d.CompanyName, "endpoint": d.Name, "last_seen": d.LastSeen,
					"signature_outdated": boolText(d.SignatureOutdated), "infected": boolText(d.Infected)}
				if d.Error != "" {
					row["last_seen"] = "error: " + d.Error
				}
				det = append(det, row)
			}
			return printAutoTable(cmd.OutOrStdout(), det)
		}
		return nil
	}
	return printJSONFiltered(cmd.OutOrStdout(), view, flags)
}
