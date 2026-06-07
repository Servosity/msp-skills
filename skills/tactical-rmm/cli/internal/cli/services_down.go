// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live
package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// Data-source strategy: Tactical RMM `services` are NOT one of the
// default-synced resources (see defaultSyncResources()), so the local store
// has no per-agent service inventory to read. This command does a read-only
// live fan-out: it takes agent IDs from the local `agents` resource and calls
// GET /services/{agent_id}/ for each agent (capped by --max-scan-agents),
// keeping rows where the named service is present and not running. The JSON
// envelope reports scanned_agents. Empty store / no API key returns an honest
// empty result.

func newNovelServicesDownCmd(flags *rootFlags) *cobra.Command {
	var name string
	var maxScan int
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Agents where a named Windows service is stopped",
		Long: `Use this command to find agents where a named Windows service is stopped. Do
NOT use it to find installed software packages; use 'software find' instead.

Services are not stored locally, so this fans out live to GET
/services/{agent_id}/ for each synced agent, capped by --max-scan-agents.
Read-only.`,
		Example:     "  tactical-rmm-cli services down --name Spooler\n  tactical-rmm-cli services down --name wuauserv --max-scan-agents 100 --json",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			svc := strings.TrimSpace(name)
			if svc == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--name <service> is required"))
			}
			type item struct {
				AgentID     string `json:"agent_id"`
				Hostname    string `json:"hostname,omitempty"`
				Service     string `json:"service"`
				DisplayName string `json:"display_name,omitempty"`
				Status      string `json:"status"`
			}
			items := make([]item, 0)
			envelope := map[string]interface{}{"items": items, "scanned_agents": 0, "service": svc, "total": 0, "fetch_failures": 0, "status_unknown": 0}

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
				envelope["note"] = "live fan-out unavailable (no API client configured); set TRMM_API_KEY and re-run"
				return flags.printJSON(cmd, envelope)
			}

			scanned := 0
			fetchFailures := 0
			statusUnknown := 0
			var firstErr string
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
				data, gerr := c.Get(cmd.Context(), "/services/"+url.PathEscape(id)+"/", nil)
				scanned++
				if gerr != nil {
					fetchFailures++
					if firstErr == "" {
						firstErr = gerr.Error()
					}
					continue
				}
				var svcs []map[string]any
				if json.Unmarshal(data, &svcs) != nil {
					// A non-empty body that doesn't decode as the service array
					// is a failure, not an empty result.
					if len(strings.TrimSpace(string(data))) > 0 {
						fetchFailures++
						if firstErr == "" {
							firstErr = "unrecognized services response shape"
						}
					}
					continue
				}
				for _, sv := range svcs {
					nm := asString(sv["name"])
					disp := asString(sv["display_name"])
					if !strings.EqualFold(nm, svc) && !strings.EqualFold(disp, svc) {
						continue
					}
					status := asString(sv["status"])
					// A service with no status is unknown, NOT down: do not fall
					// back to pid (which is unrelated) and do not report it.
					if status == "" {
						statusUnknown++
						continue
					}
					if strings.EqualFold(status, "running") {
						continue
					}
					items = append(items, item{AgentID: r.id, Hostname: r.host, Service: nm, DisplayName: disp, Status: status})
				}
			}
			envelope["items"] = items
			envelope["scanned_agents"] = scanned
			envelope["total"] = len(items)
			envelope["fetch_failures"] = fetchFailures
			envelope["status_unknown"] = statusUnknown
			if firstErr != "" {
				envelope["first_error"] = firstErr
			}
			if err := flags.printJSON(cmd, envelope); err != nil {
				return err
			}
			if scanned > 0 && fetchFailures == scanned {
				return fmt.Errorf("all %d services fetches failed: %s", scanned, firstErr)
			}
			if fetchFailures > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d fetches failed; totals computed over the remaining %d agents\n", fetchFailures, scanned, scanned-fetchFailures)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Windows service name (required), e.g. Spooler")
	cmd.Flags().IntVar(&maxScan, "max-scan-agents", 50, "Max agents to fan out to live")
	return cmd
}
