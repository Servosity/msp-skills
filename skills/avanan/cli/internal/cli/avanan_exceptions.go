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
	"net/url"
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

	// A row with no partition (written by an older mirror, or unreadable)
	// cannot be attributed to a tenant or reconciled against a listing, so an
	// answer built on it may report an exception the API no longer lists.
	// Refuse until a mirror that reads every partition replaces them.
	out := make([]avananmirror.StoredException, 0, len(rows))
	unattributable := 0
	for _, r := range rows {
		var se avananmirror.StoredException
		if err := json.Unmarshal(r.Raw, &se); err != nil || se.Partition == "" {
			unattributable++
			continue
		}
		out = append(out, se)
	}
	if unattributable > 0 {
		return nil, false, fmt.Errorf("%d stored exception(s) were written by an older version of this CLI or cannot be read, so they cannot be attributed to a tenant or checked against the current listing; run 'avanan-cli mirror --resources exceptions' (every scope) to rebuild the inventory", unattributable)
	}
	return out, true, nil
}

type exceptionHit struct {
	SubSystem   string            `json:"sub_system"`
	Engine      string            `json:"engine"`
	ListSide    string            `json:"list_side"`
	Scope       string            `json:"scope,omitempty"`
	ID          string            `json:"id"`
	MatchString string            `json:"match_string"`
	Selectors   map[string]string `json:"selectors,omitempty"`
	MatchModes  map[string]string `json:"match_modes,omitempty"`
	Conditional bool              `json:"conditional,omitempty"`
	CreatedBy   string            `json:"created_by,omitempty"`
	Comment     string            `json:"comment,omitempty"`
}

func hitFor(e avananmirror.StoredException) exceptionHit {
	return exceptionHit{
		SubSystem:   e.SubSystem,
		Engine:      e.Engine,
		ListSide:    e.ListSide,
		Scope:       e.Scope,
		ID:          e.ID,
		MatchString: e.MatchString,
		Selectors:   e.Selectors,
		MatchModes:  e.MatchModes,
		Conditional: len(e.SelectorValues()) > 1,
		CreatedBy:   e.CreatedBy,
		Comment:     e.Comment,
	}
}

type exceptionsFindReport struct {
	Query       string         `json:"query"`
	Scanned     int            `json:"scanned_exceptions"`
	Hits        []exceptionHit `json:"hits"`
	SubSystem   []string       `json:"sub_systems_searched"`
	Unmatchable int            `json:"unmatchable_exceptions,omitempty"`
	Note        string         `json:"note,omitempty"`
}

// exceptionValueKind classifies a selector value for matching.
func exceptionValueKind(v string) (kind, host string) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "", ""
	}
	if strings.Contains(v, "://") {
		if u, err := url.Parse(v); err == nil && u.Hostname() != "" {
			return "url", strings.TrimSuffix(u.Hostname(), ".")
		}
		return "", ""
	}
	if at := strings.LastIndex(v, "@"); at > 0 && at < len(v)-1 {
		if d := v[at+1:]; isDomainName(d) {
			return "email", strings.TrimSuffix(d, ".")
		}
		return "", ""
	}
	if isDomainName(v) {
		return "domain", strings.TrimSuffix(v, ".")
	}
	return "", ""
}

