// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: every threat event that touched one mailbox.
// pp:data-source live
// pp:client-call

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type threatsMailboxView struct {
	threatsReport
	// TotalEvents counts every match; Events holds the newest --limit of them.
	TotalEvents int            `json:"total_events"`
	ByKind      map[string]int `json:"by_kind"`
	Events      []threatEvent  `json:"events"`
}

func newNovelThreatsMailboxCmd(flags *rootFlags) *cobra.Command {
	var o threatsFlags

	cmd := &cobra.Command{
		Use:   "mailbox <address>",
		Short: "Show every threat event that touched one mailbox, newest first, across five Mimecast logs",
		Long: "Use this command when a user reports a suspicious email, to see everything that reached them or was stopped for them. " +
			"Do NOT use it to explain one message; use 'why' instead. Do NOT use it to scope a campaign by sender; use 'threats sender' instead.\n\n" +
			"Pages URL Protect click logs, Attachment Protect logs, Impersonation Protect logs, the 7-day gateway rejection log and the hold queue " +
			"for the window and keeps the events whose recipient is the mailbox (direction inbound) or whose sender is (outbound, a sign of a compromised account). " +
			"The TTP logs have no address filter, so they are matched client-side within --max-scan-pages. A domain matches every mailbox in it. " +
			"Each source reports what it scanned; fetch_failures and incomplete_sources say when the timeline is missing events, so a short answer is never read as clean.",
		Example:     "  mimecast-cli threats mailbox alice@example.com --since 7d --agent\n  mimecast-cli threats mailbox alice@example.com --since 24h",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "address=alice@example.com;--since=7d"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "threats mailbox")
			}
			target, err := threatTarget(args)
			if err != nil {
				_ = cmd.Usage()
				return err
			}
			s, cancel, err := o.newThreatScan(cmd, flags)
			if err != nil {
				return err
			}
			defer cancel()
			s.searchField, s.searchValue = "all", target
			s.match = func(sender string, recipients []string) string {
				if anyAddrMatches(target, recipients...) {
					return "inbound"
				}
				if addrMatches(sender, target) {
					return "outbound"
				}
				return ""
			}
			s.run()
			if err := s.finish(cmd, flags); err != nil {
				return err
			}
			view := threatsMailboxView{threatsReport: s.report(target), TotalEvents: len(s.events), ByKind: map[string]int{}, Events: []threatEvent{}}
			for _, e := range s.events {
				view.ByKind[e.Kind]++
				if o.limit <= 0 || len(view.Events) < o.limit {
					view.Events = append(view.Events, e)
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printNovelJSON(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s: %d threat event(s) since %s\n", target, view.TotalEvents, view.Since)
			for _, e := range view.Events {
				fmt.Fprintf(w, "  %s  %-8s %-13s %-28s %-10s %s\n", e.When, e.Direction, e.Kind, e.Sender, e.Action, cmpSubject(e))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&o.since, "since", "7d", "Lookback (24h, 7d) or an RFC3339 start time; the rejection log covers 7 days")
	cmd.Flags().IntVar(&o.maxScanPages, "max-scan-pages", 5, "Maximum pages (100 entries each) to scan per source")
	cmd.Flags().IntVar(&o.limit, "limit", 200, "Maximum events to return (counts still cover every match)")
	cmd.Flags().StringVar(&o.accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}

// cmpSubject is the subject, or the event detail when there is none.
func cmpSubject(e threatEvent) string {
	if e.Subject != "" {
		return e.Subject
	}
	return e.Detail
}
