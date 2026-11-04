// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: client-report.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"blackpoint-pp-cli/internal/client"
	"blackpoint-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type clientReport struct {
	Tenant        fleetTenant        `json:"tenant"`
	Window        map[string]string  `json:"window"`
	Detections    map[string]any     `json:"detections"`
	Posture       map[string]any     `json:"posture"`
	LatestReports []map[string]any   `json:"latest_reports"`
	FetchFailures []fleetSyncFailure `json:"fetch_failures"`
}

func newNovelClientReportCmd(flags *rootFlags) *cobra.Command {
	var since string
	var top int

	cmd := &cobra.Command{
		Use:   "client-report [tenant]",
		Short: "Build one client's reporting-window summary: posture, detection count and trend, top entities and threats, latest report",
		Long: `Use this command to build a reporting-window summary for one client from live CompassOne aggregation endpoints.
Do NOT use this command for a current-state look at open detections, stale devices and coverage; use 'overview' instead.

Calls six tenant-scoped endpoints with x-tenant-id: detection count, detections
by week, top entities, top threats, posture rating, and report runs whose
interval starts inside the window. Aggregation responses are returned verbatim.
A section that fails (for example 403 when not entitled) is listed under
fetch_failures; the rest of the report still returns. The tenant may be a
tenant id or a name resolved through 'tenants list --search'.`,
		Example: strings.Trim(`
  blackpoint-cli client-report "Acme Dental" --since 30d --agent
  blackpoint-cli client-report 3f2a9c1e-7b4d-4e8a-9f10-2c6d8b5e4a71 --since 90d`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "tenant=Acme Dental"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "client-report: GET /alert-groups/count, /alert-groups/alert-groups-by-week, /alert-groups/top-detections-by-entity, /alert-groups/top-detections-by-threat, /security-posture/rating, /reports with x-tenant-id")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("tenant (name or id) is required"))
			}
			window, err := parseFleetDuration(since)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--since must be a positive duration like 30d"))
			}
			if top < 1 {
				return usageErr(fmt.Errorf("--top must be at least 1"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			query := strings.Join(args, " ")
			tenant := fleetTenant{ID: query}
			if !store.IsUUID(query) {
				items, err := pagedGet(ctx, c, "/tenants", map[string]string{"search": query}, nil, 20)
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), fmt.Errorf("resolving tenant %q: %w", query, err), flags)
				}
				var found []fleetTenant
				for _, it := range items {
					var obj map[string]any
					if json.Unmarshal(it, &obj) == nil && obj != nil {
						found = append(found, tenantFromObj("", obj))
					}
				}
				if tenant, err = resolveFleetTenant(found, query); err != nil {
					return notFoundErr(err)
				}
			}

			end := time.Now().UTC()
			start := end.Add(-window)
			win := map[string]string{"startDate": start.Format(time.RFC3339), "endDate": end.Format(time.RFC3339)}
			hdr := map[string]string{"x-tenant-id": tenant.ID}
			rep := &clientReport{Tenant: tenant, Window: win, Detections: map[string]any{}, LatestReports: []map[string]any{}, FetchFailures: []fleetSyncFailure{}}

			get := func(section, path string, params map[string]string) json.RawMessage {
				raw, err := c.GetWithHeaders(ctx, path, params, hdr)
				if err != nil {
					msg := fleetErrText(err)
					if fleetStatus(err) == 403 {
						msg = "not entitled (403): " + msg
					}
					rep.FetchFailures = append(rep.FetchFailures, fleetSyncFailure{Tenant: tenant.Name, Kind: section, Error: msg})
					return nil
				}
				return raw
			}
			withWin := func(extra map[string]string) map[string]string {
				p := map[string]string{"startDate": win["startDate"], "endDate": win["endDate"]}
				for k, v := range extra {
					p[k] = v
				}
				return p
			}
			limit := fmt.Sprint(top)
			for _, sec := range []struct {
				key, path string
				extra     map[string]string
			}{
				{"count", "/alert-groups/count", nil},
				{"by_week", "/alert-groups/alert-groups-by-week", nil},
				{"top_entities", "/alert-groups/top-detections-by-entity", map[string]string{"limit": limit}},
				{"top_threats", "/alert-groups/top-detections-by-threat", map[string]string{"limit": limit}},
			} {
				if raw := get("detections."+sec.key, sec.path, withWin(sec.extra)); raw != nil {
					rep.Detections[sec.key] = json.RawMessage(raw)
				}
			}
			if raw := get("posture", "/security-posture/rating", nil); raw != nil {
				var obj map[string]any
				_ = json.Unmarshal(raw, &obj)
				score, hasScore := fieldNum(obj, "score")
				maxScore, hasMax := fieldNum(obj, "maximumScore")
				if obj == nil || !hasScore || !hasMax {
					rep.FetchFailures = append(rep.FetchFailures, fleetSyncFailure{Tenant: tenant.Name, Kind: "posture", Error: "response lacks score or maximumScore"})
				} else {
					fn, d := weakestNISTFunction(obj)
					rep.Posture = map[string]any{"score": score, "maximum_score": maxScore, "maturity_level": fieldStr(obj, "maturityLevel"),
						"weakest_nist_function": fn, "weakest_deduction": d}
				}
			}
			// Walk every run in the window (newest first) so each report type's
			// latest run is found even when another type fills the first page.
			items, rerr := pagedGet(ctx, c, "/reports", map[string]string{"startDate": win["startDate"], "endDate": win["endDate"], "sortBy": "intervalStart", "sortOrder": "desc"}, hdr, 20)
			if rerr != nil {
				msg := fleetErrText(rerr)
				if fleetStatus(rerr) == 403 {
					msg = "not entitled (403): " + msg
				}
				rep.FetchFailures = append(rep.FetchFailures, fleetSyncFailure{Tenant: tenant.Name, Kind: "reports", Error: msg})
			} else {
				latest := map[string]map[string]any{}
				for _, it := range items {
					var obj map[string]any
					if json.Unmarshal(it, &obj) != nil || obj == nil {
						continue
					}
					typ := fieldStr(obj, "reportType")
					cur, ok := latest[typ]
					if ok {
						a, _ := parseFleetTime(fieldStr(obj, "intervalStart"))
						b, _ := parseFleetTime(fieldStr(cur, "intervalStart"))
						if !a.After(b) {
							continue
						}
					}
					latest[typ] = obj
				}
				types := make([]string, 0, len(latest))
				for typ := range latest {
					types = append(types, typ)
				}
				sort.Strings(types)
				for _, typ := range types {
					obj := latest[typ]
					rep.LatestReports = append(rep.LatestReports, map[string]any{"id": fieldStr(obj, "id"), "report_type": typ,
						"interval_start": fieldStr(obj, "intervalStart"), "interval_end": fieldStr(obj, "intervalEnd")})
				}
			}
			if len(rep.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of 6 report sections failed (see fetch_failures)\n", len(rep.FetchFailures))
			}
			if err := printClientReport(cmd, flags, rep); err != nil {
				return err
			}
			if len(rep.FetchFailures) == 6 {
				return apiErr(fmt.Errorf("every report section failed; first error: %s", rep.FetchFailures[0].Error))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "Reporting window ending now (e.g. 30d, 90d)")
	cmd.Flags().IntVar(&top, "top", 5, "Entities and threats to include")
	return cmd
}

