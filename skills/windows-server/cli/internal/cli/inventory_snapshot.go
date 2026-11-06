// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `inventory snapshot`. Recorded in
// handfixes.json (health-events).
// pp:data-source live
// pp:client-call the reads are fixed WinRM scripts run through winsrv.Runner.

package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/store"
	"windows-server-pp-cli/internal/winsrv"
)

func newInventorySnapshotCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "snapshot",
		Short: "Record the inventory of every server in the local store for inventory diff",
		Long: `Reads, from every configured server (or the one --server names), the parts
of the inventory that should change only when someone changes them: ` + winsrv.SectionNames() + `.
Volatile values (uptime, free memory, service state) are not kept. A section
that could not be read is named in errors and is not compared by inventory
diff.`,
		Example:     "  windows-server-cli inventory snapshot --agent",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "live", "pp:live-happy-path": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "inventory snapshot on "+winsrvTarget()+" and record it")
			}
			if localDataDisabled() {
				return usageErr(fmt.Errorf("inventory snapshot is disabled while %s=1: it records server inventory in the local store", noLocalDataEnvVar))
			}
			profiles, credErrs, err := winsrvServers(flags, true)
			if err != nil {
				return err
			}
			db, err := openWinsrvStore(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			runner := newWinsrvRunner(flags)
			now := winsrvNow().UTC()
			results := eachServer(cmd.Context(), profiles, credErrs, func(ctx context.Context, p winsrv.Profile) (any, error) {
				snap := winsrv.TakeSnapshot(ctx, runner, p, now)
				if len(snap.Sections) == 0 {
					return snap, fmt.Errorf("no inventory section could be read: %w", snap.FirstErr)
				}
				return snap, nil
			})
			rows := []winsrv.Row{}
			for _, r := range results {
				snap, read := r.value.(winsrv.Snapshot)
				if r.err != nil {
					row := errorRow(r.profile.Name, r.err)
					if read {
						row["errors"] = snap.Errors
					}
					rows = append(rows, row)
					continue
				}
				runID, err := db.RecordInventory(cmd.Context(), store.InventoryRun{Server: snap.Server, TakenAt: now.Format(time.RFC3339), Errors: snap.Errors, Absent: snap.Absent}, snap.Sections)
				if err != nil {
					return err
				}
				counts := map[string]int{}
				for name, sec := range snap.Sections {
					counts[name] = len(sec)
				}
				rows = append(rows, winsrv.Row{"server": snap.Server, "id": fmt.Sprintf("%s/%d", snap.Server, runID), "run_id": runID,
					"taken_at": now.Format(time.RFC3339), "sections": counts, "errors": snap.Errors, "absent": snap.Absent})
			}
			if err := printWinsrv(cmd, flags, rows, "server", "run_id", "sections", "errors", "error"); err != nil {
				return err
			}
			return scriptExit(allFailed(results))
		},
	}
}
