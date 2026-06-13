// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence commands for rewst-cli. NOT generated.
//
// These are multi-call GraphQL aggregators over Rewst's api.rewst.io/graphql
// endpoint that answer fleet-level questions the single-call API cannot:
// execution health, ROI rollup, failure triage, dormant-workflow detection,
// cross-org config drift, and pack coverage. Each reads live (most Rewst list
// queries are org-scoped and not auto-syncable), guards --dry-run for verify,
// and emits agent-native JSON via the generated printJSONFiltered helper.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"rewst-pp-cli/internal/client"
	"rewst-pp-cli/internal/cliutil"
)

// Data-source strategy for every transcendence command in this file: they are
// live GraphQL aggregators that query api.rewst.io/graphql directly and
// aggregate in memory. Most Rewst list queries are org-scoped and not part of
// defaultSyncResources, so these intentionally do not read the local store.
// pp:data-source live

// ---- shared GraphQL plumbing ----

type gqlError struct {
	Message string `json:"message"`
}

type gqlEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors"`
}

// rewstGraphQL posts a GraphQL document and returns the `data` payload. The
// Rewst gateway returns 200 with an `errors` array on auth/validation problems
// (including the credential-less "not authorized" case), so a non-empty
// errors array is surfaced as a typed error rather than swallowed. Under
// PRINTING_PRESS_VERIFY the generated client short-circuits POSTs to a
// synthetic envelope with no `data`, which decodes to a nil payload here and
// is handled by callers as an empty result.
func rewstGraphQL(ctx context.Context, c *client.Client, query string, vars map[string]any) (json.RawMessage, error) {
	body := map[string]any{"query": query}
	if len(vars) > 0 {
		body["variables"] = vars
	}
	raw, status, err := c.Post(ctx, "/graphql", body)
	if err != nil {
		return nil, err
	}
	var env gqlEnvelope
	if len(raw) > 0 {
		if uerr := json.Unmarshal(raw, &env); uerr != nil {
			return nil, fmt.Errorf("decoding GraphQL response (HTTP %d): %w", status, uerr)
		}
	}
	if len(env.Errors) > 0 {
		msgs := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			if strings.TrimSpace(e.Message) != "" {
				msgs = append(msgs, e.Message)
			}
		}
		if len(msgs) == 0 {
			msgs = append(msgs, "request returned GraphQL errors")
		}
		return nil, fmt.Errorf("rewst GraphQL error (set REWST_API_TOKEN for the target org): %s", strings.Join(msgs, "; "))
	}
	return env.Data, nil
}

// sinceTimestamp converts a loose duration ("24h", "30d", "1w") into an
// RFC3339 cutoff timestamp string, the shape Rewst's String date args expect.
func sinceTimestamp(dur time.Duration) string {
	return time.Now().Add(-dur).UTC().Format(time.RFC3339)
}

// parseFlexibleTime best-effort parses a Rewst timestamp string. Returns
// (zero, false) when the value is empty or in an unrecognized format so
// callers can decide whether to include or exclude the row.
func parseFlexibleTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999Z07:00", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// emitNovel renders a result. Machine consumers (--json/--agent/--csv/--quiet,
// or piped stdout) get filtered JSON through the generated helper so --select
// and --compact work; an interactive terminal gets the supplied human summary.
func emitNovel(cmd *cobra.Command, flags *rootFlags, machine any, human func(io.Writer)) error {
	out := cmd.OutOrStdout()
	if flags.asJSON || flags.csv || flags.quiet || flags.plain || flags.selectFields != "" || flags.compact || !isTerminal(out) {
		return printJSONFiltered(out, machine, flags)
	}
	human(out)
	return nil
}

func failedExecutionStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FAILED", "TIMEOUT", "ABANDONED", "CANCELED", "CANCELLED", "CANCELING":
		return true
	}
	return false
}

// ---- health ----

