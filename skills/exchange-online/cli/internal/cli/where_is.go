// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"exchange-online-pp-cli/internal/client"
	"exchange-online-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type traceVerdict struct {
	Received       string `json:"received"`
	Sender         string `json:"sender"`
	Recipient      string `json:"recipient"`
	Subject        string `json:"subject"`
	Status         string `json:"status"`
	Verdict        string `json:"verdict"`
	Detail         string `json:"detail,omitempty"`
	NextAction     string `json:"next_action,omitempty"`
	MessageID      string `json:"message_id,omitempty"`
	MessageTraceID string `json:"message_trace_id,omitempty"`
}

type whereIsView struct {
	Tenant   string         `json:"tenant"`
	From     string         `json:"from"`
	To       string         `json:"to"`
	Messages []traceVerdict `json:"messages"`
	Complete bool           `json:"trace_complete"`
	Hint     string         `json:"hint,omitempty"`
	Errors   []sourceErr    `json:"errors"`
}

func newNovelWhereIsCmd(flags *rootFlags) *cobra.Command {
	var recipient, sender, subject, since string
	cmd := &cobra.Command{
		Use:   "where-is <tenantId>",
		Short: "Explain why a message did or did not reach a recipient, with the next action.",
		Long: `Runs Get-MessageTraceV2 for the recipient (and optional sender and subject text) over the
window, then explains each match: quarantined messages are looked up with Get-QuarantineMessage
(type, policy, release state, whether only an admin can release), junk-filtered and in-transit
messages are labeled, and failed messages from an internal sender are checked against the
restricted-senders list. Each row ends with the next command to run.

Use this command to explain why one message did or did not reach a recipient (trace status joined
to quarantine, junk and restricted-sender state). Do NOT use this command to list pending release
requests; use 'quarantine queue' instead. Do NOT use this command to export raw trace rows; use
'message-trace list' instead.`,
		Example:     "  exchange-online-cli where-is 0550b473-9fd2-4dbb-a058-a1640b4bf458 --recipient alex@example.com --since 48h --agent\n  exchange-online-cli where-is 0550b473-9fd2-4dbb-a058-a1640b4bf458 --recipient alex@example.com --sender billing@vendor.example --subject invoice",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "tenantId=0550b473-9fd2-4dbb-a058-a1640b4bf458;--recipient=alex@example.com"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "where-is")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 || window > 90*24*time.Hour {
				return usageErr(fmt.Errorf("--since must be a positive duration up to 90d (message trace history), got %q", since))
			}
			tenant, err := exoTenantArg(cmd, args, 0)
			if err != nil {
				return err
			}
			if !strings.Contains(recipient, "@") {
				return usageErr(fmt.Errorf("--recipient is required (an email address)"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			now := time.Now().UTC()
			start := now.Add(-window)
			v := whereIsView{Tenant: tenant, From: start.Format(time.RFC3339), To: now.Format(time.RFC3339), Messages: []traceVerdict{}, Errors: []sourceErr{}}
			filter := exoRow{"RecipientAddress": recipient}
			if sender != "" {
				filter["SenderAddress"] = sender
			}
			trace, err := traceWindow(ctx, c, tenant, filter, start, now)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			rows := trace.Rows
			v.Complete = trace.Complete
			var accepted map[string]bool
			var blocked map[string]bool
			for _, r := range rows {
				subj := rs(r, "Subject")
				if subject != "" && !strings.Contains(strings.ToLower(subj), strings.ToLower(subject)) {
					continue
				}
				tv := traceVerdict{Received: rs(r, "Received"), Sender: smtpOf(rs(r, "SenderAddress")), Recipient: smtpOf(rs(r, "RecipientAddress")),
					Subject: subj, Status: rs(r, "Status"), MessageID: rs(r, "MessageId"), MessageTraceID: rs(r, "MessageTraceId")}
				detailCmd := fmt.Sprintf("exchange-online-cli message-trace detail %s --message-trace-id %s --recipient-address %s", tenant, tv.MessageTraceID, tv.Recipient)
				switch strings.ToLower(tv.Status) {
				case "delivered":
					tv.Verdict = "delivered"
					tv.NextAction = fmt.Sprintf("exchange-online-cli inbox-rules list %s --mailbox %s  (a rule may have moved or deleted it)", tenant, tv.Recipient)
				case "filteredasspam":
					tv.Verdict, tv.Detail = "delivered-to-junk", "spam filtering delivered it to Junk Email"
					tv.NextAction = "check the recipient's Junk Email folder"
				case "quarantined":
					tv.Verdict = "quarantined"
					match, err := quarantineForTrace(ctx, c, tenant, tv)
					if err != nil {
						v.Errors = append(v.Errors, sourceErr{Source: "Get-QuarantineMessage " + tv.MessageID, Error: err.Error()})
					} else {
						switch len(match) {
						case 0:
							tv.Detail = "no quarantined copy found for this message and recipient"
						case 1:
							qt := rs(match[0], "Type")
							tv.Detail = fmt.Sprintf("type %s, policy %s, release status %s, expires %s", orDash(qt), orDash(rs(match[0], "PolicyName")), orDash(rs(match[0], "ReleaseStatus")), orDash(rs(match[0], "Expires")))
							// Release to this recipient only: --release-to-all would also release other recipients' copies.
							tv.NextAction = fmt.Sprintf("exchange-online-cli quarantine release %s --identity '%s' --user %s", tenant, rs(match[0], "Identity"), tv.Recipient)
							if adminOnlyQuarantine(qt) {
								tv.Detail += "; admin-only release under the default quarantine policy"
							}
						default:
							tv.Detail = fmt.Sprintf("%d quarantined messages share this Message-ID and recipient; pick the right one before releasing", len(match))
							tv.NextAction = fmt.Sprintf("exchange-online-cli quarantine list %s --recipient-address %s --json", tenant, tv.Recipient)
						}
					}
				case "failed":
					tv.Verdict, tv.NextAction = "failed", detailCmd
					if accepted == nil {
						if accepted, err = acceptedDomainSet(ctx, c, tenant); err != nil {
							v.Errors = append(v.Errors, sourceErr{Source: "Get-AcceptedDomain", Error: err.Error()})
							accepted = map[string]bool{}
						}
					}
					if domainOf(tv.Sender) != "" && !isExternal(tv.Sender, accepted) {
						if blocked == nil {
							blocked = map[string]bool{}
							b, err := exoGet(ctx, c, exoBeta(tenant, "Get-BlockedSenderAddress"), nil, "SenderAddress")
							if err != nil {
								v.Errors = append(v.Errors, sourceErr{Source: "Get-BlockedSenderAddress", Error: err.Error()})
							}
							for _, x := range b {
								blocked[smtpOf(rs(x, "SenderAddress"))] = true
							}
						}
						// Restricted senders are listed by UPN, which can differ from the SMTP
						// address in the trace, so match every address of the sender's mailbox.
						keys := map[string]bool{tv.Sender: true}
						if m, err := getMailbox(ctx, c, tenant, tv.Sender, ""); err != nil {
							v.Errors = append(v.Errors, sourceErr{Source: "Get-Mailbox " + tv.Sender, Error: err.Error()})
						} else if m != nil {
							keys = smtpAddresses(m)
						}
						for k := range keys {
							if blocked[k] {
								tv.Verdict, tv.Detail = "sender-restricted", "the internal sender is on the restricted senders list (outbound spam)"
								tv.NextAction = fmt.Sprintf("exchange-online-cli blocked-senders remove %s --sender-address %s  (after checking the account is not compromised)", tenant, k)
								break
							}
						}
					}
				case "pending", "gettingstatus", "expanded", "resolved":
					tv.Verdict, tv.NextAction = "in-transit-or-expanded", detailCmd
				default:
					tv.Verdict, tv.NextAction = strings.ToLower(orDash(tv.Status)), detailCmd
				}
				v.Messages = append(v.Messages, tv)
			}
			warnPartial(cmd, v.Errors, len(v.Messages), "follow-up")
			// Newest first by parsed time; unparseable times sort last.
			sort.SliceStable(v.Messages, func(i, j int) bool {
				a, aok := parseExoTime(v.Messages[i].Received)
				b, bok := parseExoTime(v.Messages[j].Received)
				if aok != bok {
					return aok
				}
				return a.After(b)
			})
			switch {
			case !trace.Complete:
				v.Hint = trace.Note
			case len(v.Messages) == 0:
				v.Hint = "No trace rows match. Widen --since (up to 90d), drop --subject, or check the sender's address; the message may never have reached Exchange Online."
			}
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}
	cmd.Flags().StringVar(&recipient, "recipient", "", "Recipient email address (required)")
	cmd.Flags().StringVar(&sender, "sender", "", "Only messages from this sender")
	cmd.Flags().StringVar(&subject, "subject", "", "Only messages whose subject contains this text")
	cmd.Flags().StringVar(&since, "since", "48h", "How far back to trace (max 90d, queried in 10-day slices)")
	return cmd
}

// quarantineForTrace finds the quarantine record for one trace row: filtered
// server-side by Message-ID, sender and recipient, bounded to a day either side
// of the trace time (the cmdlet's default window is shorter than trace history),
// every page read, and each row's sender and recipient verified.
func quarantineForTrace(ctx context.Context, c *client.Client, tenant string, tv traceVerdict) ([]exoRow, error) {
	if tv.Sender == "" || tv.Recipient == "" || tv.MessageID == "" {
		return nil, nil
	}
	params := exoRow{"MessageId": tv.MessageID, "SenderAddress": tv.Sender, "RecipientAddress": tv.Recipient, "PageSize": quarantinePageSize}
	if at, ok := parseExoTime(tv.Received); ok {
		params["StartReceivedDate"] = traceTime(at.Add(-24 * time.Hour))
		params["EndReceivedDate"] = traceTime(at.Add(24 * time.Hour))
	}
	var out []exoRow
	for page := 1; ; page++ {
		params["Page"] = page
		rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-QuarantineMessage"), params, "")
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			recips := map[string]bool{}
			for _, a := range rl(r, "RecipientAddress") {
				recips[smtpOf(a)] = true
			}
			if smtpOf(rs(r, "SenderAddress")) == tv.Sender && recips[tv.Recipient] {
				out = append(out, r)
			}
		}
		if len(rows) < quarantinePageSize {
			return out, nil
		}
		if page >= quarantineMaxPages {
			return nil, fmt.Errorf("more than %d pages of quarantine records for one message", quarantineMaxPages)
		}
	}
}

// adminOnlyQuarantine reports quarantine types the default policies reserve for
// admins (AdminOnlyAccessPolicy: high confidence phishing and malware).
func adminOnlyQuarantine(t string) bool {
	t = strings.ToLower(t)
	return strings.Contains(t, "high confidence phish") || strings.Contains(t, "highconfphish") || strings.Contains(t, "malware")
}
