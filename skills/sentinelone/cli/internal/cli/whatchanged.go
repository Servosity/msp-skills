// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// parseSinceWindow parses a window like "24h", "7d", "2w" (and any Go
// time.ParseDuration form). Empty defaults to 24h.
func parseSinceWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 24 * time.Hour, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	unit := s[len(s)-1]
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, fmt.Errorf("invalid --since %q (use forms like 24h, 7d, 2w)", s)
	}
	switch unit {
	case 'd', 'D':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w', 'W':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid --since unit in %q (use h, d, or w)", s)
	}
}

type wcChange struct {
	Endpoint string `json:"endpoint"`
	ID       string `json:"id,omitempty"`
	Site     string `json:"site,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// newNovelWhatchangedCmd diffs the latest fleet snapshot against the snapshot
// nearest the start of the window, answering "what changed across all my
// tenants since I logged off?" — new threats, agents that went offline,
// version regressions, and protection-mode flips. The live API has no
// cross-entity "delta since T" endpoint.
// pp:data-source local
func newNovelWhatchangedCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var since string
	var limit int

	cmd := &cobra.Command{
		Use:   "whatchanged",
		Short: "Diff the fleet against an earlier snapshot: new threats, agents offline, version/mode changes",
		Long: `Compare the most recent history snapshot against the one nearest the start of
the --since window and report what changed across every site:

  + new threats          threats seen now but not at the baseline
  + agents went offline   network status flipped connected -> disconnected
  + new / removed agents  endpoints that appeared or disappeared
  + version changes       agentVersion changed between snapshots
  + protection flips      mitigation mode changed (e.g. protect -> detect)

History accrues one snapshot per 'sentinelone-cli sync', so at least two
syncs spanning the window are required.`,
		Example: `  # What changed in the last 24h
  sentinelone-cli whatchanged --since 24h

  # The last week, as JSON
  sentinelone-cli whatchanged --since 7d --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			window, err := parseSinceWindow(since)
			if err != nil {
				return err
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openS1Store(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			times, err := db.FleetSnapshotTimes("agents")
			if err != nil {
				return fmt.Errorf("reading snapshot history: %w", err)
			}
			if len(times) < 2 {
				return honestEmptyJSON(cmd, flags,
					"Need at least two history snapshots to diff. Run 'sentinelone-cli sync' again over time (one snapshot is captured per sync).",
					map[string]any{"snapshots": len(times)})
			}

			latest := times[len(times)-1]
			cutoff := time.Now().Add(-window)
			baseline := times[0]
			if t, ok, _ := db.FleetSnapshotNearestBefore("agents", cutoff); ok && t.Before(latest) {
				baseline = t
			}
			if !baseline.Before(latest) {
				// Every snapshot falls inside the window; fall back to the oldest.
				baseline = times[0]
			}
			if !baseline.Before(latest) {
				return honestEmptyJSON(cmd, flags,
					"All snapshots are within the window; need an earlier baseline to diff against.", nil)
			}

			baseAgents, err := db.FleetSnapshotRowsByID("agents", baseline)
			if err != nil {
				return err
			}
			curAgents, err := db.FleetSnapshotRowsByID("agents", latest)
			if err != nil {
				return err
			}
			baseThreats, err := db.FleetSnapshotRowsByID("threats", baseline)
			if err != nil {
				return err
			}
			curThreats, err := db.FleetSnapshotRowsByID("threats", latest)
			if err != nil {
				return err
			}

			var newAgents, removedAgents, wentOffline, versionChanged, modeFlipped []wcChange
			for id, raw := range curAgents {
				cur := decodeObjects([]json.RawMessage{raw})
				if len(cur) == 0 {
					continue
				}
				ca := cur[0]
				braw, ok := baseAgents[id]
				if !ok {
					newAgents = append(newAgents, wcChange{Endpoint: gstrFirst(ca, "computerName", "id"), ID: id, Site: orUnknown(agentSite(ca))})
					continue
				}
				base := decodeObjects([]json.RawMessage{braw})
				if len(base) == 0 {
					continue
				}
				ba := base[0]
				name := gstrFirst(ca, "computerName", "id")
				site := orUnknown(agentSite(ca))
				if agentOnline(ba) && !agentOnline(ca) {
					wentOffline = append(wentOffline, wcChange{Endpoint: name, ID: id, Site: site, Detail: "connected → disconnected"})
				}
				if bv, cv := gstr(ba, "agentVersion"), gstr(ca, "agentVersion"); bv != cv && bv != "" && cv != "" {
					versionChanged = append(versionChanged, wcChange{Endpoint: name, ID: id, Site: site, Detail: bv + " → " + cv})
				}
				if bm, cm := gstr(ba, "mitigationMode"), gstr(ca, "mitigationMode"); bm != cm && bm != "" && cm != "" {
					modeFlipped = append(modeFlipped, wcChange{Endpoint: name, ID: id, Site: site, Detail: bm + " → " + cm})
				}
			}
			for id, raw := range baseAgents {
				if _, ok := curAgents[id]; !ok {
					ba := decodeObjects([]json.RawMessage{raw})
					name, site := id, ""
					if len(ba) > 0 {
						name = gstrFirst(ba[0], "computerName", "id")
						site = orUnknown(agentSite(ba[0]))
					}
					removedAgents = append(removedAgents, wcChange{Endpoint: name, ID: id, Site: site})
				}
			}

			var newThreats, resolvedThreats []wcChange
			for id, raw := range curThreats {
				if _, ok := baseThreats[id]; ok {
					continue
				}
				ct := decodeObjects([]json.RawMessage{raw})
				if len(ct) == 0 {
					continue
				}
				newThreats = append(newThreats, wcChange{Endpoint: threatEndpoint(ct[0]), ID: id, Site: orUnknown(threatSite(ct[0])), Detail: threatName(ct[0])})
			}
			for id, raw := range baseThreats {
				bt := decodeObjects([]json.RawMessage{raw})
				if len(bt) == 0 || !threatActive(bt[0]) {
					continue
				}
				cur, ok := curThreats[id]
				if !ok {
					resolvedThreats = append(resolvedThreats, wcChange{Endpoint: threatEndpoint(bt[0]), ID: id, Site: orUnknown(threatSite(bt[0])), Detail: "no longer present"})
					continue
				}
				ct := decodeObjects([]json.RawMessage{cur})
				if len(ct) > 0 && !threatActive(ct[0]) {
					resolvedThreats = append(resolvedThreats, wcChange{Endpoint: threatEndpoint(ct[0]), ID: id, Site: orUnknown(threatSite(ct[0])), Detail: "now " + threatMitigation(ct[0])})
				}
			}

			capList := func(c []wcChange) []wcChange {
				if limit > 0 && len(c) > limit {
					return c[:limit]
				}
				return c
			}

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"baseline":         baseline.Format(time.RFC3339),
					"latest":           latest.Format(time.RFC3339),
					"window":           since,
					"new_threats":      capList(newThreats),
					"resolved_threats": capList(resolvedThreats),
					"agents_offline":   capList(wentOffline),
					"new_agents":       capList(newAgents),
					"removed_agents":   capList(removedAgents),
					"version_changes":  capList(versionChanged),
					"mode_flips":       capList(modeFlipped),
					"counts": map[string]int{
						"new_threats":      len(newThreats),
						"resolved_threats": len(resolvedThreats),
						"agents_offline":   len(wentOffline),
						"new_agents":       len(newAgents),
						"removed_agents":   len(removedAgents),
						"version_changes":  len(versionChanged),
						"mode_flips":       len(modeFlipped),
					},
				})
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Changes from %s to %s (window %s):\n\n",
				baseline.Format("2006-01-02 15:04"), latest.Format("2006-01-02 15:04"), since)
			section := func(title string, c []wcChange) {
				fmt.Fprintf(w, "%s: %d\n", title, len(c))
				for _, ch := range capList(c) {
					detail := ch.Detail
					if detail != "" {
						detail = " — " + detail
					}
					fmt.Fprintf(w, "    %-26s %-20s%s\n", clip(ch.Endpoint, 26), clip(ch.Site, 20), detail)
				}
			}
			section("New threats", newThreats)
			section("Resolved/cleared threats", resolvedThreats)
			section("Agents went offline", wentOffline)
			section("New agents", newAgents)
			section("Removed agents", removedAgents)
			section("Version changes", versionChanged)
			section("Protection-mode flips", modeFlipped)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().StringVar(&since, "since", "24h", "Window to diff against (e.g. 24h, 7d, 2w)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum items to list per change category (0 = all)")
	return cmd
}
