// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/posture"
)

// mfaRow is one account (or the admin policy) on one firewall.
type mfaRow struct {
	Serial       string `json:"serial"`
	Name         string `json:"firewall"`
	Account      string `json:"account"`
	Kind         string `json:"kind"`          // admin-policy, local-user, directory, sign-in or firewall
	SSLVPNMember *bool  `json:"sslvpn_member"` // null when membership was not reported
	Method       string `json:"mfa_method"`    // totp, otp-email, delegated, none, unknown
	Source       string `json:"mfa_source,omitempty"`
	Status       string `json:"status"` // pass, warn, fail, unknown
	Error        string `json:"error,omitempty"`
}

// auditDeviceMFA returns the admin-policy row, one sign-in row when SAML or a
// directory signs users in (or the sign-in source was not reported), and one
// row per local user. A user's verdict is unknown, never fail, when the SSL-VPN
// auth mode (delegated or not) or the group list could not be read.
func auditDeviceMFA(read func(path string) ([]byte, error), d fleetDevice) []mfaRow {
	base := mfaRow{Serial: d.SerialNumber, Name: d.FriendlyName}
	rows := make([]mfaRow, 0)

	admin := base
	admin.Account, admin.Kind = "administrators (TOTP policy)", "admin-policy"
	if raw, err := read("/firewall/administration/global"); err != nil {
		admin.Method, admin.Status, admin.Error = "unknown", posture.Unknown, errText(err)
	} else if m, ok := posture.AdminOTP(raw); !ok {
		admin.Method, admin.Status, admin.Error = "unknown", posture.Unknown, "unexpected response shape"
	} else if m == "totp" {
		admin.Method, admin.Status = m, posture.Pass
	} else {
		admin.Method, admin.Status = m, posture.Fail // e-mail OTP or none: the policy asks for TOTP
	}
	rows = append(rows, admin)

	// Sign-in source: SAML on the SSL-VPN server, else the firewall's user
	// authentication method (local accounts, RADIUS, LDAP or TACACS+).
	authKnown, delegated, authType := false, false, ""
	authErrText := ""
	var external []string
	if raw, err := read("/firewall/ssl-vpn/server/base"); err != nil {
		authErrText = "ssl-vpn auth mode unreadable: " + errText(err)
	} else if t, saml, ok := posture.SSLVPNAuth(raw); !ok {
		authErrText = "ssl-vpn auth mode unreadable: auth_type not reported or not a documented value"
	} else if saml {
		authKnown, delegated, authType = true, true, t
	} else if mraw, err := read("/firewall/user/authentication/methods"); err != nil {
		authErrText = "user authentication methods unreadable: " + errText(err)
	} else if methods, ok := posture.UserAuthMethods(mraw); !ok {
		authErrText = "user authentication methods unreadable: auth_method not reported or not a documented value"
	} else {
		var local bool
		external, local = posture.SignInSources(methods)
		authKnown, delegated, authType = true, !local, t
		if len(external) > 0 {
			authType += "+" + strings.Join(external, "+")
		}
	}
	// One sign-in row per firewall whenever local accounts are not the whole
	// population: the sign-in source was not reported, or SAML or a directory
	// signs users in. Those accounts are never read, so they never pass.
	switch {
	case !authKnown:
		sr := base
		sr.Account, sr.Kind, sr.Method, sr.Status, sr.Error = "SSL-VPN sign-in source", "sign-in", "unknown", posture.Unknown, authErrText
		rows = append(rows, sr)
	case delegated || len(external) > 0:
		label := "saml"
		if len(external) > 0 {
			label = strings.Join(external, ", ")
		}
		dir := base
		dir.Account, dir.Kind, dir.Method, dir.Source, dir.Status = "directory users ("+label+")", "directory", "delegated", "ssl-vpn "+authType, posture.Warn
		dir.Error = "these accounts are not local; their MFA is enforced (or not) by the directory or identity provider"
		rows = append(rows, dir)
	}
	usersRaw, err := read("/firewall/user/local/users")
	if err != nil {
		u := base
		u.Account, u.Kind, u.Method, u.Status, u.Error = "local users", "local-user", "unknown", posture.Unknown, errText(err)
		return append(rows, u)
	}
	groupsRaw, gerr := read("/firewall/user/local/groups")
	if gerr != nil {
		groupsRaw = nil
	}
	accounts, ok := posture.LocalAccounts(usersRaw, groupsRaw)
	if !ok {
		u := base
		u.Account, u.Kind, u.Method, u.Status, u.Error = "local users", "local-user", "unknown", posture.Unknown, "unexpected response shape"
		return append(rows, u)
	}
	for _, a := range accounts {
		r := base
		r.Account, r.Kind, r.SSLVPNMember, r.Method, r.Source = a.Name, "local-user", a.SSLVPNMember, a.Method, a.MethodSource
		switch {
		case a.Method == "unknown" && gerr != nil:
			r.Status, r.Error = posture.Unknown, "group list unreadable: "+errText(gerr)
		case a.Method == "unknown":
			r.Status, r.Error = posture.Unknown, "MFA coverage not reported (the user's or a group's one-time password setting)"
		case a.Method != "none":
			r.Status = posture.Pass
		case a.SSLVPNMember == nil:
			r.Status, r.Error = posture.Unknown, "SSL-VPN membership not reported"
		case !*a.SSLVPNMember:
			r.Status = posture.Warn // no OTP, but not an SSL-VPN user
		case !authKnown:
			r.Status, r.Error = posture.Unknown, authErrText
		case delegated:
			r.Method, r.Source, r.Status = "delegated", "ssl-vpn "+authType, posture.Warn
		default:
			r.Status = posture.Fail
		}
		rows = append(rows, r)
	}
	return rows
}