// isDomainName accepts dotted DNS names (letters, digits, hyphens, at least
// one dot) and nothing else, so a hash, a display name or free text is never
// treated as a domain.
func isDomainName(v string) bool {
	v = strings.TrimSuffix(v, ".")
	if !strings.Contains(v, ".") {
		return false
	}
	for _, label := range strings.Split(v, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// domainCovers reports whether child is parent or a subdomain of it. The
// comparison is on whole DNS labels: badexample.com is not under example.com.
func domainCovers(parent, child string) bool {
	return child == parent || strings.HasSuffix(child, "."+parent)
}

// exceptionValueMatches is the one matcher `exceptions find` uses.
//
// Equality (case-insensitive) always matches. Otherwise matching is on DNS
// label boundaries, never substrings:
//
//   - a domain finds addresses and URLs on it or its subdomains, and is found
//     by them (the documented bidirectional behaviour);
//   - two domains match when one is the other or a parent of it;
//   - two addresses, or an address and a URL, match only when equal;
//   - anything that is not an address, domain or URL (a hash, a name, free
//     text) matches only when equal.
func exceptionValueMatches(query, stored string, exact bool) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	h := strings.ToLower(strings.TrimSpace(stored))
	if q == "" || h == "" {
		return false
	}
	if q == h {
		return true
	}
	if exact {
		return false
	}
	qk, qh := exceptionValueKind(q)
	hk, hh := exceptionValueKind(h)
	if qk == "" || hk == "" {
		return false
	}
	switch {
	case qk == "email" && hk == "email":
		return false
	case (qk == "email" && hk == "url") || (qk == "url" && hk == "email"):
		return false
	case qk == "email":
		// An address is covered by a domain-wide rule on its domain or a
		// parent of it.
		return domainCovers(hh, qh)
	case hk == "email":
		// A domain query finds the addresses under it.
		return domainCovers(qh, hh)
	default:
		// domain/url against domain/url: either contains the other.
		return domainCovers(qh, hh) || domainCovers(hh, qh)
	}
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

Searches the unified local exception table across all seven engines (ten
listing endpoints in total):
anti-phishing, spam, anti-malware, URL reputation, DLP, anomaly, and
click-time protection. The live equivalent is ten separate calls across
three different URL prefixes.

Matching is case-insensitive and bidirectional by default, on whole domain
labels: querying a domain finds the addresses and subdomains under it, and
querying an address finds a domain-wide exception that covers it
(badexample.com is not under example.com). Hashes, names and other values
match only when equal. Every selector an entry carries is checked, and an
entry conditioned on more than one (a sender domain AND a recipient) is shown
as conditional with all of them. Pass --exact for whole-value equality.

Reads the local mirror only. Run 'mirror --resources exceptions' first.

Use this command to answer "is this domain/sender/URL excepted anywhere"
across all seven engines and ten exception tables at once. Do NOT use this command to
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

			for _, e := range all {
				values := e.SelectorValues()
				if len(values) == 0 {
					// An entry with no field this CLI can match on cannot be
					// ruled in or out. Count it so "not excepted" is never
					// stated over entries that were not evaluated.
					report.Unmatchable++
					continue
				}
				// Every selector the entry carries is checked, in BOTH
				// directions on label boundaries: querying a domain finds the
				// addresses under it, and querying an address finds a
				// domain-wide exception that covers it.
				matched := false
				for _, v := range values {
					if exceptionValueMatches(query, v, exact) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
				report.Hits = append(report.Hits, hitFor(e))
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
				if report.Unmatchable > 0 {
					report.Note = fmt.Sprintf(
						"scanned %d mirrored exceptions across %d sub-systems and found no match for %q, but %d of them carry no field this CLI can match on, so \"not excepted\" is NOT confirmed; inspect them with 'exceptions get-ap' or 'sectool-exceptions exceptions'.",
						report.Scanned, len(subsystems), query, report.Unmatchable)
				}
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
			fmt.Fprintf(cmd.OutOrStdout(), "%-26s %-7s %-22s %-34s %s\n", "SUB-SYSTEM", "SIDE", "SCOPE", "MATCH", "CREATED BY")
			for _, h := range report.Hits {
				fmt.Fprintf(cmd.OutOrStdout(), "%-26s %-7s %-22s %-34s %s\n",
					h.SubSystem, h.ListSide, truncateCell(orDefault(h.Scope, "-"), 22), truncateCell(h.MatchString, 34), h.CreatedBy)
				if h.Conditional {
					// A rule on more than one selector applies only when ALL
					// of them hold; never present it as covering one value.
					fmt.Fprintf(cmd.OutOrStdout(), "  conditional: applies only when all of %s\n", describeSelectors(h.Selectors))
				}
			}
			if report.Unmatchable > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d exception(s) carry no field this CLI can match on and were not evaluated.\n", report.Unmatchable)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&exact, "exact", false, "Require whole-value equality instead of substring matching")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

type exceptionConflict struct {
	Scope       string   `json:"scope,omitempty"`
	MatchString string   `json:"match_string"`
	AllowIn     []string `json:"allowed_in"`
	BlockIn     []string `json:"blocked_in"`
}

type exceptionDuplicate struct {
	Scope       string   `json:"scope,omitempty"`
	MatchString string   `json:"match_string"`
	SubSystems  []string `json:"sub_systems"`
	Count       int      `json:"count"`
}

// describeSelectors renders a predicate as field=value pairs in a stable order.
func describeSelectors(sel map[string]string) string {
	keys := make([]string, 0, len(sel))
	for k := range sel {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+sel[k])
	}
	return strings.Join(parts, ", ")
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

Reports three things, each within one tenant (scope):

  conflicts      one predicate allowed in one engine and blocked in another
  duplicates     the same predicate entered in more than one sub-system
  never matched  entries whose values never appear in mirrored traffic

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

	// Entries are compared only within one tenant (scope) and only when they
	// express the SAME predicate: two tenants' policies are not a conflict,
	// and a rule conditioned on a recipient is not the domain-wide rule on
	// the same domain. The predicate is the multiset of selector values (one
	// per populated field, equal values kept), so a single-value entry still
	// lines up across engines that name the field differently (senderDomain
	// in one, domain in another) while a two-field rule never collapses into
	// a one-field rule on the same value.
	type sides struct {
		scope        string
		display      string
		values       []string
		allow, block []string
		subsystems   map[string]bool
		sample       exceptionHit
	}
	byValue := map[string]*sides{}

	for _, e := range all {
		values := e.SelectorValues()
		if len(values) == 0 {
			continue
		}
		lowered := make([]string, 0, len(values))
		for _, v := range values {
			lowered = append(lowered, strings.ToLower(v))
		}
		sort.Strings(lowered)
		key := e.Scope + "\x00" + strings.Join(lowered, "\x1f")
		s, ok := byValue[key]
		if !ok {
			s = &sides{scope: e.Scope, display: strings.Join(lowered, " + "), values: lowered, subsystems: map[string]bool{}}
			byValue[key] = s
		}
		s.subsystems[e.SubSystem] = true
		switch e.ListSide {
		case "allow":
			s.allow = append(s.allow, e.SubSystem)
		case "block":
			s.block = append(s.block, e.SubSystem)
		}
		if s.sample.SubSystem == "" {
			s.sample = hitFor(e)
		}
	}

	for _, s := range byValue {
		if len(s.allow) > 0 && len(s.block) > 0 {
			report.Conflicts = append(report.Conflicts, exceptionConflict{
				Scope:       s.scope,
				MatchString: s.display,
				AllowIn:     dedupeSorted(s.allow),
				BlockIn:     dedupeSorted(s.block),
			})
		}
		if len(s.subsystems) > 1 {
			report.Duplicates = append(report.Duplicates, exceptionDuplicate{
				Scope:       s.scope,
				MatchString: s.display,
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
			for _, s := range byValue {
				matched := false
				for _, h := range haystack {
					for _, value := range s.values {
						if strings.Contains(h, value) {
							matched = true
							break
						}
					}
					if matched {
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

	sort.Slice(report.Conflicts, func(i, j int) bool {
		if report.Conflicts[i].Scope != report.Conflicts[j].Scope {
			return report.Conflicts[i].Scope < report.Conflicts[j].Scope
		}
		return report.Conflicts[i].MatchString < report.Conflicts[j].MatchString
	})
	sort.Slice(report.Duplicates, func(i, j int) bool {
		if report.Duplicates[i].Scope != report.Duplicates[j].Scope {
			return report.Duplicates[i].Scope < report.Duplicates[j].Scope
		}
		return report.Duplicates[i].MatchString < report.Duplicates[j].MatchString
	})
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
			fmt.Fprintf(cmd.OutOrStdout(), "  %-22s %-40s allow: %-30s block: %s\n",
				truncateCell(orDefault(c.Scope, "-"), 22), truncateCell(c.MatchString, 40), strings.Join(c.AllowIn, ","), strings.Join(c.BlockIn, ","))
		}
	}
	if len(report.Duplicates) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nDUPLICATES (%d) — same value in more than one sub-system\n", len(report.Duplicates))
		for _, d := range report.Duplicates {
			fmt.Fprintf(cmd.OutOrStdout(), "  %-22s %-40s %s\n", truncateCell(orDefault(d.Scope, "-"), 22), truncateCell(d.MatchString, 40), strings.Join(d.SubSystems, ", "))
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
