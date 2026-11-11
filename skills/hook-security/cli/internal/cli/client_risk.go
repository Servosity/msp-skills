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

type hookOrgRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type hookClientSummary struct {
	OrganizationID   string           `json:"organization_id"`
	OrganizationName string           `json:"organization_name"`
	StartDate        string           `json:"start_date"`
	EndDate          string           `json:"end_date"`
	Simulations      *hookSimulations `json:"simulations"`
	Training         *hookTraining    `json:"training"`
	TooLarge         bool             `json:"too_large"`
	WithheldOutcomes []string         `json:"withheld_outcomes"`
}

type clientRiskRow struct {
	hookClientSummary
	MSPID     string   `json:"msp_id"`
	ClickRate *float64 `json:"click_rate"`
}

type clientRiskFleet struct {
	Sent                 int      `json:"sent"`
	Clicked              int      `json:"clicked"`
	ClickRate            *float64 `json:"click_rate"`
	ClientsCounted       int      `json:"clients_counted"`
	ClientsTooLarge      int      `json:"clients_too_large"`
	ClientsNoSimulations int      `json:"clients_no_simulations"`
}

type hookFetchFailure struct {
	OrganizationID string `json:"organization_id"`
	Error          string `json:"error"`
}

type clientRiskView struct {
	MSPsScanned   int                `json:"msps_scanned"`
	ClientsTotal  int                `json:"clients_total"`
	Clients       []clientRiskRow    `json:"clients"`
	Fleet         clientRiskFleet    `json:"fleet"`
	FetchFailures []hookFetchFailure `json:"fetch_failures"`
	Truncated     bool               `json:"truncated"`
	Note          string             `json:"note,omitempty"`
}

// rankClientRisk orders by click rate, highest first. Clients with no rate go
// last, because no measurement is not the same as a zero click rate.
func rankClientRisk(rows []clientRiskRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].ClickRate, rows[j].ClickRate
		if (a == nil) != (b == nil) {
			return a != nil
		}
		if a != nil && *a != *b {
			return *a > *b
		}
		return rows[i].OrganizationName < rows[j].OrganizationName
	})
}

// clientRiskTotals computes the fleet rate from summed counts. Averaging each
// client's rate would let a client with one email outweigh one with thousands.
func clientRiskTotals(rows []clientRiskRow) clientRiskFleet {
	var fleet clientRiskFleet
	for _, row := range rows {
		switch {
		case row.TooLarge:
			fleet.ClientsTooLarge++
		case row.Simulations == nil:
			fleet.ClientsNoSimulations++
		default:
			fleet.ClientsCounted++
			fleet.Sent += row.Simulations.Sent
			fleet.Clicked += row.Simulations.Clicked
		}
	}
	if fleet.Sent > 0 {
		rate := float64(fleet.Clicked) / float64(fleet.Sent)
		fleet.ClickRate = &rate
	}
	return fleet
}