type wfExecStats struct {
	Succeeded         int `json:"succeeded"`
	Failed            int `json:"failed"`
	Running           int `json:"running"`
	Pending           int `json:"pending"`
	Delayed           int `json:"delayed"`
	Paused            int `json:"paused"`
	HumanSecondsSaved int `json:"humanSecondsSaved"`
}

type healthView struct {
	Org        string  `json:"org"`
	Since      string  `json:"since"`
	Succeeded  int     `json:"succeeded"`
	Failed     int     `json:"failed"`
	Running    int     `json:"running"`
	Pending    int     `json:"pending"`
	Delayed    int     `json:"delayed"`
	Paused     int     `json:"paused"`
	Total      int     `json:"total"`
	HoursSaved float64 `json:"hours_saved"`
	Healthy    bool    `json:"healthy"`
	Verdict    string  `json:"verdict"`
}

func newHealthCmd(flags *rootFlags) *cobra.Command {
	var org, since string
	cmd := &cobra.Command{
		Use:         "health",
		Short:       "Fleet execution health for an org (succeeded/failed/running + time saved)",
		Long:        "Roll up workflowExecutionStats for an organization into a single health verdict over a time window. Reach for this first to answer 'is automation healthy for this client right now'. Use 'failures' to drill into the failed runs.",
		Example:     "  rewst-cli health --org 11111111-1111-1111-1111-111111111111 --since 24h --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch workflowExecutionStats for the org")
				return nil
			}
			if strings.TrimSpace(org) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--org is required"))
			}
			dur, err := cliutil.ParseDurationLoose(since)
			if err != nil {
				return usageErr(fmt.Errorf("invalid --since %q: %w", since, err))
			}
			cutoff := sinceTimestamp(dur)
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			const q = `query Health($orgId: ID!, $since: String!) {
  workflowExecutionStats(orgId: $orgId, createdSince: $since) {
    succeeded failed running pending delayed paused humanSecondsSaved
  }
}`
			data, err := rewstGraphQL(cmd.Context(), c, q, map[string]any{"orgId": org, "since": cutoff})
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var payload struct {
				Stats wfExecStats `json:"workflowExecutionStats"`
			}
			if len(data) > 0 {
				_ = json.Unmarshal(data, &payload)
			}
			s := payload.Stats
			total := s.Succeeded + s.Failed + s.Running + s.Pending + s.Delayed + s.Paused
			view := healthView{
				Org: org, Since: cutoff,
				Succeeded: s.Succeeded, Failed: s.Failed, Running: s.Running,
				Pending: s.Pending, Delayed: s.Delayed, Paused: s.Paused,
				Total:      total,
				HoursSaved: float64(s.HumanSecondsSaved) / 3600.0,
				Healthy:    s.Failed == 0,
			}
			if view.Healthy {
				view.Verdict = "healthy"
			} else {
				view.Verdict = fmt.Sprintf("%d failed execution(s) in window", s.Failed)
			}
			return emitNovel(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "Org %s — last %s\n", org, since)
				fmt.Fprintf(w, "  succeeded=%d failed=%d running=%d pending=%d delayed=%d paused=%d (total=%d)\n",
					s.Succeeded, s.Failed, s.Running, s.Pending, s.Delayed, s.Paused, total)
				fmt.Fprintf(w, "  time saved: %.1f hours\n", view.HoursSaved)
				fmt.Fprintf(w, "  verdict: %s\n", view.Verdict)
			})
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization id (required)")
	cmd.Flags().StringVar(&since, "since", "24h", "Window for stats (e.g. 24h, 7d, 30d)")
	return cmd
}

// ---- roi ----

type timeSavedRow struct {
	WorkflowID           string `json:"workflowId"`
	WorkflowName         string `json:"workflowName"`
	SecondsSaved         int    `json:"secondsSaved"`
	TotalExecutions      int    `json:"totalExecutions"`
	SuccessfulExecutions int    `json:"successfulExecutions"`
	FailedExecutions     int    `json:"failedExecutions"`
}

