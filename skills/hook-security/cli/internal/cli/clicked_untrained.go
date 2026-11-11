// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hook-security-pp-cli/internal/cliutil"
)

type hookClickResult struct {
	CampaignName     string      `json:"campaign_name"`
	OrganizationID   string      `json:"organization_id"`
	OrganizationName string      `json:"organization_name"`
	Person           *hookPerson `json:"person"`
	ClickedAt        *string     `json:"clicked_at"`
}

type clickedCourse struct {
	Course  string  `json:"course"`
	Status  string  `json:"status"`
	Overdue bool    `json:"overdue"`
	DueAt   *string `json:"due_at"`
}

type clickedUntrainedRow struct {
	PersonID         string          `json:"person_id"`
	Name             string          `json:"name"`
	Email            string          `json:"email"`
	OrganizationID   string          `json:"organization_id"`
	OrganizationName string          `json:"organization_name"`
	Clicks           int             `json:"clicks"`
	LastClickedAt    string          `json:"last_clicked_at"`
	Campaigns        []string        `json:"campaigns"`
	TrainingState    string          `json:"training_state"`
	Courses          []clickedCourse `json:"courses"`
}

type clickedUntrainedView struct {
	People             []clickedUntrainedRow `json:"people"`
	TotalUntrained     int                   `json:"total_untrained"`
	FinishedCount      int                   `json:"finished_count"`
	RemovedPeople      int                   `json:"removed_people"`
	ScannedResults     int                   `json:"scanned_results"`
	ScannedEnrollments int                   `json:"scanned_enrollments"`
	MaxScanPages       int                   `json:"max_scan_pages"`
	Complete           bool                  `json:"complete"`
	Note               string                `json:"note,omitempty"`
}

func hookEmailKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func hookParseSince(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, hookDateLayout} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, usageErr(fmt.Errorf("--since must be YYYY-MM-DD or an RFC 3339 time, got %q", value))
}

// clickedBefore reports whether a click is older than the --since cutoff. A
// timestamp that cannot be parsed is kept, so a format surprise never hides a
// person who clicked.
func clickedBefore(clickedAt *string, since time.Time) bool {
	if since.IsZero() || clickedAt == nil {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, *clickedAt)
	return err == nil && parsed.Before(since)
}

// joinClickedUntrained groups clicks per person and matches enrollments by
// person id or email. Legacy learners have a null person id, so email matters.
//
// A person is finished only when every enrollment that was not cancelled is
// completed. When the enrollment scan stopped early, a person with no open
// enrollment in view could still have one we did not read, so they report as
// unknown rather than finished.
func joinClickedUntrained(results []hookClickResult, enrollments []hookEnrollment, since time.Time, enrollmentsTruncated bool) (rows []clickedUntrainedRow, finished, removed int) {
	byKey := map[string][]int{}
	for i, enrollment := range enrollments {
		if enrollment.Person == nil {
			continue
		}
		if enrollment.Person.ID != "" {
			byKey["id:"+enrollment.Person.ID] = append(byKey["id:"+enrollment.Person.ID], i)
		}
		if email := hookEmailKey(enrollment.Person.Email); email != "" {
			byKey["email:"+email] = append(byKey["email:"+email], i)
		}
	}

	people := map[string]*clickedUntrainedRow{}
	campaigns := map[string]map[string]bool{}
	for _, result := range results {
		if clickedBefore(result.ClickedAt, since) {
			continue
		}
		if result.Person == nil {
			removed++
			continue
		}
		key := "email:" + hookEmailKey(result.Person.Email)
		if result.Person.ID != "" {
			key = "id:" + result.Person.ID
		}
		row := people[key]
		if row == nil {
			row = &clickedUntrainedRow{PersonID: result.Person.ID, Name: result.Person.Name, Email: result.Person.Email, OrganizationID: result.OrganizationID, OrganizationName: result.OrganizationName, Campaigns: make([]string, 0), Courses: make([]clickedCourse, 0)}
			people[key] = row
			campaigns[key] = map[string]bool{}
		}
		row.Clicks++
		if at := hookString(result.ClickedAt); at > row.LastClickedAt {
			row.LastClickedAt = at
		}
		if !campaigns[key][result.CampaignName] {
			campaigns[key][result.CampaignName] = true
			row.Campaigns = append(row.Campaigns, result.CampaignName)
		}
	}

	rows = make([]clickedUntrainedRow, 0, len(people))
	for _, row := range people {
		matched := map[int]bool{}
		if row.PersonID != "" {
			for _, i := range byKey["id:"+row.PersonID] {
				matched[i] = true
			}
		}
		for _, i := range byKey["email:"+hookEmailKey(row.Email)] {
			matched[i] = true
		}
		anyFinished := false
		for i := range matched {
			enrollment := enrollments[i]
			switch enrollment.Status {
			case "cancelled":
			case "completed":
				anyFinished = true
			default:
				row.Courses = append(row.Courses, clickedCourse{Course: enrollment.Course.Name, Status: enrollment.Status, Overdue: enrollment.Overdue, DueAt: enrollment.DueAt})
			}
		}
		switch {
		case len(row.Courses) > 0:
			row.TrainingState = "open"
			sort.Slice(row.Courses, func(a, b int) bool { return row.Courses[a].Course < row.Courses[b].Course })
		case enrollmentsTruncated:
			row.TrainingState = "unknown"
		case anyFinished:
			finished++
			continue
		default:
			row.TrainingState = "none_assigned"
		}
		sort.Strings(row.Campaigns)
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LastClickedAt != rows[j].LastClickedAt {
			return rows[i].LastClickedAt > rows[j].LastClickedAt
		}
		return hookEmailKey(rows[i].Email) < hookEmailKey(rows[j].Email)
	})
	return rows, finished, removed
}

