// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: maintenance failure clustering.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"immybot-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

// triageCluster is one root cause. MatchedActions counts the cluster's actions
// the filters selected; it equals action_count and mirrors the view's
// matched_actions. FirstSeen and LastSeen are the earliest and latest action
// time (immyActionTimeFields) as mirrored, compared as instants; an action with
// no usable time does not move them.
type triageCluster struct {
	Action         string   `json:"action"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason"`
	ComputerCount  int      `json:"computer_count"`
	TenantCount    int      `json:"tenant_count"`
	ActionCount    int      `json:"action_count"`
	MatchedActions int      `json:"matched_actions"`
	Tenants        []string `json:"tenants"`
	Computers      []string `json:"computers"`
	SessionIDs     []string `json:"session_ids"`
	FirstSeen      string   `json:"first_seen,omitempty"`
	LastSeen       string   `json:"last_seen,omitempty"`
}

// triageView is the session-triage result. MatchedActions counts the actions
// the filters selected (the failures by default, or what --status names, which
// can be successes); FailedActions counts only the matched actions whose
// outcome is a failure, so --status Success never reports a success as failed.
// UndatedActions counts the actions the other filters selected that --since
// left out because none of endTime, startTime, updatedDateUTC or createdDateUTC
// parses, so their time is unknown.
type triageView struct {
	Since          string          `json:"since,omitempty"`
	ScannedActions int             `json:"scanned_actions"`
	MatchedActions int             `json:"matched_actions"`
	FailedActions  int             `json:"failed_actions"`
	UndatedActions int             `json:"undated_actions"`
	Clusters       []triageCluster `json:"clusters"`
	Note           string          `json:"note,omitempty"`
}

