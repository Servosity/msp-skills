// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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

func newNovelChangesCmd(flags *rootFlags) *cobra.Command {
	var since, kinds, rep, docType, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "changes",
		Short: "What moved between mirror runs: new, accepted, lost, re-statused, re-valued or removed documents",
		Long: `List the differences 'mirror' recorded between runs: new documents, QuoteValet
acceptances and losses, DocStatus, SalesRep, DocType and GrandTotal changes, and documents
removed upstream. The first mirror only records a baseline. Reads the local mirror.

Use this command for what moved between mirror runs: new, accepted, lost, re-statused,
re-valued or removed documents. Do NOT use it for the current open pipeline or aging; use
'pipeline' instead.`,
		Example: strings.Trim(`
  quotewerks-cli changes --since 7d --json
  quotewerks-cli changes --kind accepted,lost --rep "Derek Bryan"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "changes")
			}
			cutoff, hasCutoff, err := sinceCutoff(since, time.Now())
			if err != nil {
				return err
			}
			kindSet := map[string]bool{}
			valid := map[string]bool{qw.ChangeNew: true, qw.ChangeAccepted: true, qw.ChangeLost: true, qw.ChangeStatus: true, qw.ChangeValue: true, qw.ChangeRep: true, qw.ChangeType: true, qw.ChangeRemoved: true}
			for _, k := range strings.Split(kinds, ",") {
				if k = strings.ToLower(strings.TrimSpace(k)); k == "" {
					continue
				}
				if !valid[k] {
					return usageErr(fmt.Errorf("--kind %q: use new, accepted, lost, status, value, rep, type or removed", k))
				}
				kindSet[k] = true
			}
			typeSet := upperCSVSet(docType)
			if err := validateDocTypes(typeSet, "--type"); err != nil {
				return err
			}
			ctx := cmd.Context()
			events := make([]map[string]any, 0)
			db, ok, err := openMirror(ctx, cmd, flags, dbPath, qwResDocuments)
			if err != nil {
				return err
			}
			if !ok {
				return noMirrorResult(cmd, flags, map[string]any{"mirrored": false, "filters": filterEcho(cmd), "count": 0, "events": events})
			}
			defer db.Close()
			rows, err := loadMirror(db, qwResDocChanges)
			if err != nil {
				return err
			}
			for _, e := range rows {
				if len(kindSet) > 0 && !kindSet[qw.Str(e["kind"])] {
					continue
				}
				if rep != "" && !strings.EqualFold(qw.Str(e["SalesRep"]), rep) {
					continue
				}
				if len(typeSet) > 0 && !typeSet[strings.ToUpper(qw.Str(e["DocType"]))] {
					continue
				}
				if hasCutoff {
					at, ok := qw.ParseDate(e["at"])
					if !ok || at.Before(cutoff) {
						continue
					}
				}
				events = append(events, e)
			}
			sort.SliceStable(events, func(i, j int) bool {
				a, _ := qw.ParseDate(events[i]["at"])
				b, _ := qw.ParseDate(events[j]["at"])
				if !a.Equal(b) {
					return a.After(b)
				}
				return qw.Str(events[i]["DocNo"]) < qw.Str(events[j]["DocNo"])
			})
			count := len(events)
			if limit > 0 && len(events) > limit {
				events = events[:limit]
			}
			view := map[string]any{"mirrored": true, "filters": filterEcho(cmd), "count": count, "events": events, "partial_resources": partialResources(db, qwResDocuments)}
			if len(rows) == 0 {
				view["note"] = "no changes recorded yet; the first mirror records a baseline and later mirrors record differences"
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(events) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No recorded changes match.")
				return nil
			}
			for _, e := range events {
				from, to := sc(e["from"]), sc(e["to"])
				var what string
				switch {
				case qw.Str(e["kind"]) == qw.ChangeNew:
					what = "new (" + to + ")"
				case qw.Str(e["kind"]) == qw.ChangeRemoved:
					what = "removed (was " + from + ")"
				default:
					what = from + " -> " + to
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %-9s %-10.10s %-24.24s %s\n", sc(e["at"]), sc(e["kind"]), sc(e["DocNo"]), sc(e["SoldToCompany"]), what)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only changes recorded within this window, e.g. 7d")
	cmd.Flags().StringVar(&kinds, "kind", "", "Only these kinds, comma-separated: new, accepted, lost, status, value, rep, type, removed")
	cmd.Flags().StringVar(&rep, "rep", "", "Only documents for this sales rep")
	cmd.Flags().StringVar(&docType, "type", "", "Only these document types, comma-separated: QUOTE, ORDER, INVOICE")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum events to return, newest first")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}
