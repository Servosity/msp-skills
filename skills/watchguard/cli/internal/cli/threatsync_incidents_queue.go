// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type queueRow struct {
	ID                      string   `json:"id"`
	AccountID               string   `json:"account_id"`
	AccountName             string   `json:"account_name,omitempty"`
	Severity                string   `json:"severity"`
	ThreatScore             int      `json:"threat_score"`
	Status                  string   `json:"status"`
	Category                string   `json:"category,omitempty"`
	Type                    string   `json:"type,omitempty"`
	Timestamp               string   `json:"timestamp,omitempty"`
	AgeHours                int      `json:"age_hours"`
	Untouched               bool     `json:"untouched"`
	Remediated              bool     `json:"remediated"`
	Performed               []string `json:"actions_performed"`
	RecommendedNotPerformed []string `json:"recommended_not_performed"`
	Comments                int      `json:"comments"`
}

type queueView struct {
	IncidentsScanned int        `json:"incidents_scanned"`
	Queue            []queueRow `json:"queue"`
	Truncated        bool       `json:"truncated"`
	NameLookupErr    string     `json:"account_names_error,omitempty"`
	Note             string     `json:"note,omitempty"`
}

var incidentStatuses = map[string]bool{"new": true, "read": true, "closed": true}

// queueFromIncidents keeps incidents whose status is in statuses, flattens the
// GUID-keyed entities map into performed vs recommended-but-not-performed
// actions, and ranks by severity, threat score, then age (oldest first).
func queueFromIncidents(items []map[string]any, statuses map[string]bool, minSeverity int, names map[string]string, now time.Time) []queueRow {
	out := make([]queueRow, 0)
	for _, m := range items {
		status := strings.ToLower(strField(m, "status"))
		if len(statuses) > 0 && !statuses[status] {
			continue
		}
		if sevRank(strField(m, "severity")) < minSeverity {
			continue
		}
		score, _ := intField(m, "threatScore")
		row := queueRow{
			ID: strField(m, "id"), AccountID: strField(m, "account"), Severity: strField(m, "severity"),
			ThreatScore: score, Status: status, Category: strField(m, "category"), Type: strField(m, "type"),
			Timestamp: strField(m, "timestamp"), Remediated: boolField(m, "remediated"),
			Performed: make([]string, 0), RecommendedNotPerformed: make([]string, 0),
		}
		row.AccountName = names[row.AccountID]
		if ts, ok := parseTime(row.Timestamp); ok {
			row.AgeHours = int(now.Sub(ts).Hours())
		}
		if cs, ok := m["comments"].([]any); ok {
			row.Comments = len(cs)
		}
		row.Performed, row.RecommendedNotPerformed = entityActions(m)
		row.Untouched = status == "new" && row.Comments == 0 && len(row.Performed) == 0
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if sa, sb := sevRank(a.Severity), sevRank(b.Severity); sa != sb {
			return sa > sb
		}
		if a.ThreatScore != b.ThreatScore {
			return a.ThreatScore > b.ThreatScore
		}
		return a.AgeHours > b.AgeHours
	})
	return out
}

// entityActions flattens the GUID-keyed entities map. A recommendation counts
// as taken only when the same entity performed the same command on the same
// MD5; labels carry the MD5 when the action targets one.
func entityActions(incident map[string]any) (performed, open []string) {
	performed, open = make([]string, 0), make([]string, 0)
	ents, _ := incident["entities"].(map[string]any)
	label := func(a map[string]any) string {
		params, _ := a["parameters"].(map[string]any)
		if md5 := strField(params, "md5"); md5 != "" {
			return strField(a, "command") + ":" + md5
		}
		return strField(a, "command")
	}
	seenP, seenO := map[string]bool{}, map[string]bool{}
	for _, k := range sortedMapKeys(ents) {
		ent, _ := ents[k].(map[string]any)
		done := ""
		if ap, ok := ent["actionPerformed"].(map[string]any); ok && strField(ap, "command") != "" {
			done = label(ap)
			if !seenP[done] {
				seenP[done] = true
				performed = append(performed, done)
			}
		}
		recs, _ := ent["recommendedActions"].([]any)
		for _, r := range recs {
			rm, _ := r.(map[string]any)
			if strField(rm, "command") == "" {
				continue
			}
			if l := label(rm); l != done && !seenO[l] {
				seenO[l] = true
				open = append(open, l)
			}
		}
	}
	return performed, open
}

