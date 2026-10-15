// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: assets whose scan data cannot be trusted.
// pp:data-source local

package cli

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type blindSpotRow struct {
	CompanyID          string   `json:"company_id"`
	CompanyName        string   `json:"company_name"`
	AssetID            string   `json:"asset_id"`
	AssetName          string   `json:"asset_name"`
	IP                 string   `json:"ip"`
	Reasons            []string `json:"reasons"`
	LastDiscoveredTime string   `json:"last_discovered_time"`
	AuthScanStatus     string   `json:"auth_scan_status"`
	AgentID            string   `json:"agent_id"`
	AgentLastPing      string   `json:"agent_last_ping"`
}

type blindSpotOpts struct {
	stale      time.Duration
	agentStale time.Duration
	authOK     map[string]bool
	now        time.Time
}

// auth_scan_status is a boolean in the vendor spec (true = authenticated scan).
// ponytail: string values are also accepted; with no --auth-ok list they go
// through this keyword heuristic, so pass --auth-ok for an exact check.
var blindSpotAuthBad = regexp.MustCompile(`(?i)unauth|fail|not|error|denied|invalid|partial`)

func buildBlindSpots(data map[string][]csRow, o blindSpotOpts) []blindSpotRow {
	names := csCompanyNames(data[csResCompanies])
	agents := csIndex(data[csResAgents], "id")
	out := make([]blindSpotRow, 0)
	for _, a := range data[csResAssets] {
		if csBool(a["is_deprecated"]) {
			continue
		}
		row := blindSpotRow{
			CompanyID: csString(a["company_id"]), CompanyName: csString(a["company_name"]),
			AssetID: csString(a["id"]), AssetName: csAssetName(a), IP: csString(a["ip"]),
			LastDiscoveredTime: csString(a["last_discovered_time"]), AuthScanStatus: csString(a["auth_scan_status"]),
			AgentID: csString(a["agent_id"]), Reasons: make([]string, 0, 3),
		}
		if row.CompanyName == "" {
			row.CompanyName = names[row.CompanyID]
		}
		if o.stale > 0 {
			seen, ok := csTime(a["last_discovered_time"])
			switch {
			case !ok:
				row.Reasons = append(row.Reasons, "never_discovered")
			case seen.Before(o.now.Add(-o.stale)):
				row.Reasons = append(row.Reasons, "stale_discovery")
			}
		}
		if blindSpotUnauthenticated(a["auth_scan_status"], o.authOK) {
			row.Reasons = append(row.Reasons, "unauthenticated_scan")
		}
		if ag, ok := agents[row.AgentID]; ok && row.AgentID != "0" && o.agentStale > 0 {
			row.AgentLastPing = csString(ag["last_ping_time"])
			ping, ok := csTime(ag["last_ping_time"])
			if !ok || ping.Before(o.now.Add(-o.agentStale)) {
				row.Reasons = append(row.Reasons, "silent_agent")
			}
		}
		if len(row.Reasons) > 0 {
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CompanyName != out[j].CompanyName {
			return out[i].CompanyName < out[j].CompanyName
		}
		if len(out[i].Reasons) != len(out[j].Reasons) {
			return len(out[i].Reasons) > len(out[j].Reasons)
		}
		return out[i].AssetName < out[j].AssetName
	})
	return out
}

// blindSpotUnauthenticated: false for a boolean true; for strings, outside
// --auth-ok when given, else the keyword heuristic. Missing values never flag.
func blindSpotUnauthenticated(v any, authOK map[string]bool) bool {
	switch t := v.(type) {
	case bool:
		return !t
	case nil:
		return false
	}
	st := csString(v)
	if st == "" {
		return false
	}
	if len(authOK) > 0 {
		return !authOK[strings.ToLower(st)]
	}
	return blindSpotAuthBad.MatchString(st)
}

func newNovelBlindSpotsCmd(flags *rootFlags) *cobra.Command {
	var staleFlag, agentStaleFlag, authOK, company, dbPath string
	cmd := &cobra.Command{
		Use:   "blind-spots",
		Short: "Assets whose scan data cannot be trusted: stale discovery, unauthenticated scan, silent agent",
		Long: `List non-deprecated assets whose vulnerability data cannot be trusted, grouped by
client, with reason codes:
  stale_discovery / never_discovered  last_discovered_time older than --stale
  unauthenticated_scan                auth_scan_status false (or a string outside --auth-ok)
  silent_agent                        the asset's agent last pinged before --agent-stale
Reads the local store (run 'connectsecure-cli sync' first).

Use this command to list assets whose scan data cannot be trusted.
Do NOT use this command for vulnerability findings; use 'exposure' instead.
Do NOT use it for per-client totals; use 'portfolio' instead.`,
		Example: strings.Trim(`
  connectsecure-cli blind-spots --stale 14d --agent-stale 48h
  connectsecure-cli blind-spots --company "Acme Dental" --agent
  connectsecure-cli blind-spots --select asset_id --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "blind-spots")
			}
			stale, err := csDuration("stale", staleFlag)
			if err != nil {
				return err
			}
			agentStale, err := csDuration("agent-stale", agentStaleFlag)
			if err != nil {
				return err
			}
			data, unsynced, err := csLocalData(cmd, flags, dbPath, csResAssets, csResAgents, csResCompanies)
			if err != nil {
				return err
			}
			rows := buildBlindSpots(data, blindSpotOpts{stale: stale, agentStale: agentStale, authOK: csCSVSet(authOK), now: time.Now().UTC()})
			rows = slices.DeleteFunc(rows, func(r blindSpotRow) bool { return !csCompanyMatch(company, r.CompanyID, r.CompanyName) })
			return csEmit(cmd, flags, rows, unsynced, "No blind spots among the synced assets.")
		},
	}
	cmd.Flags().StringVar(&staleFlag, "stale", "14d", "Flag assets not discovered within this window (0 disables)")
	cmd.Flags().StringVar(&agentStaleFlag, "agent-stale", "48h", "Flag assets whose agent has not pinged within this window (0 disables)")
	cmd.Flags().StringVar(&authOK, "auth-ok", "", "For string auth_scan_status values: comma-separated values that count as authenticated (default: keyword heuristic; booleans need no list)")
	cmd.Flags().StringVar(&company, "company", "", "Only this client (name substring or company ID)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the synced store)")
	return cmd
}
