// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command group (keeper handfix novel-literal-registration: literal AddCommand).
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelPermitsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "permits",
		Short:       "MSP permits: the products and add-ons the MSP may allocate",
		Example:     "  keeper-cli permits list --agent",
		Annotations: map[string]string{"pp:parent-group": "true", "mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelPermitsListCmd(flags))
	return cmd
}
