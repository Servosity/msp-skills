// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): what's-down triage board — down/degraded monitors
// joined to their open incidents and whether anyone is actually being paged.

package cli

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
)

type downItem struct {
	MonitorID     string   `json:"monitor_id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Status        string   `json:"status"`
	OpenIncidents int      `json:"open_incidents"`
	IncidentIDs   []string `json:"incident_ids,omitempty"`
	Acknowledged  bool     `json:"any_incident_acknowledged"`
	HasPolicy     bool     `json:"has_escalation_policy"`
	HasChannel    bool     `json:"has_alert_channel"`
	PagesNobody   bool     `json:"pages_nobody"`
}

type downReport struct {
	Items         []downItem `json:"items"`
	DownCount     int        `json:"down_count"`
	UnackedCount  int        `json:"unacknowledged_count"`
	UnpagedCount  int        `json:"pages_nobody_count"`
	OnCallCovered bool       `json:"someone_on_call_now"`
}

// buildDownReport joins down/degraded monitors to their open incidents and
// paging posture. A monitor is on the board when its status counts as down OR
// it has at least one open incident (status can lag the incident stream).
func buildDownReport(monitors []monitorRow, incidents []incidentRow, onCalls []onCallRow) downReport {
	openBySource := openIncidentsBySource(incidents)
	covered := false
	for _, oc := range onCalls {
		if oc.OnCallUsers > 0 {
			covered = true
			break
		}
	}
	items := make([]downItem, 0)
	for _, m := range monitors {
		open := openBySource[m.ID]
		if m.Paused || (!monitorDown(m.Status) && len(open) == 0) {
			continue
		}
		name := m.Name
		if name == "" {
			name = m.URL
		}
		it := downItem{
			MonitorID:     m.ID,
			Name:          name,
			URL:           m.URL,
			Status:        m.Status,
			OpenIncidents: len(open),
			HasPolicy:     m.PolicyID != "",
			HasChannel:    m.hasAlertChannel(),
		}
		for _, in := range open {
			it.IncidentIDs = append(it.IncidentIDs, in.ID)
			if in.AcknowledgedAt != "" {
				it.Acknowledged = true
			}
		}
		it.PagesNobody = !it.HasPolicy && !it.HasChannel
		items = append(items, it)
	}
	// Worst first: pages-nobody, then unacknowledged, then by name.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].PagesNobody != items[j].PagesNobody {
			return items[i].PagesNobody
		}
		if items[i].Acknowledged != items[j].Acknowledged {
			return !items[i].Acknowledged
		}
		return items[i].Name < items[j].Name
	})
	rep := downReport{Items: items, OnCallCovered: covered}
	for _, it := range items {
		rep.DownCount++
		if !it.Acknowledged && it.OpenIncidents > 0 {
			rep.UnackedCount++
		}
		if it.PagesNobody {
			rep.UnpagedCount++
		}
	}
	return rep
}

// pp:data-source local
func newNovelDownCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "down",
		Short: "Focused triage list of monitors currently down or degraded, joined to their open incidents and whether anyone is actually paged for each.",
		Long: "Use this command for the focused list of monitors currently down and whether anyone is paged. " +
			"Do NOT use it for a full-account summary including paused/healthy counts; use 'fleet' instead. " +
			"Do NOT use it to age open incidents by acknowledgement state; use 'triage' instead. " +
			"Reads the local SQLite mirror; run `sync` first.",
		Example:     "  betterstack-cli down\n  betterstack-cli down --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			s, err := openAnalyticsStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer s.Close()
			maybeEmitSyncHints(cmd, s, "monitors", flags.maxAge)

			monitors, err := loadMonitors(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading monitors: %w", err)
			}
			incidents, err := loadIncidents(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading incidents: %w", err)
			}
			onCalls, err := loadOnCalls(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading on-call calendars: %w", err)
			}

			rep := buildDownReport(monitors, incidents, onCalls)
			if flags.asJSON {
				return flags.printJSON(cmd, rep)
			}
			if len(rep.Items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing is down: no monitors in a down state and no open incidents in the local mirror.")
				return nil
			}
			rows := make([][]string, 0, len(rep.Items))
			for _, it := range rep.Items {
				paged := "yes"
				if it.PagesNobody {
					paged = "NOBODY"
				}
				acked := "no"
				if it.Acknowledged {
					acked = "yes"
				}
				rows = append(rows, []string{it.MonitorID, truncateField(it.Name, 40), it.Status, strconv.Itoa(it.OpenIncidents), acked, paged})
			}
			if err := flags.printTable(cmd, []string{"ID", "MONITOR", "STATUS", "OPEN", "ACKED", "PAGES"}, rows); err != nil {
				return err
			}
			if !rep.OnCallCovered {
				fmt.Fprintln(cmd.OutOrStdout(), "\nwarning: no on-call calendar currently has anyone on call (see `oncall-gaps`).")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	return cmd
}
