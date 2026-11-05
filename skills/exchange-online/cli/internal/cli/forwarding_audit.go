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

// Gate rules, from Microsoft Learn "Control external email forwarding":
// https://learn.microsoft.com/en-us/defender-office-365/outbound-spam-policies-external-email-forwarding
//   - The outbound spam policy's automatic forwarding setting governs inbox-rule and
//     mailbox (SMTP) forwarding to external recipients: On allows, Off blocks (NDR 5.7.520),
//     Automatic behaves as Off for most organizations since 2021 but can still mean On.
//   - Remote domains can also block automatic forwarding; when one control allows and
//     another blocks, the block wins.
//   - Forwarding between internal users is not affected.
// Mail flow (transport) rule redirects are admin rules, not automatic forwarding, so they
// are reported as leaving the tenant when enabled. Risky actions per CIPP BecHeuristics.json
// transportRules.riskyParameterRegex: BlindCopyTo, RedirectMessageTo, CopyTo, AddToRecipients.

type forwardPath struct {
	Source   string `json:"source"`
	Owner    string `json:"owner"`
	Rule     string `json:"rule,omitempty"`
	Target   string `json:"target"`
	External bool   `json:"external"`
	Verdict  string `json:"verdict"`
	Reason   string `json:"reason"`
	KeepCopy *bool  `json:"keeps_copy,omitempty"`
}

type forwardingView struct {
	Tenant         string `json:"tenant"`
	OutboundPolicy struct {
		DefaultMode    string   `json:"default_auto_forwarding_mode"`
		CustomPolicies []string `json:"custom_policies,omitempty"`
	} `json:"outbound_policy"`
	InboxRules struct {
		Source     string `json:"source"`
		StoredAt   string `json:"stored_at,omitempty"`
		Incomplete bool   `json:"incomplete,omitempty"`
	} `json:"inbox_rules"`
	Counts map[string]int `json:"counts"`
	Paths  []forwardPath  `json:"paths"`
	Notes  []string       `json:"notes,omitempty"`
	Errors []sourceErr    `json:"errors"`
}

