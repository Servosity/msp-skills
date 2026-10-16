// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"bitdefender-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type incidentRow struct {
	IncidentID   string `json:"incident_id"`
	Number       int    `json:"number"`
	Type         string `json:"type"`
	CompanyID    string `json:"company_id"`
	CompanyName  string `json:"company_name"`
	Status       string `json:"status"`
	Priority     string `json:"priority"`
	Severity     int    `json:"severity"`
	MainAction   string `json:"main_action"`
	Created      string `json:"created"`
	AgeHours     int    `json:"age_hours"`
	LastChange   string `json:"last_change"`
	Assignee     string `json:"assignee,omitempty"`
	Unassigned   bool   `json:"unassigned"`
	SLABreached  bool   `json:"sla_breached"`
	ComputerName string `json:"computer_name,omitempty"`
	Link         string `json:"link,omitempty"`
	createdAt    time.Time
}

type companyCount struct {
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
	Incidents   int    `json:"incidents"`
}

type incidentQueueView struct {
	Incidents     []incidentRow  `json:"incidents"`
	Total         int            `json:"total"`
	Unassigned    int            `json:"unassigned"`
	SLABreached   int            `json:"sla_breached"`
	ByCompany     []companyCount `json:"by_company"`
	ScannedPages  int            `json:"scanned_pages"`
	ChangedSince  string         `json:"changed_since,omitempty"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
	Note          string         `json:"note,omitempty"`
}

// incidentsPageGap keeps getIncidentsList under its documented 3 calls per 60
// seconds per API key.
var incidentsPageGap = 21 * time.Second

func newNovelFleetIncidentQueueCmd(flags *rootFlags) *cobra.Command {
	var sla, statuses, priorities string
	var newOnly bool
	var perPage, maxPages, limit int
	cmd := &cobra.Command{
		Use:   "incident-queue",
		Short: "List open EDR/XDR incidents across all child companies, worst and oldest first, with unassigned and --sla flags",
		Long: `Pulls getIncidentsList (v1.2) once at partner scope with includeChildCompanies, so one call
covers every customer, then ranks open and in-progress incidents by severity score and age,
flagging unassigned incidents and, with --sla, incidents older than the SLA. getIncidentsList
allows about 3 calls per minute per key, so extra pages are spaced 21 seconds apart.
--new returns only incidents changed since the previous --new run with the same --status and
--priority filters (one local cursor per filter set).
Read-only: pass the incident ids to 'incidents set-status', 'incidents assign' or
'incidents note' to act on them.

Use this command for the open EDR/XDR incident queue across all child companies, sorted by
severity and age. Do NOT use this command to find the same file quarantined at multiple
companies; use 'fleet threats' instead.`,
		Example:     "  bitdefender-cli fleet incident-queue --sla 24h --agent\n  bitdefender-cli fleet incident-queue --new --priority high,critical --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet incident-queue")
			}
			var slaDur time.Duration
			if sla != "" {
				d, err := parseSpan(sla)
				if err != nil {
					return fleetUsageErr(cmd, flags, fmt.Errorf("--sla: %w", err))
				}
				slaDur = d
			}
			statusList, err := documentedValues("--status", statuses, incidentStatuses)
			if err != nil {
				return fleetUsageErr(cmd, flags, err)
			}
			priorityList, err := documentedValues("--priority", priorities, incidentPriorities)
			if err != nil {
				return fleetUsageErr(cmd, flags, err)
			}
			if maxPages < 1 {
				return fleetUsageErr(cmd, flags, fmt.Errorf("--max-pages must be at least 1"))
			}
			if perPage < 10 || perPage > 10000 {
				return fleetUsageErr(cmd, flags, fmt.Errorf("--per-page must be 10-10000"))
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
			filters := map[string]any{}
			if len(statusList) > 0 {
				filters["status"] = statusList
			}
			if len(priorityList) > 0 {
				filters["priority"] = priorityList
			}
			view := incidentQueueView{Incidents: make([]incidentRow, 0), ByCompany: make([]companyCount, 0), FetchFailures: make([]fetchFailure, 0)}
			now := time.Now().UTC()
			// --new reads a fixed [cursor, now] change window; the API takes
			// changeStartDate and changeEndDate as a pair.
			windowEnd := now.Format(time.RFC3339)
			// Each status/priority filter set has its own cursor: a filtered run
			// must not advance past incidents an unfiltered run has not seen.
			cursorKey := incidentCursorKey(statusList, priorityList)
			if newOnly {
				var last string
				if err := s.DB().QueryRowContext(ctx, `SELECT last_change FROM gz_incident_cursors WHERE filter_key = ?`, cursorKey).Scan(&last); err == nil && last != "" {
					// The API needs end > start at second resolution; the end is never
					// in the future, so wait for the clock instead of inventing one.
					if start, ok := parseGZTime(last); ok && !now.Truncate(time.Second).After(start) {
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-time.After(time.Until(start.Add(time.Second))):
						}
						now = time.Now().UTC()
						windowEnd = now.Format(time.RFC3339)
					}
					filters["changeStartDate"] = last
					filters["changeEndDate"] = windowEnd
					view.ChangedSince = last
				}
			}
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			pageCapHit := false
			firstTotal := -1 // page 1's "total": the window's size when paging started
			firstPages := 0
			var items []json.RawMessage
			for page := 1; page <= maxPages; page++ {
				if page > 1 {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(incidentsPageGap):
					}
				}
				raw, err := gzRead(ctx, c, gzPathIncidentsList, map[string]any{
					"page": page, "perPage": perPage, "filters": filters,
					"options": map[string]any{"includeChildCompanies": true, "sortBy": "lastIncidentChange"},
				})
				if err != nil {
					if page == 1 {
						return classifyAPIError(cmd.OutOrStdout(), err, flags)
					}
					view.FetchFailures = append(view.FetchFailures, fetchFailure{Scope: fmt.Sprintf("getIncidentsList page %d", page), Error: err.Error()})
					break
				}
				view.ScannedPages++
				var resp struct {
					Items      []json.RawMessage `json:"items"`
					PagesCount int               `json:"pagesCount"`
					Total      *int              `json:"total"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return apiErr(fmt.Errorf("unexpected getIncidentsList response: %w", err))
				}
				items = append(items, resp.Items...)
				if page == 1 {
					firstPages = resp.PagesCount // pagesCount arrives on page 1 only
					if resp.Total != nil {
						firstTotal = *resp.Total
					}
				}
				if len(resp.Items) < perPage || (firstPages > 0 && page >= firstPages) {
					break
				}
				if page == maxPages {
					pageCapHit = true
					view.Note = fmt.Sprintf("stopped after --max-pages %d; raise it (each extra page waits 21s for the rate limit) or narrow --status/--priority", maxPages)
				}
			}
			rows := rankIncidents(items, now, slaDur)
			// Incidents that change while we page move out of the window and shift
			// later pages, so an unchanged one can be skipped: if fewer unique
			// incidents arrived than page 1 counted, keep the cursor. A page cap or
			// a failed page already explains the gap and reports itself.
			shifted := !pageCapHit && len(view.FetchFailures) == 0 && firstTotal >= 0 && len(rows) < firstTotal
			limitHit := limit > 0 && len(rows) > limit
			if limitHit {
				rows = rows[:limit]
			}
			view.Incidents = rows
			view.Total = len(rows)
			counts := map[string]*companyCount{}
			for _, r := range rows {
				if r.Unassigned {
					view.Unassigned++
				}
				if r.SLABreached {
					view.SLABreached++
				}
				cc, ok := counts[r.CompanyID]
				if !ok {
					cc = &companyCount{CompanyID: r.CompanyID, CompanyName: r.CompanyName}
					counts[r.CompanyID] = cc
				}
				cc.Incidents++
			}
			for _, cc := range counts {
				view.ByCompany = append(view.ByCompany, *cc)
			}
			sort.Slice(view.ByCompany, func(i, j int) bool {
				if view.ByCompany[i].Incidents != view.ByCompany[j].Incidents {
					return view.ByCompany[i].Incidents > view.ByCompany[j].Incidents
				}
				return view.ByCompany[i].CompanyName < view.ByCompany[j].CompanyName
			})
			if len(view.FetchFailures) > 0 {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" partial queue: %d page(s) failed; incidents on them are missing.", len(view.FetchFailures)))
			}
			// Advance the --new cursor only when every changed incident in the
			// window was fetched and returned; otherwise the next run re-reads it.
			advance := newOnly && view.ScannedPages > 0 && !pageCapHit && !limitHit && !shifted && len(view.FetchFailures) == 0
			if shifted {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" incidents changed while paging (%d of %d received); rerun to catch the rest.", len(rows), firstTotal))
			}
			if newOnly && !advance {
				view.Note = strings.TrimSpace(view.Note + " --new cursor not advanced (truncated by --max-pages or --limit, or a page failed); the next --new run repeats this window. Raise --per-page (up to 10000) or --max-pages to read it all.")
			}
			saveCursor := func() {
				if !advance {
					return
				}
				if _, err := s.DB().ExecContext(ctx, `INSERT INTO gz_incident_cursors (filter_key, last_change) VALUES (?, ?)
					ON CONFLICT(filter_key) DO UPDATE SET last_change = excluded.last_change`, cursorKey, windowEnd); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: --new cursor not saved: %v\n", err)
				}
			}
			if flags.quiet {
				if view.Note != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+view.Note) // ids stay clean on stdout
				}
				for _, r := range rows {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), r.IncidentID); err != nil {
						return err
					}
				}
				saveCursor()
				return nil
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
				saveCursor()
				return nil
			}
			humanNote(cmd, view.Note)
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No matching incidents.")
				saveCursor()
				return nil
			}
			table := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				flag := ""
				if r.SLABreached {
					flag = "SLA"
				}
				if r.Unassigned {
					flag = strings.TrimSpace(flag + " unassigned")
				}
				table = append(table, map[string]any{"#": r.Number, "company": r.CompanyName, "severity": r.Severity, "priority": r.Priority,
					"status": r.Status, "age_h": r.AgeHours, "endpoint": r.ComputerName, "flags": flag, "id": r.IncidentID})
			}
			if err := printAutoTable(cmd.OutOrStdout(), table); err != nil {
				return err
			}
			saveCursor()
			return nil
		},
	}
	cmd.Flags().StringVar(&sla, "sla", "", "Flag incidents older than this (e.g. 4h, 24h, 3d)")
	cmd.Flags().StringVar(&statuses, "status", "open,in_progress", "Comma-separated statuses: open,in_progress,closed,false_positive,closed_mdr_reviewed")
	cmd.Flags().StringVar(&priorities, "priority", "", "Comma-separated priorities to keep: unknown,low,medium,high,critical")
	cmd.Flags().BoolVar(&newOnly, "new", false, "Only incidents changed since the previous --new run (advances a local cursor)")
	cmd.Flags().IntVar(&perPage, "per-page", 1000, "Incidents per call (10-10000)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 1, "Maximum getIncidentsList pages (each extra page waits 21s)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Return at most this many incidents after ranking (0 = all)")
	return cmd
}

