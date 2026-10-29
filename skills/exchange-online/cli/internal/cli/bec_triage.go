// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"exchange-online-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

// Thresholds from CIPP's BecHeuristics.json (window.days 7; sentMail burstMessages 10,
// burstRecipients 30, burstWindowMinutes 10; repeatSubjectMessages 5,
// repeatSubjectRecipients 20):
// https://github.com/KelvinTegelaar/CIPP-API/blob/075913f36b9ab1395d7a9231924c573c40706c18/Config/BecHeuristics.json
const (
	becBurstMessages    = 10
	becBurstRecipients  = 30
	becBurstWindow      = 10 * time.Minute
	becRepeatMessages   = 5
	becRepeatRecipients = 20
)

type becSignal struct {
	Signal   string `json:"signal"`
	Severity string `json:"severity"`
	Evidence any    `json:"evidence"`
}

type becView struct {
	Tenant   string      `json:"tenant"`
	Mailbox  string      `json:"mailbox"`
	Since    string      `json:"since"`
	Severity string      `json:"severity"`
	Signals  []becSignal `json:"signals"`
	Outbound struct {
		Messages           int    `json:"messages"`
		ExternalRecipients int    `json:"distinct_external_recipients"`
		Complete           bool   `json:"complete"`
		Note               string `json:"note,omitempty"`
	} `json:"outbound"`
	Errors []sourceErr `json:"errors"`
}

