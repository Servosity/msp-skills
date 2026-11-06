// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet backups proves every PBX has a recent restorable backup.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/fleet"
)

func newNovelFleetBackupsCmd(flags *rootFlags) *cobra.Command {
	var scope fleetScope
	var stale string
	var problems bool

	cmd := &cobra.Command{
		Use:   "backups",
		Short: "Check backup age, schedule, rotation and newest backup file on every client phone system (--stale 48h; --problems for failures only)",
		Long: `Backup age, schedule, rotation and newest backup file for every client phone
system, flagged NEVER (no backup), STALE (older than --stale), UNSCHEDULED
(schedule off), SHRUNK (newest file under half the size of the previous one,
which usually means recordings or voicemail were dropped), SCHEDULE_UNKNOWN
(no schedule reported) or SIZE_UNKNOWN (the newest file's size is not reported).

Use this command for backup age, schedule, rotation and newest backup file per PBX. Do NOT use this command for the overall per-PBX pass/warn/fail roll-up; use 'fleet health' instead.`,
		Example: strings.Trim(`
  threecx-cli fleet backups --stale 48h --agent
  threecx-cli fleet backups --problems --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet backups")
			}
			staleAfter, err := cliutil.ParseDurationLoose(stale)
			if err != nil || staleAfter <= 0 {
				return usageErr(fmt.Errorf("--stale %q: use a duration like 48h or 2d", stale))
			}
			pbxes, _, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(pbxes) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			now := time.Now().UTC()
			reqs := []fleet.Request{{Path: "/SystemStatus"}, {Path: "/Backups/Pbx.GetBackupSettings()"}, {Path: "/Backups"}}
			rows, failed := collect(cmd, &scope, flags, pbxes, reqs,
				func(p fleet.PBX, bodies []json.RawMessage) ([]fleet.BackupRow, error) {
					var s fleet.SystemStatus
					var set fleet.BackupSettings
					var files []fleet.Backup
					if err := decodeInto(bodies, &s, &set, &files); err != nil {
						return nil, fmt.Errorf("decoding backup status: %w", err)
					}
					return []fleet.BackupRow{fleet.BackupRowFor(p, s, set, files, staleAfter, now)}, nil
				}, func(p fleet.PBX, err error) fleet.BackupRow {
					return fleet.BackupRow{PBX: p.Name, Status: "error", Flags: []string{}, Error: err.Error()}
				})
			if problems {
				kept := make([]fleet.BackupRow, 0, len(rows))
				for _, r := range rows {
					if r.Status != "ok" {
						kept = append(kept, r)
					}
				}
				rows = kept
			}
			sortBackups(rows)
			err = render(cmd, flags, rows, []string{"pbx", "status", "flags", "last_backup", "age_hours",
				"schedule_enabled", "newest_file", "error"}, "No backup problems.", func(r fleet.BackupRow) map[string]any {
				return map[string]any{"pbx": r.PBX, "status": r.Status, "flags": strings.Join(r.Flags, ","),
					"age_h": intOrDash(r.AgeHours), "schedule": r.Schedule, "newest_file": r.NewestFile, "error": r.Error}
			})
			if err != nil {
				return err
			}
			return allFailed(failed, len(pbxes))
		},
	}
	addFleetScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&stale, "stale", "48h", "Flag backups older than this as STALE (e.g. 26h, 2d)")
	cmd.Flags().BoolVar(&problems, "problems", false, "Only PBXes with a backup problem or a read error")
	return cmd
}

// sortBackups puts errors, then flagged PBXes, then healthy ones.
func sortBackups(rows []fleet.BackupRow) {
	rank := func(r fleet.BackupRow) int {
		switch {
		case r.Error != "":
			return 0
		case r.Status != "ok":
			return 1
		}
		return 2
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if ri, rj := rank(rows[i]), rank(rows[j]); ri != rj {
			return ri < rj
		}
		return rows[i].PBX < rows[j].PBX
	})
}
