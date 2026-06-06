// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type driftView struct {
	Hostname       string `json:"hostname"`
	SiteName       string `json:"siteName"`
	CagVersion     string `json:"cagVersion"`
	LatestVersion  string `json:"latestVersion"`
	VersionsBehind int    `json:"versionsBehind"`
	LastSeen       string `json:"lastSeen"`
}

// compareVersions compares two dotted version strings segment-by-segment as
// integers (non-numeric segments count as 0). Returns -1 if a<b, 0 if equal,
// +1 if a>b.
func compareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(as) {
			ai = atoiSeg(as[i])
		}
		if i < len(bs) {
			bi = atoiSeg(bs[i])
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}

// atoiSeg parses a leading integer out of a version segment; non-numeric => 0.
func atoiSeg(s string) int {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return 0
	}
	return n
}

// computeAgentDrift flags devices running behind the highest cagVersion seen.
// Distinct versions are ranked descending (rank 0 = newest); a device is
// flagged when its version rank >= behind. behind==0 flags any below the max.
// Devices with an empty cagVersion are skipped. Sorted by VersionsBehind desc.
func computeAgentDrift(devices []fleetDevice, behind int) []driftView {
	// Collect distinct versions.
	seen := map[string]struct{}{}
	for _, d := range devices {
		if d.CagVersion == "" {
			continue
		}
		seen[d.CagVersion] = struct{}{}
	}
	if len(seen) == 0 {
		return []driftView{}
	}
	versions := make([]string, 0, len(seen))
	for v := range seen {
		versions = append(versions, v)
	}
	// Descending so versions[0] is newest.
	sort.SliceStable(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) > 0 })
	latest := versions[0]

	// Rank: newest distinct version = 0.
	rank := map[string]int{}
	for i, v := range versions {
		rank[v] = i
	}

	out := []driftView{}
	for _, d := range devices {
		if d.CagVersion == "" {
			continue
		}
		// Must be strictly below the max to be drifting.
		if compareVersions(d.CagVersion, latest) >= 0 {
			continue
		}
		r := rank[d.CagVersion]
		if r < behind {
			continue
		}
		lastSeen := ""
		if t, ok := parseDattoTime(d.LastSeen); ok {
			lastSeen = t.Format(time.RFC3339)
		}
		out = append(out, driftView{
			Hostname:       d.Hostname,
			SiteName:       d.SiteName,
			CagVersion:     d.CagVersion,
			LatestVersion:  latest,
			VersionsBehind: r,
			LastSeen:       lastSeen,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].VersionsBehind != out[j].VersionsBehind {
			return out[i].VersionsBehind > out[j].VersionsBehind
		}
		return out[i].Hostname < out[j].Hostname
	})
	return out
}

// pp:data-source local
func newNovelFleetAgentDriftCmd(flags *rootFlags) *cobra.Command {
	var behind int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "agent-drift",
		Short:       "Shows which devices run out-of-date RMM agents, ranked by how far behind the newest version they are",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet agent-drift
  datto-rmm-cli fleet agent-drift --behind 0 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			devices, err := loadFleetDevices(cmd.Context(), db)
			if err != nil {
				return err
			}
			view := computeAgentDrift(devices, behind)

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"HOSTNAME", "SITE", "VERSION", "LATEST", "BEHIND", "LAST SEEN"}
			rows := make([][]string, 0, len(view))
			for _, v := range view {
				rows = append(rows, []string{v.Hostname, v.SiteName, v.CagVersion, v.LatestVersion, strconv.Itoa(v.VersionsBehind), v.LastSeen})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().IntVar(&behind, "behind", 1, "Minimum version-rank behind newest to flag (0 = flag any below max)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
