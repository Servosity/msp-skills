// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-umbrella-pp-cli/internal/cliutil"
)

type allowEntry struct {
	List        string `json:"list"`
	ListID      int64  `json:"list_id"`
	Destination string `json:"destination"`
	Comment     string `json:"comment,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

type riskyAllow struct {
	allowEntry
	Status             string   `json:"status"`
	SecurityCategories []string `json:"security_categories"`
}

type allowConflict struct {
	AllowList  string `json:"allow_list"`
	AllowEntry string `json:"allow_entry"`
	BlockList  string `json:"block_list"`
	BlockEntry string `json:"block_entry"`
}

type allowAudit struct {
	AllowLists    int             `json:"allow_lists"`
	AllowEntries  int             `json:"allow_entries"`
	Since         string          `json:"since,omitempty"`
	Checked       bool            `json:"categorization_checked"`
	Uncategorized []string        `json:"uncategorized"`
	Risky         []riskyAllow    `json:"risky"`
	Conflicts     []allowConflict `json:"conflicts"`
	NewAllows     []allowEntry    `json:"new_allows"`
	UndatedAllows []allowEntry    `json:"undated_allows"`     // --since only: createdAt missing or unreadable
	Unreadable    []allowEntry    `json:"unreadable_entries"` // could not be parsed, so not checked
	Notes         []string        `json:"notes,omitempty"`
}

func newNovelAllowlistAuditCmd(flags *rootFlags) *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Find allow-list entries now categorized as malicious, allow/block conflicts, and recent allows",
		Long: `Read every allow-type destination list, check its domains against Investigate
(bulk categorization), and report: entries now categorized malicious or carrying a
security category, entries that collide with a block-list entry (same value or a
parent domain), and entries created inside --since. Without the investigate scope
it still reports conflicts and recent allows.

Use this command to review the org's own allow-list entries for risk and conflicts.
Do NOT use this command to look up domains from a ticket or IOC feed; use 'verdict'
instead.`,
		Example: strings.Trim(`
  cisco-umbrella-cli allowlist audit --since 7d --agent
  cisco-umbrella-cli allowlist audit --json --select risky.destination,risky.status`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "--since=7d"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "allowlist audit")
			}
			var cutoff time.Time
			if since != "" {
				d, err := cliutil.ParseDurationLoose(since)
				if err != nil || d <= 0 {
					return usageErr(fmt.Errorf("--since wants a duration like 7d, 24h, 2w; got %q", since))
				}
				cutoff = time.Now().Add(-d)
			}
			// ponytail: no whole-command --timeout here; this fans out across every list/customer
			// and each request is already bounded by the client's per-request timeout.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if err := requireCredentials(c); err != nil {
				return err
			}
			lists, err := fetchDestinationLists(ctx, c)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			report := allowAudit{Since: since, Uncategorized: []string{}, Risky: []riskyAllow{}, Conflicts: []allowConflict{}, NewAllows: []allowEntry{}, UndatedAllows: []allowEntry{}, Unreadable: []allowEntry{}}
			allows := make([]allowEntry, 0)
			type blockEntry struct{ list, value, kind, raw string }
			blocks := make([]blockEntry, 0)
			for _, l := range lists {
				access := strings.ToLower(l.Access)
				if access != "allow" && access != "block" {
					continue
				}
				dests, err := fetchDestinations(ctx, c, l.ID)
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				if access == "allow" {
					report.AllowLists++
				}
				for _, d := range dests {
					if access == "allow" {
						allows = append(allows, allowEntry{List: l.Name, ListID: l.ID, Destination: d.Destination, Comment: d.Comment, CreatedAt: d.CreatedAt})
						continue
					}
					if v, k, ok := normalizeDestination(d.Destination); ok {
						blocks = append(blocks, blockEntry{l.Name, v, k, d.Destination})
					}
				}
			}
			report.AllowEntries = len(allows)

			domainSet := map[string]bool{}
			for _, a := range allows {
				v, k, ok := normalizeDestination(a.Destination)
				if !ok {
					report.Unreadable = append(report.Unreadable, a)
					continue
				}
				for _, b := range blocks {
					if conflicts(v, k, b.value, b.kind) {
						report.Conflicts = append(report.Conflicts, allowConflict{AllowList: a.List, AllowEntry: a.Destination, BlockList: b.list, BlockEntry: b.raw})
					}
				}
				if hasDomainHost(v, k) {
					domainSet[hostOf(v)] = true
				}
				if !cutoff.IsZero() {
					t, ok := parseCreatedAt(a.CreatedAt)
					switch {
					case !ok:
						report.UndatedAllows = append(report.UndatedAllows, a) // age unknown: may be new
					case t.After(cutoff):
						report.NewAllows = append(report.NewAllows, a)
					}
				}
			}

			domains := sortedKeys(domainSet)
			cats, missing, forbidden, err := categorizeDomains(ctx, c, domains)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			switch {
			case forbidden:
				report.Notes = append(report.Notes, "categorization skipped: API key lacks the investigate scope")
			case len(missing) > 0:
				report.Uncategorized = missing
				report.Notes = append(report.Notes, fmt.Sprintf("Investigate returned no categorization for %d domain(s); see uncategorized", len(missing)))
			default:
				report.Checked = true
			}
			for _, a := range allows {
				v, k, ok := normalizeDestination(a.Destination)
				if !ok || !hasDomainHost(v, k) {
					continue
				}
				cat, found := cats[hostOf(v)]
				if found && (cat.Status == -1 || len(cat.SecurityCategories) > 0) {
					report.Risky = append(report.Risky, riskyAllow{allowEntry: a, Status: statusLabel(cat.Status), SecurityCategories: cat.SecurityCategories})
				}
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d allow lists, %d entries: %d risky, %d conflicts", report.AllowLists, report.AllowEntries, len(report.Risky), len(report.Conflicts))
			if since != "" {
				fmt.Fprintf(w, ", %d added in the last %s", len(report.NewAllows), since)
			}
			fmt.Fprintln(w)
			for _, r := range report.Risky {
				fmt.Fprintf(w, "RISKY     %s (%s): %s %s\n", r.Destination, r.List, r.Status, strings.Join(r.SecurityCategories, ","))
			}
			for _, x := range report.Conflicts {
				fmt.Fprintf(w, "CONFLICT  %s (%s) vs %s (%s)\n", x.AllowEntry, x.AllowList, x.BlockEntry, x.BlockList)
			}
			if len(report.Unreadable) > 0 {
				fmt.Fprintf(w, "%d allow entr(ies) could not be parsed and were not checked:\n", len(report.Unreadable))
				for _, n := range report.Unreadable {
					fmt.Fprintf(w, "UNREADABLE %s (%s)\n", n.Destination, n.List)
				}
			}
			if len(report.UndatedAllows) > 0 {
				fmt.Fprintf(w, "%d allow entr(ies) have no readable createdAt, so their age is unknown:\n", len(report.UndatedAllows))
				for _, n := range report.UndatedAllows {
					fmt.Fprintf(w, "UNDATED   %s (%s)\n", n.Destination, n.List)
				}
			}
			for _, n := range report.NewAllows {
				fmt.Fprintf(w, "NEW       %s (%s) %s %s\n", n.Destination, n.List, n.CreatedAt, n.Comment)
			}
			for _, n := range report.Notes {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %s\n", n)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Also list allow entries created within this window, e.g. 7d, 24h, 2w")
	return cmd
}

// conflicts reports whether an allow value and a block value address the same
// destination: equal values, or one domain entry covering the other.
// conflicts reports whether an allow entry and a block entry address the same
// destination, using the one matcher verdict uses, in both directions: equal
// values, a parent domain covering a subdomain or URL, an IPv4 entry covering a
// URL on that address, or a URL entry covering a longer URL.
func conflicts(av, ak, bv, bk string) bool {
	return av == bv ||
		destinationMatches(umbrellaDestination{Destination: bv}, av, ak) ||
		destinationMatches(umbrellaDestination{Destination: av}, bv, bk)
}

// parseCreatedAt reads a destination's createdAt, which the API returns as a
// string: RFC 3339, "YYYY-MM-DD HH:MM:SS", or epoch seconds/milliseconds.
func parseCreatedAt(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		if n > 1e12 {
			return time.UnixMilli(n), true
		}
		return time.Unix(n, 0), true
	}
	return time.Time{}, false
}
