// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"runzero-pp-cli/internal/inventory"
)

// pp:data-source local
func newNovelExposureMapCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "exposure-map <cidr>",
		Short: "Roll up which protocols and ports are exposed across a CIDR, asset-counted per service.",
		Long: strings.Trim(`
Roll up exposed services across an address range: for the given CIDR, group the
locally-synced services by transport/port/protocol and count how many assets in
that range expose each. Surfaces which subnets concentrate risky exposed
services before a scan or segmentation change. Reads the local store; run
'inventory sync' first.`, "\n"),
		Example: strings.Trim(`
  runzero-cli exposure-map 10.0.0.0/8 --agent
  runzero-cli exposure-map 192.168.1.0/24 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			cidr := strings.TrimSpace(args[0])
			if cidr == "" {
				return fmt.Errorf("a CIDR is required, e.g. exposure-map 10.0.0.0/8")
			}
			st, err := openInvStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer st.Close()
			rows, err := inventory.ExposureMap(cmd.Context(), st.DB(), cidr)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/runzero-cli/data.db)")
	return cmd
}
