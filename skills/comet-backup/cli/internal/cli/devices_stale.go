// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/comet"
)

func newNovelDevicesStaleCmd(flags *rootFlags) *cobra.Command {
	var flagOlderThan, flagTenant string
	var flagBillableOnly bool
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Find offline devices: registered devices not connected now, ranked by how long since their last backup",
		Long: `Find offline devices: registered devices not connected now, ranked by how long since their last backup.

Use this command for registered devices that are not connected now, ranked by how long since their last backup, with whether they still carry billable protected items. Do NOT use this command for connected devices on old client versions; use 'devices outdated' instead. Do NOT use it to list every live connection; use 'devices online' instead.

Comet keeps no last-seen time for a device, so the newest job end time across the device's protected items is used instead (days_since_last_job). Devices that never ran a job are listed first.`,
		Example: strings.Trim(`
  comet-backup-cli devices stale --older-than 7d --agent
  comet-backup-cli devices stale --older-than 30d --billable-only --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: guardRunE(flags, func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "devices stale")
			}
			olderThan, err := durationFlag("older-than", flagOlderThan)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			f, err := loadFleet(ctx, c)
			if err != nil {
				return fleetError(cmd, flags, c, err)
			}
			all := comet.Stale(comet.Devices(f.profiles, f.live, time.Now()), olderThan)
			rows := make([]comet.DeviceRow, 0, len(all))
			for _, r := range all {
				if flagTenant != "" && !strings.EqualFold(r.Tenant, flagTenant) {
					continue
				}
				if flagBillableOnly && !r.StillBillable {
					continue
				}
				rows = append(rows, r)
				if flagLimit > 0 && len(rows) >= flagLimit {
					break
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), rows, flags, deviceAgentKeep...)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No offline devices silent for longer than %s.\n", flagOlderThan)
				return nil
			}
			rows2 := make([][]string, 0, len(rows))
			for _, r := range rows {
				days := "never"
				if r.DaysSinceLastJob != nil {
					days = fmt.Sprintf("%.1f", *r.DaysSinceLastJob)
				}
				rows2 = append(rows2, []string{r.Tenant, r.Device, days, fmt.Sprint(r.ProtectedItems), fmt.Sprint(r.StillBillable)})
			}
			return printCleanTable(cmd, flags, []string{"TENANT", "DEVICE", "DAYS_SINCE_JOB", "ITEMS", "BILLABLE"}, rows2)
		}),
	}
	cmd.Flags().StringVar(&flagOlderThan, "older-than", "7d", "Only devices whose newest job ended longer ago than this (e.g. 7d, 30d); devices that never ran a job always match")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only this tenant (Comet user account username)")
	cmd.Flags().BoolVar(&flagBillableOnly, "billable-only", false, "Only devices that still own at least one protected item")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}
