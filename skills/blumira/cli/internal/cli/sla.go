// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: SLA aging watchlist.
// pp:data-source local

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func newNovelSlaCmd(flags *rootFlags) *cobra.Command {
	var flagBreachIn string
	var flagPriority string
	var flagSLA string

	cmd := &cobra.Command{
		Use:   "sla",
		Short: "Open findings ranked by time-to-breach against an age-based SLA, across all accounts",
		Long: "Rank open findings by how close they are to breaching an age SLA. Defaults are\n" +
			"per-priority (P1=4h, P2=8h, P3=24h, P4=72h, P5=168h); override with --sla.\n" +
			"Breached findings sort first (negative breach_in_hours). Reads the local store.",
		Example: "  blumira-cli sla --breach-in 4h --priority high\n" +
			"  blumira-cli sla --sla 24h --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			prioMatch, err := parsePriorityFilter(flagPriority)
			if err != nil {
				return usageErr(err)
			}
			breachIn, err := parseWindow(flagBreachIn)
			if err != nil {
				return usageErr(err)
			}
			override, err := parseWindow(flagSLA)
			if err != nil {
				return usageErr(err)
			}
			s, err := openAnalyticsStore(cmd.Context())
			if err != nil {
				return configErr(err)
			}
			defer s.Close()
			if !hintIfUnsynced(cmd, s, "") {
				hintIfStale(cmd, s, "", flags.maxAge)
			}
			findings, err := loadFindings(s)
			if err != nil {
				return apiErr(err)
			}
			rows := computeSLA(findings, time.Now().UTC(), slaOpts{
				override: override,
				breachIn: breachIn,
				priority: prioMatch,
			})
			return emitAnalyticsRows(cmd, flags, rows, len(rows), emptyStoreHint)
		},
	}
	cmd.Flags().StringVar(&flagBreachIn, "breach-in", "", "Only show findings breaching within this window (e.g. 4h, 1d); empty shows all open")
	cmd.Flags().StringVar(&flagPriority, "priority", "", "Filter by priority: high, critical, medium, low, p1..p5, or a number")
	cmd.Flags().StringVar(&flagSLA, "sla", "", "Override the age SLA for all priorities (e.g. 24h, 2d); empty uses per-priority defaults")
	return cmd
}
