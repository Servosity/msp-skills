// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"dnsfilter-pp-cli/internal/client"
	"dnsfilter-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type allowTarget struct {
	PolicyID   string   `json:"policy_id"`
	PolicyName string   `json:"policy_name"`
	UsedBy     []string `json:"used_by"`
	Action     string   `json:"action"` // add, noop, skip, review (assignment fallback: not written)
	Reason     string   `json:"reason"`
	Result     string   `json:"result,omitempty"` // success, failure, noop (after --confirm)
}

type allowView struct {
	FQDN        string        `json:"fqdn"`
	Org         orgRef        `json:"org"`
	Categories  []categoryRef `json:"categories"`
	Attribution string        `json:"attribution"` // query_log or assignment
	Note        string        `json:"note"`
	Applied     bool          `json:"applied"`
	Targets     []allowTarget `json:"targets"`
	Notes       []string      `json:"notes,omitempty"`
}

func newNovelAllowCmd(flags *rootFlags) *cobra.Command {
	var flagOrg, flagHost, flagNote, flagSince string
	var flagForce, flagIncludeGlobal, flagConfirm bool
	var maxScanPages int

	cmd := &cobra.Command{
		Use:   "allow [fqdn]",
		Short: "Allow a domain on the policies that actually block it for a client (preview by default)",
		Long: `Use this command to allow a domain for one client org by adding it to the allow list of the policies that actually govern that client, with a threat-category guard and a read-back receipt.
Do NOT use this command to find out why a domain is blocked; use 'why-blocked' instead.
Do NOT use this command to work through the queue of end-user unblock requests; use 'unblocks triage' instead.

Without --confirm the command resolves and prints the plan; nothing is written. With --confirm (and --note) it adds the domain via add_whitelist_domain (one policy) or the bulk allow-list endpoint (several), then re-reads each policy and exits 6 if any target failed.

Targets are the policies that blocked this exact domain in recent query-log hits for the org (--since window); when there are no hits, the site (or --host roaming client) assignments are used instead and the output says attribution=assignment. Then only an assigned policy whose evaluation blocks the domain is a target: one that already allows it is a noop, and one whose verdict is unknown (an unreadable policy attribute, unknown categories, an unverified parent-domain entry) gets action review and is never written. Refused unless --force: a domain in a security (threat) category or in a category this key cannot resolve. Skipped: global policies unless --include-global, policies whose can_edit is not true, policies whose allow list is unreadable, and deployments whose current scheduled-policy slot cannot be resolved.`,
		Example: strings.Trim(`
  dnsfilter-cli allow example.com --org 'Acme Dental' --note 'TKT-4821'
  dnsfilter-cli allow example.com --org 'Acme Dental' --note 'TKT-4821' --confirm --agent`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "pp:preview-happy-path": "true", "pp:happy-args": "fqdn=example.com;--org=example-value;--note=example-value", "pp:typed-exit-codes": "0,2,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "allow")
			}
			if err := rejectExtraArgs(args, 1); err != nil {
				return err
			}
			if len(args) == 0 || normalizeDomain(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a domain argument is required, e.g. allow example.com --org 'Acme Dental' --note 'TKT-4821'"))
			}
			if strings.TrimSpace(flagOrg) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--org is required (organization name or id)"))
			}
			if flagConfirm && strings.TrimSpace(flagNote) == "" {
				return usageErr(fmt.Errorf("--note is required with --confirm (DNSFilter stores it with the allow-list entry)"))
			}
			since, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				return usageErr(fmt.Errorf("--since: %w", err))
			}
			if maxScanPages < 1 {
				return usageErr(fmt.Errorf("--max-scan-pages must be >= 1"))
			}
			if flagConfirm && cliutil.IsAnyHarness() {
				return writeHarnessRefusal(cmd.OutOrStdout(), flags, "add allow-list entry")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			now := time.Now()
			bc, err := resolveBlockContext(ctx, c, args[0], flagOrg, flagHost, maxScanPages, now)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			if !bc.CategoriesKnown && !flagForce {
				return usageErr(fmt.Errorf("domain lookup for %s returned no domain record, so its categories (and threat status) are unknown; refusing to allow it without --force", bc.FQDN))
			}
			for _, cat := range bc.Categories {
				if (cat.Security || cat.Unknown) && !flagForce {
					kind := "security category"
					if cat.Unknown {
						kind = "category this key cannot resolve"
					}
					return usageErr(fmt.Errorf("%s is in %s %q (%s); refusing to allow it without --force", bc.FQDN, kind, cat.Name, cat.ID))
				}
			}

			agentIDs := make([]string, 0)
			for _, d := range bc.Deployments {
				if d.Kind == "roaming_client" {
					agentIDs = append(agentIDs, d.ID)
				}
			}
			hits, complete, err := fetchBlockedHits(ctx, c, bc.Org.ID, bc.FQDN, agentIDs, since, now)
			if err != nil {
				// A partial scan must not pick targets: fall back to assignments
				// entirely, exactly as the note says.
				hits = nil
				bc.Notes = append(bc.Notes, fmt.Sprintf("query log unavailable, using assignments: %v", err))
			} else if !complete {
				bc.Notes = append(bc.Notes, fmt.Sprintf("query-log scan stopped after %d pages; attribution may be incomplete", queryLogScanPages))
			}
			blockedBy := map[string][]string{}
			for _, h := range hits {
				if h.PolicyID != "" {
					blockedBy[h.PolicyID] = appendUnique(blockedBy[h.PolicyID], "query-log source "+h.Source)
				}
			}
			for id := range blockedBy {
				if _, err := ensurePolicy(ctx, c, bc.Policies, id); err != nil {
					bc.Notes = append(bc.Notes, fmt.Sprintf("policy %s from the query log is unreadable: %v", id, err))
				}
			}

			view := allowView{FQDN: bc.FQDN, Org: bc.Org, Categories: bc.Categories, Note: flagNote, Notes: bc.Notes}
			if len(blockedBy) > 0 {
				view.Attribution = "query_log"
				view.Targets = planAllowTargets(bc.FQDN, bc.Policies, blockedBy, flagIncludeGlobal)
			} else {
				view.Attribution = "assignment"
				view.Targets = planAssignmentTargets(bc, flagIncludeGlobal)
				for _, d := range bc.Deployments {
					if d.PolicyID == "" {
						view.Targets = append(view.Targets, allowTarget{UsedBy: []string{d.Kind + " " + d.Name}, Action: "skip", Reason: "effective policy unresolved (" + d.Via + ")"})
					}
				}
			}

			toAdd, review := make([]string, 0), 0
			for _, t := range view.Targets {
				switch t.Action {
				case "add":
					toAdd = append(toAdd, t.PolicyID)
				case "review":
					review++
				}
			}
			if review > 0 {
				view.Notes = append(view.Notes, fmt.Sprintf("%d assigned policies could not be ruled out as blocking %s (action review) and are never written; check them, or re-run once the query log shows the block", review, bc.FQDN))
			}
			var failed int
			switch {
			case flagConfirm && len(toAdd) > 0:
				result, err := applyAllow(ctx, c, bc.FQDN, flagNote, toAdd)
				if err != nil {
					return classifyAPIErrorOnly(err)
				}
				view.Applied = true
				failed = reconcileAllow(ctx, c, &view, result)
			case len(toAdd) > 0:
				view.Notes = append(view.Notes, "preview only; re-run with --confirm --note '<ticket>' to apply")
			}

			if err := renderAllow(cmd, flags, view); err != nil {
				return err
			}
			if failed > 0 {
				return partialFailureErr(fmt.Errorf("%d of %d allow-list writes did not take effect", failed, len(toAdd)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagOrg, "org", "", "Client organization name or id to allow the domain for")
	cmd.Flags().StringVar(&flagHost, "host", "", "Only consider this roaming client (hostname or friendly name)")
	cmd.Flags().StringVar(&flagNote, "note", "", "Note stored with the allow-list entry, e.g. the ticket number (required with --confirm)")
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Query-log window used to find the policies that blocked the domain (e.g. 24h, 7d)")
	cmd.Flags().BoolVar(&flagConfirm, "confirm", false, "Apply the change; without it the command only prints the plan")
	cmd.Flags().BoolVar(&flagForce, "force", false, "Allow a domain even when it is in a security (threat) or unresolvable category")
	cmd.Flags().BoolVar(&flagIncludeGlobal, "include-global", false, "Also edit global policies, which can be shared across client orgs")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum 100-row pages of sites or roaming clients to scan")
	return cmd
}

func hasBool(r jaRes, key string) bool {
	_, ok := r.Attrs[key].(bool)
	return ok
}

// assignmentUse maps each resolved effective policy to the deployments using it.
func assignmentUse(deps []deployment) map[string][]string {
	use := map[string][]string{}
	for _, d := range deps {
		if d.PolicyID != "" {
			use[d.PolicyID] = append(use[d.PolicyID], d.Kind+" "+d.Name)
		}
	}
	return use
}

// planAssignmentTargets plans the assignment fallback (no query-log hit names
// a policy). HAND-FIX(allow-assignment-targets-blocking-policies): it used to
// add the domain to every editable assigned policy, including policies that
// already allow it. Now only a policy whose evaluation blocks the domain is a
// candidate; one that allows it is a noop, and one whose verdict is unknown
// is left for review and never written.
func planAssignmentTargets(bc *blockContext, includeGlobal bool) []allowTarget {
	use := assignmentUse(bc.Deployments)
	blocking := map[string][]string{}
	others := make([]allowTarget, 0)
	for _, id := range sortedKeys(use) {
		p, readable := bc.Policies[id]
		if !readable {
			blocking[id] = use[id] // planAllowTargets skips it as unreadable
			continue
		}
		v := evaluatePolicy(bc.FQDN, p, bc.Categories, bc.CategoriesKnown)
		t := allowTarget{PolicyID: id, PolicyName: p.str("name"), UsedBy: use[id]}
		switch v.Decision {
		case "blocked":
			blocking[id] = use[id]
			continue
		case "allowed":
			t.Action, t.Reason = "noop", fmt.Sprintf("policy already allows the domain (%s: %s)", v.Rule, v.Detail)
		default:
			t.Action, t.Reason = "review", fmt.Sprintf("cannot rule out that this policy blocks the domain (%s: %s); not written, check it in the dashboard or re-run once the query log shows the block", v.Rule, v.Detail)
		}
		others = append(others, t)
	}
	targets := append(planAllowTargets(bc.FQDN, bc.Policies, blocking, includeGlobal), others...)
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].PolicyID < targets[j].PolicyID })
	return targets
}

