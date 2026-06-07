// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live
package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Data-source strategy: Tactical RMM's `pendingactions` is NOT one of the
// default-synced resources (see defaultSyncResources()), so the local store
// has no fleet-wide pending-actions snapshot to read. This command therefore
// does a read-only live fan-out: it takes the agent IDs from the local
// `agents` resource and calls GET /agents/{agent_id}/pendingactions/ for each
// agent, capped by --max-scan-agents. The JSON envelope reports scanned_agents
// so the caller knows the fan-out was bounded. With an empty store / no API
// key it returns an honest empty result (no fabricated rows).

func newNovelActionsPendingCmd(flags *rootFlags) *cobra.Command {
	var maxScan int
	cmd := &cobra.Command{
		Use:         "pending",
		Short:       "Cross-fleet pending-actions backlog grouped by agent and age",
		Long:        "Lists queued (pending) agent actions across the fleet so stuck dispatches surface. Pending actions are not stored locally, so this fans out live to GET /agents/{agent_id}/pendingactions/ for each synced agent, capped by --max-scan-agents. Read-only.",
		Example:     "  tactical-rmm-cli actions pending --json\n  tactical-rmm-cli actions pending --max-scan-agents 100",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			type item struct {
				AgentID  string `json:"agent_id"`
				Hostname string `json:"hostname,omitempty"`
				Action   string `json:"action,omitempty"`
				Status   string `json:"status,omitempty"`
				AgeHours int    `json:"age_hours,omitempty"`
				Due      string `json:"due,omitempty"`
			}
			items := make([]item, 0)
			envelope := map[string]interface{}{"items": items, "scanned_agents": 0, "total_pending": 0, "fetch_failures": 0}

			// Resolve agent IDs from the local store.
			type agentRef struct{ id, host string }
			refs := make([]agentRef, 0)
			if s := novelLocalRead(cmd, flags, "agents"); s != nil {
				defer s.Close()
				if rows, qe := s.DB().QueryContext(cmd.Context(), "SELECT json_extract(data,'$.agent_id'),json_extract(data,'$.hostname') FROM resources WHERE resource_type='agents'"); qe == nil {
					defer rows.Close()
					for rows.Next() {
						var id, h sql.NullString
						if rows.Scan(&id, &h) == nil && id.String != "" {
							refs = append(refs, agentRef{id.String, h.String})
						}
					}
				}
			}
			if len(refs) == 0 {
				envelope["note"] = "no agents in local store; run 'tactical-rmm-cli sync' first"
				return flags.printJSON(cmd, envelope)
			}
			if maxScan > 0 && len(refs) > maxScan {
				refs = refs[:maxScan]
				envelope["note"] = "scan capped by --max-scan-agents"
			}

			c, err := flags.newClient()
			if err != nil {
				// No usable client (e.g. missing API key): report honestly
				// rather than fabricating data.
				envelope["scanned_agents"] = 0
				envelope["note"] = "live fan-out unavailable (no API client configured); set TRMM_API_KEY and re-run"
				return flags.printJSON(cmd, envelope)
			}

			scanned := 0
			fetchFailures := 0
			var firstErr string
			now := time.Now()
			for _, r := range refs {
				id := strings.TrimSpace(r.id)
				if id == "" {
					scanned++
					fetchFailures++
					if firstErr == "" {
						firstErr = "empty agent id skipped"
					}
					continue
				}
				data, gerr := c.Get(cmd.Context(), "/agents/"+url.PathEscape(id)+"/pendingactions/", nil)
				scanned++
				if gerr != nil {
					fetchFailures++
					if firstErr == "" {
						firstErr = gerr.Error()
					}
					continue
				}
				var raws []json.RawMessage
				matched := json.Unmarshal(data, &raws) == nil
				if !matched {
					// Some deployments wrap in {"pending_actions":[...]}.
					var wrap struct {
						PA []json.RawMessage `json:"pending_actions"`
					}
					if json.Unmarshal(data, &wrap) == nil {
						raws = wrap.PA
						matched = true
					}
				}
				if !matched {
					// A non-empty body that matched neither accepted shape is a
					// failure, not an empty result, so the gap stays visible.
					if len(strings.TrimSpace(string(data))) > 0 {
						fetchFailures++
						if firstErr == "" {
							firstErr = "unrecognized pending-actions response shape"
						}
					}
					continue
				}
				for _, raw := range raws {
					var pa map[string]any
					if json.Unmarshal(raw, &pa) != nil {
						continue
					}
					status := asString(pa["status"])
					if status != "" && !strings.EqualFold(status, "pending") {
						continue
					}
					it := item{AgentID: r.id, Hostname: r.host, Action: asString(pa["action_type"]), Status: status}
					if it.Action == "" {
						it.Action = asString(pa["details"])
					}
					for _, k := range []string{"due_date", "created_time", "due"} {
						if ts := asString(pa[k]); ts != "" {
							it.Due = ts
							if t, perr := time.Parse(time.RFC3339, strings.Replace(ts, " ", "T", 1)); perr == nil {
								it.AgeHours = int(now.Sub(t).Hours())
							}
							break
						}
					}
					items = append(items, it)
				}
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].AgentID == items[j].AgentID {
					return items[i].AgeHours > items[j].AgeHours
				}
				return items[i].AgentID < items[j].AgentID
			})
			envelope["items"] = items
			envelope["scanned_agents"] = scanned
			envelope["total_pending"] = len(items)
			envelope["fetch_failures"] = fetchFailures
			if firstErr != "" {
				envelope["first_error"] = firstErr
			}
			if err := flags.printJSON(cmd, envelope); err != nil {
				return err
			}
			if scanned > 0 && fetchFailures == scanned {
				return fmt.Errorf("all %d pending-actions fetches failed: %s", scanned, firstErr)
			}
			if fetchFailures > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d fetches failed; totals computed over the remaining %d agents\n", fetchFailures, scanned, scanned-fetchFailures)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&maxScan, "max-scan-agents", 50, "Max agents to fan out to live")
	return cmd
}

// asString coerces a decoded JSON value to a string for display, returning ""
// for nil so empty fields are omitted from JSON rather than printed as <nil>.
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return strings.Trim(string(b), `"`)
	}
}
