// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/comet"
)

type outdatedView struct {
	ServerVersion string            `json:"server_version"`
	Outdated      []comet.DeviceRow `json:"outdated"`
	Versions      map[string]int    `json:"versions"`
}

func newNovelDevicesOutdatedCmd(flags *rootFlags) *cobra.Command {
	var flagOnlineOnly bool
	var flagTenant string

	cmd := &cobra.Command{
		Use:   "outdated",
		Short: "List devices whose Comet client version differs from the server version, with the connection id updates need",
		Long: `List devices whose Comet client version differs from the server version, with the connection id updates need.

Use this command for devices whose Comet client version differs from the server version, with live connection IDs ready for 'devices update-software'. Do NOT use this command for devices that have stopped connecting; use 'devices stale' instead.

Connected devices are compared on the version they report live; disconnected devices on the version stored in their profile. 'versions' counts every registered device per client version.`,
		Example: strings.Trim(`
  comet-backup-cli devices outdated --agent
  comet-backup-cli devices outdated --online-only --select outdated.tenant,outdated.device,outdated.connection_id --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: guardRunE(flags, func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "devices outdated")
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			version, err := serverVersion(ctx, c)
			if err != nil {
				return fleetError(cmd, flags, c, err)
			}
			f, err := loadFleet(ctx, c)
			if err != nil {
				return fleetError(cmd, flags, c, err)
			}
			devices := comet.Devices(f.profiles, f.live, time.Now())
			out, counts := comet.Outdated(devices, version)
			view := outdatedView{ServerVersion: version, Outdated: make([]comet.DeviceRow, 0, len(out)), Versions: counts}
			for _, r := range out {
				if flagOnlineOnly && !r.Online {
					continue
				}
				if flagTenant != "" && !strings.EqualFold(r.Tenant, flagTenant) {
					continue
				}
				view.Outdated = append(view.Outdated, r)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, deviceAgentKeep...)
			}
			if len(view.Outdated) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Every device with a known version runs %s.\n", cleanCell(version))
				return nil
			}
			rows2 := make([][]string, 0, len(view.Outdated))
			for _, r := range view.Outdated {
				rows2 = append(rows2, []string{r.Tenant, r.Device, r.ClientVersion, fmt.Sprint(r.Online), r.ConnectionID})
			}
			return printCleanTable(cmd, flags, []string{"TENANT", "DEVICE", "VERSION", "ONLINE", "CONNECTION_ID"}, rows2)
		}),
	}
	cmd.Flags().BoolVar(&flagOnlineOnly, "online-only", false, "Only connected devices (the ones devices update-software can reach now)")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only this tenant (Comet user account username)")
	return cmd
}
