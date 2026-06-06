// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelTicketAgingCmd buckets open tickets by age — a service-desk staleness
// read no single Autotask API call returns.
// pp:data-source local
func newNovelTicketAgingCmd(flags *rootFlags) *cobra.Command {
	var flagBy string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "ticket-aging",
		Short: "Bucket open tickets by age so you see how stale the service desk is at a glance.",
		Long:  "Bucket open tickets by age across the local store. Use --by to break the buckets down by status, queue, or priority. Run `sync` first.",
		Example: strings.Trim(`
  autotask-cli ticket-aging
  autotask-cli ticket-aging --by queue --agent
  autotask-cli ticket-aging --by priority --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			switch flagBy {
			case "", "status", "queue", "priority":
			default:
				return usageErr(fmt.Errorf("invalid --by %q: must be status, queue, or priority", flagBy))
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "tickets") {
				hintIfStale(cmd, db, "tickets", flags.maxAge)
			}
			tickets, err := listEntity(db, "tickets")
			if err != nil {
				return apiErr(err)
			}
			now := time.Now()
			bucketOrder := []string{"0-1d", "1-3d", "3-7d", "7-14d", "14-30d", "30d+"}
			counts := map[string]map[string]int{}
			for _, t := range tickets {
				if !isTicketOpen(t) {
					continue
				}
				created, ok := ticketCreated(t)
				if !ok {
					continue
				}
				bucket := ageBucketLabel(now.Sub(created).Hours())
				group := ""
				switch flagBy {
				case "status":
					group = "status:" + strAt(t, "status")
				case "queue":
					group = "queue:" + strAt(t, "queueID", "queueId")
				case "priority":
					group = "priority:" + strAt(t, "priority")
				}
				if counts[bucket] == nil {
					counts[bucket] = map[string]int{}
				}
				counts[bucket][group]++
			}
			type row struct {
				Bucket string `json:"bucket"`
				Group  string `json:"group,omitempty"`
				Count  int    `json:"count"`
			}
			var rows []row
			for _, b := range bucketOrder {
				groups := counts[b]
				if len(groups) == 0 {
					rows = append(rows, row{Bucket: b, Count: 0})
					continue
				}
				keys := make([]string, 0, len(groups))
				for g := range groups {
					keys = append(keys, g)
				}
				sort.Strings(keys)
				for _, g := range keys {
					rows = append(rows, row{Bucket: b, Group: g, Count: groups[g]})
				}
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&flagBy, "by", "", "break buckets down by: status, queue, or priority")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
