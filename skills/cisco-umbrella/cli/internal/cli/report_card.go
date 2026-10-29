// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-umbrella-pp-cli/internal/client"
	"cisco-umbrella-pp-cli/internal/config"
)

type reportWindow struct {
	From int64 `json:"from_ms"`
	To   int64 `json:"to_ms"`
}

// metricDelta compares one summary metric; Prior and Delta are null when the
// prior window did not return a number for it.
type metricDelta struct {
	Metric  string   `json:"metric"`
	Current float64  `json:"current"`
	Prior   *float64 `json:"prior"`
	Delta   *float64 `json:"delta"`
}

// rankedDelta compares one ranked item across windows. Prior and Delta are null
// when the item fell outside the prior window's top 100, so its prior count is unknown.
type rankedDelta struct {
	Name    string   `json:"name"`
	Current float64  `json:"current"`
	Prior   *float64 `json:"prior"`
	Delta   *float64 `json:"delta"`
}

// deploymentCount is one identity type's deployment status; counts are null
// when the API did not return a number.
type deploymentCount struct {
	Label  string   `json:"label"`
	Count  *float64 `json:"count"`
	Active *float64 `json:"active"`
}

type reportCard struct {
	Org                  string            `json:"org"`
	Window               reportWindow      `json:"window"`
	PriorWindow          reportWindow      `json:"prior_window"`
	Summary              []metricDelta     `json:"summary"`
	TopThreatTypes       []rankedDelta     `json:"top_threat_types"` // null when the section failed
	TopBlockedCategories []rankedDelta     `json:"top_blocked_categories"`
	DeploymentStatus     []deploymentCount `json:"deployment_status"`
	Notes                []string          `json:"notes,omitempty"`
}

