// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix commander-allowlist).
// pp:data-source computed

package cli

import (
	"fmt"
	"strings"

	"keeper-pp-cli/internal/keeper"

	"github.com/spf13/cobra"
)

func newNovelAllowlistCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allowlist",
		Short: "Print the read-only Commander commands this CLI can send and the matching Service Mode -c value",
		Long: `The allowlist is compiled into this CLI: these Commander commands, with fixed read-only
flags, are the only ones it can send. Set the Service Mode key's own command list to the same
set (service-create or service-config-add with -c) so Service Mode refuses anything else too.
switch-to-mc and switch-to-msp move Service Mode's shared session between managed companies; no
command on the list reads vault records or changes Keeper.`,
		Example: strings.Trim(`
  keeper-cli allowlist
  keeper-cli allowlist --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "computed"},
		RunE: func(cmd *cobra.Command, args []string) error {
			list := strings.Join(keeper.AllowedCommands, ",")
			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				return flags.printJSON(cmd, map[string]any{"commands": keeper.AllowedCommands, "service_mode_command_list": list})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Commander commands this CLI can send (read-only reports and the managed-company switch):")
			for _, c := range keeper.AllowedCommands {
				fmt.Fprintln(cmd.OutOrStdout(), "  "+c)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nService Mode command list (-c):")
			fmt.Fprintln(cmd.OutOrStdout(), "  "+list)
			return nil
		},
	}
	return cmd
}
