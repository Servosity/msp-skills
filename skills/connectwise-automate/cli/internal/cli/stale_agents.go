// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/fleet"
)

func newNovelStaleAgentsCmd(flags *rootFlags) *cobra.Command {
	var days int

	cmd := &cobra.Command{
		Use:   "stale-agents",
		Short: "List computers not seen in N days, grouped by client — the offline agents bleeding license and hiding risk",
		Long: strings.Trim(`
List every computer whose last contact is older than --days (and agents that
never reported a contact at all), most-stale first, with its owning client.
The offline agents bleeding license and hiding risk.

Reads the local SQLite mirror — run 'sync' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-automate-cli stale-agents --days 30 --agent
  connectwise-automate-cli stale-agents --days 45 --select computer,client,days_stale`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			s, err := openTranscendStore(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			computers, err := loadResource(s, "computers")
			if err != nil {
				return err
			}
			clients, err := loadResource(s, "clients")
			if err != nil {
				return err
			}
			result := fleet.StaleAgents(computers, clients, days, time.Now())
			return emitResult(cmd, flags, result)
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "A computer is stale when its last contact is older than this many days")
	return cmd
}
