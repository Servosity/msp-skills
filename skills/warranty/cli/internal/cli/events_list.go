// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (handfixes.json warranty-novels).
// pp:data-source local

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"warranty-pp-cli/internal/store"
	"warranty-pp-cli/internal/warranty"

	"github.com/spf13/cobra"
)

func newNovelEventsListCmd(flags *rootFlags) *cobra.Command {
	var since int64
	var eventType string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Coverage expiring, expired and changed events after a cursor, once each.",
		Long: `Use this command to consume coverage events after a cursor, once each, for
ticketing or quoting. Do NOT use this command for the current list of devices
that expire soon; use 'report' instead.

Reads the local event table that lookup, recheck and apple-pull fill (no
network call). Types: coverage.expiring (coverage ends within the lookup's
--within window), coverage.expired, coverage.changed (a device's coverage lines
changed since the last lookup). Pass the returned cursor as --since next time
to get only newer events. Each event is {eventId, name, timestamp, client,
resource, data, cursor}; client is the device's customer. Event ids are
stable, so a repeated lookup never repeats an event.`,
		Example: strings.Trim(`
  warranty-cli events list --since 0 --agent
  warranty-cli events list --since 41 --type coverage.expiring --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if eventType != "" {
				known := false
				for _, t := range warranty.EventTypes {
					known = known || t == eventType
				}
				if !known {
					return usageErr(fmt.Errorf("unknown --type %q: one of %s", eventType, strings.Join(warranty.EventTypes, ", ")))
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
			db, err := openWarrantyRead(cmd.Context())
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
				if rows, more, err = db.VendorEventsSince(cmd.Context(), since, eventType, limit); err != nil {
					return err
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
	cmd.Flags().Int64Var(&since, "since", 0, "Cursor: return events after this one (0 for all)")
	cmd.Flags().StringVar(&eventType, "type", "", "Only this event: "+strings.Join(warranty.EventTypes, ", "))
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum events (1 to 1000)")
	return cmd
}
