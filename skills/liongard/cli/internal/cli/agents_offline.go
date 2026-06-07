// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// agentOfflineWords / agentOnlineWords classify a Liongard agent's Status string
// when no explicit Online boolean is present. Matching is case-insensitive
// substring; an unrecognized non-empty status is treated as online to avoid
// false positives (an agent is only reported offline on positive evidence).
var agentOfflineWords = []string{"offline", "disconnected", "down", "dead", "error", "inactive", "unreachable", "stopped", "expired"}
var agentOnlineWords = []string{"online", "active", "connected", "healthy", "running", "ok", "up", "enabled", "ready"}

func agentIsOffline(a map[string]any) bool {
	// Prefer an explicit online/connected boolean.
	switch v := lgGet(a, "Online", "IsOnline", "Connected", "IsConnected").(type) {
	case bool:
		return !v
	}
	status := strings.ToLower(lgStr(a, "Status", "State", "ConnectionStatus"))
	if status == "" {
		return false
	}
	for _, w := range agentOfflineWords {
		if strings.Contains(status, w) {
			return true
		}
	}
	return false
}

// newNovelAgentsOfflineCmd implements `agents offline` — every offline agent
// across the estate, joined to the environment it serves.
// pp:data-source local
func newNovelAgentsOfflineCmd(flags *rootFlags) *cobra.Command {
	var flagEnv string

	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Every offline agent across the estate, joined to the environment it serves.",
		Long: `Reports agents whose synced status indicates they are offline, joined to the
owning environment. Reads the locally synced store; run 'liongard-cli sync'
first.`,
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

			rows := filterOfflineAgents(agents, envs, flagEnv)

			result := map[string]any{
				"count":  len(rows),
				"agents": rows,
			}
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagEnv, "env", "", "Restrict to a single environment ID")
	return cmd
}
