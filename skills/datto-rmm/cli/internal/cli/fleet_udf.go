// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// fleetUdfSlots is the number of user-defined fields a Datto RMM device
// carries (vendor schema Udf: udf1 .. udf300).
const fleetUdfSlots = 300

type udfValueRow struct {
	Value   string `json:"value"`
	Devices int    `json:"devices"`
	Sites   int    `json:"sites"`
}

type udfDeviceRow struct {
	Hostname string `json:"hostname"`
	SiteName string `json:"siteName"`
	UID      string `json:"uid"`
	Value    string `json:"value"`
}

type udfView struct {
	Slot           string         `json:"slot"`
	DevicesTotal   int            `json:"devices_total"`
	WithValue      int            `json:"with_value"`
	Empty          int            `json:"empty"`
	DistinctValues int            `json:"distinct_values"`
	Values         []udfValueRow  `json:"values"`
	Devices        []udfDeviceRow `json:"devices"`
	Truncated      bool           `json:"truncated"`
}

// normalizeUdfSlot accepts "udf7", "UDF7" or "7" and returns "udf7".
func normalizeUdfSlot(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "udf")
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > fleetUdfSlots || strconv.Itoa(n) != s {
		return "", fmt.Errorf("unknown UDF slot %q: use udf1 .. udf%d (or 1 .. %d)", raw, fleetUdfSlots, fleetUdfSlots)
	}
	return "udf" + s, nil
}

type udfDevice struct {
	UID      string         `json:"uid"`
	Hostname string         `json:"hostname"`
	SiteName string         `json:"siteName"`
	SiteUID  string         `json:"siteUid"`
	Udf      map[string]any `json:"udf"`
}

// computeUdf rolls one UDF slot up across the synced fleet: how many devices
// carry a value, each distinct value with its device and site counts (most
// devices first), and, when match is set ("value" or "empty"), the matching
// devices. Totals are counted before limit; limit 0 keeps every row.
func computeUdf(raws [][]byte, slot, match, value string, limit int) udfView {
	view := udfView{Slot: slot, Values: []udfValueRow{}, Devices: []udfDeviceRow{}}
	type agg struct {
		devices int
		sites   map[string]bool
	}
	byValue := map[string]*agg{}
	for _, raw := range raws {
		var d udfDevice
		if json.Unmarshal(raw, &d) != nil {
			continue
		}
		view.DevicesTotal++
		v := ""
		if x, ok := d.Udf[slot]; ok && x != nil {
			v = strings.TrimSpace(fmt.Sprint(x))
		}
		if v == "" {
			view.Empty++
		} else {
			view.WithValue++
			a := byValue[v]
			if a == nil {
				a = &agg{sites: map[string]bool{}}
				byValue[v] = a
			}
			a.devices++
			site := d.SiteUID
			if site == "" {
				site = d.SiteName
			}
			a.sites[site] = true
		}
		if (match == "empty" && v == "") || (match == "value" && v == value) {
			view.Devices = append(view.Devices, udfDeviceRow{Hostname: d.Hostname, SiteName: d.SiteName, UID: d.UID, Value: v})
		}
	}
	view.DistinctValues = len(byValue)
	for v, a := range byValue {
		view.Values = append(view.Values, udfValueRow{Value: v, Devices: a.devices, Sites: len(a.sites)})
	}
	sort.Slice(view.Values, func(i, j int) bool {
		if view.Values[i].Devices != view.Values[j].Devices {
			return view.Values[i].Devices > view.Values[j].Devices
		}
		return view.Values[i].Value < view.Values[j].Value
	})
	sort.SliceStable(view.Devices, func(i, j int) bool {
		if view.Devices[i].SiteName != view.Devices[j].SiteName {
			return view.Devices[i].SiteName < view.Devices[j].SiteName
		}
		return view.Devices[i].Hostname < view.Devices[j].Hostname
	})
	if limit > 0 {
		if len(view.Values) > limit {
			view.Values, view.Truncated = view.Values[:limit], true
		}
		if len(view.Devices) > limit {
			view.Devices, view.Truncated = view.Devices[:limit], true
		}
	}
	return view
}

// pp:data-source local
func newNovelFleetUdfCmd(flags *rootFlags) *cobra.Command {
	var value string
	var empty bool
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "udf <slot>",
		Short:       "Rolls one device user-defined field up across the fleet: values, device and site counts, gaps",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.ExactArgs(1),
		Example: `  datto-rmm-cli fleet udf udf7
  datto-rmm-cli fleet udf 3 --value "Managed" --json
  datto-rmm-cli fleet udf udf12 --empty --agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			slot, err := normalizeUdfSlot(args[0])
			if err != nil {
				return usageErr(err)
			}
			if empty && cmd.Flags().Changed("value") {
				return usageErr(fmt.Errorf("--value and --empty are exclusive"))
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openFleetStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, fleetDevicesResource) {
				hintIfStale(cmd, db, fleetDevicesResource, flags.maxAge)
			}
			raws, err := queryRaw(cmd.Context(), db, fleetDevicesResource)
			if err != nil {
				return err
			}
			match := ""
			switch {
			case empty:
				match = "empty"
			case cmd.Flags().Changed("value"):
				match = "value"
			}
			view := computeUdf(raws, slot, match, strings.TrimSpace(value), limit)

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			if match != "" {
				rows := make([][]string, 0, len(view.Devices))
				for _, d := range view.Devices {
					rows = append(rows, []string{d.Hostname, d.SiteName, d.Value})
				}
				if err := flags.printTable(cmd, []string{"HOSTNAME", "SITE", strings.ToUpper(slot)}, rows); err != nil {
					return err
				}
			} else {
				rows := make([][]string, 0, len(view.Values))
				for _, v := range view.Values {
					rows = append(rows, []string{v.Value, strconv.Itoa(v.Devices), strconv.Itoa(v.Sites)})
				}
				if err := flags.printTable(cmd, []string{strings.ToUpper(slot), "DEVICES", "SITES"}, rows); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d of %d devices carry a value (%d distinct), %d empty\n",
				slot, view.WithValue, view.DevicesTotal, view.DistinctValues, view.Empty)
			return nil
		},
	}
	cmd.Flags().StringVar(&value, "value", "", "List the devices whose slot holds exactly this value")
	cmd.Flags().BoolVar(&empty, "empty", false, "List the devices whose slot is empty")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to show (0 = all); totals always count every device")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
