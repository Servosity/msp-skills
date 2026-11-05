// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: audit-events actors.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type actorRow struct {
	Actor      string       `json:"actor"`
	ActorType  string       `json:"actor_type,omitempty"`
	Events     int          `json:"events"`
	NotSuccess int          `json:"not_success"`
	Categories []countByKey `json:"categories"`
	Types      []countByKey `json:"types"`
	First      string       `json:"first"`
	Last       string       `json:"last"`
}

type actorsReport struct {
	Start         string         `json:"start"`
	End           string         `json:"end"`
	Total         int            `json:"total"`
	Actors        []actorRow     `json:"actors"`
	ScannedPages  int            `json:"scanned_pages"`
	MaxScanPages  int            `json:"max_scan_pages"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
	Note          string         `json:"note,omitempty"`
}

func newNovelAuditEventsActorsCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath string
	var types []string
	var maxPages int
	cmd := &cobra.Command{
		Use:   "actors",
		Short: "Who made changes over a relative window: one row per actor with event counts, non-success outcomes and categories",
		Long: `Use this command to see who made changes in an organization over a relative window (for example the last 30 days): one row per actor (an admin user or an API account) with how many events, how many did not succeed, and which categories and event types. Do NOT use this command to read the events themselves; use 'audit-events digest' instead.

Reads the same events as 'audit-events digest' (same --since, --type and
--max-pages), then groups them by actor. A blank actor name is filled from the
local users store when it knows the id, else the actor id is shown.
not_success counts events whose outcome is anything but SUCCESS. Rows sort by
event count, most active first.`,
		Example: strings.Trim(`
  apple-business-cli audit-events actors --since 30d --agent
  apple-business-cli audit-events actors --since 7d --type API_ACCOUNT_KEY_GENERATED --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit-events actors")
			}
			ev, partialErr, err := collectAuditEvents(cmd, flags, since, types, maxPages, dbPath)
			if err != nil {
				return err
			}
			rep := actorsReport{Start: ev.Start, End: ev.End, Total: ev.Total, Actors: actorRows(ev.Events),
				ScannedPages: ev.ScannedPages, MaxScanPages: ev.MaxScanPages, FetchFailures: ev.FetchFailures, Note: ev.Note}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), rep, flags); err != nil {
					return err
				}
				return partialErr
			}
			w := cmd.OutOrStdout()
			if rep.Note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: "+rep.Note)
			}
			fmt.Fprintf(w, "%d event(s) by %d actor(s) %s .. %s\n", rep.Total, len(rep.Actors), rep.Start, rep.End)
			for _, a := range rep.Actors {
				cats := make([]string, 0, len(a.Categories))
				for _, c := range a.Categories {
					cats = append(cats, fmt.Sprintf("%s %d", c.Key, c.Count))
				}
				fmt.Fprintf(w, "%-40s %-12s %5d events  %3d not success  %s\n", a.Actor, firstNonEmpty(a.ActorType, "-"), a.Events, a.NotSuccess, strings.Join(cats, ", "))
			}
			return partialErr
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Window ending now (e.g. 24h, 7d, 4w)")
	cmd.Flags().StringArrayVar(&types, "type", nil, "Only this event type (repeatable; one API call each), e.g. DEVICE_ASSIGNED_TO_SERVER")
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, "Maximum 1000-event pages to read per query")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store used to fill blank actor names")
	return cmd
}

// actorRows groups flattened events by actor id (the name is optional on an
// event, so one actor can arrive with and without it), falling back to the
// name, then "(unknown)". The row shows the first real name seen.
func actorRows(events []digestEvent) []actorRow {
	type acc struct {
		row         actorRow
		cats, types map[string]int
	}
	by := map[string]*acc{}
	order := []string{}
	for _, e := range events {
		key := firstNonEmpty(e.ActorID, e.Actor, "(unknown)") + "\x00" + e.ActorType
		a, ok := by[key]
		if !ok {
			a = &acc{row: actorRow{Actor: firstNonEmpty(e.Actor, "(unknown)"), ActorType: e.ActorType}, cats: map[string]int{}, types: map[string]int{}}
			by[key] = a
			order = append(order, key)
		}
		// flattenAuditEvent shows the id when no name is known; a later
		// event that carries the name replaces it.
		if e.Actor != "" && e.Actor != e.ActorID && a.row.Actor == firstNonEmpty(e.ActorID, "(unknown)") {
			a.row.Actor = e.Actor
		}
		a.row.Events++
		if !strings.EqualFold(e.Outcome, "SUCCESS") {
			a.row.NotSuccess++
		}
		a.cats[firstNonEmpty(e.Category, "(none)")]++
		a.types[firstNonEmpty(e.Type, "(none)")]++
		t, ok := parseTime(e.Time)
		if ok {
			if f, fok := parseTime(a.row.First); !fok || t.Before(f) {
				a.row.First = e.Time
			}
			if l, lok := parseTime(a.row.Last); !lok || t.After(l) {
				a.row.Last = e.Time
			}
		}
	}
	rows := make([]actorRow, 0, len(order))
	for _, k := range order {
		a := by[k]
		for c, n := range a.cats {
			a.row.Categories = append(a.row.Categories, countByKey{c, n})
		}
		for t, n := range a.types {
			a.row.Types = append(a.row.Types, countByKey{t, n})
		}
		sortCounts(a.row.Categories)
		sortCounts(a.row.Types)
		rows = append(rows, a.row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Events > rows[j].Events || (rows[i].Events == rows[j].Events && rows[i].Actor < rows[j].Actor)
	})
	return rows
}
