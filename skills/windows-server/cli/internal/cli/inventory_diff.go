// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `inventory diff`.
// Recorded in handfixes.json (health-events).
// pp:data-source local

package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/config"
	"windows-server-pp-cli/internal/store"
	"windows-server-pp-cli/internal/winsrv"
)

func newNovelInventoryDiffCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "diff",
		Short: "See what changed in roles, features, shares, local administrators, certificates, installed updates and VMs since the last inventory run.",
		Long: `Compares each server's two newest inventory snapshots in the local store
(no server is contacted) and lists every row added, removed or changed, with
the old and new value of each changed field. A section either snapshot could
not read is listed in not_compared instead of showing as all added or all
removed. Run 'inventory snapshot' on a schedule to feed it.`,
		Example: "  windows-server-cli inventory diff --agent\n  windows-server-cli inventory diff --server fs01 --agent --select server,changes.section,changes.change,changes.key",
		Args:    cobra.NoArgs,
		// pp:data-source local
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "inventory diff from the local store")
			}
			dbPath := defaultDBPath("windows-server-cli")
			if _, err := os.Stat(dbPath); err != nil {
				// SQLite reports a missing file as "out of memory (14)"; say what it means.
				return notFoundErr(fmt.Errorf("no inventory recorded yet: run 'windows-server-cli inventory snapshot' twice"))
			}
			db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			servers := []string{winsrvServer}
			if winsrvServer == "" {
				if servers, err = db.InventoryServers(cmd.Context()); err != nil {
					return err
				}
				// A configured server whose snapshots all failed still gets a row.
				if cfg, cerr := config.Load(flags.configPath); cerr == nil {
					profiles, _ := winsrv.LoadProfiles(cfg.Path)
					for _, p := range profiles {
						if !slices.Contains(servers, p.Name) {
							servers = append(servers, p.Name)
						}
					}
				}
			}
			if len(servers) == 0 {
				// No snapshot and no configured server: the same answer as a
				// store that does not exist yet, never an empty "no drift" list.
				return notFoundErr(fmt.Errorf("no inventory recorded yet: run 'windows-server-cli inventory snapshot' twice"))
			}
			out := []winsrv.Row{}
			for _, server := range servers {
				row, err := diffServer(cmd.Context(), db, server)
				if err != nil {
					return err
				}
				out = append(out, row)
			}
			sort.Slice(out, func(i, j int) bool { return winsrv.Str(out[i]["server"]) < winsrv.Str(out[j]["server"]) })
			return printWinsrv(cmd, flags, out, "server", "changes", "note")
		},
	}
}

func diffServer(ctx context.Context, db *store.Store, server string) (winsrv.Row, error) {
	runs, err := db.InventoryRuns(ctx, server, 2)
	if err != nil {
		return nil, err
	}
	row := winsrv.Row{"server": server, "id": server, "changes": []winsrv.Change{}}
	if len(runs) < 2 {
		row["note"] = "no snapshot recorded yet; diff needs two: run 'windows-server-cli inventory snapshot' now and again later"
		if len(runs) == 1 {
			row["note"] = "1 snapshot recorded; diff needs two: run 'windows-server-cli inventory snapshot' again later"
		}
		return row, nil
	}
	cur, err := db.InventorySections(ctx, runs[0].RunID)
	if err != nil {
		return nil, err
	}
	old, err := db.InventorySections(ctx, runs[1].RunID)
	if err != nil {
		return nil, err
	}
	changes, notCompared := winsrv.Diff(old, cur)
	if changes == nil {
		changes = []winsrv.Change{}
	}
	row["from_run"], row["from_taken_at"] = runs[1].RunID, runs[1].TakenAt
	row["to_run"], row["to_taken_at"] = runs[0].RunID, runs[0].TakenAt
	row["changes"], row["not_compared"] = changes, notCompared
	return row, nil
}