// planAllowTargets decides add / noop / skip for each candidate policy.
func planAllowTargets(fqdn string, policies map[string]jaRes, use map[string][]string, includeGlobal bool) []allowTarget {
	targets := make([]allowTarget, 0, len(use))
	for _, id := range sortedKeys(use) {
		p, readable := policies[id]
		t := allowTarget{PolicyID: id, PolicyName: p.str("name"), UsedBy: use[id], Action: "add", Reason: "domain not on this policy's allow list"}
		canEdit, _ := p.Attrs["can_edit"].(bool)
		allowList, allowKnown := p.strsKnown("whitelist_domains")
		switch {
		case !readable:
			t.Action, t.Reason = "skip", "policy not readable with this API key"
		case hasExactEntry(allowList, fqdn):
			t.Action, t.Reason = "noop", fmt.Sprintf("already on the allow list as %q", fqdn)
		case !allowKnown:
			// "Not on the allow list" needs a readable list, and so does
			// the read-back that proves the write.
			t.Action, t.Reason = "skip", "policy's whitelist_domains is absent, null or unreadable, so the plan cannot tell whether the domain is already listed or read the write back"
		case !hasBool(p, "is_global_policy") && !includeGlobal:
			t.Action, t.Reason = "skip", "policy does not report is_global_policy, so it may be shared across client orgs; pass --include-global to edit it"
		case p.boolean("is_global_policy") && !includeGlobal:
			t.Action, t.Reason = "skip", "global policy can be shared across client orgs; pass --include-global to edit it"
		case !canEdit:
			t.Action, t.Reason = "skip", "policy does not report can_edit=true for this API key"
		}
		targets = append(targets, t)
	}
	return targets
}

