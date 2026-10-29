// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Effective-state rules, from Microsoft Learn:
//   - EWS: a mailbox's EwsEnabled "is meaningful only if the EwsEnabled parameter on
//     Set-OrganizationConfig isn't set to $false"; the mailbox default is $true.
//     https://learn.microsoft.com/en-us/powershell/module/exchangepowershell/set-casmailbox
//   - SMTP AUTH: the mailbox SmtpClientAuthenticationDisabled overrides the org
//     setting (Get-TransportConfig); $null means "use the organization setting".
//     https://learn.microsoft.com/en-us/exchange/clients-and-mobile-in-exchange-online/authenticated-client-smtp-submission
//   - IMAP, POP, ActiveSync: per-mailbox switches, default on.

var postureProtocols = []string{"ews", "imap", "pop", "activesync", "smtpauth"}

type mailboxPosture struct {
	Mailbox  string          `json:"mailbox"`
	Name     string          `json:"name,omitempty"`
	Enabled  map[string]bool `json:"enabled"`
	SmtpAuth string          `json:"smtp_auth_source"`
}

type ewsPostureView struct {
	Tenant string `json:"tenant"`
	Org    struct {
		EwsEnabled                 *bool  `json:"ews_enabled"`
		EwsApplicationAccessPolicy string `json:"ews_application_access_policy,omitempty"`
		SmtpAuthDisabled           *bool  `json:"smtp_auth_disabled"`
	} `json:"org"`
	Mailboxes int              `json:"mailboxes"`
	Counts    map[string]int   `json:"enabled_counts"`
	Note      string           `json:"note,omitempty"`
	List      []mailboxPosture `json:"list,omitempty"`
}

func newNovelEwsPostureCmd(flags *rootFlags) *cobra.Command {
	var stillEnabled string
	var all bool
	cmd := &cobra.Command{
		Use:   "ews-posture <tenantId>",
		Short: "See which mailboxes still have EWS, IMAP, POP, ActiveSync or SMTP AUTH effectively on once org defaults are applied.",
		Long: `Reads Get-OrganizationConfig (EwsEnabled, EwsApplicationAccessPolicy), Get-TransportConfig
(SmtpClientAuthenticationDisabled) and every Get-CASMailbox row, then resolves each mailbox's
effective state: org EwsEnabled $false switches EWS off everywhere, and a mailbox's SMTP AUTH
value of null inherits the org setting. Prints per-protocol counts; --still-enabled lists the
mailboxes where one protocol is still effectively on, --all lists every mailbox.

Use this command for each mailbox's EFFECTIVE EWS, IMAP, POP, ActiveSync and SMTP AUTH state after
org defaults are applied. Do NOT use this command to switch a protocol off; use 'client-access set'
instead. For raw per-mailbox CAS values without inheritance, use 'client-access list'.`,
		Example:     "  exchange-online-cli ews-posture 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent\n  exchange-online-cli ews-posture 0550b473-9fd2-4dbb-a058-a1640b4bf458 --still-enabled smtpauth --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "tenantId=0550b473-9fd2-4dbb-a058-a1640b4bf458"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "ews-posture")
			}
			if stillEnabled != "" && !containsStr(postureProtocols, strings.ToLower(stillEnabled)) {
				return usageErr(fmt.Errorf("--still-enabled must be one of %s", strings.Join(postureProtocols, ", ")))
			}
			tenant, err := exoTenantArg(cmd, args, 0)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			org, err := exoGet(ctx, c, exoBeta(tenant, "Get-OrganizationConfig"), nil, "EwsEnabled,EwsApplicationAccessPolicy")
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			tc, err := exoGet(ctx, c, exoBeta(tenant, "Get-TransportConfig"), nil, "SmtpClientAuthenticationDisabled")
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			cas, err := exoGet(ctx, c, exoBeta(tenant, "Get-CASMailbox"), exoRow{"ResultSize": "Unlimited"},
				"Name,PrimarySmtpAddress,EwsEnabled,ImapEnabled,PopEnabled,ActiveSyncEnabled,SmtpClientAuthenticationDisabled")
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			v := ewsPostureView{Tenant: tenant, Counts: map[string]int{}}
			if len(org) > 0 {
				v.Org.EwsEnabled = rb(org[0], "EwsEnabled")
				v.Org.EwsApplicationAccessPolicy = rs(org[0], "EwsApplicationAccessPolicy")
			}
			if len(tc) > 0 {
				v.Org.SmtpAuthDisabled = rb(tc[0], "SmtpClientAuthenticationDisabled")
			}
			for _, p := range postureProtocols {
				v.Counts[p] = 0
			}
			want := strings.ToLower(stillEnabled)
			for _, r := range cas {
				mp := resolvePosture(r, v.Org.EwsEnabled, v.Org.SmtpAuthDisabled)
				v.Mailboxes++
				for p, on := range mp.Enabled {
					if on {
						v.Counts[p]++
					}
				}
				if all || (want != "" && mp.Enabled[want]) {
					v.List = append(v.List, mp)
				}
			}
			sort.Slice(v.List, func(i, j int) bool { return v.List[i].Mailbox < v.List[j].Mailbox })
			if v.Org.EwsApplicationAccessPolicy != "" {
				v.Note = "EwsApplicationAccessPolicy further limits which EWS clients connect; counts show mailboxes where EWS is not switched off."
			}
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}
	cmd.Flags().StringVar(&stillEnabled, "still-enabled", "", "List mailboxes where this protocol is effectively on: ews, imap, pop, activesync, smtpauth")
	cmd.Flags().BoolVar(&all, "all", false, "List every mailbox's effective state")
	return cmd
}

func resolvePosture(r exoRow, orgEws, orgSmtpDisabled *bool) mailboxPosture {
	mp := mailboxPosture{Mailbox: smtpOf(rs(r, "PrimarySmtpAddress")), Name: rs(r, "Name"), Enabled: map[string]bool{}}
	mp.Enabled["ews"] = !isFalse(orgEws) && !isFalse(rb(r, "EwsEnabled"))
	mp.Enabled["imap"] = !isFalse(rb(r, "ImapEnabled"))
	mp.Enabled["pop"] = !isFalse(rb(r, "PopEnabled"))
	mp.Enabled["activesync"] = !isFalse(rb(r, "ActiveSyncEnabled"))
	if mbx := rb(r, "SmtpClientAuthenticationDisabled"); mbx != nil {
		mp.Enabled["smtpauth"] = !*mbx
		mp.SmtpAuth = "mailbox"
	} else {
		mp.Enabled["smtpauth"] = !isTrue(orgSmtpDisabled)
		mp.SmtpAuth = "organization"
	}
	return mp
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
