// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// oncall who: resolves who is on call right now (and who is on next, and the
// handoff time) by reading the synced on-call entries — optionally scoped to a
// service, team, escalation policy, or user. The single /oncalls endpoint
// returns who is on now; joining the service→escalation-policy mapping and
// surfacing the next rotation is the cross-entity value the API does not.
package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type pdOncallEntry struct {
	EscalationPolicy string `json:"escalation_policy"`
	EPID             string `json:"escalation_policy_id"`
	EscalationLevel  int    `json:"escalation_level"`
	Schedule         string `json:"schedule,omitempty"`
	User             string `json:"user"`
	UserID           string `json:"user_id"`
	Since            string `json:"since,omitempty"`
	Until            string `json:"until,omitempty"`
	HandoffIn        string `json:"handoff_in,omitempty"`
}

type pdOncallWhoResult struct {
	AsOf      string          `json:"as_of"`
	ScopeType string          `json:"scope_type,omitempty"`
	ScopeID   string          `json:"scope_id,omitempty"`
	Current   []pdOncallEntry `json:"current"`
	Next      []pdOncallEntry `json:"next"`
}

// pp:data-source local
func newNovelOncallWhoCmd(flags *rootFlags) *cobra.Command {
	var flagService, flagTeam, flagEP, flagUser string

	cmd := &cobra.Command{
		Use:         "who",
		Short:       "Who is on call now and next (and the handoff time) for a service, team, or escalation policy",
		Long:        "Reads the synced on-call entries and shows who is on call right now per escalation level, who is on next, and when the current responder hands off. Scope with --service (resolved to its escalation policy), --team, --escalation-policy, or --user. With no scope flag it shows all current on-calls. Run `sync` first; exits 0 with an empty result when nothing is synced.",
		Example:     "  pagerduty-cli oncall who\n  pagerduty-cli oncall who --service PXXXXXX --agent\n  pagerduty-cli oncall who --escalation-policy PYYYYYY",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			pdHintSync(cmd, flags, "oncalls")
			oncalls, err := pdLoadResource(cmd.Context(), "oncalls")
			if err != nil {
				return fmt.Errorf("reading oncalls from local store: %w", err)
			}

			res := pdOncallWhoResult{AsOf: time.Now().UTC().Format(time.RFC3339), Current: []pdOncallEntry{}, Next: []pdOncallEntry{}}

			epFilter := flagEP
			switch {
			case flagService != "":
				res.ScopeType, res.ScopeID = "service", flagService
				services, serr := pdLoadResource(cmd.Context(), "services")
				if serr != nil {
					return fmt.Errorf("reading services from local store: %w", serr)
				}
				if ep := pdServiceEscalationPolicy(services, flagService); ep != "" {
					epFilter = ep
				} else {
					// Service not in store (or has no EP) → no resolvable on-call.
					return pdEmit(cmd, flags, res, func(w io.Writer) {
						fmt.Fprintf(w, "Service %q not found in the local store, or it has no escalation policy. Run `pagerduty-cli sync` first.\n", flagService)
					})
				}
			case flagEP != "":
				res.ScopeType, res.ScopeID = "escalation_policy", flagEP
			case flagTeam != "":
				res.ScopeType, res.ScopeID = "team", flagTeam
			case flagUser != "":
				res.ScopeType, res.ScopeID = "user", flagUser
			}

			buildOncallWho(oncalls, epFilter, flagUser, time.Now(), &res)

			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if len(res.Current) == 0 && len(res.Next) == 0 {
					fmt.Fprintln(w, "No matching on-call entries in the local store (run `pagerduty-cli sync` first).")
					return
				}
				tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "WHEN\tLVL\tUSER\tESCALATION POLICY\tSCHEDULE\tHANDOFF")
				for _, e := range res.Current {
					fmt.Fprintf(tw, "now\t%d\t%s\t%s\t%s\t%s\n", e.EscalationLevel, e.User, e.EscalationPolicy, dash(e.Schedule), dash(e.HandoffIn))
				}
				for _, e := range res.Next {
					fmt.Fprintf(tw, "next\t%d\t%s\t%s\t%s\t%s\n", e.EscalationLevel, e.User, e.EscalationPolicy, dash(e.Schedule), dash(e.Since))
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&flagService, "service", "", "Service ID or name; resolved to its escalation policy")
	cmd.Flags().StringVar(&flagTeam, "team", "", "Team ID or name to scope on-calls to")
	cmd.Flags().StringVar(&flagEP, "escalation-policy", "", "Escalation policy ID or name to scope on-calls to")
	cmd.Flags().StringVar(&flagUser, "user", "", "User ID or name to scope on-calls to")
	return cmd
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// pdServiceEscalationPolicy finds a service by id or name in the synced
// services and returns its escalation policy id.
func pdServiceEscalationPolicy(services []map[string]any, idOrName string) string {
	for _, s := range services {
		if pdString(s, "id") == idOrName || pdRefLabel(s) == idOrName || pdString(s, "name") == idOrName {
			return pdString(pdMap(s, "escalation_policy"), "id")
		}
	}
	return ""
}

// buildOncallWho is the pure split-out for tests. It populates res.Current and
// res.Next from on-call entries, applying the EP and user filters.
func buildOncallWho(oncalls []map[string]any, epFilter, userFilter string, now time.Time, res *pdOncallWhoResult) {
	type nextKey struct {
		ep    string
		level int
	}
	earliestNext := map[nextKey]pdOncallEntry{}

	for _, oc := range oncalls {
		epRef := pdMap(oc, "escalation_policy")
		epID := pdString(epRef, "id")
		if epFilter != "" && epID != epFilter && pdRefLabel(epRef) != epFilter {
			continue
		}
		userRef := pdMap(oc, "user")
		if userFilter != "" && pdString(userRef, "id") != userFilter && pdRefLabel(userRef) != userFilter {
			continue
		}

		level := 0
		if lv, err := strconv.Atoi(pdString(oc, "escalation_level")); err == nil {
			level = lv
		}
		entry := pdOncallEntry{
			EscalationPolicy: pdRefLabel(epRef),
			EPID:             epID,
			EscalationLevel:  level,
			Schedule:         pdRefLabel(pdMap(oc, "schedule")),
			User:             pdRefLabel(userRef),
			UserID:           pdString(userRef, "id"),
		}
		start, hasStart := pdParseTime(pdString(oc, "start"))
		end, hasEnd := pdParseTime(pdString(oc, "end"))
		if hasStart {
			entry.Since = start.UTC().Format(time.RFC3339)
		}
		if hasEnd {
			entry.Until = end.UTC().Format(time.RFC3339)
		}

		isCurrent := (!hasStart || !start.After(now)) && (!hasEnd || end.After(now))
		if isCurrent {
			if hasEnd {
				entry.HandoffIn = pdHumanDur(end.Sub(now))
			}
			res.Current = append(res.Current, entry)
			continue
		}
		if hasStart && start.After(now) {
			k := nextKey{ep: epID, level: level}
			if cur, ok := earliestNext[k]; !ok || start.Before(mustTime(cur.Since)) {
				earliestNext[k] = entry
			}
		}
	}

	for _, e := range earliestNext {
		res.Next = append(res.Next, e)
	}
	sort.SliceStable(res.Current, func(i, j int) bool {
		return res.Current[i].EscalationLevel < res.Current[j].EscalationLevel
	})
	sort.SliceStable(res.Next, func(i, j int) bool {
		if res.Next[i].EscalationLevel != res.Next[j].EscalationLevel {
			return res.Next[i].EscalationLevel < res.Next[j].EscalationLevel
		}
		return res.Next[i].Since < res.Next[j].Since
	})
}

func mustTime(s string) time.Time {
	t, _ := pdParseTime(s)
	return t
}
