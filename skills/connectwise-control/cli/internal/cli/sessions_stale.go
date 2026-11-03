// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type staleSession struct {
	SessionID        string `json:"session_id"`
	Name             string `json:"name"`
	GuestMachineName string `json:"guest_machine_name,omitempty"`
	LastConnected    string `json:"last_connected,omitempty"`
	DaysSince        int    `json:"days_since,omitempty"`
	Raw              string `json:"raw_last_connected,omitempty"`
}

type staleReport struct {
	SessionType          string         `json:"session_type"`
	Group                string         `json:"group"`
	Days                 int            `json:"days"`
	Cutoff               string         `json:"cutoff"`
	TotalSessions        int            `json:"total_sessions"`
	StaleCount           int            `json:"stale_count"`
	Stale                []staleSession `json:"stale"`
	NoConnectionRecorded []staleSession `json:"no_connection_recorded"`
	UnreadableTimestamp  []staleSession `json:"unreadable_timestamp"`
}

// newNovelSessionsStaleCmd lists sessions whose guest has not connected for
// at least --days: the dark access agents an MSP pays for and nobody uses.
// pp:data-source live
func newNovelSessionsStaleCmd(flags *rootFlags) *cobra.Command {
	var sessionType, group string
	var days int

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "List sessions whose guest has not connected for at least --days (default 30), oldest first",
		Long: `Read the live session list and report every session whose guest last connected
more than --days ago, oldest first. Sessions with no connection on record and
sessions whose timestamp cannot be read are listed separately, never dropped.`,
		Example: strings.Trim(`
  connectwise-control-cli sessions stale
  connectwise-control-cli sessions stale --session-type Access --days 60 --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if days < 1 {
				return usageErr(fmt.Errorf("--days must be at least 1, got %d", days))
			}
			typeNum, ok := cwcSessionTypes[strings.ToLower(sessionType)]
			if !ok {
				return usageErr(fmt.Errorf("--session-type must be Access, Support or Meeting, got %q", sessionType))
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "sessions stale")
			}
			data, err := cwcPost(cmd, flags, "/Services/PageService.ashx/GetLiveData", cwcLiveDataBody(typeNum, group))
			if err != nil {
				return err
			}
			sessions, err := cwcSessionList(data)
			if err != nil {
				return apiErr(err)
			}
			rep := buildStaleReport(sessions, sessionType, group, days, cwcNow().UTC())
			rows := make([]map[string]any, 0, len(rep.Stale))
			for _, s := range rep.Stale {
				rows = append(rows, map[string]any{"name": s.Name, "machine": s.GuestMachineName, "last_connected": s.LastConnected, "days": s.DaysSince})
			}
			summary := fmt.Sprintf("%d of %d %s sessions in %q have not connected in %d+ days (%d never connected, %d unreadable).",
				rep.StaleCount, rep.TotalSessions, sessionType, group, days, len(rep.NoConnectionRecorded), len(rep.UnreadableTimestamp))
			return cwcEmit(cmd, flags, rep, rows, summary)
		},
	}
	cmd.Flags().StringVar(&sessionType, "session-type", "Access", "Session type to check: Access, Support or Meeting")
	cmd.Flags().StringVar(&group, "group", "All Machines", "Session group to check")
	cmd.Flags().IntVar(&days, "days", 30, "Report sessions whose guest last connected at least this many days ago")
	return cmd
}

func buildStaleReport(sessions []map[string]any, sessionType, group string, days int, now time.Time) staleReport {
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	rep := staleReport{
		SessionType:          sessionType,
		Group:                group,
		Days:                 days,
		Cutoff:               cutoff.Format(time.RFC3339),
		TotalSessions:        len(sessions),
		Stale:                []staleSession{},
		NoConnectionRecorded: []staleSession{},
		UnreadableTimestamp:  []staleSession{},
	}
	type dated struct {
		s staleSession
		t time.Time
	}
	var stale []dated
	for _, m := range sessions {
		s := staleSession{
			SessionID:        cwcString(m, "sessionID"),
			Name:             cwcString(m, "name"),
			GuestMachineName: cwcString(m, "guestMachineName"),
		}
		raw := cwcString(m, "lastConnectedEventTime")
		t, ok, err := cwcTime(raw)
		switch {
		case err != nil:
			s.Raw = raw
			rep.UnreadableTimestamp = append(rep.UnreadableTimestamp, s)
		case !ok:
			rep.NoConnectionRecorded = append(rep.NoConnectionRecorded, s)
		case !t.After(cutoff):
			s.LastConnected = t.UTC().Format(time.RFC3339)
			s.DaysSince = int(math.Floor(now.Sub(t).Hours() / 24))
			stale = append(stale, dated{s, t})
		}
	}
	sort.SliceStable(stale, func(i, j int) bool { return stale[i].t.Before(stale[j].t) })
	for _, d := range stale {
		rep.Stale = append(rep.Stale, d.s)
	}
	rep.StaleCount = len(rep.Stale)
	return rep
}
