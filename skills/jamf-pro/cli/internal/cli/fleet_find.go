// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: fleet find locates a device across every client instance.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/fleet"
)

func newNovelFleetFindCmd(flags *rootFlags) *cobra.Command {
	var kind string
	var detail bool
	var limit int
	var scope fleetScope

	cmd := &cobra.Command{
		Use:   "find <serial|name|username|email>",
		Short: "Find a Mac, iPad or iPhone by serial, name, username or email across every client's Jamf Pro instance at once.",
		Long: `Find a Mac, iPad or iPhone by serial, name, username or email across every client's Jamf Pro instance at once.

Use this command to locate a Mac or mobile device by serial, name, username, or email across every registered instance, and with --detail to see what is scoped to and failing on a single match. Do NOT use this command for fleet-wide compliance lists; use 'fleet posture' instead. Do NOT use it for offline lookups in a synced store; use 'search' instead.

Each instance is queried with one RSQL filter that ORs an exact serial, username
and email match with a contains-match on the device name, on both computer
inventory and mobile devices. Every match carries the instance it lives in and
the next per-instance command to run; nothing is executed for you. With
--detail and exactly one match, the row is joined with Classic API computer
management (scoped policies and profiles), computer history, and the device's
recent MDM commands.`,
		Example: strings.Trim(`
  jamf-pro-cli fleet find C02XK1ABJG5H --agent
  jamf-pro-cli fleet find jdoe --kind computer --json
  jamf-pro-cli fleet find C02XK1ABJG5H --detail --agent`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "query=C02XK1ABJG5H",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet find")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one serial, device name, username or email is required"))
			}
			if kind != "all" && kind != "computer" && kind != "mobile" {
				return usageErr(fmt.Errorf("--kind %q: use all, computer or mobile", kind))
			}
			if limit < 1 || limit > 2000 {
				return usageErr(fmt.Errorf("--limit must be between 1 and 2000"))
			}
			q := strings.TrimSpace(args[0])
			instances, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(instances) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			compFilter, mobFilter := fleet.FindFilters(q)
			bin := cmd.Root().Name()
			size := strconv.Itoa(limit)
			var reqs []fleet.Request
			if kind != "mobile" {
				// page=0 pins one page of at most --limit matches per instance.
				reqs = append(reqs, jamfGet("/api/v4/computers-inventory", "section", "GENERAL", "section", "HARDWARE",
					"section", "USER_AND_LOCATION", "filter", compFilter, "page", "0", "page-size", size))
			}
			if kind != "computer" {
				reqs = append(reqs, jamfGet("/api/v2/mobile-devices/detail", "section", "GENERAL", "section", "HARDWARE",
					"section", "USER_AND_LOCATION", "filter", mobFilter, "page", "0", "page-size", size))
			}
			rows, failed := collect(cmd, &scope, flags, instances, reqs,
				func(p fleet.Instance, bodies []json.RawMessage) ([]fleet.FindRow, error) {
					var out []fleet.FindRow
					i := 0
					if kind != "mobile" {
						comps, err := fleet.Results[fleet.Computer](bodies[i], "id")
						if err != nil {
							return nil, fmt.Errorf("computers-inventory: %w", err)
						}
						cut := warnTruncated(cmd, p.Name, "computers", bodies[i], len(comps))
						for _, c := range comps {
							r := fleet.ComputerFindRow(bin, p.Name, c)
							r.Truncated = cut
							out = append(out, r)
						}
						i++
					}
					if kind != "computer" {
						mobs, err := fleet.Results[fleet.Mobile](bodies[i], "mobileDeviceId")
						if err != nil {
							return nil, fmt.Errorf("mobile-devices: %w", err)
						}
						cut := warnTruncated(cmd, p.Name, "mobile devices", bodies[i], len(mobs))
						for _, m := range mobs {
							r := fleet.MobileFindRow(bin, p.Name, m)
							r.Truncated = cut
							out = append(out, r)
						}
					}
					return out, nil
				},
				func(p fleet.Instance, err error) fleet.FindRow {
					return fleet.FindRow{Instance: p.Name, Error: err.Error()}
				})
			if detail {
				return findDetail(cmd, flags, &scope, instances, rows, failed)
			}
			if err := render(cmd, flags, rows, []string{"instance", "error"}, fmt.Sprintf("No device matching %q.", q), findColumns, findItem); err != nil {
				return err
			}
			return allFailed(failed, len(instances))
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "all", "Device kind to search: all, computer or mobile")
	cmd.Flags().BoolVar(&detail, "detail", false, "With exactly one match, add its scoped policies/profiles, history and recent MDM commands")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum matches returned per instance and device kind")
	addFleetScopeFlags(cmd, &scope)
	return cmd
}

