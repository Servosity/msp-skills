// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type fleetBucket struct {
	Key         string `json:"key"`
	Count       int    `json:"count"`
	Online      int    `json:"online"`
	Offline     int    `json:"offline"`
	Maintenance int    `json:"maintenance"`
}

type fleetSummary struct {
	Total       int `json:"total"`
	Online      int `json:"online"`
	Offline     int `json:"offline"`
	Maintenance int `json:"maintenance"`
}

type fleetResult struct {
	Dimension string            `json:"dimension"`
	Filters   map[string]string `json:"filters,omitempty"`
	Summary   fleetSummary      `json:"summary"`
	Buckets   []fleetBucket     `json:"buckets"`
}

var fleetDimensions = map[string]bool{
	"platform": true, "os": true, "group": true, "role": true, "country": true, "tag": true,
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
}

func lvlHasTag(d lvlDevice, tag string) bool {
	for _, t := range d.Tags {
		if strings.EqualFold(strings.TrimSpace(t), strings.TrimSpace(tag)) {
			return true
		}
	}
	return false
}

// lvlComputeFleet cross-tabs devices by a chosen dimension with online/offline
// and maintenance counts per bucket.
func lvlComputeFleet(devices []lvlDevice, groups []lvlGroup, by, tagFilter string, onlineOnly, offlineOnly bool) fleetResult {
	idx := lvlBuildGroupIndex(groups)
	res := fleetResult{Dimension: by}
	if tagFilter != "" || onlineOnly || offlineOnly {
		res.Filters = map[string]string{}
		if tagFilter != "" {
			res.Filters["tag"] = tagFilter
		}
		if onlineOnly {
			res.Filters["online"] = "true"
		}
		if offlineOnly {
			res.Filters["offline"] = "true"
		}
	}

	type agg struct{ count, online, offline, maint int }
	buckets := map[string]*agg{}
	order := []string{}
	add := func(key string, d lvlDevice) {
		a, ok := buckets[key]
		if !ok {
			a = &agg{}
			buckets[key] = a
			order = append(order, key)
		}
		a.count++
		if d.Online {
			a.online++
		} else {
			a.offline++
		}
		if d.MaintenanceMode {
			a.maint++
		}
	}

	for _, d := range devices {
		if tagFilter != "" && !lvlHasTag(d, tagFilter) {
			continue
		}
		if onlineOnly && !d.Online {
			continue
		}
		if offlineOnly && d.Online {
			continue
		}
		res.Summary.Total++
		if d.Online {
			res.Summary.Online++
		} else {
			res.Summary.Offline++
		}
		if d.MaintenanceMode {
			res.Summary.Maintenance++
		}
		switch by {
		case "tag":
			if len(d.Tags) == 0 {
				add("(untagged)", d)
			} else {
				for _, t := range d.Tags {
					add(t, d)
				}
			}
		case "group":
			add(idx.name(d.GroupID), d)
		case "os":
			add(lvlOSLabel(d), d)
		case "role":
			add(orUnknown(d.Role), d)
		case "country":
			add(orUnknown(d.Country), d)
		default: // platform
			add(orUnknown(d.Platform), d)
		}
	}

	for _, k := range order {
		a := buckets[k]
		res.Buckets = append(res.Buckets, fleetBucket{Key: k, Count: a.count, Online: a.online, Offline: a.offline, Maintenance: a.maint})
	}
	sort.SliceStable(res.Buckets, func(i, j int) bool {
		if res.Buckets[i].Count != res.Buckets[j].Count {
			return res.Buckets[i].Count > res.Buckets[j].Count
		}
		return res.Buckets[i].Key < res.Buckets[j].Key
	})
	return res
}

// pp:data-source local
func newNovelFleetCmd(flags *rootFlags) *cobra.Command {
	var by string
	var tagFilter string
	var onlineOnly bool
	var offlineOnly bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "fleet",
		Short:       "One-screen fleet inventory rollup, cross-tabbed by OS, platform, group, role, country, or tag",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Cross-tab the synced device estate by a single dimension with online,
offline, and maintenance counts per bucket. Computed offline from the local
store. Choose the dimension with --by (platform|os|group|role|country|tag) and
narrow with --tag / --online / --offline.

Use this command for an inventory cross-tab by OS/platform/group/tag. Do NOT
use it for a per-CLIENT posture rollup (alerts/score/patch exposure); use
'client-scorecard' instead.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Inventory by operating system
  levelio-cli fleet --by os

  # Online devices grouped by Level group, JSON for agents
  levelio-cli fleet --by group --online --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			by = strings.ToLower(strings.TrimSpace(by))
			if by == "" {
				by = "platform"
			}
			if !fleetDimensions[by] {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --by %q: choose one of platform, os, group, role, country, tag", by))
			}
			if onlineOnly && offlineOnly {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--online and --offline are mutually exclusive"))
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("levelio-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w\nRun 'levelio-cli sync' first.", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "devices") {
				hintIfStale(cmd, db, "devices", flags.maxAge)
			}

			devices, err := lvlDevices(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			groups, err := lvlGroups(db)
			if err != nil {
				return fmt.Errorf("loading groups: %w", err)
			}
			res := lvlComputeFleet(devices, groups, by, tagFilter, onlineOnly, offlineOnly)

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "Fleet by %s — %d device(s): %d online, %d offline, %d in maintenance\n",
				res.Dimension, res.Summary.Total, res.Summary.Online, res.Summary.Offline, res.Summary.Maintenance)
			if len(res.Buckets) == 0 {
				return nil
			}
			fmt.Fprintln(out, "COUNT\tONLINE\tOFFLINE\tKEY")
			for _, b := range res.Buckets {
				fmt.Fprintf(out, "%d\t%d\t%d\t%s\n", b.Count, b.Online, b.Offline, b.Key)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "platform", "Dimension to roll up: platform|os|group|role|country|tag")
	cmd.Flags().StringVar(&tagFilter, "tag", "", "Only count devices carrying this tag")
	cmd.Flags().BoolVar(&onlineOnly, "online", false, "Only count online devices")
	cmd.Flags().BoolVar(&offlineOnly, "offline", false, "Only count offline devices")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
