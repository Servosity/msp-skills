// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/fleet"
)

func newNovelAgentVersionsCmd(flags *rootFlags) *cobra.Command {
	var behindOnly bool

	cmd := &cobra.Command{
		Use:   "agent-versions",
		Short: "Automate agent versions across the fleet, newest first, with the computers still behind counted per client",
		Long: strings.Trim(`
Group every computer by its Automate agent version (RemoteAgentVersion), newest
version first, with a per-client count for each version. The report names the
newest version any synced computer runs and counts the computers on an older
one; computers that report no version are counted apart. Pass --behind-only to
list just the versions that are behind. Agents left behind after a server
upgrade are the ones that stop patching and scripting first.

Reads the local SQLite mirror: run 'sync' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-automate-cli agent-versions --agent
  connectwise-automate-cli agent-versions --behind-only --agent`, "\n"),
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
			return emitResult(cmd, flags, fleet.AgentVersions(computers, clients, behindOnly))
		},
	}
	cmd.Flags().BoolVar(&behindOnly, "behind-only", false, "Show only versions older than the newest one in the fleet")
	return cmd
}
