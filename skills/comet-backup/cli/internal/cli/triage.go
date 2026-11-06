// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/client"
	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/comet"
)

type triageJob struct {
	Tenant         string   `json:"tenant"`
	TenantName     string   `json:"tenant_name,omitempty"`
	Device         string   `json:"device,omitempty"`
	DeviceID       string   `json:"device_id,omitempty"`
	Item           string   `json:"item,omitempty"`
	ItemID         string   `json:"item_id"`
	Vault          string   `json:"vault,omitempty"`
	VaultID        string   `json:"vault_id"`
	JobID          string   `json:"job_id"`
	Status         int64    `json:"status"`
	StatusName     string   `json:"status_name"`
	Classification string   `json:"classification"`
	StartedAt      string   `json:"started_at,omitempty"`
	EndedAt        string   `json:"ended_at,omitempty"`
	Online         bool     `json:"online"`
	ConnectionID   string   `json:"connection_id,omitempty"`
	ErrorLines     []string `json:"error_lines"`
	ErrorGroup     string   `json:"error_group,omitempty"`
	LogFetched     bool     `json:"log_fetched"`
}

type triageGroup struct {
	Error   string   `json:"error"`
	Count   int      `json:"count"`
	Tenants []string `json:"tenants"`
	JobIDs  []string `json:"job_ids"`
}

type fetchFailure struct {
	JobID string `json:"job_id"`
	Error string `json:"error"`
}

