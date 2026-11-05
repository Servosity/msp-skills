// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: audit-events digest.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"apple-business-pp-cli/internal/cliutil"
	"apple-business-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type digestEvent struct {
	Time        string `json:"time"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Outcome     string `json:"outcome,omitempty"`
	ActorType   string `json:"actor_type,omitempty"`
	Actor       string `json:"actor,omitempty"`
	ActorID     string `json:"-"` // groups audit-events actors; not in digest output
	SubjectType string `json:"subject_type,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Details     string `json:"details,omitempty"`
}

type digestReport struct {
	Start         string         `json:"start"`
	End           string         `json:"end"`
	Total         int            `json:"total"`
	Categories    []countByKey   `json:"categories"`
	Events        []digestEvent  `json:"events"`
	ScannedPages  int            `json:"scanned_pages"`
	MaxScanPages  int            `json:"max_scan_pages"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
	Note          string         `json:"note,omitempty"`
}

func newNovelAuditEventsDigestCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath string
	var types []string
	var maxPages int
	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Readable change log over a relative window: one flattened row per event, plus per-category counts",
		Long: `Use this command for a readable change log over a relative window (for example the last 7 days): one row per event with its details flattened, newest first, plus per-category counts. Do NOT use this command for an exact-timestamp single-type query; use 'audit-events' with --filter-start-timestamp/--filter-end-timestamp instead.

The API needs absolute start/end timestamps and accepts one event type per call;
--since turns a duration into that window and each repeated --type becomes its
own call. Each event's type-specific payload (eventDataDeviceAssignedToServer
and so on) is flattened into details. Blank actor and subject names are filled
from the local users and devices store when it knows the id.`,
		Example: strings.Trim(`
  apple-business-cli audit-events digest --since 7d --agent
  apple-business-cli audit-events digest --since 30d --type DEVICE_REMOVED_FROM_ORG --type API_ACCOUNT_KEY_GENERATED`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit-events digest")
			}
			rep, partialErr, err := collectAuditEvents(cmd, flags, since, types, maxPages, dbPath)
			if err != nil {
				return err
			}
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
			fmt.Fprintf(w, "%d event(s) %s .. %s\n", rep.Total, rep.Start, rep.End)
			for _, g := range rep.Categories {
				fmt.Fprintf(w, "  %-28s %d\n", g.Key, g.Count)
			}
			for _, e := range rep.Events {
				fmt.Fprintf(w, "%s  %-32s %s -> %s  %s\n", e.Time, e.Type, firstNonEmpty(e.Actor, "-"), firstNonEmpty(e.Subject, "-"), e.Details)
			}
			return partialErr
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Window ending now (e.g. 24h, 7d, 4w)")
	cmd.Flags().StringArrayVar(&types, "type", nil, "Only this event type (repeatable; one API call each), e.g. DEVICE_ASSIGNED_TO_SERVER")
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, "Maximum 1000-event pages to read per query")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store used to fill blank actor/subject names")
	return cmd
}

// collectAuditEvents reads every audit event in the window ending now (one
// call per --type), flattened and newest first, with per-category counts.
// partialErr is set when some but not all type queries failed.
func collectAuditEvents(cmd *cobra.Command, flags *rootFlags, since string, types []string, maxPages int, dbPath string) (digestReport, error, error) {
	window, err := cliutil.ParseDurationLoose(since)
	if err != nil || window <= 0 {
		return digestReport{}, nil, usageErr(fmt.Errorf("--since must be a positive duration such as 24h, 7d or 4w"))
	}
	if cliutil.IsDogfoodEnv() {
		maxPages = 1
	}
	end := time.Now().UTC().Truncate(time.Second)
	start := end.Add(-window)
	rep := digestReport{Start: start.Format(time.RFC3339), End: end.Format(time.RFC3339), Events: make([]digestEvent, 0), Categories: make([]countByKey, 0), MaxScanPages: maxPages, FetchFailures: make([]fetchFailure, 0)}
	// The generated client bounds each request with --timeout; a
	// whole-command deadline would cut multi-page reads short.
	ctx := cmd.Context()
	// One config for the events and the store that names their actors.
	c, cur, err := frozenClient(flags)
	if err != nil {
		return digestReport{}, nil, err
	}
	calls := make([]string, 0, len(types))
	seenType := map[string]bool{}
	for _, t := range types {
		if t = strings.ToUpper(strings.TrimSpace(t)); t != "" && !seenType[t] {
			seenType[t] = true
			calls = append(calls, t)
		}
	}
	if len(calls) == 0 {
		calls = []string{""}
	}
	var raw []json.RawMessage
	var firstErr error
	capped := false
	for _, t := range calls {
		params := map[string]string{"filter[startTimestamp]": rep.Start, "filter[endTimestamp]": rep.End, "limit": "1000"}
		if t != "" {
			params["filter[type]"] = t
		}
		got, pages, more, err := listAllPages(ctx, c, "/auditEvents", params, maxPages)
		rep.ScannedPages += pages
		capped = capped || more
		raw = append(raw, got...)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			rep.FetchFailures = append(rep.FetchFailures, fetchFailure{Kind: firstNonEmpty(t, "all"), Error: err.Error()})
		}
	}
	if len(rep.FetchFailures) == len(calls) {
		return digestReport{}, nil, classifyAPIErrorOnly(firstErr)
	}
	names, err := digestNames(ctx, cmd, flags, dbPath, cur)
	if err != nil {
		return digestReport{}, nil, err
	}
	counts := map[string]int{}
	for _, r := range raw {
		e := flattenAuditEvent(r, names)
		counts[firstNonEmpty(e.Category, "(none)")]++
		rep.Events = append(rep.Events, e)
	}
	sort.SliceStable(rep.Events, func(i, j int) bool {
		a, _ := parseTime(rep.Events[i].Time)
		b, _ := parseTime(rep.Events[j].Time)
		return a.After(b)
	})
	for k, n := range counts {
		rep.Categories = append(rep.Categories, countByKey{k, n})
	}
	sortCounts(rep.Categories)
	rep.Total = len(rep.Events)
	switch {
	case capped:
		rep.Note = "page cap reached; raise --max-pages to see older events in the window"
	case rep.Total == 0 && len(rep.FetchFailures) == 0:
		rep.Note = "no audit events in the window; widen --since"
	}
	if len(rep.FetchFailures) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d event-type queries failed; digest covers the rest\n", len(rep.FetchFailures), len(calls))
	}
	var partialErr error
	if len(rep.FetchFailures) > 0 {
		partialErr = apiErr(fmt.Errorf("%d of %d event-type queries failed; the digest is partial", len(rep.FetchFailures), len(calls)))
	}
	return rep, partialErr, nil
}

// digestNames maps local user and device ids to readable names, from the
// active account's store only.
func digestNames(ctx context.Context, cmd *cobra.Command, flags *rootFlags, dbPath, cur string) (map[string]string, error) {
	out := map[string]string{}
	path := storeDBPath(dbPath)
	if _, err := os.Stat(path); err != nil {
		return out, nil
	}
	db, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return out, nil
	}
	defer db.Close()
	if err := verifyStoreOwner(ctx, db, path, dbPath, cur); err != nil {
		return nil, err
	}
	users, _ := loadResourceRows(ctx, db, "users")
	for _, u := range users {
		var v struct {
			ID         string `json:"id"`
			Attributes struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
				Email     string `json:"email"`
			} `json:"attributes"`
		}
		if json.Unmarshal(u, &v) == nil && v.ID != "" {
			out[v.ID] = firstNonEmpty(v.Attributes.Email, strings.TrimSpace(v.Attributes.FirstName+" "+v.Attributes.LastName))
		}
	}
	devices, _ := loadLocalDevices(ctx, db)
	for _, d := range devices {
		out[d.ID] = strings.TrimSpace(d.Attributes.DeviceModel + " " + d.serial())
	}
	return out, nil
}

func flattenAuditEvent(raw json.RawMessage, names map[string]string) digestEvent {
	var env struct {
		Attributes map[string]json.RawMessage `json:"attributes"`
	}
	_ = json.Unmarshal(raw, &env)
	str := func(k string) string {
		var s string
		_ = json.Unmarshal(env.Attributes[k], &s)
		return s
	}
	e := digestEvent{Time: str("eventDateTime"), Type: str("type"), Category: str("category"), Outcome: str("outcome"),
		ActorType: str("actorType"), Actor: str("actorName"), ActorID: str("actorId"), SubjectType: str("subjectType"), Subject: str("subjectName")}
	if e.Actor == "" {
		e.Actor = firstNonEmpty(names[str("actorId")], str("actorId"))
	}
	if e.Subject == "" {
		e.Subject = firstNonEmpty(names[str("subjectId")], str("subjectId"))
	}
	var payload map[string]json.RawMessage
	if key := str("eventDataPropertyKey"); key != "" && json.Unmarshal(env.Attributes[key], &payload) == nil {
		keys := make([]string, 0, len(payload))
		for k := range payload {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			v := strings.Trim(string(payload[k]), `"`)
			if v == "" || v == "null" {
				continue
			}
			parts = append(parts, k+"="+v)
		}
		e.Details = strings.Join(parts, "; ")
	}
	return e
}
