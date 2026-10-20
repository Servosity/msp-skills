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
	// IsGlobal marks a global (MSP-shared) policy: one finding there
	// affects every client org that uses it.
	IsGlobal bool   `json:"is_global"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"` // high, medium, info
	Detail   string `json:"detail"`
}

type auditView struct {
	PoliciesScanned  int    `json:"policies_scanned"`
	DomainsLooked    int    `json:"allowlist_domains_categorized"`
	LookupsTruncated bool   `json:"lookups_truncated"`
	Baseline         string `json:"baseline,omitempty"`
	// UnclassifiedCategories lists catalog category ids with no boolean
	// security flag: their security status is unknown, so they were not
	// checked as security categories and the audit is not an all-clear.
	UnclassifiedCategories []string       `json:"unclassified_categories,omitempty"`
	Findings               []auditFinding `json:"findings"`
	Note                   string         `json:"note,omitempty"`
	CatalogEmpty           bool           `json:"catalog_empty,omitempty"`          // categories/all listed nothing: no security status is known
	NoSecurityCategories   bool           `json:"no_security_categories,omitempty"` // the catalog flags no category security=true: nothing was checked as a threat
	NoPolicies             bool           `json:"no_policies,omitempty"`            // no policy was listed: nothing was audited
	UnknownCategoryIDs     []string       `json:"unknown_category_ids,omitempty"`   // blocked category ids (policies, baseline) not in the catalog
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

Findings: threat_allowlisted (an allow-list entry is now in a security category), security_category_not_blocked, unknown_domains_allowed, allow_list_only, and with --baseline, baseline_category_missing / baseline_flag_differs. A policy (or --baseline policy) whose lists or flags are absent, null or unreadable gets a policy_attributes_unknown (baseline_attributes_unknown) finding naming them, and the checks that read them are not counted as passed. Allow-list entries are re-categorized live through the bulk domain lookup, capped by --max-lookups; an entry with no usable lookup record of its own is counted as unchecked.
Global (MSP-shared) policies are audited too, with or without --org; their findings carry is_global=true, and each policy is scanned once however many client orgs share it.
A catalog category without a boolean security flag is never treated as safe: it is listed in unclassified_categories and the note, and the audit is not an all-clear. The same holds for an empty category catalog (catalog_empty), a catalog with no security category (no_security_categories), no listed policies (no_policies) and blocked category ids the catalog does not contain (unknown_category_ids).`,
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
			// Global (MSP-shared) policies are listed only on request.
			params := map[string]string{"include_global_policies": "true"}
			if orgRefStr != "" {
				org, err := resolveOrg(ctx, c, orgRefStr)
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				params["organization_id"] = org.ID
			}
			listed, _, err := fetchAllPages(ctx, c, "/v1/policies/all", params, 0)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			// A global policy may be listed once per client org; audit and count it once.
			policies, seen := make([]jaRes, 0, len(listed)), map[string]bool{}
			for _, p := range listed {
				// A policy without an id cannot be a repeat of another one.
				if p.ID == "" || !seen[p.ID] {
					seen[p.ID] = true
					policies = append(policies, p)
				}
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
			// unusable: requested domains with an unusable record of their own,
			// or in a batch that returned a record naming no host (it could be
			// any of them).
			unusable := map[string]bool{}
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
					host := lookupRecordHost(l)
					// Only a well-formed record (host + category list) counts as
					// checked; anything else stays unchecked and is reported.
					ok, ids := domainRecordCategories(l)
					switch {
					case host == "":
						for _, d := range domains[i:end] {
							unusable[d] = true
						}
					case !ok:
						unusable[host] = true
					default:
						domainCats[host] = append(domainCats[host], ids...)
					}
				}
			}
			// HAND-FIX(policy-audit-lookup-coverage-by-membership): coverage is
			// counted per requested domain, not by comparing counts: a record
			// for a host nobody asked about must not stand in for a missing one.
			missing := 0
			for _, d := range domains {
				if _, ok := domainCats[d]; ok && !unusable[d] {
					view.DomainsLooked++
				} else {
					missing++
				}
			}
			if missing > 0 {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" %d allow-list domains returned no usable lookup record of their own and were not checked for threat categories.", missing))
			}
			view.PoliciesScanned = len(policies)
			view.Findings = auditPolicies(policies, cats, domainCats, baseline)
			if n := countFindings(view.Findings, "policy_attributes_unknown", "baseline_attributes_unknown"); n > 0 {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" %d policies carry absent, null or unreadable attributes (see the policy_attributes_unknown and baseline_attributes_unknown findings); the checks that read them did not run, so this audit is not an all-clear.", n))
			}
			if len(cats) == 0 {
				view.CatalogEmpty = true
				view.Note = strings.TrimSpace(view.Note + " The category catalog returned no categories, so no category's security status is known: blocked security categories and threat-categorized allow-list entries were not checked, and this audit is not an all-clear.")
			} else if ids := unknownBlockedCategories(policies, cats, baseline); len(ids) > 0 {
				view.UnknownCategoryIDs = ids
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" %d blocked category ids are not in the category catalog (%s); their security status is unknown, so this audit is not an all-clear.", len(ids), strings.Join(ids, ", ")))
			}
			unclassifiedIDs, unclassifiedLabels := unclassifiedCategories(cats)
			// (An unclassified category already makes the audit not an
			// all-clear, with its own note.)
			if len(cats) > 0 && len(unclassifiedIDs) == 0 && !anySecurityCategory(cats) {
				view.NoSecurityCategories = true
				view.Note = strings.TrimSpace(view.Note + " No category in the category catalog is flagged as a security category, so blocked security categories and threat-categorized allow-list entries were not checked, and this audit is not an all-clear.")
			}
			if len(policies) == 0 {
				view.NoPolicies = true
				view.Note = strings.TrimSpace(view.Note + " No policies were listed (check --org and the API key's scope), so nothing was audited and this audit is not an all-clear.")
			}
			if ids, labels := unclassifiedIDs, unclassifiedLabels; len(ids) > 0 {
				view.UnclassifiedCategories = ids
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" %d categories in the category catalog have no boolean security flag (%s); they were not checked as security categories, so this audit is not an all-clear.", len(ids), strings.Join(labels, ", ")))
			}
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
				if f.IsGlobal {
					f.Detail += " [global policy, shared across client orgs]"
				}
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

// countFindings counts the findings of the given kinds.
func countFindings(findings []auditFinding, kinds ...string) int {
	n := 0
	for _, f := range findings {
		for _, k := range kinds {
			if f.Kind == k {
				n++
			}
		}
	}
	return n
}

// anySecurityCategory reports whether the catalog flags any category security=true.
func anySecurityCategory(cats []jaRes) bool {
	for _, c := range cats {
		if c.boolean("security") {
			return true
		}
	}
	return false
}

// unclassifiedCategories returns the catalog categories whose security flag
// is absent or not a boolean (sorted ids, and "name (id)" labels). Unknown is
// not false: auditPolicies cannot tell whether such a category is a threat.
// Names are listed, not matched against a list of known threat names.
func unclassifiedCategories(cats []jaRes) (ids, labels []string) {
	byCat := map[string]string{}
	for _, c := range cats {
		if _, isBool := c.Attrs["security"].(bool); !isBool {
			byCat[c.ID] = c.str("name")
		}
	}
	ids = sortedKeys(byCat)
	labels = make([]string, 0, len(ids))
	for _, id := range ids {
		labels = append(labels, fmt.Sprintf("%s (%s)", byCat[id], id))
	}
	return ids, labels
}

// unknownBlockedCategories returns the blocked-category ids of the audited
// policies and the baseline that the category catalog does not contain.
func unknownBlockedCategories(policies, cats []jaRes, baseline *jaRes) []string {
	known := byID(cats)
	unknown := map[string]bool{}
	check := func(p jaRes) {
		for _, id := range p.strs("blacklist_categories") {
			if _, ok := known[id]; !ok {
				unknown[id] = true
			}
		}
	}
	for _, p := range policies {
		check(p)
	}
	if baseline != nil {
		check(*baseline)
	}
	return sortedKeys(unknown)
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
	// HAND-FIX(policy-audit-unknown-attributes): boolean() and strs() read an
	// absent or null attribute as false / empty, so a policy whose lists and
	// flags were unavailable audited clean. Each unreadable attribute is
	// listed in a policy_attributes_unknown finding instead, and the checks
	// that read it are not counted as passed.
	var baseBlocked []string
	baseUnknown := make([]string, 0)
	if baseline != nil {
		var ok bool
		if baseBlocked, ok = baseline.strsKnown("blacklist_categories"); !ok {
			baseUnknown = append(baseUnknown, "blacklist_categories")
		}
		for _, f := range baselineFlags {
			if _, ok := baseline.boolKnown(f); !ok {
				baseUnknown = append(baseUnknown, f)
			}
		}
		if _, ok := baseline.strKnown("youtube_restricted_level"); !ok {
			baseUnknown = append(baseUnknown, "youtube_restricted_level")
		}
	}
	for _, p := range policies {
		add := func(kind, sev, detail string) {
			out = append(out, auditFinding{PolicyID: p.ID, PolicyName: p.str("name"), OrgID: p.str("organization_id"), IsGlobal: p.boolean("is_global_policy"), Kind: kind, Severity: sev, Detail: detail})
		}
		unknown := make([]string, 0)
		allowList, ok := p.strsKnown("whitelist_domains")
		if !ok {
			unknown = append(unknown, "whitelist_domains")
		}
		for _, d := range allowList {
			for _, id := range domainCats[normalizeDomain(d)] {
				if security[id] {
					add("threat_allowlisted", "high", fmt.Sprintf("allow-list entry %q is now categorized %q", d, catName[id]))
					break
				}
			}
		}
		blockedList, blockedKnown := p.strsKnown("blacklist_categories")
		if !blockedKnown {
			unknown = append(unknown, "blacklist_categories")
		}
		blocked := map[string]bool{}
		for _, id := range blockedList {
			blocked[id] = true
		}
		// Allow-list-only mode blocks everything not allow-listed, so category
		// and unknown-domain exposure checks do not apply to it. Only a known
		// true switches them off.
		listOnly, listOnlyKnown := p.boolKnown("allow_list_only")
		if !listOnlyKnown {
			unknown = append(unknown, "allow_list_only")
		}
		missing := make([]string, 0)
		for _, id := range sortedKeys(security) {
			if !blocked[id] {
				missing = append(missing, catName[id])
			}
		}
		if blockedKnown && len(missing) > 0 && !listOnly {
			add("security_category_not_blocked", "high", "not blocking: "+strings.Join(missing, ", "))
		}
		allowUnknown, allowUnknownKnown := p.boolKnown("allow_unknown_domains")
		if !allowUnknownKnown {
			unknown = append(unknown, "allow_unknown_domains")
		}
		if allowUnknown && !listOnly {
			add("unknown_domains_allowed", "medium", "uncategorized domains are allowed")
		}
		if listOnly {
			add("allow_list_only", "info", "policy blocks everything not on its allow list")
		}
		if baseline != nil && baseline.ID != p.ID {
			if blockedKnown {
				for _, id := range baseBlocked {
					if !blocked[id] {
						add("baseline_category_missing", "medium", fmt.Sprintf("baseline blocks %q (%s); this policy does not", catName[id], id))
					}
				}
			}
			for _, f := range baselineFlags {
				pv, pKnown := p.boolKnown(f)
				bv, bKnown := baseline.boolKnown(f)
				if !pKnown {
					unknown = appendUnique(unknown, f)
				}
				if pKnown && bKnown && pv != bv {
					add("baseline_flag_differs", "info", fmt.Sprintf("%s=%v (baseline %v)", f, pv, bv))
				}
			}
			pl, pKnown := p.strKnown("youtube_restricted_level")
			bl, bKnown := baseline.strKnown("youtube_restricted_level")
			if !pKnown {
				unknown = append(unknown, "youtube_restricted_level")
			}
			if pKnown && bKnown && pl != bl {
				add("baseline_flag_differs", "info", fmt.Sprintf("youtube_restricted_level=%q (baseline %q)", pl, bl))
			}
		}
		if len(unknown) > 0 {
			add("policy_attributes_unknown", "medium", fmt.Sprintf("absent, null or unreadable: %s; the checks that read them did not run, so this policy is not clean", strings.Join(unknown, ", ")))
		}
	}
	if len(baseUnknown) > 0 {
		out = append(out, auditFinding{PolicyID: baseline.ID, PolicyName: baseline.str("name"), OrgID: baseline.str("organization_id"), IsGlobal: baseline.boolean("is_global_policy"), Kind: "baseline_attributes_unknown", Severity: "medium", Detail: fmt.Sprintf("the --baseline policy has absent, null or unreadable %s; drift on them was not compared", strings.Join(baseUnknown, ", "))})
	}
	rank := map[string]int{"high": 0, "medium": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}