type roiTop struct {
	WorkflowID   string  `json:"workflow_id"`
	WorkflowName string  `json:"workflow_name"`
	HoursSaved   float64 `json:"hours_saved"`
	Executions   int     `json:"executions"`
}

type roiView struct {
	Org               string   `json:"org"`
	Since             string   `json:"since"`
	WorkflowCount     int      `json:"workflow_count"`
	TotalSecondsSaved int      `json:"total_seconds_saved"`
	HoursSaved        float64  `json:"hours_saved"`
	DaysSaved         float64  `json:"days_saved"`
	TopWorkflows      []roiTop `json:"top_workflows"`
}

func newRoiCmd(flags *rootFlags) *cobra.Command {
	var org, since string
	var top int
	cmd := &cobra.Command{
		Use:         "roi",
		Short:       "Aggregate automation time saved (humanSecondsSaved) into hours/days for an org",
		Long:        "Sum Rewst's per-workflow time-saved metric across an org into a fleet ROI figure plus the top time-saving workflows. Use this to turn automation into a time/dollar story for a client report or QBR.",
		Example:     "  rewst-cli roi --org 11111111-1111-1111-1111-111111111111 --since 30d --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch timeSavedGroupByWorkflow for the org")
				return nil
			}
			if strings.TrimSpace(org) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--org is required"))
			}
			dur, err := cliutil.ParseDurationLoose(since)
			if err != nil {
				return usageErr(fmt.Errorf("invalid --since %q: %w", since, err))
			}
			cutoff := sinceTimestamp(dur)
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			const q = `query Roi($orgId: ID!, $updatedAt: String!) {
  timeSavedGroupByWorkflow(orgId: $orgId, updatedAt: $updatedAt, useStatsTable: true) {
    workflowId workflowName secondsSaved totalExecutions successfulExecutions failedExecutions
  }
}`
			data, err := rewstGraphQL(cmd.Context(), c, q, map[string]any{"orgId": org, "updatedAt": cutoff})
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var payload struct {
				Rows []timeSavedRow `json:"timeSavedGroupByWorkflow"`
			}
			if len(data) > 0 {
				_ = json.Unmarshal(data, &payload)
			}
			rows := payload.Rows
			sort.SliceStable(rows, func(i, j int) bool { return rows[i].SecondsSaved > rows[j].SecondsSaved })
			totalSecs := 0
			for _, r := range rows {
				totalSecs += r.SecondsSaved
			}
			tops := make([]roiTop, 0, top)
			for i, r := range rows {
				if i >= top {
					break
				}
				tops = append(tops, roiTop{
					WorkflowID:   r.WorkflowID,
					WorkflowName: r.WorkflowName,
					HoursSaved:   float64(r.SecondsSaved) / 3600.0,
					Executions:   r.TotalExecutions,
				})
			}
			view := roiView{
				Org: org, Since: cutoff,
				WorkflowCount:     len(rows),
				TotalSecondsSaved: totalSecs,
				HoursSaved:        float64(totalSecs) / 3600.0,
				DaysSaved:         float64(totalSecs) / 86400.0,
				TopWorkflows:      tops,
			}
			return emitNovel(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "Org %s — time saved over %s: %.1f hours (%.1f days) across %d workflows\n",
					org, since, view.HoursSaved, view.DaysSaved, view.WorkflowCount)
				for _, t := range tops {
					name := t.WorkflowName
					if name == "" {
						name = t.WorkflowID
					}
					fmt.Fprintf(w, "  %-40s %.1fh (%d runs)\n", name, t.HoursSaved, t.Executions)
				}
			})
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization id (required)")
	cmd.Flags().StringVar(&since, "since", "30d", "Window for time-saved stats (e.g. 7d, 30d)")
	cmd.Flags().IntVar(&top, "top", 10, "Number of top workflows to list")
	return cmd
}

// ---- shared execution shape ----

