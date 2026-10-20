// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
// The --live flag additionally calls v2 GET /backups/{backupId}/results per
// day; RunE rejects --live with --data-source local and sets the --agent
// meta.source to "live" when it runs.

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"backup-radar-pp-cli/internal/cliutil"
	"backup-radar-pp-cli/internal/radar"
)

type timelineFailure struct {
	BackupID int64  `json:"backup_id"`
	Date     string `json:"date"`
	Error    string `json:"error"`
}

type timelineView struct {
	Device        string              `json:"device"`
	Job           string              `json:"job,omitempty"`
	Days          int                 `json:"days"`
	Matched       []string            `json:"matched_devices"`
	Jobs          []radar.TimelineJob `json:"jobs"`
	History       []radar.TimelineDay `json:"history"`
	FetchFailures []timelineFailure   `json:"fetch_failures,omitempty"`
	Sources       radarSources        `json:"sources"`
	Note          string              `json:"note,omitempty"`
}

func newNovelTimelineCmd(flags *rootFlags) *cobra.Command {
	var job, client, dbPath string
	var days int
	var live bool

	cmd := &cobra.Command{
		Use:   "timeline [device]",
		Short: "Show one device's day-by-day backup history from the local store, merging v3 SLA days with v2 result counts",
		Long: `Merge v3 per-day SLA status with v2 per-day result counts for one device
(matched by Backup Radar or ScalePad device name; exact first, then substring),
plus each job's open ticket count, tags and note. Every row names its client
and device; when one name matches several machines the command warns and
--client narrows it. --live adds the individual v2 results for each day (one
API call per v2 job per day; needs BACKUP_RADAR_V2_API_KEY).

Use this command to explain one device's backup history across both API generations.
Do NOT use this command for a fleet-wide list of today's problems; use 'check' instead.`,
		Example: `  backup-radar-cli timeline "DC01" --days 14
  backup-radar-cli timeline "DC01" --job "Nightly Image" --days 30 --agent
  backup-radar-cli timeline "DC01" --client "Acme Dental" --days 3 --live --json`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "mcp:write-flags": "csv,plain,quiet"}, // HAND-FIX(radar-machine-modes): refused modes leave the MCP schema
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseTabularModes(flags, "timeline"); err != nil {
				return err
			}
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "timeline")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("timeline needs exactly one device name; quote names with spaces"))
			}
			if days < 1 || days > 365 {
				return usageErr(fmt.Errorf("--days must be between 1 and 365"))
			}
			if live && flags.dataSource == "local" {
				return usageErr(fmt.Errorf("--live calls the API; drop --data-source local"))
			}
			if live && strings.TrimSpace(cliutil.EnvOverride("BACKUP_RADAR_V2_API_KEY")) == "" {
				return authErr(fmt.Errorf("--live reads v2 results and needs BACKUP_RADAR_V2_API_KEY; set it in the environment or drop --live"))
			}
			snap, ok, err := loadRadarSnapshot(cmd, flags, dbPath)
			if err != nil {
				return err
			}
			view := timelineView{Device: args[0], Job: job, Days: days, Matched: make([]string, 0), Jobs: make([]radar.TimelineJob, 0), History: make([]radar.TimelineDay, 0), Sources: sourcesOf(snap, "devices", "backups")}
			now := time.Now().UTC()
			if ok {
				view.Jobs, view.History = radar.Timeline(snap, args[0], job, client, days, now)
				view.Matched = radar.DistinctDevices(view.Jobs)
			}
			switch {
			case !snap.AnySynced():
				// sources.note already says "nothing synced yet".
			case len(view.Jobs) == 0 && job != "":
				if devJobs, _ := radar.Timeline(snap, args[0], "", client, days, now); len(devJobs) > 0 {
					view.Note = fmt.Sprintf("device %q has no job named %q; its jobs: %s", args[0], job, strings.Join(timelineJobNames(devJobs), "; "))
					break
				}
				view.Note = fmt.Sprintf("no synced backup matches device %q; look names up locally with: backup-radar-cli search %s", args[0], shellQuote(args[0]))
			case len(view.Jobs) == 0:
				view.Note = fmt.Sprintf("no synced backup matches device %q; look names up locally with: backup-radar-cli search %s", args[0], shellQuote(args[0]))
			case len(view.Matched) > 1:
				view.Note = fmt.Sprintf("%q matched %d machines (%s); add --client or use the exact name", args[0], len(view.Matched), strings.Join(view.Matched, "; "))
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+scrubRadarText(view.Note))
			}
			if live && len(view.Jobs) > 0 {
				flags.agentSource = "live" // output now includes API data
				failures, err := attachLiveResults(cmd, flags, &view)
				if err != nil {
					return err
				}
				view.FetchFailures = failures
				if len(failures) > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d live results fetch(es) failed; those days show no results\n", len(failures))
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, append(radarKeep(radar.TimelineDay{}), radarKeep(radar.TimelineJob{})...)...)
			}
			out := cmd.OutOrStdout()
			if len(view.Jobs) == 0 {
				if view.Note != "" {
					fmt.Fprintln(out, scrubRadarText(view.Note))
				}
				printSourcesNote(out, view.Sources.Note) // HAND-FIX(radar-human-notes)
				return nil
			}
			for _, j := range view.Jobs {
				tickets := "-"
				if j.TicketCount != nil {
					tickets = strconv.Itoa(*j.TicketCount)
				}
				fmt.Fprintf(out, "%s / %s / %s  status=%s tickets=%s last_success=%s tags=%s\n", scrubRadarText(j.Client), scrubRadarText(j.Device), scrubRadarText(j.Job), scrubRadarText(j.V2Status), tickets, scrubRadarText(j.LastSuccess), scrubRadarText(strings.Join(j.Tags, ",")))
				if j.Note != "" {
					fmt.Fprintf(out, "    note: %s\n", scrubRadarText(j.Note))
				}
			}
			headers := []string{"date", "client", "device", "job", "sla_status", "v2_status", "failure", "no_result"}
			if live {
				headers = append(headers, "live_results")
			}
			failed := map[string]bool{}
			for _, f := range view.FetchFailures {
				failed[fmt.Sprintf("%d/%s", f.BackupID, f.Date)] = true
			}
			table := make([][]string, 0, len(view.History))
			for _, d := range view.History {
				row := []string{d.Date, d.Client, d.Device, d.Job, d.SLAStatus, d.V2Status, intStr(d.Failure), intStr(d.NoResult)}
				if live {
					cell := liveSummary(d.Results)
					if failed[fmt.Sprintf("%d/%s", d.BackupID, d.Date)] {
						cell = "fetch failed"
					}
					row = append(row, cell)
				}
				table = append(table, row)
			}
			if err := printRadarTable(out, headers, table); err != nil {
				return err
			}
			printSourcesNote(cmd.ErrOrStderr(), view.Sources.Note)
			if live {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: --json shows each live result (time, success, warning, failure, manual)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&job, "job", "", "Only this backup job (exact name, case-insensitive)")
	cmd.Flags().StringVar(&client, "client", "", "Only devices of clients whose ScalePad or Backup Radar name contains this text")
	cmd.Flags().IntVar(&days, "days", 30, "Calendar days to show, ending on the newest synced day (1-365; each sync fetches 90 days of v3 and 31 of v2, and stored days accumulate up to a year)")
	cmd.Flags().BoolVar(&live, "live", false, "Also fetch each day's individual v2 results from the API (v2 key; one call per job per day; use the default --data-source auto)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the synced local store)")
	return cmd
}