func newNovelMfaAuditCmd(flags *rootFlags) *cobra.Command {
	var tenant, serial string
	var all bool
	cmd := &cobra.Command{
		Use:   "mfa-audit",
		Short: "List accounts missing MFA: admin TOTP per firewall and local SSL-VPN users without a one-time password (SAML, RADIUS, LDAP or TACACS+ sign-in marked delegated); --all lists passing accounts too",
		Long: "Use this command for the per-account MFA remediation list: admin TOTP per firewall and every SSL-VPN-capable\n" +
			"local user without a one-time password. Do NOT use this command for the one-row-per-firewall posture\n" +
			"summary; use 'posture' instead.\n\n" +
			"A local user counts as covered by its own one-time password (TOTP or e-mail OTP) or by a group it\n" +
			"belongs to directly. SSL-VPN users are members of the built-in 'SSLVPN Services' group. When SSL VPN\n" +
			"authenticates through SAML, or the firewall's user authentication method is RADIUS, LDAP or TACACS+,\n" +
			"users are marked 'delegated' (verify MFA at that provider) and the firewall gets one 'directory users'\n" +
			"row (warn); with an '-local' method local accounts are still graded. A firewall whose sign-in source\n" +
			"was not reported gets one 'SSL-VPN sign-in source' row (unknown).\n" +
			"By default only accounts that are not a clean pass are listed; --all lists everyone.",
		Example: strings.Trim(`
  sonicwall-cli mfa-audit --agent
  sonicwall-cli mfa-audit --serial 2CB8ED000001 --all --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "mfa-audit")
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			devices, err := fetchFleet(ctx, c, tenant, serial)
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), err, flags)
			}
			devices, _ = capForDogfood(devices)
			if len(devices) == 0 {
				return noFirewallsErr(cmd.OutOrStdout(), flags)
			}
			results, fails := cliutil.FanoutRun(ctx, devices,
				func(d fleetDevice) string { return d.SerialNumber },
				func(ctx context.Context, d fleetDevice) ([]mfaRow, error) {
					read := func(path string) ([]byte, error) { return sonicOSGet(ctx, c, d.SerialNumber, path) }
					return auditDeviceMFA(read, d), nil
				})
			bySerial := map[string][]mfaRow{}
			for _, res := range results {
				bySerial[res.Source] = res.Value
			}
			failErr := map[string]error{}
			for _, f := range fails {
				failErr[f.Source] = f.Err
			}
			rows := make([]mfaRow, 0)
			unknown := 0
			for _, d := range devices {
				devRows, done := bySerial[d.SerialNumber]
				if !done {
					devRows = []mfaRow{{Serial: d.SerialNumber, Name: d.FriendlyName, Account: "all accounts", Kind: "firewall",
						Method: "unknown", Status: posture.Unknown, Error: "not checked: " + errText(failErr[d.SerialNumber])}}
				}
				for _, r := range devRows {
					if r.Status == posture.Unknown {
						unknown++
					}
					if all || r.Status != posture.Pass {
						rows = append(rows, r)
					}
				}
			}
			partial := interrupted(ctx)
			if partial == nil && len(fails) > 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+fanoutFailureNote(fails, len(devices)))
				partial = partialFailureErr(fmt.Errorf("%s", fanoutFailureNote(fails, len(devices))))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), rows, flags); err != nil {
					return err
				}
				return partial
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No MFA gaps found across %d firewall(s); every account was read.\n", len(devices))
				return partial
			}
			table := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				table = append(table, map[string]any{"serial": r.Serial, "firewall": r.Name, "account": r.Account, "sslvpn": triState(r.SSLVPNMember), "mfa": r.Method, "status": r.Status, "note": r.Error})
			}
			if err := printOrderedTable(cmd.OutOrStdout(), []string{"serial", "firewall", "account", "sslvpn", "mfa", "status", "note"}, table); err != nil {
				return err
			}
			if unknown > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d account checks could not be decided (status unknown); see the note column (error field in JSON)\n", unknown)
			}
			return partial
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "", "Only firewalls in this NSM tenant ID")
	cmd.Flags().StringVar(&serial, "serial", "", "Only this firewall serial number")
	cmd.Flags().BoolVar(&all, "all", false, "List every account, including ones that pass")
	return cmd
}

// triState renders a nullable yes/no for the human table.
func triState(b *bool) string {
	switch {
	case b == nil:
		return "unknown"
	case *b:
		return "yes"
	}
	return "no"
}
