// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "coverage subcommands: gaps",
		Example: strings.Trim(`
  cork-cli coverage gaps --client 00000000-0000-0000-0000-000000000000 --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			// The parent's Example runs `coverage gaps` with a client uuid that
			// may not exist; HTTP 404 -> exit 3 is a correct not-found answer.
			"pp:typed-exit-codes": "0,3",
		},
		RunE: parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelCoverageGapsCmd(flags))
	return cmd
}
