// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// fleet health: operational red/amber/green row per device.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/cliutil"
	"fortinet-pp-cli/internal/fortios"
)

type fleetHealthView struct {
	Devices       []fortios.HealthRow `json:"devices"`
	FetchFailures []fetchFailure      `json:"fetch_failures,omitempty"`
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var fo fleetOpts
	var certWithin string
	var cpuWarn, memWarn float64
	cmd := &cobra.Command{
		Use:   "health",
		Args:  rejectExtraArgs,
		Short: "One PASS/WARN/FAIL/UNKNOWN row per FortiGate: HA sync, IPsec tunnels down, SD-WAN members down, CPU/memory, expiring certificates",
		Long: strings.Trim(`
Use this command for operational state across the fleet: reachability, HA sync,
IPsec tunnels, SD-WAN SLA, resource pressure and certificate expiry. Do NOT use
this command for security posture such as firmware, SSL-VPN exposure, admin
accounts or licences; use 'fleet audit' instead.

Per device it reads monitor system/status, system/ha-peer, system/ha-checksums,
vpn/ipsec and virtual-wan/health-check (per VDOM), system/resource/usage and
system/available-certificates. HA sync compares each member's checksum object;
it is "unknown" when the device returns none to compare. SD-WAN counts members
whose status is not "up" (SLA targets are not evaluated). A device that cannot
be read is UNKNOWN, and so is a device where any of these reads fails (listed
under "unreadable"; the API token needs read access to monitor data). Signals
answered in a shape this CLI does not recognize are listed under
"not_evaluated" and do not affect the status: the PASS covers only the
signals that were evaluated.`, "\n"),
		Example:     "  fortinet-cli fleet health --agent\n  fortinet-cli fleet health --cert-within 14d --cpu-warn 80",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			within, err := cliutil.ParseDurationLoose(certWithin)
			if err != nil {
				return usageErr(fmt.Errorf("--cert-within: %w", err))
			}
			ctx, cancel := context.WithCancel(cmd.Context()) // --timeout bounds each request, not the fleet: see fanOut
			defer cancel()
			targets, failures, err := loadFleet(flags, fo)
			if err != nil {
				return err
			}
			rows, fails := fanOut(ctx, targets, fo.concurrency, func(ctx context.Context, t fleetTarget) (fortios.HealthRow, error) {
				c, err := t.client(flags)
				if err != nil {
					return fortios.HealthRow{}, err
				}
				status, so, err := fortiGetObj(ctx, c, "/monitor/system/status", nil)
				if err != nil {
					return fortios.HealthRow{}, err
				}
				h := fortios.HealthRow{Device: t.Name, Hostname: fortios.Str(so, "hostname"), Model: fortios.Str(so, "model"), Serial: status.Serial, Version: status.Version, HASync: "unknown"}
				// unreadable records a failed read; every one (404 included) leaves
				// that signal unchecked, so the row cannot be PASS.
				unreadable := func(what string, err error) bool {
					if err == nil {
						return false
					}
					h.Unreadable = append(h.Unreadable, fmt.Sprintf("%s (%s)", what, shortErr(err)))
					return true
				}
				if _, peers, err := fortiGetList(ctx, c, "/monitor/system/ha-peer", nil); !unreadable("ha-peer", err) {
					h.HAMembers = len(peers)
				}
				if _, sums, err := fortiGetList(ctx, c, "/monitor/system/ha-checksums", nil); !unreadable("ha-checksums", err) {
					h.HASync = fortios.HAChecksumsInSync(sums)
					if h.HASync == "unknown" {
						h.NotEvaluated = append(h.NotEvaluated, "ha-sync (members carry no checksum object)")
					}
				}
				vdoms, vErr := t.vdoms(ctx, c)
				unreadable("vdom list", vErr)
				for _, vdom := range vdoms {
					if _, tun, err := fortiGetList(ctx, c, "/monitor/vpn/ipsec", vdomParams(vdom)); !unreadable("ipsec "+vdom, err) {
						d, n := fortios.IPsecDown(tun)
						h.TunnelsDown += d
						h.TunnelsTotal += n
					}
					if sd, err := fortiGet(ctx, c, "/monitor/virtual-wan/health-check", vdomParams(vdom)); !unreadable("sdwan "+vdom, err) {
						d, n := fortios.SDWANDown(sd.any())
						h.SDWANDown += d
						h.SDWANChecked += n
						if n == 0 && sd.any() != nil && string(sd.Results) != "{}" && string(sd.Results) != "[]" {
							h.NotEvaluated = append(h.NotEvaluated, "sdwan "+vdom+" (no member status fields)")
						}
					}
				}
				if _, res, err := fortiGetObj(ctx, c, "/monitor/system/resource/usage", nil); !unreadable("resource-usage", err) {
					if v, ok := fortios.CurrentUsage(res, "cpu"); ok {
						h.CPUPercent = &v
					}
					if v, ok := fortios.CurrentUsage(res, "mem"); ok {
						h.MemPercent = &v
					}
					if h.CPUPercent == nil || h.MemPercent == nil {
						h.NotEvaluated = append(h.NotEvaluated, "cpu/memory (unrecognized resource/usage shape)")
					}
				}
				h.CertsExpiring = []string{}
				if _, certs, err := fortiGetList(ctx, c, "/monitor/system/available-certificates", nil); !unreadable("certificates", err) {
					h.CertsExpiring = fortios.ExpiringCerts(certs, time.Now(), within)
					if undated := fortios.UndatedCerts(certs); len(undated) > 0 {
						h.NotEvaluated = append(h.NotEvaluated, "certificates "+strings.Join(undated, ",")+" (no recognizable valid_to)")
					}
				}
				h.Grade(cpuWarn, memWarn)
				return h, nil
			})
			failures = append(failures, fails...)
			for _, f := range failures {
				rows = append(rows, fortios.HealthRow{Device: f.ID, HASync: "unknown", CertsExpiring: []string{}, Signals: []string{"unreachable: " + f.Error}, Status: fortios.Unknown})
			}
			warnFailures(cmd, failures, len(targets)+len(failures)-len(fails))
			view := fleetHealthView{Devices: rows, FetchFailures: failures}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "unreadable", "not_evaluated", "signals")
			}
			for _, r := range rows {
				sig := "ok"
				if len(r.Signals) > 0 {
					sig = strings.Join(r.Signals, "; ")
				}
				// A PASS covers only the evaluated signals; say which were
				// skipped so "ok" never reads as full coverage.
				if len(r.NotEvaluated) > 0 {
					sig += "; not evaluated: " + strings.Join(r.NotEvaluated, ", ")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-7s %-24s %-10s %-9s HA:%-11s %s\n", r.Status, r.Device, r.Model, r.Version, r.HASync, sig)
			}
			return nil
		},
	}
	addFleetFlags(cmd, &fo)
	cmd.Flags().StringVar(&certWithin, "cert-within", "30d", "Flag certificates expiring inside this window")
	cmd.Flags().Float64Var(&cpuWarn, "cpu-warn", 90, "CPU percent that turns a device amber")
	cmd.Flags().Float64Var(&memWarn, "mem-warn", 90, "Memory percent that turns a device amber")
	return cmd
}