func newNovelClickedUntrainedCmd(flags *rootFlags) *cobra.Command {
	var campaign, org, since string
	var limit, maxScanPages int

	cmd := &cobra.Command{
		Use:   "clicked-untrained",
		Short: "List people who clicked a simulation and have not finished training, with course status and due date.",
		Long: "Reads the clicked results for one campaign or one organization and joins them to that organization's\n" +
			"enrollments. A person counts as finished only when every enrollment that was not cancelled is completed;\n" +
			"completion dates are not compared with click dates, and courses are not matched to campaigns.\n" +
			"training_state is open, none_assigned, or unknown when a scan stopped at --max-scan-pages.",
		Example: strings.Trim(`
  hook-security-cli clicked-untrained --campaign cmp_88213 --agent
  hook-security-cli clicked-untrained --org org_4521 --since 2026-07-01`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "--campaign=cmp_88213"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "clicked-untrained")
			}
			if (campaign == "") == (org == "") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass exactly one of --campaign or --org"))
			}
			cutoff, err := hookParseSince(since)
			if err != nil {
				return err
			}
			if limit < 0 || maxScanPages < 1 {
				return usageErr(fmt.Errorf("--limit must be 0 or more and --max-scan-pages must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := hookCtx(cmd, flags)
			defer cancel()

			path := replacePathParam("/v1/organizations/{id}/results", "id", org)
			if campaign != "" {
				path = replacePathParam("/v1/campaigns/{id}/results", "id", campaign)
			}
			results, resultsCut, err := hookScan[hookClickResult](ctx, c, path, map[string]string{"outcome": "clicked"}, 500, maxScanPages)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}

			orgs := map[string]bool{}
			for _, result := range results {
				orgs[result.OrganizationID] = true
			}
			enrollments := make([]hookEnrollment, 0)
			enrollmentsCut := false
			for id := range orgs {
				rows, cut, err := hookScan[hookEnrollment](ctx, c, "/v1/enrollments", map[string]string{"organization_id": id}, 500, maxScanPages)
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				enrollments = append(enrollments, rows...)
				enrollmentsCut = enrollmentsCut || cut
			}

			people, finished, removed := joinClickedUntrained(results, enrollments, cutoff, enrollmentsCut)
			view := clickedUntrainedView{People: people, TotalUntrained: len(people), FinishedCount: finished, RemovedPeople: removed, ScannedResults: len(results), ScannedEnrollments: len(enrollments), MaxScanPages: maxScanPages, Complete: !resultsCut && !enrollmentsCut}
			switch {
			case !view.Complete:
				view.Note = fmt.Sprintf("stopped at --max-scan-pages %d before reading every page; this list is partial, raise it for a full answer", maxScanPages)
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+view.Note)
			case len(results) == 0:
				view.Note = "no clicked results for this scope"
			}
			if limit > 0 && limit < len(view.People) {
				view.People = view.People[:limit]
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				tw := newTabWriter(cmd.OutOrStdout())
				fmt.Fprintln(tw, "NAME\tEMAIL\tCLICKS\tTRAINING\tCOURSE\tSTATUS\tDUE")
				for _, row := range view.People {
					course, status, due := "", "", ""
					if len(row.Courses) > 0 {
						course, status, due = row.Courses[0].Course, row.Courses[0].Status, hookString(row.Courses[0].DueAt)
					}
					fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n", row.Name, row.Email, row.Clicks, row.TrainingState, course, status, due)
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d not finished, %d finished, %d removed people\n", view.TotalUntrained, view.FinishedCount, view.RemovedPeople)
				if view.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				}
				return nil
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "person_id", "name", "email", "organization_id", "organization_name", "clicks", "last_clicked_at", "campaigns", "training_state", "courses")
		},
	}
	cmd.Flags().StringVar(&campaign, "campaign", "", "Campaign to check, for example cmp_88213 (use this or --org)")
	cmd.Flags().StringVar(&org, "org", "", "Organization to check across all its campaigns, for example org_4521 (use this or --campaign)")
	cmd.Flags().StringVar(&since, "since", "", "Only count clicks at or after this date, YYYY-MM-DD or RFC 3339")
	cmd.Flags().IntVar(&limit, "limit", 0, "Show at most this many people; totals still cover everyone scanned (0 shows all)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 20, "Maximum pages of results and of enrollments to read before reporting a partial answer")
	return cmd
}
