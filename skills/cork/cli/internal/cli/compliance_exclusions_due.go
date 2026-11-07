// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command. Preserved across `generate --force`.
// pp:data-source auto

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"cork-pp-cli/internal/types"

	"github.com/spf13/cobra"
)

type exclusionDueRow struct {
	ClientUUID           string `json:"client_uuid"`
	Client               string `json:"client"`
	EventDescription     string `json:"event_description"`
	Reason               string `json:"reason"`
	Notes                string `json:"notes,omitempty"`
	ConfiguredBy         string `json:"configured_by"`
	ReviewOn             string `json:"review_on"`
	DaysPastReview       *int   `json:"days_past_review"`
	NeedsReview          bool   `json:"needs_review"`
	ExcludedFromCoverage bool   `json:"excluded_from_coverage"`
	DeviceUUID           string `json:"device_uuid,omitempty"`
	InboxUUID            string `json:"inbox_uuid,omitempty"`
	DomainUUID           string `json:"domain_uuid,omitempty"`
}

type exclusionsDueView struct {
	Items           []exclusionDueRow `json:"items"`
	TotalDue        int               `json:"total_due"`
	ClientsScanned  int               `json:"clients_scanned"`
	ClientsFailed   int               `json:"clients_failed"`
	FirstError      string            `json:"first_error,omitempty"`
	SettingsScanned int               `json:"settings_scanned"`
	ScanCapHit      bool              `json:"scan_cap_hit"`
	Note            string            `json:"note,omitempty"`
}

