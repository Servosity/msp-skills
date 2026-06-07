// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): per-service reliability
// scorecard joining incidents + services + on-call + action items + SLAs from the
// local mirror. No single Rootly view assembles this.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelServiceHealthCmd(flags *rootFlags) *cobra.Command {
	var since string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "service-health [service]",
		Short: "Per-service reliability scorecard: incidents, MTTR, on-call, SLA.",
		Long: `Build a reliability scorecard per service from the local mirror: incident
count, MTTR, last incident, open action items, current on-call, and whether the
service has an SLA. Pass a service name to scope to one; omit it for the whole
portfolio. Wide offline join — no single Rootly screen assembles this.`,
		Example: `  rootly-cli service-health
  rootly-cli service-health checkout-api --json
  rootly-cli service-health --since 90d`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			var only string
			if len(args) == 1 {
				only = strings.ToLower(strings.TrimSpace(args[0]))
			}

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "incidents")
			if err != nil {
				return err
			}
			defer db.Close()

			services, err := novelLoad(db, novelResolveType(db, "services"))
			if err != nil {
				return err
			}
			incidents, _ := novelLoad(db, novelResolveType(db, "incidents"))
			slas, _ := novelLoad(db, novelResolveType(db, "slas"))
			hasAnySLA := len(slas) > 0

			// Precompute current on-call grouped by escalation policy, and map
			// each service to its escalation policy, so per-card on-call lookup
			// is a map hit rather than a re-scan.
			rosterByPolicy := map[string][]string{}
			for _, e := range currentOncallEntries(db) {
				label := e.User
				if e.Schedule != "" {
					label = fmt.Sprintf("%s (%s)", e.User, e.Schedule)
				}
				if label == "" || e.EscalationPolicyID == "" {
					continue
				}
				rosterByPolicy[e.EscalationPolicyID] = append(rosterByPolicy[e.EscalationPolicyID], label)
			}
			policyByService := map[string]string{}
			for _, svc := range services {
				if pid := recStr(svc.Attrs, "escalation_policy_id"); pid != "" {
					policyByService[strings.ToLower(recStr(svc.Attrs, "name"))] = pid
				}
			}

			var cutoff time.Time
			if d, ok := parseWindowDuration(since); ok {
				cutoff = time.Now().Add(-d)
			}

			// Index incidents by service name (lowercased).
			type sstat struct {
				count   int
				openN   int
				ttrSum  time.Duration
				ttrN    int
				lastAt  time.Time
				lastID  string
				openAIs int
			}
			stats := map[string]*sstat{}
			get := func(name string) *sstat {
				k := strings.ToLower(name)
				s := stats[k]
				if s == nil {
					s = &sstat{}
					stats[k] = s
				}
				return s
			}
			for _, inc := range incidents {
				start, hasStart := incidentStart(inc)
				if hasStart && !cutoff.IsZero() && start.Before(cutoff) {
					continue
				}
				openCount := len(collectOpenActionItems(db, inc.ID))
				for _, svc := range incidentServiceNames(inc) {
					s := get(svc)
					s.count++
					s.openAIs += openCount
					if incidentOpen(inc) {
						s.openN++
					}
					if resolved, ok := incidentResolved(inc); ok && hasStart && resolved.After(start) {
						s.ttrSum += resolved.Sub(start)
						s.ttrN++
					}
					if hasStart && start.After(s.lastAt) {
						s.lastAt = start
						s.lastID = inc.ID
					}
				}
			}

			type card struct {
				Service       string `json:"service"`
				Incidents     int    `json:"incidents"`
				OpenIncidents int    `json:"open_incidents"`
				MTTRHuman     string `json:"mttr_human,omitempty"`
				MTTRMinutes   int    `json:"mttr_minutes"`
				LastIncident  string `json:"last_incident,omitempty"`
				OpenActions   int    `json:"open_action_items"`
				OnCall        string `json:"current_oncall,omitempty"`
				HasSLA        bool   `json:"has_sla"`
			}
			var cards []card
			for _, svc := range services {
				name := recStr(svc.Attrs, "name")
				if name == "" {
					continue
				}
				if only != "" && strings.ToLower(name) != only {
					continue
				}
				s := stats[strings.ToLower(name)]
				if s == nil {
					s = &sstat{}
				}
				c := card{
					Service:       name,
					Incidents:     s.count,
					OpenIncidents: s.openN,
					MTTRMinutes:   0,
					OpenActions:   s.openAIs,
					HasSLA:        hasAnySLA,
				}
				if s.ttrN > 0 {
					avg := s.ttrSum / time.Duration(s.ttrN)
					c.MTTRMinutes = roundMinutes(avg)
					c.MTTRHuman = humanDuration(avg)
				}
				if !s.lastAt.IsZero() {
					c.LastIncident = s.lastAt.Format("2006-01-02")
				}
				if pid := policyByService[strings.ToLower(name)]; pid != "" {
					c.OnCall = strings.Join(rosterByPolicy[pid], ", ")
				}
				cards = append(cards, c)
			}
			sort.Slice(cards, func(i, j int) bool {
				if cards[i].Incidents == cards[j].Incidents {
					return cards[i].Service < cards[j].Service
				}
				return cards[i].Incidents > cards[j].Incidents
			})

			if only != "" && len(cards) == 0 {
				return notFoundErr(fmt.Errorf("service %q not found in the local mirror (run 'rootly-cli sync')", args[0]))
			}

			out := struct {
				Since    string `json:"since,omitempty"`
				Services []card `json:"services"`
			}{Since: since, Services: cards}
			if out.Services == nil {
				out.Services = []card{}
			}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				if len(cards) == 0 {
					fmt.Fprintln(w, "No services found (run 'rootly-cli sync').")
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "SERVICE\tINC\tOPEN\tMTTR\tLAST\tACTIONS\tON-CALL")
				for _, c := range cards {
					fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\t%d\t%s\n",
						truncate(c.Service, 30), c.Incidents, c.OpenIncidents,
						dash(c.MTTRHuman), dash(c.LastIncident), c.OpenActions, dash(c.OnCall))
				}
				flushHuman(cmd, tw)
			})
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only count incidents started within this window, e.g. 90d")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
