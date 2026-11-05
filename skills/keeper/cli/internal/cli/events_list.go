// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix posture-events).
// pp:data-source local

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"keeper-pp-cli/internal/keeper"
	"keeper-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

func newNovelEventsListCmd(flags *rootFlags) *cobra.Command {
	var since int64
	var eventType string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Read posture change events after a cursor, once each",
		Long: `Use this command to read change events incrementally by sequence cursor for automation.
Do NOT use this command for a human-readable comparison of two snapshots; use 'posture diff' instead.

Reads the local event table that 'posture report' fills (no network call). Pass the returned
cursor as --since next time to get only newer events. Each event is {eventId, name, timestamp,
client, resource, data, cursor}; event ids are stable, so an event is never repeated.`,
		Example: strings.Trim(`
  keeper-cli events list --since 0 --agent
  keeper-cli events list --since 41 --type user.twofa_disabled --json`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if eventType != "" {
				known := false
				for _, t := range keeper.EventTypes {
					known = known || t == eventType
				}
				if !known {
					return usageErr(fmt.Errorf("unknown --type %q: one of %s", eventType, strings.Join(keeper.EventTypes, ", ")))
				}
			}
			if since < 0 {
				return usageErr(fmt.Errorf("--since must be 0 or a cursor returned by a previous call"))
			}
			if limit < 1 || limit > 1000 {
				return usageErr(fmt.Errorf("--limit must be between 1 and 1000"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "events list")
			}
			rows := []store.VendorEvent{}
			more := false
			db, err := openStoreForRead(cmd.Context(), "keeper-cli")
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
				if rows, more, err = db.VendorEventsSince(cmd.Context(), since, eventType, limit); err != nil {
					return err
				}
			}
			if since > 0 {
				// A cursor past the newest event (a rebuilt or replaced store)
				// would silently skip every new event: say so on stderr; stdout
				// stays the same JSON.
				var newest int64
				if db != nil {
					if newest, err = db.MaxEventSeq(cmd.Context()); err != nil {
						return err
					}
				}
				if since > newest {
					fmt.Fprintf(cmd.ErrOrStderr(), "note: cursor %d is past the newest event %d; the local store may have been rebuilt; use --since 0\n", since, newest)
				}
			}
			events := make([]map[string]any, 0, len(rows))
			cursor := since
			for _, e := range rows {
				cursor = e.Seq
				events = append(events, map[string]any{"eventId": e.EventID, "name": e.Type, "timestamp": e.OccurredAt,
					"client": e.Client, "resource": e.ResourceID, "data": e.Data, "cursor": strconv.FormatInt(e.Seq, 10)})
			}
			return flags.printJSON(cmd, map[string]any{"events": events, "cursor": strconv.FormatInt(cursor, 10), "hasMore": more})
		},
	}
	cmd.Flags().Int64Var(&since, "since", 0, "Return events after this cursor (0 for the beginning)")
	cmd.Flags().StringVar(&eventType, "type", "", "Only events of this type (e.g. user.twofa_disabled)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum events (1 to 1000)")
	return cmd
}