// corkParseReviewDate reads review_on, which Cork may send as a date or a timestamp.
func corkParseReviewDate(s string) (time.Time, bool) {
	if t, ok := corkParseTime(s); ok {
		return t, true
	}
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

// exclusionDue decides one notification setting: due when Cork flags it for
// review or its review date is today or earlier. Days past review is nil when
// there is no parseable review date (a flagged row with no date).
func exclusionDue(s types.ComplianceEventNotificationSetting, now time.Time) (bool, *int) {
	var days *int
	due := s.NeedsReview
	if t, ok := corkParseReviewDate(s.ReviewOn); ok {
		d := int(now.Sub(t).Hours() / 24)
		days = &d
		due = due || !t.After(now)
	}
	return due, days
}

func newNovelComplianceExclusionsDueCmd(flags *rootFlags) *cobra.Command {
	var flagClientUUID string
	var flagLimit int
	var flagMaxClients int
	var flagMaxScanPages int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "exclusions-due",
		Short: "List compliance exclusions and silenced alert rules whose review date has passed or that are flagged for review",
		Long: "List compliance exclusions and silenced alert rules whose review date has\n" +
			"passed or that Cork flags for review, across every client, oldest first.\n\n" +
			"Exclusions live per client on the notification-settings endpoint, each with a\n" +
			"review_on date and a needs_review flag, and no endpoint lists the ones that\n" +
			"are due across clients. An exclusion nobody re-reviewed is a risk the score\n" +
			"silently stopped counting.\n\n" +
			"This is a live fan-out: a capped or failed scan sets scan_cap_hit, so an\n" +
			"empty result is a clean bill of health only when scan_cap_hit is false.",
		Example: "  cork-cli compliance exclusions-due --agent",
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "compliance exclusions-due")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Which clients to walk: the local mirror when synced, else the live roster.
			type target struct{ uuid, name string }
			targets := make([]target, 0)
			rosterPartial := false
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
						if !cl.Hidden {
							targets = append(targets, target{uuid: cl.UUID, name: cl.Name})
						}
					}
				}
				if len(targets) == 0 {
					rawClients, _, rosterCap, cErr := corkFetchPages(ctx, c, "/clients", nil, flagMaxScanPages)
					if cErr != nil {
						return classifyAPIError(cmd.OutOrStdout(), cErr, flags)
					}
					roster, undecodable := corkDecodeClients(rawClients)
					if len(roster) == 0 && undecodable > 0 {
						return corkDecodeFailure("client record(s)", undecodable)
					}
					for _, cl := range roster {
						if !cl.Hidden {
							targets = append(targets, target{uuid: cl.UUID, name: cl.Name})
						}
					}
					if rosterCap || undecodable > 0 {
						fmt.Fprintln(cmd.ErrOrStderr(), "warning: the client roster was read partially")
						rosterPartial = true
					}
				}
			}

			capHit := rosterPartial
			if flagMaxClients > 0 && len(targets) > flagMaxClients {
				targets, capHit = targets[:flagMaxClients], true
			}

			now := time.Now()
			rows := make([]exclusionDueRow, 0)
			scanned, failed, bad := 0, 0, 0
			firstErr := ""
			for _, t := range targets {
				raw, _, pageCap, fErr := corkFetchPages(ctx, c, "/compliance/client/"+corkPathSeg(t.uuid)+"/notification-settings", nil, flagMaxScanPages)
				if fErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: notification settings for client %s failed: %v\n", corkResolve(map[string]string{t.uuid: t.name}, t.uuid), fErr)
					if firstErr == "" {
						firstErr = fErr.Error()
					}
					failed++
					continue
				}
				capHit = capHit || pageCap
				for _, r := range raw {
					var s types.ComplianceEventNotificationSetting
					if json.Unmarshal(r, &s) != nil {
						bad++
						continue
					}
					scanned++
					due, days := exclusionDue(s, now)
					if !due {
						continue
					}
					rows = append(rows, exclusionDueRow{
						ClientUUID: t.uuid, Client: t.name, EventDescription: s.EventDescription, Reason: s.Reason,
						Notes: s.Notes, ConfiguredBy: s.ConfiguredBy, ReviewOn: s.ReviewOn, DaysPastReview: days,
						NeedsReview: s.NeedsReview, ExcludedFromCoverage: s.ExcludedFromCoverage,
						DeviceUUID: s.DeviceUuid, InboxUUID: s.InboxUuid, DomainUUID: s.DomainUuid,
					})
				}
			}
			if failed > 0 && failed == len(targets) {
				return apiErr(fmt.Errorf("notification-settings lookups failed for all %d client(s); refusing to report that no exclusion is due: %s", failed, firstErr))
			}
			if bad > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d notification setting(s) could not be decoded and were not evaluated\n", bad)
				capHit = true
			}

			// Oldest review first; a flagged row with no review date sorts last.
			corkSortStable(rows, func(a, b exclusionDueRow) bool {
				if (a.DaysPastReview == nil) != (b.DaysPastReview == nil) {
					return b.DaysPastReview == nil
				}
				return a.DaysPastReview != nil && *a.DaysPastReview > *b.DaysPastReview
			})
			total := len(rows)
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}
			view := exclusionsDueView{
				Items: rows, TotalDue: total, ClientsScanned: len(targets), ClientsFailed: failed,
				FirstError: firstErr, SettingsScanned: scanned, ScanCapHit: capHit || failed > 0,
			}
			switch {
			case total == 0 && !view.ScanCapHit:
				view.Note = fmt.Sprintf("scanned %d setting(s) across %d client(s); none due for review", scanned, len(targets))
			case total == 0:
				view.Note = fmt.Sprintf("scanned %d setting(s); none due, but the sweep was partial (scan_cap_hit), so this is not a clean bill of health", scanned)
			case total > len(rows):
				view.Note = fmt.Sprintf("showing %d of %d due exclusion(s); raise --limit to see more", len(rows), total)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "CLIENT\tEVENT\tREVIEW ON\tDAYS PAST\tFLAGGED\tREASON")
			for _, r := range rows {
				days := "-"
				if r.DaysPastReview != nil {
					days = fmt.Sprint(*r.DaysPastReview)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\n", truncate(r.Client, 28), truncate(r.EventDescription, 32), r.ReviewOn, days, r.NeedsReview, truncate(r.Reason, 30))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "\n"+view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagClientUUID, "client-uuid", "", "Restrict the sweep to one client")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum due exclusions to return (0 for all)")
	cmd.Flags().IntVar(&flagMaxClients, "max-clients", 50, "Maximum clients to walk before returning partial results")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", corkDefaultScanPages, "Maximum settings pages to read per client")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path used to resolve the client roster and names")
	return cmd
}
