// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelEnvironmentsOverviewCmd implements `environments overview <id>` — one
// client's complete picture assembled from the local store: its systems (each
// with its launchpoint, inspector, and latest inspection), the agents serving
// it, and recent detections. Liongard entities carry nested Environment/
// Launchpoint/System/Inspector objects, so the lineage assembles from refs.
// pp:data-source local
func newNovelEnvironmentsOverviewCmd(flags *rootFlags) *cobra.Command {
	var flagFull bool

	cmd := &cobra.Command{
		Use:   "overview [environment-id]",
		Short: "One client's complete picture: its systems, launchpoints, agents, latest inspections, and recent detections",
		Long: `Assembles a single environment's full lineage from the locally synced store:
systems (each with its launchpoint, inspector, and most-recent inspection), the
agents serving it, and recent detections. Pass --full to include per-system
detail and the recent-detections list; omit it for a counts-only summary. Run
'liongard-cli sync' first.`,
		Example: strings.Trim(`
  # Summary for environment 42
  liongard-cli environments overview 42

  # Full lineage as agent JSON
  liongard-cli environments overview 42 --full --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			envID := strings.TrimSpace(args[0])
			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			envs, err := loadObjs(db, rtEnvironments)
			if err != nil {
				return err
			}
			var env map[string]any
			for _, e := range envs {
				if lgID(e) == envID || strings.EqualFold(lgStr(e, "Name", "name"), envID) {
					env = e
					break
				}
			}
			if env == nil {
				return notFoundErr(fmt.Errorf("environment %q not found in local store; run 'liongard-cli sync' or check the ID", envID))
			}
			resolvedID := lgID(env)
			envName := lgStr(env, "Name", "name")

			systems, _ := loadObjs(db, rtSystems)
			launchpoints, _ := loadObjs(db, rtLaunchpoints)
			agents, _ := loadObjs(db, rtAgents)
			timeline, _ := loadObjs(db, rtTimeline)
			detections, _ := loadObjs(db, rtDetections)

			// Newest inspection per launchpoint (timeline carries a Launchpoint ref).
			latestByLP := map[string]time.Time{}
			for _, t := range timeline {
				// Restrict to this environment's timeline when the entry names one.
				if e := lgRefID(t, "Environment", "EnvironmentID"); e != "" && e != resolvedID {
					continue
				}
				lpID := lgRefID(t, "Launchpoint", "LaunchpointID")
				if lpID == "" {
					continue
				}
				if ts, ok := lgTime(t, tsKeysInspection...); ok {
					if cur, seen := latestByLP[lpID]; !seen || ts.After(cur) {
						latestByLP[lpID] = ts
					}
				}
			}

			type systemRow struct {
				SystemID       string `json:"system_id"`
				System         string `json:"system,omitempty"`
				Inspector      string `json:"inspector,omitempty"`
				LaunchpointID  string `json:"launchpoint_id,omitempty"`
				Launchpoint    string `json:"launchpoint,omitempty"`
				Status         string `json:"status,omitempty"`
				LastInspection string `json:"last_inspection,omitempty"`
			}
			systemRows := []systemRow{}
			for _, s := range systems {
				if lgRefID(s, "Environment", "EnvironmentID") != resolvedID {
					continue
				}
				lpID := lgRefID(s, "Launchpoint", "LaunchpointID")
				row := systemRow{
					SystemID:      lgID(s),
					System:        lgStr(s, "Name", "name", "Hostname", "FQDN"),
					Inspector:     lgRefName(s, "Inspector"),
					LaunchpointID: lpID,
					Launchpoint:   lgRefName(s, "Launchpoint"),
					Status:        lgStr(s, "Status", "State"),
				}
				if ts, ok := latestByLP[lpID]; ok {
					row.LastInspection = ts.Format(time.RFC3339)
				}
				systemRows = append(systemRows, row)
			}
			sort.SliceStable(systemRows, func(i, j int) bool { return systemRows[i].System < systemRows[j].System })

			lpCount := 0
			for _, lp := range launchpoints {
				if lgRefID(lp, "Environment", "EnvironmentID") == resolvedID {
					lpCount++
				}
			}

			type agentRow struct {
				AgentID string `json:"agent_id"`
				Name    string `json:"name,omitempty"`
				Status  string `json:"status,omitempty"`
				Offline bool   `json:"offline"`
			}
			agentRows := []agentRow{}
			offlineAgents := 0
			for _, a := range agents {
				if lgRefID(a, "Environment", "EnvironmentID") != resolvedID {
					continue
				}
				off := agentIsOffline(a)
				if off {
					offlineAgents++
				}
				agentRows = append(agentRows, agentRow{
					AgentID: lgID(a),
					Name:    lgStr(a, "Name", "name", "FriendlyName"),
					Status:  lgStr(a, "Status", "State", "ConnectionStatus"),
					Offline: off,
				})
			}

			type detRow struct {
				DetectionID string `json:"detection_id"`
				Name        string `json:"name,omitempty"`
				CreatedOn   string `json:"created_on,omitempty"`
				System      string `json:"system,omitempty"`
			}
			detRows := []detRow{}
			for _, d := range detections {
				if lgRefID(d, "Environment", "EnvironmentID") != resolvedID {
					continue
				}
				cre := ""
				if ts, ok := lgTime(d, tsKeysDetection...); ok {
					cre = ts.Format(time.RFC3339)
				}
				detRows = append(detRows, detRow{
					DetectionID: lgID(d),
					Name:        lgStr(d, "Name", "name", "Detection", "Description", "Title"),
					CreatedOn:   cre,
					System:      lgRefName(d, "System"),
				})
			}
			sort.SliceStable(detRows, func(i, j int) bool { return detRows[i].CreatedOn > detRows[j].CreatedOn })
			totalDetections := len(detRows)
			const recentDetCap = 25
			if len(detRows) > recentDetCap {
				detRows = detRows[:recentDetCap]
			}

			result := map[string]any{
				"environment": map[string]any{
					"id":   resolvedID,
					"name": envName,
				},
				"summary": map[string]any{
					"system_count":      len(systemRows),
					"launchpoint_count": lpCount,
					"agent_count":       len(agentRows),
					"offline_agents":    offlineAgents,
					"recent_detections": totalDetections,
				},
			}
			if flagFull {
				result["systems"] = systemRows
				result["agents"] = agentRows
				result["recent_detection_list"] = detRows
			} else {
				names := make([]string, 0, len(systemRows))
				for _, s := range systemRows {
					names = append(names, s.System)
				}
				result["systems"] = names
				result["agents"] = agentRows
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().BoolVar(&flagFull, "full", false, "Include per-system detail and the recent-detections list")
	return cmd
}
