// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): live SLA breach watch.
// Computes breach / near-breach state from open-incident timestamps vs synced
// SLA definitions — the API stores SLAs but never surfaces live breach status.
// Exits 8 when an active breach exists so dashboards and pipelines can gate.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelSlaBreachCmd(flags *rootFlags) *cobra.Command {
	var within string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "sla-breach",
		Short: "List incidents breaching or about to breach their SLA target.",
		Long: `Use this command to find incidents breaching or near their SLA target right
now, sorted by time remaining. Joins open incidents to synced SLA definitions
(matched by severity when the SLA names one), computes elapsed vs target from
incident timestamps, and exits 8 when any active breach exists so a dashboard
or pipeline can gate on it.

Do NOT use this command for historical mean-time trends; use 'mttr' instead.
Do NOT use it for a per-service health rollup; use 'service-health' instead.`,
		Example: `  rootly-cli sla-breach --within 2h
  rootly-cli sla-breach --json`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:typed-exit-codes": "0,8"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			nearWindow := 2 * time.Hour
			if d, ok := parseWindowDuration(within); ok {
				nearWindow = d
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "incidents")
			if err != nil {
				return err
			}
			defer db.Close()

			// SLA targets: severity-scoped when the SLA names a severity,
			// otherwise global fallbacks.
			slas, _ := novelLoad(db, novelResolveType(db, "slas"))
			bySeverity := map[string]time.Duration{}
			var globalTargets []time.Duration
			for _, s := range slas {
				target, ok := slaTargetDuration(s.Attrs)
				if !ok {
					continue
				}
				sev := strings.ToLower(firstNonEmpty(recName(s.Attrs["severity"]), recName(s.Rels["severity"])))
				if sev != "" {
					if cur, exists := bySeverity[sev]; !exists || target < cur {
						bySeverity[sev] = target
					}
				} else {
					globalTargets = append(globalTargets, target)
				}
			}
			var globalTarget time.Duration
			for _, t := range globalTargets {
				if globalTarget == 0 || t < globalTarget {
					globalTarget = t
				}
			}

			incidents, err := novelLoad(db, novelResolveType(db, "incidents"))
			if err != nil {
				return err
			}

			now := time.Now()
			type row struct {
				ID           string `json:"id"`
				Title        string `json:"title"`
				Severity     string `json:"severity,omitempty"`
				Elapsed      string `json:"elapsed"`
				Target       string `json:"target"`
				Remaining    string `json:"remaining"`
				remainingDur time.Duration
				Breached     bool `json:"breached"`
			}
			var rows []row
			scanned := 0
			for _, r := range incidents {
				if !incidentOpen(r) {
					continue
				}
				scanned++
				start, ok := incidentStart(r)
				if !ok {
					continue
				}
				sev := strings.ToLower(incidentSeverity(r))
				target, exists := bySeverity[sev]
				if !exists {
					target = globalTarget
				}
				if target == 0 {
					continue // no applicable SLA
				}
				elapsed := now.Sub(start)
				remaining := target - elapsed
				breached := remaining <= 0
				if !breached && remaining > nearWindow {
					continue // healthy and not near
				}
				rows = append(rows, row{
					ID: r.ID, Title: incidentTitle(r), Severity: incidentSeverity(r),
					Elapsed: humanDuration(elapsed), Target: humanDuration(target),
					Remaining: humanDuration(remaining), remainingDur: remaining, Breached: breached,
				})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].remainingDur < rows[j].remainingDur })
			if rows == nil {
				rows = []row{}
			}

			breachCount := 0
			for _, r := range rows {
				if r.Breached {
					breachCount++
				}
			}
			note := ""
			if len(slas) == 0 {
				note = "no SLAs synced; run 'rootly-cli sync --resources slas,incidents' first"
			} else if len(bySeverity) == 0 && globalTarget == 0 {
				note = "synced SLAs had no parseable duration target; nothing to evaluate"
			}
			out := struct {
				OpenIncidents int    `json:"open_incidents_scanned"`
				Breached      int    `json:"breached"`
				NearBreach    int    `json:"near_breach"`
				Within        string `json:"near_window"`
				Items         []row  `json:"items"`
				Note          string `json:"note,omitempty"`
			}{OpenIncidents: scanned, Breached: breachCount, NearBreach: len(rows) - breachCount, Within: humanDuration(nearWindow), Items: rows, Note: note}

			renderErr := novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "SLA watch: %d breached, %d near-breach (within %s) of %d open incidents\n", breachCount, out.NearBreach, out.Within, scanned)
				for _, r := range rows {
					state := "NEAR"
					if r.Breached {
						state = "BREACH"
					}
					fmt.Fprintf(w, "  %-6s %-40.40s sev=%-10s elapsed=%-8s target=%-8s remaining=%s\n", state, r.Title, r.Severity, r.Elapsed, r.Target, r.Remaining)
				}
				if note != "" {
					fmt.Fprintf(w, "  note: %s\n", note)
				}
			})
			if renderErr != nil {
				return renderErr
			}
			if breachCount > 0 {
				return &cliError{code: 8, err: fmt.Errorf("sla-breach: %d incident(s) actively breaching SLA", breachCount)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&within, "within", "2h", "Near-breach window, e.g. 2h, 30m, 1d")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
