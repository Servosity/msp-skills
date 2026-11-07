// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `events list` and the read-only MCP tool
// events_list. Recorded in handfixes.json (health-events).
// pp:data-source local

package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/store"
)

func newNovelEventsListCmd(flags *rootFlags) *cobra.Command {
	var since int64
	var eventType string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Poll health-state changes between health runs with a sequence cursor.",
		Long: `Reads the local event table that 'health' fills (no server is contacted):
one windows_server.health.verdict_changed event when a server's verdict
changes between runs, and one windows_server.health.check_changed event per
check whose status changes. Pass the returned cursor as --since next time to
get only newer events. Each event is {eventId, name, timestamp, data, cursor}.`,
		Example: "  windows-server-cli events list --since 0 --agent\n  windows-server-cli events list --since 41 --type " + EventVerdictChanged + " --agent",
		Args:    cobra.NoArgs,
		Annotations: map[string]string{
			"mcp:read-only": "true", "pp:data-source": "local",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if eventType != "" && eventType != EventVerdictChanged && eventType != EventCheckChanged {
				return usageErr(fmt.Errorf("--type must be %s or %s", EventVerdictChanged, EventCheckChanged))
			}
			if since < 0 {
				return usageErr(fmt.Errorf("--since must be 0 or a cursor from a previous events list"))
			}
			if limit < 1 || limit > 1000 {
				return usageErr(fmt.Errorf("--limit must be from 1 to 1000"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "events list from the local store")
			}
			events := make([]map[string]any, 0)
			cursor, more := since, false
			db, err := store.OpenReadOnlyContext(cmd.Context(), defaultDBPath("windows-server-cli"))
			if err == nil {
				defer db.Close()
				var rows []store.VendorEvent
				rows, more, err = db.VendorEventsSince(cmd.Context(), since, eventType, limit)
				if err != nil {
					return err
				}
				for _, e := range rows {
					cursor = e.Seq
					events = append(events, map[string]any{"eventId": e.EventID, "name": e.Type, "timestamp": e.OccurredAt,
						"data": e.Data, "cursor": strconv.FormatInt(e.Seq, 10), "source": e.Source})
				}
			}
			return printWinsrv(cmd, flags, map[string]any{"events": events, "cursor": strconv.FormatInt(cursor, 10), "hasMore": more}, "events", "cursor", "hasMore")
		},
	}
	cmd.Flags().Int64Var(&since, "since", 0, "Cursor: return events after this one (0 for all)")
	cmd.Flags().StringVar(&eventType, "type", "", "Only this event type: "+EventVerdictChanged+" or "+EventCheckChanged)
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum events (1 to 1000)")
	return cmd
}
