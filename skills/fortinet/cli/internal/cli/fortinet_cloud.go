// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cloud: read-only FortiGate Cloud device inventory.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/cliutil"
	"fortinet-pp-cli/internal/fgtcloud"
)

func newCloudClient(flags *rootFlags) (*fgtcloud.Client, error) {
	base, err := fgtcloud.BaseURL(cliutil.EnvOverride("FORTINET_CLOUD_REGION"))
	if err != nil {
		return nil, configErr(err)
	}
	cache := ""
	if dir, err := cliutil.StateDir(); err == nil {
		cache = filepath.Join(dir, "fortigate-cloud-token.json")
	}
	c, err := fgtcloud.New(base, cliutil.EnvOverride("FORTINET_CLOUD_API_USER"), cliutil.EnvOverride("FORTINET_CLOUD_API_PASSWORD"),
		cliutil.EnvOverride("FORTINET_CLOUD_ACCOUNT_ID"), cache, flags.timeout)
	if err != nil {
		return nil, configErr(err)
	}
	return c, nil
}

func newCloudCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloud",
		Short: "Read-only FortiGate Cloud inventory: devices, firmware, contract end and management tunnel state",
		Long: strings.Trim(`
FortiGate Cloud commands, authenticated with a FortiCloud IAM API user
(IAM > API Users, with FortiGate Cloud portal permission):
  FORTINET_CLOUD_API_USER      API user ID
  FORTINET_CLOUD_API_PASSWORD  API user password
  FORTINET_CLOUD_REGION        global (default), us, eu or jp: the region the devices were provisioned in
  FORTINET_CLOUD_ACCOUNT_ID    required when FortiCloud Organizations are enabled
The access token is cached (0600) and at most one password login runs per
invocation: repeated failed logins lock the FortiCloud account for 15-30 minutes.`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,2", "pp:parent-group": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}

	var pageSize, maxPages int
	devices := &cobra.Command{
		Use:   "devices",
		Args:  rejectExtraArgs,
		Short: "List FortiGates in FortiGate Cloud with model, firmware, contract end and tunnel state",
		Long: strings.Trim(`
GET /forticloudapi/v1/devices, paged with limit/offset until a short page.
Fields are passed through as FortiGate Cloud returns them (sn, name, model,
firmwareVersion, tunnelAlive, management, contractEndTime, ...). --max-pages
caps the walk; capped=true means more devices may exist.`, "\n"),
		Example:     "  fortinet-cli cloud devices --agent\n  fortinet-cli cloud devices --select devices.sn,devices.firmwareVersion,devices.tunnelAlive",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cloud devices")
			}
			if pageSize < 1 || maxPages < 1 {
				return usageErr(fmt.Errorf("--page-size and --max-pages must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := newCloudClient(flags)
			if err != nil {
				return err
			}
			items, capped, err := c.Devices(ctx, pageSize, maxPages)
			if c.CacheWarning != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+c.CacheWarning)
			}
			if err != nil {
				return fgtcloudErr(err)
			}
			view := struct {
				Devices []map[string]any `json:"devices"`
				Count   int              `json:"count"`
				Capped  bool             `json:"capped"`
			}{items, len(items), capped}
			if capped {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: stopped after %d page(s); raise --max-pages to see more\n", maxPages)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			for _, d := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%-18s %-24s %-10s %-10s tunnel=%v\n", str(d["sn"]), str(d["name"]), str(d["model"]), str(d["firmwareVersion"]), d["tunnelAlive"])
			}
			return nil
		},
	}
	devices.Flags().IntVar(&pageSize, "page-size", 100, "Devices per request (limit)")
	devices.Flags().IntVar(&maxPages, "max-pages", 20, "Maximum pages to fetch")

	device := &cobra.Command{
		Use:         "device <serial>",
		Short:       "Show one FortiGate Cloud device by serial number",
		Example:     "  fortinet-cli cloud device FGT60FTK21000000 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cloud device")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a device serial number is required"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := newCloudClient(flags)
			if err != nil {
				return err
			}
			raw, err := c.Get(ctx, "/devices/"+url.PathEscape(strings.TrimSpace(args[0])), nil)
			if c.CacheWarning != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+c.CacheWarning)
			}
			if err != nil {
				return fgtcloudErr(err)
			}
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				return apiErr(fmt.Errorf("decode device: %w", err))
			}
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}

	cmd.AddCommand(devices, device)
	return cmd
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newCloudCmd(flags))
	})
}

// fgtcloudErr maps rejected credentials to exit 4 and everything else to exit 5.
func fgtcloudErr(err error) error {
	if fgtcloud.IsAuth(err) {
		return authErr(err)
	}
	return apiErr(err)
}
