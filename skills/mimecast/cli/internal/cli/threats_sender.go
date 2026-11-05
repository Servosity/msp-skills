// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: who one sender (or domain) reached.
// pp:data-source live
// pp:client-call

package cli

import (
	"cmp"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type threatRecipient struct {
	Recipient             string         `json:"recipient"`
	Clicked               bool           `json:"clicked"`
	ClickedThroughWarning bool           `json:"clicked_through_warning"`
	CurrentlyHeld         int            `json:"currently_held"`
	Events                int            `json:"events"`
	ByKind                map[string]int `json:"by_kind"`
	Senders               []string       `json:"senders"`
	MessageIDs            []string       `json:"message_ids"`
	HeldIDs               []string       `json:"held_ids"`
	LastSeen              string         `json:"last_seen"`
}

type threatsSenderView struct {
	threatsReport
	TotalEvents int `json:"total_events"`
	// TotalRecipients counts every recipient reached; Recipients holds the
	// first --limit, clickers first.
	TotalRecipients int               `json:"total_recipients"`
	Clicked         int               `json:"recipients_clicked"`
	CurrentlyHeld   int               `json:"recipients_with_held_messages"`
	ByKind          map[string]int    `json:"by_kind"`
	Recipients      []threatRecipient `json:"recipients"`
}

func newNovelThreatsSenderCmd(flags *rootFlags) *cobra.Command {
	var o threatsFlags

	cmd := &cobra.Command{
		Use:   "sender <address|@domain>",
		Short: "List every recipient one sender or domain reached, with who clicked and what is still held",
		Long: "Use this command to scope a phishing campaign after one report: which users received mail from the sender, which clicked, and which still have messages in the hold queue. " +
			"Do NOT use it for one user's whole history; use 'threats mailbox' instead. Do NOT use it to reject the held messages; use 'held purge-sender' instead.\n\n" +
			"Give a sender address or a domain (@phish.example or phish.example; a domain includes its subdomains). Pages URL Protect click logs, Attachment Protect logs, " +
			"Impersonation Protect logs, the 7-day gateway rejection log and the hold queue for the window and groups the sender's events by recipient: " +
			"counts by kind, whether the recipient clicked (and clicked through a warning), and the held message IDs. The TTP logs have no sender filter, so they are matched " +
			"client-side within --max-scan-pages. fetch_failures and incomplete_sources say when recipients may be missing.",
		Example:     "  mimecast-cli threats sender @phish.example --since 3d --agent\n  mimecast-cli threats sender billing@vendor.example --since 7d",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "sender=@phish.example;--since=3d"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "threats sender")
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
			s.searchField, s.searchValue = "from", target
			s.match = func(sender string, _ []string) string {
				if addrMatches(sender, target) {
					return "*"
				}
				return ""
			}
			s.run()
			if err := s.finish(cmd, flags); err != nil {
				return err
			}
			view := buildThreatsSender(s.report(target), s.events, o.limit)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printNovelJSON(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s reached %d recipient(s) since %s: %d clicked, %d with held messages\n", target, view.TotalRecipients, view.Since, view.Clicked, view.CurrentlyHeld)
			for _, r := range view.Recipients {
				flag := ""
				if r.Clicked {
					flag = "CLICKED"
				}
				fmt.Fprintf(w, "  %-36s %-8s held=%d events=%d last=%s\n", r.Recipient, flag, r.CurrentlyHeld, r.Events, r.LastSeen)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&o.since, "since", "7d", "Lookback (24h, 7d) or an RFC3339 start time; the rejection log covers 7 days")
	cmd.Flags().IntVar(&o.maxScanPages, "max-scan-pages", 5, "Maximum pages (100 entries each) to scan per source")
	cmd.Flags().IntVar(&o.limit, "limit", 200, "Maximum recipients to return (counts still cover every match)")
	cmd.Flags().StringVar(&o.accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}

// buildThreatsSender groups a sender's events (newest first) by recipient.
// Events with no recipient are grouped under "(unknown)".
func buildThreatsSender(rep threatsReport, events []threatEvent, limit int) threatsSenderView {
	view := threatsSenderView{threatsReport: rep, TotalEvents: len(events), ByKind: map[string]int{}, Recipients: []threatRecipient{}}
	byRcpt := map[string]*threatRecipient{}
	var order []string
	for _, e := range events {
		view.ByKind[e.Kind]++
		key := cmp.Or(strings.ToLower(strings.TrimSpace(e.Recipient)), "(unknown)")
		r := byRcpt[key]
		if r == nil {
			r = &threatRecipient{Recipient: key, ByKind: map[string]int{}, Senders: []string{}, MessageIDs: []string{}, HeldIDs: []string{}, LastSeen: e.When}
			byRcpt[key] = r
			order = append(order, key)
		}
		r.Events++
		r.ByKind[e.Kind]++
		if e.Kind == "url-click" {
			r.Clicked = true
			r.ClickedThroughWarning = r.ClickedThroughWarning || e.ClickedThroughWarning
		}
		if e.Kind == "held" {
			r.CurrentlyHeld++
			r.HeldIDs = appendUnique(r.HeldIDs, e.HeldID)
		}
		r.Senders = appendUnique(r.Senders, strings.ToLower(e.Sender))
		r.MessageIDs = appendUnique(r.MessageIDs, e.MessageID)
	}
	all := make([]threatRecipient, 0, len(order))
	for _, k := range order {
		r := byRcpt[k]
		if r.Clicked {
			view.Clicked++
		}
		if r.CurrentlyHeld > 0 {
			view.CurrentlyHeld++
		}
		all = append(all, *r)
	}
	// Clickers first, then the most events; ties keep newest-first order.
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Clicked != all[j].Clicked {
			return all[i].Clicked
		}
		return all[i].Events > all[j].Events
	})
	view.TotalRecipients = len(all)
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	view.Recipients = all
	return view
}

func appendUnique(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
