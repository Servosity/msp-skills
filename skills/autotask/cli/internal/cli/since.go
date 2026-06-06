// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelSinceCmd reports what changed across tickets in a recent window —
// the "what did I miss" read keyed on lastActivityDate.
// pp:data-source local
func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:   "since [duration]",
		Short: "See what changed across tickets in the last N hours or days.",
		Long:  "List tickets whose lastActivityDate falls within a recent window. Duration accepts forms like 24h, 7d, 2w, 30m, or a bare integer (days). Run `sync` first.",
		Example: strings.Trim(`
  autotask-cli since 24h
  autotask-cli since 7d --agent
  autotask-cli since 2w --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("duration argument is required (e.g. 24h, 7d)"))
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			dur, ok := parseNovelDuration(args[0])
			if !ok {
				return usageErr(fmt.Errorf("invalid duration %q (use forms like 24h, 7d, 2w, 30m, or a bare integer for days)", args[0]))
			}
			cutoff := time.Now().Add(-dur)
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
			type row struct {
				ID           int64  `json:"id"`
				Title        string `json:"title,omitempty"`
				Status       string `json:"status,omitempty"`
				LastActivity string `json:"lastActivityDate,omitempty"`
			}
			var rows []row
			for _, t := range tickets {
				la, ok := timeAt(t, "lastActivityDate", "lastActivityDateTime")
				if !ok || la.Before(cutoff) {
					continue
				}
				id, _ := intAt(t, "id")
				rows = append(rows, row{
					ID:           id,
					Title:        strAt(t, "title"),
					Status:       strAt(t, "status"),
					LastActivity: strAt(t, "lastActivityDate", "lastActivityDateTime"),
				})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].LastActivity > rows[j].LastActivity })
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			out := map[string]any{
				"since":   args[0],
				"cutoff":  cutoff.Format(time.RFC3339),
				"count":   len(rows),
				"tickets": rows,
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}
