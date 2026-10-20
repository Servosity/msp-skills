// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. abb device: one machine's backup status by host name.
// pp:data-source local

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"synology-pp-cli/internal/fleet"
)

func newNovelAbbDeviceCmd(flags *rootFlags) *cobra.Command {
	var r fleetReport
	var runs int
	cmd := &cobra.Command{
		Use:   "device [host-name]",
		Short: "Find a machine by host name across every NAS: protecting task, last success and recent runs",
		Long: strings.Trim(`
Use this command for one machine's backup status looked up by host name. Do
NOT use this command for a fleet-wide list of under-protected machines; use
'abb stale' instead.

Matches are case-insensitive substrings of the host name, so a partial name
returns every candidate. An unknown name prints an empty list and exits 3; a NAS whose
mirror is not fresh adds a nas_not_synced row (exit 6), because "not found"
from a stale mirror is not an answer.`, "\n"),
		Example:     "  synology-cli abb device BOB-LAPTOP --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "host=example", "pp:typed-exit-codes": "0,3,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "abb device")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a host name (or part of one) is required"))
			}
			return r.run(cmd, flags, "no protected device matches "+strings.ReplaceAll(args[0], "%", "%%")+" on %d NAS",
				func(ctx context.Context, db fleet.Querier, nas []fleet.NAS, maxSyncAge time.Duration) (any, int, error) {
					rows, err := fleet.FindDevice(ctx, db, nas, args[0], runs, time.Now(), maxSyncAge)
					devices, unanswered := 0, 0
					for _, m := range rows {
						switch m.Kind {
						case "device":
							devices++
						case "nas_not_synced", "abb_missing":
							unanswered++
						}
					}
					if err == nil && devices == 0 && unanswered == 0 {
						return rows, noMatch, nil // "is X protected?" must not read as yes
					}
					return rows, unanswered, err
				})
		},
	}
	r.bind(cmd, true)
	cmd.Flags().IntVar(&runs, "runs", 5, "Recent per-device runs to include")
	return cmd
}
