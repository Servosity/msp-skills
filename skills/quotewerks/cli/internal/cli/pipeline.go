// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

type pipelineBucket struct {
	Bucket       string  `json:"bucket"`
	Count        int     `json:"count"`
	Value        float64 `json:"value"`
	UnknownValue int     `json:"unknown_value,omitempty"`
}

type pipelineGroup struct {
	Key     string           `json:"key"`
	Count   int              `json:"count"`
	Value   float64          `json:"value"`
	Buckets []pipelineBucket `json:"buckets"`
}

func roundBuckets(bs []pipelineBucket) {
	for i := range bs {
		bs[i].Value = qw.Round2(bs[i].Value)
	}
}

func newBuckets() []pipelineBucket {
	out := make([]pipelineBucket, 0, len(qw.AgeBuckets)+1)
	for _, b := range qw.AgeBuckets {
		out = append(out, pipelineBucket{Bucket: b})
	}
	return append(out, pipelineBucket{Bucket: "undated"})
}

func addToBuckets(bs []pipelineBucket, bucket string, value float64, known bool) {
	for i := range bs {
		if bs[i].Bucket == bucket {
			bs[i].Count++
			if known {
				bs[i].Value += value // rounded once, in roundBuckets
			} else {
				bs[i].UnknownValue++
			}
			return
		}
	}
}

