// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// pulse: a live triage view over the local incident mirror. Buckets every open
// incident by service with urgency/status counts and how long the oldest
// triggered incident has gone unacknowledged — the cross-entity "what's hot
// right now" the live API has no single endpoint for.
package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type pdPulseService struct {
	Service            string `json:"service"`
	ServiceID          string `json:"service_id"`
	Open               int    `json:"open"`
	Triggered          int    `json:"triggered"`
	Acknowledged       int    `json:"acknowledged"`
	HighUrgency        int    `json:"high_urgency"`
	OldestUnackedMin   int64  `json:"oldest_unacked_minutes"`
	OldestUnackedHuman string `json:"oldest_unacked"`
}

type pdPulseResult struct {
	GeneratedAt  string           `json:"generated_at"`
	TotalOpen    int              `json:"total_open"`
	Triggered    int              `json:"triggered"`
	Acknowledged int              `json:"acknowledged"`
	Services     []pdPulseService `json:"services"`
}

// pp:data-source local
func newNovelPulseCmd(flags *rootFlags) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:         "pulse",
		Short:       "Show what's hot right now: open incidents bucketed by service with unacked age",
		Long:        "Reads the local incident mirror and buckets every open (triggered or acknowledged) incident by service, with per-service triggered/acknowledged/high-urgency counts and how long the oldest still-unacknowledged incident has been waiting. Run `sync` first. Exits 0 with an empty result when nothing has been synced.",
		Example:     "  pagerduty-cli pulse\n  pagerduty-cli pulse --agent\n  pagerduty-cli pulse --limit 10",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			pdHintSync(cmd, flags, "incidents")
			incidents, err := pdLoadResource(cmd.Context(), "incidents")
			if err != nil {
				return fmt.Errorf("reading incidents from local store: %w", err)
			}
			res := buildPulse(incidents, time.Now())
			if limit > 0 && len(res.Services) > limit {
				res.Services = res.Services[:limit]
			}
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if res.TotalOpen == 0 {
					fmt.Fprintln(w, "No open incidents in the local store (run `pagerduty-cli sync` first).")
					return
				}
				fmt.Fprintf(w, "%d open incident(s): %d triggered, %d acknowledged\n\n", res.TotalOpen, res.Triggered, res.Acknowledged)
				tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "SERVICE\tOPEN\tTRIG\tACK\tHIGH\tOLDEST UNACKED")
				for _, s := range res.Services {
					oldest := s.OldestUnackedHuman
					if oldest == "" {
						oldest = "-"
					}
					fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%s\n", s.Service, s.Open, s.Triggered, s.Acknowledged, s.HighUrgency, oldest)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit the number of services shown (0 = all)")
	return cmd
}

// buildPulse is the pure aggregation, split out for table-driven tests.
func buildPulse(incidents []map[string]any, now time.Time) pdPulseResult {
	type acc struct {
		svc         pdPulseService
		oldestUnack time.Duration
		hasUnack    bool
	}
	byService := map[string]*acc{}
	order := []string{}
	res := pdPulseResult{GeneratedAt: now.UTC().Format(time.RFC3339), Services: []pdPulseService{}}

	for _, inc := range incidents {
		status := pdString(inc, "status")
		if status != "triggered" && status != "acknowledged" {
			continue // only open incidents
		}
		res.TotalOpen++
		if status == "triggered" {
			res.Triggered++
		} else {
			res.Acknowledged++
		}
		svcRef := pdMap(inc, "service")
		svcID := pdString(svcRef, "id")
		svcName := pdRefLabel(svcRef)
		if svcName == "" {
			svcName = "(unknown service)"
		}
		key := svcID
		if key == "" {
			key = svcName
		}
		a := byService[key]
		if a == nil {
			a = &acc{svc: pdPulseService{Service: svcName, ServiceID: svcID}}
			byService[key] = a
			order = append(order, key)
		}
		a.svc.Open++
		if status == "triggered" {
			a.svc.Triggered++
		} else {
			a.svc.Acknowledged++
		}
		if pdString(inc, "urgency") == "high" {
			a.svc.HighUrgency++
		}
		if status == "triggered" {
			if created, ok := pdParseTime(pdString(inc, "created_at")); ok {
				age := now.Sub(created)
				if age > a.oldestUnack {
					a.oldestUnack = age
					a.hasUnack = true
				}
			}
		}
	}

	for _, key := range order {
		a := byService[key]
		if a.hasUnack {
			a.svc.OldestUnackedMin = int64(a.oldestUnack.Minutes())
			a.svc.OldestUnackedHuman = pdHumanDur(a.oldestUnack)
		}
		res.Services = append(res.Services, a.svc)
	}
	sort.SliceStable(res.Services, func(i, j int) bool {
		if res.Services[i].OldestUnackedMin != res.Services[j].OldestUnackedMin {
			return res.Services[i].OldestUnackedMin > res.Services[j].OldestUnackedMin
		}
		return res.Services[i].Open > res.Services[j].Open
	})
	return res
}