type wfExecution struct {
	ID                       string `json:"id"`
	Status                   string `json:"status"`
	CreatedAt                string `json:"createdAt"`
	NumSuccessfulTasks       int    `json:"numSuccessfulTasks"`
	NumAwaitingResponseTasks int    `json:"numAwaitingResponseTasks"`
	Workflow                 struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"workflow"`
}

// ---- failures ----

type failureRow struct {
	ExecutionID  string `json:"execution_id"`
	WorkflowID   string `json:"workflow_id"`
	WorkflowName string `json:"workflow_name"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
}

type failuresView struct {
	Org               string       `json:"org"`
	Since             string       `json:"since"`
	ScannedExecutions int          `json:"scanned_executions"`
	MaxScan           int          `json:"max_scan"`
	Failures          []failureRow `json:"failures"`
	Note              string       `json:"note,omitempty"`
}

func newFailuresCmd(flags *rootFlags) *cobra.Command {
	var org, since string
	var limit, maxScan int
	cmd := &cobra.Command{
		Use:         "failures",
		Short:       "Recent failed workflow executions for an org, newest first",
		Long:        "Scan recent workflow executions and return only the failed ones (FAILED/TIMEOUT/ABANDONED/CANCELED) within a window. --limit caps matches returned; --max-scan caps executions examined. Use this when something broke and you need the failed runs, not every run.",
		Example:     "  rewst-cli failures --org 11111111-1111-1111-1111-111111111111 --since 12h --limit 20 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan recent workflowExecutions and filter to failures")
				return nil
			}
			if strings.TrimSpace(org) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--org is required"))
			}
			dur, err := cliutil.ParseDurationLoose(since)
			if err != nil {
				return usageErr(fmt.Errorf("invalid --since %q: %w", since, err))
			}
			if cliutil.IsDogfoodEnv() && maxScan > 50 {
				maxScan = 50
			}
			cutoff := time.Now().Add(-dur)
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			const q = `query Failures($orgId: ID!, $limit: Int) {
  workflowExecutions(where: {orgId: $orgId}, limit: $limit, order: [["createdAt", "DESC"]]) {
    id status createdAt numSuccessfulTasks numAwaitingResponseTasks workflow { id name }
  }
}`
			data, err := rewstGraphQL(cmd.Context(), c, q, map[string]any{"orgId": org, "limit": maxScan})
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var payload struct {
				Execs []wfExecution `json:"workflowExecutions"`
			}
			if len(data) > 0 {
				_ = json.Unmarshal(data, &payload)
			}
			matches := make([]failureRow, 0)
			scanned := 0
			for _, e := range payload.Execs {
				scanned++
				if !failedExecutionStatus(e.Status) {
					continue
				}
				if t, ok := parseFlexibleTime(e.CreatedAt); ok && t.Before(cutoff) {
					continue
				}
				matches = append(matches, failureRow{
					ExecutionID:  e.ID,
					WorkflowID:   e.Workflow.ID,
					WorkflowName: e.Workflow.Name,
					Status:       e.Status,
					CreatedAt:    e.CreatedAt,
				})
				if len(matches) >= limit {
					break
				}
			}
			view := failuresView{
				Org: org, Since: cutoff.UTC().Format(time.RFC3339),
				ScannedExecutions: scanned, MaxScan: maxScan, Failures: matches,
			}
			if len(matches) == 0 && scanned >= maxScan {
				view.Note = fmt.Sprintf("scanned %d executions (--max-scan cap) without finding a failure in the window; raise --max-scan or widen --since", scanned)
			}
			return emitNovel(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "Org %s — %d failed execution(s) in last %s (scanned %d)\n", org, len(matches), since, scanned)
				for _, m := range matches {
					name := m.WorkflowName
					if name == "" {
						name = m.WorkflowID
					}
					fmt.Fprintf(w, "  %-12s %-40s %s\n", m.Status, name, m.CreatedAt)
				}
				if view.Note != "" {
					fmt.Fprintf(w, "  note: %s\n", view.Note)
				}
			})
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization id (required)")
	cmd.Flags().StringVar(&since, "since", "24h", "Only failures newer than this (e.g. 12h, 24h, 7d)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Max failures to return")
	cmd.Flags().IntVar(&maxScan, "max-scan", 200, "Max recent executions to examine")
	return cmd
}

