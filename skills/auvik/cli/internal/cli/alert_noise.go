// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
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

type noiseRow struct {
	Rank        int            `json:"rank"`
	Client      string         `json:"client"`
	DeviceID    string         `json:"device_id,omitempty"`
	DeviceName  string         `json:"device_name"`
	DeviceType  string         `json:"device_type,omitempty"`
	Alerts      int            `json:"alerts"`
	Dismissed   int            `json:"dismissed"`
	TopSeverity string         `json:"top_severity,omitempty"`
	Severities  map[string]int `json:"severities,omitempty"`
	Statuses    map[string]int `json:"statuses,omitempty"`
}

type noiseReport struct {
	Since          string     `json:"since,omitempty"`
	GroupBy        string     `json:"group_by"`
	AlertsTotal    int        `json:"alerts_total"`
	AlertsInWindow int        `json:"alerts_in_window"`
	Rows           []noiseRow `json:"rows"`
	Note           string     `json:"note,omitempty"`
}

func newNovelAlertNoiseCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath  string
		sinceIn string
		groupBy string
		client  string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "noise",
		Short: "Rank devices and clients by alert volume over a window, with device names, types, clients and severity mix resolved.",
		Long: strings.Trim(`
Aggregates the locally synced alert history over a time window, resolving each
alert's device id to a device name, type, and client, and computing the mean
detected-to-dismissed duration per offender.

A bare count is just 'analytics --type alert --group-by deviceId'. The value
here is the join that turns opaque device ids into device names, types and
client names, plus the severity and status mix per offender.

Use this command for retrospective alert-volume ranking over the local alert
history. Do NOT use this command for working the live alert queue or dismissing
alerts; use 'alert triage' instead.

Reads the local mirror. Run this first:
  auvik-cli sync --resources tenants,inventory,alert --full
`, "\n"),
		Example: strings.Trim(`
  # Who ate my shift, last 30 days
  auvik-cli alert noise --since 30d --agent

  # Roll up by client instead of device
  auvik-cli alert noise --since 7d --group-by client --agent
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
				return writeDryRun(cmd.OutOrStdout(), flags, "alert noise")
			}
			if groupBy != "device" && groupBy != "client" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--group-by must be device or client"))
			}
			var since time.Time
			if sinceIn != "" {
				d, err := cliutil.ParseDurationLoose(sinceIn)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--since %q: %w", sinceIn, err))
				}
				since = time.Now().UTC().Add(-d)
			}

			empty := noiseReport{GroupBy: groupBy, Rows: []noiseRow{}}
			db, handled, err := openLocalMirror(cmd, flags, dbPath, empty)
			if err != nil || handled {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "alert") {
				hintIfStale(cmd, db, "alert", flags.maxAge)
			}

			ctx := cmd.Context()
			devices, err := loadResources(ctx, db, rtDevices)
			if err != nil {
				return err
			}
			alerts, err := loadResources(ctx, db, rtAlerts)
			if err != nil {
				return err
			}
			names := tenantNames(ctx, db)

			report := buildNoiseReport(alerts, devices, names, since, groupBy)
			if sinceIn != "" {
				report.Since = since.Format(time.RFC3339)
			}
			if client != "" {
				needle := strings.ToLower(client)
				filtered := make([]noiseRow, 0, len(report.Rows))
				for _, r := range report.Rows {
					if strings.Contains(strings.ToLower(r.Client), needle) {
						filtered = append(filtered, r)
					}
				}
				report.Rows = filtered
			}
			if limit > 0 && len(report.Rows) > limit {
				report.Rows = report.Rows[:limit]
			}
			if report.AlertsTotal == 0 {
				report.Note = "no alerts in the local mirror; run 'auvik-cli sync --resources alert --full'"
			} else if report.AlertsInWindow == 0 {
				report.Note = fmt.Sprintf("no alerts fell inside the requested window (%d alerts stored)", report.AlertsTotal)
			}

			done, err := emitLocalReport(cmd, flags, report, report.Rows)
			if err != nil || done {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Alert noise by %s -- %d alert(s) in window (of %d stored)\n\n",
				report.GroupBy, report.AlertsInWindow, report.AlertsTotal)
			if len(report.Rows) == 0 {
				fmt.Fprintln(out, report.Note)
				return nil
			}
			fmt.Fprintf(out, "%-4s %-22s %-28s %7s %10s  %s\n",
				"#", "CLIENT", "DEVICE", "ALERTS", "DISMISSED", "TOP SEVERITY")
			for _, r := range report.Rows {
				fmt.Fprintf(out, "%-4d %-22s %-28s %7d %10d  %s\n",
					r.Rank, truncate(r.Client, 22), truncate(r.DeviceName, 28),
					r.Alerts, r.Dismissed, r.TopSeverity)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&sinceIn, "since", "30d", "Only alerts newer than this age (e.g. 24h, 7d, 4w)")
	cmd.Flags().StringVar(&groupBy, "group-by", "device", "Roll up by device or client")
	cmd.Flags().StringVar(&client, "client", "", "Only show rows whose client name contains this substring")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum rows to return (0 = all)")
	return cmd
}

// buildNoiseReport is the pure aggregation core, split out for table tests.
func buildNoiseReport(alerts, devices []auvikRow, tenants map[string]string, since time.Time, groupBy string) noiseReport {
	type devMeta struct{ name, dtype, tenant string }
	meta := map[string]devMeta{}
	for _, d := range devices {
		meta[d.ID] = devMeta{
			name:   firstNonEmpty(d.attrString("deviceName"), d.attrString("name"), d.ID),
			dtype:  d.attrString("deviceType"),
			tenant: d.rel("tenant"),
		}
	}

	type agg struct {
		client     string
		deviceID   string
		deviceName string
		deviceType string
		alerts     int
		dismissed  int
		severities map[string]int
		statuses   map[string]int
	}
	buckets := map[string]*agg{}

	report := noiseReport{GroupBy: groupBy, AlertsTotal: len(alerts), Rows: make([]noiseRow, 0)}

	for _, a := range alerts {
		detected, ok := parseAuvikTime(a.attrString(fAlertDetectedOn.Field))
		if !ok {
			continue
		}
		if !since.IsZero() && detected.Before(since) {
			continue
		}
		report.AlertsInWindow++

		devID := firstNonEmpty(a.rel("entity"), a.rel("device"), a.attrString("entityId"), a.attrString("deviceId"))
		m := meta[devID]
		tenantID := firstNonEmpty(m.tenant, a.rel("tenant"))
		clientName := tenantLabel(tenants, tenantID)

		key := devID
		if groupBy == "client" {
			key = tenantID
		}
		if key == "" {
			key = "(unattributed)"
		}
		b := buckets[key]
		if b == nil {
			b = &agg{client: clientName, severities: map[string]int{}, statuses: map[string]int{}}
			if groupBy == "device" {
				b.deviceID = devID
				b.deviceName = firstNonEmpty(m.name, devID, "(unknown device)")
				b.deviceType = m.dtype
			} else {
				b.deviceName = clientName
			}
			buckets[key] = b
		}
		b.alerts++
		if sev := a.attrString(fAlertSeverity.Field); sev != "" {
			b.severities[sev]++
		}
		if truthy(a.attr(fAlertDismissed.Field)) {
			b.dismissed++
		}
		// NOTE: Auvik exposes `dismissed` as a BOOLEAN and publishes no
		// dismissal timestamp anywhere in alertAttributes, so a
		// mean-time-to-dismiss cannot be computed from this API. See
		// TestNoDismissalTimestampExists, which fails if that ever changes.
		if st := a.attrString(fAlertStatus.Field); st != "" {
			b.statuses[st]++
		}
	}

	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]noiseRow, 0, len(keys))
	for _, k := range keys {
		b := buckets[k]
		row := noiseRow{
			Client: b.client, DeviceID: b.deviceID, DeviceName: b.deviceName,
			DeviceType: b.deviceType, Alerts: b.alerts, Dismissed: b.dismissed,
			Severities: b.severities, TopSeverity: topKey(b.severities), Statuses: b.statuses,
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Alerts != rows[j].Alerts {
			return rows[i].Alerts > rows[j].Alerts
		}
		if rows[i].Client != rows[j].Client {
			return rows[i].Client < rows[j].Client
		}
		return rows[i].DeviceName < rows[j].DeviceName
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	report.Rows = rows
	return report
}

func topKey(counts map[string]int) string {
	best, bestN := "", -1
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if counts[k] > bestN {
			best, bestN = k, counts[k]
		}
	}
	return best
}
