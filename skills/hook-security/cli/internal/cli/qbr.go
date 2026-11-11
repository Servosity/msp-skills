// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hook-security-pp-cli/internal/cliutil"
)

type qbrOrganization struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Type            string          `json:"type"`
	State           string          `json:"state"`
	ActiveUserCount int             `json:"active_user_count"`
	ReportButton    json.RawMessage `json:"report_button"`
}

type qbrSummary struct {
	Simulations      *hookSimulations  `json:"simulations"`
	Training         *hookTraining     `json:"training"`
	SimulationTrend  []json.RawMessage `json:"simulation_trend"`
	TrainingTrend    []json.RawMessage `json:"training_trend"`
	WithheldOutcomes []string          `json:"withheld_outcomes"`
}

type qbrOverdue struct {
	Total     int             `json:"total"`
	Truncated bool            `json:"truncated"`
	People    []overduePerson `json:"people"`
}

type qbrView struct {
	Organization     qbrOrganization   `json:"organization"`
	StartDate        string            `json:"start_date"`
	EndDate          string            `json:"end_date"`
	Simulations      *hookSimulations  `json:"simulations"`
	ReportRate       *float64          `json:"report_rate"`
	Training         *hookTraining     `json:"training"`
	SimulationTrend  []json.RawMessage `json:"simulation_trend"`
	TrainingTrend    []json.RawMessage `json:"training_trend"`
	WithheldOutcomes []string          `json:"withheld_outcomes"`
	RecentCampaigns  []json.RawMessage `json:"recent_campaigns"`
	MoreCampaigns    bool              `json:"more_campaigns"`
	OverdueTraining  qbrOverdue        `json:"overdue_training"`
	Notes            []string          `json:"notes"`
}

// qbrReportRate is reported over sent. It stays nil when reported is withheld:
// a client with reporting off has an unknown report rate, not a zero one.
func qbrReportRate(sim *hookSimulations) *float64 {
	if sim == nil || sim.Reported == nil || sim.Sent == 0 {
		return nil
	}
	rate := float64(*sim.Reported) / float64(sim.Sent)
	return &rate
}

func qbrWindow(now time.Time, days int, start, end string) (string, string, error) {
	if days < 1 {
		return "", "", usageErr(fmt.Errorf("--days must be at least 1"))
	}
	if err := hookCheckDate("start", start); err != nil {
		return "", "", err
	}
	if err := hookCheckDate("end", end); err != nil {
		return "", "", err
	}
	if end == "" {
		end = now.UTC().Format(hookDateLayout)
	}
	endDay, _ := time.Parse(hookDateLayout, end)
	if start == "" {
		// --days counts the end date itself, so N days reach back N-1.
		start = endDay.AddDate(0, 0, -(days - 1)).Format(hookDateLayout)
	}
	if startDay, _ := time.Parse(hookDateLayout, start); startDay.After(endDay) {
		return "", "", usageErr(fmt.Errorf("--start %s is after --end %s", start, end))
	}
	return start, end, nil
}

