// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live
package cli

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"tactical-rmm-pp-cli/internal/cliutil"
)

func newNovelMaintenanceSetCmd(flags *rootFlags) *cobra.Command {
	var filter []string
	var until string
	var clear, execute bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Put a filtered cohort of agents into (or out of) maintenance mode",
		Long: `Put a filtered cohort of agents into maintenance mode. Do NOT use it to run a
script on a cohort; use 'agents bulk-run' instead.

The cohort is selected from the local store with --filter (k=v entries over
client, site, os/plat, status, online). Each matching agent is toggled live via
PUT /agents/{agent_id}/.

Previews the cohort by default (no mutation); pass --execute to apply.

Tactical RMM has no native maintenance-expiry field, so --until does NOT
auto-clear on the server. It is recorded in the preview as a reminder only:
you MUST re-run with --clear (and the same --filter) to take the cohort back
out of maintenance when the window ends.`,
		Example: "  tactical-rmm-cli maintenance set --filter site=HQ\n  tactical-rmm-cli maintenance set --filter client=Acme --until 4h --execute\n  tactical-rmm-cli maintenance set --filter client=Acme --clear --execute",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would preview the maintenance-mode cohort for the given --filter (pass --execute to apply)")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			where, whereArgs := novelAgentFilterWhere(filter)
			ids, hosts := novelSelectCohort(cmd, flags, where, whereArgs)
			action := "enable"
			maint := true
			if clear {
				action = "clear"
				maint = false
			}
			expiry := ""
			if until != "" && !clear {
				if d, err := cliutil.ParseDurationLoose(until); err == nil {
					expiry = time.Now().Add(d).UTC().Format(time.RFC3339)
				} else {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("invalid --until %q: %w", until, err))
				}
			}
			preview := map[string]interface{}{
				"dry_run":          true,
				"action":           action,
				"maintenance_mode": maint,
				"cohort_size":      len(ids),
				"hostnames":        hosts,
			}
			if expiry != "" {
				preview["expires_hint"] = expiry
				preview["expiry_note"] = "Tactical RMM has no server-side expiry; re-run with --clear to end maintenance."
			}
			if !execute || cliutil.IsVerifyEnv() {
				if cliutil.IsVerifyEnv() {
					preview["note"] = fmt.Sprintf("would %s maintenance on %d agents (verify mode: no network)", action, len(ids))
				} else {
					preview["note"] = "preview only; pass --execute to apply"
				}
				return flags.printJSON(cmd, preview)
			}
			if len(ids) == 0 {
				return fmt.Errorf("no agents matched the filter")
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			type result struct {
				AgentID string `json:"agent_id"`
				OK      bool   `json:"ok"`
				Error   string `json:"error,omitempty"`
			}
			results := make([]result, 0, len(ids))
			ok := 0
			var firstErr string
			for _, id := range ids {
				id = strings.TrimSpace(id)
				if id == "" {
					if firstErr == "" {
						firstErr = "empty agent id skipped"
					}
					results = append(results, result{AgentID: id, OK: false, Error: "empty agent id skipped"})
					continue
				}
				body := map[string]interface{}{"maintenance_mode": maint}
				if _, _, perr := c.Put(cmd.Context(), "/agents/"+url.PathEscape(id)+"/", body); perr != nil {
					if firstErr == "" {
						firstErr = perr.Error()
					}
					results = append(results, result{AgentID: id, OK: false, Error: perr.Error()})
					continue
				}
				ok++
				results = append(results, result{AgentID: id, OK: true})
			}
			if err := flags.printJSON(cmd, map[string]interface{}{
				"action":      action,
				"applied":     ok,
				"cohort_size": len(ids),
				"results":     results,
			}); err != nil {
				return err
			}
			if len(ids) > 0 && ok == 0 {
				return fmt.Errorf("all %d maintenance PUTs failed: %s", len(ids), firstErr)
			}
			if ok < len(ids) {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d PUTs failed\n", len(ids)-ok, len(ids))
				return fmt.Errorf("%d of %d maintenance PUTs failed: %s", len(ids)-ok, len(ids), firstErr)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&filter, "filter", nil, "Cohort filter, e.g. client=Acme,site=HQ,os=windows,online=true")
	cmd.Flags().StringVar(&until, "until", "", "Reminder window (e.g. 4h, 1d). NOT server-enforced; re-run with --clear to end maintenance")
	cmd.Flags().BoolVar(&clear, "clear", false, "Take the cohort OUT of maintenance mode")
	cmd.Flags().BoolVar(&execute, "execute", false, "Actually apply (default: preview only)")
	return cmd
}