// ---- dormant ----

type workflowRow struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	UpdatedAt         string `json:"updatedAt"`
	HumanSecondsSaved int    `json:"humanSecondsSaved"`
}

type dormantEntry struct {
	WorkflowID        string `json:"workflow_id"`
	WorkflowName      string `json:"workflow_name"`
	HumanSecondsSaved int    `json:"human_seconds_saved"`
	UpdatedAt         string `json:"updated_at"`
}

type dormantView struct {
	Org               string         `json:"org"`
	Days              int            `json:"days"`
	WorkflowCount     int            `json:"workflow_count"`
	ScannedExecutions int            `json:"scanned_executions"`
	Dormant           []dormantEntry `json:"dormant"`
	Note              string         `json:"note,omitempty"`
}

func newDormantCmd(flags *rootFlags) *cobra.Command {
	var org string
	var days, limit, maxScan int
	cmd := &cobra.Command{
		Use:         "dormant",
		Short:       "Workflows with no execution in the last N days (dead automation)",
		Long:        "Cross-reference an org's workflows against recent executions to find automation that is installed but no longer running. Use this to clean up or re-enable workflows that silently stopped firing after a trigger or integration broke.",
		Example:     "  rewst-cli dormant --org 11111111-1111-1111-1111-111111111111 --days 30 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch workflows and recent executions to find dormant workflows")
				return nil
			}
			if strings.TrimSpace(org) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--org is required"))
			}
			if cliutil.IsDogfoodEnv() && maxScan > 50 {
				maxScan = 50
			}
			cutoff := time.Now().AddDate(0, 0, -days)
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			const qWorkflows = `query Workflows($orgId: ID!, $limit: Int) {
  workflows(where: {orgId: $orgId}, limit: $limit) { id name updatedAt humanSecondsSaved }
}`
			wfData, err := rewstGraphQL(cmd.Context(), c, qWorkflows, map[string]any{"orgId": org, "limit": limit})
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var wfPayload struct {
				Workflows []workflowRow `json:"workflows"`
			}
			if len(wfData) > 0 {
				_ = json.Unmarshal(wfData, &wfPayload)
			}
			const qExecs = `query RecentExecs($orgId: ID!, $limit: Int) {
  workflowExecutions(where: {orgId: $orgId}, limit: $limit, order: [["createdAt", "DESC"]]) {
    id createdAt workflow { id }
  }
}`
			exData, err := rewstGraphQL(cmd.Context(), c, qExecs, map[string]any{"orgId": org, "limit": maxScan})
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var exPayload struct {
				Execs []wfExecution `json:"workflowExecutions"`
			}
			if len(exData) > 0 {
				_ = json.Unmarshal(exData, &exPayload)
			}
			active := make(map[string]struct{})
			scanned := 0
			for _, e := range exPayload.Execs {
				scanned++
				if e.Workflow.ID == "" {
					continue
				}
				// Treat unparseable timestamps as recent (avoid false dormant).
				if t, ok := parseFlexibleTime(e.CreatedAt); ok && t.Before(cutoff) {
					continue
				}
				active[e.Workflow.ID] = struct{}{}
			}
			dormant := make([]dormantEntry, 0)
			for _, w := range wfPayload.Workflows {
				if _, ok := active[w.ID]; ok {
					continue
				}
				dormant = append(dormant, dormantEntry{
					WorkflowID:        w.ID,
					WorkflowName:      w.Name,
					HumanSecondsSaved: w.HumanSecondsSaved,
					UpdatedAt:         w.UpdatedAt,
				})
			}
			view := dormantView{
				Org: org, Days: days,
				WorkflowCount:     len(wfPayload.Workflows),
				ScannedExecutions: scanned,
				Dormant:           dormant,
			}
			if scanned >= maxScan {
				view.Note = fmt.Sprintf("execution scan hit the --max-scan cap of %d; a very active org may misreport recently-run workflows as dormant. Raise --max-scan to widen.", maxScan)
			}
			return emitNovel(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "Org %s — %d dormant workflow(s) of %d (no run in %dd, scanned %d executions)\n",
					org, len(dormant), view.WorkflowCount, days, scanned)
				for _, dEntry := range dormant {
					name := dEntry.WorkflowName
					if name == "" {
						name = dEntry.WorkflowID
					}
					fmt.Fprintf(w, "  %s\n", name)
				}
				if view.Note != "" {
					fmt.Fprintf(w, "  note: %s\n", view.Note)
				}
			})
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Organization id (required)")
	cmd.Flags().IntVar(&days, "days", 30, "Consider a workflow dormant after this many days without an execution")
	cmd.Flags().IntVar(&limit, "limit", 500, "Max workflows to examine")
	cmd.Flags().IntVar(&maxScan, "max-scan", 500, "Max recent executions to scan for activity")
	return cmd
}

