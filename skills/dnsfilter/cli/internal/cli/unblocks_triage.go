// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type triageGroup struct {
	Domain         string   `json:"domain"`
	Requests       int      `json:"requests"`
	RequestIDs     []string `json:"request_ids"`
	Orgs           []string `json:"orgs"`
	PolicyIDs      []string `json:"policy_ids"`
	BlockedBy      []string `json:"blocked_by"`
	Categories     []string `json:"categories"`
	IsThreat       bool     `json:"is_threat"`
	AlreadyAllowed bool     `json:"already_allowed"`
	CoveredBy      []string `json:"covered_by"`         // policy, universal_org, universal_msp
	Unchecked      int      `json:"coverage_unchecked"` // requests whose coverage was not read
	OldestAttempt  string   `json:"oldest_attempt"`
	AgeHours       int      `json:"age_hours"`
	Hint           string   `json:"hint"` // deny, resolve-already-allowed, review
}

type triageView struct {
	ScannedRequests int           `json:"scanned_requests"`
	Truncated       bool          `json:"truncated"`
	Groups          []triageGroup `json:"groups"`
	Note            string        `json:"note,omitempty"`
}

func newNovelUnblocksTriageCmd(flags *rootFlags) *cobra.Command {
	var orgRefStr string
	var maxScanPages, maxCoverageChecks int

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Review pending unblock requests grouped by domain, with threat and already-allowed flags",
		Long: `Use this command to review the pending end-user unblock request queue grouped by domain across clients, with threat flags and already-allowed detection.
Do NOT use this command to diagnose a block reported by phone or ticket with no unblock request; use 'why-blocked' instead.
Do NOT use this command to apply an allow; use 'allow' instead.

Coverage comes from each request's already_covered field (policy list or org/MSP Universal Allow List, evaluated by DNSFilter at call time), read for up to --max-coverage-checks requests. Hints follow a fixed rule: threat category -> deny; every request in the group confirmed covered -> resolve-already-allowed; otherwise review. The unblock queue only has data when the account has unblock requests enabled. Apply a decision with 'unblock-requests decision unblock-requests <id> --decision <allow|deny|resolve-already-allowed>'.`,
		Example: strings.Trim(`
  dnsfilter-cli unblocks triage --agent
  dnsfilter-cli unblocks triage --org 'Acme Dental'`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "unblocks triage")
			}
			if err := rejectExtraArgs(args, 0); err != nil {
				return err
			}
			if maxScanPages < 1 || maxCoverageChecks < 0 {
				return usageErr(fmt.Errorf("--max-scan-pages must be >= 1 and --max-coverage-checks >= 0"))
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			params := map[string]string{"tab": "active"}
			if orgRefStr != "" {
				org, err := resolveOrg(ctx, c, orgRefStr)
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				params["organization_id"] = org.ID
			}
			requests, truncated, err := fetchAllPages(ctx, c, "/v2/unblock_requests", params, maxScanPages)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			if cliutil.IsDogfoodEnv() && maxCoverageChecks > 5 {
				maxCoverageChecks = 5
			}
			// Read the documented already_covered result per request (bounded).
			coverage := map[string]map[string]any{}
			for i, r := range requests {
				if i >= maxCoverageChecks {
					break
				}
				raw, err := c.GetNoCache(ctx, "/v2/unblock_requests/"+url.PathEscape(r.ID), nil)
				if err != nil {
					continue
				}
				if list, _ := decodeJA(raw); len(list) > 0 {
					if cov, ok := list[0].Attrs["already_covered"].(map[string]any); ok {
						coverage[r.ID] = cov
					}
				}
			}
			view := triageView{ScannedRequests: len(requests), Truncated: truncated, Groups: groupUnblockRequests(requests, coverage, time.Now())}
			if truncated {
				view.Note = fmt.Sprintf("stopped after --max-scan-pages=%d pages; raise it to include older requests", maxScanPages)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			if len(view.Groups) == 0 {
				fmt.Fprintln(w, "No pending unblock requests.")
				return nil
			}
			for _, g := range view.Groups {
				fmt.Fprintf(w, "%-24s %-30s requests=%-3d age=%dh threat=%v allowed=%v orgs=%s\n", g.Hint, g.Domain, g.Requests, g.AgeHours, g.IsThreat, g.AlreadyAllowed, strings.Join(g.Orgs, ", "))
			}
			if view.Note != "" {
				fmt.Fprintln(w, "note:", view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgRefStr, "org", "", "Only this client org (name or id)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum 100-row pages of unblock requests to scan")
	cmd.Flags().IntVar(&maxCoverageChecks, "max-coverage-checks", 50, "Maximum requests whose already-covered status is read (one API call each)")
	return cmd
}

// nestedID reads {id, name} linked-resource objects.
func nestedID(v any) string {
	m, _ := v.(map[string]any)
	return idString(m["id"])
}

func nestedName(v any) string {
	m, _ := v.(map[string]any)
	s, _ := m["name"].(string)
	return s
}

// groupUnblockRequests is the pure grouping and hint rule behind `unblocks triage`.
// coverage holds each checked request's already_covered object by request id.
func groupUnblockRequests(requests []jaRes, coverage map[string]map[string]any, now time.Time) []triageGroup {
	groups := map[string]*triageGroup{}
	covered := map[string]int{} // domain -> requests confirmed covered
	for _, r := range requests {
		domain := normalizeDomain(r.str("domain"))
		if domain == "" {
			continue
		}
		g := groups[domain]
		if g == nil {
			g = &triageGroup{Domain: domain, RequestIDs: make([]string, 0), Orgs: make([]string, 0), PolicyIDs: make([]string, 0), BlockedBy: make([]string, 0), Categories: make([]string, 0), CoveredBy: make([]string, 0)}
			groups[domain] = g
		}
		g.Requests++
		g.RequestIDs = append(g.RequestIDs, r.ID)
		g.Orgs = appendUnique(g.Orgs, nestedName(r.Attrs["organization"]))
		g.BlockedBy = appendUnique(g.BlockedBy, r.str("blocked_by"))
		if cat, ok := r.Attrs["category"].(map[string]any); ok {
			if threat, _ := cat["is_threat"].(bool); threat {
				g.IsThreat = true
			}
			names, _ := cat["names"].([]any)
			for _, n := range names {
				g.Categories = appendUnique(g.Categories, idString(n))
			}
		}
		g.PolicyIDs = appendUnique(g.PolicyIDs, nestedID(r.Attrs["policy"]))
		if cov, ok := coverage[r.ID]; !ok {
			g.Unchecked++
		} else if isCovered, _ := cov["covered"].(bool); isCovered {
			covered[domain]++
			list, _ := cov["list"].(string)
			g.CoveredBy = appendUnique(g.CoveredBy, list)
		}
		if t := r.str("time_of_attempt"); t != "" && (g.OldestAttempt == "" || attemptBefore(t, g.OldestAttempt)) {
			g.OldestAttempt = t
		}
	}
	out := make([]triageGroup, 0, len(groups))
	for domain, g := range groups {
		g.AlreadyAllowed = g.Requests > 0 && covered[domain] == g.Requests
		if t, ok := parseAttempt(g.OldestAttempt); ok {
			g.AgeHours = int(now.Sub(t).Hours())
		}
		switch {
		case g.IsThreat:
			g.Hint = "deny"
		case g.AlreadyAllowed:
			g.Hint = "resolve-already-allowed"
		default:
			g.Hint = "review"
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}

// parseAttempt reads an ISO 8601 time_of_attempt with a colon or colon-less
// UTC offset.
func parseAttempt(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999Z0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// attemptBefore is a total order over time_of_attempt values: parseable ones
// by instant (offsets differ, so text order is wrong), all of them ahead of
// unparseable ones, which fall back to text order. Mixing instant and text
// comparisons pairwise would be non-transitive.
func attemptBefore(a, b string) bool {
	ta, okA := parseAttempt(a)
	tb, okB := parseAttempt(b)
	switch {
	case okA && okB && !ta.Equal(tb):
		return ta.Before(tb)
	case okA && okB:
		return a < b // same instant, different offsets: text breaks the tie
	case okA != okB:
		return okA
	default:
		return a < b
	}
}

func appendUnique(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