func newNovelReportCardCmd(flags *rootFlags) *cobra.Command {
	var from, to string
	var top int
	cmd := &cobra.Command{
		Use:   "report-card",
		Short: "A customer's security summary, top threat types, blocked categories, and deployment status vs the prior window",
		Long: `Compose the summary, top threat types, top blocked categories, and deployment
status reports for a window. Summary, threat types, and blocked categories are
compared with the equal-length window before it; deployment status is current-window
only. Needs the Reports scope (a 403 on the summary fails the command).
Run it from a customer's own key, or from a parent console key with --org <child id>.
--csv emits the summary metrics as metric,current,prior,delta rows.

Use this command for one customer's security metrics over a time window with a
prior-period comparison. Do NOT use this command for deployment health across all
customers; use 'fleet health' instead.`,
		Example: strings.Trim(`
  cisco-umbrella-cli report-card --from -30days --csv
  cisco-umbrella-cli report-card --org 8675309 --from -7days --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "--from=-7days"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "report-card")
			}
			now := time.Now()
			fromT, err := parseReportTime(from, now)
			if err != nil {
				return usageErr(fmt.Errorf("--from: %w", err))
			}
			toT, err := parseReportTime(to, now)
			if err != nil {
				return usageErr(fmt.Errorf("--to: %w", err))
			}
			if !fromT.Before(toT) {
				return usageErr(fmt.Errorf("--from must be before --to"))
			}
			if top < 1 || top > 100 {
				return usageErr(fmt.Errorf("--top must be between 1 and 100"))
			}
			cur := reportWindow{From: fromT.UnixMilli(), To: toT.UnixMilli()}
			prior := reportWindow{From: cur.From - (cur.To - cur.From), To: cur.From}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if err := requireCredentials(c); err != nil {
				return err
			}
			card := reportCard{Org: config.ChildOrgID(), Window: cur, PriorWindow: prior, Summary: []metricDelta{}, TopThreatTypes: []rankedDelta{}, TopBlockedCategories: []rankedDelta{}, DeploymentStatus: []deploymentCount{}}
			if card.Org == "" {
				card.Org = "key's own org"
			}

			curSum, err := reportObject(ctx, c, "/reports/v2/summary", cur, nil)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			priorSum, err := reportObject(ctx, c, "/reports/v2/summary", prior, nil)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			for _, k := range sortedKeys(curSum) {
				cv, ok := curSum[k].(float64)
				if !ok {
					card.Notes = append(card.Notes, "summary metric "+k+" has no numeric value in the current window")
					continue
				}
				row := metricDelta{Metric: k, Current: cv}
				if pv, ok := priorSum[k].(float64); ok {
					d := cv - pv
					row.Prior, row.Delta = &pv, &d
				}
				card.Summary = append(card.Summary, row)
			}

			limit := map[string]string{"limit": strconv.Itoa(top), "offset": "0"}
			card.TopThreatTypes, err = rankedReport(ctx, c, "/reports/v2/top-threat-types", cur, prior, limit, func(m map[string]any) (string, float64, bool) {
				n, ok := m["count"].(float64)
				return anyString(m["threattype"]), n, ok
			})
			if err != nil {
				card.TopThreatTypes = nil // null = not checked; [] = checked, nothing ranked
				card.Notes = append(card.Notes, "top threat types: "+err.Error())
			}
			blocked := map[string]string{"limit": strconv.Itoa(top), "offset": "0", "verdict": "blocked"}
			card.TopBlockedCategories, err = rankedReport(ctx, c, "/reports/v2/top-categories", cur, prior, blocked, func(m map[string]any) (string, float64, bool) {
				n, ok := m["count"].(float64)
				cat, _ := m["category"].(map[string]any)
				return anyString(cat["label"]), n, ok
			})
			if err != nil {
				card.TopBlockedCategories = nil
				card.Notes = append(card.Notes, "top blocked categories: "+err.Error())
			}
			deploy, err := reportRows(ctx, c, "/reports/v2/deployment-status", cur, nil)
			if err != nil {
				card.DeploymentStatus = nil
				card.Notes = append(card.Notes, "deployment status: "+err.Error())
			}
			for i, m := range deploy {
				if m == nil {
					card.Notes = append(card.Notes, fmt.Sprintf("deployment status row %d is empty; not reported", i))
					continue
				}
				// The schema nests the label under type (IdentityType); one example puts it top-level.
				label := ""
				if t, ok := m["type"].(map[string]any); ok {
					label = anyString(t["label"])
				}
				if label == "" {
					label = anyString(m["label"])
				}
				row := deploymentCount{Label: label}
				if n, ok := m["count"].(float64); ok {
					row.Count = &n
				}
				if a, ok := m["activecount"].(float64); ok {
					row.Active = &a
				}
				card.DeploymentStatus = append(card.DeploymentStatus, row)
			}

			if flags.csv {
				return printJSONFiltered(cmd.OutOrStdout(), card.Summary, flags)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), card, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Report card for %s: %s to %s (prior window %s to %s)\n\n", card.Org,
				time.UnixMilli(cur.From).UTC().Format(time.RFC3339), time.UnixMilli(cur.To).UTC().Format(time.RFC3339),
				time.UnixMilli(prior.From).UTC().Format(time.RFC3339), time.UnixMilli(prior.To).UTC().Format(time.RFC3339))
			rows := make([]map[string]any, 0, len(card.Summary))
			for _, s := range card.Summary {
				rows = append(rows, map[string]any{"metric": s.Metric, "current": s.Current, "prior": fmtNum(s.Prior), "delta": fmtNum(s.Delta)})
			}
			if err := printAutoTable(w, rows); err != nil {
				return err
			}
			for _, sec := range []struct {
				title string
				rows  []rankedDelta
			}{{"Top threat types", card.TopThreatTypes}, {"Top blocked categories", card.TopBlockedCategories}} {
				fmt.Fprintf(w, "\n%s\n", sec.title)
				for _, r := range sec.rows {
					prior, delta := "?", "?"
					if r.Prior != nil {
						prior, delta = strconv.FormatFloat(*r.Prior, 'f', -1, 64), strconv.FormatFloat(*r.Delta, 'f', -1, 64)
					}
					fmt.Fprintf(w, "  %-40s %10.0f  prior %s  delta %s\n", r.Name, r.Current, prior, delta)
				}
			}
			fmt.Fprintln(w, "\nDeployment status (current window)")
			for _, d := range card.DeploymentStatus {
				fmt.Fprintf(w, "  %-40s %s active of %s\n", d.Label, fmtNum(d.Active), fmtNum(d.Count))
			}
			for _, n := range card.Notes {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %s\n", n)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "-30days", "Window start: epoch ms, 'now', or relative like -30days, -12hours, -2weeks")
	cmd.Flags().StringVar(&to, "to", "now", "Window end: epoch ms, 'now', or relative like -1days")
	cmd.Flags().IntVar(&top, "top", 10, "Rows per ranked section (1-100)")
	return cmd
}

var relativeTime = regexp.MustCompile(`^-(\d+)(minutes?|hours?|days?|weeks?)$`)

// parseReportTime accepts the Umbrella reporting time forms: epoch milliseconds,
// "now", or a relative offset such as -30days. The card needs absolute times to
// compute the prior window.
func parseReportTime(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "now" {
		return now, nil
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil && ms > 0 {
		return time.UnixMilli(ms), nil
	}
	m := relativeTime.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, fmt.Errorf("want epoch ms, now, or -N(minutes|hours|days|weeks); got %q", s)
	}
	n, err := strconv.Atoi(m[1])
	unit := map[byte]time.Duration{'m': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour}[m[2][0]]
	// Reject offsets past ~100 years: they would overflow time.Duration and wrap.
	if err != nil || n <= 0 || time.Duration(n) > 100*365*24*time.Hour/unit {
		return time.Time{}, fmt.Errorf("relative time %q is out of range", s)
	}
	return now.Add(-time.Duration(n) * unit), nil
}

func windowParams(w reportWindow, extra map[string]string) map[string]string {
	p := map[string]string{"from": strconv.FormatInt(w.From, 10), "to": strconv.FormatInt(w.To, 10)}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

func reportObject(ctx context.Context, c *client.Client, path string, w reportWindow, extra map[string]string) (map[string]any, error) {
	raw, err := c.Get(ctx, path, windowParams(w, extra))
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := decodeObject(unwrapData(raw), &out); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return out, nil
}

func reportRows(ctx context.Context, c *client.Client, path string, w reportWindow, extra map[string]string) ([]map[string]any, error) {
	raw, err := c.Get(ctx, path, windowParams(w, extra))
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0)
	if err := decodeArray(unwrapData(raw), &out); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return out, nil
}

// priorRankLimit is the reports API maximum page size; the prior window is read
// this deep so current leaders that ranked lower before still get a prior count.
const priorRankLimit = 100

// rankedReport joins one ranked report across the current and prior windows by name.
func rankedReport(ctx context.Context, c *client.Client, path string, cur, prior reportWindow, extra map[string]string, key func(map[string]any) (string, float64, bool)) ([]rankedDelta, error) {
	curRows, err := reportRows(ctx, c, path, cur, extra)
	if err != nil {
		return []rankedDelta{}, err
	}
	priorExtra := map[string]string{}
	for k, v := range extra {
		priorExtra[k] = v
	}
	priorExtra["limit"], priorExtra["offset"] = strconv.Itoa(priorRankLimit), "0"
	priorRows, err := reportRows(ctx, c, path, prior, priorExtra)
	if err != nil {
		return []rankedDelta{}, err
	}
	priorBy := map[string]float64{}
	// A full page may have cut off lower items, and a row without a numeric count
	// leaves its item's prior unknown, so absence only means zero when neither happened.
	priorComplete := len(priorRows) < priorRankLimit
	for _, m := range priorRows {
		name, n, ok := key(m)
		if !ok || strings.TrimSpace(name) == "" {
			priorComplete = false // an unusable prior row could be any item
			continue
		}
		priorBy[name] += n
	}
	out := make([]rankedDelta, 0, len(curRows))
	for _, m := range curRows {
		name, n, ok := key(m)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%s: a ranked row has no name or numeric count", path)
		}
		row := rankedDelta{Name: name, Current: n}
		if p, ok := priorBy[name]; ok || priorComplete {
			d := n - p
			row.Prior, row.Delta = &p, &d
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Current > out[j].Current })
	if top, err := strconv.Atoi(extra["limit"]); err == nil && top > 0 && len(out) > top {
		out = out[:top] // not every ranked report honors limit
	}
	return out, nil
}

// fmtNum renders a nullable metric for humans; "?" means the API gave no number.
func fmtNum(v *float64) string {
	if v == nil {
		return "?"
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}
