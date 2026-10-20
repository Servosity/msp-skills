// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"backup-radar-pp-cli/internal/radar"
)

// Device-name keys tried, in order, on each vendor object when --name-field
// is not given. They cover the common shapes of vendor CLI JSON.
var vendorNameFields = []string{"hostname", "host_name", "hostName", "device_name", "deviceName", "computer_name", "computerName", "agent_name", "agentName", "machine_name", "machineName", "server_name", "serverName", "name"}

type coverageView struct {
	radar.CoverageResult
	NameFields     []string     `json:"name_fields"`
	SkippedVendor  int          `json:"skipped_vendor_items"`
	SkippedExample string       `json:"skipped_vendor_example,omitempty"`
	Sources        radarSources `json:"sources"`
}

func newNovelCoverageDiffCmd(flags *rootFlags) *cobra.Command {
	var devicesPath, names, nameField, client, dbPath string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff a backup vendor's device list against what Backup Radar monitors",
		Long: `Compare a backup vendor's device list with the synced Backup Radar store in
both directions: vendor_only (protected but not monitored), backup_radar_only,
and matched (match_method exact, short-name for the host name without its DNS
suffix, or short-name-ambiguous with every candidate listed). A match found
only among v2 inactive backups is reported with monitored=false. With
--client, a vendor device monitored under a different client is matched with
other_client=true rather than reported as a gap.

Give the list with --devices (a JSON array, an object wrapping one, JSONL, or
one name per line; - reads stdin), as exported by a vendor CLI such as
datto-bcdr, veeam, cove, axcient or acronis, or inline with --names. Objects
without a recognizable name field are counted in skipped_vendor_items.

Use this command to reconcile an external vendor's device list against Backup Radar in both directions.
Do NOT use this command for blind spots inside Backup Radar such as inactive, unmapped or ticketless backups; use 'unwatched' instead.`,
		Example: `  backup-radar-cli coverage diff --devices vendor-devices.json --name-field hostname --agent
  backup-radar-cli coverage diff --names DC01,SQL01,FS01 --client "Acme Dental"
  datto-bcdr-cli agent list --all --json | backup-radar-cli coverage diff --devices -`,
		// --devices reads a file path; MCP callers use --names instead so a
		// tool call can never read arbitrary files from the server account.
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "mcp:write-flags": "devices,csv,plain,quiet"}, // HAND-FIX(radar-machine-modes)
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseTabularModes(flags, "coverage diff"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "coverage diff")
			}
			if (devicesPath == "") == (names == "") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("give exactly one of --devices (file path, or - for stdin) or --names (comma-separated)"))
			}
			var list []string
			fields := []string{}
			skipped, skippedExample := 0, ""
			if names != "" {
				list = splitCSVFlag(names)
				fields = append(fields, "(--names)")
			} else {
				data, err := readPathOrStdin(cmd, devicesPath)
				if err != nil {
					return fmt.Errorf("reading --devices: %w", err)
				}
				list, fields, skipped, skippedExample, err = vendorDeviceNames(data, nameField)
				if err != nil {
					return usageErr(err)
				}
				if skipped > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d vendor item(s) had no device name in %s and were not compared; pass --name-field\n", skipped, strings.Join(vendorNameFields, "/"))
				}
			}
			snap, _, err := loadRadarSnapshot(cmd, flags, dbPath)
			if err != nil {
				return err
			}
			view := coverageView{CoverageResult: radar.Coverage(snap, list, client), NameFields: fields, SkippedVendor: skipped, SkippedExample: skippedExample, Sources: sourcesOf(snap, "devices", "backups", "backups-inactive")}
			if !snap.AnySynced() {
				view.Sources.Note = "nothing synced yet, so every vendor device shows as vendor_only; run 'backup-radar-cli sync' first"
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				keep := append(radarKeep(radar.CoverageMatch{}), radarKeep(radar.CoverageBROnly{})...)
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, append(keep, radarKeep(radar.VendorDevice{})...)...)
			}
			out := cmd.OutOrStdout()
			if view.Sources.Note != "" {
				fmt.Fprintln(out, "note: "+scrubRadarText(view.Sources.Note))
			}
			c := view.Counts
			fmt.Fprintf(out, "vendor devices %d: matched %d (%d not confirmed monitored, %d ambiguous), vendor-only %d; Backup Radar-only %d\n",
				c["vendor_devices"], c["matched"], c["matched_inactive_only"], c["ambiguous"], c["vendor_only"], c["backup_radar_only"])
			rows := [][]string{}
			for _, v := range view.VendorOnly {
				rows = append(rows, []string{"vendor-only (not monitored)", v.Name, ""})
			}
			for _, m := range view.Matched {
				switch {
				case len(m.Candidates) > 0:
					rows = append(rows, []string{"ambiguous", m.Name, strings.Join(m.Candidates, " | ")})
				case !m.Monitored:
					rows = append(rows, []string{"inactive only", m.Name, strings.Join(m.Clients, ", ")})
				case m.OtherClient:
					rows = append(rows, []string{"monitored under another client", m.Name, strings.Join(m.Clients, ", ")})
				}
			}
			for _, b := range view.BackupRadarOnly {
				rows = append(rows, []string{"backup-radar-only", b.Device, strings.Join(b.Clients, ", ")})
			}
			if len(rows) == 0 {
				if view.SkippedVendor > 0 {
					fmt.Fprintf(out, "Comparison incomplete: %d vendor item(s) had no device name and were not checked; pass --name-field.\n", view.SkippedVendor)
					return nil
				}
				fmt.Fprintln(out, "No gaps: every vendor device is monitored and every monitored device is in the vendor list.")
				return nil
			}
			return printRadarTable(out, []string{"gap", "device", "clients / candidates"}, rows)
		},
	}
	cmd.Flags().StringVar(&devicesPath, "devices", "", "Vendor device list file (JSON, JSONL, or one name per line), or - for stdin")
	cmd.Flags().StringVar(&names, "names", "", "Vendor device names inline, comma-separated (alternative to --devices)")
	cmd.Flags().StringVar(&nameField, "name-field", "", "JSON key holding the device name (default: per item, first of hostname, device_name, deviceName, computer_name, name...)")
	cmd.Flags().StringVar(&client, "client", "", "Only compare Backup Radar companies whose ScalePad or Backup Radar name contains this text")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the synced local store)")
	return cmd
}

