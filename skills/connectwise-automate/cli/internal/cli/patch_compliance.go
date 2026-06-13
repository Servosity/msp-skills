// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/fleet"
)

func newNovelPatchComplianceCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "patch-compliance",
		Short: "Per-client patch posture from synced patch history joined to computers — worst offenders first",
		Long: strings.Trim(`
Group synced patch history by client (joined computer -> client) and compute a
per-client compliance percentage (installed / total), worst compliance first.
The Automate API only exposes per-computer patch stats; this is the cross-client
roll-up for a QBR or a security conversation.

Reads the local SQLite mirror — run 'sync' first.`, "\n"),
		Example: strings.Trim(`
  connectwise-automate-cli patch-compliance --agent
  connectwise-automate-cli patch-compliance --select client,compliance_pct,failed`, "\n"),
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
			patches, err := loadResource(s, "patching")
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
			result := fleet.PatchCompliance(patches, computers, clients)
			return emitResult(cmd, flags, result)
		},
	}
	return cmd
}
