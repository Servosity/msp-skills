// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command group (keeper handfix novel-literal-registration: literal AddCommand).
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelCompaniesCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "companies",
		Short:       "Managed companies: plan, seats and add-ons",
		Example:     "  keeper-cli companies get 4421 --agent",
		Annotations: map[string]string{"pp:parent-group": "true", "mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelCompaniesGetCmd(flags))
	cmd.AddCommand(newNovelCompaniesListCmd(flags))
	return cmd
}
