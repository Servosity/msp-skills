// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/cliutil"
	"threatdown-pp-cli/internal/fleet"
)

type pendingRow struct {
	Site        string `json:"site"`
	AccountID   string `json:"account_id"`
	Command     string `json:"command"`
	MachineName string `json:"machine_name"`
	MachineID   string `json:"machine_id"`
	JobID       string `json:"job_id"`
	Status      string `json:"status"`
	Failed      bool   `json:"failed"`
	IssuedAt    string `json:"issued_at"`
	AgeHours    int    `json:"age_hours"`
	Online      *bool  `json:"endpoint_online"` // null when OneView did not report it
	Reissue     string `json:"reissue_command,omitempty"`
	ReissueNote string `json:"reissue_note,omitempty"`
}

type pendingGroup struct {
	Site    string `json:"site"`
	Command string `json:"command"`
	Waiting int    `json:"waiting"`
	Failed  int    `json:"failed"`
}

type respondPendingView struct {
	Source    string         `json:"source"`
	Note      string         `json:"note,omitempty"`
	Since     string         `json:"since"`
	Count     int            `json:"count"`
	Truncated bool           `json:"truncated"`
	Groups    []pendingGroup `json:"groups"`
	Jobs      []pendingRow   `json:"jobs"`
}

// commandVerb maps a job command back to the respond verb that re-issues it.
func commandVerb(command string) string {
	for _, v := range respondVerbs {
		if v.command == command {
			return v.name
		}
	}
	return ""
}

// reissueFlags turns a job's data back into respond flags. ok is false when a
// verb that carries settings (isolate, scan) has data that cannot be read, so
// no command with guessed settings is suggested.
func reissueFlags(verb string, raw json.RawMessage) (string, bool) {
	data := []byte(raw)
	var s string
	if json.Unmarshal(raw, &s) == nil {
		data = []byte(s)
	}
	switch verb {
	case "isolate":
		var d map[string]any
		if json.Unmarshal(data, &d) != nil {
			return "", false
		}
		var modes []string
		for _, m := range []string{"network", "process", "desktop"} {
			if d[m] == true {
				modes = append(modes, m)
			}
		}
		if len(modes) == 0 {
			return "", false
		}
		return " --mode " + strings.Join(modes, ","), true
	case "scan":
		var d struct {
			Scan struct {
				Type   string `json:"type"`
				Remove bool   `json:"remove"`
			} `json:"scan_settings"`
		}
		if json.Unmarshal(data, &d) != nil || (d.Scan.Type != "ThreatScan" && d.Scan.Type != "HyperScan") {
			return "", false
		}
		extra := " --scan-type " + d.Scan.Type
		if d.Scan.Remove {
			extra += " --remove"
		}
		return extra, true
	}
	// respond has no flags for other verbs' settings (reboot delay, message),
	// so only re-issue a job that had none.
	switch strings.TrimSpace(string(data)) {
	case "", "null", "{}":
		return "", true
	}
	return "", false
}

