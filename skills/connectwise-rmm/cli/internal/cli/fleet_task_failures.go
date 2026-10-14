// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
// Reads the synced fleet list locally; availability, patches and sessions come from the live API.

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"connectwise-rmm-pp-cli/internal/fleet"
)

type taskFailuresView struct {
	TaskID          string               `json:"task_id"`
	Instances       []fleet.TaskInstance `json:"instances"`
	Failed          int                  `json:"failed"`
	Succeeded       int                  `json:"succeeded"`
	Groups          []fleet.FailureGroup `json:"groups"`
	FailedEndpoints []string             `json:"failed_endpoint_ids"`
	NoResult        []string             `json:"no_result_endpoint_ids,omitempty"`
	FetchFailures   []fetchFailure       `json:"fetch_failures,omitempty"`
}

func newNovelFleetTaskFailuresCmd(flags *rootFlags) *cobra.Command {
	var allInstances bool
	var dbPath string
	cmd := &cobra.Command{
		Use:   "task-failures [task-id]",
		Short: "Group a task run's failed endpoints by identical error text, by task ID, with IDs ready to re-run",
		Long: strings.Trim(`
Failed endpoints from a task run, grouped by identical errorDetails text and
joined to endpoint, company and site names. Reads the newest task instance by
default (--all-instances reads every instance). failed_endpoint_ids is ready
to pass back to 'run --endpoint'. When the task was started with 'run', the
targets that never reported a result are listed under no_result_endpoint_ids.

Use this command after a task ran, to see which endpoints failed and why. Do
NOT use this command to schedule or watch a new run; use 'run' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-rmm-cli fleet task-failures 7c9e6679-7425-40de-944b-e07fc1f90ae7 --agent
  connectwise-rmm-cli fleet task-failures 7c9e6679-7425-40de-944b-e07fc1f90ae7 --all-instances --select failed_endpoint_ids`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "task-id=7c9e6679-7425-40de-944b-e07fc1f90ae7"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet task-failures")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a task ID is required (see 'tasks list-summaries' or the ID printed by 'run')"))
			}
			taskID := strings.TrimSpace(args[0])
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			view := taskFailuresView{TaskID: taskID, Instances: make([]fleet.TaskInstance, 0), Groups: make([]fleet.FailureGroup, 0), FailedEndpoints: make([]string, 0)}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			base := "/api/platform/v1/automation/tasks/" + url.PathEscape(taskID) + "/instances/"
			raw, err := c.GetNoCache(ctx, base+"summary", nil)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			instances, ok := fleet.ParseTaskInstances(raw)
			if !ok {
				return apiErr(fmt.Errorf("task %s: unreadable instance summary", taskID))
			}
			if !allInstances && len(instances) > 1 {
				instances = instances[:1]
			}
			view.Instances = instances
			results := make([]fleet.TaskResult, 0)
			for _, inst := range instances {
				rraw, err := c.GetNoCache(ctx, base+url.PathEscape(inst.ID)+"/results", nil)
				if err != nil {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{ID: inst.ID, Error: err.Error()})
					continue
				}
				parsed, ok := fleet.ParseTaskResults(rraw)
				if !ok {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{ID: inst.ID, Error: "unreadable results payload"})
					continue
				}
				results = append(results, parsed...)
			}
			warnFailures(cmd, "task result", view.FetchFailures, len(instances))

			names := map[string]fleet.Endpoint{}
			var targets []string
			db, missing, err := openFleetStore(ctx, cmd, dbPath)
			if err != nil {
				return err
			}
			if !missing {
				defer db.Close()
				if eps, err := loadFleet(db); err == nil {
					for _, e := range eps {
						names[e.ID] = e
					}
				}
				if rec, err := db.Get(cacheTaskRun, taskID); err == nil {
					var r struct {
						Targets []string `json:"targets"`
					}
					if json.Unmarshal(rec, &r) == nil {
						targets = r.Targets
					}
				}
			}
			reported := map[string]bool{}
			for _, r := range results {
				reported[r.EndpointID] = true
				if r.Failed() {
					view.Failed++
					view.FailedEndpoints = append(view.FailedEndpoints, r.EndpointID)
				} else if strings.Contains(strings.ToUpper(r.Status), "SUCCESS") {
					view.Succeeded++
				}
			}
			for _, t := range targets {
				if !reported[t] {
					view.NoResult = append(view.NoResult, t)
				}
			}
			view.Groups = fleet.GroupFailures(results, names)
			// Every results read failed: print what is known, then fail loudly
			// instead of reporting "0 failed".
			var allFailed error
			if len(instances) > 0 && len(view.FetchFailures) == len(instances) {
				allFailed = classifyAPIErrorOnly(fmt.Errorf("reading task results: %s", view.FetchFailures[0].Error))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
				return allFailed
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Task %s: %d failed, %d succeeded across %d instance(s)\n", taskID, view.Failed, view.Succeeded, len(view.Instances))
			for _, g := range view.Groups {
				ids := make([]string, 0, len(g.Endpoints))
				for _, e := range g.Endpoints {
					ids = append(ids, firstNonEmpty(e.Name, e.ID))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d x %s\n  %s\n", g.Count, g.Error, strings.Join(ids, ", "))
			}
			if len(view.NoResult) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nNo result yet from %d target(s): %s\n", len(view.NoResult), strings.Join(view.NoResult, ", "))
			}
			return allFailed
		},
	}
	cmd.Flags().BoolVar(&allInstances, "all-instances", false, "Read every task instance, not just the newest")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
