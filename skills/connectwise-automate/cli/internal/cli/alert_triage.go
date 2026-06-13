// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/fleet"
)

func newNovelAlertTriageCmd(flags *rootFlags) *cobra.Command {
	var minPriority int

	cmd := &cobra.Command{
		Use:   "alert-triage",
		Short: "Open alerts across every client, ranked by priority and joined to computer → location → client",
		Long: strings.Trim(`
Filter open alerts to those at or above --min-priority, resolve each to its
computer, location, and client, collapse duplicate (computer, message) pairs to
the highest-priority instance, and rank priority-first. The cross-client triage
the per-monitor Automate view can't do.

Reads the local SQLite mirror — run 'sync' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-automate-cli alert-triage --min-priority 3 --agent
  connectwise-automate-cli alert-triage --agent --select client,computer,priority,message`, "\n"),
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
			alerts, err := loadResource(s, "alerts")
			if err != nil {
				return err
			}
			computers, err := loadResource(s, "computers")
			if err != nil {
				return err
			}
			clients, err := loadResource(s, "clients")
			if err != nil {
				return err
			}
			result := fleet.AlertTriage(alerts, computers, clients, minPriority)
			return emitResult(cmd, flags, result)
		},
	}
	cmd.Flags().IntVar(&minPriority, "min-priority", 0, "Only include alerts at or above this priority (higher = more severe)")
	return cmd
}