// selectPending keeps jobs that failed, or are still waiting after minAge.
func selectPending(jobs []jobRow, sites map[string]fleet.Site, now time.Time, since time.Time, minAge time.Duration, commandFilter string) []pendingRow {
	rows := make([]pendingRow, 0)
	for _, j := range jobs {
		issued, ok := fleet.ParseTime(j.IssuedAt)
		if ok && issued.Before(since) {
			continue
		}
		if commandFilter != "" && j.Command != commandFilter && commandVerb(j.Command) != commandFilter {
			continue
		}
		failed := fleet.JobFailed(j.Status)
		waiting := j.Status&fleet.JobStatusWaiting != 0
		age := 0
		if ok {
			age = int(now.Sub(issued).Hours())
		}
		if !failed && !(waiting && ok && now.Sub(issued) >= minAge) {
			continue
		}
		site := sites[j.AccountID].Name
		if site == "" {
			site = j.AccountName
		}
		r := pendingRow{Site: site, AccountID: j.AccountID, Command: j.Command, MachineName: j.MachineName, MachineID: j.MachineID,
			JobID: j.ID, Status: fleet.DecodeJobStatus(j.Status), Failed: failed, IssuedAt: j.IssuedAt, AgeHours: age, Online: j.Online}
		if verb := commandVerb(j.Command); verb != "" && j.MachineID != "" && j.AccountID != "" {
			// Target the job's own machine id (a host name can be reused by a
			// replacement machine) and carry its original settings over.
			if extra, ok := reissueFlags(verb, j.Data); ok {
				ref := fmt.Sprintf(`[{"endpoint_id":%q,"account_id":%q}]`, j.MachineID, j.AccountID)
				r.Reissue = fmt.Sprintf("echo %s | %s respond %s --stdin%s", fleet.ShellQuote(ref), tdBinary, verb, extra)
			} else {
				r.ReissueNote = "original " + verb + " settings are not readable from the job; re-issue by hand with the settings you want"
			}
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, k int) bool {
		if rows[i].Site != rows[k].Site {
			return rows[i].Site < rows[k].Site
		}
		if rows[i].Command != rows[k].Command {
			return rows[i].Command < rows[k].Command
		}
		return rows[i].AgeHours > rows[k].AgeHours
	})
	return rows
}

func groupPending(rows []pendingRow) []pendingGroup {
	idx := map[string]*pendingGroup{}
	out := make([]pendingGroup, 0)
	order := []string{}
	for _, r := range rows {
		k := r.Site + "\x00" + r.Command
		g := idx[k]
		if g == nil {
			g = &pendingGroup{Site: r.Site, Command: r.Command}
			idx[k] = g
			order = append(order, k)
		}
		if r.Failed {
			g.Failed++
		} else {
			g.Waiting++
		}
	}
	for _, k := range order {
		out = append(out, *idx[k])
	}
	return out
}

