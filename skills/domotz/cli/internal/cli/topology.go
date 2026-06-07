// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Network topology summary for a site: edge/node
// counts and the highest-degree devices (the gateways/switches everything hangs
// off). Fetches live and caches to the local store so it can be re-queried
// offline with --cached.

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
)

type topologyEdge struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

type topologyDoc struct {
	Edges []topologyEdge `json:"edges"`
}

type topologyNode struct {
	DeviceID    int64  `json:"device_id"`
	Degree      int    `json:"degree"`
	DisplayName string `json:"display_name,omitempty"`
}

type topologySummary struct {
	AgentID  string         `json:"agent_id"`
	Source   string         `json:"source"` // "live" or "cached"
	Edges    int            `json:"edges"`
	Nodes    int            `json:"nodes"`
	TopNodes []topologyNode `json:"top_nodes"`
}

// pp:data-source auto
func newNovelTopologyCmd(flags *rootFlags) *cobra.Command {
	var flagAgent string
	var cached bool
	var topN int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "topology [agent-id]",
		Short: "Network topology summary for a site (edge/node counts, top devices by degree)",
		Long: "Summarize a site's network topology — edge and node counts plus the highest-degree " +
			"devices (gateways/switches). Fetches live and caches to the local store; pass --cached " +
			"to re-read the last fetch offline. Identify the site with a positional agent id or --agent-id.",
		Example:     "  domotz-cli topology --agent-id 12345 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := flagAgent
			if agentID == "" && len(args) > 0 {
				agentID = args[0]
			}
			if agentID == "" {
				if dryRunOK(flags) {
					return nil
				}
				return usageErr(fmt.Errorf("an agent id is required (positional <agent-id> or --agent-id)"))
			}
			if dryRunOK(flags) {
				return nil
			}

			switch flags.dataSource {
			case "local":
				cached = true
			case "live":
				cached = false
			}
			var doc topologyDoc
			source := "live"
			if cached {
				source = "cached"
				db, err := openFleetStore(cmd.Context(), dbPath)
				if err != nil {
					return err
				}
				defer db.Close()
				var raw string
				err = db.DB().QueryRowContext(cmd.Context(),
					`SELECT data FROM "network_topology" WHERE agent_id = ? ORDER BY synced_at DESC LIMIT 1`, agentID).Scan(&raw)
				if err != nil {
					return fmt.Errorf("no cached topology for agent %s (run without --cached to fetch): %w", agentID, err)
				}
				if err := json.Unmarshal([]byte(raw), &doc); err != nil {
					return fmt.Errorf("parsing cached topology: %w", err)
				}
			} else {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				data, err := c.Get(cmd.Context(), "/agent/"+url.PathEscape(agentID)+"/network-topology", map[string]string{})
				if err != nil {
					return classifyAPIError(err, flags)
				}
				if err := json.Unmarshal(data, &doc); err != nil {
					return fmt.Errorf("parsing topology: %w", err)
				}
				cacheTopology(cmd, dbPath, agentID, data)
			}

			summary := summarizeTopology(agentID, source, doc, topN)
			enrichTopologyNames(cmd, dbPath, agentID, summary.TopNodes)
			return printJSONFiltered(cmd.OutOrStdout(), summary, flags)
		},
	}
	cmd.Flags().StringVar(&flagAgent, "agent-id", "", "Agent (Collector) id whose topology to summarize")
	cmd.Flags().BoolVar(&cached, "cached", false, "Read the last cached topology from the local store instead of fetching live")
	cmd.Flags().IntVar(&topN, "top", 10, "Number of highest-degree devices to list")
	addFleetDBFlag(cmd, &dbPath)
	return cmd
}

// summarizeTopology computes edge/node counts and the highest-degree nodes.
func summarizeTopology(agentID, source string, doc topologyDoc, topN int) topologySummary {
	degree := make(map[int64]int)
	for _, e := range doc.Edges {
		degree[e.From]++
		degree[e.To]++
	}
	nodes := make([]topologyNode, 0, len(degree))
	for id, d := range degree {
		nodes = append(nodes, topologyNode{DeviceID: id, Degree: d})
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Degree != nodes[j].Degree {
			return nodes[i].Degree > nodes[j].Degree
		}
		return nodes[i].DeviceID < nodes[j].DeviceID
	})
	if topN > 0 && len(nodes) > topN {
		nodes = nodes[:topN]
	}
	return topologySummary{
		AgentID:  agentID,
		Source:   source,
		Edges:    len(doc.Edges),
		Nodes:    len(degree),
		TopNodes: nodes,
	}
}

// cacheTopology stores the raw topology JSON keyed by agent so --cached can
// re-read it offline. Best-effort: cache failures never fail the command.
func cacheTopology(cmd *cobra.Command, dbPath, agentID string, data json.RawMessage) {
	db, err := openFleetStore(cmd.Context(), dbPath)
	if err != nil {
		return
	}
	defer db.Close()
	_, _ = db.DB().ExecContext(cmd.Context(),
		`INSERT OR REPLACE INTO "network_topology" (id, agent_id, data, synced_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		agentID, agentID, string(data))
}

// enrichTopologyNames fills display names for the top nodes from the device
// table when it has been synced. Best-effort.
func enrichTopologyNames(cmd *cobra.Command, dbPath, agentID string, nodes []topologyNode) {
	if len(nodes) == 0 {
		return
	}
	db, err := openFleetStore(cmd.Context(), dbPath)
	if err != nil {
		return
	}
	defer db.Close()
	names := make(map[string]string)
	rows, err := queryFleetRows(cmd.Context(), db,
		`SELECT id, json_extract(data, '$.display_name') AS display_name FROM "device" WHERE agent_id = ?`, agentID)
	if err != nil {
		return
	}
	for _, r := range rows {
		names[asString(r["id"])] = asString(r["display_name"])
	}
	for i := range nodes {
		if n, ok := names[strconv.FormatInt(nodes[i].DeviceID, 10)]; ok {
			nodes[i].DisplayName = n
		}
	}
}
