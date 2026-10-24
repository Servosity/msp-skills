// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelUsagesCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "usages",
		Short:       "Work with usages",
		Example:     "  acronis-cli usages drift --from 2026-04-01 --to 2026-05-01 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelUsagesSnapshotCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelUsagesDriftCmd(flags))
	return cmd
}
