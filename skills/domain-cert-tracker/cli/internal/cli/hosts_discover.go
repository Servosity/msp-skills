// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json tracker-novels).
// pp:data-source local

package cli

import (
	"domain-cert-tracker-pp-cli/internal/tracker"

	"github.com/spf13/cobra"
)

func newNovelHostsDiscoverCmd(flags *rootFlags) *cobra.Command {
	var client string
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "List hostnames that have certificates in CT for a client's domains but are not in that client's inventory.",
		Long: `Use this command to list hostnames that appear in CT for a client's domains but
are not in that client's inventory. Do NOT use this command to judge whether a
certificate's issuer was authorized; use 'issuers audit' instead.

Wildcard names are listed with wildcard: true. Add a host you want watched
with 'clients set <client> --host <name>'. Reads the local store only.`,
		Example:     "  domain-cert-tracker-cli hosts discover --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "hosts discover")
			}
			db, err := openTrackerRead(cmd.Context())
			if err != nil {
				return err
			}
			res := tracker.HostsResult{Hosts: []tracker.UnwatchedHost{}, Unverified: []tracker.Unverified{}}
			if db != nil {
				defer db.Close()
				if res, err = tracker.DiscoverHosts(cmd.Context(), db, client); err != nil {
					return trackerErr(err)
				}
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&client, "client", "", "Only this inventoried client")
	return cmd
}