// parseStatuses validates --status: new, read, closed or all (comma-separated).
func parseStatuses(s string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, v := range strings.Split(strings.ToLower(s), ",") {
		v = strings.TrimSpace(v)
		switch {
		case v == "" || v == "all":
		case incidentStatuses[v]:
			out[v] = true
		default:
			return nil, usageErr(fmt.Errorf("--status %q: use new, read, closed or all", v))
		}
	}
	return out, nil
}

func newNovelThreatsyncIncidentsQueueCmd(flags *rootFlags) *cobra.Command {
	var duration, status, minSeverity string
	var ownOnly bool
	var limit, maxPages int
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Rank open ThreatSync incidents across every subscriber by severity, threat score and age (filter with --status, --min-severity, --duration)",
		Long: "Use this command for the incident-level ThreatSync work queue across subscribers. " +
			"Do NOT use this command for per-customer incident counts alongside other products; use 'accounts health' instead.\n\n" +
			"Reads incidents for --account-id and, unless --own-only, every account it manages (ThreatSync tenants=true). " +
			"An incident is untouched when its status is new and it has no comments and no performed action. " +
			"recommended_not_performed lists entity recommendations (isolate, quarantineFile, ...) that no performed action covers.\n\n" +
			"Exit codes: 0 every read succeeded, 6 the account-name lookup failed (account_names_error; --allow-partial-failure exits 0), 5 the incident read failed.",
		Example: strings.Trim(`
  watchguard-cli threatsync-incidents queue --agent
  watchguard-cli threatsync-incidents queue --duration 7D --min-severity high --json
  watchguard-cli threatsync-incidents queue --status new --limit 20`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "threatsync-incidents queue")
			}
			statuses, err := parseStatuses(status)
			if err != nil {
				return err
			}
			dur, err := incidentDuration(duration)
			if err != nil {
				return err
			}
			minSev := 0
			if minSeverity != "" {
				if minSev = sevRank(minSeverity); minSev == 0 {
					return usageErr(fmt.Errorf("--min-severity must be one of low, medium, high, critical"))
				}
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			own, err := ownAccount(c)
			if err != nil {
				return err
			}
			pages := listPages(maxPages)
			items, truncated, err := fetchIncidents(ctx, c, own, dur, !ownOnly, pages)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			view := queueView{IncidentsScanned: len(items), Truncated: truncated}
			names := map[string]string{}
			if !ownOnly {
				if children, _, err := listChildren(ctx, c, own, 5); err != nil {
					view.NameLookupErr = err.Error()
				} else {
					for _, ch := range children {
						names[ch.ID] = ch.Name
					}
				}
			}
			view.Queue = queueFromIncidents(items, statuses, minSev, names, time.Now())
			if limit > 0 && len(view.Queue) > limit {
				view.Queue = view.Queue[:limit]
			}
			if truncated {
				view.Note = fmt.Sprintf("stopped after %d pages of 500 incidents; raise --max-pages or narrow --duration", pages)
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+view.Note)
			}
			// Hand-fix (handfixes.json fanout-partial-data-exit): a failed lookup
			// exits 6 like every other fan-out.
			namesFailed := 0
			if view.NameLookupErr != "" {
				namesFailed = 1
			}
			verdict := readVerdict(cmd.ErrOrStderr(), flags, 1, namesFailed, "incident and account-name reads")
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
				return verdict
			}
			if len(view.Queue) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No matching incidents (%d scanned).\n", view.IncidentsScanned)
				return verdict
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "SEVERITY\tSCORE\tAGE(h)\tACCOUNT\tSTATUS\tUNTOUCHED\tTYPE\tRECOMMENDED")
			for _, r := range view.Queue {
				fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\t%t\t%s\t%s\n", r.Severity, r.ThreatScore, r.AgeHours, subscriber{ID: r.AccountID, Name: r.AccountName}.label(),
					r.Status, r.Untouched, truncate(r.Type, 40), strings.Join(r.RecommendedNotPerformed, ","))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			return verdict
		},
	}
	cmd.Flags().StringVar(&duration, "duration", "7D", "Days of incidents to read, a number followed by D (e.g. 7D, 30D)")
	cmd.Flags().StringVar(&status, "status", "new,read", "Incident statuses to keep: new, read, closed, or all (comma-separated)")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "", "Lowest severity to keep: low, medium, high or critical")
	cmd.Flags().BoolVar(&ownOnly, "own-only", false, "Only the --account-id account, not the accounts it manages")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum incidents to return (0 = all)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, fmt.Sprintf("Maximum pages of 500 incidents to read (1-%d)", maxListPages))
	return cmd
}
