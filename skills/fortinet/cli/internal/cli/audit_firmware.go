// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// audit firmware: running FortiOS vs per-branch floors and the newest patch
// the device itself is offered.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/fortios"
)

func newNovelAuditFirmwareCmd(flags *rootFlags) *cobra.Command {
	var fo fleetOpts
	var ao auditOpts
	cmd := &cobra.Command{
		Use:   "firmware",
		Args:  rejectExtraArgs,
		Short: "Flag FortiGates below your per-branch firmware floor or behind the newest patch in their branch",
		Long: strings.Trim(`
Use this command for firmware findings: versions below a floor and patches
behind the newest image in the same branch. Do NOT use this command for the
all-checks posture matrix; use 'fleet audit' instead.

Reads monitor system/status and system/firmware on every device. --min-version
takes one floor per branch (7.2.10,7.4.5): a device below its branch floor is
FAIL, a branch older than every floor is FAIL, and being behind the newest
image the device is offered in its own branch is WARN. With no floor and no
offered images the device is UNKNOWN. For the model's supported upgrade hops
run 'monitor upgrade-paths' against that device.`, "\n"),
		Example:     "  fortinet-cli audit firmware --min-version 7.2.10,7.4.5 --agent\n  fortinet-cli audit firmware --device acme-hq",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit firmware")
			}
			if err := validFailOn(ao.failOn); err != nil {
				return err
			}
			floors, err := ao.floors()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(cmd.Context()) // --timeout bounds each request, not the fleet: see fanOut
			defer cancel()
			targets, failures, err := loadFleet(flags, fo)
			if err != nil {
				return err
			}
			reports, fails := fanOut(ctx, targets, fo.concurrency, func(ctx context.Context, t fleetTarget) (fortios.FirmwareReport, error) {
				c, err := t.client(flags)
				if err != nil {
					return fortios.FirmwareReport{}, err
				}
				return collectFirmware(ctx, c, t, floors).Report, nil
			})
			failures = append(failures, fails...)
			for _, f := range failures {
				reports = append(reports, fortios.FirmwareReport{Device: f.ID, Status: fortios.Unknown, Findings: []fortios.Finding{{Device: f.ID, Check: "firmware", Status: fortios.Unknown, Code: "unreachable", Detail: f.Error}}})
			}
			var all []fortios.Finding
			for _, r := range reports {
				all = append(all, r.Findings...)
			}
			warnUnchecked(cmd, all, len(targets)+len(failures)-len(fails))
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), reports, flags); err != nil {
					return err
				}
				return failOnErr(ao.failOn, all)
			}
			for _, r := range reports {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-9s %-8s floor=%-8s latest=%-8s %s\n", r.Device, r.Current, fortios.Status(r.Findings), dash(r.Floor), dash(r.LatestInBranch), r.Findings[0].Detail)
			}
			return failOnErr(ao.failOn, all)
		},
	}
	addFleetFlags(cmd, &fo)
	cmd.Flags().StringVar(&ao.minVersion, "min-version", "", "Minimum FortiOS per branch, comma-separated (e.g. 7.2.10,7.4.5)")
	addFailOnFlag(cmd, &ao)
	return cmd
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
