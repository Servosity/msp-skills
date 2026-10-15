// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/client"
	"quotewerks-pp-cli/internal/cliutil"
	"quotewerks-pp-cli/internal/qw"
	"quotewerks-pp-cli/internal/store"
)

// mirrorItemBatch is how many DocRecGUIDs go into one DocumentItems "in" filter.
const mirrorItemBatch = 50

func toRawRows(rows []map[string]any) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// mirrorScope is what one documents mirror covered, so the next run only
// calls a document "new" when this run would have fetched it.
type mirrorScope struct {
	ID       string   `json:"id"`
	Since    string   `json:"since,omitempty"`
	Types    []string `json:"types,omitempty"`
	Complete bool     `json:"complete"`
}

func (s mirrorScope) covers(doc map[string]any) bool {
	if !s.Complete {
		return false
	}
	if len(s.Types) > 0 && !docTypeIs(doc, upperCSVSet(strings.Join(s.Types, ","))) {
		return false
	}
	if s.Since != "" {
		cutoff, ok := qw.ParseDate(s.Since)
		return ok && inWindow(doc, cutoff, true)
	}
	return true
}

func newNovelMirrorCmd(flags *rootFlags) *cobra.Command {
	var since, types, only, dbPath string
	var maxPages int

	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Copy quotes, line items and product databases into the local database for offline analysis",
		Long: `Copy DocumentHeaders, the DocumentItems of those documents, and every
Products_<Db>_Products table into the local SQLite mirror that margin, winrate, pipeline,
cost-drift, changes and 'docs lint --data-source local' read. Each run is a full refresh
of the selected window (QuoteWerks documents no change cursor) and is committed in one
transaction: stored rows are replaced, not merged, so values cleared in QuoteWerks clear
here too. Lines and products that disappeared upstream are removed for every document
and product database read in full. Before overwriting, the previous copy of each document
is compared with the new one and the differences are recorded for 'changes'; a document
is reported removed only after an unfiltered full run and a direct lookup that returns
not found.`,
		Example: strings.Trim(`
  quotewerks-cli mirror --since 180d
  quotewerks-cli mirror --types QUOTE,ORDER --json
  quotewerks-cli mirror --only products`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:local-write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "mirror")
			}
			want := map[string]bool{"documents": true, "items": true, "products": true}
			if only != "" {
				want = map[string]bool{}
				for _, p := range strings.Split(only, ",") {
					p = strings.ToLower(strings.TrimSpace(p))
					if p != "documents" && p != "items" && p != "products" {
						return usageErr(fmt.Errorf("--only %q: use documents, items or products", p))
					}
					want[p] = true
				}
			}
			now := time.Now()
			cutoff, hasCutoff, err := sinceCutoff(since, now)
			if err != nil {
				return err
			}
			typeSet := upperCSVSet(types)
			if err := validateDocTypes(typeSet, "--types"); err != nil {
				return err
			}
			if maxPages <= 0 {
				return usageErr(fmt.Errorf("--max-pages must be positive"))
			}
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			c.NoCache = true
			// No whole-command deadline: the client already bounds each request
			// with --timeout, and a mirror makes many requests.
			ctx := cmd.Context()
			db, err := store.OpenWithContext(ctx, mirrorDBPath(dbPath))
			if err != nil {
				return fmt.Errorf("opening mirror: %w", err)
			}
			defer db.Close()

			prevRows, err := loadMirror(db, qwResDocuments)
			if err != nil {
				return err
			}
			prev := make(map[string]map[string]any, len(prevRows))
			for _, r := range prevRows {
				prev[qw.Str(r["id"])] = r
			}
			existingItems, err := loadMirror(db, qwResLineItems)
			if err != nil {
				return err
			}
			itemsByDoc := map[string][]string{}
			for _, it := range existingItems {
				d := qw.Str(it["DocRecGUID"])
				itemsByDoc[d] = append(itemsByDoc[d], qw.Str(it["id"]))
			}

			var writes []store.MirrorWrite
			add := func(w store.MirrorWrite, rows []map[string]any) error {
				raw, err := toRawRows(rows)
				if err != nil {
					return err
				}
				w.Rows = raw
				writes = append(writes, w)
				return nil
			}
			summary := map[string]any{"db": mirrorDBPath(dbPath)}
			var docIDs []string
			removedDocs := map[string]bool{}

			if want["documents"] {
				filters := make([]map[string]any, 0)
				if hasCutoff {
					filters = append(filters, qw.Cond("DocDate", "gte", qw.FilterDate(cutoff)))
				}
				if len(typeSet) > 0 {
					vals := make([]any, 0, len(typeSet))
					for t := range typeSet {
						vals = append(vals, t)
					}
					filters = append(filters, qw.Cond("DocType", "in", vals))
				}
				rows, complete, err := qwSearch(ctx, c, qw.TableHeaders, qwSearchOpts{filters: filters, sort: "ID", maxPages: maxPages})
				if err != nil {
					return asUsage(err)
				}
				docs := make([]map[string]any, 0, len(rows))
				for _, r := range rows {
					docs = append(docs, r.Flat())
					docIDs = append(docIDs, r.ID)
				}
				var inPrevScope func(map[string]any) bool
				hasBaseline := false // a previous documents mirror ran, even if it stored nothing
				if scopes, err := loadMirror(db, qwResMirrorScope); err != nil {
					return err
				} else {
					for _, sr := range scopes {
						if qw.Str(sr["id"]) != "documents" {
							continue
						}
						hasBaseline = true
						var prevScope mirrorScope
						b, _ := json.Marshal(sr)
						if json.Unmarshal(b, &prevScope) == nil {
							inPrevScope = prevScope.covers
						}
					}
				}
				events := make([]qw.ChangeEvent, 0)
				// Templates, superseded and AutoRecover rows are not business events.
				activePrev := make(map[string]map[string]any, len(prev))
				for id, d := range prev {
					if qw.IsActiveDocument(d) {
						activePrev[id] = d
					}
				}
				activeDocs := make([]map[string]any, 0, len(docs))
				for _, d := range docs {
					if qw.IsActiveDocument(d) {
						activeDocs = append(activeDocs, d)
					}
				}
				if hasBaseline || len(prev) > 0 {
					// Business events come from active documents only.
					events = append(events, qw.DiffDocs(activePrev, activeDocs, false, now, inPrevScope)...)
				}
				// Removal candidates come from every previous header (templates
				// and superseded rows must leave the mirror too), but only after an
				// unfiltered walk that read every page.
				var candidates []string
				if complete && len(filters) == 0 {
					fetched := make(map[string]bool, len(docs))
					for _, d := range docs {
						fetched[qw.Str(d["id"])] = true
					}
					for id := range prev {
						if !fetched[id] {
							candidates = append(candidates, id)
						}
					}
					sort.Strings(candidates)
				}
				for _, id := range candidates {
					// A short walk can miss a row that moved between pages;
					// only a direct 404 proves the document is gone.
					_, err := qwGetRow(ctx, c, qw.TableHeaders, id)
					var ae *client.APIError
					switch {
					case err == nil:
						// still there: it moved between pages
					case errors.As(err, &ae) && ae.StatusCode == 404:
						removedDocs[id] = true
						if qw.IsActiveDocument(prev[id]) {
							events = append(events, qw.NewChangeEvent(prev[id], qw.ChangeRemoved, qw.Str(prev[id]["DocStatus"]), "", now))
						}
					case errors.As(err, &ae):
						return err
					default:
						// An empty successful response (not found, but no 404) is
						// not proof of deletion: keep the row. Anything else, such
						// as a timeout or a malformed body, fails the run.
						var ce *cliError
						if !errors.As(err, &ce) || ce.code != 3 {
							return err
						}
					}
				}
				removed := make([]string, 0, len(removedDocs))
				for id := range removedDocs {
					removed = append(removed, id)
				}
				sort.Strings(removed)
				if err := add(store.MirrorWrite{Resource: qwResDocuments, Delete: removed, Stamp: true, Count: len(rows), Partial: !complete}, docs); err != nil {
					return err
				}
				evRows := make([]map[string]any, 0, len(events))
				for _, e := range events {
					b, _ := json.Marshal(e)
					var m map[string]any
					_ = json.Unmarshal(b, &m)
					evRows = append(evRows, m)
				}
				if err := add(store.MirrorWrite{Resource: qwResDocChanges, Stamp: true, Count: len(evRows)}, evRows); err != nil {
					return err
				}
				scope := mirrorScope{ID: "documents", Complete: complete}
				if hasCutoff {
					scope.Since = qw.FilterDate(cutoff) // the exact DocDate boundary sent to the API
				}
				for t := range typeSet {
					scope.Types = append(scope.Types, t)
				}
				sort.Strings(scope.Types)
				b, _ := json.Marshal(scope)
				var scopeRow map[string]any
				_ = json.Unmarshal(b, &scopeRow)
				if err := add(store.MirrorWrite{Resource: qwResMirrorScope}, []map[string]any{scopeRow}); err != nil {
					return err
				}
				// Lines and coverage of removed documents go with them.
				var goneItems []string
				for _, id := range removed {
					goneItems = append(goneItems, itemsByDoc[id]...)
				}
				writes = append(writes, store.MirrorWrite{Resource: qwResLineItems, Delete: goneItems}, store.MirrorWrite{Resource: qwResItemCoverage, Delete: removed})
				summary["documents"] = len(rows)
				summary["documents_complete"] = complete
				summary["changes"] = len(events)
				summary["removed"] = len(removed)
				summary["baseline"] = !hasBaseline && len(prev) == 0
				if !complete {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: stopped after %d pages of documents; raise --max-pages to mirror the rest\n", maxPages)
				}
			}

			if want["items"] {
				if !want["documents"] {
					for _, r := range prevRows {
						if inWindow(r, cutoff, hasCutoff) && docTypeIs(r, typeSet) {
							docIDs = append(docIDs, qw.Str(r["id"]))
						}
					}
				}
				items := make([]map[string]any, 0)
				coverage := make([]map[string]any, 0, len(docIDs))
				var stale []string
				for start := 0; start < len(docIDs); start += mirrorItemBatch {
					end := min(start+mirrorItemBatch, len(docIDs))
					batch := make([]any, 0, end-start)
					for _, id := range docIDs[start:end] {
						batch = append(batch, id)
					}
					rows, complete, err := qwSearch(ctx, c, qw.TableItems, qwSearchOpts{filters: []map[string]any{qw.Cond("DocRecGUID", "in", batch)}, maxPages: qwAllPages})
					if err != nil {
						return asUsage(err)
					}
					if !complete {
						return apiErr(fmt.Errorf("line items changed while they were being read; run mirror again"))
					}
					fetched := map[string]bool{}
					lineCount := map[string]int{}
					for _, r := range rows {
						it := r.Flat()
						items = append(items, it)
						fetched[r.ID] = true
						lineCount[qw.Str(it["DocRecGUID"])]++
					}
					for _, id := range docIDs[start:end] {
						for _, itemID := range itemsByDoc[id] {
							if !fetched[itemID] {
								stale = append(stale, itemID)
							}
						}
						coverage = append(coverage, map[string]any{"id": id, "lines": lineCount[id], "at": now.UTC().Format(time.RFC3339)})
					}
					if cliutil.IsDogfoodEnv() {
						break
					}
				}
				if err := add(store.MirrorWrite{Resource: qwResLineItems, Delete: stale, Stamp: true, Count: len(items)}, items); err != nil {
					return err
				}
				if err := add(store.MirrorWrite{Resource: qwResItemCoverage, Stamp: true, Count: len(coverage)}, coverage); err != nil {
					return err
				}
				summary["line_items"] = len(items)
				summary["line_items_removed"] = len(stale)
			}

			if want["products"] {
				tables, err := productTables(ctx, c, "")
				if err != nil {
					return err
				}
				existing, err := loadMirror(db, qwResProducts)
				if err != nil {
					return err
				}
				byTable := map[string][]string{}
				for _, p := range existing {
					t := qw.Str(p["source_table"])
					byTable[t] = append(byTable[t], qw.Str(p["id"]))
				}
				listed := map[string]bool{}
				partial := false
				products := make([]map[string]any, 0)
				var stale []string
				dbs := make([]string, 0, len(tables))
				for _, t := range tables {
					listed[t] = true
					dbName, _ := qw.ProductDB(t)
					dbs = append(dbs, dbName)
					rows, complete, err := qwSearch(ctx, c, t, qwSearchOpts{maxPages: maxPages})
					if err != nil {
						return asUsage(err)
					}
					fetched := map[string]bool{}
					for _, r := range rows {
						p := r.Flat()
						p["id"] = t + ":" + r.ID
						p["source_id"] = r.ID
						p["source_table"] = t
						p["product_db"] = dbName
						products = append(products, p)
						fetched[t+":"+r.ID] = true
					}
					if complete {
						for _, id := range byTable[t] {
							if !fetched[id] {
								stale = append(stale, id)
							}
						}
					} else {
						partial = true
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: stopped after %d pages of %s; raise --max-pages to mirror the rest\n", maxPages, t)
					}
				}
				for t, ids := range byTable {
					if !listed[t] {
						stale = append(stale, ids...) // the product database itself is gone
					}
				}
				if err := add(store.MirrorWrite{Resource: qwResProducts, Delete: stale, Stamp: true, Count: len(products), Partial: partial}, products); err != nil {
					return err
				}
				summary["products"] = len(products)
				summary["products_removed"] = len(stale)
				summary["product_dbs"] = dbs
			}

			if err := db.ApplyMirror(ctx, writes); err != nil {
				return fmt.Errorf("saving mirror: %w", err)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), summary, flags)
			}
			for _, k := range []string{"documents", "line_items", "products", "changes", "removed"} {
				if v, ok := summary[k]; ok {
					fmt.Fprintf(cmd.OutOrStdout(), "%-11s %v\n", k, v)
				}
			}
			if b, _ := summary["baseline"].(bool); b {
				fmt.Fprintln(cmd.OutOrStdout(), "first mirror: baseline recorded; 'changes' reports differences from the next run on")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only documents dated within this window, e.g. 180d (DocDate); default all")
	cmd.Flags().StringVar(&types, "types", "", "Only these document types, comma-separated: QUOTE,ORDER,INVOICE")
	cmd.Flags().StringVar(&only, "only", "", "Mirror only some parts, comma-separated: documents,items,products")
	cmd.Flags().IntVar(&maxPages, "max-pages", 50, "Maximum 1000-row pages to read per document or product table")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}
