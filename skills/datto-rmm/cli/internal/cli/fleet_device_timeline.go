// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored novel feature: one device's full chronological history.
// Stitches synced alerts (open + resolved) and activity-log entries (which
// carry job, audit, and user actions) into a single time-ordered stream — a
// view that spans three API resource families and only exists as a local join.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"datto-rmm-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type timelineEvent struct {
	Time     string `json:"time"`
	Source   string `json:"source"`
	Category string `json:"category,omitempty"`
	Summary  string `json:"summary"`
	Site     string `json:"site,omitempty"`
}

type timelineView struct {
	Device string          `json:"device"`
	Days   int             `json:"days"`
	Events []timelineEvent `json:"events"`
	Note   string          `json:"note,omitempty"`
}

type fleetActivityLog struct {
	Action   string          `json:"action"`
	Category string          `json:"category"`
	Date     json.RawMessage `json:"date"`
	Details  string          `json:"details"`
	DeviceID json.RawMessage `json:"deviceId"`
	Entity   string          `json:"entity"`
	Hostname string          `json:"hostname"`
	Site     string          `json:"site"`
	User     string          `json:"user"`
}

const fleetActivityLogsResource = "activity-logs"

// matchesDevice reports whether an alert or log row belongs to the requested
// device key (device UID, hostname, or numeric device id — case-insensitive).
func matchesDevice(key, uid, name string, deviceID json.RawMessage) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	if strings.ToLower(uid) == k || strings.ToLower(name) == k {
		return true
	}
	if len(deviceID) > 0 {
		var n json.Number
		if err := json.Unmarshal(deviceID, &n); err == nil && n.String() == k {
			return true
		}
	}
	return false
}

// buildTimeline merges alert and activity-log events for one device into a
// single stream, newest first, bounded by days (0 = no window) and limit.
func buildTimeline(alerts []fleetAlert, logs []fleetActivityLog, key string, days, limit int, now time.Time) []timelineEvent {
	cutoff := now.AddDate(0, 0, -days)
	type stamped struct {
		t  time.Time
		ev timelineEvent
	}
	events := []stamped{}

	for _, a := range alerts {
		if !matchesDevice(key, a.AlertSourceInfo.DeviceUID, a.AlertSourceInfo.DeviceName, nil) {
			continue
		}
		t, ok := parseDattoTime(a.Timestamp)
		if !ok {
			continue
		}
		if days > 0 && t.Before(cutoff) {
			continue
		}
		source := "alert-open"
		if a.Resolved {
			source = "alert-resolved"
		}
		events = append(events, stamped{t: t, ev: timelineEvent{
			Time:     t.UTC().Format(time.RFC3339),
			Source:   source,
			Category: a.Priority,
			Summary:  "alert " + a.AlertUID,
			Site:     a.AlertSourceInfo.SiteName,
		}})
	}

	for _, l := range logs {
		if !matchesDevice(key, "", l.Hostname, l.DeviceID) {
			continue
		}
		t, ok := parseDattoTime(l.Date)
		if !ok {
			continue
		}
		if days > 0 && t.Before(cutoff) {
			continue
		}
		summary := strings.TrimSpace(l.Action)
		if l.Details != "" {
			summary = strings.TrimSpace(summary + ": " + l.Details)
		}
		if summary == "" {
			summary = l.Entity
		}
		if l.User != "" {
			summary += " (by " + l.User + ")"
		}
		events = append(events, stamped{t: t, ev: timelineEvent{
			Time:     t.UTC().Format(time.RFC3339),
			Source:   "activity",
			Category: l.Category,
			Summary:  summary,
			Site:     l.Site,
		}})
	}

	sort.SliceStable(events, func(i, j int) bool { return events[i].t.After(events[j].t) })
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	out := make([]timelineEvent, 0, len(events))
	for _, e := range events {
		out = append(out, e.ev)
	}
	return out
}

func loadFleetActivityLogs(ctx context.Context, db *store.Store) ([]fleetActivityLog, error) {
	raws, err := queryRaw(ctx, db, fleetActivityLogsResource)
	if err != nil {
		return nil, err
	}
	out := make([]fleetActivityLog, 0, len(raws))
	for _, raw := range raws {
		var l fleetActivityLog
		if err := json.Unmarshal(raw, &l); err != nil {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

// pp:data-source local
func newNovelFleetDeviceTimelineCmd(flags *rootFlags) *cobra.Command {
	var days int
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "device-timeline <deviceUid|hostname>",
		Short: "One device's chronological history across alerts and activity-log events",
		Long: strings.TrimSpace(`
Use this command to assemble one device's full chronological history across
alerts and activity-log entries (which include job, audit, and user actions).
Do NOT use it for a fleet-wide alert ranking; use 'fleet storms' instead.
Do NOT use it to fetch raw alerts; use 'device alerts' instead.`),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "device=ACME-DC01",
		},
		Example: `  datto-rmm-cli fleet device-timeline ACME-DC01 --days 30
  datto-rmm-cli fleet device-timeline 3a9c4e2f-7d1b-4c8a-9914-6655440000aa --days 14 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would merge the device's alerts and activity-log entries into one timeline")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("<deviceUid|hostname> is required"))
			}
			key := args[0]

			db, err := openFleetStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, fleetAlertsOpenResource) {
				hintIfStale(cmd, db, fleetAlertsOpenResource, flags.maxAge)
			}

			ctx := cmd.Context()
			alerts, err := loadFleetAlerts(ctx, db)
			if err != nil {
				return err
			}
			logs, err := loadFleetActivityLogs(ctx, db)
			if err != nil {
				return err
			}

			events := buildTimeline(alerts, logs, key, days, limit, time.Now().UTC())
			view := timelineView{Device: key, Days: days, Events: events}
			if len(events) == 0 {
				view.Note = fmt.Sprintf("no events for %q in the last %d days; check the device UID/hostname and that 'sync' has run (alerts + activity-logs)", key, days)
			}
			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"TIME", "SOURCE", "CATEGORY", "SUMMARY"}
			rows := make([][]string, 0, len(events))
			for _, e := range events {
				rows = append(rows, []string{e.Time, e.Source, e.Category, e.Summary})
			}
			if err := flags.printTable(cmd, headers, rows); err != nil {
				return err
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Event window in days (0 = all)")
	cmd.Flags().IntVar(&limit, "limit", 200, "Maximum events to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