func newNovelClientRiskCmd(flags *rootFlags) *cobra.Command {
	var org, start, end string
	var top, maxScanPages int

	cmd := &cobra.Command{
		Use:   "client-risk",
		Short: "Rank every client of your MSP by phishing click rate, with a fleet rate computed from totals.",
		Long: "Walks every MSP organization the key covers (or one with --org) and ranks their direct clients by\n" +
			"click rate. The fleet rate is total clicked over total sent. Clients Hook marks too_large, and clients\n" +
			"with no simulations, are listed but left out of the fleet totals. Needs an MSP key; an organization\n" +
			"key has no clients to compare.",
		Example: strings.Trim(`
  hook-security-cli client-risk --top 10 --agent
  hook-security-cli client-risk --org org_4521 --start 2026-07-01 --end 2026-09-30`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "client-risk")
			}
			if err := hookCheckDate("start", start); err != nil {
				return err
			}
			if err := hookCheckDate("end", end); err != nil {
				return err
			}
			if top < 0 || maxScanPages < 1 {
				return usageErr(fmt.Errorf("--top must be 0 or more and --max-scan-pages must be at least 1"))
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

			msps := []hookOrgRef{{ID: org}}
			truncated := false
			if org == "" {
				msps, truncated, err = hookScan[hookOrgRef](ctx, c, "/v1/organizations", map[string]string{"type": "msp"}, 500, maxScanPages)
				if err != nil {
					return hookError(cmd.OutOrStdout(), err, flags)
				}
			}

			params := map[string]string{}
			if start != "" {
				params["start_date"] = start
			}
			if end != "" {
				params["end_date"] = end
			}
			view := clientRiskView{MSPsScanned: len(msps), Clients: make([]clientRiskRow, 0), FetchFailures: make([]hookFetchFailure, 0)}
			seen := map[string]bool{}
			var firstErr error
			for _, msp := range msps {
				path := replacePathParam("/v1/organizations/{id}/clients/summary", "id", msp.ID)
				clients, cut, err := hookScan[hookClientSummary](ctx, c, path, params, 25, maxScanPages)
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					view.FetchFailures = append(view.FetchFailures, hookFetchFailure{OrganizationID: msp.ID, Error: err.Error()})
					continue
				}
				truncated = truncated || cut
				for _, row := range clients {
					if seen[row.OrganizationID] {
						continue
					}
					seen[row.OrganizationID] = true
					out := clientRiskRow{hookClientSummary: row, MSPID: msp.ID}
					if row.Simulations != nil {
						out.ClickRate = row.Simulations.ClickRate
					}
					view.Clients = append(view.Clients, out)
				}
			}
			if len(msps) > 0 && len(view.FetchFailures) == len(msps) {
				return hookError(cmd.OutOrStdout(), firstErr, flags)
			}
			if failed := len(view.FetchFailures); failed > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d MSP fetches failed; totals cover the remaining %d\n", failed, len(msps), len(msps)-failed)
			}

			rankClientRisk(view.Clients)
			view.Fleet = clientRiskTotals(view.Clients)
			view.ClientsTotal = len(view.Clients)
			view.Truncated = truncated
			switch {
			case truncated:
				view.Note = fmt.Sprintf("stopped at --max-scan-pages %d before reading every page; totals are partial, raise it for a full answer", maxScanPages)
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+view.Note)
			case len(msps) == 0:
				view.Note = "this key covers no MSP organization, so there are no clients to compare; use qbr <organization id> for one organization"
			case view.ClientsTotal == 0:
				view.Note = "no client rows returned; --org must be an MSP organization with direct clients"
			}
			if top > 0 && top < len(view.Clients) {
				view.Clients = view.Clients[:top]
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				tw := newTabWriter(cmd.OutOrStdout())
				fmt.Fprintln(tw, "CLIENT\tCLICK RATE\tCLICKED\tSENT\tOVERDUE TRAINING")
				for _, row := range view.Clients {
					clicked, sent, overdue := "n/a", "n/a", "n/a"
					if row.Simulations != nil {
						clicked, sent = fmt.Sprint(row.Simulations.Clicked), fmt.Sprint(row.Simulations.Sent)
					}
					if row.Training != nil {
						overdue = fmt.Sprint(row.Training.Overdue)
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", row.OrganizationName, hookRate(row.ClickRate), clicked, sent, overdue)
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nFleet: %s (%d clicked of %d sent, %d clients counted)\n", hookRate(view.Fleet.ClickRate), view.Fleet.Clicked, view.Fleet.Sent, view.Fleet.ClientsCounted)
				if view.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				}
				return nil
			}
			return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "organization_id", "organization_name", "msp_id", "click_rate", "simulations", "training", "too_large", "withheld_outcomes", "start_date", "end_date", "error")
		},
	}
	cmd.Flags().StringVar(&org, "org", "", "Compare the clients of this one MSP organization (for example org_4521) instead of every MSP the key covers")
	cmd.Flags().StringVar(&start, "start", "", "First day counted, YYYY-MM-DD (Hook defaults to the last 12 months)")
	cmd.Flags().StringVar(&end, "end", "", "Last day counted, YYYY-MM-DD")
	cmd.Flags().IntVar(&top, "top", 0, "Show only this many riskiest clients; fleet totals still cover all of them (0 shows all)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 40, "Maximum pages to read per list before reporting a partial answer")
	return cmd
}