var (
	incidentStatuses   = []string{"open", "in_progress", "closed", "false_positive", "closed_mdr_reviewed"}
	incidentPriorities = []string{"unknown", "low", "medium", "high", "critical"}
)

// documentedValues lowercases a comma list and rejects values the
// getIncidentsList filter does not document.
func documentedValues(flag, csv string, allowed []string) ([]string, error) {
	out := make([]string, 0)
	for _, v := range splitCSV(csv) {
		v = strings.ToLower(v)
		if !slices.Contains(allowed, v) {
			return nil, fmt.Errorf("%s: %q is not one of %s", flag, v, strings.Join(allowed, ","))
		}
		out = append(out, v)
	}
	return out, nil
}

// incidentCursorKey normalizes a status/priority filter set (order-insensitive).
func incidentCursorKey(statuses, priorities []string) string {
	norm := func(v []string) string {
		c := make([]string, 0, len(v))
		for _, x := range v {
			c = append(c, strings.ToLower(x))
		}
		sort.Strings(c)
		return strings.Join(c, ",")
	}
	return "status=" + norm(statuses) + "|priority=" + norm(priorities)
}

func splitCSV(s string) []string {
	out := make([]string, 0)
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// rankIncidents converts raw items to rows sorted by severity (desc) then age
// (oldest first).
func rankIncidents(items []json.RawMessage, now time.Time, sla time.Duration) []incidentRow {
	rows := make([]incidentRow, 0, len(items))
	seen := map[string]bool{}
	for _, raw := range items {
		var it struct {
			IncidentID     string `json:"incidentId"`
			IncidentNumber int    `json:"incidentNumber"`
			IncidentType   string `json:"incidentType"`
			Company        struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"company"`
			Status             string `json:"status"`
			Priority           string `json:"priority"`
			SeverityScore      int    `json:"severityScore"`
			MainAction         string `json:"mainAction"`
			Created            string `json:"created"`
			LastIncidentChange string `json:"lastIncidentChange"`
			Assignee           string `json:"assignee"`
			IncidentLink       string `json:"incidentLink"`
			Details            struct {
				ComputerName string `json:"computerName"`
			} `json:"details"`
		}
		if json.Unmarshal(raw, &it) != nil || it.IncidentID == "" || seen[it.IncidentID] {
			continue
		}
		seen[it.IncidentID] = true
		r := incidentRow{IncidentID: it.IncidentID, Number: it.IncidentNumber, Type: it.IncidentType, CompanyID: it.Company.ID,
			CompanyName: it.Company.Name, Status: it.Status, Priority: it.Priority, Severity: it.SeverityScore, MainAction: it.MainAction,
			Created: it.Created, LastChange: it.LastIncidentChange, Assignee: it.Assignee, Unassigned: it.Assignee == "",
			ComputerName: it.Details.ComputerName, Link: it.IncidentLink}
		if t, ok := parseGZTime(it.Created); ok {
			r.createdAt = t
			age := now.Sub(t)
			r.AgeHours = int(age.Hours())
			r.SLABreached = sla > 0 && age > sla
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Severity != rows[j].Severity {
			return rows[i].Severity > rows[j].Severity
		}
		return rows[i].createdAt.Before(rows[j].createdAt)
	})
	return rows
}