func newNovelBecTriageCmd(flags *rootFlags) *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use:   "triage <tenantId> <mailbox>",
		Short: "Collect compromise evidence for one mailbox in one call.",
		Long: `Checks one mailbox for the Exchange-side signs of compromise: inbox rules matching CIPP's BEC
signatures, mailbox forwarding outside the tenant, restricted-sender status, mobile devices first
synced inside the window, outbound mail bursts and repeated subjects in message trace, and legacy
protocols (IMAP, POP, SMTP AUTH) still on. Each signal carries a severity and its evidence; a
lookup that fails is listed under errors and the rest still runs. Read-only.

Use this command to collect compromise evidence for ONE mailbox (rules, forwarding,
restricted-sender status, new mobile devices, outbound mail volume, legacy protocols). Do NOT use
this command for a tenant-wide sweep; use 'inbox-rules sweep' or 'forwarding-audit' instead. Do
NOT use this command to list the mailboxes this user can reach; use 'access-map' instead. To
remediate, use 'inbox-rules remove' and 'client-access set'.`,
		Example:     "  exchange-online-cli bec triage 0550b473-9fd2-4dbb-a058-a1640b4bf458 alex@example.com --agent\n  exchange-online-cli bec triage 0550b473-9fd2-4dbb-a058-a1640b4bf458 alex@example.com --since 3d --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "tenantId=0550b473-9fd2-4dbb-a058-a1640b4bf458;mailbox=alex@example.com"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "bec triage")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 || window > 90*24*time.Hour {
				return usageErr(fmt.Errorf("--since must be a positive duration up to 90d, got %q", since))
			}
			tenant, err := exoTenantArg(cmd, args, 0)
			if err != nil {
				return err
			}
			if len(args) < 2 || !strings.Contains(args[1], "@") {
				return usageErr(fmt.Errorf("missing <mailbox> (a UPN or email address)\nUsage: %s", cmd.UseLine()))
			}
			upn := strings.ToLower(strings.TrimSpace(args[1]))
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			now := time.Now().UTC()
			start := now.Add(-window)
			v := becView{Tenant: tenant, Mailbox: upn, Since: start.Format(time.RFC3339), Signals: []becSignal{}, Errors: []sourceErr{}}
			fail := func(src string, err error) { v.Errors = append(v.Errors, sourceErr{Source: src, Error: err.Error()}) }

			accepted, err := acceptedDomainSet(ctx, c, tenant)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			// Resolve the mailbox first: trace and restricted-sender data are keyed by
			// SMTP address, which can differ from the UPN the caller passed.
			dr := newDestResolver(ctx, c, tenant, accepted)
			m, err := getMailbox(ctx, c, tenant, upn, "ForwardingSmtpAddress,ForwardingAddress,DeliverToMailboxAndForward")
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if m == nil {
				return notFoundErr(fmt.Errorf("mailbox %s not found", upn))
			}
			addrs := smtpAddresses(m)
			addrs[upn] = true
			primary := firstNonEmpty(smtpOf(rs(m, "PrimarySmtpAddress")), upn)
			v.Mailbox = primary
			if rules, err := exoGet(ctx, c, exoBeta(tenant, "Get-InboxRule"), exoRow{"Mailbox": primary, "IncludeHidden": true}, ""); err != nil {
				fail("Get-InboxRule", err)
			} else {
				for _, r := range rules {
					f := ruleFlags(r, accepted)
					var targets []string
					for _, raw := range ruleTargets(r) {
						d := dr.resolve(raw)
						targets = append(targets, firstNonEmpty(d.Addr, raw))
						switch {
						case d.Unresolved != "" && !containsStr(f, "forwards-unresolved"):
							f = append(f, "forwards-unresolved") // could be a contact pointing outside
						case d.Addr != "" && isExternal(d.Addr, accepted) && !containsStr(f, "forwards-external"):
							f = append(f, "forwards-external") // e.g. an internal contact pointing outside
						}
					}
					if len(f) > 0 {
						sev := "medium"
						if containsStr(f, "forwards-external") || containsStr(f, "deletes-messages") || containsStr(f, "moves-to-low-visibility-folder") {
							sev = "high"
						}
						v.Signals = append(v.Signals, becSignal{"suspicious-inbox-rule", sev, map[string]any{
							"name": rs(r, "Name"), "identity": rs(r, "Identity"), "enabled": rb(r, "Enabled"), "flags": f, "forwards_to": targets}})
					}
				}
			}
			keep := rb(m, "DeliverToMailboxAndForward")
			// Set-Mailbox: with both set, ForwardingSmtpAddress is ignored, so only the
			// active target counts; resolve it to where mail really goes.
			active := firstNonEmpty(rs(m, "ForwardingAddress"), rs(m, "ForwardingSmtpAddress"))
			if active != "" {
				d := dr.resolve(active)
				ev := map[string]any{"configured": active, "delivers_to": d.Addr, "keeps_copy": keep}
				if rs(m, "ForwardingAddress") != "" && rs(m, "ForwardingSmtpAddress") != "" {
					ev["ignored_forwarding_smtp_address"] = smtpOf(rs(m, "ForwardingSmtpAddress"))
				}
				switch {
				case d.Unresolved != "":
					ev["unresolved"] = d.Unresolved
					v.Signals = append(v.Signals, becSignal{"mailbox-forwarding-unresolved", "medium", ev})
				case isExternal(d.Addr, accepted):
					v.Signals = append(v.Signals, becSignal{"external-mailbox-forwarding", "high", ev})
				default:
					v.Signals = append(v.Signals, becSignal{"mailbox-forwarding", "low", ev})
				}
			}
			if blocked, err := exoGet(ctx, c, exoBeta(tenant, "Get-BlockedSenderAddress"), nil, ""); err != nil {
				fail("Get-BlockedSenderAddress", err)
			} else {
				for _, b := range blocked {
					if addrs[smtpOf(rs(b, "SenderAddress"))] {
						v.Signals = append(v.Signals, becSignal{"restricted-sender", "high", map[string]any{"sender": smtpOf(rs(b, "SenderAddress")), "reason": rs(b, "Reason"), "created": rs(b, "CreatedDatetime")}})
					}
				}
			}
			if devs, err := exoGet(ctx, c, exoBeta(tenant, "Get-MobileDevice"), exoRow{"Mailbox": primary}, ""); err != nil {
				fail("Get-MobileDevice", err)
			} else {
				for _, d := range devs {
					if t, ok := parseExoTime(rs(d, "FirstSyncTime")); ok && t.After(start) {
						v.Signals = append(v.Signals, becSignal{"new-mobile-device", "medium", map[string]any{
							"device": firstNonEmpty(rs(d, "FriendlyName"), rs(d, "DeviceModel"), rs(d, "DeviceId")), "os": rs(d, "DeviceOS"), "first_sync": t.Format(time.RFC3339), "identity": rs(d, "Identity")}})
					}
				}
			}
			// Mail sent from an alias is traced under that alias, so query every SMTP proxy.
			senders := smtpProxies(m)
			if len(senders) == 0 {
				senders = []string{primary}
			}
			if trace, err := traceWindow(ctx, c, tenant, exoRow{"SenderAddress": senders}, start, now); err != nil {
				fail("Get-MessageTraceV2", err)
			} else {
				v.Outbound.Complete, v.Outbound.Note = trace.Complete, trace.Note
				v.Signals = append(v.Signals, outboundSignals(trace.Rows, accepted, &v)...)
			}
			if cas, err := exoGet(ctx, c, exoBeta(tenant, "Get-CASMailbox"), exoRow{"Identity": primary}, "ImapEnabled,PopEnabled,SmtpClientAuthenticationDisabled"); err != nil {
				fail("Get-CASMailbox", err)
			} else if len(cas) > 0 {
				var orgSmtp *bool
				if tc, err := exoGet(ctx, c, exoBeta(tenant, "Get-TransportConfig"), nil, "SmtpClientAuthenticationDisabled"); err != nil {
					fail("Get-TransportConfig", err)
				} else if len(tc) > 0 {
					orgSmtp = rb(tc[0], "SmtpClientAuthenticationDisabled")
				}
				mp := resolvePosture(cas[0], nil, orgSmtp)
				var on []string
				for _, p := range []string{"imap", "pop", "smtpauth"} {
					if mp.Enabled[p] {
						on = append(on, p)
					}
				}
				if len(on) > 0 {
					v.Signals = append(v.Signals, becSignal{"legacy-protocols-enabled", "low", map[string]any{"protocols": on, "smtp_auth_source": mp.SmtpAuth}})
				}
			}
			v.Errors = append(v.Errors, dr.Errors...)
			warnPartial(cmd, v.Errors, 7, "evidence")
			v.Severity = "none"
			for _, s := range v.Signals {
				if sevRank(s.Severity) > sevRank(v.Severity) {
					v.Severity = s.Severity
				}
			}
			sort.SliceStable(v.Signals, func(i, j int) bool { return sevRank(v.Signals[i].Severity) > sevRank(v.Signals[j].Severity) })
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Look-back window for devices and sent mail (CIPP's default is 7d; max 90d)")
	return cmd
}

