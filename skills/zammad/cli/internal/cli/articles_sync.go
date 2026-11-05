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
			// A preview never opens the store for writing: opening it
			// read-write creates and migrates the file (Codex round 1 [P2]).
			if dryRunOK(flags) {
				n := 0
				if ro, err := store.OpenReadOnlyContext(cmd.Context(), defaultNovelDBPath(dbPath)); err == nil {
					if ids, err := ro.ListIDs("tickets"); err == nil {
						n = len(articlesSyncTicketOrder(ids, limit))
					}
					ro.Close()
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "would sync articles for %d tickets\n", n)
				return nil
			}
			// Like sync and workflow archive, it saves API data locally (Codex r1 [P2]).
			if localDataDisabled() {
				return fmt.Errorf("articles sync is disabled while %s=1: it would save API data to the local store", noLocalDataEnvVar)
			}
			db, err := store.OpenWithContext(cmd.Context(), defaultNovelDBPath(dbPath))
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'zammad-cli sync' first to populate tickets.", err)
			}
			defer db.Close()

			ticketIDs, err := db.ListIDs("tickets")
			if err != nil {
				return fmt.Errorf("listing ticket ids: %w", err)
			}
			allTickets := len(ticketIDs)
			ticketIDs = articlesSyncTicketOrder(ticketIDs, limit)
			// The checkpoint may say complete only when this walk covers every
			// local ticket: --limit (or any other cut) leaves it unfinished, so
			// the article commands refuse a store they would read partially
			// (Codex round 2).
			truncated := len(ticketIDs) < allTickets
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
			// The articles checkpoint reads "unfinished" until every ticket
			// was fetched and stored, so an interrupted or failed run is never
			// mistaken for a complete one by escalate, churn-risk or
			// feedback-scan (novelRequireSynced).
			if err := db.SaveSyncProgress("articles", "", 0); err != nil {
				return fmt.Errorf("recording articles sync progress: %w", err)
			}
			for i, storageID := range ticketIDs {
				ticketID := store.BareResourceID(storageID)
				fmt.Fprintf(cmd.ErrOrStderr(), "syncing articles for ticket %s (%d/%d)\n", ticketID, i+1, len(ticketIDs))
				// A sync reads Zammad, never the response cache (Codex r1 [P2]).
				data, err := c.GetNoCache(cmd.Context(), "/ticket_articles/by_ticket/"+url.PathEscape(ticketID), nil)
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
				// The answer is this ticket's complete article list, so every
				// stored article it no longer names was removed upstream. The
				// sweep runs only when every listed article carried an id;
				// otherwise the seen set is not proven complete.
				seen := make([]string, 0, len(articles))
				complete := articles != nil // a bare null is not a complete list (Codex round 2)
				for _, article := range articles {
					if obj, err := store.DecodeJSONObject(article); err == nil {
						if id := store.ResolveStorageID("articles", obj); id != "" {
							seen = append(seen, id)
						} else {
							complete = false
						}
					} else {
						complete = false
					}
					if err := db.UpsertArticles(article); err != nil {
						summary.FetchFailures = append(summary.FetchFailures, articlesSyncFailure{TicketID: ticketID, Error: "upserting article: " + err.Error()})
						continue
					}
					summary.ArticlesUpserted++
				}
				if complete {
					removed, err := db.ReconcileTicketArticles(ticketID, seen)
					if err != nil {
						summary.FetchFailures = append(summary.FetchFailures, articlesSyncFailure{TicketID: ticketID, Error: "removing deleted articles: " + err.Error()})
						continue
					}
					summary.ArticlesRemoved += removed
				}
			}
			if len(summary.FetchFailures) == 0 && !truncated {
				if err := db.SaveSyncState("articles", "", summary.ArticlesUpserted); err != nil {
					return fmt.Errorf("recording articles sync completion: %w", err)
				}
			}
			if truncated {
				fmt.Fprintf(cmd.ErrOrStderr(), "articles synced for %d of %d local tickets; escalate, churn-risk and feedback-scan need a run that covers every ticket (no --limit)\n", len(ticketIDs), allTickets)
			}
			if err := printJSONFiltered(cmd.OutOrStdout(), summary, flags); err != nil {
				return err
			}
			// A partial run is a failure, not a clean result: the summary
			// names the tickets, and the exit code says the store is not
			// complete (the article commands refuse it until a clean run).
			if n := len(summary.FetchFailures); n > 0 {
				return fmt.Errorf("articles sync did not finish: %d failure(s) across %d ticket(s); see fetch_failures, then run 'zammad-cli articles sync' again", n, len(ticketIDs))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum tickets to process (0 means all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

type articlesSyncSummary struct {
	TicketsScanned   int                   `json:"tickets_scanned"`
	ArticlesUpserted int                   `json:"articles_upserted"`
	ArticlesRemoved  int                   `json:"articles_removed"`
	FetchFailures    []articlesSyncFailure `json:"fetch_failures"`
}

// articlesSyncTicketOrder is the ticket set one articles sync covers: newest
// ticket id first, cut to limit (0 means all), and to 25 under dogfood.
func articlesSyncTicketOrder(ticketIDs []string, limit int) []string {
	ids := append([]string(nil), ticketIDs...)
	sort.SliceStable(ids, func(i, j int) bool {
		left := store.BareResourceID(ids[i])
		right := store.BareResourceID(ids[j])
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
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	if cliutil.IsDogfoodEnv() && len(ids) > 25 {
		ids = ids[:25]
	}
	return ids
}

type articlesSyncFailure struct {
	TicketID string `json:"ticket_id"`
	Error    string `json:"error"`
}
