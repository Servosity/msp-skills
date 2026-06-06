// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// audit coverage: flags services whose escalation chain would page nobody. It
// joins services → escalation policies → schedules in the local store and
// applies structural rules (no policy, empty policy, empty tier, empty
// schedule, single point of failure). PagerDuty ships no coverage-gap report;
// this reconstructs one from the synced topology. Registered as a subcommand of
// the `audit` (audit-trail records) command so the natural `audit coverage`
// path is preserved.
package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type pdCoverageGap struct {
	Service   string `json:"service"`
	ServiceID string `json:"service_id"`
	Severity  string `json:"severity"`
	Issue     string `json:"issue"`
	Detail    string `json:"detail"`
}

type pdCoverageResult struct {
	ServicesChecked int             `json:"services_checked"`
	GapsFound       int             `json:"gaps_found"`
	Gaps            []pdCoverageGap `json:"gaps"`
}

var pdSeverityRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

// pp:data-source local
func newNovelAuditCoverageCmd(flags *rootFlags) *cobra.Command {
	var minSeverity, flagService string

	cmd := &cobra.Command{
		Use:         "coverage",
		Short:       "Flag services whose escalation chain is broken (no policy, empty tier, single point of failure)",
		Long:        "Joins the synced services, escalation policies, and schedules and flags coverage gaps: a service with no escalation policy, a policy with no rules, a tier with no targets, an empty schedule, or a chain that resolves to a single person (single point of failure). Run `sync` first; exits 0 with an empty result when nothing is synced.",
		Example:     "  pagerduty-cli audit coverage\n  pagerduty-cli audit coverage --agent\n  pagerduty-cli audit coverage --severity high",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			pdHintSync(cmd, flags, "services")
			services, err := pdLoadResource(cmd.Context(), "services")
			if err != nil {
				return fmt.Errorf("reading services from local store: %w", err)
			}
			eps, err := pdLoadResource(cmd.Context(), "escalation_policies")
			if err != nil {
				return fmt.Errorf("reading escalation_policies from local store: %w", err)
			}
			schedules, err := pdLoadResource(cmd.Context(), "schedules")
			if err != nil {
				return fmt.Errorf("reading schedules from local store: %w", err)
			}
			res := buildCoverageAudit(services, eps, schedules, flagService)
			if minSeverity != "" {
				minRank, ok := pdSeverityRank[minSeverity]
				if !ok {
					return fmt.Errorf("invalid --severity %q: must be one of critical, high, medium, low", minSeverity)
				}
				filtered := res.Gaps[:0]
				for _, g := range res.Gaps {
					if pdSeverityRank[g.Severity] <= minRank {
						filtered = append(filtered, g)
					}
				}
				res.Gaps = filtered
				res.GapsFound = len(filtered)
			}
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if res.ServicesChecked == 0 {
					fmt.Fprintln(w, "No services in the local store (run `pagerduty-cli sync` first).")
					return
				}
				if res.GapsFound == 0 {
					fmt.Fprintf(w, "Checked %d service(s): no coverage gaps found.\n", res.ServicesChecked)
					return
				}
				fmt.Fprintf(w, "Checked %d service(s): %d coverage gap(s)\n\n", res.ServicesChecked, res.GapsFound)
				tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "SEVERITY\tSERVICE\tISSUE\tDETAIL")
				for _, g := range res.Gaps {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", g.Severity, g.Service, g.Issue, g.Detail)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&minSeverity, "severity", "", "Only show gaps at or above this severity (critical|high|medium|low)")
	cmd.Flags().StringVar(&flagService, "service", "", "Limit the audit to a single service ID or name")
	return cmd
}

// buildCoverageAudit is the pure split-out for tests.
func buildCoverageAudit(services, eps, schedules []map[string]any, serviceFilter string) pdCoverageResult {
	epByID := map[string]map[string]any{}
	for _, ep := range eps {
		if id := pdString(ep, "id"); id != "" {
			epByID[id] = ep
		}
	}
	schedByID := map[string]map[string]any{}
	for _, s := range schedules {
		if id := pdString(s, "id"); id != "" {
			schedByID[id] = s
		}
	}

	res := pdCoverageResult{Gaps: []pdCoverageGap{}}
	for _, svc := range services {
		svcID := pdString(svc, "id")
		svcName := pdRefLabel(svc)
		if serviceFilter != "" && svcID != serviceFilter && svcName != serviceFilter {
			continue
		}
		res.ServicesChecked++
		add := func(sev, issue, detail string) {
			res.Gaps = append(res.Gaps, pdCoverageGap{Service: svcName, ServiceID: svcID, Severity: sev, Issue: issue, Detail: detail})
		}

		epRef := pdMap(svc, "escalation_policy")
		epID := pdString(epRef, "id")
		if epID == "" {
			add("critical", "no_escalation_policy", "service has no escalation policy — incidents would page nobody")
			continue
		}
		ep, ok := epByID[epID]
		if !ok {
			// EP not synced; cannot analyze its rules without false positives.
			continue
		}
		rules := pdSlice(ep, "escalation_rules")
		if len(rules) == 0 {
			add("critical", "empty_policy", fmt.Sprintf("escalation policy %q has no rules", pdRefLabel(epRef)))
			continue
		}

		distinctUsers := map[string]bool{}
		hadUnresolvedTarget := false
		for i, rule := range rules {
			targets := pdSlice(rule, "targets")
			if len(targets) == 0 {
				add("high", "empty_tier", fmt.Sprintf("escalation rule %d has no targets", i+1))
				continue
			}
			for _, t := range targets {
				ttype := pdString(t, "type")
				tid := pdString(t, "id")
				switch {
				case ttype == "user_reference" || ttype == "user":
					if tid != "" {
						distinctUsers[tid] = true
					}
				case ttype == "schedule_reference" || ttype == "schedule":
					sch, ok := schedByID[tid]
					if !ok {
						hadUnresolvedTarget = true
						continue
					}
					users := pdSlice(sch, "users")
					layers := pdSlice(sch, "schedule_layers")
					if len(users) == 0 && len(layers) == 0 {
						add("high", "empty_schedule", fmt.Sprintf("schedule %q (tier %d) has no users or layers", pdRefLabel(sch), i+1))
						continue
					}
					for _, u := range users {
						if uid := pdString(u, "id"); uid != "" {
							distinctUsers[uid] = true
						}
					}
				default:
					hadUnresolvedTarget = true
				}
			}
		}

		// Single point of failure: every tier resolves to exactly one human, and
		// no target was left unresolved (which could have widened the set).
		if !hadUnresolvedTarget && len(distinctUsers) == 1 {
			add("medium", "single_point_of_failure", "the entire escalation chain resolves to a single person")
		}
		// Single-tier policy with no escalation fallback.
		if len(rules) == 1 {
			add("low", "single_tier", "escalation policy has only one tier — no fallback if the first responder misses")
		}
	}

	res.Gaps = dedupeCoverageGaps(res.Gaps)
	res.GapsFound = len(res.Gaps)
	sort.SliceStable(res.Gaps, func(i, j int) bool {
		ri, rj := pdSeverityRank[res.Gaps[i].Severity], pdSeverityRank[res.Gaps[j].Severity]
		if ri != rj {
			return ri < rj
		}
		return res.Gaps[i].Service < res.Gaps[j].Service
	})
	return res
}

func dedupeCoverageGaps(gaps []pdCoverageGap) []pdCoverageGap {
	seen := map[string]bool{}
	out := gaps[:0]
	for _, g := range gaps {
		k := g.ServiceID + "|" + g.Issue + "|" + g.Detail
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, g)
	}
	return out
}
