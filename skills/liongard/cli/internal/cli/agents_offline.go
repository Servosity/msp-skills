// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// agentOfflineWords / agentOnlineWords classify a status string. Matching is
// case-insensitive substring. An agent is only reported offline on positive
// evidence, and any one offline signal wins.
var agentOfflineWords = []string{"offline", "disconnected", "down", "dead", "error", "inactive", "unreachable", "stopped", "expired"}
var agentOnlineWords = []string{"online", "active", "connected", "healthy", "running", "ok", "up", "enabled", "ready"}

// defaultAgentOfflineAfter is how long an agent may go without a heartbeat
// before it counts as offline (--offline-after).
const defaultAgentOfflineAfter = time.Hour

// agentOfflineReason is the ONE agent classifier (agents offline, health and
// the environments overview), "" when no signal says offline. Codex r1 class
// documented-agent-shape-never-classified: Liongard documents an agent's
// Status as an integer whose values no document maps, and LastHeartbeat as a
// timestamp (components/schemas/Agents), while the classifier read only
// online booleans and status words, so a documented agent was never offline.
// Signals, any one of which wins: the documented LastHeartbeat older than
// after; an online/connected boolean that is false; a status word that reads
// offline. A heartbeat that is absent or does not parse is no evidence.
func agentOfflineReason(a map[string]any, now time.Time, after time.Duration) string {
	var reasons []string
	if hb, ok := lgTime(a, "LastHeartbeat"); ok && now.Sub(hb) > after {
		reasons = append(reasons, "no heartbeat since "+hb.Format(time.RFC3339))
	}
	if v, ok := lgGet(a, "Online", "IsOnline", "Connected", "IsConnected").(bool); ok && !v {
		reasons = append(reasons, "reported not connected")
	}
	if status := strings.ToLower(lgStr(a, "Status", "State", "ConnectionStatus")); status != "" {
		for _, w := range agentOfflineWords {
			if strings.Contains(status, w) {
				reasons = append(reasons, "status "+status)
				break
			}
		}
	}
	return strings.Join(reasons, "; ")
}

// agentIsOffline is agentOfflineReason now, with the default threshold.
func agentIsOffline(a map[string]any) bool {
	return agentOfflineReason(a, time.Now().UTC(), defaultAgentOfflineAfter) != ""
}

// parseOfflineAfter reads --offline-after.
func parseOfflineAfter(s string) (time.Duration, error) {
	d, err := parseLookbackDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid --offline-after %q; use a positive duration like 30m, 1h or 1d", s)
	}
	return d, nil
}

// newNovelAgentsOfflineCmd implements `agents offline` — every offline agent
// across the estate, joined to the environment it serves.
// pp:data-source local
func newNovelAgentsOfflineCmd(flags *rootFlags) *cobra.Command {
	var flagEnv string
	var flagOfflineAfter string

	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Every offline agent across the estate, joined to the environment it serves.",
		Long: `Reports agents that are offline, joined to the owning environment: no
heartbeat (LastHeartbeat) for longer than --offline-after, a connected flag that
is false, or a status that reads offline. Reads the locally synced store; run
'liongard-cli sync' first.`,
		Example: strings.Trim(`
  # All offline agents across every client
  liongard-cli agents offline --agent

  # Offline agents in one environment
  liongard-cli agents offline --env 42
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			after, err := parseOfflineAfter(flagOfflineAfter)
			if err != nil {
				return err
			}
			db, err := openNovelStore(cmd, flags)
			if err != nil {
				return err
			}
			defer db.Close()

			agents, err := loadObjs(db, rtAgents)
			if err != nil {
				return err
			}
			envs, err := loadObjs(db, rtEnvironments)
			if err != nil {
				return err
			}

			rows := filterOfflineAgentsAt(agents, envs, flagEnv, time.Now().UTC(), after)

			result := map[string]any{
				"count":  len(rows),
				"agents": rows,
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagEnv, "env", "", "Restrict to a single environment ID")
	cmd.Flags().StringVar(&flagOfflineAfter, "offline-after", "1h", "An agent with no heartbeat for longer than this is offline (e.g. 30m, 1h, 1d)")
	return cmd
}
