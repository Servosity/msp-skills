// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command. Preserved across `generate --force`.
// pp:data-source auto

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// corkEventType carries the remediation window. cure_period_hours is nullable
// in the spec, so it is decoded as a pointer: absent means "no cure period
// defined", which is different from a zero-hour cure period.
type corkEventType struct {
	EventType       string `json:"event_type"`
	Description     string `json:"description"`
	CurePeriodHours *int   `json:"cure_period_hours"`
	Provisional     bool   `json:"provisional"`
}

type corkComplianceEvent struct {
	UUID       string `json:"uuid"`
	ClientUUID string `json:"client_uuid"`
	DeviceUUID string `json:"device_uuid"`
	EventType  string `json:"event_type"`
	AtRisk     bool   `json:"at_risk"`
	AtRiskAt   string `json:"at_risk_at"`
	ResolvedAt string `json:"resolved_at"`
	Silenced   bool   `json:"silenced"`
	CreatedAt  string `json:"created_at"`
}

type overdueRow struct {
	EventUUID       string  `json:"event_uuid"`
	ClientUUID      string  `json:"client_uuid"`
	Client          string  `json:"client"`
	EventType       string  `json:"event_type"`
	AtRiskAt        string  `json:"at_risk_at"`
	CurePeriodHours int     `json:"cure_period_hours"`
	OverdueHours    float64 `json:"overdue_hours"`
	Bucket          string  `json:"bucket"`
	Silenced        bool    `json:"silenced"`
}

type overdueView struct {
	Items           []overdueRow   `json:"items"`
	Buckets         map[string]int `json:"buckets,omitempty"`
	TotalOverdue    int            `json:"total_overdue"`
	ClientsScanned  int            `json:"clients_scanned"`
	ClientsFailed   int            `json:"clients_failed"`
	EventsScanned   int            `json:"events_scanned"`
	EventTypesKnown int            `json:"event_types_known"`
	ScanCapHit      bool           `json:"scan_cap_hit"`
	Note            string         `json:"note,omitempty"`
}

// overdueBucket groups an overdue event by how far past its cure period it is.
func overdueBucket(h float64) string {
	switch {
	case h < 24:
		return "<1d"
	case h < 72:
		return "1-3d"
	case h < 168:
		return "3-7d"
	case h < 720:
		return "7-30d"
	default:
		return ">30d"
	}
}

