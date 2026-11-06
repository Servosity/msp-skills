// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet policy-hygiene finds broken policies on every client instance.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/fleet"
)

type hygieneView struct {
	Rows            []fleet.HygieneRow `json:"rows"`
	ScannedPolicies int                `json:"scanned_policies"`
	MaxPolicies     int                `json:"max_policies"`
	SkippedPolicies int                `json:"skipped_policies"`
	Note            string             `json:"note,omitempty"`
}

func newNovelFleetPolicyHygieneCmd(flags *rootFlags) *cobra.Command {
	var maxPolicies int
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "policy-hygiene",
		Short: "Flags policies that are disabled, can never run, have no scope, are expired, uncategorized, or point at a missing script or package",
		Long: `Flags policies that are disabled, can never run, have no scope, are expired, uncategorized, or point at a missing script or package.

Use this command to find policies that are disabled, can never run, have no scope, are expired, are uncategorized, or reference a missing script or package. Do NOT use this command to compare a client against the MSP standard; use 'fleet baseline' instead.

Reads every UI- or API-created Classic policy (/JSSResource/policies/createdBy/jss,
then each /JSSResource/policies/id/<id>; one-off Jamf Remote policies are skipped) and joins it with the instance's Jamf Pro API
scripts and packages. Defects:
  disabled         general.enabled is false
  no_trigger       enabled, no trigger_* set, empty custom trigger, not in Self Service
  no_scope         not all computers and no computers, groups, buildings or departments
  expired          date_time_limitations expiration date has passed
  uncategorized    no category
  missing_script   a referenced script id and name are both unknown
  missing_package  a referenced package id and name are both unknown
At most --max-policies policies are read per instance; scanned_policies and
skipped_policies report how many were checked.`,
		Example: strings.Trim(`
  jamf-pro-cli fleet policy-hygiene --agent
  jamf-pro-cli fleet policy-hygiene --only acme --max-policies 200 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet policy-hygiene")
			}
			if maxPolicies < 1 {
				return usageErr(fmt.Errorf("--max-policies must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxPolicies > 25 {
				maxPolicies = 25
			}
			instances, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(instances) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			now := time.Now().UTC()
			type result struct {
				row              fleet.HygieneRow
				scanned, skipped int
				summary          bool
			}
			results, failed := collectFn(cmd, &scope, flags, instances,
				func(fetch fetchFunc, p fleet.Instance) ([]result, error) {
					// createdBy/jss: UI- and API-created policies; one-off Jamf
					// Remote policies are not configuration to audit.
					bodies, err := fetch(jamfGet("/JSSResource/policies/createdBy/jss"), jamfGet("/api/v1/scripts", "page-size", "100"), jamfGet("/api/v1/packages", "page-size", "100"))
					if err != nil {
						return nil, err
					}
					list, err := fleet.ClassicList(bodies[0], "policies", "policy", "id")
					if err != nil {
						return nil, fmt.Errorf("policies: %w", err)
					}
					scripts, err := fleet.Results[fleet.Script](bodies[1], "id")
					if err != nil {
						return nil, fmt.Errorf("scripts: %w", err)
					}
					packages, err := fleet.Results[fleet.Package](bodies[2], "id")
					if err != nil {
						return nil, fmt.Errorf("packages: %w", err)
					}
					known := fleet.NewKnown(scripts, packages)
					var ids []string
					for i, o := range list {
						id := fleet.IDString(o["id"])
						if !isDigits(id) {
							return nil, fmt.Errorf("policies: entry %d has an id that is not numeric", i)
						}
						ids = append(ids, id)
					}
					skipped := 0
					if len(ids) > maxPolicies {
						skipped = len(ids) - maxPolicies
						ids = ids[:maxPolicies]
					}
					out := []result{{summary: true, scanned: len(ids), skipped: skipped}}
					if len(ids) == 0 {
						return out, nil
					}
					reqs := make([]fleet.Request, len(ids))
					for i, id := range ids {
						reqs[i] = jamfGet("/JSSResource/policies/id/" + id)
					}
					details, err := fetch(reqs...)
					if err != nil {
						return nil, fmt.Errorf("policy detail: %w", err)
					}
					for _, d := range details {
						row, err := fleet.PolicyDefects(p.Name, d, known, now)
						if err != nil {
							return nil, err
						}
						if len(row.Defects) > 0 {
							out = append(out, result{row: row})
						}
					}
					return out, nil
				},
				func(p fleet.Instance, err error) result {
					return result{row: fleet.HygieneRow{Instance: p.Name, Error: err.Error()}}
				})
			view := hygieneView{Rows: []fleet.HygieneRow{}, MaxPolicies: maxPolicies}
			for _, r := range results {
				if r.summary {
					view.ScannedPolicies += r.scanned
					view.SkippedPolicies += r.skipped
					continue
				}
				view.Rows = append(view.Rows, r.row)
			}
			if view.SkippedPolicies > 0 {
				view.Note = fmt.Sprintf("%d policies were not checked; raise --max-policies to read them all", view.SkippedPolicies)
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+view.Note)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) && !flags.csv && !flags.plain {
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "rows", "instance", "defects", "error", "scanned_policies", "skipped_policies", "note"); err != nil {
					return err
				}
				return allFailed(failed, len(instances))
			}
			if err := render(cmd, flags, view.Rows, []string{"instance", "defects", "error"}, fmt.Sprintf("No defects in %d checked policies.", view.ScannedPolicies),
				[]string{"instance", "id", "policy", "defects", "detail", "error"}, func(r fleet.HygieneRow) map[string]any {
					return map[string]any{"instance": r.Instance, "id": r.ID, "policy": r.Name, "defects": r.Defects, "detail": r.Detail, "error": r.Error}
				}); err != nil {
				return err
			}
			return allFailed(failed, len(instances))
		},
	}
	cmd.Flags().IntVar(&maxPolicies, "max-policies", 500, "Most policies read per instance (each costs one API call)")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