// ---- drift ----

type driftView struct {
	Org          string   `json:"org"`
	Against      string   `json:"against"`
	VarsOnlyOrg  []string `json:"variables_only_in_org"`
	VarsOnlyAgn  []string `json:"variables_only_in_against"`
	VarsShared   int      `json:"variables_shared"`
	PacksOnlyOrg []string `json:"packs_only_in_org"`
	PacksOnlyAgn []string `json:"packs_only_in_against"`
	PacksShared  int      `json:"packs_shared"`
	FetchErrors  []string `json:"fetch_errors,omitempty"`
}

func fetchVarNames(ctx context.Context, c *client.Client, org string) (map[string]struct{}, error) {
	const q = `query OrgVars($orgId: ID!) { orgVariables(where: {orgId: $orgId}, maskSecrets: true) { name } }`
	data, err := rewstGraphQL(ctx, c, q, map[string]any{"orgId": org})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Vars []struct {
			Name string `json:"name"`
		} `json:"orgVariables"`
	}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &payload)
	}
	set := make(map[string]struct{}, len(payload.Vars))
	for _, v := range payload.Vars {
		if v.Name != "" {
			set[v.Name] = struct{}{}
		}
	}
	return set, nil
}

func fetchPackRefs(ctx context.Context, c *client.Client, org string) (map[string]struct{}, error) {
	const q = `query OrgPacks($orgId: ID!) { packsForOrg(orgId: $orgId) { name ref } }`
	data, err := rewstGraphQL(ctx, c, q, map[string]any{"orgId": org})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Packs []struct {
			Name string `json:"name"`
			Ref  string `json:"ref"`
		} `json:"packsForOrg"`
	}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &payload)
	}
	set := make(map[string]struct{}, len(payload.Packs))
	for _, p := range payload.Packs {
		key := p.Ref
		if key == "" {
			key = p.Name
		}
		if key != "" {
			set[key] = struct{}{}
		}
	}
	return set, nil
}

