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
	"immybot-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type triageCluster struct {
	Action        string   `json:"action"`
	Status        string   `json:"status"`
	Reason        string   `json:"reason"`
	ComputerCount int      `json:"computer_count"`
	TenantCount   int      `json:"tenant_count"`
	ActionCount   int      `json:"action_count"`
	Tenants       []string `json:"tenants"`
	Computers     []string `json:"computers"`
	SessionIDs    []string `json:"session_ids"`
	FirstSeen     string   `json:"first_seen,omitempty"`
	LastSeen      string   `json:"last_seen,omitempty"`
}

type triageView struct {
	Since          string          `json:"since,omitempty"`
	ScannedActions int             `json:"scanned_actions"`
	FailedActions  int             `json:"failed_actions"`
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
			"same error on forty machines.\n\n" +
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

			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "maintenance-actions") {
				hintIfStale(cmd, db, "maintenance-actions", flags.maxAge)
			}

			rows, err := db.DB().QueryContext(ctx, `
				SELECT
					COALESCE(json_extract(data,'$.maintenanceDisplayName'), json_extract(data,'$.actionTypeName'), json_extract(data,'$.actionType')),
					COALESCE(json_extract(data,'$.statusName'), json_extract(data,'$.status')),
					COALESCE(json_extract(data,'$.reason'), json_extract(data,'$.result')),
					json_extract(data,'$.computerName'),
					json_extract(data,'$.tenantName'),
					json_extract(data,'$.maintenanceSessionId'),
					COALESCE(json_extract(data,'$.startTime'), json_extract(data,'$.endTime'))
				FROM resources
				WHERE resource_type IN ('maintenance-actions','maintenance-actions-maintenance-item','maintenance-actions-version')`)
			if err != nil {
				return fmt.Errorf("querying maintenance actions: %w", err)
			}

			type actionRow struct {
				action, status, reason, computer, tenant, session, ts string
			}
			// Drain the parent result set fully before any aggregation. SQLite
			// uses a single connection, so a follow-up query while these rows
			// are open would block.
			scanned := make([]actionRow, 0)
			for rows.Next() {
				var action, status, reason, computer, tenant, session, ts sql.NullString
				if err := rows.Scan(&action, &status, &reason, &computer, &tenant, &session, &ts); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning maintenance action: %w", err)
				}
				scanned = append(scanned, actionRow{
					action:   nullStr(action),
					status:   nullStr(status),
					reason:   nullStr(reason),
					computer: nullStr(computer),
					tenant:   nullStr(tenant),
					session:  nullStr(session),
					ts:       nullStr(ts),
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
				cluster   triageCluster
				computers map[string]struct{}
				tenants   map[string]struct{}
				sessions  map[string]struct{}
			}
			groups := map[string]*acc{}

			for _, r := range scanned {
				view.ScannedActions++
				if tenantFilter != "" && !strings.Contains(strings.ToLower(r.tenant), tenantFilter) {
					continue
				}
				if wantStatus != "" {
					if !strings.EqualFold(strings.TrimSpace(r.status), wantStatus) {
						continue
					}
				} else if !immyIsFailureStatus(r.status) {
					continue
				}
				if !cutoff.IsZero() && r.ts != "" {
					if t, err := time.Parse(time.RFC3339, r.ts); err == nil && t.Before(cutoff) {
						continue
					}
				}
				view.FailedActions++

				action := r.action
				if action == "" {
					action = "unknown"
				}
				reason := immyNormalizeReason(r.reason)
				key := action + "\x00" + r.status + "\x00" + reason
				a := groups[key]
				if a == nil {
					a = &acc{
						cluster:   triageCluster{Action: action, Status: r.status, Reason: reason},
						computers: map[string]struct{}{},
						tenants:   map[string]struct{}{},
						sessions:  map[string]struct{}{},
					}
					groups[key] = a
				}
				a.cluster.ActionCount++
				if r.computer != "" {
					a.computers[r.computer] = struct{}{}
				}
				if r.tenant != "" {
					a.tenants[r.tenant] = struct{}{}
				}
				if r.session != "" {
					a.sessions[r.session] = struct{}{}
				}
				if r.ts != "" {
					if a.cluster.FirstSeen == "" || r.ts < a.cluster.FirstSeen {
						a.cluster.FirstSeen = r.ts
					}
					if r.ts > a.cluster.LastSeen {
						a.cluster.LastSeen = r.ts
					}
				}
			}

			for _, a := range groups {
				a.cluster.ComputerCount = len(a.computers)
				a.cluster.TenantCount = len(a.tenants)
				a.cluster.Computers = sortedSetSlice(a.computers)
				a.cluster.Tenants = sortedSetSlice(a.tenants)
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
			} else if view.FailedActions == 0 {
				view.Note = fmt.Sprintf("scanned %d maintenance actions and found no failures matching the filters", view.ScannedActions)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Clusters) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No failed maintenance actions found (scanned %d).\n", view.ScannedActions)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-6s %-6s %-32s %s\n", "MACHS", "TENTS", "ACTION", "REASON")
			for _, c := range view.Clusters {
				fmt.Fprintf(cmd.OutOrStdout(), "%-6d %-6d %-32s %s\n",
					c.ComputerCount, c.TenantCount, immyTruncate(c.Action, 32), immyTruncate(c.Reason, 80))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d distinct failure cause(s) across %d failed action(s); %d action(s) scanned.\n",
				len(view.Clusters), view.FailedActions, view.ScannedActions)
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only consider actions newer than this age (e.g. 24h, 7d, 1w)")
	cmd.Flags().StringVar(&flagStatus, "status", "", "Cluster actions with this exact status name instead of the default failure statuses")
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
