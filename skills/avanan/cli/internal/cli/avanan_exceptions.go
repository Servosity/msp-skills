// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// `exceptions find` and `exceptions audit`.
//
// pp:data-source local
//
// Avanan spreads exceptions across seven engines living behind three different
// URL prefixes with three ID schemes and no shared response shape. The API has
// no cross-sub-system concept at all, so questions like "is this domain
// excepted anywhere" and "do any two engines contradict each other" cannot be
// asked of it — only of a unified local copy.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"avanan-pp-cli/internal/avananmirror"

	"github.com/spf13/cobra"
)

// loadStoredExceptions reads the unified exception table.
func loadStoredExceptions(cmd *cobra.Command, flags *rootFlags, dbPath string, empty any) ([]avananmirror.StoredException, bool, error) {
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()

	db, _, ok, err := openMirror(cmd, ctx, flags, dbPath, empty)
	if err != nil || !ok {
		return nil, false, err
	}
	defer db.Close()

	hintMirrorFreshness(cmd, db, avananmirror.ResourceExceptions, flags.maxAge)

	rows, err := loadResources(ctx, db, avananmirror.ResourceExceptions)
	if err != nil {
		return nil, false, err
	}

	out := make([]avananmirror.StoredException, 0, len(rows))
	for _, r := range rows {
		var se avananmirror.StoredException
		if err := json.Unmarshal(r.Raw, &se); err != nil {
			continue
		}
		out = append(out, se)
	}
	return out, true, nil
}

type exceptionHit struct {
	SubSystem   string `json:"sub_system"`
	Engine      string `json:"engine"`
	ListSide    string `json:"list_side"`
	ID          string `json:"id"`
	MatchString string `json:"match_string"`
	CreatedBy   string `json:"created_by,omitempty"`
	Comment     string `json:"comment,omitempty"`
}

type exceptionsFindReport struct {
	Query     string         `json:"query"`
	Scanned   int            `json:"scanned_exceptions"`
	Hits      []exceptionHit `json:"hits"`
	SubSystem []string       `json:"sub_systems_searched"`
	Note      string         `json:"note,omitempty"`
}

func newNovelExceptionsFindCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath string
		exact  bool
	)

	cmd := &cobra.Command{
		Use:   "find [string]",
		Short: "Search every exception sub-system at once for a domain, sender, URL, or hash",
		Long: strings.Trim(`
Answer "is this excepted anywhere" in one query.

Searches the unified local exception table across all seven engines (nine
listing endpoints in total):
anti-phishing, spam, anti-malware, URL reputation, DLP, anomaly, and
click-time protection. The live equivalent is nine separate calls across
three different URL prefixes.

Matching is case-insensitive and bidirectional by default: querying a domain
finds the addresses under it, and querying an address finds a domain-wide
exception that covers it. Pass --exact for whole-value equality.

Reads the local mirror only. Run 'mirror --resources exceptions' first.

Use this command to answer "is this domain/sender/URL excepted anywhere"
across all seven engines and nine exception tables at once. Do NOT use this command to
enumerate one sub-system's entries or to create/modify an entry; use
'exceptions get-ap', 'sectool-exceptions exceptions', or
'sectools list-anomaly-exceptions' for that sub-system directly.
`, "\n"),
		Example: strings.Trim(`
  avanan-cli exceptions find partner-domain.com
  avanan-cli exceptions find partner-domain.com --agent
  avanan-cli exceptions find noreply@vendor.example --exact --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:happy-args":       "<string>=example.com",
			"pp:typed-exit-codes": "0,3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "exceptions find")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a search string is required (a domain, sender address, URL, or file hash)"))
			}
			if flags.dataSource == "live" {
				return usageErr(fmt.Errorf("exceptions find has no live equivalent: it queries the unified local table; run 'mirror --resources exceptions' then retry without --data-source live"))
			}

			query := strings.TrimSpace(args[0])
			subsystems := make([]string, 0)
			for _, s := range avananmirror.ExceptionSources() {
				subsystems = append(subsystems, s.SubSystem)
			}
			empty := exceptionsFindReport{Query: query, Hits: []exceptionHit{}, SubSystem: subsystems}

			all, ok, err := loadStoredExceptions(cmd, flags, dbPath, empty)
			if err != nil || !ok {
				return err
			}

			report := empty
			report.Scanned = len(all)
			needle := strings.ToLower(query)

			for _, e := range all {
				hay := strings.ToLower(e.MatchString)
				matched := hay == needle
				if !exact && !matched && hay != "" {
					// Match in BOTH directions. Querying a domain must find the
					// addresses under it, and querying an address must find a
					// domain-wide exception that covers it. Only checking
					// Contains(stored, query) misses the second case, which is
					// the dangerous one: it answers "not excepted anywhere" for
					// a sender that is in fact allow-listed by domain.
					matched = strings.Contains(hay, needle) || strings.Contains(needle, hay)
				}
				if !matched {
					continue
				}
				report.Hits = append(report.Hits, exceptionHit{
					SubSystem:   e.SubSystem,
					Engine:      e.Engine,
					ListSide:    e.ListSide,
					ID:          e.ID,
					MatchString: e.MatchString,
					CreatedBy:   e.CreatedBy,
					Comment:     e.Comment,
				})
			}

			sort.Slice(report.Hits, func(i, j int) bool {
				if report.Hits[i].SubSystem != report.Hits[j].SubSystem {
					return report.Hits[i].SubSystem < report.Hits[j].SubSystem
				}
				return report.Hits[i].MatchString < report.Hits[j].MatchString
			})

			if len(report.Hits) == 0 {
				report.Note = fmt.Sprintf(
					"scanned %d mirrored exceptions across %d sub-systems; %q is not excepted anywhere. If the mirror is stale, run 'avanan-cli mirror --resources exceptions'.",
					report.Scanned, len(subsystems), query)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), report, flags); err != nil {
					return err
				}
				if len(report.Hits) == 0 {
					return &cliError{code: 3, err: fmt.Errorf("no exception matches %q", query)}
				}
				return nil
			}

			if len(report.Hits) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), report.Note)
				return &cliError{code: 3, err: fmt.Errorf("no exception matches %q", query)}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%d matches for %q (scanned %d exceptions)\n\n", len(report.Hits), query, report.Scanned)
			fmt.Fprintf(cmd.OutOrStdout(), "%-26s %-7s %-34s %s\n", "SUB-SYSTEM", "SIDE", "MATCH", "CREATED BY")
			for _, h := range report.Hits {
				fmt.Fprintf(cmd.OutOrStdout(), "%-26s %-7s %-34s %s\n",
					h.SubSystem, h.ListSide, truncateCell(h.MatchString, 34), h.CreatedBy)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&exact, "exact", false, "Require whole-value equality instead of substring matching")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

type exceptionConflict struct {
	MatchString string   `json:"match_string"`
	AllowIn     []string `json:"allowed_in"`
	BlockIn     []string `json:"blocked_in"`
}

type exceptionDuplicate struct {
	MatchString string   `json:"match_string"`
	SubSystems  []string `json:"sub_systems"`
	Count       int      `json:"count"`
}

type exceptionsAuditReport struct {
	Scanned      int                  `json:"scanned_exceptions"`
	Conflicts    []exceptionConflict  `json:"conflicts"`
	Duplicates   []exceptionDuplicate `json:"duplicates"`
	NeverMatched []exceptionHit       `json:"never_matched"`
	WindowNote   string               `json:"window_note"`
	Note         string               `json:"note,omitempty"`
}

func newNovelExceptionsAuditCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath        string
		skipUnmatched bool
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Flag contradictory, duplicate, and never-matched exceptions across sub-systems",
		Long: strings.Trim(`
A hygiene sweep over the whole exception inventory.

Reports three things:

  conflicts      one value allowed in one engine and blocked in another
  duplicates     the same value entered in more than one sub-system
  never matched  entries whose value never appears in mirrored traffic

The never-matched check is bounded by what the local mirror contains. An entry
listed there is not proven dead — it is unused within the mirrored window.
Widen the window with 'mirror --since 30d' before acting on it.

Use this command for a hygiene sweep of the exception inventory. Do NOT use
this command to look up one specific string; use 'exceptions find' instead.
`, "\n"),
		Example: strings.Trim(`
  avanan-cli exceptions audit
  avanan-cli exceptions audit --agent
  avanan-cli exceptions audit --skip-unmatched --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 && !flags.agent && !flags.asJSON {
				// An audit with no arguments is a legitimate full invocation,
				// so only treat a bare interactive call as a help request.
				if isTerminal(cmd.OutOrStdout()) {
					return runExceptionsAudit(cmd, flags, dbPath, skipUnmatched)
				}
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "exceptions audit")
			}
			if flags.dataSource == "live" {
				return usageErr(fmt.Errorf("exceptions audit has no live equivalent: it self-joins the local table; run 'mirror --resources exceptions' then retry without --data-source live"))
			}
			return runExceptionsAudit(cmd, flags, dbPath, skipUnmatched)
		},
	}

	cmd.Flags().BoolVar(&skipUnmatched, "skip-unmatched", false, "Skip the never-matched check (faster; avoids scanning mirrored traffic)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func runExceptionsAudit(cmd *cobra.Command, flags *rootFlags, dbPath string, skipUnmatched bool) error {
	empty := exceptionsAuditReport{
		Conflicts:    []exceptionConflict{},
		Duplicates:   []exceptionDuplicate{},
		NeverMatched: []exceptionHit{},
		WindowNote:   "never-matched is scoped to the mirrored window only",
	}

	all, ok, err := loadStoredExceptions(cmd, flags, dbPath, empty)
	if err != nil || !ok {
		return err
	}

	report := empty
	report.Scanned = len(all)

	type sides struct {
		allow, block []string
		subsystems   map[string]bool
		sample       exceptionHit
	}
	byValue := map[string]*sides{}

	for _, e := range all {
		key := strings.ToLower(strings.TrimSpace(e.MatchString))
		if key == "" {
			continue
		}
		s, ok := byValue[key]
		if !ok {
			s = &sides{subsystems: map[string]bool{}}
			byValue[key] = s
		}
		s.subsystems[e.SubSystem] = true
		switch e.ListSide {
		case "allow":
			s.allow = append(s.allow, e.SubSystem)
		case "block":
			s.block = append(s.block, e.SubSystem)
		}
		if s.sample.MatchString == "" {
			s.sample = exceptionHit{
				SubSystem: e.SubSystem, Engine: e.Engine, ListSide: e.ListSide,
				ID: e.ID, MatchString: e.MatchString, CreatedBy: e.CreatedBy,
			}
		}
	}

	for value, s := range byValue {
		if len(s.allow) > 0 && len(s.block) > 0 {
			report.Conflicts = append(report.Conflicts, exceptionConflict{
				MatchString: value,
				AllowIn:     dedupeSorted(s.allow),
				BlockIn:     dedupeSorted(s.block),
			})
		}
		if len(s.subsystems) > 1 {
			report.Duplicates = append(report.Duplicates, exceptionDuplicate{
				MatchString: value,
				SubSystems:  sortedKeys(s.subsystems),
				Count:       len(s.subsystems),
			})
		}
	}

	if !skipUnmatched {
		ctx, cancel := boundCtx(cmd.Context(), flags)
		defer cancel()

		db, _, ok, err := openMirror(cmd, ctx, flags, dbPath, report)
		if err != nil || !ok {
			return err
		}
		defer db.Close()

		// Sequential drains; never a nested query.
		events, err := loadResources(ctx, db, avananmirror.ResourceEvents)
		if err != nil {
			return err
		}
		entities, err := loadResources(ctx, db, avananmirror.ResourceEntities)
		if err != nil {
			return err
		}

		haystack := make([]string, 0, len(events)+len(entities))
		for _, r := range append(events, entities...) {
			if len(r.Raw) > 0 {
				haystack = append(haystack, strings.ToLower(string(r.Raw)))
			}
		}

		if len(haystack) == 0 {
			report.WindowNote = "never-matched check skipped: the local mirror contains no events or entities to match against"
		} else {
			for value, s := range byValue {
				matched := false
				for _, h := range haystack {
					if strings.Contains(h, value) {
						matched = true
						break
					}
				}
				if !matched {
					report.NeverMatched = append(report.NeverMatched, s.sample)
				}
			}
			report.WindowNote = fmt.Sprintf(
				"never-matched is scoped to %d mirrored events and entities; an entry listed here is unused within that window, not proven dead",
				len(haystack))
		}
	} else {
		report.WindowNote = "never-matched check skipped (--skip-unmatched)"
	}

	sort.Slice(report.Conflicts, func(i, j int) bool { return report.Conflicts[i].MatchString < report.Conflicts[j].MatchString })
	sort.Slice(report.Duplicates, func(i, j int) bool { return report.Duplicates[i].MatchString < report.Duplicates[j].MatchString })
	sort.Slice(report.NeverMatched, func(i, j int) bool { return report.NeverMatched[i].MatchString < report.NeverMatched[j].MatchString })

	if report.Scanned == 0 {
		report.Note = "no exceptions in the local mirror; run 'avanan-cli mirror --resources exceptions' first"
	}

	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), report, flags)
	}

	if report.Scanned == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), report.Note)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Audited %d exceptions across %d sub-systems\n", report.Scanned, len(avananmirror.ExceptionSources()))

	if len(report.Conflicts) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nCONFLICTS (%d) — allowed in one engine, blocked in another\n", len(report.Conflicts))
		for _, c := range report.Conflicts {
			fmt.Fprintf(cmd.OutOrStdout(), "  %-40s allow: %-30s block: %s\n",
				truncateCell(c.MatchString, 40), strings.Join(c.AllowIn, ","), strings.Join(c.BlockIn, ","))
		}
	}
	if len(report.Duplicates) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nDUPLICATES (%d) — same value in more than one sub-system\n", len(report.Duplicates))
		for _, d := range report.Duplicates {
			fmt.Fprintf(cmd.OutOrStdout(), "  %-40s %s\n", truncateCell(d.MatchString, 40), strings.Join(d.SubSystems, ", "))
		}
	}
	if len(report.NeverMatched) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nNEVER MATCHED (%d)\n", len(report.NeverMatched))
		for _, n := range report.NeverMatched {
			fmt.Fprintf(cmd.OutOrStdout(), "  %-40s %s\n", truncateCell(n.MatchString, 40), n.SubSystem)
		}
	}
	if len(report.Conflicts) == 0 && len(report.Duplicates) == 0 && len(report.NeverMatched) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nNo conflicts, duplicates, or unused entries found.")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nNote: %s\n", report.WindowNote)
	return nil
}

func dedupeSorted(in []string) []string {
	seen := map[string]bool{}
	for _, v := range in {
		seen[v] = true
	}
	return sortedKeys(seen)
}

func truncateCell(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
