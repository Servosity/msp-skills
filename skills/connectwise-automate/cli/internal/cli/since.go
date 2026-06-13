// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/fleet"
)

func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var hours int

	cmd := &cobra.Command{
		Use:   "since",
		Short: "Fleet activity in the last N hours: new alerts, agents that checked in, and patches installed",
		Long: strings.Trim(`
Summarize what happened across the fleet in the last --hours, derived from the
records' own timestamps (no fabricated drift): alerts created in the window
(highest priority first), the count of agents that checked in (last contact in
the window), and the count of patches installed in the window.

Reads the local SQLite mirror — run 'sync' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-automate-cli since --hours 24 --agent
  connectwise-automate-cli since --hours 8 --agent`, "\n"),
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
			patches, err := loadResource(s, "patching")
			if err != nil {
				return err
			}
			clients, err := loadResource(s, "clients")
			if err != nil {
				return err
			}
			result := fleet.Since(alerts, computers, patches, clients, hours, time.Now())
			return emitResult(cmd, flags, result)
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 24, "Look back this many hours")
	return cmd
}
