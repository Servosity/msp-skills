// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet health is the one-row-per-PBX morning check.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/fleet"
)

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var scope fleetScope
	var warnWindow, backupStale, failOn string
	var noSave bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Rate each client phone system pass/warn/fail on version, licence, backup, trunks, disk, services and capacity (one row per PBX; --fail-on exits 8)",
		Long: `One row per client phone system with pass, warn or fail for version, licence and
maintenance expiry, last backup, trunks and extensions registered, disk,
stopped services, recording or voicemail quotas, and simultaneous-call capacity.
A check whose telemetry the PBX did not report is unknown (worse than warn), never pass.

Use this command for the one-row-per-PBX pass/warn/fail morning check. Do NOT use it to list individual trunks; use 'fleet trunks' instead. Do NOT use it for what changed since the last run; use 'fleet changes' instead.

Each run is saved locally so 'fleet changes' can compare runs (skip with --no-save).
With --fail-on warn|fail the command exits 8 when any PBX is at or above that level
(unknown counts for --fail-on warn).`,
		Example: strings.Trim(`
  threecx-cli fleet health --agent
  threecx-cli fleet health --tag gold --fail-on fail
  threecx-cli fleet health --warn-within 60d --backup-stale 26h --json`, "\n"),
		Annotations: map[string]string{"mcp:local-write": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,8"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			warnDays, err := parseDays("warn-within", warnWindow)
			if err != nil {
				return err
			}
			stale, err := cliutil.ParseDurationLoose(backupStale)
			if err != nil || stale <= 0 {
				return usageErr(fmt.Errorf("--backup-stale %q: use a duration like 48h or 2d", backupStale))
			}
			threshold, err := failLevel(failOn)
			if err != nil {
				return err
			}
			pbxes, full, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(pbxes) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			opts := fleet.HealthOptions{Now: time.Now().UTC(), WarnDays: warnDays, BackupStale: stale}
			rows, failed := collect(cmd, &scope, flags, pbxes, []fleet.Request{{Path: "/SystemStatus"}},
				func(p fleet.PBX, bodies []json.RawMessage) ([]fleet.HealthRow, error) {
					var s fleet.SystemStatus
					if err := decodeInto(bodies, &s); err != nil {
						return nil, fmt.Errorf("decoding /SystemStatus: %w", err)
					}
					return []fleet.HealthRow{fleet.EvaluateHealth(p, s, opts)}, nil
				}, fleet.ErrorHealthRow)
			fleet.SortHealth(rows)
			saveRun(cmd, noSave, fleet.Snapshot{RunAt: opts.Now, Kind: fleet.KindHealth, Scope: fleet.NamesOf(pbxes), Full: full, Health: rows})
			err = render(cmd, flags, rows, []string{"pbx", "status", "problems", "error", "version",
				"license_days_left", "maintenance_days_left", "backup_age_hours", "trunks_registered", "trunks_total"},
				"No PBXes in scope.", func(r fleet.HealthRow) map[string]any {
					return map[string]any{"pbx": r.PBX, "status": colorLevel(r.Status), "version": r.Version,
						"licence_days": intOrDash(r.LicenseDaysLeft), "backup_age_h": intOrDash(r.BackupAgeHours),
						"trunks": ratio(r.TrunksRegistered, r.TrunksTotal), "problems": fleet.FirstNonEmpty(r.Problems, r.Error)}
				})
			if err != nil {
				return err
			}
			if err := allFailed(failed, len(pbxes)); err != nil {
				return err
			}
			if threshold != "" {
				for _, r := range rows {
					if r.Status == threshold || fleet.Worse(r.Status, threshold) {
						return fleetProblemsErr("%s is %s: %s", r.PBX, r.Status, fleet.FirstNonEmpty(r.Problems, r.Error))
					}
				}
			}
			return nil
		},
	}
	addFleetScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&warnWindow, "warn-within", "30d", "Warn when licence or maintenance expires within this many days")
	cmd.Flags().StringVar(&backupStale, "backup-stale", "48h", "Fail when the last backup is older than this")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "Exit 8 when any PBX is at this level or worse: warn or fail")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Do not save this run to the local history used by 'fleet changes'")
	return cmd
}

func failLevel(v string) (fleet.Level, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case "warn":
		return fleet.Warn, nil
	case "fail":
		return fleet.Fail, nil
	}
	return "", usageErr(fmt.Errorf("--fail-on %q: use warn or fail", v))
}

func colorLevel(l fleet.Level) string {
	switch l {
	case fleet.Pass:
		return green(string(l))
	case fleet.Warn, fleet.Unknown:
		return yellow(string(l))
	}
	return red(string(l))
}

func intOrDash(p *int) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprint(*p)
}

func ratio(a, b *int) string {
	if a == nil || b == nil {
		return "-"
	}
	return fmt.Sprintf("%d/%d", *a, *b)
}
