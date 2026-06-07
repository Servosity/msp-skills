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

type secBucketDist struct {
	Range string `json:"range"`
	Count int    `json:"count"`
}

type secDevice struct {
	Hostname string `json:"hostname"`
	DeviceID string `json:"device_id"`
	Score    int    `json:"score"`
	GroupID  string `json:"group_id,omitempty"`
	Group    string `json:"group,omitempty"`
}

type secGroupRollup struct {
	Group       string  `json:"group"`
	GroupID     string  `json:"group_id,omitempty"`
	CountScored int     `json:"count_scored"`
	Avg         float64 `json:"avg"`
	Min         int     `json:"min"`
	BelowCount  int     `json:"below_count"`
}

type secResult struct {
	Threshold     int              `json:"threshold"`
	CountScored   int              `json:"count_scored"`
	CountUnscored int              `json:"count_unscored"`
	Avg           float64          `json:"avg"`
	Min           int              `json:"min"`
	Max           int              `json:"max"`
	BelowCount    int              `json:"below_count"`
	Distribution  []secBucketDist  `json:"distribution"`
	Below         []secDevice      `json:"below"`
	ByGroup       []secGroupRollup `json:"by_group,omitempty"`
}

func secBucket(score int) string {
	switch {
	case score < 50:
		return "0-49"
	case score < 70:
		return "50-69"
	case score < 90:
		return "70-89"
	default:
		return "90-100"
	}
}

// lvlComputeSecurityPosture summarises the device security-score distribution,
// the below-threshold list, and (optionally) a per-group rollup.
func lvlComputeSecurityPosture(devices []lvlDevice, groups []lvlGroup, below int, byGroup bool) secResult {
	res := secResult{Threshold: below, Min: -1, Max: -1}
	idx := lvlBuildGroupIndex(groups)

	dist := map[string]int{"0-49": 0, "50-69": 0, "70-89": 0, "90-100": 0}
	sum := 0

	type gagg struct {
		count, sum, min, below int
	}
	groupAgg := map[string]*gagg{}
	groupOrder := []string{}

	for _, d := range devices {
		if d.SecurityScore == nil {
			res.CountUnscored++
			continue
		}
		score := *d.SecurityScore
		res.CountScored++
		sum += score
		dist[secBucket(score)]++
		if res.Min < 0 || score < res.Min {
			res.Min = score
		}
		if res.Max < 0 || score > res.Max {
			res.Max = score
		}
		if score < below {
			res.BelowCount++
			res.Below = append(res.Below, secDevice{
				Hostname: lvlDeviceLabel(d), DeviceID: d.ID, Score: score,
				GroupID: d.GroupID, Group: idx.name(d.GroupID),
			})
		}
		if byGroup {
			g, ok := groupAgg[d.GroupID]
			if !ok {
				g = &gagg{min: score}
				groupAgg[d.GroupID] = g
				groupOrder = append(groupOrder, d.GroupID)
			}
			g.count++
			g.sum += score
			if score < g.min {
				g.min = score
			}
			if score < below {
				g.below++
			}
		}
	}

	if res.CountScored > 0 {
		res.Avg = round1(float64(sum) / float64(res.CountScored))
	}
	if res.Min < 0 {
		res.Min = 0
	}
	if res.Max < 0 {
		res.Max = 0
	}
	for _, r := range []string{"0-49", "50-69", "70-89", "90-100"} {
		res.Distribution = append(res.Distribution, secBucketDist{Range: r, Count: dist[r]})
	}
	sort.SliceStable(res.Below, func(i, j int) bool {
		if res.Below[i].Score != res.Below[j].Score {
			return res.Below[i].Score < res.Below[j].Score
		}
		return res.Below[i].Hostname < res.Below[j].Hostname
	})

	if byGroup {
		for _, gid := range groupOrder {
			g := groupAgg[gid]
			avg := 0.0
			if g.count > 0 {
				avg = round1(float64(g.sum) / float64(g.count))
			}
			res.ByGroup = append(res.ByGroup, secGroupRollup{
				Group: idx.name(gid), GroupID: gid, CountScored: g.count, Avg: avg, Min: g.min, BelowCount: g.below,
			})
		}
		sort.SliceStable(res.ByGroup, func(i, j int) bool {
			if res.ByGroup[i].Avg != res.ByGroup[j].Avg {
				return res.ByGroup[i].Avg < res.ByGroup[j].Avg
			}
			return res.ByGroup[i].Group < res.ByGroup[j].Group
		})
	}
	return res
}

// pp:data-source local
func newNovelSecurityPostureCmd(flags *rootFlags) *cobra.Command {
	var below int
	var byGroup bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "security-posture",
		Short:       "Fleet security-score distribution, below-threshold list, and optional per-group rollup",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Summarise the device security-score distribution across the synced fleet,
list every device below --below, and optionally roll the scores up by group with
--by-group. Computed offline from the local store; devices without a score are
counted separately.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Distribution plus everyone under 70
  levelio-cli security-posture --below 70

  # Per-group rollup, JSON for agents
  levelio-cli security-posture --below 70 --by-group --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
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
			res := lvlComputeSecurityPosture(devices, groups, below, byGroup)

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "%d scored device(s): avg %.1f, min %d, max %d; %d below %d (%d unscored)\n",
				res.CountScored, res.Avg, res.Min, res.Max, res.BelowCount, res.Threshold, res.CountUnscored)
			fmt.Fprintln(out, "RANGE\tCOUNT")
			for _, b := range res.Distribution {
				fmt.Fprintf(out, "%s\t%d\n", b.Range, b.Count)
			}
			if len(res.Below) > 0 {
				fmt.Fprintf(out, "\nBelow %d:\nSCORE\tHOSTNAME\tGROUP\n", res.Threshold)
				for _, d := range res.Below {
					fmt.Fprintf(out, "%d\t%s\t%s\n", d.Score, d.Hostname, d.Group)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&below, "below", 70, "List devices with a security score under this threshold")
	cmd.Flags().BoolVar(&byGroup, "by-group", false, "Also roll scores up by group")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
