// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command group (keeper handfix novel-literal-registration: literal AddCommand).
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelPostureCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "posture",
		Short:       "Cross-client security posture: record runs, compare them, list them",
		Example:     "  keeper-cli posture diff --agent",
		Annotations: map[string]string{"pp:parent-group": "true", "mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelPostureDiffCmd(flags))
	cmd.AddCommand(newNovelPostureReportCmd(flags))
	cmd.AddCommand(newNovelPostureRunsCmd(flags))
	return cmd
}
