// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var versionNumbers = regexp.MustCompile(`\d+`)

// compareFirmware orders Fireware version strings ("2025.1.1", "12.10.4") by
// their numeric segments; it returns <0, 0 or >0.
func compareFirmware(a, b string) int {
	x, y := versionNumbers.FindAllString(a, -1), versionNumbers.FindAllString(b, -1)
	for i := 0; i < len(x) || i < len(y); i++ {
		var p, q int
		if i < len(x) {
			p, _ = strconv.Atoi(x[i])
		}
		if i < len(y) {
			q, _ = strconv.Atoi(y[i])
		}
		if p != q {
			return p - q
		}
	}
	return 0
}

type lagRow struct {
	Model         string `json:"model"`
	NewestVersion string `json:"newest_version_in_fleet"`
	DeviceID      string `json:"device_id"`
	Name          string `json:"name,omitempty"`
	Version       string `json:"version"`
	State         string `json:"state,omitempty"`
	Uptime        string `json:"uptime,omitempty"`
}

// firmwareLag lists devices older than the newest version seen for their model.
func firmwareLag(fleet map[string]fleetDevice) []lagRow {
	newest := map[string]string{}
	for _, d := range fleet {
		m, v := string(d.Model), string(d.Version)
		if m == "" || v == "" {
			continue
		}
		if cur, ok := newest[m]; !ok || compareFirmware(v, cur) > 0 {
			newest[m] = v
		}
	}
	out := make([]lagRow, 0)
	for _, id := range sortedMapKeys(fleet) {
		d := fleet[id]
		top, ok := newest[string(d.Model)]
		if !ok || d.Version == "" || compareFirmware(string(d.Version), top) >= 0 {
			continue
		}
		out = append(out, lagRow{Model: string(d.Model), NewestVersion: top, DeviceID: d.ID, Name: string(d.Name), Version: string(d.Version), State: string(d.State), Uptime: string(d.Uptime)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return compareFirmware(out[i].Version, out[j].Version) < 0
	})
	return out
}

type lagView struct {
	Devices int      `json:"devices_scanned"`
	Lagging []lagRow `json:"lagging"`
	Note    string   `json:"note"`
}

func newNovelFireboxDevicesFirmwareLagCmd(flags *rootFlags) *cobra.Command {
	var dbPath, model string
	var limit int
	cmd := &cobra.Command{
		Use:   "firmware-lag",
		Short: "Find synced Fireboxes and APs running older firmware than the newest version seen for the same model in your fleet",
		Long: "Use this command to find Fireboxes, FireClusters and Access Points running older firmware than the newest version observed for the same model in your fleet. " +
			"Do NOT use this command for what changed since the last check; use 'firebox-devices changes' instead.\n\n" +
			"Reads the locally synced device list, so sync first. \"Newest\" is the newest version among your synced devices, not WatchGuard's latest release, which no API publishes.",
		Example: strings.Trim(`
  watchguard-cli firebox-devices firmware-lag --agent
  watchguard-cli firebox-devices firmware-lag --model M390 --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "firebox-devices firmware-lag")
			}
			view := lagView{Lagging: make([]lagRow, 0), Note: "newest_version_in_fleet is the newest version observed among your synced devices of that model"}
			db, ok, err := openFleetStore(cmd, flags, dbPath, true)
			if err != nil {
				return err
			}
			if !ok {
				view.Note = noMirrorNote
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			defer db.Close()
			fleet, skipped, err := readFleet(cmd.Context(), db.DB(), `SELECT id, data FROM resources WHERE resource_type = ?`, fireboxDevicesResource)
			if err != nil {
				return fmt.Errorf("reading synced devices: %w", err)
			}
			warnSkipped(cmd, skipped)
			view.Devices = len(fleet)
			if len(fleet) == 0 {
				view.Note = "no synced devices; " + strings.TrimPrefix(noMirrorNote, "no local mirror: ")
			}
			for _, r := range firmwareLag(fleet) {
				if model == "" || strings.EqualFold(r.Model, model) {
					view.Lagging = append(view.Lagging, r)
				}
			}
			if limit > 0 && len(view.Lagging) > limit {
				view.Lagging = view.Lagging[:limit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Lagging) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No device is behind its model peers (%d scanned). %s\n", view.Devices, view.Note)
				return nil
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "MODEL\tDEVICE\tNAME\tVERSION\tNEWEST IN FLEET\tSTATE")
			for _, r := range view.Lagging {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Model, r.DeviceID, r.Name, r.Version, r.NewestVersion, r.State)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite database path (default: the CLI's data directory)")
	cmd.Flags().StringVar(&model, "model", "", "Only this model, e.g. M390 or T80")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}