func newNovelForwardingAuditCmd(flags *rootFlags) *cobra.Command {
	var sweep, all bool
	var concurrency, maxMailboxes int
	cmd := &cobra.Command{
		Use:   "forwarding-audit <tenantId>",
		Short: "List every way mail can auto-forward out of a tenant and whether policy blocks it.",
		Long: `Collects mailbox forwarding (ForwardingSmtpAddress, ForwardingAddress resolved to its address),
inbox-rule forward and redirect targets, and transport-rule redirect/BCC/copy targets. Each target
outside the accepted domains gets a verdict from the default outbound spam policy's
AutoForwardingMode and the matching remote domain's AutoForwardEnabled, following Microsoft's
precedence (a block from either control wins). Inbox rules come from the last 'inbox-rules sweep'
stored for this tenant; pass --sweep to sweep now. Internal targets are listed only with --all.

Use this command to list every way mail can auto-forward out of one tenant (mailbox forwarding,
inbox-rule forward/redirect, transport-rule redirect) and whether the outbound spam policy and
remote-domain gates block it. Do NOT use this command to find hide/delete-style suspicious inbox
rules; use 'inbox-rules sweep' instead. Do NOT use this command to investigate one
suspected-compromised user; use 'bec triage' instead.`,
		Example:     "  exchange-online-cli forwarding-audit 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent\n  exchange-online-cli forwarding-audit 0550b473-9fd2-4dbb-a058-a1640b4bf458 --sweep --json",
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:write-flags": "sweep", "pp:preview-happy-path": "true", "pp:data-source": "live", "pp:happy-args": "tenantId=0550b473-9fd2-4dbb-a058-a1640b4bf458"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "forwarding-audit")
			}
			if concurrency < 1 || concurrency > 16 || maxMailboxes < 1 {
				return usageErr(fmt.Errorf("--concurrency must be 1-16 and --max-mailboxes at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxMailboxes > 10 {
				maxMailboxes = 10
			}
			tenant, err := exoTenantArg(cmd, args, 0)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			v, err := forwardingAudit(cmd.Context(), c, tenant, sweep, concurrency, maxMailboxes)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			warnPartial(cmd, v.Errors, len(v.Paths), "forwarding")
			if !all {
				kept := v.Paths[:0]
				for _, p := range v.Paths {
					if p.External {
						kept = append(kept, p)
					}
				}
				v.Paths = kept
			}
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}
	cmd.Flags().BoolVar(&sweep, "sweep", false, "Run 'inbox-rules sweep' first instead of using the stored sweep")
	cmd.Flags().BoolVar(&all, "all", false, "Also list forwarding to internal recipients")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Mailboxes queried in parallel during --sweep (1-16)")
	cmd.Flags().IntVar(&maxMailboxes, "max-mailboxes", 2000, "Most mailboxes to sweep with --sweep")
	return cmd
}

func forwardingAudit(ctx context.Context, c *client.Client, tenant string, sweep bool, concurrency, maxMailboxes int) (forwardingView, error) {
	v := forwardingView{Tenant: tenant, Counts: map[string]int{}, Paths: []forwardPath{}, Errors: []sourceErr{}}
	accepted, err := acceptedDomainSet(ctx, c, tenant)
	if err != nil {
		return v, err
	}
	policies, err := exoGet(ctx, c, exoBeta(tenant, "Get-HostedOutboundSpamFilterPolicy"), nil, "Name,IsDefault,AutoForwardingMode")
	if err != nil {
		return v, err
	}
	for _, p := range policies {
		if isTrue(rb(p, "IsDefault")) {
			v.OutboundPolicy.DefaultMode = rs(p, "AutoForwardingMode")
		} else {
			v.OutboundPolicy.CustomPolicies = append(v.OutboundPolicy.CustomPolicies, rs(p, "Name")+"="+rs(p, "AutoForwardingMode"))
		}
	}
	if len(v.OutboundPolicy.CustomPolicies) > 0 {
		v.Notes = append(v.Notes, "Custom outbound spam policies exist; verdicts use the default policy, so users scoped to a custom policy may differ.")
	}
	remote, err := exoGet(ctx, c, exoBeta(tenant, "Get-RemoteDomain"), nil, "DomainName,AutoForwardEnabled")
	if err != nil {
		return v, err
	}
	gate := func(target string) (string, string) {
		rd, allowed := remoteDomainFor(remote, domainOf(target))
		mode := v.OutboundPolicy.DefaultMode
		switch {
		case !allowed:
			return "blocked-by-remote-domain", "remote domain " + rd + " has AutoForwardEnabled false"
		case strings.EqualFold(mode, "Off"):
			return "blocked-by-outbound-policy", "default outbound spam policy AutoForwardingMode Off"
		case strings.EqualFold(mode, "On"):
			return "leaves-tenant", "outbound policy On and remote domain " + rd + " allows auto-forwarding"
		default:
			return "depends-on-automatic-policy", "outbound policy AutoForwardingMode " + orDash(mode) + ": Off for most tenants since 2021, On for some older tenants; set On or Off explicitly"
		}
	}
	dr := newDestResolver(ctx, c, tenant, accepted)
	add := func(p forwardPath) {
		d := dr.resolve(p.Target)
		if d.Unresolved != "" {
			// It may be a contact or a deleted recipient: surface it rather than guess.
			p.External, p.Verdict, p.Reason = true, "unresolved-recipient", "could not resolve "+p.Target+": "+d.Unresolved
			v.Paths = append(v.Paths, p)
			v.Counts[p.Verdict]++
			return
		}
		resolved := ""
		if d.Addr != smtpOf(p.Target) {
			resolved = "resolves to " + d.Addr + "; "
		}
		p.Target = d.Addr
		p.External = isExternal(p.Target, accepted)
		switch {
		case !p.External:
			p.Verdict, p.Reason = "internal", "target is in an accepted domain"
		case p.Verdict == "":
			p.Verdict, p.Reason = gate(p.Target)
		}
		p.Reason = resolved + p.Reason
		v.Paths = append(v.Paths, p)
		v.Counts[p.Verdict]++
	}

	// Mailbox (SMTP) forwarding.
	mbx, err := exoGet(ctx, c, exoBeta(tenant, "Get-Mailbox"), exoRow{"ResultSize": "Unlimited"},
		"UserPrincipalName,PrimarySmtpAddress,ForwardingSmtpAddress,ForwardingAddress,DeliverToMailboxAndForward")
	if err != nil {
		return v, err
	}
	for _, m := range mbx {
		owner := rs(m, "UserPrincipalName")
		keep := rb(m, "DeliverToMailboxAndForward")
		fa := rs(m, "ForwardingAddress")
		if t := rs(m, "ForwardingSmtpAddress"); t != "" {
			if fa == "" {
				add(forwardPath{Source: "mailbox-forwarding", Owner: owner, Target: smtpOf(t), KeepCopy: keep})
			} else {
				// Set-Mailbox: with both set, ForwardingSmtpAddress is ignored.
				v.Paths = append(v.Paths, forwardPath{Source: "mailbox-forwarding", Owner: owner, Target: smtpOf(t), External: isExternal(smtpOf(t), accepted),
					Verdict: "inactive", Reason: "ForwardingAddress is also set, so Exchange ignores ForwardingSmtpAddress", KeepCopy: keep})
				v.Counts["inactive"]++
			}
		}
		if fa != "" {
			add(forwardPath{Source: "mailbox-forwarding", Owner: owner, Target: fa, KeepCopy: keep})
		}
	}

	// Inbox rules, from a fresh or stored sweep.
	var rules []exoRow
	if sweep {
		sv, stored, err := sweepInboxRules(ctx, c, tenant, auditMailboxTypes, concurrency, maxMailboxes)
		if err != nil {
			return v, err
		}
		at, err := storeSweep(ctx, tenant, sv, stored)
		if err != nil {
			return v, err
		}
		rules = stored
		v.InboxRules.Source, v.InboxRules.StoredAt, v.InboxRules.Incomplete = "sweep", at.Format(time.RFC3339), len(sv.Errors) > 0 || sv.Note != ""
		if sv.Note != "" {
			v.Notes = append(v.Notes, sv.Note)
		}
		v.Errors = append(v.Errors, sv.Errors...)
	} else {
		got, err := readSnapshots(ctx, tenant, "inbox_rule", "inbox_rule_meta")
		if err != nil {
			return v, err
		}
		snap, meta := got["inbox_rule"], got["inbox_rule_meta"]
		switch {
		case len(meta) == 0:
			v.InboxRules.Source = "none"
			v.Notes = append(v.Notes, "No inbox-rule sweep stored for this tenant; run with --sweep or 'inbox-rules sweep' first.")
		default:
			v.InboxRules.Source = "stored-sweep"
			v.InboxRules.StoredAt = meta[0].FetchedAt.Format(time.RFC3339)
			n, _ := meta[0].Data["errors"].(float64)
			if n > 0 || isTrue(rb(meta[0].Data, "truncated")) || !sweepCovers(meta[0].Data, auditMailboxTypes) {
				v.InboxRules.Incomplete = true
			}
		}
		for _, s := range snap {
			rules = append(rules, s.Data)
		}
	}
	for _, r := range rules {
		if isFalse(rb(r, "Enabled")) {
			continue
		}
		for _, t := range ruleTargets(r) {
			add(forwardPath{Source: "inbox-rule", Owner: rs(r, "_mailbox"), Rule: rs(r, "Name"), Target: t})
		}
	}

	// Mail flow (transport) rules.
	trs, err := exoGet(ctx, c, exoBeta(tenant, "Get-TransportRule"), nil, "Name,State,Mode,RedirectMessageTo,BlindCopyTo,CopyTo,AddToRecipients")
	if err != nil {
		return v, err
	}
	for _, r := range trs {
		enabled := strings.EqualFold(rs(r, "State"), "Enabled")
		// Audit and AuditAndNotify only log what the rule would do (Enable-TransportRule -Mode).
		mode := rs(r, "Mode")
		enforced := mode == "" || strings.EqualFold(mode, "Enforce")
		for _, k := range []string{"RedirectMessageTo", "BlindCopyTo", "CopyTo", "AddToRecipients"} {
			for _, t := range rl(r, k) {
				p := forwardPath{Source: "transport-rule", Owner: "organization", Rule: rs(r, "Name") + " (" + k + ")", Target: t}
				switch {
				case enabled && enforced:
					p.Verdict, p.Reason = "leaves-tenant", "enabled mail flow rule in Enforce mode; not subject to automatic-forwarding controls"
				case enabled:
					p.Verdict, p.Reason = "rule-not-enforced", "mail flow rule Mode is "+mode+": actions are only logged"
				default:
					p.Verdict, p.Reason = "rule-disabled", "mail flow rule State is "+orDash(rs(r, "State"))
				}
				add(p)
			}
		}
	}
	v.Errors = append(v.Errors, dr.Errors...)
	sort.SliceStable(v.Paths, func(i, j int) bool { return verdictRank(v.Paths[i].Verdict) < verdictRank(v.Paths[j].Verdict) })
	return v, nil
}

// remoteDomainFor returns the remote domain entry that applies to domain (exact,
// then the longest *.suffix wildcard, then the "*" default) and whether it allows
// automatic forwarding. No matching entry means allowed.
// auditMailboxTypes are the mailbox types whose inbox rules forwarding-audit needs.
const auditMailboxTypes = "UserMailbox,SharedMailbox"

// sweepCovers reports whether a stored sweep included every mailbox type in want.
func sweepCovers(meta exoRow, want string) bool {
	have := map[string]bool{}
	for _, t := range rl(meta, "types") {
		have[strings.ToLower(t)] = true
	}
	for _, t := range strings.Split(want, ",") {
		if !have[strings.ToLower(strings.TrimSpace(t))] {
			return false
		}
	}
	return true
}

func remoteDomainFor(remote []exoRow, domain string) (string, bool) {
	best, bestLen := "", -1
	var allowed = true
	for _, r := range remote {
		dn := strings.ToLower(rs(r, "DomainName"))
		match, l := false, 0
		switch {
		case dn == domain:
			match, l = true, 1000
		case dn == "*":
			match, l = true, 0
		case strings.HasPrefix(dn, "*.") && strings.HasSuffix(domain, dn[1:]):
			match, l = true, len(dn)
		}
		if match && l > bestLen {
			best, bestLen, allowed = dn, l, !isFalse(rb(r, "AutoForwardEnabled"))
		}
	}
	if best == "" {
		best = "(none)"
	}
	return best, allowed
}

func verdictRank(v string) int {
	switch v {
	case "leaves-tenant":
		return 0
	case "depends-on-automatic-policy":
		return 1
	case "blocked-by-outbound-policy", "blocked-by-remote-domain":
		return 2
	case "rule-disabled", "rule-not-enforced", "inactive":
		return 3
	}
	return 4
}

func orDash(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}
