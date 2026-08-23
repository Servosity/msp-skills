// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel local article sync for ticket article deep-sync.

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
	"zammad-pp-cli/internal/cliutil"
	"zammad-pp-cli/internal/store"
)

func newNovelArticlesSyncCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync ticket articles for locally synced tickets into the local SQLite store",
		Example: `  zammad-cli sync
  zammad-cli articles sync --limit 100 --json`,
		Annotations: map[string]string{"mcp:local-write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := store.OpenWithContext(cmd.Context(), defaultNovelDBPath(dbPath))
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'zammad-cli sync' first to populate tickets.", err)
			}
			defer db.Close()

			ticketIDs, err := db.ListIDs("tickets")
			if err != nil {
				return fmt.Errorf("listing ticket ids: %w", err)
			}
			sort.SliceStable(ticketIDs, func(i, j int) bool {
				left := store.BareResourceID(ticketIDs[i])
				right := store.BareResourceID(ticketIDs[j])
				leftID, leftErr := strconv.ParseInt(left, 10, 64)
				rightID, rightErr := strconv.ParseInt(right, 10, 64)
				if leftErr == nil && rightErr == nil {
					return leftID > rightID
				}
				if leftErr == nil {
					return true
				}
				if rightErr == nil {
					return false
				}
				return left > right
			})
			if limit > 0 && len(ticketIDs) > limit {
				ticketIDs = ticketIDs[:limit]
			}
			if cliutil.IsDogfoodEnv() && len(ticketIDs) > 25 {
				ticketIDs = ticketIDs[:25]
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.ErrOrStderr(), "would sync articles for %d tickets\n", len(ticketIDs))
				return nil
			}
			if len(ticketIDs) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "No local tickets found; run 'zammad-cli sync' first.")
				return printJSONFiltered(cmd.OutOrStdout(), articlesSyncSummary{
					TicketsScanned:   0,
					ArticlesUpserted: 0,
					FetchFailures:    make([]articlesSyncFailure, 0),
				}, flags)
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			summary := articlesSyncSummary{
				FetchFailures: make([]articlesSyncFailure, 0),
			}
			for i, storageID := range ticketIDs {
				ticketID := store.BareResourceID(storageID)
				fmt.Fprintf(cmd.ErrOrStderr(), "syncing articles for ticket %s (%d/%d)\n", ticketID, i+1, len(ticketIDs))
				data, err := c.Get(cmd.Context(), "/ticket_articles/by_ticket/"+url.PathEscape(ticketID), nil)
				summary.TicketsScanned++
				if err != nil {
					summary.FetchFailures = append(summary.FetchFailures, articlesSyncFailure{TicketID: ticketID, Error: err.Error()})
					continue
				}
				var articles []json.RawMessage
				if err := json.Unmarshal(data, &articles); err != nil {
					summary.FetchFailures = append(summary.FetchFailures, articlesSyncFailure{TicketID: ticketID, Error: "decoding articles: " + err.Error()})
					continue
				}
				for _, article := range articles {
					if err := db.UpsertArticles(article); err != nil {
						summary.FetchFailures = append(summary.FetchFailures, articlesSyncFailure{TicketID: ticketID, Error: "upserting article: " + err.Error()})
						continue
					}
					summary.ArticlesUpserted++
				}
			}
			return printJSONFiltered(cmd.OutOrStdout(), summary, flags)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum tickets to process (0 means all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

type articlesSyncSummary struct {
	TicketsScanned   int                   `json:"tickets_scanned"`
	ArticlesUpserted int                   `json:"articles_upserted"`
	FetchFailures    []articlesSyncFailure `json:"fetch_failures"`
}

type articlesSyncFailure struct {
	TicketID string `json:"ticket_id"`
	Error    string `json:"error"`
}
