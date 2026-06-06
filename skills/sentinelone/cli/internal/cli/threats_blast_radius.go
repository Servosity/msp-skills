// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type blastEndpoint struct {
	Endpoint   string `json:"endpoint"`
	Site       string `json:"site,omitempty"`
	ThreatID   string `json:"threat_id,omitempty"`
	Mitigation string `json:"mitigation_status,omitempty"`
	Active     bool   `json:"active"`
	DetectedAt string `json:"detected_at,omitempty"`
}

// newNovelThreatsBlastRadiusCmd traces one threat (by sha1, name, or id) across
// the whole fleet: every endpoint it touched, which are mitigated vs still
// active, the affected sites, and the spread timeline. The API returns threat
// rows, not an endpoint-joined containment view.
// pp:data-source local
func newNovelThreatsBlastRadiusCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "blast-radius <hash|name|threat-id>",
		Short: "Trace one threat across the fleet: every endpoint touched, mitigated vs active, spread timeline",
		Long: `Given a threat's SHA1, name, or id, join it across every synced threat row to
show the full containment picture: which endpoints it touched, which are
mitigated vs still active, the affected sites and groups, and the first/last
seen timeline. The API returns per-threat rows; this is the endpoint-joined
view you actually need during incident response.`,
		Example: `  # By SHA1
  sentinelone-cli threats blast-radius 3f5a9c2e1b7d8a4f6c0e2d1a9b8c7f6e5d4c3b2a

  # By name fragment, as JSON
  sentinelone-cli threats blast-radius Mimikatz --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			needle := strings.TrimSpace(args[0])
			needleLow := strings.ToLower(needle)

			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openS1Store(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "threats") {
				hintIfStale(cmd, db, "threats", flags.maxAge)
			}

			threats, err := loadThreats(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading threats: %w", err)
			}

			var eps []blastEndpoint
			sites := map[string]bool{}
			var mitigated, active int
			var firstSeen, lastSeen time.Time
			var threatLabel string
			for _, t := range threats {
				sha := threatSHA1(t)
				name := threatName(t)
				id := threatID(t)
				match := strings.EqualFold(sha, needle) ||
					strings.EqualFold(id, needle) ||
					(needleLow != "" && strings.Contains(strings.ToLower(name), needleLow))
				if !match {
					continue
				}
				if threatLabel == "" {
					threatLabel = name
					if sha != "" {
						threatLabel = name + " (" + sha + ")"
					}
				}
				isActive := threatActive(t)
				if isActive {
					active++
				} else {
					mitigated++
				}
				site := orUnknown(threatSite(t))
				sites[site] = true
				detected := threatCreatedAt(t)
				if ts, ok := parseS1Time(detected); ok {
					if firstSeen.IsZero() || ts.Before(firstSeen) {
						firstSeen = ts
					}
					if lastSeen.IsZero() || ts.After(lastSeen) {
						lastSeen = ts
					}
				}
				eps = append(eps, blastEndpoint{
					Endpoint:   threatEndpoint(t),
					Site:       site,
					ThreatID:   id,
					Mitigation: threatMitigation(t),
					Active:     isActive,
					DetectedAt: detected,
				})
			}

			if len(eps) == 0 {
				return honestEmptyJSON(cmd, flags,
					fmt.Sprintf("No threat matching %q in the local store. Run 'sentinelone-cli sync' or check the hash/name.", needle), nil)
			}

			// Active endpoints first, then by detection time.
			sort.SliceStable(eps, func(i, j int) bool {
				if eps[i].Active != eps[j].Active {
					return eps[i].Active
				}
				return eps[i].DetectedAt > eps[j].DetectedAt
			})
			siteList := make([]string, 0, len(sites))
			for s := range sites {
				siteList = append(siteList, s)
			}
			sort.Strings(siteList)

			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"query":           needle,
					"threat":          threatLabel,
					"endpoints_total": len(eps),
					"active":          active,
					"mitigated":       mitigated,
					"sites_affected":  siteList,
					"first_seen":      tsOrEmpty(firstSeen),
					"last_seen":       tsOrEmpty(lastSeen),
					"endpoints":       eps,
				})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Blast radius for %s:\n", threatLabel)
			fmt.Fprintf(w, "  %d endpoints across %d site(s) — %d active, %d mitigated\n", len(eps), len(siteList), active, mitigated)
			if !firstSeen.IsZero() {
				fmt.Fprintf(w, "  first seen %s, last seen %s\n", firstSeen.Format("2006-01-02 15:04"), lastSeen.Format("2006-01-02 15:04"))
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "%-28s %-20s %-14s %s\n", "ENDPOINT", "SITE", "STATE", "MITIGATION")
			for _, e := range eps {
				state := "mitigated"
				if e.Active {
					state = "ACTIVE"
				}
				fmt.Fprintf(w, "%-28s %-20s %-14s %s\n", clip(e.Endpoint, 28), clip(e.Site, 20), state, e.Mitigation)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	return cmd
}

func tsOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
