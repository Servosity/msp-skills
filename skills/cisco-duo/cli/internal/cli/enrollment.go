// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelEnrollmentCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "enrollment",
		Short:       "Find users stuck unenrolled and re-send their Duo enrollment emails",
		Example:     "  cisco-duo-cli enrollment chase --older-than 72h --limit 25",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelEnrollmentChaseCmd(flags))
	return cmd
}
