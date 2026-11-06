// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix commander-allowlist).
// pp:data-source live
// pp:client-call

package cli

import (
	"strings"

	"keeper-pp-cli/internal/keeper"

	"github.com/spf13/cobra"
)

func newNovelProductsListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List MSP products, add-ons and file plans with pricing",
		Long:  `Runs 'msp-info --format=json --pricing'.`,
		Example: strings.Trim(`
  keeper-cli products list --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, "", fixed(keeper.Products()))
		},
	}
	return cmd
}