func intStr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// attachLiveResults fills TimelineDay.Results from v2 GET
// /backups/{backupId}/results?date=. Failures travel with the day so a
// missing call never reads as "no results that day".
func attachLiveResults(cmd *cobra.Command, flags *rootFlags, view *timelineView) ([]timelineFailure, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	v2 := c.WithTier("v2")
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()
	failures := make([]timelineFailure, 0)
	calls := 0
	for i := range view.History {
		day := &view.History[i]
		if day.BackupID == 0 {
			continue
		}
		if cliutil.IsDogfoodEnv() && calls >= 1 {
			break // stay inside the live-dogfood per-command budget
		}
		calls++
		data, err := v2.Get(ctx, fmt.Sprintf("/backups/%d/results", day.BackupID), map[string]string{"date": day.Date})
		if err != nil {
			failures = append(failures, timelineFailure{BackupID: day.BackupID, Date: day.Date, Error: err.Error()})
			continue
		}
		var page struct {
			Results json.RawMessage `json:"Results"`
		}
		if json.Unmarshal(data, &page) == nil && len(page.Results) > 0 {
			day.Results = page.Results
		} else {
			day.Results = data
		}
	}
	return failures, nil
}

// liveSummary condenses one day's live v2 results for the terminal table.
func liveSummary(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var items []struct {
		Success, Warning, Failure bool
	}
	if json.Unmarshal(raw, &items) != nil {
		return "?"
	}
	s, w, f := 0, 0, 0
	for _, it := range items {
		switch {
		case it.Failure:
			f++
		case it.Warning:
			w++
		case it.Success:
			s++
		}
	}
	return fmt.Sprintf("%d ok / %d warn / %d fail", s, w, f)
}

// timelineJobNames lists a device's distinct job names in first-seen order.
func timelineJobNames(jobs []radar.TimelineJob) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(jobs))
	for _, j := range jobs {
		if j.Job != "" && !seen[j.Job] {
			seen[j.Job] = true
			names = append(names, j.Job)
		}
	}
	return names
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
