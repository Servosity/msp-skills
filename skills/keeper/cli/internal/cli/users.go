// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command group (keeper handfix novel-literal-registration: literal AddCommand).
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelUsersCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "users",
		Short:       "Enterprise users: roster, last logins, and cross-client find, hygiene and risk views",
		Example:     "  keeper-cli users find jane@example.com --agent",
		Annotations: map[string]string{"pp:parent-group": "true", "mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelUsersFindCmd(flags))
	cmd.AddCommand(newNovelUsersHygieneCmd(flags))
	cmd.AddCommand(newNovelUsersLastLoginCmd(flags))
	cmd.AddCommand(newNovelUsersListCmd(flags))
	cmd.AddCommand(newNovelUsersRiskCmd(flags))
	return cmd
}