var findColumns = []string{"instance", "kind", "id", "name", "serial", "username", "last_contact", "next", "error"}

func findItem(r fleet.FindRow) map[string]any {
	return map[string]any{"instance": r.Instance, "kind": r.Kind, "id": r.ID, "name": r.Name, "serial": r.Serial,
		"username": r.Username, "last_contact": r.LastContact, "next": r.Next, "error": r.Error}
}

// findDetail expands a single match. Several matches (or none) are printed
// as rows with a note instead, so --detail never guesses which device.
func findDetail(cmd *cobra.Command, flags *rootFlags, scope *fleetScope, instances []fleet.Instance, rows []fleet.FindRow, failed int) error {
	var matches []fleet.FindRow
	errs := []fleet.FindRow{}
	truncated := false
	for _, r := range rows {
		if r.Error == "" {
			matches = append(matches, r)
			truncated = truncated || r.Truncated
		} else {
			errs = append(errs, r)
		}
	}
	// An instance that could not be read may hold another match, so one
	// match is only proven unique when every instance answered in full.
	if len(matches) != 1 || truncated || len(errs) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "note: --detail needs exactly one match from fully read instances (found %d, %d instance errors); narrow with --kind, --only or a serial\n", len(matches), len(errs))
		if err := render(cmd, flags, rows, []string{"instance", "error"}, "No matching device.", findColumns, findItem); err != nil {
			return err
		}
		return allFailed(failed, len(instances))
	}
	m := matches[0]
	var target fleet.Instance
	for _, p := range instances {
		if p.Name == m.Instance {
			target = p
		}
	}
	reqs := []fleet.Request{}
	if m.ManagementID != "" {
		reqs = append(reqs, jamfGet("/api/v2/mdm/commands", "filter", "clientManagementId=="+fleet.RSQLQuote(m.ManagementID),
			"sort", "dateSent:desc", "page", "0", "page-size", "25"))
	}
	if m.Kind == "computer" {
		reqs = append(reqs, jamfGet("/JSSResource/computermanagement/id/"+m.ID), jamfGet("/JSSResource/computerhistory/id/"+m.ID))
	}
	out := map[string]any{"match": m}
	if len(reqs) > 0 {
		r := fleet.Runner{Exe: fleetExe, Timeout: flags.timeout, Registry: scope.reg}
		bodies, err := r.Fetch(cmd.Context(), target, reqs...)
		if err != nil {
			out["detail_error"] = err.Error()
		} else {
			i := 0
			if m.ManagementID != "" {
				if cmds, cerr := fleet.Items(bodies[i]); cerr != nil {
					out["detail_error"] = cerr.Error()
				} else {
					out["mdm_commands"] = cmds
				}
				i++
			}
			if m.Kind == "computer" {
				out["management"] = json.RawMessage(bodies[i])
				out["history"] = json.RawMessage(bodies[i+1])
			}
		}
	}
	return printJSONFiltered(cmd.OutOrStdout(), out, flags)
}

// warnTruncated says when an instance matched more devices than --limit let
// through, so a capped answer is never mistaken for the full set.
func warnTruncated(cmd *cobra.Command, instance, kind string, body json.RawMessage, got int) bool {
	var page struct {
		TotalCount int `json:"totalCount"`
	}
	if json.Unmarshal(body, &page) == nil && page.TotalCount > got {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s matched %d %s, showing %d; narrow the query or raise --limit\n", instance, page.TotalCount, kind, got)
		return true
	}
	return false
}
