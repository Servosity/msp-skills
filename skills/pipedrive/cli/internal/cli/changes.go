// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: since-window change feed. Hand-authored against the local store.

package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"pipedrive-pp-cli/internal/pipeintel"
)

type changedItem struct {
	ID         string `json:"id"`
	UpdateTime string `json:"update_time"`
	Label      string `json:"label,omitempty"`
}

type changedEntity struct {
	Entity       string        `json:"entity"`
	ChangedCount int           `json:"changed_count"`
	Items        []changedItem `json:"items,omitempty"`
}

type changesResult struct {
	Since        string          `json:"since"`
	Cutoff       string          `json:"cutoff"`
	TotalChanged int             `json:"total_changed"`
	Entities     []changedEntity `json:"entities"`
}

// changeEntity pairs a store table with the column to show as its label.
type changeEntity struct{ table, label string }

var changeEntities = []changeEntity{
	{"deals", "title"},
	{"persons", "name"},
	{"organizations", "name"},
	{"activities", "subject"},
	{"products", "name"},
	{"leads", "title"},
	{"notes", "content"},
}

// pp:data-source local
func newNovelChangesCmd(flags *rootFlags) *cobra.Command {
	var since string
	var entityFilter string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Everything whose update time moved since a timestamp, grouped by entity.",
		Long:  "Use this command for a machine-readable per-entity diff (records whose update_time moved since a timestamp) to feed downstream automation.\nDo NOT use this command for a human standup summary; use 'digest' instead.",
		Example: strings.Trim(`
  pipedrive-cli changes --since 24h
  pipedrive-cli changes --since 7d --entity deals,persons --json
  pipedrive-cli changes --since 2026-05-01 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			cutoff, err := pipeintel.ParseSince(since, time.Now().UTC())
			if err != nil {
				return usageErr(fmt.Errorf("--since: %w", err))
			}
			cutoffStr := cutoff.Format("2006-01-02 15:04:05")

			want := map[string]bool{}
			for _, e := range strings.Split(entityFilter, ",") {
				e = strings.TrimSpace(e)
				if e != "" {
					want[e] = true
				}
			}

			db, err := pdOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			res := changesResult{Since: since, Cutoff: cutoffStr, Entities: []changedEntity{}}
			for _, ent := range changeEntities {
				if len(want) > 0 && !want[ent.table] {
					continue
				}
				if !tableHasColumn(cmd.Context(), db.DB(), ent.table, "update_time") {
					continue
				}
				labelExpr := "''"
				if tableHasColumn(cmd.Context(), db.DB(), ent.table, ent.label) {
					labelExpr = fmt.Sprintf("COALESCE(%q,'')", ent.label)
				}

				var count int
				if err := db.DB().QueryRowContext(cmd.Context(),
					fmt.Sprintf("SELECT COUNT(*) FROM %q WHERE update_time IS NOT NULL AND update_time >= ?", ent.table),
					cutoffStr).Scan(&count); err != nil {
					return fmt.Errorf("counting %s changes: %w", ent.table, err)
				}
				ce := changedEntity{Entity: ent.table, ChangedCount: count, Items: []changedItem{}}
				if count > 0 {
					q := fmt.Sprintf(`SELECT id, COALESCE(update_time,''), %s FROM %q
						WHERE update_time IS NOT NULL AND update_time >= ?
						ORDER BY update_time DESC`, labelExpr, ent.table)
					if limit > 0 {
						q += fmt.Sprintf(" LIMIT %d", limit)
					}
					rows, err := db.DB().QueryContext(cmd.Context(), q, cutoffStr)
					if err != nil {
						return fmt.Errorf("listing %s changes: %w", ent.table, err)
					}
					for rows.Next() {
						var it changedItem
						if err := rows.Scan(&it.ID, &it.UpdateTime, &it.Label); err != nil {
							_ = rows.Close()
							return fmt.Errorf("scanning %s change: %w", ent.table, err)
						}
						ce.Items = append(ce.Items, it)
					}
					_ = rows.Close()
					if err := rows.Err(); err != nil {
						return err
					}
				}
				res.Entities = append(res.Entities, ce)
				res.TotalChanged += count
			}

			return emitNovel(cmd, flags, res, func(w io.Writer) {
				fmt.Fprintf(w, "%d record(s) changed since %s (%s):\n\n", res.TotalChanged, since, cutoffStr)
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "ENTITY\tCHANGED")
				for _, e := range res.Entities {
					fmt.Fprintf(tw, "%s\t%d\n", e.Entity, e.ChangedCount)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Window or timestamp: 24h, 7d, 2w, or 2026-05-01")
	cmd.Flags().StringVar(&entityFilter, "entity", "", "Comma-separated entities to include (default: all)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum items per entity in the listing (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}
