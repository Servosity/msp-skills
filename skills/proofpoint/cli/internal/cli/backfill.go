// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature for proofpoint-cli.

// pp:data-source live
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"proofpoint-pp-cli/internal/client"
	"proofpoint-pp-cli/internal/cliutil"
	"proofpoint-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// backfillWindow is one 1-hour-or-less interval the SIEM API will accept.
type backfillWindow struct {
	Start time.Time
	End   time.Time
}

// backfillResourcePaths maps sync resource names to their SIEM endpoint path
// and the envelope array key that carries the events for that endpoint.
var backfillResourcePaths = map[string]struct {
	Path string
	Key  string
}{
	"siem-clicks-blocked":     {Path: "/siem/clicks/blocked", Key: "clicksBlocked"},
	"siem-clicks-permitted":   {Path: "/siem/clicks/permitted", Key: "clicksPermitted"},
	"siem-messages-blocked":   {Path: "/siem/messages/blocked", Key: "messagesBlocked"},
	"siem-messages-delivered": {Path: "/siem/messages/delivered", Key: "messagesDelivered"},
}

// backfillResourceOrder is the deterministic iteration order for output.
var backfillResourceOrder = []string{
	"siem-clicks-blocked",
	"siem-clicks-permitted",
	"siem-messages-blocked",
	"siem-messages-delivered",
}

// chunkBackfillWindows splits [start, end) into intervals no longer than step.
// The SIEM API rejects windows over 1 hour and under 30 seconds, so trailing
// slivers shorter than 30s are dropped.
func chunkBackfillWindows(start, end time.Time, step time.Duration) []backfillWindow {
	if step <= 0 {
		step = time.Hour
	}
	var windows []backfillWindow
	for cur := start; cur.Before(end); cur = cur.Add(step) {
		wEnd := cur.Add(step)
		if wEnd.After(end) {
			wEnd = end
		}
		if wEnd.Sub(cur) < 30*time.Second {
			break
		}
		windows = append(windows, backfillWindow{Start: cur, End: wEnd})
	}
	return windows
}

// extractEnvelopeEvents pulls the named array out of a SIEM response envelope.
func extractEnvelopeEvents(data json.RawMessage, key string) ([]json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("parsing SIEM envelope: %w", err)
	}
	raw, ok := envelope[key]
	if !ok {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("parsing %s array: %w", key, err)
	}
	return items, nil
}

type backfillResourceResult struct {
	Resource      string `json:"resource"`
	Events        int    `json:"events"`
	WindowsOK     int    `json:"windows_ok"`
	WindowsFailed int    `json:"windows_failed"`
}

type backfillFailure struct {
	Resource string `json:"resource"`
	Window   string `json:"window"`
	Error    string `json:"error"`
}

type backfillView struct {
	Since         string                   `json:"since"`
	Windows       int                      `json:"windows"`
	APICalls      int                      `json:"api_calls"`
	TotalEvents   int                      `json:"total_events"`
	Resources     []backfillResourceResult `json:"resources"`
	FetchFailures []backfillFailure        `json:"fetch_failures,omitempty"`
	Note          string                   `json:"note,omitempty"`
}

func runBackfillWindows(ctx context.Context, c *client.Client, db *store.Store, resources []string, windows []backfillWindow) backfillView {
	view := backfillView{
		Windows:       len(windows),
		Resources:     make([]backfillResourceResult, 0, len(resources)),
		FetchFailures: make([]backfillFailure, 0),
	}
	successes := 0
	for _, resource := range resources {
		rp := backfillResourcePaths[resource]
		result := backfillResourceResult{Resource: resource}
		for _, w := range windows {
			// Fail fast on a broken setup: if the first three calls all fail
			// with zero successes (bad credentials, dead network), abort the
			// loop instead of burning the remaining windows.
			if successes == 0 && len(view.FetchFailures) >= 3 {
				view.TotalEvents += result.Events
				view.Resources = append(view.Resources, result)
				return view
			}
			interval := w.Start.UTC().Format(time.RFC3339) + "/" + w.End.UTC().Format(time.RFC3339)
			params := map[string]string{"interval": interval, "format": "json"}
			view.APICalls++
			data, err := c.GetNoCache(ctx, rp.Path, params)
			if err != nil {
				result.WindowsFailed++
				view.FetchFailures = append(view.FetchFailures, backfillFailure{Resource: resource, Window: interval, Error: err.Error()})
				continue
			}
			items, err := extractEnvelopeEvents(data, rp.Key)
			if err != nil {
				result.WindowsFailed++
				view.FetchFailures = append(view.FetchFailures, backfillFailure{Resource: resource, Window: interval, Error: err.Error()})
				continue
			}
			if len(items) > 0 {
				stored, _, err := db.UpsertBatch(resource, items)
				if err != nil {
					result.WindowsFailed++
					view.FetchFailures = append(view.FetchFailures, backfillFailure{Resource: resource, Window: interval, Error: err.Error()})
					continue
				}
				result.Events += stored
			}
			result.WindowsOK++
			successes++
		}
		view.TotalEvents += result.Events
		view.Resources = append(view.Resources, result)
	}
	return view
}

func newNovelBackfillCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagResources string
	var flagDB string

	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Reconstruct up to 7 days of SIEM threat events by auto-looping the API's 1-hour windows",
		Long: strings.Trim(`
Reconstruct up to 7 days of SIEM threat events in one command. The SIEM API
caps every request at a 1-hour window with a 7-day lookback; backfill chunks
the requested range into compliant windows, fetches each single-type feed,
and persists every event to the local SQLite store with stable IDs.

Use this command to build or repair the local event store. Do NOT use it for
a quick last-hour live check; use 'siem list-issues' instead.`, "\n"),
		Example: strings.Trim(`
  proofpoint-cli backfill --since 24h
  proofpoint-cli backfill --since 7d --resources siem-clicks-permitted,siem-messages-delivered
  proofpoint-cli backfill --since 48h --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:happy-args": "--since=1h"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			since := flagSince
			if since == "" {
				since = "24h"
			}
			start, err := parseSinceDuration(since)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--since: %v", err))
			}
			now := time.Now()
			note := ""
			if now.Sub(start) > 7*24*time.Hour {
				start = now.Add(-7 * 24 * time.Hour)
				note = "requested range exceeded the API's 7-day lookback; clamped to 7d"
			}

			selected := backfillResourceOrder
			if flagResources != "" {
				requested := cliutil.SplitCSV(flagResources)
				selected = make([]string, 0, len(requested))
				for _, name := range backfillResourceOrder {
					for _, want := range requested {
						if want == name {
							selected = append(selected, name)
						}
					}
				}
				if len(selected) != len(requested) {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--resources accepts only: %s", strings.Join(backfillResourceOrder, ", ")))
				}
			}

			windows := chunkBackfillWindows(start, now, time.Hour)
			if cliutil.IsDogfoodEnv() && len(windows) > 1 {
				windows = windows[len(windows)-1:]
			}

			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would backfill %d windows x %d resources (~%d API calls) since %s\n",
					len(windows), len(selected), len(windows)*len(selected), start.UTC().Format(time.RFC3339))
				return nil
			}

			if calls := len(windows) * len(selected); calls > 200 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: this backfill will spend %d of the shared 1800/24h SIEM API quota\n", calls)
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("proofpoint-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()

			view := runBackfillWindows(cmd.Context(), c, db, selected, windows)
			view.Since = start.UTC().Format(time.RFC3339)
			view.Note = note
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d window fetches failed; %d events stored from the remaining windows\n",
					len(view.FetchFailures), view.APICalls, view.TotalEvents)
			}
			if view.APICalls > 0 && len(view.FetchFailures) == view.APICalls {
				return apiErr(fmt.Errorf("every backfill window failed; first error: %s", view.FetchFailures[0].Error))
			}
			if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
				return err
			}
			// Partial window failures surface as exit 6 unless the caller
			// opted into tolerance with --allow-partial-failure.
			if len(view.FetchFailures) > 0 && !flags.allowPartialFailure {
				return partialFailureErr(fmt.Errorf("%d of %d backfill windows failed (pass --allow-partial-failure to tolerate)", len(view.FetchFailures), view.APICalls))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "24h", "How far back to reconstruct (e.g. 6h, 24h, 7d; API maximum 7d)")
	cmd.Flags().StringVar(&flagResources, "resources", "", "CSV of SIEM feeds to backfill (default: all four single-type feeds)")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path")
	return cmd
}
