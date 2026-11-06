// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel parent: licence-position export and cross-distributor true-up.
// pp:data-source local

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelPositionCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "position",
		Short:       "Export licence positions and true them up across distributors",
		Example:     "  td-synnex-cli position export --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelPositionExportCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelPositionTrueupCmd(flags))
	return cmd
}