func diffKeys(a, b map[string]struct{}) (onlyA, onlyB []string, shared int) {
	onlyA = make([]string, 0)
	onlyB = make([]string, 0)
	for k := range a {
		if _, ok := b[k]; ok {
			shared++
		} else {
			onlyA = append(onlyA, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			onlyB = append(onlyB, k)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	return onlyA, onlyB, shared
}

func newDriftCmd(flags *rootFlags) *cobra.Command {
	var org, against string
	cmd := &cobra.Command{
		Use:         "drift",
		Short:       "Compare org variables and installed packs between two organizations",
		Long:        "Diff the configuration of two tenants — org variable names (secret values are masked and never compared) and installed pack refs — to explain why automation works in one org but not another.",
		Example:     "  rewst-cli drift --org 11111111-1111-1111-1111-111111111111 --against 22222222-2222-2222-2222-222222222222 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch variables and packs for both orgs and diff them")
				return nil
			}
			if strings.TrimSpace(org) == "" || strings.TrimSpace(against) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("both --org and --against are required"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			var fetchErrs []string
			varsOrg, err := fetchVarNames(ctx, c, org)
			if err != nil {
				fetchErrs = append(fetchErrs, fmt.Sprintf("variables(%s): %v", org, err))
				varsOrg = map[string]struct{}{}
			}
			varsAgn, err := fetchVarNames(ctx, c, against)
			if err != nil {
				fetchErrs = append(fetchErrs, fmt.Sprintf("variables(%s): %v", against, err))
				varsAgn = map[string]struct{}{}
			}
			packsOrg, err := fetchPackRefs(ctx, c, org)
			if err != nil {
				fetchErrs = append(fetchErrs, fmt.Sprintf("packs(%s): %v", org, err))
				packsOrg = map[string]struct{}{}
			}
			packsAgn, err := fetchPackRefs(ctx, c, against)
			if err != nil {
				fetchErrs = append(fetchErrs, fmt.Sprintf("packs(%s): %v", against, err))
				packsAgn = map[string]struct{}{}
			}
			// If every fetch failed (e.g. no credential), surface as an error.
			if len(fetchErrs) == 4 {
				return classifyAPIError(fmt.Errorf("%s", strings.Join(fetchErrs, "; ")), flags)
			}
			vOnlyOrg, vOnlyAgn, vShared := diffKeys(varsOrg, varsAgn)
			pOnlyOrg, pOnlyAgn, pShared := diffKeys(packsOrg, packsAgn)
			view := driftView{
				Org: org, Against: against,
				VarsOnlyOrg: vOnlyOrg, VarsOnlyAgn: vOnlyAgn, VarsShared: vShared,
				PacksOnlyOrg: pOnlyOrg, PacksOnlyAgn: pOnlyAgn, PacksShared: pShared,
				FetchErrors: fetchErrs,
			}
			return emitNovel(cmd, flags, view, func(w io.Writer) {
				fmt.Fprintf(w, "Drift %s vs %s\n", org, against)
				fmt.Fprintf(w, "  variables: %d only in org, %d only in against, %d shared\n", len(vOnlyOrg), len(vOnlyAgn), vShared)
				fmt.Fprintf(w, "  packs:     %d only in org, %d only in against, %d shared\n", len(pOnlyOrg), len(pOnlyAgn), pShared)
				if len(vOnlyOrg) > 0 {
					fmt.Fprintf(w, "  variables only in %s: %s\n", org, strings.Join(vOnlyOrg, ", "))
				}
				if len(vOnlyAgn) > 0 {
					fmt.Fprintf(w, "  variables only in %s: %s\n", against, strings.Join(vOnlyAgn, ", "))
				}
			})
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "First organization id (required)")
	cmd.Flags().StringVar(&against, "against", "", "Second organization id to compare against (required)")
	return cmd
}

// ---- coverage ----

type orgPackEntry struct {
	OrgID   string   `json:"org_id"`
	OrgName string   `json:"org_name"`
	Packs   []string `json:"packs"`
}

type packCoverage struct {
	Pack             string `json:"pack"`
	InstalledInCount int    `json:"installed_in_count"`
}

type coverageView struct {
	Parent      string         `json:"parent"`
	ScannedOrgs int            `json:"scanned_orgs"`
	PackFilter  string         `json:"pack_filter,omitempty"`
	OrgsWith    []string       `json:"orgs_with,omitempty"`
	OrgsMissing []string       `json:"orgs_missing,omitempty"`
	Coverage    []packCoverage `json:"coverage,omitempty"`
}

func newCoverageCmd(flags *rootFlags) *cobra.Command {
	var parent, packFilter string
	var limit int
	cmd := &cobra.Command{
		Use:         "coverage",
		Short:       "Pack/integration coverage across a parent org's managed sub-orgs",
		Long:        "Show which integration packs are installed across all managed sub-organizations of a parent org. With --pack, report which orgs have or are missing a specific pack; without it, summarize coverage per pack. Use this to confirm an integration is rolled out to every client tenant.",
		Example:     "  rewst-cli coverage --parent 11111111-1111-1111-1111-111111111111 --pack microsoft --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch managed sub-orgs and their installed packs")
				return nil
			}
			if strings.TrimSpace(parent) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--parent is required"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			const q = `query Coverage($parentOrgId: ID!, $limit: Int) {
  managedAndSubOrganizations(parentOrgId: $parentOrgId, limit: $limit) {
    id name installedPacks { name ref }
  }
}`
			data, err := rewstGraphQL(cmd.Context(), c, q, map[string]any{"parentOrgId": parent, "limit": limit})
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var payload struct {
				Orgs []struct {
					ID             string `json:"id"`
					Name           string `json:"name"`
					InstalledPacks []struct {
						Name string `json:"name"`
						Ref  string `json:"ref"`
					} `json:"installedPacks"`
				} `json:"managedAndSubOrganizations"`
			}
			if len(data) > 0 {
				_ = json.Unmarshal(data, &payload)
			}
			view := coverageView{Parent: parent, ScannedOrgs: len(payload.Orgs)}
			if strings.TrimSpace(packFilter) != "" {
				needle := strings.ToLower(packFilter)
				view.PackFilter = packFilter
				view.OrgsWith = make([]string, 0)
				view.OrgsMissing = make([]string, 0)
				for _, o := range payload.Orgs {
					label := o.Name
					if label == "" {
						label = o.ID
					}
					has := false
					for _, p := range o.InstalledPacks {
						if strings.Contains(strings.ToLower(p.Name), needle) || strings.Contains(strings.ToLower(p.Ref), needle) {
							has = true
							break
						}
					}
					if has {
						view.OrgsWith = append(view.OrgsWith, label)
					} else {
						view.OrgsMissing = append(view.OrgsMissing, label)
					}
				}
				sort.Strings(view.OrgsWith)
				sort.Strings(view.OrgsMissing)
			} else {
				counts := make(map[string]int)
				for _, o := range payload.Orgs {
					seen := make(map[string]struct{})
					for _, p := range o.InstalledPacks {
						key := p.Ref
						if key == "" {
							key = p.Name
						}
						if key == "" {
							continue
						}
						if _, dup := seen[key]; dup {
							continue
						}
						seen[key] = struct{}{}
						counts[key]++
					}
				}
				cov := make([]packCoverage, 0, len(counts))
				for k, n := range counts {
					cov = append(cov, packCoverage{Pack: k, InstalledInCount: n})
				}
				sort.SliceStable(cov, func(i, j int) bool { return cov[i].InstalledInCount > cov[j].InstalledInCount })
				view.Coverage = cov
			}
			return emitNovel(cmd, flags, view, func(w io.Writer) {
				if view.PackFilter != "" {
					fmt.Fprintf(w, "Pack %q across %d managed org(s): %d have it, %d missing\n",
						view.PackFilter, view.ScannedOrgs, len(view.OrgsWith), len(view.OrgsMissing))
					if len(view.OrgsMissing) > 0 {
						fmt.Fprintf(w, "  missing: %s\n", strings.Join(view.OrgsMissing, ", "))
					}
					return
				}
				fmt.Fprintf(w, "Pack coverage across %d managed org(s)\n", view.ScannedOrgs)
				for _, cEntry := range view.Coverage {
					fmt.Fprintf(w, "  %-40s %d/%d orgs\n", cEntry.Pack, cEntry.InstalledInCount, view.ScannedOrgs)
				}
			})
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "Parent organization id whose managed sub-orgs to scan (required)")
	cmd.Flags().StringVar(&packFilter, "pack", "", "Report coverage for a specific pack (name or ref substring)")
	cmd.Flags().IntVar(&limit, "limit", 500, "Max managed sub-orgs to scan")
	return cmd
}