func newNovelRespondPendingCmd(flags *rootFlags) *cobra.Command {
	var since, minAge, commandFilter, siteFilter, dbPath string
	var maxPages int

	cmd := &cobra.Command{
		Use:   "pending",
		Short: "Jobs still waiting, or that timed out, expired or failed, grouped by site and command with a re-issue hint.",
		Long: `Use this command to find jobs, including RMM-issued ones, that have not completed or that timed out, expired or failed. Do NOT use this command to issue new jobs; use 'respond isolate' (or scan/restore/reboot/update) instead.

OneView reports job status as a bitmask (1 CREATED, 2 SENT, 4 RECEIVED,
8 STARTED, 16 TIMED_OUT, 32 COMPLETED, 64 EXPIRED, 128 FAILED, 256 CANCELLED).
This command asks for the failed and waiting states in one search, decodes
them, and keeps waiting jobs only once they are older than --min-age.`,
		Example: strings.Trim(`
  threatdown-cli respond pending --since 7d --agent
  threatdown-cli respond pending --command scan --json --select jobs.machine_name,jobs.status,jobs.reissue_command`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "respond pending")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--since must be a duration like 7d, got %q", since))
			}
			age, err := cliutil.ParseDurationLoose(minAge)
			if err != nil || age < 0 {
				return usageErr(fmt.Errorf("--min-age must be a duration like 1h, got %q", minAge))
			}
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			ctx := cmd.Context()
			now := time.Now().UTC()
			view := respondPendingView{Since: since, Jobs: make([]pendingRow, 0), Groups: make([]pendingGroup, 0)}

			mode := resolveDataSource(flags)
			var liveErr error
			if mode != "local" {
				var jobs []jobRow
				var sites []fleet.Site
				jobs, sites, view.Truncated, liveErr = pendingLive(ctx, flags, siteFilter, now.Add(-window), maxPages)
				if liveErr == nil {
					view.Source = "live"
					view.Jobs = selectPending(jobs, fleet.SiteIndex(sites), now, now.Add(-window), age, commandFilter)
					if siteFilter != "" {
						view.Note = accountlessNote(filterSitesByName(sites, siteFilter))
					}
				} else if mode == "live" || stopFallback(liveErr) {
					return classifyAPIErrorOnly(liveErr)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "live query failed (%v); falling back to the local store\n", liveErr)
				}
			}
			if mode == "local" || liveErr != nil {
				view.Source = "local"
				db, ok, err := openFleetStore(cmd, tdDBPath(dbPath))
				if err != nil {
					return err
				}
				if !ok && liveErr != nil {
					return classifyAPIErrorOnly(liveErr)
				}
				if !ok {
					view.Note = "no local mirror; run " + tdFleetSyncCommand
				}
				if ok {
					defer db.Close()
					if !hintIfUnsynced(cmd, db, tdResSites) {
						hintIfStale(cmd, db, tdResSites, 24*time.Hour)
					}
					fleetHint(cmd, db, tdResJobs, 24*time.Hour)
					if state, err := readSyncHintState(db, tdResJobs); err == nil && !state.hasState {
						view.Note = "no jobs synced yet; run " + tdFleetSyncCommand
					} else if n := coverageNote(ctx, db, tdResJobs, now.Add(-window)); n != "" {
						view.Note = n
					}
					sites, err := loadLocalSites(ctx, db)
					if err != nil {
						return err
					}
					raws, err := loadLocal(ctx, db, tdResJobs)
					if err != nil {
						return err
					}
					inScope, err := scopeSites(sites, siteFilter)
					if err != nil {
						return err
					}
					if siteFilter != "" {
						view.Note = joinNotes(view.Note, accountlessNote(inScope))
					}
					keep := map[string]bool{}
					for _, a := range fleet.AccountIDs(inScope, "") {
						keep[a] = true
					}
					jobs := parseJobs(raws)
					scoped := jobs[:0:0]
					for _, j := range jobs {
						if siteFilter == "" || keep[j.AccountID] {
							scoped = append(scoped, j)
						}
					}
					view.Jobs = selectPending(scoped, fleet.SiteIndex(sites), now, now.Add(-window), age, commandFilter)
				}
			}
			view.Groups = groupPending(view.Jobs)
			view.Count = len(view.Jobs)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "reissue_command", "reissue_note", "endpoint_online", "note")
			}
			if view.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "note: %s\n", view.Note)
			}
			if view.Count == 0 {
				if view.Note == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Every job in the last %s completed (or is still inside --min-age %s).\n", since, minAge)
				}
				return nil
			}
			if err := printColumns(cmd.OutOrStdout(), view.Groups, "site", "command", "waiting", "failed"); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return printColumns(cmd.OutOrStdout(), view.Jobs, "site", "machine_name", "command", "status", "age_hours", "endpoint_online", "reissue_command")
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Only jobs issued within this window (e.g. 7d, 24h)")
	cmd.Flags().StringVar(&minAge, "min-age", "1h", "Report a still-waiting job only once it is at least this old")
	cmd.Flags().StringVar(&commandFilter, "command", "", "Only this job command (a respond verb like scan, or the full command.* name)")
	cmd.Flags().StringVar(&siteFilter, "site", "", "Only sites whose name contains this text")
	cmd.Flags().IntVar(&maxPages, "max-scan-pages", tdDefaultMaxPages, "Maximum 2000-row job search pages to scan")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path for --data-source local")
	return cmd
}

func pendingLive(ctx context.Context, flags *rootFlags, siteFilter string, since time.Time, maxPages int) ([]jobRow, []fleet.Site, bool, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, nil, false, err
	}
	sites, err := fetchSites(ctx, c)
	if err != nil {
		return nil, nil, false, err
	}
	scoped, err := scopeSites(sites, siteFilter)
	if err != nil {
		return nil, nil, false, err
	}
	accounts := fleet.AccountIDs(scoped, "")
	if len(accounts) == 0 {
		return nil, sites, false, nil
	}
	body := map[string]any{"account_ids": accounts, "status": fleet.JobStatusNotRan, "issued_at_after": since.UTC().Format("2006-01-02T15:04:05.000Z"), "populate": map[string]any{"online": true}}
	raws, info, err := searchPages(ctx, c, "/v1/jobs/search", body, "jobs", maxPages)
	if err != nil {
		return nil, sites, false, err
	}
	return parseJobs(raws), sites, info.Truncated, nil
}
