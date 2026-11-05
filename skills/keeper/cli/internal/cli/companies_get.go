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

func newNovelCompaniesGetCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <company-id>",
		Short: "Show one managed company's plan, seats and add-ons by id",
		Long: `Runs 'msp-info --format=json --verbose --managed-company <id>'. Only a numeric company_id is accepted
(Commander would resolve anything else as a company name).`,
		Example: strings.Trim(`
  keeper-cli companies get 4421 --agent`, "\n"),
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, "", func() (keeper.Command, error) { return keeper.Company(args[0]) })
		},
	}
	return cmd
}
