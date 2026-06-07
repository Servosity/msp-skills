// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: fleet-sync.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"afi-pp-cli/internal/syncer"

	"github.com/spf13/cobra"
)

func newNovelFleetSyncCmd(flags *rootFlags) *cobra.Command {
	var flagTenants string
	var flagSkipArchives bool
	var flagPageLimit int
	var flagMaxPages int
	var flagTaskWindowDays int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "fleet-sync",
		Short: "Walk the whole Afi hierarchy (installations → orgs → tenants → resources/protections/policies/archives/quotas/task stats) into the local store",
		Long: strings.TrimSpace(`
The Afi public API exposes exactly one flat list (application installations);
everything else is path-scoped per org or tenant, so the generic 'sync'
command can only cover installations. fleet-sync walks the full hierarchy —
installations, orgs (including child orgs), tenants, then each tenant's
resources, protections, policies, archives, quotas, and a task-status
snapshot — into local SQLite with one respectful, sequential, rate-limited
pass. Every fleet question (coverage-gaps, fleet-health, backup-stale,
reconcile-licenses, tenant-scorecard, resolve) then runs offline.

Use this command to populate or refresh the local fleet store.
Do NOT use this command for the flat installations-only sync; use 'sync' instead.`),
		Example: strings.Trim(`
  # Full fleet walk (the usual first command after auth setup)
  afi-cli fleet-sync

  # Two tenants only, skip the heavy archives walk
  afi-cli fleet-sync --tenants 01F0TENANT00000000000000A,01F0TENANT00000000000000B --skip-archives
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would walk installations -> orgs -> tenants -> per-tenant entities into the local store")
				return nil
			}
			if err := errNoLocalSource(flags, "fleet-sync"); err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			db, err := openAfiStore(flagDB)
			if err != nil {
				return err
			}
			defer db.Close()

			opts := syncer.Options{
				PageLimit:           flagPageLimit,
				MaxPages:            flagMaxPages,
				SkipArchives:        flagSkipArchives,
				TaskStatsWindowDays: flagTaskWindowDays,
			}
			if flagTenants != "" {
				for t := range csvSet(flagTenants) {
					opts.Tenants = append(opts.Tenants, t)
				}
			}
			if !flags.quiet && !flags.asJSON && !flags.agent {
				opts.Progress = cmd.ErrOrStderr()
			}

			sum, err := syncer.Run(cmd.Context(), c, db, opts)
			if err != nil {
				// Emit the partial summary so callers can see how far it got.
				_ = printJSONFiltered(cmd.OutOrStdout(), sum, flags)
				return classifyAPIError(err, flags)
			}
			for _, w := range sum.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}
			return printJSONFiltered(cmd.OutOrStdout(), sum, flags)
		},
	}
	cmd.Flags().StringVar(&flagTenants, "tenants", "", "Comma-separated tenant IDs to sync (default: every tenant the application can see)")
	cmd.Flags().BoolVar(&flagSkipArchives, "skip-archives", false, "Skip the per-tenant archives walk (the heaviest list)")
	cmd.Flags().IntVar(&flagPageLimit, "page-limit", 100, "Per-page limit query value (the server may clamp it)")
	cmd.Flags().IntVar(&flagMaxPages, "max-pages", 50, "Maximum pages fetched per list endpoint (scan cap)")
	cmd.Flags().IntVar(&flagTaskWindowDays, "task-window", 7, "Lookback window in days for the per-tenant task statistics snapshot")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
