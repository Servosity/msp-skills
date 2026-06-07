// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): resolve the full ordered
// escalation ladder for a service or incident — every rung, the policy it comes
// from, who currently sits at each level, and the delay before the next page.
// The API returns these as separate normalized objects you must walk by hand.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelEscalationTraceCmd(flags *rootFlags) *cobra.Command {
	var serviceName string
	var incidentRef string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "escalation-trace",
		Short: "Trace the ordered escalation ladder for a service or incident.",
		Long: `Use this command to trace the ordered escalation ladder for a service or
incident — every rung and who sits at it. Walks services → escalation policies
→ escalation levels/paths → current on-call from the local mirror into one
resolved ladder, with each rung's delay and targets.

Do NOT use this command for the at-a-glance "who is on call right now across
the whole portfolio" view; use 'oncall-now' instead. Do NOT use it for the
full single-incident situational screen; use 'war-room' instead.`,
		Example: `  rootly-cli escalation-trace --service checkout-api
  rootly-cli escalation-trace --incident INC-1234 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if serviceName == "" && incidentRef == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("one of --service or --incident is required"))
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "")
			if err != nil {
				return err
			}
			defer db.Close()

			// Resolve the target service, possibly via an incident.
			resolvedService := serviceName
			if resolvedService == "" {
				incidents, err := novelLoad(db, novelResolveType(db, "incidents"))
				if err != nil {
					return err
				}
				ref := strings.ToLower(strings.TrimSpace(incidentRef))
				for _, r := range incidents {
					if strings.ToLower(r.ID) == ref ||
						strings.ToLower(recStr(r.Attrs, "slug")) == ref ||
						strings.ToLower(recStr(r.Attrs, "sequential_id")) == ref {
						if names := incidentServiceNames(r); len(names) > 0 {
							resolvedService = names[0]
						}
						break
					}
				}
				if resolvedService == "" {
					return fmt.Errorf("incident %q not found in the local store or has no service; run 'rootly-cli sync --resources incidents,services' first", incidentRef)
				}
			}

			// Service -> escalation policy.
			services, err := novelLoad(db, novelResolveType(db, "services"))
			if err != nil {
				return err
			}
			var policyID string
			svcLower := strings.ToLower(strings.TrimSpace(resolvedService))
			serviceFound := false
			for _, s := range services {
				if strings.ToLower(recStr(s.Attrs, "name")) == svcLower {
					serviceFound = true
					policyID = firstNonEmpty(recStr(s.Attrs, "escalation_policy_id"), relID(s, "escalation_policy"))
					break
				}
			}

			// Policy record (for its name) — tolerate missing linkage.
			policies, _ := novelLoad(db, novelResolveType(db, "escalation-policies", "escalation_policies"))
			policyName := ""
			if policyID == "" && len(policies) == 1 {
				// Single-policy orgs: the only policy is the ladder.
				policyID = policies[0].ID
			}
			for _, p := range policies {
				if p.ID == policyID {
					policyName = recStr(p.Attrs, "name")
					break
				}
			}

			// Rungs: escalation levels/paths belonging to the policy, ordered by position.
			type rung struct {
				Position   int      `json:"position"`
				Delay      string   `json:"delay,omitempty"`
				Targets    []string `json:"targets"`
				OnCallNow  []string `json:"on_call_now,omitempty"`
				SourceType string   `json:"source_type"`
			}
			var rungs []rung
			for _, candidate := range []string{"escalation-levels", "escalation_levels", "escalation-paths", "escalation_paths", "escalation-policy-levels", "escalation-policy-paths"} {
				levels, err := novelLoad(db, candidate)
				if err != nil || len(levels) == 0 {
					continue
				}
				for _, l := range levels {
					owner := firstNonEmpty(relID(l, "escalation_policy"), recStr(l.Attrs, "escalation_policy_id"))
					if policyID != "" && owner != "" && owner != policyID {
						continue
					}
					pos := 0
					if v, ok := l.Attrs["position"].(float64); ok {
						pos = int(v)
					}
					var targets []string
					for _, k := range []string{"notification_target_params", "targets", "responders", "users", "schedules", "groups"} {
						targets = append(targets, recNames(l.Attrs[k])...)
					}
					delay := recStr(l.Attrs, "delay", "escalation_timeout", "timeout", "delay_in_minutes")
					if targets == nil {
						targets = []string{}
					}
					rungs = append(rungs, rung{Position: pos, Delay: delay, Targets: targets, SourceType: candidate})
				}
				if len(rungs) > 0 {
					break
				}
			}
			sort.Slice(rungs, func(i, j int) bool { return rungs[i].Position < rungs[j].Position })

			// Who is on call right now (annotate schedule-level rungs and give
			// an answer even when no ladder objects are synced).
			oncall := oncallForServices(db, []string{resolvedService})
			for i := range rungs {
				rungs[i].OnCallNow = oncall
			}

			note := ""
			if !serviceFound {
				note = fmt.Sprintf("service %q not found in the local store; run 'rootly-cli sync --resources services,escalation-policies' first", resolvedService)
			} else if len(rungs) == 0 {
				note = "no escalation levels/paths synced for this policy; showing current on-call only. Run 'rootly-cli sync' to pull escalation objects."
			}

			out := struct {
				Service    string   `json:"service"`
				PolicyID   string   `json:"escalation_policy_id,omitempty"`
				PolicyName string   `json:"escalation_policy,omitempty"`
				Rungs      []rung   `json:"rungs"`
				OnCallNow  []string `json:"current_oncall"`
				Note       string   `json:"note,omitempty"`
			}{
				Service: resolvedService, PolicyID: policyID, PolicyName: policyName,
				Rungs: rungs, OnCallNow: nonNilStrings(oncall), Note: note,
			}
			if out.Rungs == nil {
				out.Rungs = []rung{}
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "escalation ladder for %s", resolvedService)
				if policyName != "" {
					fmt.Fprintf(w, " (policy: %s)", policyName)
				}
				fmt.Fprintln(w)
				for _, r := range rungs {
					delay := r.Delay
					if delay == "" {
						delay = "-"
					}
					fmt.Fprintf(w, "  %2d. delay=%s  %s\n", r.Position, delay, strings.Join(r.Targets, ", "))
				}
				if len(oncall) > 0 {
					fmt.Fprintf(w, "  on-call now: %s\n", strings.Join(oncall, ", "))
				}
				if note != "" {
					fmt.Fprintf(w, "  note: %s\n", note)
				}
			})
		},
	}
	cmd.Flags().StringVar(&serviceName, "service", "", "Service name to trace the ladder for")
	cmd.Flags().StringVar(&incidentRef, "incident", "", "Incident id/slug — traces the ladder of its first service")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
