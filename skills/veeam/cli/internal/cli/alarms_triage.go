// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type triagedAlarm struct {
	Company   string `json:"company"`
	Severity  string `json:"severity"`
	Alarm     string `json:"alarm"`
	Object    string `json:"object,omitempty"`
	Count     int    `json:"count"`
	FirstSeen string `json:"first_seen,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`
}

type alarmsTriageView struct {
	Alarms []triagedAlarm `json:"alarms"`
	Count  int            `json:"count"`
}

func newNovelAlarmsTriageCmd(flags *rootFlags) *cobra.Command {
	var flagSeverity, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "alarms-triage",
		Short: "Active alarms grouped by company and severity with first-seen / last-seen, deduped for fleet-wide triage.",
		Long: strings.Trim(`
Collapse the fleet's active alarms into distinct problems: group by company,
severity, and alarm, aggregating repeat activations into one row with a count
and first/last activation times. Filter to a single severity with --severity
(Error, Warning, Info).

Reads only the local SQLite mirror — run `+"`veeam-cli sync`"+` first.`, "\n"),
		Example: strings.Trim(`
  veeam-cli alarms-triage
  veeam-cli alarms-triage --severity Error
  veeam-cli alarms-triage --severity Error --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			db, ok, err := veeamOpenStoreRead(dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			if ok {
				defer db.Close()
			}

			wantSev := strings.ToLower(strings.TrimSpace(flagSeverity))
			names := veeamCompanyNames(ctx, db)
			alarms, _ := veeamLoad(ctx, db, "alarms")

			type agg struct {
				row   triagedAlarm
				first time.Time
				last  time.Time
			}
			groups := map[string]*agg{}
			order := []string{}

			for _, al := range alarms {
				sev := vstr(al, "lastActivation.status")
				if wantSev != "" && strings.ToLower(sev) != wantSev {
					continue
				}
				company := veeamCompanyLabel(names, vstr(al, "object.organizationUid"))
				alarmName := firstNonEmpty(vstr(al, "name"), vstr(al, "alarmTemplateUid"), "(alarm)")
				object := firstNonEmpty(vstr(al, "object.objectName"), vstr(al, "object.computerName"), vstr(al, "object.type"))
				key := strings.ToLower(company + "|" + sev + "|" + alarmName + "|" + object)
				g, ok := groups[key]
				if !ok {
					g = &agg{row: triagedAlarm{Company: company, Severity: sev, Alarm: alarmName, Object: object}}
					groups[key] = g
					order = append(order, key)
				}
				// repeatCount captures upstream repeats; default to 1.
				inc := 1
				if rc := int(vnum(al, "repeatCount")); rc > 0 {
					inc = rc
				}
				g.row.Count += inc
				if t, ok := vtime(al, "lastActivation.time"); ok {
					if g.last.IsZero() || t.After(g.last) {
						g.last = t
					}
					if g.first.IsZero() || t.Before(g.first) {
						g.first = t
					}
				}
			}

			out := make([]triagedAlarm, 0, len(order))
			for _, key := range order {
				g := groups[key]
				if !g.first.IsZero() {
					g.row.FirstSeen = g.first.UTC().Format(time.RFC3339)
				}
				if !g.last.IsZero() {
					g.row.LastSeen = g.last.UTC().Format(time.RFC3339)
				}
				out = append(out, g.row)
			}
			// Most urgent first: severity, then count.
			sort.SliceStable(out, func(i, j int) bool {
				ri, rj := veeamSeverityRank(out[i].Severity), veeamSeverityRank(out[j].Severity)
				if ri != rj {
					return ri < rj
				}
				if out[i].Count != out[j].Count {
					return out[i].Count > out[j].Count
				}
				return strings.ToLower(out[i].Company) < strings.ToLower(out[j].Company)
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}

			view := alarmsTriageView{Alarms: out, Count: len(out)}
			table := make([]map[string]any, 0, len(out))
			for _, a := range out {
				table = append(table, map[string]any{
					"company":   a.Company,
					"severity":  a.Severity,
					"alarm":     a.Alarm,
					"object":    a.Object,
					"count":     a.Count,
					"last_seen": a.LastSeen,
				})
			}
			return veeamEmit(cmd, flags, view, table, "No active alarms in the local mirror. Run `veeam-cli sync` first, then re-check.")
		},
	}
	cmd.Flags().StringVar(&flagSeverity, "severity", "", "Only show this severity (Error, Warning, Info)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum distinct alarms to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite mirror path (default: standard cache location)")
	return cmd
}

// firstNonEmpty returns the first non-blank string, or "" if all are blank.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
