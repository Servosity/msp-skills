// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"jira-service-management-pp-cli/internal/cliutil"
)

// connectedTicket.Breached is set only when the ticket's SLAs were read
// (absent with --no-sla or when SLAError is set).
type connectedTicket struct {
	Key      string   `json:"key"`
	Title    string   `json:"title,omitempty"`
	Status   string   `json:"status,omitempty"`
	SLA      *slaRisk `json:"sla,omitempty"`
	Breached *bool    `json:"breached,omitempty"`
	SLAError string   `json:"sla_error,omitempty"`
}

type cmdbObjectTickets struct {
	ID      string            `json:"id"`
	Key     string            `json:"key"`
	Label   string            `json:"label"`
	Type    string            `json:"type"`
	Tickets []connectedTicket `json:"tickets"`
}

type cmdbTicketsView struct {
	Query         string              `json:"aql"`
	Objects       []cmdbObjectTickets `json:"objects"`
	Truncated     bool                `json:"truncated,omitempty"`
	Note          string              `json:"note,omitempty"`
	FetchFailures []fetchFailure      `json:"fetch_failures,omitempty"`
}

func newNovelCmdbTicketsCmd(flags *rootFlags) *cobra.Command {
	var aql string
	var maxObjects int
	var noSLA bool

	cmd := &cobra.Command{
		Use:   "tickets [name-or-key]",
		Short: "Assets objects matching a name, key or AQL, with every connected ticket and its SLA breach state.",
		Long: strings.Trim(`
Resolves a hostname/label (Name = "..."), an object key (Key = "ITAM-12", which
also matches a Name of the same text) or a raw --aql query to Assets objects, lists each object's connected service desk
tickets, and adds each ticket's most urgent running SLA. Needs Assets (JSM
Premium/Enterprise) plus JIRA_SERVICE_MANAGEMENT_CLOUD_ID and _WORKSPACE_ID, or run
'cmdb discover --save' once per client site.

Use this command to see which service desk tickets touch an Assets object.
Do NOT use this command to compare Assets against an RMM/PSA inventory export; use 'cmdb reconcile' instead.`, "\n"),
		Example: strings.Trim(`
  jira-service-management-cli cmdb tickets srv-01 --agent
  jira-service-management-cli cmdb tickets --aql 'objectType = "Laptop" AND Status = "In use"' --max-objects 10`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "name-or-key=srv-01"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cmdb tickets")
			}
			ql := strings.TrimSpace(aql)
			if ql == "" {
				if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("pass an object name or key, or --aql"))
				}
				ql = cmdbLookupAQL(strings.TrimSpace(args[0]))
			}
			if maxObjects < 1 {
				return usageErr(fmt.Errorf("--max-objects must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxObjects > 5 {
				maxObjects = 5
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			objs, _, truncated, err := assetsAQL(ctx, c, ql, maxObjects)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view := cmdbTicketsView{Query: ql, Objects: make([]cmdbObjectTickets, 0, len(objs)), Truncated: truncated, FetchFailures: make([]fetchFailure, 0)}
			ticketKeys := make([]string, 0)
			seen := map[string]bool{}
			for _, o := range objs {
				ot := cmdbObjectTickets{ID: string(o.ID), Key: o.ObjectKey, Label: o.Label, Type: o.ObjectType.Name, Tickets: make([]connectedTicket, 0)}
				oseg, segErr := jsmPathSeg(string(o.ID))
				if segErr != nil {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{ID: o.ObjectKey, Error: segErr.Error(), err: segErr})
					view.Objects = append(view.Objects, ot)
					continue
				}
				raw, err := jsmGet(ctx, c, assetsBase+"/objectconnectedtickets/"+oseg+"/tickets", nil)
				if err != nil {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{ID: o.ObjectKey, Error: err.Error(), err: err})
					view.Objects = append(view.Objects, ot)
					continue
				}
				var resp struct {
					Tickets []struct {
						Key    string `json:"key"`
						Title  string `json:"title"`
						Status struct {
							Name string `json:"name"`
						} `json:"status"`
					} `json:"tickets"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{ID: o.ObjectKey, Error: "decoding connected tickets: " + err.Error()})
				}
				for _, t := range resp.Tickets {
					ot.Tickets = append(ot.Tickets, connectedTicket{Key: t.Key, Title: t.Title, Status: t.Status.Name})
					if t.Key != "" && !seen[t.Key] {
						seen[t.Key] = true
						ticketKeys = append(ticketKeys, t.Key)
					}
				}
				view.Objects = append(view.Objects, ot)
			}
			if err := allFailed(view.FetchFailures, len(objs)); err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if !noSLA && len(ticketKeys) > 0 {
				slas, failures := fanOut(ctx, ticketKeys, 4, func(ctx context.Context, key string) ([]jsmSLA, error) {
					return fetchSLAs(ctx, c, key)
				})
				// SLA is enrichment: a ticket from a non-JSM project has none
				// (404). If every lookup failed for another reason (auth, 429,
				// 5xx), "breached:false" would be a lie, so fail instead.
				if err := allFailed(failures, len(ticketKeys)); err != nil && !allNotFound(failures) {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				slaErr := map[string]string{}
				for _, f := range failures {
					slaErr[f.ID] = f.Error
				}
				view.FetchFailures = append(view.FetchFailures, failures...)
				for i := range view.Objects {
					for j := range view.Objects[i].Tickets {
						t := &view.Objects[i].Tickets[j]
						if t.SLAError = slaErr[t.Key]; t.SLAError != "" {
							continue
						}
						breached := false
						if r, ok := mostUrgent(slas[t.Key]); ok {
							t.SLA = &r
							breached = r.Breached
						}
						t.Breached = &breached
					}
				}
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d lookups failed; see fetch_failures\n", len(view.FetchFailures))
			}
			if len(view.Objects) == 0 {
				view.Note = "no Assets objects matched " + ql
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			if view.Note != "" {
				fmt.Fprintln(w, view.Note)
			}
			for _, o := range view.Objects {
				fmt.Fprintf(w, "%s  %s (%s): %d tickets\n", termSafe(o.Key), termSafe(o.Label), termSafe(o.Type), len(o.Tickets))
				for _, t := range o.Tickets {
					flag := ""
					if t.Breached != nil && *t.Breached {
						flag = " BREACHED"
					}
					fmt.Fprintf(w, "    %-12s %-22s %s%s\n", termSafe(t.Key), truncateText(t.Status, 22), truncateText(t.Title, 60), flag)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&aql, "aql", "", "Raw AQL query instead of a name or key")
	cmd.Flags().IntVar(&maxObjects, "max-objects", 25, "Maximum Assets objects to resolve")
	cmd.Flags().BoolVar(&noSLA, "no-sla", false, "Skip the per-ticket SLA lookup")
	return cmd
}

// cmdbLookupAQL turns a bare argument into AQL. Key-shaped input (ITAM-12)
// also matches Name, because uppercase hostnames like SRV-01 look like keys.
func cmdbLookupAQL(arg string) string {
	if issueKeyLike(arg) {
		return "Key = " + aqlQuote(arg) + " OR Name = " + aqlQuote(arg)
	}
	return "Name = " + aqlQuote(arg)
}
