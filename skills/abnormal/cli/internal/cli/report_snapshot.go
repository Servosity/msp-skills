// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: weekly reporting snapshot. Hand-authored; preserved across regenerations.

// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"abnormal-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

// snapshotSection names one dashboard aggregation pulled into the snapshot.
type snapshotSection struct {
	key  string
	path string
}

func snapshotSections() []snapshotSection {
	return []snapshotSection{
		{key: "attack_frequency", path: "/aggregations/attack_frequency"},
		{key: "attack_stopped", path: "/aggregations/attack_stopped"},
		{key: "most_impersonated_employee_vip", path: "/aggregations/most_impersonated_employee_vip"},
		{key: "most_impersonated_employee_non_vip", path: "/aggregations/most_impersonated_employee_non_vip"},
		{key: "sender_impersonation_breakdown", path: "/aggregations/sender_impersonation_breakdown"},
		{key: "trending_attacks", path: "/aggregations/trending_attacks"},
	}
}

type snapshotFailure struct {
	Section string `json:"section"`
	Error   string `json:"error"`
}

type snapshotView struct {
	Window        string                     `json:"window"`
	From          string                     `json:"from"`
	To            string                     `json:"to"`
	Sections      map[string]json.RawMessage `json:"sections"`
	FetchFailures []snapshotFailure          `json:"fetch_failures,omitempty"`
}

// flattenSnapshotRows converts the nested sections into flat CSV-friendly rows.
// Failed sections travel with the data as error rows so a partial snapshot is
// never silently presented as complete when only stdout is consumed.
func flattenSnapshotRows(view snapshotView) []map[string]any {
	rows := make([]map[string]any, 0)
	for section, raw := range view.Sections {
		var node any
		if err := json.Unmarshal(raw, &node); err != nil {
			continue
		}
		rows = append(rows, flattenNode(section, "", node)...)
	}
	for _, f := range view.FetchFailures {
		rows = append(rows, map[string]any{"section": f.Section, "metric": "fetch_error", "value": f.Error})
	}
	return rows
}

func flattenNode(section, prefix string, node any) []map[string]any {
	rows := make([]map[string]any, 0)
	switch v := node.(type) {
	case map[string]any:
		for k, val := range v {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			switch val.(type) {
			case map[string]any, []any:
				rows = append(rows, flattenNode(section, key, val)...)
			default:
				rows = append(rows, map[string]any{"section": section, "metric": key, "value": val})
			}
		}
	case []any:
		for i, item := range v {
			key := fmt.Sprintf("%s[%d]", prefix, i)
			switch item.(type) {
			case map[string]any, []any:
				rows = append(rows, flattenNode(section, key, item)...)
			default:
				rows = append(rows, map[string]any{"section": section, "metric": key, "value": item})
			}
		}
	default:
		rows = append(rows, map[string]any{"section": section, "metric": prefix, "value": v})
	}
	return rows
}

func newNovelReportSnapshotCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagSource string

	cmd := &cobra.Command{
		Use:   "report-snapshot",
		Short: "Consolidated dashboard aggregations for a client-ready security report",
		Long: strings.Trim(`
Use this command for a client-ready reporting snapshot across multiple live
dashboard endpoints (attack frequency, attacks stopped, VIP and non-VIP
impersonation, sender impersonation breakdown, trending attacks).
Do NOT use it for one metric; the generated 'aggregations' endpoint commands
return one each.

Failed sections are excluded from output and reported in fetch_failures so a
partial snapshot is never silently presented as complete.`, "\n"),
		Example: strings.Trim(`
  abnormal-cli report-snapshot --since 30d
  abnormal-cli report-snapshot --since 7d --csv
  abnormal-cli report-snapshot --since 90d --agent --select sections.trending_attacks`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if flags.dataSource == "local" {
				return usageErr(fmt.Errorf("report-snapshot reads the live dashboard aggregations; no local data source — use 'analytics --type threats' for local rollups"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would fetch %d dashboard aggregation sections for the last %s\n", len(snapshotSections()), flagSince)
				return nil
			}
			window, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --since %q: %w", flagSince, err))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			from := now.Add(-window)
			filter := fmt.Sprintf("receivedTime gte %s lte %s", from.Format("2006-01-02T15:04:05Z"), now.Format("2006-01-02T15:04:05Z"))
			params := map[string]string{"filter": filter}
			if flagSource != "" {
				params["source"] = flagSource
			}
			sections := snapshotSections()
			if cliutil.IsDogfoodEnv() && len(sections) > 2 {
				sections = sections[:2]
			}
			type result struct {
				idx  int
				key  string
				data json.RawMessage
				err  error
			}
			results := make(chan result, len(sections))
			var wg sync.WaitGroup
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			for idx, sec := range sections {
				wg.Add(1)
				go func(idx int, sec snapshotSection) {
					defer wg.Done()
					data, err := c.Get(ctx, sec.path, params)
					results <- result{idx: idx, key: sec.key, data: data, err: err}
				}(idx, sec)
			}
			go func() {
				wg.Wait()
				close(results)
			}()
			view := snapshotView{
				Window:        flagSince,
				From:          from.Format(time.RFC3339),
				To:            now.Format(time.RFC3339),
				Sections:      make(map[string]json.RawMessage, len(sections)),
				FetchFailures: make([]snapshotFailure, 0),
			}
			for r := range results {
				if r.err != nil {
					view.FetchFailures = append(view.FetchFailures, snapshotFailure{Section: r.key, Error: r.err.Error()})
					continue
				}
				view.Sections[r.key] = r.data
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d dashboard sections failed; snapshot covers the remaining %d sections\n", len(view.FetchFailures), len(sections), len(view.Sections))
			}
			if len(view.Sections) == 0 {
				return classifyAPIError(fmt.Errorf("all %d dashboard sections failed; first error: %s", len(sections), view.FetchFailures[0].Error), flags)
			}
			if flags.csv {
				return printJSONFiltered(cmd.OutOrStdout(), flattenSnapshotRows(view), flags)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "30d", "Reporting window ending now (e.g. 7d, 30d, 90d)")
	cmd.Flags().StringVar(&flagSource, "source", "", "Filter by detection source where the endpoint supports it (e.g. all, abnormal)")
	return cmd
}
