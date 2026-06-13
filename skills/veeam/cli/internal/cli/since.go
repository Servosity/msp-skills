// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type fleetEvent struct {
	Time    string `json:"time"`
	Kind    string `json:"kind"`
	Company string `json:"company"`
	Name    string `json:"name"`
	Detail  string `json:"detail,omitempty"`
}

type sinceView struct {
	Window string       `json:"window"`
	Events []fleetEvent `json:"events"`
	Count  int          `json:"count"`
}

func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "since [window]",
		Short: "What changed across the fleet inside a time window: jobs that failed or warned, alarms that fired, agents that newly activated.",
		Long: strings.Trim(`
Show fleet activity inside a time window (default 24h): backup job runs that
ended in failure or warning, alarms that fired, and backup agents that newly
activated — each with its timestamp and tenant, newest first. Pass the window
as a positional duration like 12h, 3d, or 1w.

Reads only the local SQLite mirror — run `+"`veeam-cli sync`"+` first.`, "\n"),
		Example: strings.Trim(`
  veeam-cli since 24h
  veeam-cli since 3d
  veeam-cli since 12h --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			windowArg := ""
			if len(args) > 0 {
				windowArg = args[0]
			}
			window, err := veeamParseWindow(windowArg, 24*time.Hour)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid window %q: %w", windowArg, err))
			}
			ctx := cmd.Context()
			db, ok, err := veeamOpenStoreRead(dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			if ok {
				defer db.Close()
			}

			names := veeamCompanyNames(ctx, db)
			now := time.Now()
			cutoff := now.Add(-window)
			events := make([]fleetEvent, 0)

			inWindow := func(t time.Time, ok bool) bool {
				return ok && !t.Before(cutoff) && !t.After(now.Add(time.Minute))
			}

			jobs, _ := veeamLoad(ctx, db,
				"infrastructure-backup-servers-jobs",
				"infrastructure-backup-agents-jobs",
				"infrastructure-backup-agents-windows-jobs",
				"infrastructure-backup-agents-linux-jobs",
				"infrastructure-backup-agents-mac-jobs",
			)
			for _, j := range jobs {
				last, ok := vtime(j, "lastRun")
				if !inWindow(last, ok) {
					continue
				}
				health := veeamJobHealth(vstr(j, "status"))
				if health != "failed" && health != "warning" {
					continue
				}
				org := vstr(j, "organizationUid")
				if org == "" {
					org = vstr(j, "mappedOrganizationUid")
				}
				events = append(events, fleetEvent{
					Time:    last.UTC().Format(time.RFC3339),
					Kind:    "job-" + health,
					Company: veeamCompanyLabel(names, org),
					Name:    vstr(j, "name"),
					Detail:  firstNonEmpty(vstr(j, "failureMessage"), vstr(j, "status")),
				})
			}

			alarms, _ := veeamLoad(ctx, db, "alarms")
			for _, al := range alarms {
				t, ok := vtime(al, "lastActivation.time")
				if !inWindow(t, ok) {
					continue
				}
				events = append(events, fleetEvent{
					Time:    t.UTC().Format(time.RFC3339),
					Kind:    "alarm-" + strings.ToLower(firstNonEmpty(vstr(al, "lastActivation.status"), "fired")),
					Company: veeamCompanyLabel(names, vstr(al, "object.organizationUid")),
					Name:    firstNonEmpty(vstr(al, "name"), vstr(al, "alarmTemplateUid")),
					Detail:  vstr(al, "lastActivation.message"),
				})
			}

			agents, _ := veeamLoad(ctx, db,
				"infrastructure-backup-agents",
				"infrastructure-backup-agents-windows",
				"infrastructure-backup-agents-linux",
				"infrastructure-backup-agents-mac",
			)
			for _, a := range agents {
				t, ok := vtime(a, "activationTime")
				if !inWindow(t, ok) {
					continue
				}
				events = append(events, fleetEvent{
					Time:    t.UTC().Format(time.RFC3339),
					Kind:    "agent-activated",
					Company: veeamCompanyLabel(names, vstr(a, "organizationUid")),
					Name:    vstr(a, "name"),
					Detail:  vstr(a, "status"),
				})
			}

			sort.SliceStable(events, func(i, j int) bool {
				return events[i].Time > events[j].Time // RFC3339 sorts lexicographically by time
			})
			if limit > 0 && len(events) > limit {
				events = events[:limit]
			}

			view := sinceView{Window: window.String(), Events: events, Count: len(events)}
			table := make([]map[string]any, 0, len(events))
			for _, e := range events {
				table = append(table, map[string]any{
					"time":    e.Time,
					"kind":    e.Kind,
					"company": e.Company,
					"name":    e.Name,
				})
			}
			return veeamEmit(cmd, flags, view, table, fmt.Sprintf("No fleet events in the last %s. Run `veeam-cli sync` first, then re-check.", window))
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum events to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite mirror path (default: standard cache location)")
	return cmd
}