func newNovelQbrCmd(flags *rootFlags) *cobra.Command {
	var start, end string
	var days, campaigns, maxScanPages int

	cmd := &cobra.Command{
		Use:   "qbr <organization id>",
		Short: "Build one client's quarterly review data: simulation and training totals, monthly trends, recent campaigns and overdue training.",
		Long: "Combines the organization, its summary for the window, its newest campaigns and its overdue training.\n" +
			"Figures are the organization's own records, not its clients'. If any section fails the command fails,\n" +
			"so a report is never missing a section without saying so. report_rate is null when Hook withholds\n" +
			"reported outcomes for the organization.",
		Example: strings.Trim(`
  hook-security-cli qbr org_4521 --days 90 --agent
  hook-security-cli qbr org_4521 --start 2026-07-01 --end 2026-09-30`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "org=org_4521"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "qbr")
			}
			if len(args) != 1 || args[0] == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass exactly one organization id, for example org_4521"))
			}
			startDate, endDate, err := qbrWindow(time.Now(), days, start, end)
			if err != nil {
				return err
			}
			if campaigns < 1 || campaigns > 100 || maxScanPages < 1 {
				return usageErr(fmt.Errorf("--campaigns must be 1 to 100 and --max-scan-pages must be at least 1"))
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
			fail := func(err error) error { return classifyAPIError(cmd.OutOrStdout(), err, flags) }
			org := args[0]

			raw, err := c.GetNoCache(ctx, replacePathParam("/v1/organizations/{id}", "id", org), nil)
			if err != nil {
				return fail(err)
			}
			view := qbrView{StartDate: startDate, EndDate: endDate, Notes: make([]string, 0)}
			if err := json.Unmarshal(raw, &view.Organization); err != nil {
				return fmt.Errorf("decoding organization: %w", err)
			}

			raw, err = c.GetNoCache(ctx, replacePathParam("/v1/organizations/{id}/summary", "id", org), map[string]string{"start_date": startDate, "end_date": endDate})
			if err != nil {
				return fail(err)
			}
			var summary qbrSummary
			if err := json.Unmarshal(raw, &summary); err != nil {
				return fmt.Errorf("decoding summary: %w", err)
			}
			view.Simulations, view.Training = summary.Simulations, summary.Training
			view.SimulationTrend, view.TrainingTrend, view.WithheldOutcomes = summary.SimulationTrend, summary.TrainingTrend, summary.WithheldOutcomes
			view.ReportRate = qbrReportRate(summary.Simulations)

			endDay, _ := time.Parse(hookDateLayout, endDate)
			view.RecentCampaigns, view.MoreCampaigns, err = hookScan[json.RawMessage](ctx, c, "/v1/campaigns", map[string]string{
				"organization_id":  org,
				"order":            "newest",
				"scheduled_after":  startDate + "T00:00:00Z",
				"scheduled_before": endDay.AddDate(0, 0, 1).Format(hookDateLayout) + "T00:00:00Z",
			}, campaigns, 1)
			if err != nil {
				return fail(err)
			}

			overdue, truncated, err := hookScan[hookEnrollment](ctx, c, "/v1/enrollments", map[string]string{"organization_id": org, "status": "overdue"}, 500, maxScanPages)
			if err != nil {
				return fail(err)
			}
			view.OverdueTraining = qbrOverdue{Total: len(overdue), Truncated: truncated, People: make([]overduePerson, 0)}
			for _, client := range groupOverdue(overdue, false) {
				view.OverdueTraining.People = append(view.OverdueTraining.People, client.People...)
			}

			if view.Organization.Type == "msp" {
				view.Notes = append(view.Notes, "this organization is an MSP: these are its own records, use client-risk to compare its clients")
			}
			if truncated {
				note := fmt.Sprintf("overdue training stopped at --max-scan-pages %d; its count is partial", maxScanPages)
				view.Notes = append(view.Notes, note)
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+note)
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "id", "name", "status", "scheduled_for", "recipients", "outcomes", "rates", "month", "sent", "opened", "clicked", "landed", "engaged", "reported", "click_rate", "engagement_rate", "assigned", "completed", "outstanding", "overdue", "completion_rate", "enrollment_id", "person_id", "email", "course", "assigned_at", "due_at")
		},
	}
	cmd.Flags().IntVar(&days, "days", 90, "Length of the review window in days, ending today or at --end")
	cmd.Flags().StringVar(&start, "start", "", "First day of the window, YYYY-MM-DD; overrides --days")
	cmd.Flags().StringVar(&end, "end", "", "Last day of the window, YYYY-MM-DD; defaults to today (UTC)")
	cmd.Flags().IntVar(&campaigns, "campaigns", 10, "How many of the newest campaigns in the window to include, 1 to 100")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 20, "Maximum pages of overdue enrollments to read before reporting a partial answer")
	return cmd
}