func newNovelComplianceOverdueCmd(flags *rootFlags) *cobra.Command {
	var flagBucket bool
	var flagClientUUID string
	var flagIncludeSilenced bool
	var flagLimit int
	var flagMaxClients int
	var flagMaxScanPages int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "overdue",
		Short: "Surface compliance events that have blown their event type's remediation window, bucketed by age.",
		Long: "Surface compliance events that have blown their event type's remediation\n" +
			"window, bucketed by age.\n\n" +
			"The cure period lives on the event-type catalog while the at-risk timestamp\n" +
			"lives on the event itself, in two endpoints nothing joins. This command joins\n" +
			"them and reports only events that are actually past due.\n\n" +
			"Use this command to find compliance events that are now costing score. Do NOT\n" +
			"use this command to list or filter a client's raw event stream; use\n" +
			"'compliance get-events' instead.",
		Example: "  cork-cli compliance overdue --bucket --agent",
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "compliance overdue")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// 1. Cure periods, keyed by event type.
			rawTypes, _, _, err := corkFetchPages(ctx, c, "/compliance/event-types", nil, 1)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			cure := map[string]int{}
			badTypes := 0
			for _, r := range rawTypes {
				var et corkEventType
				if json.Unmarshal(r, &et) != nil {
					badTypes++
					continue
				}
				if et.EventType != "" && et.CurePeriodHours != nil {
					cure[et.EventType] = *et.CurePeriodHours
				}
			}
			// Without cure periods every event is unclassifiable, so an
			// all-undecodable catalog must not read as "nothing is overdue".
			if len(cure) == 0 && badTypes > 0 {
				return corkDecodeFailure("compliance event type(s)", badTypes)
			}

			// 2. Which clients to walk. The local mirror avoids a live /clients
			// call; a missing mirror falls back to fetching the roster.
			type target struct{ uuid, name string }
			targets := make([]target, 0)
			if flagClientUUID != "" {
				targets = append(targets, target{uuid: flagClientUUID})
			} else {
				if db, ok, openErr := corkOpenStore(ctx, flagDB, cmd.ErrOrStderr(), cmd.OutOrStdout(), "clients"); openErr == nil && ok {
					defer db.Close()
					if !hintIfUnsynced(cmd, db, "clients") {
						hintIfStale(cmd, db, "clients", flags.maxAge)
					}
					clients, loadErr := corkLoadClients(ctx, db)
					if loadErr != nil {
						return loadErr
					}
					for _, cl := range clients {
						if cl.Hidden {
							continue
						}
						targets = append(targets, target{uuid: cl.UUID, name: cl.Name})
					}
				}
				if len(targets) == 0 {
					rawClients, _, _, cErr := corkFetchPages(ctx, c, "/clients", nil, flagMaxScanPages)
					if cErr != nil {
						return classifyAPIError(cErr, flags)
					}
					roster, undecodable := corkDecodeClients(rawClients)
					if len(roster) == 0 && undecodable > 0 {
						return corkDecodeFailure("client record(s)", undecodable)
					}
					for _, cl := range roster {
						if cl.Hidden {
							continue
						}
						targets = append(targets, target{uuid: cl.UUID, name: cl.Name})
					}
				}
			}

			capHit := false
			if flagMaxClients > 0 && len(targets) > flagMaxClients {
				targets = targets[:flagMaxClients]
				capHit = true
			}

			// 3. Walk each client's open events and keep the past-due ones.
			now := time.Now()
			rows := make([]overdueRow, 0)
			eventsScanned := 0
			clientsFailed := 0
			badEvents := 0
			for _, t := range targets {
				params := map[string]string{
					"show_resolved": "false",
				}
				// `at_risk=true` means "unresolved AND unsuppressed" upstream, so
				// it would filter out the silenced events --include-silenced is
				// asking for. Drop it in that mode and filter locally instead.
				if flagIncludeSilenced {
					params["show_silenced"] = "true"
				} else {
					params["at_risk"] = "true"
				}
				rawEvents, _, evCap, evErr := corkFetchPages(ctx, c, "/compliance/client/"+corkPathSeg(t.uuid)+"/events", params, flagMaxScanPages)
				if evErr != nil {
					// One client's events failing must not sink the sweep; report
					// it and keep going so the result is still actionable. The
					// count travels in the envelope so a fully-failed sweep can
					// never read as "nothing is overdue".
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: events for client %s failed: %v\n", corkResolve(map[string]string{t.uuid: t.name}, t.uuid), evErr)
					clientsFailed++
					continue
				}
				if evCap {
					capHit = true
				}
				for _, r := range rawEvents {
					var ev corkComplianceEvent
					if json.Unmarshal(r, &ev) != nil {
						badEvents++
						continue
					}
					eventsScanned++
					if ev.ResolvedAt != "" {
						continue
					}
					if ev.Silenced && !flagIncludeSilenced {
						continue
					}
					hours, known := cure[ev.EventType]
					if !known {
						// No cure period defined means no SLA to breach.
						continue
					}
					atRisk, ok := corkParseTime(ev.AtRiskAt)
					if !ok {
						continue
					}
					due := atRisk.Add(time.Duration(hours) * time.Hour)
					if !now.After(due) {
						continue
					}
					over := now.Sub(due).Hours()
					rows = append(rows, overdueRow{
						EventUUID:       ev.UUID,
						ClientUUID:      ev.ClientUUID,
						Client:          t.name,
						EventType:       ev.EventType,
						AtRiskAt:        ev.AtRiskAt,
						CurePeriodHours: hours,
						OverdueHours:    float64(int(over*10)) / 10,
						Bucket:          overdueBucket(over),
						Silenced:        ev.Silenced,
					})
				}
			}

			// Every client's fetch failing is a read failure, not a clean sweep.
			if clientsFailed > 0 && clientsFailed == len(targets) {
				return apiErr(fmt.Errorf("event lookups failed for all %d client(s); refusing to report that nothing is overdue", clientsFailed))
			}
			if badEvents > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d compliance event(s) could not be decoded and were not evaluated\n", badEvents)
				capHit = true
			}

			corkSortStable(rows, func(a, b overdueRow) bool { return a.OverdueHours > b.OverdueHours })
			total := len(rows)
			// Bucket over the full result set, before --limit truncation: the
			// histogram is meant to describe the sweep, not the printed page.
			var buckets map[string]int
			if flagBucket {
				buckets = map[string]int{}
				for _, r := range rows {
					buckets[r.Bucket]++
				}
			}
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}

			view := overdueView{
				Items:           rows,
				Buckets:         buckets,
				TotalOverdue:    total,
				ClientsScanned:  len(targets),
				ClientsFailed:   clientsFailed,
				EventsScanned:   eventsScanned,
				EventTypesKnown: len(cure),
				ScanCapHit:      capHit || clientsFailed > 0,
			}
			switch {
			case len(cure) == 0:
				view.Note = "no event types define a cure period, so no event can be classified overdue"
			case total == 0:
				view.Note = fmt.Sprintf("scanned %d open event(s) across %d client(s); none past their cure period", eventsScanned, len(targets)-clientsFailed)
				if clientsFailed > 0 {
					view.Note += fmt.Sprintf("; %d client(s) could not be read, so this is not a complete sweep", clientsFailed)
				} else if capHit {
					view.Note += "; a scan cap was reached, so this is a partial sweep"
				}
			default:
				if total > len(rows) {
					view.Note = fmt.Sprintf("showing %d of %d overdue event(s); raise --limit to see more", len(rows), total)
				}
				if clientsFailed > 0 {
					view.Note += fmt.Sprintf(" (%d client(s) could not be read)", clientsFailed)
				} else if capHit {
					view.Note += " (a scan cap was reached; results are partial)"
				}
				view.Note = strings.TrimSpace(view.Note)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "CLIENT\tEVENT TYPE\tCURE(h)\tOVERDUE(h)\tBUCKET")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%.1f\t%s\n", truncate(r.Client, 30), truncate(r.EventType, 32), r.CurePeriodHours, r.OverdueHours, r.Bucket)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if flagBucket && len(view.Buckets) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nby age:")
				for _, k := range []string{"<1d", "1-3d", "3-7d", "7-30d", ">30d"} {
					if n, ok := view.Buckets[k]; ok {
						fmt.Fprintf(cmd.OutOrStdout(), "  %-6s %d\n", k, n)
					}
				}
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "\n"+view.Note)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagBucket, "bucket", false, "Include a count of overdue events per age bucket")
	cmd.Flags().StringVar(&flagClientUUID, "client-uuid", "", "Restrict the sweep to one client")
	cmd.Flags().BoolVar(&flagIncludeSilenced, "include-silenced", false, "Include events that have been silenced in Cork")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum overdue events to return (0 for all)")
	cmd.Flags().IntVar(&flagMaxClients, "max-clients", 50, "Maximum clients to walk before returning partial results")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", corkDefaultScanPages, "Maximum event pages to read per client")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path used to resolve the client roster and names")
	return cmd
}
