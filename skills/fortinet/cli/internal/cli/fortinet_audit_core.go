// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Per-device collectors for the four posture checks. Each returns findings
// for one device; a read failure becomes an UNKNOWN finding, never a PASS.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/client"
	"fortinet-pp-cli/internal/cliutil"
	"fortinet-pp-cli/internal/fortios"
)

// rejectExtraArgs refuses positionals on commands that take none. The usual
// cause is a space-separated boolean such as "--insecure false", which Cobra
// parses as --insecure=true plus a stray "false".
func rejectExtraArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	hint := ""
	if args[0] == "true" || args[0] == "false" {
		hint = " (write booleans as --flag=false)"
	}
	return usageErr(fmt.Errorf("unexpected argument %q for %q%s", args[0], cmd.CommandPath(), hint))
}

// shortErr names the HTTP status when there is one, else the first line of err.
func shortErr(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("HTTP %d", apiErr.StatusCode)
	}
	msg, _, _ := strings.Cut(err.Error(), "\n")
	return msg
}

func unknownFinding(device, vdom, check string, err error) fortios.Finding {
	code := "read-failed"
	switch {
	case isHTTPStatus(err, 401):
		code = "unauthorized"
	case isHTTPStatus(err, 403):
		code = "forbidden"
	}
	return fortios.Finding{Device: device, VDOM: vdom, Check: check, Status: fortios.Unknown, Code: code, Detail: err.Error()}
}

type firmwareResult struct {
	Report fortios.FirmwareReport
	Serial string
	Model  string
}

func collectFirmware(ctx context.Context, c *client.Client, t fleetTarget, floors map[string]fortios.Version) firmwareResult {
	status, so, err := fortiGetObj(ctx, c, "/monitor/system/status", nil)
	if err != nil {
		return firmwareResult{Report: fortios.FirmwareReport{Device: t.Name, Status: fortios.Unknown, Findings: []fortios.Finding{unknownFinding(t.Name, "", "firmware", err)}}}
	}
	// The firmware list is optional evidence: without it a floor can still grade.
	_, results, err := fortiGetObj(ctx, c, "/monitor/system/firmware", nil)
	if err != nil {
		results = fortios.Obj{}
	}
	r := fortios.Firmware(t.Name, results, status.Version, floors)
	return firmwareResult{Report: r, Serial: status.Serial, Model: fortios.Str(so, "model")}
}

func collectSSLVPN(ctx context.Context, c *client.Client, t fleetTarget) []fortios.Finding {
	_, ifaces, err := fortiGetList(ctx, c, "/cmdb/system/interface", map[string]string{"format": "name|role|allowaccess"})
	if err != nil {
		return []fortios.Finding{unknownFinding(t.Name, "", "sslvpn", err)}
	}
	out := []fortios.Finding{}
	vdoms, vErr := t.vdoms(ctx, c)
	if vErr != nil {
		f := unknownFinding(t.Name, "", "sslvpn", vErr)
		f.Code = "vdoms-unlisted"
		out = append(out, f)
	}
	for _, vdom := range vdoms {
		// Zones (cmdb system/zone, Ansible fortios_configuration_fact selector
		// system_zone) are per-VDOM and resolve zone names used as listening
		// interfaces; an unresolved zone grades the VDOM UNKNOWN, never PASS.
		_, zones, zErr := fortiGetList(ctx, c, "/cmdb/system/zone", vdomParams(vdom))
		_, settings, err := fortiGetObj(ctx, c, "/cmdb/vpn.ssl/settings", vdomParams(vdom))
		present := true
		switch {
		case err != nil && isHTTPStatus(err, 404) && zErr != nil:
			// A 404 is "no SSL-VPN on this build" only when the VDOM itself
			// answers; a mistyped --vdoms value must not grade PASS.
			out = append(out, unknownFinding(t.Name, vdom, "sslvpn", fmt.Errorf("vpn.ssl/settings answered 404 and VDOM %q could not be confirmed (system/zone: %v)", vdom, zErr)))
			continue
		case err != nil && isHTTPStatus(err, 404):
			present = false
		case err != nil:
			out = append(out, unknownFinding(t.Name, vdom, "sslvpn", err))
			continue
		case fortios.Str(settings, "status") == "":
			out = append(out, unknownFinding(t.Name, vdom, "sslvpn", fmt.Errorf("vpn.ssl/settings has no status field")))
			continue
		}
		sessions := 0
		if present {
			if _, list, err := fortiGetList(ctx, c, "/monitor/vpn/ssl", vdomParams(vdom)); err == nil {
				sessions = len(list)
			}
		}
		var book4, book6 *fortios.AddressBook
		var bookErr error
		if present {
			book4, book6, bookErr = sslvpnSourceBooks(ctx, c, vdom, settings)
		}
		for _, f := range fortios.SSLVPN(t.Name, vdom, settings, present, ifaces, zones, book4, book6, sessions) {
			if f.Code == "source-unresolved" && bookErr != nil {
				f.Detail += " (" + bookErr.Error() + "; the API admin profile needs Firewall read access)"
			}
			out = append(out, f)
		}
	}
	return out
}

