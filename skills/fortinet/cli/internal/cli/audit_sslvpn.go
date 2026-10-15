// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// audit sslvpn: SSL-VPN exposure per device and VDOM.
// pp:data-source live

package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/fortios"
)

func newNovelAuditSslvpnCmd(flags *rootFlags) *cobra.Command {
	var fo fleetOpts
	var ao auditOpts
	cmd := &cobra.Command{
		Use:   "sslvpn",
		Args:  rejectExtraArgs,
		Short: "Grade each FortiGate and VDOM on SSL-VPN exposure: enabled, on a WAN interface, open to any source",
		Long: strings.Trim(`
Use this command for SSL-VPN exposure findings per device and VDOM. Do NOT use
this command for the all-checks posture matrix; use 'fleet audit' instead.

Joins cmdb vpn.ssl/settings (status, port, source-interface, source-address,
source-address-negate) with the role of each interface (cmdb system/interface)
and the live session count (monitor vpn/ssl). Each authentication rule is
graded on its own: a source-interface, source-address or negate set on the rule
replaces the global value for that rule, so a rule open to any source on a WAN
interface fails even when the global source list is restricted. Named source
addresses are resolved through cmdb firewall/address and addrgrp (address6 and
addrgrp6 for IPv6), group exclusions included, so a list covering every address
counts as any:
  FAIL  enabled on a WAN-role or SD-WAN interface with source address any
  WARN  enabled on WAN but restricted to named source addresses
  UNKNOWN  enabled on an interface or zone whose WAN exposure the device did
           not establish (no role, role "undefined", or an unreadable zone),
           or on WAN with a source address it could not resolve
  PASS  disabled, internal-only, or no SSL-VPN table on this FortiOS build`, "\n"),
		Example:     "  fortinet-cli audit sslvpn --agent\n  fortinet-cli audit sslvpn --device acme-hq --vdoms root,guest",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit sslvpn")
			}
			if err := validFailOn(ao.failOn); err != nil {
				return err
			}
			return runFindingsAudit(cmd, flags, fo, ao, "sslvpn", func(ctx context.Context, t fleetTarget) ([]fortios.Finding, error) {
				c, err := t.client(flags)
				if err != nil {
					return nil, err
				}
				return collectSSLVPN(ctx, c, t), nil
			})
		},
	}
	addFleetFlags(cmd, &fo)
	addFailOnFlag(cmd, &ao)
	return cmd
}

// runFindingsAudit fans a findings-producing collector over the fleet and
// prints the findings (unreachable devices become UNKNOWN findings).
func runFindingsAudit(cmd *cobra.Command, flags *rootFlags, fo fleetOpts, ao auditOpts, check string, fn func(context.Context, fleetTarget) ([]fortios.Finding, error)) error {
	ctx, cancel := context.WithCancel(cmd.Context()) // --timeout bounds each request, not the fleet: see fanOut
	defer cancel()
	targets, failures, err := loadFleet(flags, fo)
	if err != nil {
		return err
	}
	perDevice, fails := fanOut(ctx, targets, fo.concurrency, fn)
	failures = append(failures, fails...)
	all := make([]fortios.Finding, 0)
	for _, fs := range perDevice {
		all = append(all, fs...)
	}
	for _, f := range failures {
		all = append(all, fortios.Finding{Device: f.ID, Check: check, Status: fortios.Unknown, Code: "unreachable", Detail: f.Error})
	}
	warnUnchecked(cmd, all, len(targets)+len(failures)-len(fails))
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		if err := printJSONFilteredKeep(cmd.OutOrStdout(), all, flags, "vdom", "object", "code", "check"); err != nil {
			return err
		}
		return failOnErr(ao.failOn, all)
	}
	printFindingsTable(cmd, all)
	return failOnErr(ao.failOn, all)
}
