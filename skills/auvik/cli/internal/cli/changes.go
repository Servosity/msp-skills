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

type changeEvent struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Actor   string `json:"actor,omitempty"`
	RefID   string `json:"ref_id,omitempty"`
	sortKey time.Time
}

type changesReport struct {
	DeviceID   string         `json:"device_id"`
	DeviceName string         `json:"device_name,omitempty"`
	Client     string         `json:"client,omitempty"`
	Since      string         `json:"since,omitempty"`
	Counts     map[string]int `json:"counts"`
	Events     []changeEvent  `json:"events"`
	Note       string         `json:"note,omitempty"`
}

func newNovelChangesCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath  string
		sinceIn string
		kind    string
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "changes [device-id]",
		Short: "Merge config revisions, audit entries, notes, and alerts for one device into a single chronological story.",
		Long: strings.Trim(`
Merges four separate Auvik record families for ONE device -- configuration
revisions, entity audit entries, entity notes, and alert history -- into one
chronological event stream. No single Auvik endpoint returns this.

Use this command for the full change timeline of ONE device. Do NOT use this
command for searching config text across the whole fleet; use
'configuration audit' instead, which reports backup COVERAGE -- Auvik's
Configuration API exposes backup metadata only, so config text cannot be
searched.

Reads the local mirror. Run this first:
  auvik-cli sync --resources tenants,inventory,inventory-configuration,inventory-entity-audit,inventory-entity-note,alert --full
`, "\n"),
		Example: strings.Trim(`
  # Everything that ever happened to this device
  auvik-cli changes 5f2b91c4 --agent

  # Only config revisions in the last month
  auvik-cli changes 5f2b91c4 --since 30d --kind config
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "device=example-device-id",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "changes")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a device id is required"))
			}
			deviceID := args[0]

			var since time.Time
			if sinceIn != "" {
				d, err := cliutil.ParseDurationLoose(sinceIn)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--since %q: %w", sinceIn, err))
				}
				since = time.Now().UTC().Add(-d)
			}

			empty := changesReport{DeviceID: deviceID, Counts: map[string]int{}, Events: []changeEvent{}}
			db, handled, err := openLocalMirror(cmd, flags, dbPath, empty)
			if err != nil || handled {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "inventory-configuration") {
				hintIfStale(cmd, db, "inventory-configuration", flags.maxAge)
			}

			ctx := cmd.Context()
			devices, err := loadResources(ctx, db, rtDevices)
			if err != nil {
				return err
			}
			configs, err := loadResources(ctx, db, rtConfigurations)
			if err != nil {
				return err
			}
			audits, err := loadResources(ctx, db, rtEntityAudit)
			if err != nil {
				return err
			}
			notes, err := loadResources(ctx, db, rtEntityNote)
			if err != nil {
				return err
			}
			alerts, err := loadResources(ctx, db, rtAlerts)
			if err != nil {
				return err
			}
			names := tenantNames(ctx, db)

			report := buildChangesReport(deviceID, devices, configs, audits, notes, alerts, names, since)
			if kind != "" {
				filtered := make([]changeEvent, 0, len(report.Events))
				for _, e := range report.Events {
					if e.Kind == kind {
						filtered = append(filtered, e)
					}
				}
				report.Events = filtered
			}
			if limit > 0 && len(report.Events) > limit {
				report.Events = report.Events[:limit]
			}
			if sinceIn != "" {
				report.Since = since.Format(time.RFC3339)
			}
			if len(report.Events) == 0 {
				report.Note = fmt.Sprintf("no recorded events for device %q in the local mirror", deviceID)
			}

			done, err := emitLocalReport(cmd, flags, report, report.Events)
			if err != nil || done {
				return err
			}

			out := cmd.OutOrStdout()
			label := report.DeviceName
			if label == "" {
				label = deviceID
			}
			fmt.Fprintf(out, "Timeline for %s", label)
			if report.Client != "" {
				fmt.Fprintf(out, " (%s)", report.Client)
			}
			fmt.Fprintln(out)
			if len(report.Events) == 0 {
				fmt.Fprintln(out, report.Note)
				return nil
			}
			fmt.Fprintf(out, "  config: %d   audit: %d   note: %d   alert: %d\n\n",
				report.Counts["config"], report.Counts["audit"],
				report.Counts["note"], report.Counts["alert"])
			for _, e := range report.Events {
				fmt.Fprintf(out, "%-22s %-7s %s", e.At, e.Kind, e.Summary)
				if e.Actor != "" {
					fmt.Fprintf(out, "  [%s]", e.Actor)
				}
				fmt.Fprintln(out)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&sinceIn, "since", "", "Only events newer than this age (e.g. 24h, 7d, 4w)")
	cmd.Flags().StringVar(&kind, "kind", "", "Only one event kind: config, audit, note, alert")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum events to return (0 = all)")
	return cmd
}

// buildChangesReport is the pure merge core, split out for table tests.
func buildChangesReport(deviceID string, devices, configs, audits, notes, alerts []auvikRow, tenants map[string]string, since time.Time) changesReport {
	report := changesReport{
		DeviceID: deviceID,
		Counts:   map[string]int{"config": 0, "audit": 0, "note": 0, "alert": 0},
		Events:   make([]changeEvent, 0),
	}
	for _, d := range devices {
		if d.ID == deviceID {
			report.DeviceName = d.attrString(fDeviceName.Field)
			report.Client = tenantLabel(tenants, d.rel("tenant"))
			break
		}
	}

	add := func(kind, at, summary, actor, ref string) {
		t, ok := parseAuvikTime(at)
		if !ok {
			return
		}
		if !since.IsZero() && t.Before(since) {
			return
		}
		report.Events = append(report.Events, changeEvent{
			At: t.Format(time.RFC3339), Kind: kind, Summary: summary,
			Actor: actor, RefID: ref, sortKey: t,
		})
		report.Counts[kind]++
	}

	// A record belongs to this device when it either carries a device
	// relationship pointing at it, or names it via a deviceId attribute.
	belongs := func(r auvikRow) bool {
		if r.rel("device") == deviceID || r.rel("entity") == deviceID {
			return true
		}
		return r.attrString(fNoteEntityID.Field) == deviceID
	}

	for _, c := range configs {
		if !belongs(c) {
			continue
		}
		summary := "configuration backup"
		if truthy(c.attr(fConfigIsRunning.Field)) {
			summary = "configuration backup (running config)"
		}
		add("config", c.attrString(fConfigBackupTime.Field), summary, "", c.ID)
	}
	for _, a := range audits {
		if !belongs(a) {
			continue
		}
		// auditAttributes: action/category/cause/data/dateStarted/direction/
		// lastActive/status/user. There is no free-text description.
		parts := make([]string, 0, 3)
		for _, v := range []string{a.attrString(fAuditCategory.Field), a.attrString(fAuditAction.Field)} {
			if v != "" && v != "unknown" {
				parts = append(parts, v)
			}
		}
		summary := strings.Join(parts, " ")
		if summary == "" {
			summary = "audit entry"
		}
		if st := a.attrString(fAuditStatus.Field); st != "" && st != "unknown" {
			summary += " (" + st + ")"
		}
		add("audit", firstNonEmpty(a.attrString(fAuditDateStarted.Field), a.attrString(fAuditLastActive.Field)),
			summary, a.attrString(fAuditUser.Field), a.ID)
	}
	for _, n := range notes {
		if !belongs(n) {
			continue
		}
		summary := firstNonEmpty(n.attrString(fNoteTitle.Field), n.attrString(fNoteBody.Field), "note")
		add("note", n.attrString(fNoteLastModified.Field), summary,
			n.attrString(fNoteLastModifiedBy.Field), n.ID)
	}
	for _, al := range alerts {
		if !belongs(al) {
			continue
		}
		summary := firstNonEmpty(al.attrString(fAlertName.Field), al.attrString(fAlertDescription.Field), "alert")
		if sev := al.attrString(fAlertSeverity.Field); sev != "" && sev != "unknown" {
			summary = sev + ": " + summary
		}
		if truthy(al.attr(fAlertDismissed.Field)) {
			summary += " [dismissed]"
		}
		add("alert", al.attrString(fAlertDetectedOn.Field), summary, "", al.ID)
	}

	sort.SliceStable(report.Events, func(i, j int) bool {
		return report.Events[i].sortKey.After(report.Events[j].sortKey)
	})
	return report
}
