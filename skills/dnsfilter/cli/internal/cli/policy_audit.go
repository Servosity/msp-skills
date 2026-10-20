// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"dnsfilter-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type auditFinding struct {
	PolicyID   string `json:"policy_id"`
	PolicyName string `json:"policy_name"`
	OrgID      string `json:"org_id"`
	Kind       string `json:"kind"`
	Severity   string `json:"severity"` // high, medium, info
	Detail     string `json:"detail"`
}

type auditView struct {
	PoliciesScanned  int            `json:"policies_scanned"`
	DomainsLooked    int            `json:"allowlist_domains_categorized"`
	LookupsTruncated bool           `json:"lookups_truncated"`
	Baseline         string         `json:"baseline,omitempty"`
	Findings         []auditFinding `json:"findings"`
	Note             string         `json:"note,omitempty"`
}

// ponytail: bulk_lookup takes a comma-separated fqdns list; 50 per call keeps URLs short.
const auditLookupBatch = 50

func newNovelPolicyAuditCmd(flags *rootFlags) *cobra.Command {
	var orgRefStr, baselineID string
	var maxLookups int

	cmd := &cobra.Command{
		Use:   "policy-audit",
		Short: "Flag threat-categorized allow-list entries, unblocked security categories, and baseline drift",
		Long: `Use this command for a security hygiene sweep of every policy: threat-categorized allow-list entries, unblocked security categories, unknown domains allowed, and drift from a --baseline policy.
Do NOT use this command to look up one domain across the fleet; use 'lists where' instead.

Findings: threat_allowlisted (an allow-list entry is now in a security category), security_category_not_blocked, unknown_domains_allowed, allow_list_only, and with --baseline, baseline_category_missing / baseline_flag_differs. Allow-list entries are re-categorized live through the bulk domain lookup, capped by --max-lookups.`,
		Example: strings.Trim(`
  dnsfilter-cli policy-audit --agent
  dnsfilter-cli policy-audit --org 'Acme Dental' --baseline 1201`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "policy-audit")
			}
			if err := rejectExtraArgs(args, 0); err != nil {
				return err
			}
			if cliutil.IsDogfoodEnv() && maxLookups > auditLookupBatch {
				maxLookups = auditLookupBatch
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			params := map[string]string{}
			if orgRefStr != "" {
				org, err := resolveOrg(ctx, c, orgRefStr)
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				params["organization_id"] = org.ID
			}
			policies, _, err := fetchAllPages(ctx, c, "/v1/policies/all", params, 0)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			cats, _, err := fetchAllPages(ctx, c, "/v1/categories/all", nil, 0)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			var baseline *jaRes
			if baselineID != "" {
				if _, err := strconv.Atoi(baselineID); err != nil {
					return usageErr(fmt.Errorf("--baseline must be a numeric policy id, got %q", baselineID))
				}
				raw, err := c.GetNoCache(ctx, "/v1/policies/"+url.PathEscape(baselineID), nil)
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				list, err := decodeJA(raw)
				if err != nil {
					return apiErr(fmt.Errorf("--baseline policy %s: %w", baselineID, err))
				}
				if len(list) != 1 || list[0].ID != baselineID {
					return usageErr(fmt.Errorf("--baseline policy %s not found", baselineID))
				}
				baseline = &list[0]
			}

			// Re-categorize the de-duplicated allow-list entries.
			uniq := map[string]bool{}
			for _, p := range policies {
				for _, d := range p.strs("whitelist_domains") {
					uniq[normalizeDomain(d)] = true
				}
			}
			domains := sortedKeys(uniq)
			view := auditView{Baseline: baselineID, Findings: make([]auditFinding, 0)}
			if maxLookups >= 0 && len(domains) > maxLookups {
				domains, view.LookupsTruncated = domains[:maxLookups], true
			}
			domainCats := map[string][]string{}
			for i := 0; i < len(domains); i += auditLookupBatch {
				end := min(i+auditLookupBatch, len(domains))
				raw, err := c.GetNoCache(ctx, "/v1/domains/bulk_lookup", map[string]string{"fqdns": strings.Join(domains[i:end], ",")})
				if err != nil {
					return classifyAPIErrorOnly(fmt.Errorf("bulk domain lookup: %w", err))
				}
				list, err := decodeJA(raw)
				if err != nil {
					return apiErr(fmt.Errorf("bulk domain lookup: %w", err))
				}
				for _, l := range list {
					host := normalizeDomain(l.str("host"))
					if host == "" {
						host = normalizeDomain(l.str("name"))
					}
					// Only a well-formed record (host + category list) counts as
					// checked; anything else stays unchecked and is reported.
					if ok, ids := domainRecordCategories(l); ok && host != "" {
						domainCats[host] = append(domainCats[host], ids...)
					}
				}
			}
			view.DomainsLooked = len(domainCats)
			if missing := len(domains) - len(domainCats); missing > 0 {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" %d allow-list domains returned no usable lookup record and were not checked for threat categories.", missing))
			}
			view.PoliciesScanned = len(policies)
			view.Findings = auditPolicies(policies, cats, domainCats, baseline)
			if n := unresolvedCategoryDomains(domainCats, cats); n > 0 {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" %d allow-list domains carry category ids missing from the category catalog or without a security flag; their threat check is incomplete.", n))
			}
			if view.LookupsTruncated {
				view.Note = strings.TrimSpace(view.Note + " " + fmt.Sprintf("only the first %d allow-list domains were categorized; raise --max-lookups to cover the rest", maxLookups))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d policies scanned, %d allow-list domains categorized, %d findings\n", view.PoliciesScanned, view.DomainsLooked, len(view.Findings))
			for _, f := range view.Findings {
				fmt.Fprintf(w, "  %-6s %-30s policy %s %q: %s\n", humanText(f.Severity), humanText(f.Kind), humanText(f.PolicyID), f.PolicyName, humanText(f.Detail))
			}
			if view.Note != "" {
				fmt.Fprintln(w, "note:", humanText(view.Note))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgRefStr, "org", "", "Only audit this client org's policies (name or id)")
	cmd.Flags().StringVar(&baselineID, "baseline", "", "Policy id whose blocked categories and safety flags every policy should match")
	cmd.Flags().IntVar(&maxLookups, "max-lookups", 500, "Maximum distinct allow-list domains to re-categorize")
	return cmd
}