type bulkResult struct {
	Success []int `json:"success"`
	Failure []int `json:"failure"`
	Noop    []int `json:"noop"`
}

// applyAllow writes the entry: the single-policy endpoint for one target, the
// bulk allow-list endpoint (domain -> note map) for several. The bulk result
// is returned so per-policy failures are not lost.
func applyAllow(ctx context.Context, c *client.Client, fqdn, note string, policyIDs []string) (*bulkResult, error) {
	if len(policyIDs) == 1 {
		_, _, err := c.Post(ctx, "/v1/policies/"+url.PathEscape(policyIDs[0])+"/add_whitelist_domain", map[string]any{"domain": fqdn, "note": note})
		return nil, err
	}
	ids := make([]int, 0, len(policyIDs))
	for _, id := range policyIDs {
		n, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("policy id %q is not numeric", id)
		}
		ids = append(ids, n)
	}
	raw, _, err := c.Post(ctx, "/v1/policies/bulk/add_allowlist_domains", map[string]any{"domains": []string{fqdn}, "notes": map[string]string{fqdn: note}, "policy_ids": ids})
	if err != nil {
		return nil, err
	}
	var res bulkResult
	if json.Unmarshal(raw, &res) != nil {
		return nil, nil // unparseable acknowledgement: rely on the read-back
	}
	return &res, nil
}

