// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type verdictChange struct {
	ThreatID   string `json:"threat_id"`
	ThreatName string `json:"threat_name,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	Field      string `json:"field"`
	From       string `json:"from"`
	To         string `json:"to"`
}

// newNovelThreatsVerdictsCmd shows current verdict distribution, or with
// --changed, diffs the two most recent history snapshots to flag threats whose
// analyst verdict, confidence level, or incident status changed (e.g.
// suspicious -> malicious, or an auto-mitigated threat re-opened). The API
// returns the current verdict only, never a before/after.
// pp:data-source local
func newNovelThreatsVerdictsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var changed bool

	cmd := &cobra.Command{
		Use:   "verdicts",
		Short: "Verdict/confidence/incident distribution, or --changed to diff the last two snapshots",
		Long: `Without flags, show the current distribution of analyst verdicts, confidence
levels, and incident statuses across all threats.

With --changed, diff the two most recent history snapshots and flag every
threat whose analystVerdict, confidenceLevel, or incidentStatus changed — so a
suspicious->malicious escalation or a re-opened threat never slips by silently.
(--changed requires at least two syncs.)`,
		Example: `  # Current verdict distribution
  sentinelone-cli threats verdicts

  # What verdicts changed since the previous sync
  sentinelone-cli threats verdicts --changed --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
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

			if changed {
				times, err := db.FleetSnapshotTimes("threats")
				if err != nil {
					return fmt.Errorf("reading snapshot history: %w", err)
				}
				if len(times) < 2 {
					return honestEmptyJSON(cmd, flags,
						"Need at least two history snapshots to diff verdicts. Run 'sentinelone-cli sync' again over time.",
						map[string]any{"snapshots": len(times)})
				}
				prev := times[len(times)-2]
				latest := times[len(times)-1]
				prevByID, err := db.FleetSnapshotRowsByID("threats", prev)
				if err != nil {
					return err
				}
				curByID, err := db.FleetSnapshotRowsByID("threats", latest)
				if err != nil {
					return err
				}
				var changes []verdictChange
				for id, rawCur := range curByID {
					rawPrev, ok := prevByID[id]
					if !ok {
						continue
					}
					cs := decodeObjects([]json.RawMessage{rawCur})
					ps := decodeObjects([]json.RawMessage{rawPrev})
					if len(cs) == 0 || len(ps) == 0 {
						continue
					}
					c, p := cs[0], ps[0]
					name := threatName(c)
					ep := threatEndpoint(c)
					for _, f := range []struct {
						field    string
						from, to string
					}{
						{"analyst_verdict", threatVerdict(p), threatVerdict(c)},
						{"confidence_level", threatConfidence(p), threatConfidence(c)},
						{"incident_status", threatIncident(p), threatIncident(c)},
					} {
						if f.from != f.to && (f.from != "" || f.to != "") {
							changes = append(changes, verdictChange{
								ThreatID: id, ThreatName: name, Endpoint: ep,
								Field: f.field, From: f.from, To: f.to,
							})
						}
					}
				}
				if len(changes) == 0 {
					return honestEmptyJSON(cmd, flags, "No verdict, confidence, or incident-status changes between the last two snapshots.", nil)
				}
				sort.SliceStable(changes, func(i, j int) bool { return changes[i].ThreatName < changes[j].ThreatName })
				if flags.asJSON {
					return flags.printJSON(cmd, map[string]any{
						"from_snapshot": prev.Format("2006-01-02T15:04:05Z07:00"),
						"to_snapshot":   latest.Format("2006-01-02T15:04:05Z07:00"),
						"changes":       changes,
					})
				}
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "Verdict changes between the last two snapshots (%d):\n\n", len(changes))
				fmt.Fprintf(w, "%-30s %-18s %-16s %s\n", "THREAT", "FIELD", "FROM", "TO")
				for _, c := range changes {
					fmt.Fprintf(w, "%-30s %-18s %-16s %s\n", clip(orUnknown(c.ThreatName), 30), c.Field, orUnknown(c.From), orUnknown(c.To))
				}
				return nil
			}

			// Default: current distribution.
			threats, err := loadThreats(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading threats: %w", err)
			}
			if len(threats) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No threats in the local store. Run 'sentinelone-cli sync' first.", nil)
			}
			verdicts := map[string]int{}
			confidence := map[string]int{}
			incident := map[string]int{}
			for _, t := range threats {
				verdicts[orUnknown(threatVerdict(t))]++
				confidence[orUnknown(threatConfidence(t))]++
				incident[orUnknown(threatIncident(t))]++
			}
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"total_threats":    len(threats),
					"analyst_verdict":  verdicts,
					"confidence_level": confidence,
					"incident_status":  incident,
				})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Threat verdicts across %d threats:\n\n", len(threats))
			printDist(w, "Analyst verdict", verdicts)
			printDist(w, "Confidence level", confidence)
			printDist(w, "Incident status", incident)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/sentinelone-cli/data.db)")
	cmd.Flags().BoolVar(&changed, "changed", false, "Diff the two most recent snapshots and show only changed verdicts")
	return cmd
}

func printDist(w interface{ Write([]byte) (int, error) }, title string, m map[string]int) {
	type kv struct {
		k string
		v int
	}
	var kvs []kv
	for k, v := range m {
		kvs = append(kvs, kv{k, v})
	}
	sort.SliceStable(kvs, func(i, j int) bool { return kvs[i].v > kvs[j].v })
	fmt.Fprintf(w, "%s:\n", title)
	for _, p := range kvs {
		fmt.Fprintf(w, "    %-22s %d\n", p.k, p.v)
	}
	fmt.Fprintln(w)
}