// unresolvedCategoryDomains counts looked-up domains with a category id the
// catalog does not contain (threat status unknown).
func unresolvedCategoryDomains(domainCats map[string][]string, cats []jaRes) int {
	known := byID(cats)
	n := 0
	for _, ids := range domainCats {
		for _, id := range ids {
			cat, ok := known[id]
			if _, isBool := cat.Attrs["security"].(bool); !ok || !isBool {
				n++
				break
			}
		}
	}
	return n
}

// baselineFlags are the policy switches compared against --baseline.
var baselineFlags = []string{"allow_unknown_domains", "allow_list_only", "google_safesearch", "bing_safe_search", "duck_duck_go_safe_search", "ecosia_safesearch", "yandex_safe_search", "youtube_restricted", "interstitial"}

// auditPolicies is the pure rule set behind `policy-audit`.
func auditPolicies(policies, cats []jaRes, domainCats map[string][]string, baseline *jaRes) []auditFinding {
	catName, security := map[string]string{}, map[string]bool{}
	for _, c := range cats {
		catName[c.ID] = c.str("name")
		if c.boolean("security") {
			security[c.ID] = true
		}
	}
	out := make([]auditFinding, 0)
	for _, p := range policies {
		add := func(kind, sev, detail string) {
			out = append(out, auditFinding{PolicyID: p.ID, PolicyName: p.str("name"), OrgID: p.str("organization_id"), Kind: kind, Severity: sev, Detail: detail})
		}
		for _, d := range p.strs("whitelist_domains") {
			for _, id := range domainCats[normalizeDomain(d)] {
				if security[id] {
					add("threat_allowlisted", "high", fmt.Sprintf("allow-list entry %q is now categorized %q", d, catName[id]))
					break
				}
			}
		}
		blocked := map[string]bool{}
		for _, id := range p.strs("blacklist_categories") {
			blocked[id] = true
		}
		// Allow-list-only mode blocks everything not allow-listed, so category
		// and unknown-domain exposure checks do not apply to it.
		listOnly := p.boolean("allow_list_only")
		missing := make([]string, 0)
		for _, id := range sortedKeys(security) {
			if !blocked[id] {
				missing = append(missing, catName[id])
			}
		}
		if len(missing) > 0 && !listOnly {
			add("security_category_not_blocked", "high", "not blocking: "+strings.Join(missing, ", "))
		}
		if p.boolean("allow_unknown_domains") && !listOnly {
			add("unknown_domains_allowed", "medium", "uncategorized domains are allowed")
		}
		if p.boolean("allow_list_only") {
			add("allow_list_only", "info", "policy blocks everything not on its allow list")
		}
		if baseline != nil && baseline.ID != p.ID {
			for _, id := range baseline.strs("blacklist_categories") {
				if !blocked[id] {
					add("baseline_category_missing", "medium", fmt.Sprintf("baseline blocks %q (%s); this policy does not", catName[id], id))
				}
			}
			for _, f := range baselineFlags {
				if p.boolean(f) != baseline.boolean(f) {
					add("baseline_flag_differs", "info", fmt.Sprintf("%s=%v (baseline %v)", f, p.boolean(f), baseline.boolean(f)))
				}
			}
			if p.str("youtube_restricted_level") != baseline.str("youtube_restricted_level") {
				add("baseline_flag_differs", "info", fmt.Sprintf("youtube_restricted_level=%q (baseline %q)", p.str("youtube_restricted_level"), baseline.str("youtube_restricted_level")))
			}
		}
	}
	rank := map[string]int{"high": 0, "medium": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}
