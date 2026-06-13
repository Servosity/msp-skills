// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/fleet"
)

func newNovelClientRollupCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "client-rollup [client]",
		Short: "One-line-per-client snapshot: computers, locations, offline agents, and open alerts — built for the client review",
		Long: strings.Trim(`
One row per client: computers, locations, offline agents, and open alerts —
the snapshot for a client review meeting. Pass an optional [client] substring
to narrow to matching clients (case-insensitive); omit it for the whole book.

Reads the local SQLite mirror — run 'sync' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-automate-cli client-rollup --agent
  connectwise-automate-cli client-rollup "Acme" --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			filter := ""
			if len(args) == 1 {
				filter = args[0]
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
			locations, err := loadResource(s, "locations")
			if err != nil {
				return err
			}
			alerts, err := loadResource(s, "alerts")
			if err != nil {
				return err
			}
			result := fleet.ClientRollups(computers, clients, locations, alerts, filter)
			return emitResult(cmd, flags, result)
		},
	}
	return cmd
}
