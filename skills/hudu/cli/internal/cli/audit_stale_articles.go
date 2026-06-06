// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence feature: knowledge-base staleness audit. Ranks KB
// articles by how long since their last update from the local mirror — a verdict
// competitors surface articles for but never compute.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type staleArticleRow struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	CompanyID   int    `json:"company_id,omitempty"`
	CompanyName string `json:"company_name,omitempty"`
	Draft       bool   `json:"draft"`
	UpdatedAt   string `json:"updated_at"`
	DaysStale   int    `json:"days_stale"`
}

func newNovelAuditStaleArticlesCmd(flags *rootFlags) *cobra.Command {
	var olderThan string
	var flagCompany int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "stale-articles",
		Short:       "Rank knowledge-base articles by how long since they were last updated.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Report knowledge-base articles whose last update predates a threshold, oldest
first, from the local mirror (run 'sync' first). Documentation that hasn't been
touched in a long time likely no longer reflects the live environment.`,
		Example: `  # Articles untouched for a year
  hudu-cli audit stale-articles --older-than 365d

  # For one company, as JSON
  hudu-cli audit stale-articles --older-than 6m --company 42 --agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			threshold, err := parseAgeDays(olderThan)
			if err != nil {
				return usageErr(err)
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openAuditStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "articles") {
				hintIfStale(cmd, db, "articles", flags.maxAge)
			}
			companyNames := loadCompanyNames(cmd.Context(), db)

			q := `SELECT data FROM articles`
			var qargs []any
			if flagCompany > 0 {
				q += ` WHERE company_id = ?`
				qargs = append(qargs, flagCompany)
			}
			rows, err := queryDataRows(cmd.Context(), db, q, qargs...)
			if err != nil {
				return fmt.Errorf("reading articles: %w", err)
			}

			now := time.Now()
			out := []staleArticleRow{}
			for _, raw := range rows {
				var m map[string]any
				if json.Unmarshal(raw, &m) != nil {
					continue
				}
				updated := asString(m["updated_at"])
				days, ok := ageDays(updated, now)
				if !ok || days < threshold {
					continue
				}
				cid := intField(m, "company_id")
				draft := false
				if b, ok := m["draft"].(bool); ok {
					draft = b
				}
				out = append(out, staleArticleRow{
					ID: intField(m, "id"), Name: asString(m["name"]),
					CompanyID: cid, CompanyName: companyNames[cid],
					Draft: draft, UpdatedAt: updated, DaysStale: days,
				})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].DaysStale > out[j].DaysStale })

			return emitAudit(cmd, flags, out, func(w io.Writer) {
				if len(out) == 0 {
					fmt.Fprintf(w, "No articles older than %d days. (Run 'hudu-cli sync' first if unexpected.)\n", threshold)
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "DAYS\tTITLE\tCOMPANY\tDRAFT\tUPDATED")
				for _, r := range out {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%t\t%s\n", r.DaysStale, r.Name, r.CompanyName, r.Draft, r.UpdatedAt)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "365d", "Staleness threshold (e.g. 365d, 52w, 6m, 1y)")
	cmd.Flags().IntVar(&flagCompany, "company", 0, "Limit to a single company id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
