// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"auvik-pp-cli/internal/cliutil"
)

// scorecardRow is one client's health rollup. Every count comes from the same
// builder the single-purpose command uses (eol, configuration audit, device
// discovery-gaps, alert noise), so the scorecard can never disagree with the
// command an operator runs to drill in.
type scorecardRow struct {
	Client         string `json:"client"`
	Devices        int    `json:"devices"`
	EOLExpired     int    `json:"eol_expired"`
	EOLExpiring90  int    `json:"eol_expiring_90d"`
	EOLExpiring365 int    `json:"eol_expiring_365d"`
	BackupMissing  int    `json:"backup_missing"`
	BackupStale    int    `json:"backup_stale"`
	DiscoveryGaps  int    `json:"discovery_gaps"`
	Alerts         int    `json:"alerts"`
}

// scorecardReport carries the per-family record counts it was computed from:
// a zero there means "not synced", which must never read as "healthy".
type scorecardReport struct {
	Since          string         `json:"since"`
	StaleAfterDays int            `json:"stale_after_days"`
	Records        map[string]int `json:"records"`
	Totals         scorecardRow   `json:"totals"`
	Rows           []scorecardRow `json:"rows"`
	Unsynced       []string       `json:"unsynced,omitempty"`
	Note           string         `json:"note,omitempty"`
}

func newNovelTenantsScorecardCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath  string
		sinceIn string
		staleIn string
		client  string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "scorecard",
		Short: "Per-client health rollup: end-of-support exposure, backup gaps, blind spots and alerts.",
		Long: strings.Trim(`
One row per client with the numbers a client review needs: devices, devices past
or near end-of-support, devices with no or a stale configuration backup, devices
Auvik cannot fully discover, and alert volume over a window. Sorted so the
client that needs attention first is on top.

Each column is computed by the same code as its single-purpose command, so drill
in with 'eol', 'configuration audit', 'device discovery-gaps' or 'alert noise'.
A family that was never synced is listed under "unsynced"; its columns are 0
because nothing was read, not because the client is healthy.

Reads the local mirror. Run this first:
  auvik-cli sync --resources tenants,inventory-device-info,auvik-inventory-lifecycle,inventory-device-lifecycle,inventory-device-warranty,inventory-configuration,inventory-device-detail,auvik-inventory,alert --full
`, "\n"),
		Example: strings.Trim(`
  # Every client, worst first
  auvik-cli tenants scorecard --since 30d --agent

  # One client, backups older than a week count as stale
  auvik-cli tenants scorecard --client acme --stale 7d --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--limit=5",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "tenants scorecard")
			}
			window, err := cliutil.ParseDurationLoose(sinceIn)
			if err != nil || window <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--since must be a positive age like 24h, 7d or 4w"))
			}
			stale, err := cliutil.ParseDurationLoose(staleIn)
			if err != nil || stale <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--stale must be a positive age like 7d or 4w"))
			}

			empty := scorecardReport{Records: map[string]int{}, Rows: []scorecardRow{}}
			db, handled, err := openLocalMirror(cmd, flags, dbPath, empty)
			if err != nil || handled {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtDevices.current()) {
				hintIfStale(cmd, db, rtDevices.current(), flags.maxAge)
			}

			ctx := cmd.Context()
			load := map[string]auvikFamily{
				"devices": rtDevices, "lifecycle_v1": rtDeviceLifecycle, "lifecycle_v2": rtDeviceLifecycleV2,
				"warranty": rtDeviceWarranty, "configurations": rtConfigurations, "device_detail": rtDeviceDetail,
				"discovery_status": rtDiscoveryStatus, "alerts": rtAlerts,
			}
			rows := map[string][]auvikRow{}
			for key, fam := range load {
				got, err := loadResources(ctx, db, fam)
				if err != nil {
					return err
				}
				rows[key] = got
			}
			now := time.Now().UTC()
			report := buildScorecardReport(scorecardInputs{
				devices: rows["devices"], lifecycleV1: rows["lifecycle_v1"], lifecycleV2: rows["lifecycle_v2"],
				warranties: rows["warranty"], configs: rows["configurations"], details: rows["device_detail"],
				statuses: rows["discovery_status"], alerts: rows["alerts"],
			}, tenantNames(ctx, db), now, now.Add(-window), stale)
			report.Since = sinceIn

			if client != "" {
				needle := strings.ToLower(client)
				kept := make([]scorecardRow, 0, len(report.Rows))
				for _, r := range report.Rows {
					if strings.Contains(strings.ToLower(r.Client), needle) {
						kept = append(kept, r)
					}
				}
				report.Rows = kept
			}
			if limit > 0 && len(report.Rows) > limit {
				report.Rows = report.Rows[:limit]
			}

			done, err := emitLocalReport(cmd, flags, report, report.Rows)
			if err != nil || done {
				return err
			}

			out := cmd.OutOrStdout()
			if len(report.Unsynced) > 0 {
				fmt.Fprintf(out, "Not synced (columns read 0): %s\n\n", strings.Join(report.Unsynced, ", "))
			}
			if len(report.Rows) == 0 {
				if report.Note != "" {
					fmt.Fprintln(out, report.Note)
				} else {
					fmt.Fprintln(out, "No clients match the given filters.")
				}
				return nil
			}
			fmt.Fprintf(out, "%-24s %7s %7s %7s %8s %7s %7s %6s %7s\n",
				"CLIENT", "DEVICES", "EOL", "EOL<90D", "EOL<365D", "NO-BKP", "STALE", "BLIND", "ALERTS")
			for _, r := range report.Rows {
				fmt.Fprintf(out, "%-24s %7d %7d %7d %8d %7d %7d %6d %7d\n",
					truncate(r.Client, 24), r.Devices, r.EOLExpired, r.EOLExpiring90, r.EOLExpiring365,
					r.BackupMissing, r.BackupStale, r.DiscoveryGaps, r.Alerts)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&sinceIn, "since", "30d", "Count alerts newer than this age (e.g. 24h, 7d, 4w)")
	cmd.Flags().StringVar(&staleIn, "stale", "30d", "Configuration backups older than this count as stale (e.g. 7d, 4w)")
	cmd.Flags().StringVar(&client, "client", "", "Only show clients whose name contains this substring")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum clients to return (0 = all)")
	return cmd
}

type scorecardInputs struct {
	devices, lifecycleV1, lifecycleV2, warranties, configs, details, statuses, alerts []auvikRow
}

// buildScorecardReport is the pure core, table-testable without SQLite. It
// runs each single-purpose builder over the whole fleet and folds the results
// per client.
func buildScorecardReport(in scorecardInputs, tenants map[string]string, now, since time.Time, stale time.Duration) scorecardReport {
	report := scorecardReport{
		StaleAfterDays: int(stale.Hours() / 24),
		Records: map[string]int{
			"devices": len(in.devices), "lifecycle_v2": len(in.lifecycleV2), "lifecycle_v1": len(in.lifecycleV1),
			"warranty": len(in.warranties), "configurations": len(in.configs), "device_detail": len(in.details),
			"discovery_status": len(in.statuses), "alerts": len(in.alerts),
		},
		Rows: []scorecardRow{},
	}
	if len(in.devices) == 0 {
		report.Note = "no devices in the local mirror; run 'auvik-cli sync --resources tenants,inventory-device-info --full'"
		report.Unsynced = []string{"devices"}
		return report
	}
	// A family is "unsynced" for the scorecard when nothing that feeds its
	// column was read; the column then says nothing about the client.
	unsynced := func(label string, n int) {
		if n == 0 {
			report.Unsynced = append(report.Unsynced, label)
		}
	}
	unsynced("end-of-support (lifecycle + warranty)", len(in.lifecycleV1)+len(in.lifecycleV2)+len(in.warranties))
	unsynced("configuration backups", len(in.configs))
	unsynced("discovery status", len(in.statuses)+len(in.details))
	unsynced("alerts", len(in.alerts))

	byClient := map[string]*scorecardRow{}
	row := func(client string) *scorecardRow {
		if r, ok := byClient[client]; ok {
			return r
		}
		r := &scorecardRow{Client: client}
		byClient[client] = r
		return r
	}
	for _, d := range in.devices {
		row(tenantLabel(tenants, d.rel("tenant"))).Devices++
	}
	for _, e := range buildEolReport(in.devices, in.lifecycleV1, in.lifecycleV2, in.warranties, tenants, now).Rows {
		switch e.Bucket {
		case bucketExpired:
			row(e.Client).EOLExpired++
		case bucket90:
			row(e.Client).EOLExpiring90++
		case bucket365:
			row(e.Client).EOLExpiring365++
		}
	}
	if len(in.configs) > 0 {
		// With no backups synced at all, configuration audit reports every
		// device as no_backup; that is a sync gap, already listed above, not a
		// finding to count against the client.
		for _, f := range buildConfigAuditReport(in.devices, in.configs, tenants, now, stale).Findings {
			switch f.Finding {
			case findingNoBackup:
				row(f.Client).BackupMissing++
			case findingStale:
				row(f.Client).BackupStale++
			}
		}
	}
	for _, g := range buildDiscoveryGapsReport(in.devices, in.details, in.statuses, tenants).Gaps {
		row(g.Client).DiscoveryGaps++
	}
	for _, n := range buildNoiseReport(in.alerts, in.devices, tenants, since, "client").Rows {
		row(n.Client).Alerts += n.Alerts
	}

	for _, r := range byClient {
		report.Rows = append(report.Rows, *r)
		t := &report.Totals
		t.Devices += r.Devices
		t.EOLExpired += r.EOLExpired
		t.EOLExpiring90 += r.EOLExpiring90
		t.EOLExpiring365 += r.EOLExpiring365
		t.BackupMissing += r.BackupMissing
		t.BackupStale += r.BackupStale
		t.DiscoveryGaps += r.DiscoveryGaps
		t.Alerts += r.Alerts
	}
	report.Totals.Client = "(all clients)"
	// Worst first: expired gear, then missing backups, then blind spots, then
	// alert volume; client name breaks ties so the order is stable.
	sort.Slice(report.Rows, func(i, j int) bool {
		a, b := report.Rows[i], report.Rows[j]
		if a.EOLExpired != b.EOLExpired {
			return a.EOLExpired > b.EOLExpired
		}
		if a.BackupMissing != b.BackupMissing {
			return a.BackupMissing > b.BackupMissing
		}
		if a.DiscoveryGaps != b.DiscoveryGaps {
			return a.DiscoveryGaps > b.DiscoveryGaps
		}
		if a.Alerts != b.Alerts {
			return a.Alerts > b.Alerts
		}
		return a.Client < b.Client
	})
	return report
}
