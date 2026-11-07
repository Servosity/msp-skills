// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/fleet"
)

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var staleDays int

	cmd := &cobra.Command{
		Use:   "fleet-health",
		Short: "See every agent across every client in one roll-up: online/offline, last-contact age, and open-alert count",
		Long: strings.Trim(`
Roll every computer in the local mirror up per client: total agents, online,
offline, stale (no contact in --stale-days), and open-alert count. One board
the per-server Automate UI scatters across separate screens.

Reads the local SQLite mirror — run 'sync' first. Output is JSON; pair with
--agent for agent-friendly defaults.`, "\n"),
		Example: strings.Trim(`
  connectwise-automate-cli fleet-health --agent
  connectwise-automate-cli fleet-health --stale-days 14 --select client,offline,open_alerts`, "\n"),
		Annotations: map[string]string{"pp:data-source": "local", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			s, err := openTranscendStore(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			computers, err := loadResource(cmd, flags, s, "computers")
			if err != nil {
				return err
			}
			clients, err := loadResource(cmd, flags, s, "clients")
			if err != nil {
				return err
			}
			alerts, err := loadResource(cmd, flags, s, "alerts")
			if err != nil {
				return err
			}
			result := fleet.FleetHealth(computers, clients, alerts, staleDays, time.Now())
			return emitResult(cmd, flags, result)
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 7, "Days without contact before a computer counts as stale")
	return cmd
}
