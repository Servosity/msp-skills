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

func newNovelCompaniesListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every managed company with plan, seats allocated and active, storage and add-ons",
		Long: `Runs Commander 'msp-info --format=json --verbose' through Service Mode. company_id is the id every
--mc flag takes. allocated is -1 for unlimited seats; addons lists ADDON[:SEATS].`,
		Example: strings.Trim(`
  keeper-cli companies list --agent
  keeper-cli companies list --agent --select company_id,company_name,allocated,active`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, "", fixed(keeper.Companies()))
		},
	}
	return cmd
}
