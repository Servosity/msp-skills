// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newBackupCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup health across all customer accounts",
		Long: strings.Trim(`
Reports built from the synced partner-wide backup status feed. Run
'cloudally-cli sync' under a partner login (auth login --partner) first so
the local store has rows to read.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli backup health --stale-days 2
  cloudally-cli backup health --failing --group-by account
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newBackupHealthCmd(flags))
	return cmd
}

// newNovelBackupCmd keeps the generated root wiring compiling; the root
// dedupes it against the command added by newBackupCmd.
func newNovelBackupCmd(flags *rootFlags) *cobra.Command { return newBackupCmd(flags) }