type triageView struct {
	WindowHours   float64        `json:"window_hours"`
	ScannedJobs   int            `json:"scanned_jobs"`
	Problems      []triageJob    `json:"problems"`
	Groups        []triageGroup  `json:"groups,omitempty"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
	Note          string         `json:"note,omitempty"`
}

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var flagWindow, flagStuckAfter, flagGroupBy string
	var flagIncludeWarnings bool
	var flagMaxLogs int

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Find out why backups failed: the failed and stuck jobs in a window, with the first error lines from each job log",
		Long: `Find out why backups failed: the failed and stuck jobs in a window, with the first error lines from each job log.

Use this command for failed, warning, and stuck jobs in a time window with the error lines from each job log. Do NOT use this command to find protected items that stopped running or never succeeded; use 'health' instead. Do NOT use it to retry the failures; use 'rerun-failed' instead.

Only the newest backup job per tenant, protected item and Storage Vault counts, so a later success hides an earlier failure. Log lines come from get-job-log-entries (severity E, or W for warning jobs); --max-logs caps how many job logs are fetched.`,
		Example: strings.Trim(`
  comet-backup-cli triage --window 24h --agent
  comet-backup-cli triage --window 48h --include-warnings --group-by error --json
  comet-backup-cli triage --window 7d --stuck-after 12h --max-logs 0`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: guardRunE(flags, func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "triage")
			}
			window, err := durationFlag("window", flagWindow)
			if err != nil {
				return err
			}
			stuck, err := durationFlag("stuck-after", flagStuckAfter)
			if err != nil {
				return err
			}
			if flagGroupBy != "" && flagGroupBy != "error" {
				return usageErr(fmt.Errorf("--group-by: only \"error\" is supported"))
			}
			if flagMaxLogs < 0 {
				return usageErr(fmt.Errorf("--max-logs must be 0 or more"))
			}
			maxLogs := flagMaxLogs
			if cliutil.IsDogfoodEnv() && maxLogs > 5 {
				maxLogs = 5
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			now := time.Now()
			jobs, err := fetchJobs(ctx, c, now, window)
			if err != nil {
				return fleetError(cmd, flags, c, err)
			}
			f, err := loadFleet(ctx, c)
			if err != nil {
				return fleetError(cmd, flags, c, err)
			}
			online := comet.OnlineIndex(f.live)
			problems := comet.ProblemJobs(jobs, comet.TriageOptions{Now: now, IncludeWarnings: flagIncludeWarnings, StuckAfter: stuck})
			view := triageView{WindowHours: window.Hours(), ScannedJobs: len(jobs), Problems: make([]triageJob, 0, len(problems)), FetchFailures: make([]fetchFailure, 0)}
			for i, j := range problems {
				p := f.profiles[j.Username]
				row := triageJob{Tenant: j.Username, TenantName: p.AccountName, DeviceID: j.DeviceID, ItemID: j.SourceGUID,
					VaultID: j.DestinationGUID, JobID: j.GUID, Status: j.Status, StatusName: comet.StatusName(j.Status),
					Classification: comet.ClassificationName(j.Classification), StartedAt: fmtTime(j.StartTime), EndedAt: fmtTime(j.EndTime),
					ErrorLines: make([]string, 0)}
				if d, ok := p.Devices[j.DeviceID]; ok {
					row.Device = d.FriendlyName
				}
				if s, ok := p.Sources[j.SourceGUID]; ok {
					row.Item = s.Description
				}
				if v, ok := p.Destinations[j.DestinationGUID]; ok {
					row.Vault = v.Description
				}
				if cid, ok := online[j.Username+"\x00"+j.DeviceID]; ok {
					row.Online, row.ConnectionID = true, cid
				}
				if i < maxLogs && j.GUID != "" {
					lines, ferr := jobErrorLines(ctx, c, j)
					if ferr != nil {
						view.FetchFailures = append(view.FetchFailures, fetchFailure{JobID: j.GUID, Error: safeText(c, ferr.Error())})
					} else {
						row.ErrorLines, row.LogFetched = lines, true
					}
				}
				if len(row.ErrorLines) > 0 {
					row.ErrorGroup = comet.NormalizeError(row.ErrorLines[0])
				} else {
					row.ErrorGroup = row.StatusName
				}
				view.Problems = append(view.Problems, row)
			}
			if n := len(view.FetchFailures); n > 0 {
				fmt.Fprintf(os.Stderr, "warning: %d of %d job log fetches failed; those jobs have no error_lines\n", n, min(maxLogs, len(problems)))
			}
			if len(problems) > maxLogs {
				view.Note = fmt.Sprintf("error lines fetched for the first %d of %d problem jobs; raise --max-logs to fetch more", maxLogs, len(problems))
			}
			if flagGroupBy == "error" {
				view.Groups = groupByError(view.Problems)
			}
			// Partial results still print; a failed log fetch then exits non-zero
			// unless --allow-partial-failure downgrades it to the stderr warning.
			partialErr := func() error {
				if len(view.FetchFailures) == 0 || flags.allowPartialFailure {
					return nil
				}
				return &cliError{code: 6, err: fmt.Errorf("%d job log fetches failed (see fetch_failures); pass --allow-partial-failure to exit 0", len(view.FetchFailures))}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, triageAgentKeep...); err != nil {
					return err
				}
				return partialErr()
			}
			if len(view.Problems) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No failed, warning or stuck backup jobs in the last %s (%d jobs scanned).\n", flagWindow, len(jobs))
				return nil
			}
			for _, ff := range view.FetchFailures {
				fmt.Fprintf(cmd.ErrOrStderr(), "log fetch failed for job %s: %s\n", cleanCell(ff.JobID), ff.Error)
			}
			if flagGroupBy == "error" {
				grows := make([][]string, 0, len(view.Groups))
				for _, g := range view.Groups {
					grows = append(grows, []string{fmt.Sprint(g.Count), strings.Join(g.Tenants, ", "), g.Error})
				}
				if err := printCleanTable(cmd, flags, []string{"COUNT", "TENANTS", "ERROR"}, grows); err != nil {
					return err
				}
				return partialErr()
			}
			rows2 := make([][]string, 0, len(view.Problems))
			for _, r := range view.Problems {
				first := ""
				if len(r.ErrorLines) > 0 {
					first = r.ErrorLines[0]
				}
				if first == "" && !r.LogFetched && r.JobID != "" {
					first = "(log not fetched)"
				}
				rows2 = append(rows2, []string{r.Tenant, r.Device, r.Item, r.StatusName, r.EndedAt, first})
			}
			if err := printCleanTable(cmd, flags, []string{"TENANT", "DEVICE", "ITEM", "STATUS", "ENDED", "ERROR"}, rows2); err != nil {
				return err
			}
			return partialErr()
		}),
	}
	cmd.Flags().StringVar(&flagWindow, "window", "24h", "How far back to look for jobs (e.g. 24h, 48h, 7d)")
	cmd.Flags().BoolVar(&flagIncludeWarnings, "include-warnings", false, "Also list jobs that completed with warnings (status 7001)")
	cmd.Flags().StringVar(&flagStuckAfter, "stuck-after", "12h", "Report running jobs started longer ago than this as stuck (0 disables)")
	cmd.Flags().IntVar(&flagMaxLogs, "max-logs", 50, "Maximum job logs to fetch for error lines (0 skips log fetching)")
	cmd.Flags().StringVar(&flagGroupBy, "group-by", "", "Group problem jobs: \"error\" buckets them by normalized first error line")
	return cmd
}

// jobErrorLines returns up to three error messages from a job's log (warning
// messages for a warning job). Severity is filtered here too because servers
// older than 23.3.1 ignore MinSeverity.
func jobErrorLines(ctx context.Context, c *client.Client, j comet.Job) ([]string, error) {
	sev := "E"
	if comet.Family(j.Status) == "warning" {
		sev = "W"
	}
	f := url.Values{}
	f.Set("JobID", j.GUID)
	f.Set("MinSeverity", sev)
	raw, err := cometRead(ctx, c, pathJobLogEntries, f)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		Severity string
		Message  string
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decoding job log: %w", err)
	}
	lines := make([]string, 0, 3)
	for _, e := range entries {
		if e.Severity == "E" || (sev == "W" && e.Severity == "W") {
			lines = append(lines, safeText(c, strings.TrimSpace(e.Message)))
			if len(lines) == 3 {
				break
			}
		}
	}
	return lines, nil
}

func groupByError(rows []triageJob) []triageGroup {
	byKey := map[string]*triageGroup{}
	tenants := map[string]map[string]bool{}
	for _, r := range rows {
		g, ok := byKey[r.ErrorGroup]
		if !ok {
			g = &triageGroup{Error: r.ErrorGroup, Tenants: make([]string, 0), JobIDs: make([]string, 0)}
			byKey[r.ErrorGroup] = g
			tenants[r.ErrorGroup] = map[string]bool{}
		}
		g.Count++
		g.JobIDs = append(g.JobIDs, r.JobID)
		if !tenants[r.ErrorGroup][r.Tenant] {
			tenants[r.ErrorGroup][r.Tenant] = true
			g.Tenants = append(g.Tenants, r.Tenant)
		}
	}
	out := make([]triageGroup, 0, len(byKey))
	for _, g := range byKey {
		sort.Strings(g.Tenants)
		out = append(out, *g)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Count != out[b].Count {
			return out[a].Count > out[b].Count
		}
		return out[a].Error < out[b].Error
	})
	return out
}
