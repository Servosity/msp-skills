// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: reject a flooding sender's held mail, optionally block it.
// pp:data-source live
// pp:client-call

package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mimecast-pp-cli/internal/cliutil"
)

// rejectReasonTypes is the documented hold-reject reasonType enum.
var rejectReasonTypes = []string{
	"MESSAGE CONTAINS UNDESIRABLE CONTENT",
	"MESSAGE CONTAINS CONFIDENTIAL INFORMATION",
	"REVIEWER DISAPPROVES OF CONTENT",
	"INAPPROPRIATE COMMUNICATION",
	"MESSAGE GOES AGAINST EMAIL POLICIES",
}

type blockPair struct {
	Sender string `json:"sender"`
	To     string `json:"to"`
}

type purgeView struct {
	Sender         string      `json:"sender"`
	Since          string      `json:"since"`
	Plan           bool        `json:"plan"`
	ScannedHeld    int         `json:"scanned_held"`
	Matched        int         `json:"matched"`
	MatchedIDs     []string    `json:"matched_ids"`
	Rejected       []string    `json:"rejected"`
	RejectFailures []string    `json:"reject_failures"`
	BlockPairs     []blockPair `json:"block_pairs"`
	Blocked        int         `json:"blocked"`
	BlockFailures  []string    `json:"block_failures"`
	Truncated      bool        `json:"truncated"`
	Note           string      `json:"note,omitempty"`
}