func printClientReport(cmd *cobra.Command, flags *rootFlags, rep *clientReport) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "# %s security report\n\n", firstNonEmpty(rep.Tenant.Name, rep.Tenant.ID))
	fmt.Fprintf(w, "Window: %s to %s\n\n", rep.Window["startDate"], rep.Window["endDate"])
	if rep.Posture != nil {
		fmt.Fprintf(w, "## Posture\n%v of %v (%v); weakest NIST function: %v\n\n", rep.Posture["score"], rep.Posture["maximum_score"], rep.Posture["maturity_level"], rep.Posture["weakest_nist_function"])
	}
	fmt.Fprintln(w, "## Detections")
	for _, k := range []string{"count", "by_week", "top_entities", "top_threats"} {
		if v, ok := rep.Detections[k]; ok {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "- %s: %s\n", k, b)
		}
	}
	if len(rep.LatestReports) > 0 {
		fmt.Fprintln(w, "\n## Latest report runs")
		for _, r := range rep.LatestReports {
			fmt.Fprintf(w, "- %v %v (%v to %v)\n", r["report_type"], r["id"], r["interval_start"], r["interval_end"])
		}
	}
	for _, f := range rep.FetchFailures {
		fmt.Fprintf(w, "\n(failed: %s: %s)", f.Kind, f.Error)
	}
	fmt.Fprintln(w)
	return nil
}

// pagedGet walks a page/pageSize list route (100 rows per page, at most
// maxPages pages) with the same completeness checks as fleet-sync.
func pagedGet(ctx context.Context, c *client.Client, path string, params, headers map[string]string, maxPages int) ([]json.RawMessage, error) {
	var all []json.RawMessage
	pt := newPageTracker(pageByNumber, 100)
	for page := 1; page <= maxPages; page++ {
		p := map[string]string{"page": fmt.Sprint(page), "pageSize": "100"}
		for k, v := range params {
			p[k] = v
		}
		raw, err := c.GetWithHeaders(ctx, path, p, headers)
		if err != nil {
			return all, err
		}
		items, meta, err := splitFleetPage(raw, false)
		if err != nil {
			return all, fmt.Errorf("GET %s: %w", path, err)
		}
		all = append(all, items...)
		done, err := pt.observe(page, items, meta)
		if err != nil {
			return all, fmt.Errorf("GET %s: %w", path, err)
		}
		if done {
			return all, nil
		}
	}
	return all, fmt.Errorf("GET %s: more than %d pages; narrow the request", path, maxPages)
}
