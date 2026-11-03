// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// audit licenses: fleet renewal queue from monitor license/status.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/fortios"
)

var errBroadPrefix = errors.New("--broad-prefix must be between 0 and 32")

type licenseView struct {
	Queue         []fortios.LicenseRow `json:"queue"`
	Findings      []fortios.Finding    `json:"findings"`
	FetchFailures []fetchFailure       `json:"fetch_failures,omitempty"`
}

func newNovelAuditLicensesCmd(flags *rootFlags) *cobra.Command {
	var fo fleetOpts
	var ao auditOpts
	cmd := &cobra.Command{
		Use:   "licenses",
		Args:  rejectExtraArgs,
		Short: "One renewal queue of FortiGuard, FortiCare and service licences across the fleet, sorted by days left",
		Long: strings.Trim(`
Use this command for FortiGuard and FortiCare licence expiry as a fleet renewal
queue. Do NOT use this command for certificate expiry; use 'fleet health'
instead. Do NOT use it for the all-checks posture matrix; use 'fleet audit'
instead.

Reads monitor license/status (expiry as Unix epoch seconds, including the
FortiCare support contracts) and system/status (serial, model). Expired is
FAIL, expiring inside --within is WARN. queue lists every dated licence,
soonest first.`, "\n"),
		Example:     "  fortinet-cli audit licenses --within 60d --agent\n  fortinet-cli audit licenses --within 30d --select queue,findings",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit licenses")
			}
			if err := validFailOn(ao.failOn); err != nil {
				return err
			}
			within, err := ao.withinDuration()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(cmd.Context()) // --timeout bounds each request, not the fleet: see fanOut
			defer cancel()
			targets, failures, err := loadFleet(flags, fo)
			if err != nil {
				return err
			}
			type out struct {
				rows     []fortios.LicenseRow
				findings []fortios.Finding
			}
			per, fails := fanOut(ctx, targets, fo.concurrency, func(ctx context.Context, t fleetTarget) (out, error) {
				c, err := t.client(flags)
				if err != nil {
					return out{}, err
				}
				rows, findings := collectLicenses(ctx, c, t, within)
				return out{rows, findings}, nil
			})
			failures = append(failures, fails...)
			view := licenseView{Queue: []fortios.LicenseRow{}, Findings: []fortios.Finding{}, FetchFailures: failures}
			for _, p := range per {
				view.Queue = append(view.Queue, p.rows...)
				for _, f := range p.findings {
					if f.Status != fortios.Pass {
						view.Findings = append(view.Findings, f)
					}
				}
			}
			for _, f := range failures {
				view.Findings = append(view.Findings, fortios.Finding{Device: f.ID, Check: "licenses", Status: fortios.Unknown, Code: "unreachable", Detail: f.Error})
			}
			sort.SliceStable(view.Queue, func(i, j int) bool { return view.Queue[i].DaysLeft < view.Queue[j].DaysLeft })
			warnUnchecked(cmd, view.Findings, len(targets)+len(failures)-len(fails))
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "vdom", "object", "code", "check"); err != nil {
					return err
				}
				return failOnErr(ao.failOn, view.Findings)
			}
			for _, r := range view.Queue {
				fmt.Fprintf(cmd.OutOrStdout(), "%-5s %5dd  %-10s %-24s %-28s %s\n", r.Grade, r.DaysLeft, r.Expires, r.Device, r.Service, r.Serial)
			}
			inQueue := map[string]bool{}
			for _, r := range view.Queue {
				inQueue[r.Device+"|"+r.Service] = true
			}
			for _, f := range view.Findings {
				if !inQueue[f.Device+"|"+f.Object] {
					fmt.Fprintf(cmd.OutOrStdout(), "%-7s %-24s %s\n", f.Status, f.Device, f.Detail)
				}
			}
			return failOnErr(ao.failOn, view.Findings)
		},
	}
	addFleetFlags(cmd, &fo)
	cmd.Flags().StringVar(&ao.within, "within", "60d", "Warn for licences expiring inside this window (e.g. 30d, 8w)")
	addFailOnFlag(cmd, &ao)
	return cmd
}
