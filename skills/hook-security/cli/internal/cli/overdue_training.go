// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"hook-security-pp-cli/internal/cliutil"
)

type overduePerson struct {
	EnrollmentID string  `json:"enrollment_id"`
	PersonID     string  `json:"person_id"`
	Name         string  `json:"name"`
	Email        string  `json:"email"`
	Course       string  `json:"course"`
	Status       string  `json:"status"`
	AssignedAt   *string `json:"assigned_at"`
	DueAt        *string `json:"due_at"`
}

type overdueClient struct {
	OrganizationID   string          `json:"organization_id"`
	OrganizationName string          `json:"organization_name"`
	OverdueCount     int             `json:"overdue_count"`
	People           []overduePerson `json:"people,omitempty"`
}

type overdueView struct {
	Clients            []overdueClient `json:"clients"`
	TotalOverdue       int             `json:"total_overdue"`
	ScannedEnrollments int             `json:"scanned_enrollments"`
	MaxScanPages       int             `json:"max_scan_pages"`
	Truncated          bool            `json:"truncated"`
	Note               string          `json:"note,omitempty"`
}

// groupOverdue groups enrollments per client, most overdue first. A learner
// who has since been removed still counts, with empty person fields.
func groupOverdue(enrollments []hookEnrollment, countsOnly bool) []overdueClient {
	byOrg := map[string]*overdueClient{}
	for _, enrollment := range enrollments {
		client := byOrg[enrollment.OrganizationID]
		if client == nil {
			client = &overdueClient{OrganizationID: enrollment.OrganizationID, OrganizationName: enrollment.OrganizationName}
			byOrg[enrollment.OrganizationID] = client
		}
		client.OverdueCount++
		if countsOnly {
			continue
		}
		person := overduePerson{EnrollmentID: enrollment.ID, Course: enrollment.Course.Name, Status: enrollment.Status, AssignedAt: enrollment.AssignedAt, DueAt: enrollment.DueAt}
		if enrollment.Person != nil {
			person.PersonID, person.Name, person.Email = enrollment.Person.ID, enrollment.Person.Name, enrollment.Person.Email
		}
		client.People = append(client.People, person)
	}
	clients := make([]overdueClient, 0, len(byOrg))
	for _, client := range byOrg {
		clients = append(clients, *client)
	}
	sort.Slice(clients, func(i, j int) bool {
		if clients[i].OverdueCount != clients[j].OverdueCount {
			return clients[i].OverdueCount > clients[j].OverdueCount
		}
		return clients[i].OrganizationName < clients[j].OrganizationName
	})
	return clients
}

func newNovelOverdueTrainingCmd(flags *rootFlags) *cobra.Command {
	var org string
	var countsOnly bool
	var maxScanPages int

	cmd := &cobra.Command{
		Use:   "overdue-training",
		Short: "See overdue training grouped by client, with the people and courses behind each count.",
		Long: "Reads overdue enrollments for every organization the key covers, or one with --org, and groups them\n" +
			"per client. Use --counts-only to leave names and emails out of the output.",
		Example: strings.Trim(`
  hook-security-cli overdue-training --org org_4521 --agent
  hook-security-cli overdue-training --counts-only`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "overdue-training")
			}
			if maxScanPages < 1 {
				return usageErr(fmt.Errorf("--max-scan-pages must be at least 1"))
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

			params := map[string]string{"status": "overdue"}
			if org != "" {
				params["organization_id"] = org
			}
			enrollments, truncated, err := hookScan[hookEnrollment](ctx, c, "/v1/enrollments", params, 500, maxScanPages)
			if err != nil {
				return hookError(cmd.OutOrStdout(), err, flags)
			}
			view := overdueView{Clients: groupOverdue(enrollments, countsOnly), TotalOverdue: len(enrollments), ScannedEnrollments: len(enrollments), MaxScanPages: maxScanPages, Truncated: truncated}
			switch {
			case truncated:
				view.Note = fmt.Sprintf("stopped at --max-scan-pages %d before reading every page; counts are partial, raise it for a full answer", maxScanPages)
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+view.Note)
			case len(enrollments) == 0:
				view.Note = "no overdue training for this scope"
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				tw := newTabWriter(cmd.OutOrStdout())
				if countsOnly {
					fmt.Fprintln(tw, "CLIENT\tOVERDUE")
				} else {
					fmt.Fprintln(tw, "CLIENT\tOVERDUE\tNAME\tEMAIL\tCOURSE\tDUE")
				}
				for _, client := range view.Clients {
					if countsOnly {
						fmt.Fprintf(tw, "%s\t%d\n", client.OrganizationName, client.OverdueCount)
						continue
					}
					for _, person := range client.People {
						fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\n", client.OrganizationName, client.OverdueCount, person.Name, person.Email, person.Course, hookString(person.DueAt))
					}
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d overdue across %d clients\n", view.TotalOverdue, len(view.Clients))
				if view.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				}
				return nil
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "organization_id", "organization_name", "overdue_count", "people")
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Only this organization, for example org_4521; required when the key covers too many organizations for one request")
	cmd.Flags().BoolVar(&countsOnly, "counts-only", false, "Show per-client counts without names or emails")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 20, "Maximum pages of enrollments to read before reporting a partial answer")
	return cmd
}
