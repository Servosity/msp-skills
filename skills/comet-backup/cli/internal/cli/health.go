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

func newNovelHealthCmd(flags *rootFlags) *cobra.Command {
	var flagState, flagGrace, flagMaxAge, flagTenant string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "health",
		Short: "See the backup state of every protected item across every tenant in one list",
		Long: `See the backup state of every protected item across every tenant in one list.

Use this command for the current backup state of every protected item across all tenants (ok, warning, failed, overdue, never-run), including items that silently stopped running. Do NOT use this command for the error lines of failed jobs in a time window; use 'triage' instead. Do NOT use it for what changed since the last snapshot; use 'changes' instead.

States: failed (last job 7000/7002-7007), warning (last job 7001), never (no successful backup and a scheduled run was due), overdue (a run its backup-rule schedules called for, after the last success, is more than --grace in the past), running, pending (new item, no run due yet), ok. Scheduled runs are worked out in the device's own timezone with the schedule's day and time-window restrictions; when the device reports no timezone, unrestricted schedules are checked across every UTC offset, and schedules restricted to certain days or hours are reported with schedule_unresolved=true and never flagged overdue (--state unresolved lists them). Yearly schedules are not checked. An item that is both failed and overdue has state failed and overdue=true; --state overdue matches it. Items with no recurring schedule use --unscheduled-interval.`,
		Example: strings.Trim(`
  comet-backup-cli health --state failed,overdue,never --agent
  comet-backup-cli health --tenant acme --json
  comet-backup-cli health --state problem --grace 12h --select tenant,device,item,state,hours_since_success`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: guardRunE(flags, func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "health")
			}
			want := csvSet(flagState)
			if err := validateChoices("state", want, "ok", "running", "pending", "warning", "failed", "overdue", "never", "problem", "unresolved"); err != nil {
				return err
			}
			grace, err := durationFlag("grace", flagGrace)
			if err != nil {
				return err
			}
			maxAge, err := durationFlag("unscheduled-interval", flagMaxAge)
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
			all := comet.Health(f.profiles, f.live, comet.HealthOptions{Now: time.Now(), Grace: grace, MaxAge: maxAge})
			rows := make([]comet.HealthRow, 0, len(all))
			for _, r := range all {
				if flagTenant != "" && !strings.EqualFold(r.Tenant, flagTenant) {
					continue
				}
				if !comet.MatchState(r, want) {
					continue
				}
				rows = append(rows, r)
				if flagLimit > 0 && len(rows) >= flagLimit {
					break
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), rows, flags, healthAgentKeep...)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No protected items match.")
				return nil
			}
			rows2 := make([][]string, 0, len(rows))
			for _, r := range rows {
				since := "never"
				if r.HoursSinceSuccess != nil {
					since = fmt.Sprintf("%.1fh", *r.HoursSinceSuccess)
				}
				rows2 = append(rows2, []string{r.Tenant, r.Device, r.Item, r.State, r.LastStatusName, since, fmt.Sprint(r.Online)})
			}
			return printCleanTable(cmd, flags, []string{"TENANT", "DEVICE", "ITEM", "STATE", "LAST", "SINCE_SUCCESS", "ONLINE"}, rows2)
		}),
	}
	cmd.Flags().StringVar(&flagState, "state", "", "Comma-separated states to keep: failed, warning, overdue, never, running, pending, ok, problem (failed+warning+overdue+never), or unresolved (schedule could not be checked); empty keeps all")
	cmd.Flags().StringVar(&flagGrace, "grace", fleetDefaultGrace, "How late a scheduled run may be before the item counts as overdue (e.g. 6h, 1d)")
	cmd.Flags().StringVar(&flagMaxAge, "unscheduled-interval", fleetDefaultMaxAge, "Expected interval for protected items with no recurring schedule (e.g. 48h, 7d)")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only this tenant (Comet user account username)")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}