func collectAdmins(ctx context.Context, c *client.Client, t fleetTarget, broadPrefix int) []fortios.Finding {
	read := func(path string) ([]fortios.Obj, error) {
		_, rows, err := fortiGetList(ctx, c, path, nil)
		return rows, err
	}
	admins, err := read("/cmdb/system/admin")
	if err == nil && len(admins) == 0 {
		err = fmt.Errorf("system/admin returned no administrators (a FortiGate always has at least one)")
	}
	if err != nil {
		return []fortios.Finding{unknownFinding(t.Name, "", "admins", err)}
	}
	ifaces, err := read("/cmdb/system/interface")
	if err != nil {
		return []fortios.Finding{unknownFinding(t.Name, "", "admins", err)}
	}
	var extra []fortios.Finding
	// Every FortiOS release has these tables, so any failure (404 included)
	// means the checks were skipped and is reported as UNKNOWN.
	optional := func(path, code string) []fortios.Obj {
		rows, err := read(path)
		if err != nil {
			f := unknownFinding(t.Name, "", "admins", err)
			f.Code, f.Detail = code, path+" unreadable ("+shortErr(err)+"); its checks were skipped"
			extra = append(extra, f)
		}
		return rows
	}
	profiles := optional("/cmdb/system/accprofile", "profiles-unreadable")
	apiUsers := optional("/cmdb/system/api-user", "api-users-unreadable")
	out := fortios.Admins(t.Name, admins, profiles, apiUsers, ifaces, broadPrefix)
	if len(extra) > 0 && fortios.Status(out) == fortios.Pass {
		return extra
	}
	return append(out, extra...)
}

func collectLicenses(ctx context.Context, c *client.Client, t fleetTarget, within time.Duration) ([]fortios.LicenseRow, []fortios.Finding) {
	status, so, err := fortiGetObj(ctx, c, "/monitor/system/status", nil)
	if err != nil {
		return nil, []fortios.Finding{unknownFinding(t.Name, "", "licenses", err)}
	}
	_, lic, err := fortiGetObj(ctx, c, "/monitor/license/status", nil)
	if err != nil {
		return nil, []fortios.Finding{unknownFinding(t.Name, "", "licenses", err)}
	}
	return fortios.Licenses(t.Name, status.Serial, fortios.Str(so, "model"), lic, time.Now(), within)
}

// auditOpts are the grading knobs shared by fleet audit and the per-check audits.
type auditOpts struct {
	minVersion  string
	within      string
	broadPrefix int
	failOn      string
}

func addFailOnFlag(cmd *cobra.Command, o *auditOpts) {
	cmd.Flags().StringVar(&o.failOn, "fail-on", "none", "Exit 6 when a finding at this level, or any UNKNOWN (not checked), exists: fail, warn or none (for cron/CI)")
}

func (o auditOpts) floors() (map[string]fortios.Version, error) {
	f, err := fortios.ParseFloors(o.minVersion)
	if err != nil {
		return nil, usageErr(err)
	}
	return f, nil
}

