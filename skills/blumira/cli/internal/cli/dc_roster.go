// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: domain-controller roster.
// pp:data-source local

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func newNovelDcRosterCmd(flags *rootFlags) *cobra.Command {
	var flagStaleAfter string

	cmd := &cobra.Command{
		Use:   "dc-roster",
		Short: "Full inventory of every domain controller across accounts with check-in state",
		Long: "Use this command for the plain inventory roster of all domain controllers and\n" +
			"their agent check-in state (protected, stale, or never-checked-in). Do NOT use\n" +
			"this command for the prioritized list of stale/unprotected DCs needing action;\n" +
			"use 'exposure' instead. Reads the local store.",
		Example: "  blumira-cli dc-roster\n" +
			"  blumira-cli dc-roster --stale-after 12h --json",
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
				return emitAnalyticsRows(cmd, flags, []dcRosterRow{}, 0,
					"no agent devices in the local store — run 'sync --resources org-agents-devices' first")
			}
			rows := computeDCRoster(devices, time.Now().UTC(), staleAfter)
			return emitAnalyticsRows(cmd, flags, rows, len(rows),
				"no domain controllers found among the synced agent devices")
		},
	}
	cmd.Flags().StringVar(&flagStaleAfter, "stale-after", "24h", "Treat a DC as stale if last seen longer ago than this (e.g. 12h, 1d)")
	return cmd
}
