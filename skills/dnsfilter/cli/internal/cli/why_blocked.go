// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type whyBlockedDeployment struct {
	deployment
	Verdict verdict `json:"verdict"`
}

type queryLogHit struct {
	Time       string   `json:"time"`
	Result     string   `json:"result"`
	FQDN       string   `json:"fqdn"`
	PolicyID   string   `json:"policy_id"`
	PolicyName string   `json:"policy_name"`
	MethodName string   `json:"method_name"`
	Categories []string `json:"categories"`
	Source     string   `json:"source"`
}

type whyBlockedView struct {
	FQDN        string                 `json:"fqdn"`
	Org         orgRef                 `json:"org"`
	Blocked     bool                   `json:"blocked"`
	Categories  []categoryRef          `json:"categories"`
	Deployments []whyBlockedDeployment `json:"deployments"`
	RecentHits  []queryLogHit          `json:"recent_hits"`
	// BlockingPolicyIDs are the policies recent hits were actually blocked by.
	BlockingPolicyIDs []string `json:"blocking_policy_ids"`
	Notes             []string `json:"notes,omitempty"`
}

func newNovelWhyBlockedCmd(flags *rootFlags) *cobra.Command {
	var flagOrg, flagHost, flagSince string
	var maxScanPages, maxHits int

	cmd := &cobra.Command{
		Use:   "why-blocked [fqdn]",
		Short: "Name the policy and rule that blocks (or allows) a domain for one client org or one roaming client",
		Long: `Use this command to diagnose why one domain is blocked (or allowed) for one client org or one roaming client: it names the governing policy, the rule that decided it, and recent blocked query-log hits. Read-only.
Do NOT use this command to change a policy; use 'allow' instead.
Do NOT use this command to find every client whose allow or block lists contain a domain; use 'lists where' instead.

Each site (or, with --host, roaming client) is evaluated against the policy it uses right now: a scheduled policy's current 15-minute slot is resolved in the schedule's timezone, and a roaming client without its own assignment inherits its site's. Rules follow DNSFilter's documented policy order: allow list, allow-list-only mode, block list, uncategorized domains, blocked categories. Recent blocked query-log hits (exact domain match) name the policy that actually applied.

Not evaluated (not exposed by the policy API): user, collection and LAN-subnet policy assignments, the org/MSP Universal Allow and Block Lists, and AppAware application rules. When a hit's policy is not one of the site or roaming-client policies, the output says so.`,
		Example: strings.Trim(`
  dnsfilter-cli why-blocked example.com --org 'Acme Dental' --agent
  dnsfilter-cli why-blocked cdn.example.com --org 4821 --host FRONTDESK-01 --since 48h`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "fqdn=example.com;--org=example-value"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "why-blocked")
			}
			if err := rejectExtraArgs(args, 1); err != nil {
				return err
			}
			if len(args) == 0 || normalizeDomain(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a domain argument is required, e.g. why-blocked example.com --org 'Acme Dental'"))
			}
			if strings.TrimSpace(flagOrg) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--org is required (organization name or id)"))
			}
			since, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				return usageErr(fmt.Errorf("--since: %w", err))
			}
			if maxHits < 0 || maxScanPages < 1 {
				return usageErr(fmt.Errorf("--max-hits must be >= 0 and --max-scan-pages >= 1"))
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
			now := time.Now()
			bc, err := resolveBlockContext(ctx, c, args[0], flagOrg, flagHost, maxScanPages, now)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			view := whyBlockedView{FQDN: bc.FQDN, Org: bc.Org, Categories: bc.Categories, Deployments: make([]whyBlockedDeployment, 0, len(bc.Deployments)), RecentHits: make([]queryLogHit, 0), Notes: bc.Notes}
			if view.Categories == nil {
				view.Categories = make([]categoryRef, 0)
			}
			for _, d := range bc.Deployments {
				row := whyBlockedDeployment{deployment: d}
				if p, ok := bc.Policies[d.PolicyID]; ok && d.PolicyID != "" {
					row.Verdict = evaluatePolicy(bc.FQDN, p, bc.Categories, bc.CategoriesKnown)
				} else {
					row.Verdict = verdict{"unknown", "no_policy", "effective policy could not be resolved (" + d.Via + ")"}
				}
				if row.Verdict.Decision == "blocked" {
					view.Blocked = true
				}
				view.Deployments = append(view.Deployments, row)
			}

			agentIDs := make([]string, 0)
			assigned := map[string]bool{}
			for _, d := range bc.Deployments {
				if d.Kind == "roaming_client" {
					agentIDs = append(agentIDs, d.ID)
				}
				assigned[d.PolicyID] = true
			}
			view.BlockingPolicyIDs = make([]string, 0)
			if hits, complete, err := fetchBlockedHits(ctx, c, bc.Org.ID, bc.FQDN, agentIDs, since, now); err != nil {
				view.Notes = append(view.Notes, fmt.Sprintf("query log unavailable: %v", err))
			} else {
				if !complete {
					view.Notes = append(view.Notes, fmt.Sprintf("query-log scan stopped after %d pages; older hits may name other blocking policies", queryLogScanPages))
				}
				view.RecentHits = hits
				if len(view.RecentHits) > maxHits {
					view.RecentHits = view.RecentHits[:maxHits]
				}
				for _, h := range hits {
					view.Blocked = true
					if h.PolicyID != "" {
						view.BlockingPolicyIDs = appendUnique(view.BlockingPolicyIDs, h.PolicyID)
					}
				}
				for _, id := range view.BlockingPolicyIDs {
					if !assigned[id] {
						view.Notes = append(view.Notes, fmt.Sprintf("recent hits were blocked by policy %s, which is not a site or roaming-client assignment shown here (likely a user, collection or LAN-subnet assignment)", id))
					}
				}
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			cats := make([]string, 0, len(view.Categories))
			for _, c := range view.Categories {
				cats = append(cats, c.Name)
			}
			fmt.Fprintf(w, "%s for %s (org %s)  categories: %s\n", humanText(view.FQDN), humanText(view.Org.Name), humanText(view.Org.ID), humanText(strings.Join(cats, ", ")))
			for _, d := range view.Deployments {
				fmt.Fprintf(w, "  %-8s %-14s %s via %s\n    -> %s: %s\n", humanText(strings.ToUpper(d.Verdict.Decision)), humanText(d.Kind), humanText(d.Name), humanText(d.Via), humanText(d.Verdict.Rule), humanText(d.Verdict.Detail))
			}
			fmt.Fprintf(w, "recent blocked hits (%s): %d\n", humanText(flagSince), len(view.RecentHits))
			for _, h := range view.RecentHits {
				fmt.Fprintf(w, "  %s %s policy=%q method=%q %s\n", humanText(h.Time), humanText(h.FQDN), h.PolicyName, h.MethodName, humanText(h.Source))
			}
			for _, n := range view.Notes {
				fmt.Fprintf(w, "note: %s\n", humanText(n))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagOrg, "org", "", "Client organization name or id the request came from")
	cmd.Flags().StringVar(&flagHost, "host", "", "Roaming client hostname or friendly name (checks that device instead of the org's sites)")
	cmd.Flags().StringVar(&flagSince, "since", "24h", "How far back to search the query log for blocked hits (e.g. 24h, 7d)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum 100-row pages of sites or roaming clients to scan")
	cmd.Flags().IntVar(&maxHits, "max-hits", 20, "Maximum recent query-log hits to return")
	return cmd
}

// parseQueryLogHits decodes {data:{values:[...]}}; a response without a values
// list is an error, never an empty success.
func parseQueryLogHits(raw []byte) ([]queryLogHit, error) {
	hits := make([]queryLogHit, 0)
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Data == nil {
		return hits, fmt.Errorf("response has no data object")
	}
	list, err := decodeJA(raw)
	if err != nil {
		return hits, err
	}
	if len(list) == 0 {
		return hits, fmt.Errorf("response has no data object")
	}
	for _, block := range list {
		values, isList := block.Attrs["values"].([]any)
		if !isList {
			return hits, fmt.Errorf("response data has no values list")
		}
		for i, v := range values {
			m, ok := v.(map[string]any)
			if !ok {
				return hits, fmt.Errorf("query-log entry %d is not an object", i)
			}
			r := toJARes(m)
			src := r.str("agentname")
			if src == "" {
				src = r.str("network_name")
			}
			hits = append(hits, queryLogHit{Time: r.str("time"), Result: r.str("result"), FQDN: r.str("fqdn"), PolicyID: r.str("policy_id"), PolicyName: r.str("policy_name"), MethodName: r.str("method_name"), Categories: r.strs("categories_names"), Source: src})
		}
	}
	return hits, nil
}
