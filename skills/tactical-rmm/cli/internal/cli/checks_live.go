// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix check-status-from-agent-checks.

package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"tactical-rmm-pp-cli/internal/client"
)

// Tactical RMM keeps a check's status on the per-agent CheckResult, not on the
// Check row. The synced "checks" resource is GET /checks/ (CheckSerializer over
// Check.objects.filter_by_role), which carries no status: its check_result is
// {} because no agent's result is attached there. Only GET
// /agents/{agent_id}/checks/ (GetAddChecks.get -> agent.get_checks_with_policies(),
// which attaches the agent's CheckResult) carries it, nested as
// check_result.status ("passing", "failing", "pending"; {} when the check has
// not run). See vendor-src/checks/views.py GetAddChecks.get and
// vendor-src/agents/urls.py "<agent:agent_id>/checks/". That GET reads the
// database and the server cache; it dispatches nothing to the endpoint, so it is
// a plain read (client GetNoCache, which the action-route guard still checks).

// checkStatusUnknown marks a check whose result carries no status. It is never
// counted as failing or passing.
const checkStatusUnknown = "unknown"

// trmmAgentRef is one synced agent to fan out to.
type trmmAgentRef struct{ id, host string }

// trmmAgentCheck is one check of one agent, with that agent's status.
type trmmAgentCheck struct {
	AgentID   string `json:"agent_id"`
	Hostname  string `json:"hostname,omitempty"`
	CheckID   string `json:"check_id"`
	Name      string `json:"name"`
	CheckType string `json:"check_type"`
	Status    string `json:"status"`
}

// trmmCheckScan is the result of a live fan-out over agents' checks.
type trmmCheckScan struct {
	Rows          []trmmAgentCheck
	Scanned       int
	FetchFailures int
	StatusUnknown int
	FirstErr      string
}

// loadTrmmAgentRefs reads agent ids from the synced agents resource. With
// onlyFailing it keeps agents whose synced row reports a failing check of any
// alert severity (checks.failing, checks.warning or checks.info > 0, or
// has_failing_checks), most failing first.
func loadTrmmAgentRefs(cmd *cobra.Command, flags *rootFlags, onlyFailing bool) []trmmAgentRef {
	refs := make([]trmmAgentRef, 0)
	s := novelLocalRead(cmd, flags, "agents")
	if s == nil {
		return refs
	}
	defer s.Close()
	q := "SELECT json_extract(data,'$.agent_id'),json_extract(data,'$.hostname') FROM resources WHERE resource_type='agents'"
	if onlyFailing {
		// The vendor's agent checks summary files a failing check under failing,
		// warning or info by its alert severity (ping, service and event-log
		// checks default to warning), so a candidate is any agent with one.
		const red = "(COALESCE(json_extract(data,'$.checks.failing'),0)+COALESCE(json_extract(data,'$.checks.warning'),0)+COALESCE(json_extract(data,'$.checks.info'),0))"
		q += " AND (" + red + ">0 OR json_extract(data,'$.checks.has_failing_checks')=1) ORDER BY " + red + " DESC, json_extract(data,'$.agent_id')"
	} else {
		q += " ORDER BY json_extract(data,'$.agent_id')"
	}
	rows, err := s.DB().QueryContext(cmd.Context(), q)
	if err != nil {
		return refs
	}
	defer rows.Close()
	for rows.Next() {
		var id, h sql.NullString
		if rows.Scan(&id, &h) == nil && strings.TrimSpace(id.String) != "" {
			refs = append(refs, trmmAgentRef{id.String, h.String})
		}
	}
	return refs
}

// fetchAgentChecksLive calls GET /agents/{agent_id}/checks/ for each agent and
// returns one row per check with the agent's status. A request error, or a
// body that is not a JSON array of check objects, is a fetch failure, never an
// empty result.
func fetchAgentChecksLive(ctx context.Context, c *client.Client, refs []trmmAgentRef) trmmCheckScan {
	var scan trmmCheckScan
	fail := func(msg string) {
		scan.FetchFailures++
		if scan.FirstErr == "" {
			scan.FirstErr = msg
		}
	}
	for _, r := range refs {
		scan.Scanned++
		id := strings.TrimSpace(r.id)
		if id == "" {
			fail("empty agent id skipped")
			continue
		}
		data, err := c.GetNoCache(ctx, "/agents/"+url.PathEscape(id)+"/checks/", nil)
		if err != nil {
			fail(err.Error())
			continue
		}
		var checks []map[string]any
		body := bytes.TrimSpace(data)
		if len(body) == 0 || body[0] != '[' || json.Unmarshal(body, &checks) != nil {
			fail(fmt.Sprintf("GET /agents/%s/checks/ did not answer a check array", id))
			continue
		}
		for _, ck := range checks {
			if ck == nil {
				continue
			}
			ctype := asString(ck["check_type"])
			name := strings.TrimSpace(asString(ck["readable_desc"]))
			if name == "" {
				name = strings.TrimSpace(asString(ck["name"]))
			}
			if name == "" {
				name = ctype
			}
			if name == "" {
				name = "(unnamed)"
			}
			status := ""
			if res, ok := ck["check_result"].(map[string]any); ok {
				status = strings.ToLower(strings.TrimSpace(asString(res["status"])))
			}
			if status == "" {
				status = checkStatusUnknown
				scan.StatusUnknown++
			}
			scan.Rows = append(scan.Rows, trmmAgentCheck{
				AgentID:   r.id,
				Hostname:  r.host,
				CheckID:   asString(ck["id"]),
				Name:      name,
				CheckType: ctype,
				Status:    status,
			})
		}
	}
	return scan
}

// reportCheckScan copies the scan counters into a JSON envelope.
func reportCheckScan(envelope map[string]any, scan trmmCheckScan) {
	envelope["scanned_agents"] = scan.Scanned
	envelope["fetch_failures"] = scan.FetchFailures
	envelope["status_unknown"] = scan.StatusUnknown
	if scan.FirstErr != "" {
		envelope["first_error"] = scan.FirstErr
	}
}

// checkScanOutcome is the exit contract shared by the check reports: every
// fetch failed is an error; some failed is a stderr warning.
func checkScanOutcome(cmd *cobra.Command, scan trmmCheckScan) error {
	if scan.Scanned > 0 && scan.FetchFailures == scan.Scanned {
		return fmt.Errorf("all %d agent check fetches failed (GET /agents/{agent_id}/checks/): %s", scan.Scanned, scan.FirstErr)
	}
	if scan.FetchFailures > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d agent check fetches failed; results cover the remaining %d agents\n", scan.FetchFailures, scan.Scanned, scan.Scanned-scan.FetchFailures)
	}
	return nil
}