func newNovelPipelineCmd(flags *rootFlags) *cobra.Command {
	var by, rep, customer, olderThan, wonStatus, lostStatus, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Open quote count and value by age bucket, or the stale quotes to chase, from the local mirror",
		Long: `Summarize open quotes (not won or lost, same classifier as 'winrate') by DocDate age:
0-7, 8-30, 31-60, 61-90 and 90+ days, with count and GrandTotal value, optionally per rep
or customer. With --older-than, list the individual open quotes at least that old with
the contact and QuoteValet state to follow up on, oldest first. Reads the local mirror;
run 'mirror' first.

Use this command for the current open-quote pipeline: count and value by age bucket, and
with --older-than the individual stale quotes to chase. Do NOT use it to see what changed
recently; use 'changes' instead. Do NOT use it for win and loss rates; use 'winrate'
instead.`,
		Example: strings.Trim(`
  quotewerks-cli pipeline --json
  quotewerks-cli pipeline --by rep --json
  quotewerks-cli pipeline --older-than 30d --rep "Derek Bryan"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "pipeline")
			}
			if by != "" {
				if err := validateBy(by, "rep", "customer"); err != nil {
					return err
				}
			}
			var minAge time.Duration
			if olderThan != "" {
				d, err := parseLooseDurationFlag("--older-than", olderThan)
				if err != nil {
					return err
				}
				minAge = d
			}
			now := time.Now()
			ctx := cmd.Context()
			db, ok, err := openMirror(ctx, cmd, flags, dbPath, qwResDocuments)
			if err != nil {
				return err
			}
			listMode := olderThan != ""
			if !ok {
				if listMode {
					return noMirrorResult(cmd, flags, map[string]any{"mirrored": false, "filters": filterEcho(cmd), "count": 0, "value": 0, "quotes": make([]map[string]any, 0)})
				}
				return noMirrorResult(cmd, flags, map[string]any{"mirrored": false, "filters": filterEcho(cmd), "count": 0, "value": 0, "buckets": newBuckets()})
			}
			defer db.Close()
			quotes, err := mirroredDocs(db, map[string]bool{"QUOTE": true}, time.Time{}, false)
			if err != nil {
				return err
			}
			cls := qw.NewClassifier(wonStatus, lostStatus)
			type openQuote struct {
				doc    map[string]any
				age    int
				ageDur time.Duration
				aged   bool
			}
			open := make([]openQuote, 0)
			for _, q := range quotes {
				if outcome, _ := cls.Classify(q); outcome != qw.OutcomeOpen {
					continue
				}
				if rep != "" && !strings.EqualFold(qw.Str(q["SalesRep"]), rep) {
					continue
				}
				if customer != "" && !strings.Contains(strings.ToLower(qw.Str(q["SoldToCompany"])), strings.ToLower(customer)) {
					continue
				}
				oq := openQuote{doc: q}
				if d, ok := qw.ParseDate(q["DocDate"]); ok {
					oq.ageDur = now.Sub(d)
					oq.age, oq.aged = int(oq.ageDur.Hours()/24), true
				}
				open = append(open, oq)
			}
			if listMode {
				rows := make([]map[string]any, 0)
				value := 0.0
				unknownValue := 0
				for _, oq := range open {
					if !oq.aged || oq.ageDur < minAge {
						continue
					}
					row := project(oq.doc, []string{"id", "DocNo", "DocName", "DocDate", "DocStatus", "SoldToCompany", "SoldToContact", "SalesRep", "GrandTotal"})
					row["age_days"] = oq.age
					if label := qw.QuoteValetLabel(oq.doc["QuoteValetStatusCode"]); label != "" {
						row["QuoteValetStatus"] = label
					}
					rows = append(rows, row)
					if v, ok := qw.Num(oq.doc["GrandTotal"]); ok {
						value += v
					} else {
						unknownValue++
					}
				}
				sort.SliceStable(rows, func(i, j int) bool { return rows[i]["age_days"].(int) > rows[j]["age_days"].(int) })
				count := len(rows)
				if limit > 0 && len(rows) > limit {
					rows = rows[:limit]
				}
				view := map[string]any{"mirrored": true, "filters": filterEcho(cmd), "partial_resources": partialResources(db, qwResDocuments), "count": count, "value": qw.Round2(value), "unknown_value": unknownValue, "quotes": rows}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				if len(rows) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "No open quotes older than %s.\n", olderThan)
					return nil
				}
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "%-10s %-10s %5s  %-24s %-18s %-14s %12s  %s\n", "DocNo", "DocDate", "Age", "Customer", "Contact", "Rep", "GrandTotal", "QuoteValet")
				for _, r := range rows {
					date := qw.Str(r["DocDate"])
					if len(date) > 10 {
						date = date[:10]
					}
					total := "         n/a"
					if v, ok := qw.Num(r["GrandTotal"]); ok {
						total = fmt.Sprintf("%12.2f", v)
					}
					fmt.Fprintf(w, "%-10.10s %-10s %5v  %-24.24s %-18.18s %-14.14s %s  %s\n", sc(r["DocNo"]), date, r["age_days"], sc(r["SoldToCompany"]), sc(r["SoldToContact"]), sc(r["SalesRep"]), total, sc(r["QuoteValetStatus"]))
				}
				fmt.Fprintf(w, "%d open quotes, value %.2f\n", count, qw.Round2(value))
				if unknownValue > 0 {
					fmt.Fprintf(w, "note: %d of them have no GrandTotal and add nothing to the value\n", unknownValue)
				}
				return nil
			}
			buckets := newBuckets()
			groups := map[string]*pipelineGroup{}
			count, value, unknownValue := 0, 0.0, 0
			for _, oq := range open {
				b := "undated"
				if oq.aged {
					b = qw.AgeBucket(oq.age)
				}
				v, known := qw.Num(oq.doc["GrandTotal"])
				addToBuckets(buckets, b, v, known)
				count++
				if known {
					value += v
				} else {
					unknownValue++
				}
				if by != "" {
					k := groupKey(oq.doc, by)
					if groups[k] == nil {
						groups[k] = &pipelineGroup{Key: k, Buckets: newBuckets()}
					}
					g := groups[k]
					g.Count++
					if known {
						g.Value += v
					}
					addToBuckets(g.Buckets, b, v, known)
				}
			}
			roundBuckets(buckets)
			for _, g := range groups {
				g.Value = qw.Round2(g.Value)
				roundBuckets(g.Buckets)
			}
			view := map[string]any{"mirrored": true, "filters": filterEcho(cmd), "partial_resources": partialResources(db, qwResDocuments), "count": count, "value": qw.Round2(value), "unknown_value": unknownValue, "buckets": buckets}
			if by != "" {
				gs := make([]pipelineGroup, 0, len(groups))
				for _, g := range groups {
					gs = append(gs, *g)
				}
				sort.Slice(gs, func(i, j int) bool {
					if gs[i].Value != gs[j].Value {
						return gs[i].Value > gs[j].Value
					}
					return gs[i].Key < gs[j].Key
				})
				if limit > 0 && len(gs) > limit {
					gs = gs[:limit]
				}
				view["by"] = by
				view["groups"] = gs
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "open quotes %d  value %.2f\n", count, qw.Round2(value))
			if unknownValue > 0 {
				fmt.Fprintf(w, "note: %d open quotes have no GrandTotal and add nothing to the value\n", unknownValue)
			}
			for _, b := range buckets {
				fmt.Fprintf(w, "  %-8s %5d  %12.2f\n", b.Bucket, b.Count, b.Value)
			}
			if gs, ok := view["groups"].([]pipelineGroup); ok {
				for _, g := range gs {
					fmt.Fprintf(w, "\n%s: %d quotes, value %.2f\n", sc(g.Key), g.Count, g.Value)
					for _, b := range g.Buckets {
						if b.Count > 0 {
							fmt.Fprintf(w, "  %-8s %5d  %12.2f\n", b.Bucket, b.Count, b.Value)
						}
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Also group by: rep or customer")
	cmd.Flags().StringVar(&rep, "rep", "", "Only this sales rep (SalesRep)")
	cmd.Flags().StringVar(&customer, "customer", "", "Only customers whose name contains this text")
	cmd.Flags().StringVar(&olderThan, "older-than", "", "List open quotes at least this old instead of buckets, e.g. 30d")
	cmd.Flags().StringVar(&wonStatus, "won-status", "", "DocStatus values that mean won (excluded from open), comma-separated")
	cmd.Flags().StringVar(&lostStatus, "lost-status", "", "DocStatus values that mean lost (excluded from open), comma-separated")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum quotes (with --older-than) or groups to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}
