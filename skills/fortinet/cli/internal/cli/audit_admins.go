// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// audit admins: administrator and API-user exposure.
// pp:data-source live

package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/fortios"
)

func newNovelAuditAdminsCmd(flags *rootFlags) *cobra.Command {
	var fo fleetOpts
	var ao auditOpts
	cmd := &cobra.Command{
		Use:   "admins",
		Args:  rejectExtraArgs,
		Short: "Find admins without trusted hosts or two-factor, super_admin exposure, and API users with broad trusted hosts",
		Long: strings.Trim(`
Use this command for admin and API-user findings: no trusted hosts, no 2FA,
super_admin profiles, broad API-user trusthosts. Do NOT use this command for
the all-checks posture matrix; use 'fleet audit' instead.

Reads cmdb system/admin, system/accprofile, system/api-user and system/interface:
  FAIL  admin with no trusted hosts while https/ssh is allowed on a WAN-role
        interface (IPv4 and IPv6 judged separately: allowaccess vs ip6-allowaccess;
        a secondary IP's own allowaccess counts for its interface)
  FAIL  super_admin without two-factor; WAN interface allowing http or telnet
  WARN  admin with no trusted hosts (management not on WAN), admin without 2FA,
        API user trusted from a network broader than --broad-prefix, API user on super_admin
  UNKNOWN  admin with no trusted hosts (per family) while management is enabled
        on an interface with no role or role "undefined" (WAN exposure not
        established); http or telnet on such an interface
Admins authenticating against RADIUS/LDAP/TACACS+ (remote-auth) are not flagged
for missing local 2FA. The API token needs read access to System settings.`, "\n"),
		Example:     "  fortinet-cli audit admins --broad-prefix 16 --agent\n  fortinet-cli audit admins --device acme-hq",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit admins")
			}
			if err := validFailOn(ao.failOn); err != nil {
				return err
			}
			if ao.broadPrefix < 0 || ao.broadPrefix > 32 {
				return usageErr(errBroadPrefix)
			}
			return runFindingsAudit(cmd, flags, fo, ao, "admins", func(ctx context.Context, t fleetTarget) ([]fortios.Finding, error) {
				c, err := t.client(flags)
				if err != nil {
					return nil, err
				}
				return collectAdmins(ctx, c, t, ao.broadPrefix), nil
			})
		},
	}
	addFleetFlags(cmd, &fo)
	cmd.Flags().IntVar(&ao.broadPrefix, "broad-prefix", 16, "Flag API-user trusted hosts with a prefix shorter than this (0-32)")
	addFailOnFlag(cmd, &ao)
	return cmd
}
