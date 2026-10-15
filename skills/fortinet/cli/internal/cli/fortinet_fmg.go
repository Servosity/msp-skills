// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// fmg: read-only FortiManager view (one ADOM per MSP client).
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"fortinet-pp-cli/internal/cliutil"
	"strings"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/fortimgr"
)

func newFMGClient(flags *rootFlags) (*fortimgr.Client, error) {
	base := strings.TrimSpace(cliutil.EnvOverride("FORTINET_FMG_URL"))
	key := strings.TrimSpace(cliutil.EnvOverride("FORTINET_FMG_API_KEY"))
	if base == "" || key == "" {
		return nil, configErr(fmt.Errorf("set FORTINET_FMG_URL (https://<fortimanager>) and FORTINET_FMG_API_KEY (a FortiManager 7.2.2+ REST API admin key; add this host as a trusted host)"))
	}
	insecure := insecureRequested() || strings.EqualFold(cliutil.EnvOverride("FORTINET_FMG_INSECURE"), "true")
	c, err := fortimgr.New(base, key, insecure, flags.timeout)
	if err != nil {
		return nil, configErr(err)
	}
	return c, nil
}

type fmgDeviceRow struct {
	ADOM       string `json:"adom"`
	Name       string `json:"name"`
	Hostname   string `json:"hostname,omitempty"`
	Serial     string `json:"sn,omitempty"`
	Platform   string `json:"platform,omitempty"`
	Firmware   string `json:"firmware,omitempty"`
	ConnStatus string `json:"conn_status,omitempty"`
	ConfStatus string `json:"conf_status,omitempty"`
	DBStatus   string `json:"db_status,omitempty"`
	HAMode     string `json:"ha_mode,omitempty"`
	IP         string `json:"ip,omitempty"`
}

func str(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprintf("%d", int64(x))
	case []any:
		parts := make([]string, 0, len(x))
		for _, p := range x {
			parts = append(parts, str(p))
		}
		return strings.Join(parts, ",")
	}
	return fmt.Sprint(v)
}

func fmgADOMs(ctx context.Context, c *fortimgr.Client, all bool) ([]map[string]any, error) {
	extra := map[string]any{"fields": []string{"name", "desc", "os_ver", "mr", "state", "mode", "restricted_prds"}}
	if !all {
		extra["filter"] = []any{"restricted_prds", "==", "fos"}
	}
	data, err := c.Get(ctx, "/dvmdb/adom", extra)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil || rows == nil {
		return nil, fmt.Errorf("FortiManager returned no ADOM list (data %s)", strings.TrimSpace(string(data)))
	}
	return rows, nil
}

func newFMGCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fmg",
		Short: "Read-only FortiManager view: ADOMs, managed devices with firmware and sync status",
		Long: strings.Trim(`
Read-only FortiManager JSON-RPC commands (only the "get" method is ever sent).
Configure with FORTINET_FMG_URL=https://<fortimanager> and FORTINET_FMG_API_KEY
(System Settings > Administrators > REST API admin, FortiManager 7.2.2 or later,
with this host as a trusted host). FORTINET_FMG_INSECURE=true, --insecure or
FORTINET_SKIP_TLS_VERIFY=true accepts a self-signed certificate.`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,2", "pp:parent-group": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}

	status := &cobra.Command{
		Use:         "status",
		Args:        rejectExtraArgs,
		Short:       "FortiManager version, serial, hostname, HA mode and ADOM settings",
		Example:     "  fortinet-cli fmg status --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fmg status")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := newFMGClient(flags)
			if err != nil {
				return err
			}
			data, err := c.Get(ctx, "/sys/status", nil)
			if err != nil {
				return fortimgrErr(err)
			}
			var v any
			_ = json.Unmarshal(data, &v)
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}

	var allADOMs bool
	adoms := &cobra.Command{
		Use:         "adoms",
		Args:        rejectExtraArgs,
		Short:       "List FortiGate ADOMs (one per MSP client) with their FortiOS version",
		Example:     "  fortinet-cli fmg adoms --agent\n  fortinet-cli fmg adoms --all",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fmg adoms")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := newFMGClient(flags)
			if err != nil {
				return err
			}
			rows, err := fmgADOMs(ctx, c, allADOMs)
			if err != nil {
				return fortimgrErr(err)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			for _, r := range rows {
				// os_ver is the major release ("7.0") and mr the minor: branch 7.4.
				ver := fortimgr.Firmware(map[string]any{"os_ver": r["os_ver"], "mr": r["mr"], "patch": -1.0})
				fmt.Fprintf(cmd.OutOrStdout(), "%-28s %-5s %s\n", str(r["name"]), ver, str(r["desc"]))
			}
			return nil
		},
	}
	adoms.Flags().BoolVar(&allADOMs, "all", false, "Include non-FortiGate ADOMs (FortiAnalyzer, FortiSwitch, ...)")

	var adomFilter string
	devices := &cobra.Command{
		Use:   "devices",
		Args:  rejectExtraArgs,
		Short: "List managed FortiGates per ADOM with firmware, connection, config-sync and HA status",
		Long: strings.Trim(`
List managed devices from /dvmdb/adom/<adom>/device. Without --adom every
FortiGate ADOM is listed (one call per ADOM). conn_status is the management
tunnel (up/down), conf_status whether the device matches the FortiManager copy
(insync/outofsync) and db_status whether the FortiManager copy has unsaved
modifications. Device secrets are never requested.`, "\n"),
		Example:     "  fortinet-cli fmg devices --agent\n  fortinet-cli fmg devices --adom acme --select name,firmware,conn_status",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fmg devices")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := newFMGClient(flags)
			if err != nil {
				return err
			}
			names := splitCSV(adomFilter)
			if len(names) == 0 {
				list, err := fmgADOMs(ctx, c, false)
				if err != nil {
					return fortimgrErr(err)
				}
				for _, a := range list {
					names = append(names, str(a["name"]))
				}
			}
			rows := make([]fmgDeviceRow, 0)
			failures := []fetchFailure{}
			for _, adom := range names {
				if adom == "" || strings.ContainsAny(adom, "/ ") {
					failures = append(failures, fetchFailure{ID: adom, Error: "invalid ADOM name"})
					continue
				}
				data, err := c.Get(ctx, "/dvmdb/adom/"+adom+"/device", map[string]any{"fields": fortimgr.DeviceFields, "loadsub": 0})
				if err != nil {
					failures = append(failures, fetchFailure{ID: adom, Error: err.Error()})
					continue
				}
				var devs []map[string]any
				if err := json.Unmarshal(data, &devs); err != nil || devs == nil {
					failures = append(failures, fetchFailure{ID: adom, Error: "FortiManager returned no device list"})
					continue
				}
				for _, d := range devs {
					rows = append(rows, fmgDeviceRow{ADOM: adom, Name: str(d["name"]), Hostname: str(d["hostname"]), Serial: str(d["sn"]),
						Platform: str(d["platform_str"]), Firmware: fortimgr.Firmware(d), ConnStatus: str(d["conn_status"]),
						ConfStatus: str(d["conf_status"]), DBStatus: str(d["db_status"]), HAMode: str(d["ha_mode"]), IP: str(d["ip"])})
				}
			}
			if len(failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d ADOM(s) could not be read\n", len(failures), len(names))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				view := struct {
					Devices       []fmgDeviceRow `json:"devices"`
					FetchFailures []fetchFailure `json:"fetch_failures,omitempty"`
				}{rows, failures}
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			for _, r := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-24s %-16s %-18s conn=%-5s conf=%s\n", r.ADOM, r.Name, r.Serial, r.Firmware, r.ConnStatus, r.ConfStatus)
			}
			for _, f := range failures {
				fmt.Fprintf(cmd.OutOrStdout(), "ADOM %s: %s\n", f.ID, f.Error)
			}
			return nil
		},
	}
	devices.Flags().StringVar(&adomFilter, "adom", "", "Only these ADOMs (comma-separated); default every FortiGate ADOM")

	cmd.AddCommand(status, adoms, devices)
	return cmd
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newFMGCmd(flags))
	})
}

// fortimgrErr maps rejected credentials to exit 4 and everything else to exit 5.
func fortimgrErr(err error) error {
	if fortimgr.IsAuth(err) {
		return authErr(err)
	}
	return apiErr(err)
}