// vendorDeviceNames extracts device names from a vendor CLI's output: a JSON
// array, an object wrapping the array under a common key, JSONL, or plain text
// with one name per line. Without --name-field each object falls back through
// vendorNameFields on its own; objects with no usable name are counted.
func vendorDeviceNames(data []byte, field string) (names, fields []string, skipped int, skippedExample string, err error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if len(data) == 0 {
		return nil, nil, 0, "", fmt.Errorf("--devices input is empty")
	}
	var items []any
	switch data[0] {
	case '[':
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, nil, 0, "", fmt.Errorf("parsing --devices JSON: %w", err)
		}
	case '{':
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err == nil {
			for _, k := range []string{"data", "results", "Results", "items", "devices", "rows", "value"} {
				if arr, ok := obj[k].([]any); ok {
					items = arr
					break
				}
			}
			if items == nil {
				items = []any{obj}
			}
		} else {
			for i, line := range strings.Split(string(data), "\n") {
				if line = strings.TrimSpace(line); line == "" {
					continue
				}
				var o any
				if err := json.Unmarshal([]byte(line), &o); err != nil {
					return nil, nil, 0, "", fmt.Errorf("--devices line %d is not JSON: %w", i+1, err)
				}
				items = append(items, o)
			}
		}
	default:
		for _, line := range strings.Split(string(data), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				names = append(names, line)
			}
		}
		return names, []string{"(text lines)"}, 0, "", nil
	}
	candidates := vendorNameFields
	if field != "" {
		candidates = []string{field}
	}
	used := map[string]bool{}
	for _, it := range items {
		switch v := it.(type) {
		case string:
			names = append(names, v)
			used["(string items)"] = true
		case map[string]any:
			found := false
			for _, f := range candidates {
				if s, ok := v[f].(string); ok && strings.TrimSpace(s) != "" {
					names, found = append(names, s), true
					used[f] = true
					break
				}
			}
			if !found {
				skipped++
				if skippedExample == "" {
					if b, err := json.Marshal(v); err == nil && len(b) <= 200 {
						skippedExample = string(b)
					}
				}
			}
		default:
			skipped++
		}
	}
	for f := range used {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	return names, fields, skipped, skippedExample, nil
}