func newNovelSessionTriageCmd(flags *rootFlags) *cobra.Command {
	var (
		flagSince  string
		flagStatus string
		flagTenant string
		flagLimit  int
		dbPath     string
	)

	cmd := &cobra.Command{
		Use:   "session-triage",
		Short: "Group recent failed maintenance actions by root cause",
		Long: "Group last night's failed maintenance actions by root cause instead of reading the " +
			"same error on forty machines. --status clusters the actions with that outcome or " +
			"lifecycle instead; matched_actions counts what the filters selected and failed_actions " +
			"only the failures among them.\n\n" +
			"An action's time is its endTime, else startTime, else updatedDateUTC, else createdDateUTC. " +
			"--since keeps only actions whose time is known and inside the window; an action with none " +
			"of them readable is left out and counted in undated_actions.\n\n" +
			"Use this command for grouping recent maintenance failures by root cause across sessions " +
			"and tenants. Do NOT use this command for explaining which deployments apply to one " +
			"computer; use 'assignment-explain' instead.",
		Example: strings.Trim(`
  immybot-cli session-triage --since 24h
  immybot-cli session-triage --since 7d --tenant "Contoso" --agent
  immybot-cli session-triage --since 24h --agent --select clusters.reason,clusters.computer_count
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--since=24h",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "session-triage")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			var cutoff time.Time
			if s := strings.TrimSpace(flagSince); s != "" {
				d, err := cliutil.ParseDurationLoose(s)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--since %q: %w", s, err))
				}
				cutoff = time.Now().UTC().Add(-d)
			}

			dbPath = immyMirrorPath(dbPath)
			view := triageView{Since: strings.TrimSpace(flagSince), Clusters: make([]triageCluster, 0)}
			if immyMirrorMissing(cmd, dbPath, "maintenance-actions") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}

			db, err := immyOpenMirror(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "maintenance-actions") {
				hintIfStale(cmd, db, "maintenance-actions", flags.maxAge)
			}

			rows, err := db.DB().QueryContext(ctx, `
				SELECT id,
					COALESCE(json_extract(data,'$.maintenanceDisplayName'), json_extract(data,'$.actionTypeName'), json_extract(data,'$.actionType')),
					`+immyActionOutcomeColumns+`,
					json_extract(data,'$.computerName'),
					json_extract(data,'$.tenantName'),
					json_extract(data,'$.maintenanceSessionId'),
					json_extract(data,'$.computerId'),
					json_extract(data,'$.tenantId'),
					`+immyTimeColumns(immyActionTimeFields)+`
				`+immyActionFrom+immyActionOrder)
			if err != nil {
				return fmt.Errorf("querying maintenance actions: %w", err)
			}

			type actionRow struct {
				action, computer, tenant, session string
				computerKey, tenantKey            string
				at                                immyRowTime
				outcome                           immyActionOutcome
			}
			// Drain the parent result set fully before any aggregation. SQLite
			// uses a single connection, so a follow-up query while these rows
			// are open would block.
			scanned := make([]actionRow, 0)
			seen := map[string]struct{}{}
			for rows.Next() {
				var rowID, action, computer, tenant, session, computerID, tenantID sql.NullString
				var oc immyActionOutcomeRow
				ts := newImmyTimeScan(immyActionTimeFields)
				dest := append([]any{&rowID, &action}, oc.dest()...)
				dest = append(dest, &computer, &tenant, &session, &computerID, &tenantID)
				dest = append(dest, ts.dest()...)
				if err := rows.Scan(dest...); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning maintenance action: %w", err)
				}
				if immyActionSeen(seen, rowID) {
					continue
				}
				scanned = append(scanned, actionRow{
					action:   nullStr(action),
					outcome:  oc.outcome(),
					computer: nullStr(computer),
					tenant:   nullStr(tenant),
					session:  nullStr(session),
					at:       ts.time(),
					// Counted by id (GetMaintenanceActionResponse.computerId,
					// tenantId): names repeat across tenants.
					computerKey: immyComputerCountKey(nullStr(computerID), len(scanned)),
					tenantKey:   immyTenantCountKey(nullStr(tenantID), nullStr(tenant)),
				})
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("iterating maintenance actions: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("closing maintenance actions: %w", err)
			}

			wantStatus := strings.ToLower(strings.TrimSpace(flagStatus))
			tenantFilter := strings.ToLower(strings.TrimSpace(flagTenant))

			type acc struct {
				cluster     triageCluster
				computers   map[string]struct{}
				tenants     map[string]struct{}
				names       map[string]struct{}
				tenantNames map[string]struct{}
				sessions    map[string]struct{}
				first, last time.Time
			}
			groups := map[string]*acc{}

			for _, r := range scanned {
				view.ScannedActions++
				if tenantFilter != "" && !strings.Contains(strings.ToLower(r.tenant), tenantFilter) {
					continue
				}
				if !r.outcome.Selected(wantStatus) {
					continue
				}
				if !cutoff.IsZero() && !r.at.NotBefore(cutoff) {
					if !r.at.Known {
						view.UndatedActions++
					}
					continue
				}
				view.MatchedActions++
				if r.outcome.Failed {
					view.FailedActions++
				}

				action := r.action
				if action == "" {
					action = "unknown"
				}
				reason := immyNormalizeReasonFor(r.outcome.Cause, r.computer)
				key := action + "\x00" + r.outcome.Label + "\x00" + reason
				a := groups[key]
				if a == nil {
					a = &acc{
						cluster:     triageCluster{Action: action, Status: r.outcome.Label, Reason: reason},
						computers:   map[string]struct{}{},
						tenants:     map[string]struct{}{},
						names:       map[string]struct{}{},
						tenantNames: map[string]struct{}{},
						sessions:    map[string]struct{}{},
					}
					groups[key] = a
				}
				a.cluster.ActionCount++
				a.cluster.MatchedActions++
				a.computers[r.computerKey] = struct{}{}
				if r.tenantKey != "" {
					a.tenants[r.tenantKey] = struct{}{}
				}
				if r.computer != "" {
					a.names[r.computer] = struct{}{}
				}
				if r.tenant != "" {
					a.tenantNames[r.tenant] = struct{}{}
				}
				if r.session != "" {
					a.sessions[r.session] = struct{}{}
				}
				if r.at.Known {
					if a.cluster.FirstSeen == "" || r.at.At.Before(a.first) {
						a.first, a.cluster.FirstSeen = r.at.At, r.at.Raw
					}
					if a.cluster.LastSeen == "" || r.at.At.After(a.last) {
						a.last, a.cluster.LastSeen = r.at.At, r.at.Raw
					}
				}
			}

			for _, a := range groups {
				a.cluster.ComputerCount = len(a.computers)
				a.cluster.TenantCount = len(a.tenants)
				a.cluster.Computers = sortedSetSlice(a.names)
				a.cluster.Tenants = sortedSetSlice(a.tenantNames)
				a.cluster.SessionIDs = sortedSetSlice(a.sessions)
				view.Clusters = append(view.Clusters, a.cluster)
			}
			sort.Slice(view.Clusters, func(i, j int) bool {
				if view.Clusters[i].ComputerCount != view.Clusters[j].ComputerCount {
					return view.Clusters[i].ComputerCount > view.Clusters[j].ComputerCount
				}
				if view.Clusters[i].ActionCount != view.Clusters[j].ActionCount {
					return view.Clusters[i].ActionCount > view.Clusters[j].ActionCount
				}
				return view.Clusters[i].Action < view.Clusters[j].Action
			})
			if flagLimit > 0 && len(view.Clusters) > flagLimit {
				view.Clusters = view.Clusters[:flagLimit]
			}
			if view.ScannedActions == 0 {
				view.Note = "no maintenance actions in the local mirror; run 'immybot-cli sync --resources maintenance-actions'"
			} else if view.MatchedActions == 0 && wantStatus != "" {
				view.Note = fmt.Sprintf("scanned %d maintenance actions and found none with --status %s matching the filters",
					view.ScannedActions, strings.TrimSpace(flagStatus))
			} else if view.MatchedActions == 0 {
				view.Note = fmt.Sprintf("scanned %d maintenance actions and found no failures matching the filters", view.ScannedActions)
			}
			if view.UndatedActions > 0 {
				msg := fmt.Sprintf("%d action(s) the other filters selected have no endTime, startTime, updatedDateUTC or "+
					"createdDateUTC that parses, so --since could not place them and left them out; run without --since to see them",
					view.UndatedActions)
				view.Note = strings.TrimPrefix(view.Note+"; "+msg, "; ")
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.UndatedActions > 0 {
				// Printed last, after whichever summary below returns.
				defer fmt.Fprintf(cmd.OutOrStdout(), "%d action(s) left out: no usable timestamp, so --since could not place them.\n", view.UndatedActions)
			}
			if len(view.Clusters) == 0 {
				if wantStatus != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "No maintenance actions matched --status %s (scanned %d).\n",
						strings.TrimSpace(flagStatus), view.ScannedActions)
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "No failed maintenance actions found (scanned %d).\n", view.ScannedActions)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-6s %-6s %-32s %s\n", "MACHS", "TENTS", "ACTION", "REASON")
			for _, c := range view.Clusters {
				fmt.Fprintf(cmd.OutOrStdout(), "%-6d %-6d %-32s %s\n",
					c.ComputerCount, c.TenantCount, immyTruncate(c.Action, 32), immyTruncate(c.Reason, 80))
			}
			// --status can select successes: the summary calls the selection
			// matched and names as failed only the failures inside it.
			if wantStatus != "" {
				failed := ""
				if view.FailedActions > 0 {
					failed = fmt.Sprintf(", %d of them failed", view.FailedActions)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d distinct cause(s) across %d action(s) matched by --status %s%s; %d action(s) scanned.\n",
					len(view.Clusters), view.MatchedActions, strings.TrimSpace(flagStatus), failed, view.ScannedActions)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d distinct failure cause(s) across %d failed action(s); %d action(s) scanned.\n",
				len(view.Clusters), view.FailedActions, view.ScannedActions)
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only consider actions newer than this age (e.g. 24h, 7d, 1w); an action with no readable endTime, startTime, updatedDateUTC or createdDateUTC never matches and is counted in undated_actions")
	cmd.Flags().StringVar(&flagStatus, "status", "", "Cluster actions whose outcome (resultName, e.g. Cancelled) or lifecycle status (statusName, e.g. Skipped) is this name instead of the default failures; they count in matched_actions, and only real failures in failed_actions")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only consider actions whose tenant name contains this substring")
	cmd.Flags().IntVar(&flagLimit, "limit", 20, "Maximum clusters to return (0 for all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

// sortedSetSlice returns the members of a set in stable sorted order so JSON
// output does not churn between runs on identical data.
func sortedSetSlice(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// immyTenantCountKey identifies a tenant for a distinct count: its id, else its
// name; "" when the row names neither.
func immyTenantCountKey(tenantID, tenantName string) string {
	if id := strings.TrimSpace(tenantID); id != "" {
		return "id:" + id
	}
	if name := strings.TrimSpace(tenantName); name != "" {
		return "name:" + name
	}
	return ""
}
