// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet changes diffs saved fleet runs.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/fleet"
)

type fleetChangeRow struct {
	fleet.Change
	Kind    string `json:"kind"`
	FromRun string `json:"from_run"`
	ToRun   string `json:"to_run"`
}

func newNovelFleetChangesCmd(flags *rootFlags) *cobra.Command {
	var since string

	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Show what changed between saved fleet health or trunk runs: trunks down or back, version, licence, backup, reachability (--since 24h)",
		Long: `What changed across the fleet between the latest saved run and the last run at
or before --since ago: trunks offline or back, version bumped, licence or
maintenance renewed, backup gone stale, disk filling, PBX unreachable or back.

Use this command for what changed across the fleet between two snapshot runs (trunk went offline, version bumped, licence renewed, backup went stale). Do NOT use this command for the current state of trunks or PBX health; use 'fleet trunks' or 'fleet health' instead.

Runs are saved by 'fleet health' and 'fleet trunks'. With fewer than two saved
runs the result is empty.`,
		Example: strings.Trim(`
  threecx-cli fleet changes --since 24h --agent
  threecx-cli fleet changes --since 7d --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet changes")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window < 0 {
				return usageErr(fmt.Errorf("--since %q: use a duration like 24h or 7d", since))
			}
			hist, err := fleet.LoadHistory()
			if err != nil {
				return configErr(err)
			}
			cutoff := time.Now().UTC().Add(-window)
			rows := make([]fleetChangeRow, 0)
			compared := 0
			for _, kind := range []string{fleet.KindHealth, fleet.KindTrunks} {
				from, to, ok := fleet.PickRuns(hist, kind, cutoff, pinnedPBX)
				if !ok {
					continue
				}
				compared++
				var changes []fleet.Change
				if kind == fleet.KindHealth {
					changes = fleet.DiffHealth(from, to)
				} else {
					changes = fleet.DiffTrunks(from, to)
				}
				for _, c := range changes {
					if pinnedPBX != "" && c.PBX != pinnedPBX {
						continue
					}
					rows = append(rows, fleetChangeRow{Change: c, Kind: kind,
						FromRun: from.RunAt.Format(time.RFC3339), ToRun: to.RunAt.Format(time.RFC3339)})
				}
			}
			if compared == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: fewer than two saved runs; run '%s fleet health' (and 'fleet trunks') now and again later\n", cmd.Root().Name())
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), rows, flags, "pbx", "change", "subject", "from", "to")
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No changes.")
				return nil
			}
			items := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				items = append(items, map[string]any{"pbx": r.PBX, "change": r.Change.Change, "subject": r.Subject, "from": r.From, "to": r.To})
			}
			return printAutoTable(cmd.OutOrStdout(), items)
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Compare the latest run with the last run at or before this long ago (e.g. 24h, 7d)")
	return cmd
}