func (o auditOpts) withinDuration() (time.Duration, error) {
	s := o.within
	if s == "" {
		s = "60d"
	}
	d, err := cliutil.ParseDurationLoose(s)
	if err != nil {
		return 0, usageErr(fmt.Errorf("--within: %w", err))
	}
	return d, nil
}

// failOnErr returns exit code 6 when findings reach the --fail-on level.
func failOnErr(level string, findings []fortios.Finding) error {
	threshold := map[string]string{"fail": fortios.Fail, "warn": fortios.Warn}[strings.ToLower(level)]
	if threshold == "" {
		return nil
	}
	// UNKNOWN (not checked) counts too: an unreachable fleet must not pass CI.
	n := 0
	for _, f := range findings {
		if f.Status == fortios.Unknown || fortios.Worst(f.Status, threshold) == f.Status {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	return &cliError{code: 6, err: fmt.Errorf("%d finding(s) at or above %s", n, strings.ToUpper(level))}
}

func validFailOn(level string) error {
	switch strings.ToLower(level) {
	case "", "none", "fail", "warn":
		return nil
	}
	return usageErr(fmt.Errorf("--fail-on must be fail, warn or none (got %q)", level))
}

// warnUnchecked reports how many devices have at least one UNKNOWN finding.
func warnUnchecked(cmd *cobra.Command, findings []fortios.Finding, total int) {
	seen := map[string]bool{}
	for _, f := range findings {
		if f.Status == fortios.Unknown {
			seen[f.Device] = true
		}
	}
	if len(seen) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d device(s) not fully checked (UNKNOWN findings); see the findings for the reason\n", len(seen), total)
	}
}

func printFindingsTable(cmd *cobra.Command, findings []fortios.Finding) {
	if len(findings) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No findings.")
		return
	}
	for _, f := range findings {
		where := f.Device
		if f.VDOM != "" {
			where += "/" + f.VDOM
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-7s %-22s %-10s %s\n", f.Status, where, f.Check, f.Detail)
	}
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if audit, _, err := root.Find([]string{"audit"}); err == nil && audit != root {
			audit.Short = "Per-check security audits (firmware, SSL-VPN, admins, licences) with the evidence behind each finding"
		}
	})
}

// sslvpnSourceBooks reads the address objects and groups SSL-VPN source lists
// (global and per authentication rule) can name (Ansible fortios_configuration_fact selectors firewall_address,
// firewall_addrgrp, firewall_address6, firewall_addrgrp6) for one VDOM. A
// family whose list is empty or only "all" is not read. A failed read leaves
// that book nil, so named sources stay unresolved and grade UNKNOWN; the
// first read error is returned so the finding can say why.
func sslvpnSourceBooks(ctx context.Context, c *client.Client, vdom string, settings fortios.Obj) (book4, book6 *fortios.AddressBook, firstErr error) {
	read := func(names []string, addrPath, grpPath string) *fortios.AddressBook {
		needed := false
		for _, n := range names {
			needed = needed || !strings.EqualFold(n, "all")
		}
		if !needed {
			return nil
		}
		fail := func(path string, err error) *fortios.AddressBook {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %v", strings.TrimPrefix(path, "/cmdb/"), err)
			}
			return nil
		}
		_, addrs, err := fortiGetList(ctx, c, addrPath, vdomParams(vdom))
		if err != nil {
			return fail(addrPath, err)
		}
		_, groups, err := fortiGetList(ctx, c, grpPath, vdomParams(vdom))
		if err != nil {
			return fail(grpPath, err)
		}
		return fortios.NewAddressBook(addrs, groups)
	}
	// Authentication rules carry their own source lists, so their names count too.
	names4, names6 := fortios.SSLVPNSourceNames(settings)
	book4 = read(names4, "/cmdb/firewall/address", "/cmdb/firewall/addrgrp")
	book6 = read(names6, "/cmdb/firewall/address6", "/cmdb/firewall/addrgrp6")
	return book4, book6, firstErr
}