// outboundSignals flags send bursts and repeated subjects in the sender's trace rows.
func outboundSignals(trace []exoRow, accepted map[string]bool, v *becView) []becSignal {
	type msg struct {
		at   time.Time
		id   string
		subj string
		rcpt string
	}
	var msgs []msg
	ext := map[string]bool{}
	ids := map[string]bool{}
	for _, r := range trace {
		at, _ := parseExoTime(rs(r, "Received"))
		// MessageTraceId is Exchange's own per-message ID; MessageId is sender-controlled.
		m := msg{at: at, id: firstNonEmpty(rs(r, "MessageTraceId"), rs(r, "MessageId")), subj: strings.ToLower(strings.TrimSpace(rs(r, "Subject"))), rcpt: smtpOf(rs(r, "RecipientAddress"))}
		msgs = append(msgs, m)
		ids[m.id] = true
		if isExternal(m.rcpt, accepted) {
			ext[m.rcpt] = true
		}
	}
	v.Outbound.Messages, v.Outbound.ExternalRecipients = len(ids), len(ext)
	var out []becSignal
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].at.Before(msgs[j].at) })
	// Sliding 10-minute window over trace rows (one row per recipient).
	for i, j := 0, 0; j < len(msgs); j++ {
		for msgs[j].at.Sub(msgs[i].at) > becBurstWindow {
			i++
		}
		winIDs, winRcpt := map[string]bool{}, map[string]bool{}
		for _, m := range msgs[i : j+1] {
			winIDs[m.id], winRcpt[m.rcpt] = true, true
		}
		if len(winIDs) >= becBurstMessages || len(winRcpt) >= becBurstRecipients {
			out = append(out, becSignal{"send-burst", "high", map[string]any{"window_start": msgs[i].at.Format(time.RFC3339), "messages": len(winIDs), "recipients": len(winRcpt)}})
			break
		}
	}
	bySubj := map[string]map[string]bool{}
	rcptBySubj := map[string]map[string]bool{}
	for _, m := range msgs {
		if m.subj == "" {
			continue
		}
		if bySubj[m.subj] == nil {
			bySubj[m.subj], rcptBySubj[m.subj] = map[string]bool{}, map[string]bool{}
		}
		bySubj[m.subj][m.id], rcptBySubj[m.subj][m.rcpt] = true, true
	}
	subjects := make([]string, 0, len(bySubj))
	for s := range bySubj {
		subjects = append(subjects, s)
	}
	sort.Strings(subjects)
	for _, s := range subjects {
		ids := bySubj[s]
		if len(ids) >= becRepeatMessages || len(rcptBySubj[s]) >= becRepeatRecipients {
			out = append(out, becSignal{"repeated-subject", "medium", map[string]any{"subject": s, "messages": len(ids), "recipients": len(rcptBySubj[s])}})
		}
	}
	return out
}

func sevRank(s string) int {
	switch s {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

// parseExoTime parses the date formats cmdlet output uses.
func parseExoTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "1/2/2006 3:04:05 PM"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
