// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// policies hygiene: join policy config with live hit counters per VDOM.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/fortios"
)

type hygieneView struct {
	Policies []fortios.PolicyRow `json:"policies"`
	Summary  map[string]int      `json:"summary"`
	// CountersUnavailable lists device/VDOM pairs whose hit counters could not
	// be read; "unused" was not evaluated there.
	CountersUnavailable []string `json:"counters_unavailable,omitempty"`
	// VDOMsUnlisted names devices whose VDOM list was unreadable; only root
	// was checked there.
	VDOMsUnlisted []string       `json:"vdoms_unlisted,omitempty"`
	FetchFailures []fetchFailure `json:"fetch_failures,omitempty"`
}

func newNovelPoliciesHygieneCmd(flags *rootFlags) *cobra.Command {
	var fo fleetOpts
	var unusedDays int
	cmd := &cobra.Command{
		Use:   "hygiene",
		Args:  rejectExtraArgs,
		Short: "List unused, any-any accept, logging-disabled and disabled firewall policies per device and VDOM",
		Long: strings.Trim(`
Use this command to find risky or dead firewall policies right now: unused,
any-any accept, logging disabled, disabled. Do NOT use this command to see which
policies were added or edited recently; use 'fleet changes' instead.

Joins cmdb firewall/policy (status, action, srcaddr, dstaddr, service,
logtraffic) with monitor firewall/policy (hit_count, last_used) on policyid.
"unused" means a counter row exists with zero hits or no hit for --unused-days;
policies without a counter row are never called unused. Counters reset on
reboot, so a recently rebooted box under-reports usage.`, "\n"),
		Example:     "  fortinet-cli policies hygiene --unused-days 90 --agent\n  fortinet-cli policies hygiene --device acme-hq --vdoms root",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "policies hygiene")
			}
			if unusedDays < 1 {
				return usageErr(fmt.Errorf("--unused-days must be at least 1"))
			}
			ctx, cancel := context.WithCancel(cmd.Context()) // --timeout bounds each request, not the fleet: see fanOut
			defer cancel()
			targets, failures, err := loadFleet(flags, fo)
			if err != nil {
				return err
			}
			type deviceRows struct {
				rows          []fortios.PolicyRow
				noCounters    []string
				vdomsUnlisted []string
			}
			per, fails := fanOut(ctx, targets, fo.concurrency, func(ctx context.Context, t fleetTarget) (deviceRows, error) {
				c, err := t.client(flags)
				if err != nil {
					return deviceRows{}, err
				}
				var out deviceRows
				vdoms, vErr := t.vdoms(ctx, c)
				if vErr != nil {
					out.vdomsUnlisted = append(out.vdomsUnlisted, t.Name+" ("+vErr.Error()+")")
				}
				for _, vdom := range vdoms {
					_, pol, err := fortiGetList(ctx, c, "/cmdb/firewall/policy", vdomParams(vdom))
					if err != nil {
						return deviceRows{}, fmt.Errorf("vdom %s: %w", vdom, err)
					}
					var stats []fortios.Obj
					if _, list, err := fortiGetList(ctx, c, "/monitor/firewall/policy", vdomParams(vdom)); err == nil {
						stats = list
					} else {
						out.noCounters = append(out.noCounters, fmt.Sprintf("%s/%s (%s)", t.Name, vdom, shortErr(err)))
					}
					out.rows = append(out.rows, fortios.Policies(t.Name, vdom, pol, stats, time.Now(), time.Duration(unusedDays)*24*time.Hour)...)
				}
				return out, nil
			})
			failures = append(failures, fails...)
			view := hygieneView{Policies: []fortios.PolicyRow{}, Summary: map[string]int{}, FetchFailures: failures}
			for _, d := range per {
				view.CountersUnavailable = append(view.CountersUnavailable, d.noCounters...)
				view.VDOMsUnlisted = append(view.VDOMsUnlisted, d.vdomsUnlisted...)
				for _, r := range d.rows {
					view.Policies = append(view.Policies, r)
					for _, issue := range r.Issues {
						view.Summary[issue]++
					}
				}
			}
			if len(failures) > 0 {
				// An agent reading only the summary must not see a clean result.
				view.Summary["unknown_devices"] = len(failures)
			}
			if len(view.VDOMsUnlisted) > 0 {
				view.Summary["vdoms_unlisted"] = len(view.VDOMsUnlisted)
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: VDOM list unreadable for %s\n", strings.Join(view.VDOMsUnlisted, ", "))
			}
			warnFailures(cmd, failures, len(targets)+len(failures)-len(fails))
			if len(view.CountersUnavailable) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: hit counters unreadable for %s; unused policies were not evaluated there\n", strings.Join(view.CountersUnavailable, ", "))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "vdom", "issues", "counters_unavailable", "vdoms_unlisted")
			}
			if len(view.Policies) == 0 {
				switch {
				case len(failures) == 0 && len(view.VDOMsUnlisted) == 0 && len(view.CountersUnavailable) == 0:
					fmt.Fprintln(cmd.OutOrStdout(), "No risky or unused policies found.")
				case len(failures) == 0 && len(view.VDOMsUnlisted) == 0:
					fmt.Fprintf(cmd.OutOrStdout(), "No risky policies found; hit counters were unreadable for %s, so unused policies were not evaluated there.\n", strings.Join(view.CountersUnavailable, ", "))
				case len(failures) == 0:
					fmt.Fprintf(cmd.OutOrStdout(), "No risky or unused policies found in the VDOMs checked; %d device(s) could not list their VDOMs (UNKNOWN below).\n", len(view.VDOMsUnlisted))
				case len(per) == 0:
					fmt.Fprintln(cmd.OutOrStdout(), "No policies evaluated: no device could be read (UNKNOWN below).")
				default:
					fmt.Fprintf(cmd.OutOrStdout(), "No risky or unused policies found on the %d readable device(s); %d device(s) UNKNOWN below.\n", len(per), len(failures))
				}
			}
			for _, r := range view.Policies {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-8s %6d  %-30s %s\n", r.Device, r.VDOM, r.PolicyID, r.Name, strings.Join(r.Issues, ","))
			}
			for _, f := range failures {
				fmt.Fprintf(cmd.OutOrStdout(), "UNKNOWN %-24s %s\n", f.ID, f.Error)
			}
			for _, v := range view.VDOMsUnlisted {
				fmt.Fprintf(cmd.OutOrStdout(), "UNKNOWN %s\n", v)
			}
			return nil
		},
	}
	addFleetFlags(cmd, &fo)
	cmd.Flags().IntVar(&unusedDays, "unused-days", 90, "Days without a hit before an accept policy counts as unused")
	return cmd
}
