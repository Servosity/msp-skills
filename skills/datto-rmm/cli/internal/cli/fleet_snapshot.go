// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored novel feature: labeled point-in-time fleet snapshots. Freezes
// the current synced store into an immutable, dated archive so any reported
// fleet number can be reproduced later as a receipt (QBR baselines, audit
// evidence, client disputes).
package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelFleetSnapshotCmd(flags *rootFlags) *cobra.Command {
	var label string
	var note string
	var list bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Freeze the current synced fleet state into a labeled, dated receipt",
		Long: strings.TrimSpace(`
Use this command to freeze the fleet's current state under a label so a number
can be reproduced later. Do NOT use it to compare two points in time; use
'fleet diff' instead. Do NOT use it to pull fresh data; run 'sync' first.

Snapshots are immutable receipts: a label can only be written once.`),
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet snapshot --label q2-acme --note "QBR baseline"
  datto-rmm-cli fleet snapshot --list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would freeze the current synced store into a labeled snapshot")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openFleetStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if list {
				metas, err := db.ListFleetSnapshots(cmd.Context())
				if err != nil {
					return err
				}
				if shouldPrintJSON(cmd, flags) {
					return flags.printJSON(cmd, metas)
				}
				headers := []string{"LABEL", "CREATED", "ROWS", "NOTE"}
				rows := make([][]string, 0, len(metas))
				for _, m := range metas {
					rows = append(rows, []string{m.Label, m.CreatedAt, strconv.Itoa(m.RowCount), m.Note})
				}
				return flags.printTable(cmd, headers, rows)
			}

			if label == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--label is required (or use --list to see existing snapshots)"))
			}

			if !hintIfUnsynced(cmd, db, fleetDevicesResource) {
				hintIfStale(cmd, db, fleetDevicesResource, flags.maxAge)
			}

			meta, err := db.CreateFleetSnapshot(cmd.Context(), label, note)
			if err != nil {
				return err
			}
			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, meta)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "snapshot %q created at %s (%d rows)\n", meta.Label, meta.CreatedAt, meta.RowCount)
			return nil
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "Snapshot label (immutable; required to create)")
	cmd.Flags().StringVar(&note, "note", "", "Optional note recorded with the snapshot")
	cmd.Flags().BoolVar(&list, "list", false, "List existing snapshots instead of creating one")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
