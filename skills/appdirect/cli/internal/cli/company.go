// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelCompanyCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "company",
		Short:       "Work with company",
		Example:     "  appdirect-cli company show 12345678-aaaa-bbbb-cccc-1234567890ab --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelCompanyShowCmd(flags))
	return cmd
}
