// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `eventlog new`. Recorded in handfixes.json
// (fixed-scripts).
// pp:data-source live
// pp:client-call the API call is a fixed WinRM script run through winsrv.Runner, not the HTTP client.

package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/winsrv"
)

func newNovelEventlogNewCmd(flags *rootFlags) *cobra.Command {
	var window, baseline string
	cmd := &cobra.Command{
		Use:   "new",
		Short: "System and Application errors and warnings whose source and event ID are new on a server compared with its own history.",
		Long: `Groups every critical, error and warning event in the System and
Application logs by (log, source, event ID) on each server, and lists the
groups seen in the last --window that never appeared in the --baseline before
it: the errors that are new for that server, not the ones it always has.

The counting runs on the server, so only the groups travel. When a log does
not reach back to the start of the baseline (it rolled over), or more than
20000 events were read, baseline_complete is false: a "new" group may then
be old history the log no longer holds.`,
		Example: "  windows-server-cli eventlog new --window 24h --baseline 7d --agent",
		Args:    cobra.NoArgs,
		Annotations: map[string]string{
			"mcp:read-only": "true", "pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			wh, err := parseHours("--window", window, 168)
			if err != nil {
				return usageErr(err)
			}
			bh, err := parseHours("--baseline", baseline, 720)
			if err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "eventlog new on "+winsrvTarget()+"")
			}
			profiles, credErrs, err := winsrvServers(flags, true)
			if err != nil {
				return err
			}
			runner := newWinsrvRunner(flags)
			results := eachServer(cmd.Context(), profiles, credErrs, func(ctx context.Context, p winsrv.Profile) (any, error) {
				raw, err := runner.Run(ctx, p, "eventlog_groups", map[string]any{"window_hours": wh, "baseline_hours": bh})
				if err != nil {
					return nil, err
				}
				res, err := decodeRow(raw)
				if err != nil {
					return nil, err
				}
				return newGroups(p.Name, res), nil
			})
			rows := []winsrv.Row{}
			for _, r := range results {
				if r.err != nil {
					rows = append(rows, errorRow(r.profile.Name, r.err))
					continue
				}
				rows = append(rows, r.value.([]winsrv.Row)...)
			}
			if err := printWinsrv(cmd, flags, rows, "server", "log", "provider", "event_id", "level_name", "window_count", "baseline_complete", "error"); err != nil {
				return err
			}
			return scriptExit(allFailed(results))
		},
	}
	cmd.Flags().StringVar(&window, "window", "24h", "Recent period to look for new errors in, as hours or days (1h to 7d)")
	cmd.Flags().StringVar(&baseline, "baseline", "7d", "History before the window to compare against, as hours or days (1h to 30d)")
	return cmd
}

// parseHours reads "36h" or "7d" as whole hours from 1 to max.
func parseHours(flag, v string, max int) (int, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	mult := 1
	switch {
	case strings.HasSuffix(v, "d"):
		mult, v = 24, strings.TrimSuffix(v, "d")
	case strings.HasSuffix(v, "h"):
		v = strings.TrimSuffix(v, "h")
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n*mult > max {
		return 0, fmt.Errorf("%s must be hours or days such as 24h or 7d, from 1h to %dh", flag, max)
	}
	return n * mult, nil
}

var levelNames = map[int]string{1: "Critical", 2: "Error", 3: "Warning"}

// newGroups keeps the groups seen in the window and never in the baseline.
func newGroups(server string, res winsrv.Row) []winsrv.Row {
	complete := !winsrv.Bool(res["capped"])
	start, _ := winsrv.Time(res["baseline_start"])
	oldest, _ := res["oldest_record"].(map[string]any)
	logComplete := map[string]bool{}
	for _, log := range []string{"System", "Application"} {
		t, ok := winsrv.Time(oldest[log])
		logComplete[log] = complete && (!ok || !t.After(start))
	}
	out := []winsrv.Row{}
	groups, _ := res["groups"].([]any)
	for _, g := range groups {
		row, ok := g.(map[string]any)
		if !ok {
			continue
		}
		wc, _ := winsrv.Num(row["window_count"])
		bc, _ := winsrv.Num(row["baseline_count"])
		if wc == 0 || bc > 0 {
			continue
		}
		log := winsrv.Str(row["log"])
		row["server"] = server
		row["id"] = fmt.Sprintf("%s/%s/%s/%s", server, log, winsrv.Str(row["provider"]), winsrv.Str(row["event_id"]))
		row["baseline_complete"] = logComplete[log]
		lvl, _ := winsrv.Num(row["level"])
		row["level_name"] = levelNames[int(lvl)]
		delete(row, "baseline_count")
		out = append(out, row)
	}
	// Most severe first (1 Critical, 2 Error, 3 Warning), then newest.
	sort.Slice(out, func(i, j int) bool {
		li, _ := winsrv.Num(out[i]["level"])
		lj, _ := winsrv.Num(out[j]["level"])
		if li != lj {
			return li < lj
		}
		a, _ := winsrv.Time(out[i]["last_seen"])
		b, _ := winsrv.Time(out[j]["last_seen"])
		return a.After(b) || a.Equal(b) && winsrv.Str(out[i]["id"]) < winsrv.Str(out[j]["id"])
	})
	return out
}
