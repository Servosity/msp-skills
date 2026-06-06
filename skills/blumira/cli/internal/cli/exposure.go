// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: agent & domain-controller exposure map.
// pp:data-source local

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func newNovelExposureCmd(flags *rootFlags) *cobra.Command {
	var flagFlagDcStale bool
	var flagStaleAfter string

	cmd := &cobra.Command{
		Use:   "exposure",
		Short: "Agent devices that are stale, isolated, or unprotected — domain controllers first",
		Long: "Roll up synced agent devices and flag the concerning ones: stale check-in, never\n" +
			"checked in, isolated, excluded, or sleeping. Domain controllers sort first. Use\n" +
			"--flag-dc-stale to restrict to domain controllers. Reads the local store.",
		Example: "  blumira-cli exposure --flag-dc-stale\n" +
			"  blumira-cli exposure --stale-after 12h --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			staleAfter, err := parseWindow(flagStaleAfter)
			if err != nil {
				return usageErr(err)
			}
			s, err := openAnalyticsStore(cmd.Context())
			if err != nil {
				return configErr(err)
			}
			defer s.Close()
			if !hintIfUnsynced(cmd, s, "org-agents-devices") {
				hintIfStale(cmd, s, "org-agents-devices", flags.maxAge)
			}
			devices, err := loadDevices(s)
			if err != nil {
				return apiErr(err)
			}
			if len(devices) == 0 {
				return emitAnalyticsRows(cmd, flags, []exposureRow{}, 0,
					"no agent devices in the local store — run 'sync --resource org-agents-devices' first")
			}
			rows := computeExposure(devices, time.Now().UTC(), exposureOpts{
				staleAfter: staleAfter,
				onlyDCs:    flagFlagDcStale,
			})
			hint := "no exposed devices: all agents are healthy and checked in recently"
			if flagFlagDcStale {
				hint = "no exposed domain controllers: all DC agents are healthy and checked in recently"
			}
			return emitAnalyticsRows(cmd, flags, rows, len(rows), hint)
		},
	}
	cmd.Flags().BoolVar(&flagFlagDcStale, "flag-dc-stale", false, "Restrict to domain controllers (flag stale/unprotected DCs only)")
	cmd.Flags().StringVar(&flagStaleAfter, "stale-after", "24h", "Treat an agent as stale if last seen longer ago than this (e.g. 12h, 1d)")
	return cmd
}
