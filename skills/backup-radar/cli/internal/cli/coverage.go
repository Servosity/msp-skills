// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "coverage",
		Short:       "Compare backup vendor device lists with Backup Radar monitoring",
		Example:     "  backup-radar-cli coverage diff --devices vendor-devices.json --name-field hostname --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelCoverageDiffCmd(flags))
	return cmd
}