// reconcileAllow marks each added target success/failure from the bulk result
// and a fresh read-back, returning the failure count.
func reconcileAllow(ctx context.Context, c *client.Client, view *allowView, bulk *bulkResult) int {
	reported := map[string]string{}
	if bulk != nil {
		for _, id := range bulk.Failure {
			reported[strconv.Itoa(id)] = "failure"
		}
		for _, id := range bulk.Noop {
			reported[strconv.Itoa(id)] = "noop"
		}
	}
	failed := 0
	for i := range view.Targets {
		t := &view.Targets[i]
		if t.Action != "add" {
			continue
		}
		t.Result = "failure"
		raw, err := c.GetNoCache(ctx, "/v1/policies/"+url.PathEscape(t.PolicyID), nil)
		if err != nil {
			t.Reason = fmt.Sprintf("read-back failed: %v", err)
		} else if reported[t.PolicyID] == "failure" {
			t.Reason = "bulk endpoint reported failure for this policy"
			if list, _ := decodeJA(raw); len(list) == 1 && list[0].ID == t.PolicyID && hasExactEntry(list[0].strs("whitelist_domains"), view.FQDN) {
				t.Reason += " (a covering entry is present on read-back; the note may not have been saved)"
			}
		} else if list, _ := decodeJA(raw); len(list) == 1 && list[0].ID == t.PolicyID && hasExactEntry(list[0].strs("whitelist_domains"), view.FQDN) {
			t.Result = "success"
			if reported[t.PolicyID] == "noop" {
				t.Result = "noop"
			}
		} else {
			t.Reason = "entry not on the allow list after the write"
		}
		if t.Result == "failure" {
			failed++
		}
	}
	return failed
}

func renderAllow(cmd *cobra.Command, flags *rootFlags, view allowView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		// Keep the per-policy receipt even when --agent/--compact would drop
		// fields that only some targets carry.
		return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "targets", "policy_id", "action", "reason", "result", "applied", "attribution")
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "allow %s for %s (org %s) applied=%v attribution=%s\n", humanText(view.FQDN), humanText(view.Org.Name), humanText(view.Org.ID), view.Applied, humanText(view.Attribution))
	for _, t := range view.Targets {
		fmt.Fprintf(w, "  %-5s policy %s %q used by %s: %s %s\n", humanText(t.Action), humanText(t.PolicyID), t.PolicyName, humanText(strings.Join(t.UsedBy, ", ")), humanText(t.Reason), humanText(t.Result))
	}
	for _, n := range view.Notes {
		fmt.Fprintf(w, "note: %s\n", humanText(n))
	}
	return nil
}