func newNovelHeldPurgeSenderCmd(flags *rootFlags) *cobra.Command {
	var since, reasonType, message, accountCode string
	var block, notify, plan bool
	var maxScanPages int

	cmd := &cobra.Command{
		Use:   "purge-sender [sender]",
		Short: "Reject every held message from one sender in a single command, optionally blocking it",
		Long: "Use this command when one sender is flooding the hold queue and every held message from that sender should be rejected, optionally blocking the sender for those recipients. " +
			"Do NOT use it to review mixed queue contents; use 'held triage' instead. Do NOT use it to release messages; use 'held release-safe' instead.\n\n" +
			"The sender is an address (spam@bad.example) or a domain (bad.example, including subdomains). Without --yes the command only plans: it lists the " +
			"matching held messages and the block pairs and changes nothing. With --yes, matching messages are rejected in batches with the --reason-type " +
			"you choose, and --block adds a managed-sender block for each (matched sender address, recipient) pair. An ID counts as rejected only when " +
			"Mimecast confirms reject:true. MX-based (Cloud Gateway) deployments only.",
		Example:     "  mimecast-cli held purge-sender spam@bad.example --since 7d --plan\n  mimecast-cli held purge-sender bad.example --since 7d --block --yes\n  mimecast-cli held purge-sender spam@bad.example --reason-type \"MESSAGE GOES AGAINST EMAIL POLICIES\" --notify --yes",
		Annotations: map[string]string{"pp:data-source": "live", "pp:happy-args": "sender=spam@bad.example;--since=7d;--plan", "mimecast:sends-with-yes": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "held purge-sender")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("exactly one sender address or domain is required"))
			}
			if !slices.Contains(rejectReasonTypes, reasonType) {
				return usageErr(fmt.Errorf("--reason-type must be one of: %s", strings.Join(rejectReasonTypes, ", ")))
			}
			sender := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(args[0]), "@"))
			// ponytail: dot check only; a multi-label public suffix (co.uk) still
			// passes, which the default plan-only mode makes visible before --yes.
			if !strings.Contains(sender, "@") && !strings.Contains(sender, ".") {
				return usageErr(fmt.Errorf("sender must be an address (spam@bad.example) or a domain (bad.example), not %q", args[0]))
			}
			// Rejecting mail is irreversible: without --yes this only plans.
			if !flags.yes {
				plan = true
			}
			now := time.Now()
			start, err := parseSince(since, now)
			if err != nil {
				return usageErr(err)
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			acct := mcAccountCode(accountCode)
			search := []map[string]any{{"fieldName": "from", "value": sender}}
			c, err := flags.newClient()
			if err != nil {
				return classifyAPIError(cmd.ErrOrStderr(), err, flags)
			}
			held, page, err := fetchHeld(ctx, c, start, now, acct, maxScanPages, search)
			if err != nil {
				return classifyAPIError(cmd.ErrOrStderr(), err, flags)
			}

			view := purgeView{Sender: sender, Since: mcTime(start), Plan: plan, ScannedHeld: len(held), Truncated: page.Truncated,
				MatchedIDs: []string{}, Rejected: []string{}, RejectFailures: []string{}, BlockPairs: []blockPair{}, BlockFailures: []string{}}
			pairs := map[blockPair]bool{}
			for _, h := range held {
				// The API search is fuzzy; keep exact sender/domain matches, and
				// block the identity that actually matched (never an unrelated
				// envelope sender behind a matching header).
				matched := ""
				switch {
				case addrMatches(h.FromEnv.EmailAddress, sender):
					matched = h.FromEnv.EmailAddress
				case addrMatches(h.FromHdr.EmailAddress, sender):
					matched = h.FromHdr.EmailAddress
				default:
					continue
				}
				view.MatchedIDs = append(view.MatchedIDs, h.ID)
				for _, to := range h.recipients() {
					pairs[blockPair{Sender: strings.ToLower(matched), To: strings.ToLower(to)}] = true
				}
			}
			view.Matched = len(view.MatchedIDs)
			if block {
				for p := range pairs {
					view.BlockPairs = append(view.BlockPairs, p)
				}
				sort.Slice(view.BlockPairs, func(i, j int) bool {
					return view.BlockPairs[i].Sender+view.BlockPairs[i].To < view.BlockPairs[j].Sender+view.BlockPairs[j].To
				})
			}
			if view.Matched == 0 {
				view.Note = "no held messages from this sender in the window"
			}

			var firstCause error
			if !plan && view.Matched > 0 {
				for batch := range slices.Chunk(view.MatchedIDs, 25) {
					item := map[string]any{"ids": batch, "reasonType": reasonType, "notify": notify}
					if message != "" {
						item["message"] = message
					}
					resp, err := mcWrite(ctx, c, "/api/gateway/hold-reject", mcBody([]any{item}, 0, "", acct))
					if err != nil {
						view.RejectFailures = append(view.RejectFailures, fmt.Sprintf("%s: %v", strings.Join(batch, ","), err))
						if firstCause == nil {
							firstCause = err
						}
						continue
					}
					// Count an ID as rejected only on an explicit reject:true.
					rejected := map[string]bool{}
					for _, raw := range resp.Data {
						var r struct {
							ID     string `json:"id"`
							Reject bool   `json:"reject"`
						}
						if json.Unmarshal(raw, &r) == nil && r.ID != "" {
							rejected[r.ID] = r.Reject
						}
					}
					failed := map[string]string{}
					for _, f := range resp.Fail {
						for _, id := range f.keyIDs() {
							failed[id] = f.text()
						}
					}
					for _, id := range batch {
						switch {
						case failed[id] != "":
							view.RejectFailures = append(view.RejectFailures, id+": "+failed[id])
						case rejected[id]:
							view.Rejected = append(view.Rejected, id)
						default:
							view.RejectFailures = append(view.RejectFailures, id+": Mimecast did not confirm reject:true for this ID")
						}
					}
				}
				if block {
					items := make([]any, 0, len(view.BlockPairs))
					for _, p := range view.BlockPairs {
						items = append(items, map[string]any{"sender": p.Sender, "to": p.To, "action": "block"})
					}
					for chunk := range slices.Chunk(items, 100) {
						resp, err := mcWrite(ctx, c, "/api/managedsender/permit-or-block-sender", mcBody(chunk, 0, "", acct))
						if err != nil {
							view.BlockFailures = append(view.BlockFailures, err.Error())
							if firstCause == nil {
								firstCause = err
							}
							continue
						}
						view.Blocked += len(resp.Data)
						view.BlockFailures = append(view.BlockFailures, resp.failMessages()...)
						if missing := len(chunk) - len(resp.Data) - len(resp.Fail); missing > 0 {
							view.BlockFailures = append(view.BlockFailures, fmt.Sprintf("%d block pair(s) neither confirmed nor reported failed by Mimecast", missing))
						}
					}
				}
			}
			if n := page.note("rerun to reach the rest"); n != "" {
				view.Note = strings.TrimSpace(view.Note + " " + n)
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				verb, count := "rejected", len(view.Rejected)
				if plan {
					verb, count = "would reject", view.Matched
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %d held message(s) matched, %s %d", sender, view.Matched, verb, count)
				if block {
					fmt.Fprintf(cmd.OutOrStdout(), ", %d block pair(s)", len(view.BlockPairs))
				}
				fmt.Fprintln(cmd.OutOrStdout())
				for _, f := range append(view.RejectFailures, view.BlockFailures...) {
					fmt.Fprintf(cmd.ErrOrStderr(), "failure: %s\n", f)
				}
				if view.Note != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "note: %s\n", view.Note)
				}
			} else if err := printNovelJSON(cmd.OutOrStdout(), view, flags); err != nil {
				return err
			}
			if n := len(view.RejectFailures) + len(view.BlockFailures); n > 0 {
				summary := fmt.Errorf("%d reject failure(s), %d block failure(s)", len(view.RejectFailures), len(view.BlockFailures))
				return mcWriteOutcomeErr(summary, firstCause)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "How far back to look in the hold queue")
	cmd.Flags().StringVar(&reasonType, "reason-type", rejectReasonTypes[0], "Reject reason sent to Mimecast (one of the five documented reasonType values)")
	cmd.Flags().StringVar(&message, "message", "", "Optional rejection message returned to the sender")
	cmd.Flags().BoolVar(&notify, "notify", false, "Send a rejection notification to the sender")
	cmd.Flags().BoolVar(&block, "block", false, "Also block the sender for every recipient it targeted (managed senders)")
	cmd.Flags().BoolVar(&plan, "plan", false, "Show what would be rejected and blocked without changing anything (the default unless --yes is given)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum hold-queue pages (100 messages each) to scan")
	cmd.Flags().StringVar(&accountCode, "account-code", "", "Delegated customer account code (MSP); defaults to MIMECAST_ACCOUNT_CODE")
	return cmd
}
